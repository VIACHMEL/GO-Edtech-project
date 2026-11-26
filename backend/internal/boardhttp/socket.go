package boardhttp

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/VIACHMEL/GO-Edtech-project/backend/internal/db"
	"golang.org/x/net/websocket"
)

const (
	permissionRefresh = 3 * time.Second
	heartbeatInterval = 9 * time.Second
)

type boardChange struct {
	seq      int64
	elements []json.RawMessage
}

type rooms struct {
	mu    sync.Mutex
	count int
	peers map[int64]map[chan boardChange]int64
}

func (r *rooms) subscribe(id, actor int64) (chan boardChange, func()) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.count >= 256 || len(r.peers[id]) >= 24 {
		return nil, nil
	}
	actorCount := 0
	for _, peer := range r.peers[id] {
		if peer == actor {
			actorCount++
		}
	}
	if actorCount >= 12 {
		return nil, nil
	}
	if r.peers == nil {
		r.peers = make(map[int64]map[chan boardChange]int64)
	}
	if r.peers[id] == nil {
		r.peers[id] = make(map[chan boardChange]int64)
	}
	changes := make(chan boardChange, 16)
	r.peers[id][changes] = actor
	r.count++
	return changes, func() {
		r.mu.Lock()
		defer r.mu.Unlock()
		delete(r.peers[id], changes)
		r.count--
		if len(r.peers[id]) == 0 {
			delete(r.peers, id)
		}
	}
}

func (r *rooms) notify(id int64, sender chan boardChange, change boardChange) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for peer := range r.peers[id] {
		if peer == sender {
			continue
		}
		select {
		case peer <- change:
		default:
			// A gap is repaired with a snapshot, including on the next metadata check.
		}
	}
}

type frame struct {
	db.BoardSnapshot
	Type       string   `json:"type"`
	DraftOwner int64    `json:"draft_owner"`
	Ack        string   `json:"ack_op_id,omitempty"`
	Error      string   `json:"error,omitempty"`
	Conflicts  []string `json:"conflicts,omitempty"`
}

func (h *Handler) upgrade(w http.ResponseWriter, r *http.Request) {
	actor, id, ok := h.endpoint(w, r)
	if !ok || !h.sameOrigin(w, r) {
		return
	}
	changes, unsubscribe := h.rooms.subscribe(id, actor)
	if changes == nil {
		http.Error(w, "Too many board connections", http.StatusTooManyRequests)
		return
	}
	defer unsubscribe()

	snapshot, err := h.store.GetBoard(r.Context(), actor, id)
	if err != nil {
		handleError(w, err)
		return
	}
	websocket.Handler(func(conn *websocket.Conn) {
		h.serveBoard(conn, r, actor, id, snapshot, changes)
	}).ServeHTTP(w, r)
}

type boardConnection struct {
	handler *Handler
	socket  *websocket.Conn
	request *http.Request
	actor   int64
	id      int64
	lastSeq int64
	canEdit bool
}

func (h *Handler) serveBoard(conn *websocket.Conn, r *http.Request, actor, id int64, initial db.BoardSnapshot, changes chan boardChange) {
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	defer conn.Close()
	conn.MaxPayloadBytes = db.BoardMessageLimit
	incoming := make(chan wireOperation)
	go readOperations(ctx, conn, incoming)
	client := boardConnection{handler: h, socket: conn, request: r.WithContext(ctx), actor: actor, id: id}
	if !client.send(frame{Type: "snapshot", BoardSnapshot: initial}) {
		return
	}

	refresh := time.NewTicker(permissionRefresh)
	defer refresh.Stop()
	nextHeartbeat := time.Now().Add(heartbeatInterval)
	writeWindow := time.Now()
	writeCount := 0
	for {
		select {
		case <-ctx.Done():
			return
		case incomingOp, open := <-incoming:
			if !open {
				return
			}
			now := time.Now()
			if now.Sub(writeWindow) >= 10*time.Second {
				writeWindow, writeCount = now, 0
			}
			writeCount++
			if writeCount > 100 {
				return
			}
			if !client.apply(incomingOp, changes) {
				return
			}
			nextHeartbeat = now.Add(heartbeatInterval)
		case change := <-changes:
			if change.seq <= client.lastSeq {
				continue
			}
			if change.seq == client.lastSeq+1 {
				current, ok := client.readMeta()
				if !ok {
					return
				}
				if !client.send(frame{Type: "delta", BoardSnapshot: db.BoardSnapshot{
					Seq: change.seq, CanEdit: current.CanEdit, Elements: change.elements,
				}}) {
					return
				}
			} else if !client.refreshSnapshot(frame{Type: "snapshot"}) {
				return
			}
			nextHeartbeat = time.Now().Add(heartbeatInterval)
		case <-refresh.C:
			heartbeat := !time.Now().Before(nextHeartbeat)
			if !client.refresh(heartbeat) {
				return
			}
			if heartbeat {
				nextHeartbeat = time.Now().Add(heartbeatInterval)
			}
		}
	}
}

