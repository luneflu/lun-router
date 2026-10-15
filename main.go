package main

import (
	"context"
	"embed"
	"io/fs"
	"log"
	"net/http"
	"os"
	"time"

	"lun-router/internal/antigravity"
	"lun-router/internal/caveman"
	"lun-router/internal/handler"
	"lun-router/internal/opencode"
	"lun-router/internal/ponytail"
	"lun-router/internal/rtk"
	"lun-router/internal/store"
)

//go:embed all:build
var buildFS embed.FS

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "20128"
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

	agStore, err := antigravity.NewAccountStore(dataDir)
	if err != nil {
		log.Fatalf("Failed to initialize Antigravity AccountStore: %v", err)
	}
	agClient := antigravity.NewClient(agStore)

	rtkSvc, err := rtk.NewService(dataDir)
	if err != nil {
		log.Fatalf("Failed to initialize RTK Service: %v", err)
	}

	cavemanSvc, err := caveman.NewService(dataDir)
	if err != nil {
		log.Fatalf("Failed to initialize Caveman Service: %v", err)
	}

	ponytailSvc, err := ponytail.NewService(dataDir)
	if err != nil {
		log.Fatalf("Failed to initialize Ponytail Service: %v", err)
	}

	vm := opencode.NewVersionManager(dataDir, 2*time.Hour)
	vm.StartBackgroundSync(context.Background())

	client := opencode.NewClient("", vm)

	distFS, err := fs.Sub(buildFS, "build")
	if err != nil {
		log.Fatalf("Failed to load embedded frontend: %v", err)
	}

	router := handler.NewRouter(ks, ms, client, agClient, rtkSvc, cavemanSvc, ponytailSvc, distFS)

	log.Printf("Lun Router Go backend running on :%s (opencode version: %s, data dir: %s)", port, vm.GetVersion(), dataDir)
	if err := http.ListenAndServe(":"+port, router); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
