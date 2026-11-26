package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"github.com/VIACHMEL/GO-Edtech-project/backend/internal/boardhttp"
	"github.com/VIACHMEL/GO-Edtech-project/backend/internal/db"
	"github.com/VIACHMEL/GO-Edtech-project/backend/internal/teacherdata"
)

func me(w http.ResponseWriter, r *http.Request, store *db.DB, sessions *sessions) {
	user, err := currentUser(r, store, sessions)
	if err != nil {
		sendError(w, err)
		return
	}
	sendJSON(w, 200, teacherdata.ToProfileDTO(*user))
}

func teacherList(w http.ResponseWriter, r *http.Request, store *db.DB, sessions *sessions) {
	user, err := currentUser(r, store, sessions)
	if err != nil {
		sendError(w, err)
		return
	}
	if user.Role != "teacher" {
		sendJSON(w, 403, map[string]string{"error": "Требуется роль преподавателя"})
		return
	}
	service := teacherdata.NewService(store)
	query := r.URL.Query()
	if r.URL.Path == "/api/v1/teacher/students" {
		data, err := service.GetTeacherStudents(r.Context(), user.ID, query.Get("q"), query.Get("status"))
		if err != nil {
			sendError(w, err)
			return
		}
		sendJSON(w, 200, data)
	} else {
		data, err := service.GetTeacherGroups(r.Context(), user.ID, query.Get("q"), query.Get("status"))
		if err != nil {
			sendError(w, err)
			return
		}
		sendJSON(w, 200, data)
	}
}

func groupWrite(w http.ResponseWriter, r *http.Request, store *db.DB, sessions *sessions) {
	user, err := currentUser(r, store, sessions)
	if err != nil {
		sendError(w, err)
		return
	}
	if user.Role != "teacher" {
		sendJSON(w, 403, map[string]string{"error": "Требуется роль преподавателя"})
		return
	}
	var group *db.Group
	if rawID := r.PathValue("id"); rawID != "" {
		id, err := strconv.ParseInt(rawID, 10, 64)
		if err != nil || id <= 0 {
			sendError(w, db.ErrValidation)
			return
		}
		group, err = store.GetGroup(r.Context(), id)
		if err != nil {
			sendError(w, err)
			return
		}
		if group.TeacherID != user.ID || group.Status != "active" {
			sendError(w, db.ErrNotFound)
			return
		}
	}
	if r.Pattern == "POST /api/v1/teacher/groups/{id}/archive" {
		if err := store.ArchiveGroup(r.Context(), group.ID, user.ID); err != nil {
			sendError(w, err)
			return
		}
		sendJSON(w, 200, map[string]bool{"ok": true})
		return
	}
	if r.Method == "DELETE" {
		studentID, err := strconv.ParseInt(r.PathValue("student_id"), 10, 64)
		if err != nil || studentID <= 0 {
			sendError(w, db.ErrValidation)
			return
		}
		if err := store.RemoveGroupMember(r.Context(), group.ID, studentID); err != nil {
			sendError(w, err)
			return
		}
		sendJSON(w, 200, map[string]bool{"ok": true})
		return
	}
	var input struct {
		Name      string  `json:"name"`
		Subject   *string `json:"subject"`
		StudentID int64   `json:"student_id"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		sendError(w, db.ErrValidation)
		return
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		sendError(w, db.ErrValidation)
		return
	}
	if r.Method == "POST" && group != nil {
		if input.StudentID <= 0 {
			sendError(w, db.ErrValidation)
			return
		}
		// AddGroupMember checks the active teacher-student link in SQL.
		if err := store.AddGroupMember(r.Context(), group.ID, input.StudentID); err != nil {
			sendError(w, err)
			return
		}
		sendJSON(w, 200, map[string]bool{"ok": true})
		return
	}
	status := 200
	if group == nil {
		group, err = store.CreateGroup(r.Context(), db.NewGroup{TeacherID: user.ID, Name: input.Name, Subject: input.Subject})
		status = 201
	} else {
		group, err = store.UpdateGroup(r.Context(), group.ID, user.ID, input.Name, input.Subject)
	}
	if err != nil {
		sendError(w, err)
		return
	}
	sendJSON(w, status, map[string]any{"id": group.ID, "name": group.Name, "subject": group.Subject})
}

func teacherTasks(w http.ResponseWriter, r *http.Request, store *db.DB, sessions *sessions) {
	user, err := currentUser(r, store, sessions)
	if err != nil {
		sendError(w, err)
		return
	}
	if user.Role != "teacher" {
		sendJSON(w, 403, map[string]string{"error": "Требуется роль преподавателя"})
		return
	}
	query := r.URL.Query()
	numbers := map[string]int{"limit": 20}
	for _, name := range []string{"year", "task_number", "limit"} {
		if query.Get(name) == "" {
			continue
		}
		value, err := strconv.Atoi(query.Get(name))
		if err != nil || value < 0 || (name == "limit" && (value < 1 || value > 100)) {
			sendError(w, db.ErrValidation)
			return
		}
		numbers[name] = value
	}
	tasks, err := store.ListTasks(r.Context(), db.TaskFilter{
		Subject:    query.Get("subject"),
		Year:       numbers["year"],
		TaskNumber: numbers["task_number"],
		TaskType:   query.Get("task_type"),
		Limit:      numbers["limit"],
	})
	if err != nil {
		sendError(w, err)
		return
	}
	versions := make([]int64, 0, len(tasks))
	for _, task := range tasks {
		versions = append(versions, task.CurrentVersionID)
	}
	conditions, err := store.TaskConditions(r.Context(), versions)
	if err != nil {
		sendError(w, err)
		return
	}
	result := make([]map[string]any, 0, len(tasks))
	for _, task := range tasks {
		// Only public task fields: no answer, solution or parser metadata.
		presentation := boardhttp.PresentCondition(conditions[task.CurrentVersionID],
			fmt.Sprintf("/api/v1/teacher/task-versions/%d/materials/", task.CurrentVersionID))
		result = append(result, map[string]any{
			"id": task.ID, "current_version_id": task.CurrentVersionID,
			"subject": task.Subject, "task_number": task.TaskNumber, "year": task.Year,
			"task_type": task.TaskType, "content_text": task.ContentText,
			"source_site": task.SourceSite, "source_url": task.SourceURL,
			"condition_html": presentation.HTML, "materials": presentation.Materials,
		})
	}
	sendJSON(w, 200, result)
}
