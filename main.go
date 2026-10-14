package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"time"

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

	ms, err := store.NewModelStore(dataDir)
	if err != nil {
		log.Fatalf("Failed to initialize ModelStore: %v", err)
	}

	vm := opencode.NewVersionManager(dataDir, 2*time.Hour)
	vm.StartBackgroundSync(context.Background())

	client := opencode.NewClient("", vm)
	router := handler.NewRouter(ks, ms, client)

	log.Printf("Lun Router Go backend running on :%s (opencode version: %s, data dir: %s)", port, vm.GetVersion(), dataDir)
	if err := http.ListenAndServe(":"+port, router); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
