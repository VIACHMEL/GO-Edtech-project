package boardhttp

import (
	"errors"
	"io"
	"mime"
	"net/http"

	"github.com/VIACHMEL/GO-Edtech-project/backend/internal/db"
)

func (h *Handler) getFile(w http.ResponseWriter, r *http.Request) {
	actor, id, ok := h.endpoint(w, r)
	if !ok {
		return
	}
	contentType, data, err := h.store.GetBoardFile(r.Context(), actor, id, r.PathValue("file"))
	if err != nil {
		handleError(w, err)
		return
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cross-Origin-Resource-Policy", "same-origin")
	_, _ = w.Write(data)
}

func (h *Handler) putFile(w http.ResponseWriter, r *http.Request) {
	actor, id, ok := h.endpoint(w, r)
	if !ok || !h.sameOrigin(w, r) {
		return
	}
	contentType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || (contentType != "image/png" && contentType != "image/jpeg") {
		http.Error(w, "PNG or JPEG required", http.StatusUnsupportedMediaType)
		return
	}
	if r.ContentLength > db.BoardFileLimit {
		http.Error(w, "Image is too large", http.StatusRequestEntityTooLarge)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, db.BoardFileLimit)
	data, err := io.ReadAll(r.Body)
	if err != nil {
		var limit *http.MaxBytesError
		if errors.As(err, &limit) {
			http.Error(w, "Image is too large", http.StatusRequestEntityTooLarge)
		} else {
			http.Error(w, "Can't read image", http.StatusBadRequest)
		}
		return
	}
	err = h.store.PutBoardFile(r.Context(), actor, id, r.PathValue("file"), contentType, data)
	if errors.Is(err, db.ErrBoardFileLimit) {
		http.Error(w, "Board image limit reached", http.StatusRequestEntityTooLarge)
		return
	}
	if err != nil {
		handleError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
