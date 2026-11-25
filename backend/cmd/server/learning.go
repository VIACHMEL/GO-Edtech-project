package main

import (
	"net/http"
	"strings"

	"github.com/VIACHMEL/GO-Edtech-project/backend/internal/db"
	"github.com/VIACHMEL/GO-Edtech-project/backend/internal/learningdata"
)

func assignments(w http.ResponseWriter, r *http.Request, store *db.DB, sessions *sessions) {
	user, err := currentUser(r, store, sessions)
	if err != nil {
		sendError(w, err)
		return
	}
	role := "teacher"
	if strings.HasPrefix(r.URL.Path, "/api/v1/student/") {
		role = "student"
	}
	if user.Role != role {
		sendJSON(w, 403, map[string]string{"error": "Недопустимая роль"})
		return
	}
	service := learningdata.NewService(store)
	if r.Method == "POST" {
		if strings.HasSuffix(r.URL.Path, "/group") {
			var input learningdata.AssignWorksheetToGroupRequest
			if !readJSON(w, r, &input) {
				return
			}
			result, err := service.AssignWorksheetToGroup(r.Context(), user.ID, input)
			if err != nil {
				sendError(w, err)
				return
			}
			sendJSON(w, 201, result)
		} else {
			var input learningdata.AssignWorksheetRequest
			if !readJSON(w, r, &input) {
				return
			}
			result, err := service.AssignWorksheet(r.Context(), user.ID, input)
			if err != nil {
				sendError(w, err)
				return
			}
			sendJSON(w, 201, result)
		}
		return
	}
	before, limit, err := pageParameters(r)
	if err != nil {
		sendError(w, err)
		return
	}
	var result []learningdata.AssignmentDTO
	if role == "teacher" {
		result, err = service.ListTeacherAssignments(r.Context(), user.ID, before, limit)
	} else {
		result, err = service.ListStudentAssignments(r.Context(), user.ID, before, limit)
	}
	if err != nil {
		sendError(w, err)
		return
	}
	sendJSON(w, 200, result)
}

func assignmentAction(w http.ResponseWriter, r *http.Request, store *db.DB, sessions *sessions) {
	user, err := currentUser(r, store, sessions)
	if err != nil {
		sendError(w, err)
		return
	}
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	service := learningdata.NewService(store)
	if r.Method == "GET" {
		if r.Pattern == "GET /api/v1/assignments/{id}" {
			result, err := store.GetAssignment(r.Context(), user.ID, id)
			if err != nil {
				sendError(w, err)
				return
			}
			sendJSON(w, 200, learningdata.ToAssignmentDTO(result))
			return
		}
		result, err := service.ListAssignmentAttempts(r.Context(), user.ID, id)
		if err != nil {
			sendError(w, err)
			return
		}
		sendJSON(w, 200, result)
		return
	}
	action := r.PathValue("action")
	if ((action == "start" || action == "reopen") && r.Method != "POST") || ((action == "status" || action == "deadline") && r.Method != "PUT") {
		sendError(w, db.ErrNotFound)
		return
	}
	role := "teacher"
	if action == "start" {
		role = "student"
	}
	if user.Role != role {
		sendJSON(w, 403, map[string]string{"error": "Недопустимая роль"})
		return
	}
	switch action {
	case "start":
		result, err := service.StartOrResumeAttempt(r.Context(), user.ID, id)
		if err != nil {
			sendError(w, err)
			return
		}
		sendJSON(w, 200, result)
	case "reopen":
		result, err := service.ReopenAttempt(r.Context(), user.ID, id)
		if err != nil {
			sendError(w, err)
			return
		}
		sendJSON(w, 200, result)
	case "status":
		var input learningdata.UpdateStatusRequest
		if !readJSON(w, r, &input) {
			return
		}
		if err := service.SetAssignmentStatus(r.Context(), user.ID, id, input); err != nil {
			sendError(w, err)
			return
		}
		sendJSON(w, 200, map[string]bool{"ok": true})
	case "deadline":
		var input learningdata.UpdateDeadlineRequest
		if !readJSON(w, r, &input) {
			return
		}
		if err := service.UpdateAssignmentDeadline(r.Context(), user.ID, id, input); err != nil {
			sendError(w, err)
			return
		}
		sendJSON(w, 200, map[string]bool{"ok": true})
	default:
		sendError(w, db.ErrNotFound)
	}
}

