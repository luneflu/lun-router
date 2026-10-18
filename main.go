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
	"lun-router/internal/kilocode"
	"lun-router/internal/opencode"
	"lun-router/internal/ponytail"
	"lun-router/internal/store"
	"lun-router/internal/usage"
	"lun-router/internal/zai"
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
	// Keep account balances warm so request routing can pick the account with
	// the most remaining credit instead of rotating blindly.
	agClient.StartQuotaSync(context.Background())

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
	pm := opencode.NewProxyManager(dataDir)
	client.SetProxyManager(pm)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		pm.EnsureLoaded(ctx)
	}()

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

	zaiClient := zai.NewClient(zai.Config{
		DataDir: dataDir,
	})
	defer zaiClient.Close()

	// z.ai needs a browser only to mint Aliyun captcha tokens, which are
	// single-use; warming up now means the first completion does not pay the
	// ~7s launch.
	if zaiClient.IsEnabled() {
		go zaiClient.WarmUp()
	}

	kcStore, err := kilocode.NewAccountStore(dataDir)
	if err != nil {
		log.Fatalf("Failed to initialize Kilo Code AccountStore: %v", err)
	}
	kcClient := kilocode.NewClient(kcStore, dataDir)
	kcClient.StartBackgroundSync(context.Background())

	router := handler.NewRouterWithAllProviders(ks, ms, client, agClient, cgClient, zaiClient, kcClient, compactorSvc, cavemanSvc, ponytailSvc, authSvc, usageSvc, distFS)

	log.Printf("Lun Router Go backend running on :%s (opencode version: %s, data dir: %s)", port, vm.GetVersion(), dataDir)
	if err := http.ListenAndServe(":"+port, router); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
