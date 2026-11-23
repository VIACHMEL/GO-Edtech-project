package main

import (
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/VIACHMEL/GO-Edtech-project/backend/internal/db"
)

const sessionLifetime = 24 * time.Hour
const passwordIterations = 600000

var errUnauthorized = errors.New("требуется вход")

type session struct {
	userID  int64
	expires time.Time
}

type sessions struct {
	mu     sync.Mutex
	values map[string]session
}

func hashPassword(password string) (string, error) {
	if len(password) < 8 || len(password) > 128 {
		return "", db.ErrValidation
	}
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key, err := pbkdf2.Key(sha256.New, password, salt, passwordIterations, 32)
	if err != nil {
		return "", err
	}
	return "pbkdf2-sha256$" + hex.EncodeToString(salt) + "$" + hex.EncodeToString(key), nil
}

func checkPassword(password, hash string) bool {
	parts := strings.Split(hash, "$")
	if len(parts) != 3 || parts[0] != "pbkdf2-sha256" {
		return false
	}
	salt, err := hex.DecodeString(parts[1])
	if err != nil || len(salt) != 16 {
		return false
	}
	expected, err := hex.DecodeString(parts[2])
	if err != nil || len(expected) != 32 {
		return false
	}
	key, err := pbkdf2.Key(sha256.New, password, salt, passwordIterations, 32)
	return err == nil && subtle.ConstantTimeCompare(key, expected) == 1
}

func login(w http.ResponseWriter, r *http.Request, store *db.DB, sessions *sessions) {
	var input struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if !readJSON(w, r, &input) {
		return
	}
	if len(input.Password) < 8 || len(input.Password) > 128 {
		sendJSON(w, 401, map[string]string{"error": "Неверный email или пароль"})
		return
	}
	credentials, err := store.GetUserByEmail(r.Context(), input.Email)
	if err != nil && !errors.Is(err, db.ErrNotFound) && !errors.Is(err, db.ErrValidation) {
		sendError(w, err)
		return
	}
	// Unknown users still require the same password computation.
	hash := "pbkdf2-sha256$00000000000000000000000000000000$0000000000000000000000000000000000000000000000000000000000000000"
	if err == nil {
		hash = credentials.PasswordHash
	}
	valid := checkPassword(input.Password, hash)
	if err != nil || !valid || credentials.Status != "active" {
		sendJSON(w, 401, map[string]string{"error": "Неверный email или пароль"})
		return
	}
	if credentials.Role != "teacher" && credentials.Role != "student" {
		sendJSON(w, 403, map[string]string{"error": "Доступ запрещён"})
		return
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		sendError(w, err)
		return
	}
	token := hex.EncodeToString(raw)
	sessions.mu.Lock()
	for key, value := range sessions.values {
		if !time.Now().Before(value.expires) {
			delete(sessions.values, key)
		}
	}
	sessions.values[token] = session{credentials.ID, time.Now().Add(sessionLifetime)}
	sessions.mu.Unlock()
	sendJSON(w, 200, map[string]string{"token": token})
}

func currentUser(r *http.Request, store *db.DB, sessions *sessions) (*db.User, error) {
	header := r.Header.Get("Authorization")
	if !strings.HasPrefix(header, "Bearer ") {
		return nil, errUnauthorized
	}
	token := strings.TrimPrefix(header, "Bearer ")
	sessions.mu.Lock()
	value, found := sessions.values[token]
	if found && !time.Now().Before(value.expires) {
		delete(sessions.values, token)
		found = false
	}
	sessions.mu.Unlock()
	if !found {
		return nil, errUnauthorized
	}
	user, err := store.GetUser(r.Context(), value.userID)
	if errors.Is(err, db.ErrNotFound) || (err == nil && user.Status != "active") {
		sessions.mu.Lock()
		delete(sessions.values, token)
		sessions.mu.Unlock()
		return nil, errUnauthorized
	}
	return user, err
}

// Only the board routes accept the HttpOnly session cookie used by Django.
// Browser WebSockets cannot attach an Authorization header.
func boardActor(store *db.DB, sessions *sessions) func(*http.Request) (int64, bool) {
	return func(r *http.Request) (int64, bool) {
		if r.Header.Get("Authorization") == "" {
			cookie, err := r.Cookie("go_session")
			if err != nil {
				return 0, false
			}
			r = r.Clone(r.Context())
			r.Header.Set("Authorization", "Bearer "+cookie.Value)
		}
		user, err := currentUser(r, store, sessions)
		if err != nil {
			return 0, false
		}
		return user.ID, true
	}
}

func logout(w http.ResponseWriter, r *http.Request, sessions *sessions) {
	token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	sessions.mu.Lock()
	delete(sessions.values, token)
	sessions.mu.Unlock()
	sendJSON(w, 200, map[string]bool{"ok": true})
}

func sendJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func sendError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, errUnauthorized):
		sendJSON(w, 401, map[string]string{"error": "Требуется вход"})
	case errors.Is(err, db.ErrNotFound):
		sendJSON(w, 404, map[string]string{"error": "Запись не найдена"})
	case errors.Is(err, db.ErrConflict), errors.Is(err, db.ErrWorksheetConflict), errors.Is(err, db.ErrWorksheetLocked), errors.Is(err, db.ErrWorkClosed), errors.Is(err, db.ErrAttemptLocked), errors.Is(err, db.ErrRevisionConflict), errors.Is(err, db.ErrAttemptLimit), errors.Is(err, db.ErrIncompleteGrading):
		sendJSON(w, 409, map[string]string{"error": "Запись уже существует"})
	case errors.Is(err, db.ErrValidation):
		sendJSON(w, 400, map[string]string{"error": "Некорректные параметры"})
	default:
		log.Print(err)
		sendJSON(w, 503, map[string]string{"error": "Ошибка сервера"})
	}
}

func readJSON(w http.ResponseWriter, r *http.Request, target any) bool {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2*1024*1024))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		sendError(w, db.ErrValidation)
		return false
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		sendError(w, db.ErrValidation)
		return false
	}
	return true
}
