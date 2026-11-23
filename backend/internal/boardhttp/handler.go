package boardhttp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"github.com/VIACHMEL/GO-Edtech-project/backend/internal/db"
	"github.com/VIACHMEL/GO-Edtech-project/backend/internal/learningdata"
)

// Actor resolves an existing authenticated session. No role or user ID is read from the URL.
type Actor func(*http.Request) (int64, bool)

type Handler struct {
	store    *db.DB
	learning *learningdata.Service
	actor    Actor
	origin   string
	rooms    rooms
}

func Register(mux *http.ServeMux, store *db.DB, actor Actor, origin string) {
	h := &Handler{store: store, learning: learningdata.NewService(store), actor: actor, origin: origin}
	mux.HandleFunc("GET /api/attempts/{id}/solve", h.solve)
	mux.HandleFunc("PUT /api/attempts/{id}/answers/{item}", h.saveAnswer)
	mux.HandleFunc("POST /api/attempts/{id}/submit", h.submit)
	mux.HandleFunc("GET /ws/boards/{id}", h.upgrade)
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
		CanAnswer: attempt.CanAnswer, Tasks: make([]solveTask, 0, len(attempt.Tasks))}
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

func (h *Handler) saveAnswer(w http.ResponseWriter, r *http.Request) {
	actor, id, ok := h.endpoint(w, r)
	if !ok || !h.sameOrigin(w, r) {
		return
	}
	item, err := strconv.ParseInt(r.PathValue("item"), 10, 64)
	if err != nil || item < 1 {
		http.NotFound(w, r)
		return
	}
	var input struct {
		Text     string `json:"text"`
		Revision *int   `json:"expected_revision"`
	}
	reader := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
	if reader.Decode(&input) != nil || input.Revision == nil || reader.Decode(new(any)) != io.EOF {
		handleError(w, db.ErrValidation)
		return
	}
	// Keep existing Data (e.g. attachments) when changing only the text.
	saved, err := h.saveAnswerText(r.Context(), actor, id, item, input.Text, *input.Revision)
	if err != nil {
		handleError(w, err)
		return
	}
	respond(w, saved)
}

func (h *Handler) saveAnswerText(ctx context.Context, actor, attempt, item int64, text string, revision int) (*db.SavedAnswer, error) {
	answers, err := h.learning.GetAttemptAnswers(ctx, actor, attempt)
	if err != nil {
		return nil, err
	}
	for _, answer := range answers {
		if answer.WorksheetItemID == item {
			return h.learning.SaveAnswer(ctx, actor, attempt, item, learningdata.SaveAnswerRequest{
				Text: text, Data: answer.Data, ExpectedRevision: revision,
			})
		}
	}
	return nil, db.ErrNotFound
}

func (h *Handler) submit(w http.ResponseWriter, r *http.Request) {
	actor, id, ok := h.endpoint(w, r)
	if !ok || !h.sameOrigin(w, r) {
		return
	}
	attempt, err := h.learning.SubmitAttempt(r.Context(), actor, id)
	if err != nil {
		handleError(w, err)
		return
	}
	respond(w, attempt)
}
