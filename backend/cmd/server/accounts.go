package main

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"time"

	"github.com/VIACHMEL/GO-Edtech-project/backend/internal/db"
	"github.com/VIACHMEL/GO-Edtech-project/backend/internal/teacherdata"
)

func register(w http.ResponseWriter, r *http.Request, store *db.DB) {
	var input struct {
		Email    string `json:"email"`
		Password string `json:"password"`
		Name     string `json:"name"`
		Role     string `json:"role"`
		Grade    *int16 `json:"grade"`
	}
	if !readJSON(w, r, &input) {
		return
	}
	hash, err := hashPassword(input.Password)
	if err != nil {
		sendError(w, err)
		return
	}
	user, err := store.CreateUser(r.Context(), db.NewUser{Email: input.Email, PasswordHash: hash, Name: input.Name, Role: input.Role, Grade: input.Grade})
	if err != nil {
		sendError(w, err)
		return
	}
	sendJSON(w, 201, teacherdata.ToProfileDTO(*user))
}

func updateProfile(w http.ResponseWriter, r *http.Request, store *db.DB, sessions *sessions) {
	user, err := currentUser(r, store, sessions)
	if err != nil {
		sendError(w, err)
		return
	}
	var input struct {
		Name  string `json:"name"`
		Grade *int16 `json:"grade"`
	}
	if !readJSON(w, r, &input) {
		return
	}
	updated, err := store.UpdateUserProfile(r.Context(), user.ID, input.Name, input.Grade)
	if err != nil {
		sendError(w, err)
		return
	}
	sendJSON(w, 200, teacherdata.ToProfileDTO(*updated))
}

func invitations(w http.ResponseWriter, r *http.Request, store *db.DB, sessions *sessions) {
	user, err := currentUser(r, store, sessions)
	if err != nil {
		sendError(w, err)
		return
	}
	if r.URL.Path == "/api/v1/invitations/accept" {
		if user.Role != "student" {
			sendJSON(w, 403, map[string]string{"error": "Требуется ученик"})
			return
		}
		var input struct {
			Token string `json:"token"`
		}
		if !readJSON(w, r, &input) {
			return
		}
		hash := sha256.Sum256([]byte(input.Token))
		if err := store.AcceptInvitation(r.Context(), hex.EncodeToString(hash[:]), user.ID); err != nil {
			sendError(w, err)
			return
		}
		sendJSON(w, 200, map[string]bool{"ok": true})
		return
	}
	if user.Role != "teacher" {
		sendJSON(w, 403, map[string]string{"error": "Требуется преподаватель"})
		return
	}
	if r.Method == "DELETE" {
		id, ok := pathID(w, r, "id")
		if !ok {
			return
		}
		if err := store.RevokeInvitation(r.Context(), id, user.ID); err != nil {
			sendError(w, err)
			return
		}
		sendJSON(w, 200, map[string]bool{"ok": true})
		return
	}
	var input struct {
		GroupID   *int64    `json:"group_id"`
		Email     *string   `json:"email"`
		ExpiresAt time.Time `json:"expires_at"`
	}
	if !readJSON(w, r, &input) {
		return
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		sendError(w, err)
		return
	}
	token := hex.EncodeToString(raw)
	hash := sha256.Sum256([]byte(token))
	invitation, err := store.CreateInvitation(r.Context(), db.NewInvitation{
		TeacherID:    user.ID,
		GroupID:      input.GroupID,
		InviteeEmail: input.Email,
		TokenHash:    hex.EncodeToString(hash[:]),
		ExpiresAt:    input.ExpiresAt,
	})
	if err != nil {
		sendError(w, err)
		return
	}
	sendJSON(w, 201, map[string]any{"id": invitation.ID, "token": token, "expires_at": invitation.ExpiresAt})
}
