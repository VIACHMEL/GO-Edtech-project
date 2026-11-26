package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"

	"github.com/VIACHMEL/GO-Edtech-project/backend/internal/boardhttp"
	"github.com/VIACHMEL/GO-Edtech-project/backend/internal/db"
)

func main() {
	hash := flag.Bool("hash-password", false, "read password from stdin and print its hash")
	flag.Parse()
	if *hash {
		raw, err := io.ReadAll(io.LimitReader(os.Stdin, 131))
		if err != nil {
			log.Fatal(err)
		}
		password := strings.TrimSuffix(string(raw), "\n")
		password = strings.TrimSuffix(password, "\r")
		value, err := hashPassword(password)
		if err != nil {
			log.Fatal("Пароль: 8–128 байт; ", err)
		}
		fmt.Println(value)
		return
	}

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		log.Fatal("Не задан DATABASE_URL")
	}

	store, err := db.New(context.Background(), dsn)
	if err != nil {
		log.Fatal("Не удалось подключиться к БД: ", err)
	}
	addr := os.Getenv("BACKEND_ADDR")
	if addr == "" {
		addr = "127.0.0.1:8080"
	}

	mux := http.NewServeMux()
	sessionStore := &sessions{values: make(map[string]session)}
	uploadRoot := os.Getenv("UPLOAD_DIR")
	if uploadRoot == "" {
		uploadRoot = "../data/uploads"
	}
	mux.HandleFunc("GET /health", health)
	mux.HandleFunc("POST /api/v1/register", func(w http.ResponseWriter, r *http.Request) {
		register(w, r, store)
	})
	mux.HandleFunc("POST /api/v1/login", func(w http.ResponseWriter, r *http.Request) {
		login(w, r, store, sessionStore)
	})
	mux.HandleFunc("POST /api/v1/logout", func(w http.ResponseWriter, r *http.Request) {
		logout(w, r, sessionStore)
	})
	mux.HandleFunc("GET /api/v1/me", func(w http.ResponseWriter, r *http.Request) {
		me(w, r, store, sessionStore)
	})
	mux.HandleFunc("PUT /api/v1/me", func(w http.ResponseWriter, r *http.Request) {
		updateProfile(w, r, store, sessionStore)
	})
	for _, pattern := range []string{"GET /api/v1/teacher/students", "GET /api/v1/teacher/groups"} {
		mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
			teacherList(w, r, store, sessionStore)
		})
	}
	mux.HandleFunc("GET /api/v1/teacher/tasks", func(w http.ResponseWriter, r *http.Request) {
		teacherTasks(w, r, store, sessionStore)
	})
	for _, pattern := range []string{
		"POST /api/v1/teacher/groups",
		"PUT /api/v1/teacher/groups/{id}",
		"POST /api/v1/teacher/groups/{id}/members",
		"DELETE /api/v1/teacher/groups/{id}/members/{student_id}",
		"POST /api/v1/teacher/groups/{id}/archive",
	} {
		mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
			groupWrite(w, r, store, sessionStore)
		})
	}
	for _, pattern := range []string{
		"POST /api/v1/teacher/invitations",
		"DELETE /api/v1/teacher/invitations/{id}",
		"POST /api/v1/invitations/accept",
	} {
		mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
			invitations(w, r, store, sessionStore)
		})
	}
	for _, pattern := range []string{
		"GET /api/v1/teacher/worksheets",
		"POST /api/v1/teacher/worksheets",
		"GET /api/v1/teacher/worksheets/{id}",
		"PUT /api/v1/teacher/worksheets/{id}",
	} {
		mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
			worksheets(w, r, store, sessionStore)
		})
	}
	mux.HandleFunc("POST /api/v1/teacher/worksheets/{id}/{action}", func(w http.ResponseWriter, r *http.Request) {
		worksheetAction(w, r, store, sessionStore)
	})
	for _, pattern := range []string{
		"POST /api/v1/teacher/worksheets/{id}/items",
		"PUT /api/v1/teacher/worksheets/{id}/items/order",
		"PUT /api/v1/teacher/worksheets/{id}/items/{item_id}",
		"DELETE /api/v1/teacher/worksheets/{id}/items/{item_id}",
	} {
		mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
			worksheetItems(w, r, store, sessionStore)
		})
	}
	for _, pattern := range []string{
		"GET /api/v1/teacher/assignments",
		"GET /api/v1/student/assignments",
		"POST /api/v1/teacher/assignments",
		"POST /api/v1/teacher/assignments/group",
	} {
		mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
			assignments(w, r, store, sessionStore)
		})
	}
	for _, pattern := range []string{
		"GET /api/v1/assignments/{id}",
		"GET /api/v1/assignments/{id}/attempts",
		"POST /api/v1/assignments/{id}/{action}",
		"PUT /api/v1/assignments/{id}/{action}",
	} {
		mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
			assignmentAction(w, r, store, sessionStore)
		})
	}
	for _, pattern := range []string{
		"GET /api/v1/attempts/{id}",
		"GET /api/v1/attempts/{id}/{action}",
		"POST /api/v1/attempts/{id}/{action}",
	} {
		mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
			attempts(w, r, store, sessionStore)
		})
	}
	for _, pattern := range []string{
		"PUT /api/v1/attempts/{id}/answers/{item_id}",
		"DELETE /api/v1/attempts/{id}/answers/{item_id}",
		"POST /api/v1/attempts/{id}/answers/{item_id}/{action}",
	} {
		mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
			answerWrite(w, r, store, sessionStore)
		})
	}
	for _, pattern := range []string{
		"POST /api/v1/attempts/{id}/answers/{item_id}/photos",
		"GET /api/v1/attempts/{id}/answers/{item_id}/photos/{filename}",
	} {
		mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
			photos(w, r, store, sessionStore, uploadRoot)
		})
	}

	for _, pattern := range []string{"POST /api/v1/teacher/worksheets/draft", "GET /api/v1/teacher/worksheets/{id}/draft", "PUT /api/v1/teacher/worksheets/{id}/draft"} {
		mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) { worksheetDraft(w, r, store, sessionStore) })
	}

	dataRoot := os.Getenv("DATA_ROOT")
	if dataRoot == "" {
		dataRoot = "../data/raw"
	}
	boardhttp.Register(mux, store, boardActor(store, sessionStore), os.Getenv("BOARD_ORIGIN"), dataRoot)
	if djangoURL := os.Getenv("DJANGO_PROXY_URL"); djangoURL != "" {
		target, err := url.Parse(djangoURL)
		if err != nil || target.Scheme != "http" || target.Hostname() != "127.0.0.1" ||
			target.User != nil || target.Path != "" || target.RawQuery != "" || target.Fragment != "" {
			log.Fatal("DJANGO_PROXY_URL must be a local HTTP origin")
		}
		mux.Handle("/", httputil.NewSingleHostReverseProxy(target))
	}

	log.Printf("HTTP-сервер: %s", addr)
	err = http.ListenAndServe(addr, mux)
	store.Close()
	if err != nil {
		log.Fatal(err)
	}
}

func health(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"status":"ok"}`))
}
