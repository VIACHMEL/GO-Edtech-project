package boardhttp

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/VIACHMEL/GO-Edtech-project/backend/internal/db"
)

// Actor resolves an existing authenticated session. No role or user ID is read from the URL.
type Actor func(*http.Request) (int64, bool)

type Handler struct {
	store  *db.DB
	actor  Actor
	origin string
	rooms  rooms
}

func Register(mux *http.ServeMux, store *db.DB, actor Actor, origin string) {
	h := &Handler{store: store, actor: actor, origin: origin}
	mux.HandleFunc("GET /api/v1/attempts/{id}/solve", h.solve)
	mux.HandleFunc("GET /ws/boards/{id}", h.upgrade)
	mux.HandleFunc("GET /api/v1/boards/{id}/files/{file}", h.getFile)
	mux.HandleFunc("PUT /api/v1/boards/{id}/files/{file}", h.putFile)
}

type boardRef struct {
	ID   int64  `json:"id"`
	Path string `json:"ws_path"`
}

type solveTask struct {
	ID       int64     `json:"id"`
	Title    string    `json:"title"`
	Kind     string    `json:"kind"`
	Prompt   string    `json:"prompt"`
	Answer   string    `json:"answer_text"`
	Revision int       `json:"answer_revision"`
	Board    *boardRef `json:"board,omitempty"`
}

type solvePage struct {
	ID        int64       `json:"id"`
	Title     string      `json:"title"`
	Status    string      `json:"status"`
	CanAnswer bool        `json:"can_answer"`
	IsTeacher bool        `json:"is_teacher"`
	Tasks     []solveTask `json:"tasks"`
}

func (h *Handler) endpoint(w http.ResponseWriter, r *http.Request) (actor, id int64, ok bool) {
	w.Header().Set("Cache-Control", "no-store")
	actor, ok = h.actor(r)
	if !ok || actor <= 0 {
		http.Error(w, "Authentication required", http.StatusUnauthorized)
		return 0, 0, false
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		http.NotFound(w, r)
		return 0, 0, false
	}
	return actor, id, true
}

func (h *Handler) sameOrigin(w http.ResponseWriter, r *http.Request) bool {
	if h.origin == "" || r.Header.Get("Origin") != h.origin {
		http.Error(w, "Invalid origin", http.StatusForbidden)
		return false
	}
	return true
}

func respond(w http.ResponseWriter, body any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(body)
}

func handleError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, db.ErrNotFound):
		http.Error(w, "Not found", http.StatusNotFound)
	case errors.Is(err, db.ErrValidation):
		http.Error(w, "Invalid request", http.StatusBadRequest)
	case errors.Is(err, db.ErrAttemptLocked), errors.Is(err, db.ErrWorkClosed),
		errors.Is(err, db.ErrConflict), errors.Is(err, db.ErrRevisionConflict):
		http.Error(w, "Attempt or revision conflict", http.StatusConflict)
	default:
		http.Error(w, "Board service unavailable", http.StatusServiceUnavailable)
	}
}

func (h *Handler) solve(w http.ResponseWriter, r *http.Request) {
	actor, id, ok := h.endpoint(w, r)
	if !ok {
		return
	}
	attempt, err := h.store.GetSolveAttempt(r.Context(), actor, id)
	if err != nil {
		handleError(w, err)
		return
	}
	result := solvePage{ID: attempt.ID, Title: attempt.Title, Status: attempt.Status,
		CanAnswer: attempt.CanAnswer, IsTeacher: attempt.IsTeacher, Tasks: make([]solveTask, 0, len(attempt.Tasks))}
	for _, task := range attempt.Tasks {
		item := solveTask{ID: task.ItemID, Title: fmt.Sprintf("Позиция %d", task.Position),
			Prompt: task.Prompt, Answer: task.Answer, Revision: task.Revision}
		if task.Part != nil {
			item.Kind = fmt.Sprintf("Часть %d", *task.Part)
		}
		if task.BoardID != 0 {
			item.Board = &boardRef{ID: task.BoardID, Path: fmt.Sprintf("/ws/boards/%d", task.BoardID)}
		}
		result.Tasks = append(result.Tasks, item)
	}
	respond(w, result)
}