func (c *boardConnection) send(message frame) bool {
	message.DraftOwner = c.actor
	c.lastSeq, c.canEdit = message.Seq, message.CanEdit
	_ = c.socket.SetWriteDeadline(time.Now().Add(5 * time.Second))
	return websocket.JSON.Send(c.socket, message) == nil
}

func (c *boardConnection) authorized() bool {
	id, ok := c.handler.actor(c.request)
	if !ok || id != c.actor {
		_ = c.send(frame{Type: "access_revoked"})
		return false
	}
	return true
}

func (c *boardConnection) refreshSnapshot(message frame) bool {
	if !c.authorized() {
		return false
	}
	return c.loadSnapshot(message)
}

func (c *boardConnection) loadSnapshot(message frame) bool {
	current, err := c.handler.store.GetBoard(c.request.Context(), c.actor, c.id)
	if errors.Is(err, db.ErrNotFound) {
		_ = c.send(frame{Type: "access_revoked"})
	}
	if err != nil {
		if !errors.Is(err, db.ErrNotFound) {
			log.Printf("доска %d: обновление снимка прервано: %v", c.id, err)
		}
		return false
	}
	message.BoardSnapshot = current
	return c.send(message)
}

func (c *boardConnection) readMeta() (db.BoardMeta, bool) {
	if !c.authorized() {
		return db.BoardMeta{}, false
	}
	current, err := c.handler.store.GetBoardMeta(c.request.Context(), c.actor, c.id)
	if errors.Is(err, db.ErrNotFound) {
		_ = c.send(frame{Type: "access_revoked"})
	}
	if err != nil && !errors.Is(err, db.ErrNotFound) {
		log.Printf("доска %d: проверка ревизии прервана: %v", c.id, err)
	}
	return current, err == nil
}

func (c *boardConnection) refresh(heartbeat bool) bool {
	current, ok := c.readMeta()
	if !ok {
		return false
	}
	if current.Seq != c.lastSeq || current.CanEdit != c.canEdit {
		return c.loadSnapshot(frame{Type: "snapshot"})
	}
	if heartbeat {
		return c.send(frame{Type: "heartbeat", BoardSnapshot: db.BoardSnapshot{Seq: c.lastSeq, CanEdit: c.canEdit}})
	}
	return true
}

func (c *boardConnection) apply(input wireOperation, changes chan boardChange) bool {
	if !c.authorized() {
		return false
	}
	if input.BaseSeq == nil {
		return c.loadSnapshot(frame{Type: "snapshot", Ack: input.ID, Error: "upgrade_required"})
	}
	op := input.BoardOperation
	op.BaseSeq = *input.BaseSeq
	result, err := c.handler.store.ApplyBoardOperation(c.request.Context(), c.actor, c.id, op)
	reply := frame{Type: "snapshot", Ack: op.ID, BoardSnapshot: result.BoardSnapshot}
	switch {
	case errors.Is(err, db.ErrBoardHistoryExpired):
		reply.Error = "history_expired"
		return c.send(reply)
	case operationRejected(err):
		reply.Error = "operation_rejected"
		return c.loadSnapshot(reply)
	case errors.Is(err, db.ErrNotFound):
		_ = c.send(frame{Type: "access_revoked"})
		return false
	case err != nil:
		log.Printf("доска %d: операция прервана: %v", c.id, err)
		return false
	case len(result.Conflicts) != 0:
		reply.Error = "element_conflict"
		reply.Conflicts = result.Conflicts
		return c.send(reply)
	case result.Duplicate:
		return c.send(reply)
	}

	c.handler.rooms.notify(c.id, changes, boardChange{seq: result.Seq, elements: result.Delta})
	if result.Seq != c.lastSeq+1 {
		return c.loadSnapshot(reply)
	}
	reply.Type, reply.Elements = "delta", result.Delta
	return c.send(reply)
}

type wireOperation struct {
	db.BoardOperation
	BaseSeq *int64 `json:"base_seq"`
}

func readOperations(ctx context.Context, conn *websocket.Conn, incoming chan<- wireOperation) {
	defer close(incoming)
	for {
		var op wireOperation
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

func operationRejected(err error) bool {
	return errors.Is(err, db.ErrValidation) || errors.Is(err, db.ErrConflict) || errors.Is(err, db.ErrAttemptLocked) || errors.Is(err, db.ErrWorkClosed)
}
