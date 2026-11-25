package main

import (
	"crypto/rand"
	"encoding/hex"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/VIACHMEL/GO-Edtech-project/backend/internal/db"
)

// Photos are private files; their names are stored in the existing answer_data.
func photos(w http.ResponseWriter, r *http.Request, store *db.DB, sessions *sessions, root string) {
	user, err := currentUser(r, store, sessions)
	if err != nil {
		sendError(w, err)
		return
	}
	attemptID, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	itemID, ok := pathID(w, r, "item_id")
	if !ok {
		return
	}
	attempt, err := store.GetAttempt(r.Context(), user.ID, attemptID)
	if err != nil {
		sendError(w, err)
		return
	}
	var text string
	var data map[string]any
	found := false
	if user.Role == "student" {
		answers, err := store.GetAttemptAnswers(r.Context(), user.ID, attemptID)
		if err != nil {
			sendError(w, err)
			return
		}
		for _, answer := range answers {
			if answer.WorksheetItemID == itemID {
				text, data, found = answer.Text, answer.Data, true
				break
			}
		}
	} else if user.Role == "teacher" {
		answers, err := store.GetTeacherLiveState(r.Context(), user.ID, attemptID)
		if err != nil {
			sendError(w, err)
			return
		}
		for _, answer := range answers {
			if answer.WorksheetItemID == itemID {
				data, found = answer.Data, true
				break
			}
		}
	}
	if !found {
		sendError(w, db.ErrNotFound)
		return
	}
	directory := filepath.Join(root, strconv.FormatInt(attemptID, 10), strconv.FormatInt(itemID, 10))
	if r.Method == "GET" {
		name := r.PathValue("filename")
		extension := filepath.Ext(name)
		raw, err := hex.DecodeString(strings.TrimSuffix(name, extension))
		if err != nil || len(raw) != 32 || (extension != ".jpg" && extension != ".png") {
			sendError(w, db.ErrNotFound)
			return
		}
		entries, _ := data["photos"].([]any)
		attached := false
		for _, entry := range entries {
			if storedName, ok := entry.(string); ok && storedName == name {
				attached = true
				break
			}
		}
		if !attached {
			sendError(w, db.ErrNotFound)
			return
		}
		file, err := os.Open(filepath.Join(directory, name))
		if err != nil {
			if os.IsNotExist(err) {
				sendError(w, db.ErrNotFound)
			} else {
				sendError(w, err)
			}
			return
		}
		defer file.Close()
		info, err := file.Stat()
		if err != nil {
			sendError(w, err)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if extension == ".jpg" {
			w.Header().Set("Content-Type", "image/jpeg")
		} else {
			w.Header().Set("Content-Type", "image/png")
		}
		http.ServeContent(w, r, name, info.ModTime(), file)
		return
	}
	if user.Role != "student" {
		sendJSON(w, 403, map[string]string{"error": "Загружать может только ученик"})
		return
	}
	revision, err := strconv.Atoi(r.URL.Query().Get("expected_revision"))
	if err != nil || revision < 0 {
		sendError(w, db.ErrValidation)
		return
	}
	if attempt.Status != "draft" {
		sendError(w, db.ErrAttemptLocked)
		return
	}
	content, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 8*1024*1024))
	if err != nil {
		sendJSON(w, 413, map[string]string{"error": "Фото не больше 8 МиБ"})
		return
	}
	extension := ""
	switch http.DetectContentType(content) {
	case "image/jpeg":
		extension = ".jpg"
	case "image/png":
		extension = ".png"
	default:
		sendJSON(w, 415, map[string]string{"error": "Допустимы JPEG и PNG"})
		return
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		sendError(w, err)
		return
	}
	name := hex.EncodeToString(raw) + extension
	if err := os.MkdirAll(directory, 0700); err != nil {
		sendError(w, err)
		return
	}
	path := filepath.Join(directory, name)
	if err := os.WriteFile(path, content, 0600); err != nil {
		sendError(w, err)
		return
	}
	if data == nil {
		data = make(map[string]any)
	}
	entries, _ := data["photos"].([]any)
	data["photos"] = append(entries, name)
	result, err := store.SaveAnswer(r.Context(), user.ID, attemptID, db.AnswerInput{WorksheetItemID: itemID, Text: text, Data: data, ExpectedRevision: revision})
	if err != nil {
		os.Remove(path)
		sendError(w, err)
		return
	}
	sendJSON(w, 201, map[string]any{"filename": name, "answer_revision": result.AnswerRevision})
}
