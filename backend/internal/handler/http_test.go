package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/VIACHMEL/GO-Edtech-project/backend/internal/handler"
	"github.com/VIACHMEL/GO-Edtech-project/backend/internal/storage"
	"github.com/VIACHMEL/GO-Edtech-project/backend/internal/usecase"
)

func TestDemoAndUnimplementedAuth(t *testing.T) {
	app := handler.New(usecase.New(storage.NewDemo()))
	for _, tc := range []struct {
		method, path string
		status       int
	}{
		{"GET", "/health", http.StatusOK},
		{"GET", "/api/v1/demo/students", http.StatusOK},
		{"POST", "/api/v1/auth/login", http.StatusNotImplemented},
		{"POST", "/api/v1/students", http.StatusNotImplemented},
		{"GET", "/api/v1/students", http.StatusNotImplemented},
		{"POST", "/api/v1/invitations/accept", http.StatusNotImplemented},
		{"PUT", "/health", http.StatusMethodNotAllowed},
	} {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(tc.method, tc.path, nil)
			app.ServeHTTP(rec, req)
			if rec.Code != tc.status {
				t.Fatalf("got status %d, want %d", rec.Code, tc.status)
			}
			if tc.status != http.StatusMethodNotAllowed {
				if !strings.HasPrefix(rec.Header().Get("Content-Type"), "application/json") {
					t.Fatalf("unexpected content type: %s", rec.Header().Get("Content-Type"))
				}
				if !json.Valid(rec.Body.Bytes()) {
					t.Fatalf("invalid JSON: %s", rec.Body.String())
				}
			}
		})
	}
}
