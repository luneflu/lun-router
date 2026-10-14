package main

import (
	"log"
	"net/http"
	"os"

	"lun-router/internal/handler"
	"lun-router/internal/opencode"
	"lun-router/internal/store"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	dataDir := os.Getenv("DATA_DIR")
	if dataDir == "" {
		dataDir = "data"
	}

	ks, err := store.NewKeyStore(dataDir)
	if err != nil {
		log.Fatalf("Failed to initialize KeyStore: %v", err)
	}

	client := opencode.NewClient("")
	router := handler.NewRouter(ks, client)

	log.Printf("Lun Router Go backend running on :%s (data dir: %s)", port, dataDir)
	if err := http.ListenAndServe(":"+port, router); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
