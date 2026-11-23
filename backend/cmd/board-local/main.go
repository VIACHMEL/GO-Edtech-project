// The local runner serves a real PostgreSQL board with two explicitly selected demo users.
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"time"

	"github.com/VIACHMEL/GO-Edtech-project/backend/internal/boardhttp"
	"github.com/VIACHMEL/GO-Edtech-project/backend/internal/db"
)

const sessionCookie = "board_local_session"

func token() string {
	var bytes [32]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		log.Fatal(err)
	}
	return hex.EncodeToString(bytes[:])
}

func main() {
	port := flag.Int("port", 8095, "loopback port for Django and Go")
	djangoPort := flag.Int("django-port", 8094, "existing Django process port")
	attempt := flag.Int64("attempt", 0, "existing fixture attempt ID")
	student := flag.Int64("student", 0, "existing fixture student ID")
	teacher := flag.Int64("teacher", 0, "existing fixture teacher ID")
	flag.Parse()
	if *attempt <= 0 || *student <= 0 || *teacher <= 0 || *student == *teacher {
		log.Fatal("require -attempt, -student and -teacher for a local fixture")
	}
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		log.Fatal("require DATABASE_URL for an isolated local database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	store, err := db.New(ctx, dsn)
	if err != nil {
		log.Fatal(err)
	}
	defer store.Close()
	if _, err := store.GetAttempt(ctx, *student, *attempt); err != nil {
		log.Fatal("student cannot read fixture attempt: ", err)
	}
	if _, err := store.GetAttempt(ctx, *teacher, *attempt); err != nil {
		log.Fatal("teacher cannot read fixture attempt: ", err)
	}

	host := fmt.Sprintf("127.0.0.1:%d", *port)
	origin := "http://" + host
	upstream := &url.URL{Scheme: "http", Host: fmt.Sprintf("127.0.0.1:%d", *djangoPort)}
	proxy := httputil.NewSingleHostReverseProxy(upstream)
	mux := http.NewServeMux()
	studentToken, teacherToken := token(), token()
	sessions := map[string]int64{studentToken: *student, teacherToken: *teacher}
	actor := func(r *http.Request) (int64, bool) {
		cookie, err := r.Cookie(sessionCookie)
		if err != nil {
			return 0, false
		}
		id, ok := sessions[cookie.Value]
		return id, ok
	}
	boardhttp.Register(mux, store, actor, origin)
	mux.HandleFunc("GET /__board_local/{token}", func(w http.ResponseWriter, r *http.Request) {
		key := r.PathValue("token")
		if _, ok := sessions[key]; !ok {
			http.NotFound(w, r)
			return
		}
		http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: key, Path: "/",
			HttpOnly: true, SameSite: http.SameSiteStrictMode})
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Referrer-Policy", "no-referrer")
		http.Redirect(w, r, fmt.Sprintf("/solve/?attempt=%d", *attempt), http.StatusSeeOther)
	})
	mux.Handle("/", proxy)
	server := http.Server{Addr: host, ReadHeaderTimeout: 5 * time.Second, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Host != host {
			http.Error(w, "Invalid host", http.StatusForbidden)
			return
		}
		mux.ServeHTTP(w, r)
	})}
	log.Printf("Ученик: %s/__board_local/%s", origin, studentToken)
	log.Printf("Преподаватель (другой профиль браузера): %s/__board_local/%s", origin, teacherToken)
	log.Fatal(server.ListenAndServe())
}
