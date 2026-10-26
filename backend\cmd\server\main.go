package main

import (
	"log"
	"net/http"
	"os"
	"time"

	"github.com/VIACHMEL/GO-Edtech-project/backend/internal/handler"
	"github.com/VIACHMEL/GO-Edtech-project/backend/internal/storage"
	"github.com/VIACHMEL/GO-Edtech-project/backend/internal/usecase"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	server := &http.Server{
		Addr:              ":" + port,
		Handler:           handler.New(usecase.New(storage.NewDemo())),
		ReadHeaderTimeout: 5 * time.Second,
	}
	log.Printf("Учебный сервер слушает порт %s", port)
	log.Fatal(server.ListenAndServe())
}