func attempts(w http.ResponseWriter, r *http.Request, store *db.DB, sessions *sessions) {
	user, err := currentUser(r, store, sessions)
	if err != nil {
		sendError(w, err)
		return
	}
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	service := learningdata.NewService(store)
	action := r.PathValue("action")
	if (r.Method == "GET" && action != "" && action != "answers") || (r.Method == "POST" && action != "submit" && action != "publish") {
		sendError(w, db.ErrNotFound)
		return
	}
	if action == "" {
		result, err := service.GetAttempt(r.Context(), user.ID, id)
		if err != nil {
			sendError(w, err)
			return
		}
		sendJSON(w, 200, result)
		return
	}
	if action == "answers" {
		if user.Role == "student" {
			result, err := service.GetAttemptAnswers(r.Context(), user.ID, id)
			if err != nil {
				sendError(w, err)
				return
			}
			sendJSON(w, 200, result)
		} else if user.Role == "teacher" {
			result, err := service.GetTeacherLiveState(r.Context(), user.ID, id)
			if err != nil {
				sendError(w, err)
				return
			}
			sendJSON(w, 200, result)
		} else {
			sendJSON(w, 403, map[string]string{"error": "Недопустимая роль"})
		}
		return
	}
	if action == "submit" && user.Role == "student" {
		result, err := service.SubmitAttempt(r.Context(), user.ID, id)
		if err != nil {
			sendError(w, err)
			return
		}
		sendJSON(w, 200, result)
		return
	}
	if action == "publish" && user.Role == "teacher" {
		if err := service.PublishAttemptResult(r.Context(), user.ID, id); err != nil {
			sendError(w, err)
			return
		}
		sendJSON(w, 200, map[string]bool{"ok": true})
		return
	}
	sendJSON(w, 403, map[string]string{"error": "Действие недоступно"})
}

func answerWrite(w http.ResponseWriter, r *http.Request, store *db.DB, sessions *sessions) {
	user, err := currentUser(r, store, sessions)
	if err != nil {
		sendError(w, err)
		return
	}
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	itemID, ok := pathID(w, r, "item_id")
	if !ok {
		return
	}
	service := learningdata.NewService(store)
	action := r.PathValue("action")
	if action == "" {
		if user.Role != "student" {
			sendJSON(w, 403, map[string]string{"error": "Требуется ученик"})
			return
		}
		var input learningdata.SaveAnswerRequest
		if !readJSON(w, r, &input) {
			return
		}
		if r.Method == "DELETE" {
			result, err := service.ClearAnswer(r.Context(), user.ID, id, itemID, input.ExpectedRevision)
			if err != nil {
				sendError(w, err)
				return
			}
			sendJSON(w, 200, result)
		} else {
			result, err := service.SaveAnswer(r.Context(), user.ID, id, itemID, input)
			if err != nil {
				sendError(w, err)
				return
			}
			sendJSON(w, 200, result)
		}
		return
	}
	if user.Role != "teacher" {
		sendJSON(w, 403, map[string]string{"error": "Требуется преподаватель"})
		return
	}
	switch action {
	case "review":
		var input learningdata.ReviewAnswerRequest
		if !readJSON(w, r, &input) {
			return
		}
		err = service.ReviewAnswer(r.Context(), user.ID, id, itemID, input)
	case "auto-grade":
		var input learningdata.AutoGradeRequest
		if !readJSON(w, r, &input) {
			return
		}
		err = service.AutoGradeAnswer(r.Context(), user.ID, id, itemID, input)
	default:
		err = db.ErrNotFound
	}
	if err != nil {
		sendError(w, err)
		return
	}
	sendJSON(w, 200, map[string]bool{"ok": true})
}
