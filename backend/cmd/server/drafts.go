package main

import (
	"net/http"

	"github.com/VIACHMEL/GO-Edtech-project/backend/internal/db"
	"github.com/VIACHMEL/GO-Edtech-project/backend/internal/worksheetdata"
)

func worksheetDraft(w http.ResponseWriter, r *http.Request, store *db.DB, sessions *sessions) {
	w.Header().Set("Cache-Control", "no-store")
	user, err := currentUser(r, store, sessions)
	if err != nil {
		sendError(w, err)
		return
	}
	if user.Role != "teacher" {
		sendJSON(w, 403, map[string]string{"error": "Требуется преподаватель"})
		return
	}
	var id int64
	if r.PathValue("id") != "" {
		var ok bool
		id, ok = pathID(w, r, "id")
		if !ok {
			return
		}
	}
	if r.Method == "GET" {
		ws, tasks, err := store.GetWorksheetDraft(r.Context(), user.ID, id)
		if err != nil {
			sendError(w, err)
			return
		}
		sendJSON(w, 200, struct {
			worksheetdata.WorksheetDTO
			Tasks []db.DraftTask `json:"tasks"`
		}{worksheetdata.ToWorksheetDTO(ws), tasks})
		return
	}
	var input db.SaveDraftInput
	if !readJSON(w, r, &input) {
		return
	}
	ws, err := store.SaveWorksheetDraft(r.Context(), user.ID, id, input)
	if err != nil {
		sendError(w, err)
		return
	}
	status := 200
	if id == 0 {
		status = 201
	}
	sendJSON(w, status, worksheetdata.ToWorksheetDTO(ws))
}
