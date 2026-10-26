// Package handler обрабатывает HTTP-запросы учебного сервера.
package handler

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/VIACHMEL/GO-Edtech-project/backend/internal/usecase"
)

func New(service *usecase.Service) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "mode": "demo"})
	})
	mux.HandleFunc("GET /api/v1/demo/students", func(w http.ResponseWriter, r *http.Request) {
		students, err := service.DemoStudents(r.Context())
		if err != nil {
			log.Printf("ошибка получения тестовых профилей: %v", err)
			writeError(w, http.StatusInternalServerError, "internal_error", "Не удалось получить данные")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"demo": true, "students": students})
	})
	// Вход и регистрация ещё не реализованы. Не принимать реальные пароли.
	for _, route := range []string{
		"POST /api/v1/auth/register-tutor",
		"POST /api/v1/auth/login",
		"POST /api/v1/auth/logout",
		"GET /api/v1/me",
		"POST /api/v1/students",
		"GET /api/v1/students",
		"POST /api/v1/students/{id}/credentials",
		"POST /api/v1/students/{id}/invitations",
		"POST /api/v1/invitations/preview",
		"POST /api/v1/invitations/accept",
	} {
		mux.HandleFunc(route, notImplemented)
	}
	return mux
}

func notImplemented(w http.ResponseWriter, _ *http.Request) {
	writeError(w, http.StatusNotImplemented, "not_implemented", "Функция ещё не реализована; не отправляйте реальные данные")
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{"error": map[string]string{"code": code, "message": message}})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		log.Printf("ошибка записи ответа: %v", err)
	}
}
