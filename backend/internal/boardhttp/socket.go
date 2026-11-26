package boardhttp

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/VIACHMEL/GO-Edtech-project/backend/internal/db"
	"golang.org/x/net/websocket"
)

type rooms struct {
	mu    sync.Mutex
	peers map[int64]map[chan struct{}]struct{}
}

func (r *rooms) subscribe(id int64) (chan struct{}, func()) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.peers == nil {
		r.peers = make(map[int64]map[chan struct{}]struct{})
	}
	if r.peers[id] == nil {
		r.peers[id] = make(map[chan struct{}]struct{})
	}
	wake := make(chan struct{}, 1)
	r.peers[id][wake] = struct{}{}
	return wake, func() {
		r.mu.Lock()
		defer r.mu.Unlock()
		delete(r.peers[id], wake)
		if len(r.peers[id]) == 0 {
			delete(r.peers, id)
		}
	}
}

func (r *rooms) notify(id int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for wake := range r.peers[id] {
		select {
		case wake <- struct{}{}:
		default:
		}
	}
}

type frame struct {
	db.BoardSnapshot
	Ack   string `json:"ack_op_id,omitempty"`
	Error string `json:"error,omitempty"`
}

func (h *Handler) upgrade(w http.ResponseWriter, r *http.Request) {
	actor, id, ok := h.endpoint(w, r)
	if !ok || !h.sameOrigin(w, r) {
		return
	}
	wake, unsubscribe := h.rooms.subscribe(id)
	defer unsubscribe()
	snapshot, err := h.store.GetBoard(r.Context(), actor, id)
	if err != nil {
		handleError(w, err)
		return
	}
	websocket.Handler(func(conn *websocket.Conn) {
		h.serveBoard(conn, r, actor, id, snapshot, wake)
	}).ServeHTTP(w, r)
}

func (h *Handler) serveBoard(conn *websocket.Conn, r *http.Request, actor, id int64, initial db.BoardSnapshot, wake <-chan struct{}) {
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	defer conn.Close()
	conn.MaxPayloadBytes = db.BoardMessageLimit
	incoming := make(chan db.BoardOperation)
	go readOperations(ctx, conn, incoming)
	last := initial
	if !sendFrame(conn, frame{BoardSnapshot: last}) {
		return
	}
	// Also refresh permissions after submission, and catch commits from another process.
	refresh := time.NewTicker(time.Second)
	defer refresh.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case op, open := <-incoming:
			if !open || !h.sameActor(r, actor) {
				return
			}
			result, err := h.store.ApplyBoardOperation(ctx, actor, id, op)
			if err == nil {
				h.rooms.notify(id)
				last = result
				if !sendFrame(conn, frame{BoardSnapshot: last, Ack: op.ID}) {
					return
				}
				continue
			}
			if !operationRejected(err) {
				return
			}
			last, err = h.store.GetBoard(ctx, actor, id)
			if err != nil || !sendFrame(conn, frame{BoardSnapshot: last, Ack: op.ID, Error: "operation_rejected"}) {
				return
			}
			continue
		case <-wake:
		case <-refresh.C:
		}
		if !h.sameActor(r, actor) {
			return
		}
		current, err := h.store.GetBoard(ctx, actor, id)
		if err != nil {
			return
		}
		if current.Seq != last.Seq || current.CanEdit != last.CanEdit {
			last = current
			if !sendFrame(conn, frame{BoardSnapshot: last}) {
				return
			}
		}
	}
}

func (h *Handler) sameActor(r *http.Request, actor int64) bool {
	id, ok := h.actor(r)
	return ok && actor == id
}

func readOperations(ctx context.Context, conn *websocket.Conn, incoming chan<- db.BoardOperation) {
	defer close(incoming)
	for {
		var op db.BoardOperation
		if websocket.JSON.Receive(conn, &op) != nil {
			return
		}
		select {
		case incoming <- op:
		case <-ctx.Done():
			return
		}
	}
}

func sendFrame(conn *websocket.Conn, message frame) bool {
	_ = conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	return websocket.JSON.Send(conn, message) == nil
}

func operationRejected(err error) bool {
	return errors.Is(err, db.ErrValidation) || errors.Is(err, db.ErrConflict) || errors.Is(err, db.ErrAttemptLocked)
}
