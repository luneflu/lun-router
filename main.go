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
	"lun-router/internal/auth"
	"lun-router/internal/caveman"
	"lun-router/internal/chatgpt"
	"lun-router/internal/compactor"
	"lun-router/internal/handler"
	"lun-router/internal/opencode"
	"lun-router/internal/ponytail"
	"lun-router/internal/store"
	"lun-router/internal/usage"
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
	agClient := antigravity.NewClient(agStore, dataDir)
	agClient.StartBackgroundSync(context.Background())

	compactorSvc, err := compactor.NewService(dataDir)
	if err != nil {
		log.Fatalf("Failed to initialize compactor service: %v", err)
	}

	cavemanSvc, err := caveman.NewService(dataDir)
	if err != nil {
		log.Fatalf("Failed to initialize Caveman Service: %v", err)
	}

	ponytailSvc, err := ponytail.NewService(dataDir)
	if err != nil {
		log.Fatalf("Failed to initialize Ponytail Service: %v", err)
	}

	authSvc, err := auth.NewService(dataDir)
	if err != nil {
		log.Fatalf("Failed to initialize Auth Service: %v", err)
	}

	vm := opencode.NewVersionManager(dataDir, 2*time.Hour)
	vm.StartBackgroundSync(context.Background())

	client := opencode.NewClient("", vm)
	mm := opencode.NewModelManager(dataDir, client, 1*time.Hour)
	mm.StartBackgroundSync(context.Background())
	client.SetModelManager(mm)

	distFS, err := fs.Sub(buildFS, "build")
	if err != nil {
		log.Fatalf("Failed to load embedded frontend: %v", err)
	}

	usageSvc, err := usage.NewService(dataDir)
	if err != nil {
		log.Fatalf("Failed to initialize usage service: %v", err)
	}

	cgClient := chatgpt.NewClient(chatgpt.Config{
		DataDir: dataDir,
	})
	defer cgClient.Close()

	// If the provider is already enabled, start the browser now so the first
	// completion request does not fail on a cold start.
	if cgClient.IsEnabled() {
		go cgClient.WarmUp()
	}

	router := handler.NewRouterWithChatGPT(ks, ms, client, agClient, cgClient, compactorSvc, cavemanSvc, ponytailSvc, authSvc, usageSvc, distFS)

	log.Printf("Lun Router Go backend running on :%s (opencode version: %s, data dir: %s)", port, vm.GetVersion(), dataDir)
	if err := http.ListenAndServe(":"+port, router); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
