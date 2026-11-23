package main

import (
	"net/http"
	"strconv"

	"github.com/VIACHMEL/GO-Edtech-project/backend/internal/db"
	"github.com/VIACHMEL/GO-Edtech-project/backend/internal/worksheetdata"
)

// Each handler reads an existing session; teacher IDs never come from JSON.
func worksheets(w http.ResponseWriter, r *http.Request, store *db.DB, sessions *sessions) {
	user, err := currentUser(r, store, sessions)
	if err != nil {
		sendError(w, err)
		return
	}
	if user.Role != "teacher" {
		sendJSON(w, 403, map[string]string{"error": "Требуется преподаватель"})
		return
	}
	service := worksheetdata.NewService(store)
	if r.Method == "POST" {
		var input worksheetdata.CreateWorksheetRequest
		if !readJSON(w, r, &input) {
			return
		}
		result, err := service.CreateWorksheet(r.Context(), user.ID, input)
		if err != nil {
			sendError(w, err)
			return
		}
		sendJSON(w, 201, result)
		return
	}
	if r.PathValue("id") == "" {
		before, limit, err := pageParameters(r)
		if err != nil {
			sendError(w, err)
			return
		}
		result, err := service.ListWorksheets(r.Context(), user.ID, before, limit)
		if err != nil {
			sendError(w, err)
			return
		}
		sendJSON(w, 200, result)
		return
	}
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	if r.Method == "PUT" {
		var input worksheetdata.UpdateWorksheetRequest
		if !readJSON(w, r, &input) {
			return
		}
		if err := service.UpdateWorksheet(r.Context(), user.ID, id, input); err != nil {
			sendError(w, err)
			return
		}
		sendJSON(w, 200, map[string]bool{"ok": true})
		return
	}
	result, err := service.GetWorksheet(r.Context(), user.ID, id)
	if err != nil {
		sendError(w, err)
		return
	}
	sendJSON(w, 200, result)
}

func worksheetAction(w http.ResponseWriter, r *http.Request, store *db.DB, sessions *sessions) {
	user, err := currentUser(r, store, sessions)
	if err != nil {
		sendError(w, err)
		return
	}
	if user.Role != "teacher" {
		sendJSON(w, 403, map[string]string{"error": "Требуется преподаватель"})
		return
	}
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	service := worksheetdata.NewService(store)
	switch r.PathValue("action") {
	case "publish":
		result, err := service.PublishWorksheet(r.Context(), user.ID, id)
		if err != nil {
			sendError(w, err)
			return
		}
		sendJSON(w, 200, result)
	case "archive":
		if err := service.ArchiveWorksheet(r.Context(), user.ID, id); err != nil {
			sendError(w, err)
			return
		}
		sendJSON(w, 200, map[string]bool{"ok": true})
	case "clone":
		var input worksheetdata.CloneWorksheetRequest
		if !readJSON(w, r, &input) {
			return
		}
		result, err := service.CloneWorksheet(r.Context(), user.ID, id, input)
		if err != nil {
			sendError(w, err)
			return
		}
		sendJSON(w, 201, result)
	default:
		sendError(w, db.ErrNotFound)
	}
}

func worksheetItems(w http.ResponseWriter, r *http.Request, store *db.DB, sessions *sessions) {
	user, err := currentUser(r, store, sessions)
	if err != nil {
		sendError(w, err)
		return
	}
	if user.Role != "teacher" {
		sendJSON(w, 403, map[string]string{"error": "Требуется преподаватель"})
		return
	}
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	service := worksheetdata.NewService(store)
	if r.Method == "POST" {
		var input worksheetdata.AddWorksheetItemRequest
		if !readJSON(w, r, &input) {
			return
		}
		result, err := service.AddWorksheetItem(r.Context(), user.ID, id, input)
		if err != nil {
			sendError(w, err)
			return
		}
		sendJSON(w, 201, result)
		return
	}
	if r.PathValue("item_id") == "" {
		var input worksheetdata.ReorderWorksheetItemsRequest
		if !readJSON(w, r, &input) {
			return
		}
		err = service.ReorderWorksheetItems(r.Context(), user.ID, id, input)
	} else {
		itemID, ok := pathID(w, r, "item_id")
		if !ok {
			return
		}
		if r.Method == "DELETE" {
			err = service.RemoveWorksheetItem(r.Context(), user.ID, id, itemID)
		} else {
			var input worksheetdata.UpdateWorksheetItemRequest
			if !readJSON(w, r, &input) {
				return
			}
			err = service.UpdateWorksheetItem(r.Context(), user.ID, id, itemID, input)
		}
	}
	if err != nil {
		sendError(w, err)
		return
	}
	sendJSON(w, 200, map[string]bool{"ok": true})
}

func pathID(w http.ResponseWriter, r *http.Request, name string) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue(name), 10, 64)
	if err != nil || id <= 0 {
		sendError(w, db.ErrValidation)
		return 0, false
	}
	return id, true
}

func pageParameters(r *http.Request) (int64, int, error) {
	var before int64
	limit := 20
	var err error
	if value := r.URL.Query().Get("before_id"); value != "" {
		before, err = strconv.ParseInt(value, 10, 64)
		if err != nil || before < 0 {
			return 0, 0, db.ErrValidation
		}
	}
	if value := r.URL.Query().Get("limit"); value != "" {
		limit, err = strconv.Atoi(value)
		if err != nil || limit < 1 || limit > 100 {
			return 0, 0, db.ErrValidation
		}
	}
	return before, limit, nil
}
