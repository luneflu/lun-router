package handler

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"lun-router/internal/antigravity"
	"lun-router/internal/caveman"
	"lun-router/internal/chatgpt"
	"lun-router/internal/compactor"
	"lun-router/internal/customprovider"
	"lun-router/internal/kilocode"
	"lun-router/internal/model"
	"lun-router/internal/opencode"
	"lun-router/internal/ponytail"
	"lun-router/internal/store"
	"lun-router/internal/usage"
	"lun-router/internal/zai"
)

type CompletionHandler struct {
	store            *store.KeyStore
	modelStore       *store.ModelStore
	client           *opencode.Client
	agClient         *antigravity.Client
	cgClient         *chatgpt.Client
	zaiClient        *zai.Client
	kcClient         *kilocode.Client
	customStore      *customprovider.Store
	customClient     *customprovider.Client
	compactorService *compactor.Service
	cavemanService   *caveman.Service
	ponytailService  *ponytail.Service
	usage            *usage.Service
}

// SetUsage attaches the usage monitor. Optional: nil records nothing.
func (h *CompletionHandler) SetUsage(s *usage.Service) {
	h.usage = s
}

// SetChatGPT attaches the ChatGPT provider client.
func (h *CompletionHandler) SetChatGPT(c *chatgpt.Client) {
	h.cgClient = c
}

// SetZai attaches the z.ai provider client.
func (h *CompletionHandler) SetZai(c *zai.Client) {
	h.zaiClient = c
}

// SetKiloCode attaches the Kilo Code provider client.
func (h *CompletionHandler) SetKiloCode(c *kilocode.Client) {
	h.kcClient = c
}

// SetCustomProviders attaches the custom provider store and client.
func (h *CompletionHandler) SetCustomProviders(s *customprovider.Store, c *customprovider.Client) {
	h.customStore = s
	h.customClient = c
}

func NewCompletionHandler(s *store.KeyStore, ms *store.ModelStore, client *opencode.Client, agClient *antigravity.Client, compactorSvc *compactor.Service, cavemanSvc *caveman.Service, ponytailSvc *ponytail.Service) *CompletionHandler {
	if client == nil {
		client = opencode.NewClient("", nil)
	}
	return &CompletionHandler{
		store:            s,
		modelStore:       ms,
		client:           client,
		agClient:         agClient,
		compactorService: compactorSvc,
		cavemanService:   cavemanSvc,
		ponytailService:  ponytailSvc,
	}
}

type ModelItem = model.ModelItem
type ModelCapabilities = model.ModelCapabilities

// Model namespaces. Every advertised model ID is prefixed with its provider.
const (
	opencodePrefix    = "opencode/"
	antigravityPrefix = "antigravity/"
	chatgptPrefix     = "chatgpt/"
	zaiPrefix         = "zai/"
	kilocodePrefix    = "kilocode/"
)

// GET /v1/models (OpenAI-compatible catalog: returns only enabled models)
func (h *CompletionHandler) Models(w http.ResponseWriter, r *http.Request) {
	rawModels := h.fetchAllModels(r)

	var filtered []ModelItem
	for _, m := range rawModels {
		if h.modelStore == nil || !h.modelStore.IsDisabled("opencode", m.ID) {
			m.ID = opencodePrefix + m.ID
			filtered = append(filtered, m)
		}
	}

	// Antigravity models
	if h.agClient != nil {
		for _, m := range h.agClient.GetCatalogModels() {
			if h.modelStore == nil || !h.modelStore.IsDisabled("antigravity", m) {
				item := ModelItem{
					ID:      antigravityPrefix + m,
					Object:  "model",
					OwnedBy: "antigravity",
					Enabled: true,
				}
				model.EnrichModelItem(&item, "antigravity")
				filtered = append(filtered, item)
			}
		}
	}

	// ChatGPT models (only if enabled)
	if h.cgClient != nil && h.cgClient.IsEnabled() {
		for _, m := range h.cgClient.GetCatalogModels() {
			if h.modelStore == nil || !h.modelStore.IsDisabled("chatgpt", m) {
				item := ModelItem{
					ID:      chatgptPrefix + m,
					Object:  "model",
					OwnedBy: "chatgpt",
					Enabled: true,
				}
				model.EnrichModelItem(&item, "chatgpt")
				filtered = append(filtered, item)
			}
		}
	}

	// z.ai models (only if enabled)
	if h.zaiClient != nil && h.zaiClient.IsEnabled() {
		for _, m := range h.zaiClient.GetCatalogModels() {
			if h.modelStore == nil || !h.modelStore.IsDisabled("zai", m) {
				item := ModelItem{
					ID:      zaiPrefix + m,
					Object:  "model",
					OwnedBy: "zai",
					Enabled: true,
				}
				model.EnrichModelItem(&item, "zai")
				filtered = append(filtered, item)
			}
		}
	}

	// Kilo Code models
	if h.kcClient != nil {
		for _, m := range h.kcClient.GetCatalogModels() {
			if h.modelStore == nil || !h.modelStore.IsDisabled("kilocode", m) {
				item := ModelItem{
					ID:      kilocodePrefix + m,
					Object:  "model",
					OwnedBy: "kilocode",
					Enabled: true,
				}
				model.EnrichModelItem(&item, "kilocode")
				filtered = append(filtered, item)
			}
		}
	}

	// Custom models across providers
	if h.modelStore != nil {
		for _, cm := range h.modelStore.ListCustomModels("opencode") {
			if !h.modelStore.IsDisabled("opencode", cm.ID) {
				item := ModelItem{
					ID:                  opencodePrefix + cm.ID,
					Object:              "model",
					OwnedBy:             "opencode",
					Enabled:             true,
					IsCustom:            true,
					Capabilities:        cm.Capabilities,
					ThinkingLevels:      cm.ThinkingLevels,
					ContextLength:       cm.ContextLength,
					MaxCompletionTokens: cm.MaxCompletionTokens,
				}
				if item.Capabilities == nil {
					model.EnrichModelItem(&item, "opencode")
				}
				filtered = append(filtered, item)
			}
		}
		if h.agClient != nil {
			for _, cm := range h.modelStore.ListCustomModels("antigravity") {
				if !h.modelStore.IsDisabled("antigravity", cm.ID) {
					item := ModelItem{
						ID:                  antigravityPrefix + cm.ID,
						Object:              "model",
						OwnedBy:             "antigravity",
						Enabled:             true,
						IsCustom:            true,
						Capabilities:        cm.Capabilities,
						ThinkingLevels:      cm.ThinkingLevels,
						ContextLength:       cm.ContextLength,
						MaxCompletionTokens: cm.MaxCompletionTokens,
					}
					if item.Capabilities == nil {
						model.EnrichModelItem(&item, "antigravity")
					}
					filtered = append(filtered, item)
				}
			}
		}
		if h.kcClient != nil {
			for _, cm := range h.modelStore.ListCustomModels("kilocode") {
				if !h.modelStore.IsDisabled("kilocode", cm.ID) {
					item := ModelItem{
						ID:                  kilocodePrefix + cm.ID,
						Object:              "model",
						OwnedBy:             "kilocode",
						Enabled:             true,
						IsCustom:            true,
						Capabilities:        cm.Capabilities,
						ThinkingLevels:      cm.ThinkingLevels,
						ContextLength:       cm.ContextLength,
						MaxCompletionTokens: cm.MaxCompletionTokens,
					}
					if item.Capabilities == nil {
						model.EnrichModelItem(&item, "kilocode")
					}
					filtered = append(filtered, item)
				}
			}
		}
	}

	// Custom providers models
	if h.customStore != nil {
		for _, cp := range h.customStore.List() {
			if !cp.IsEnabled {
				continue
			}
			prefix := cp.Prefix + "/"
			seenModels := make(map[string]bool)
			for _, m := range cp.Models {
				seenModels[m] = true
				if h.modelStore == nil || !h.modelStore.IsDisabled(cp.Prefix, m) {
					item := ModelItem{
						ID:      prefix + m,
						Object:  "model",
						OwnedBy: cp.Prefix,
						Enabled: true,
					}
					model.EnrichModelItem(&item, cp.Prefix)
					filtered = append(filtered, item)
				}
			}
			if h.modelStore != nil {
				for _, cm := range h.modelStore.ListCustomModels(cp.Prefix) {
					if !seenModels[cm.ID] {
						seenModels[cm.ID] = true
						if !h.modelStore.IsDisabled(cp.Prefix, cm.ID) {
							item := ModelItem{
								ID:                  prefix + cm.ID,
								Object:              "model",
								OwnedBy:             cp.Prefix,
								Enabled:             true,
								IsCustom:            true,
								Capabilities:        cm.Capabilities,
								ThinkingLevels:      cm.ThinkingLevels,
								ContextLength:       cm.ContextLength,
								MaxCompletionTokens: cm.MaxCompletionTokens,
							}
							if item.Capabilities == nil {
								model.EnrichModelItem(&item, cp.Prefix)
							}
							filtered = append(filtered, item)
						}
					}
				}
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"object": "list",
		"data":   filtered,
	})
}

// GET /api/providers/opencode/models (Provider view: returns all models with enabled status)
func (h *CompletionHandler) ProviderModels(w http.ResponseWriter, r *http.Request) {
	rawModels := h.fetchAllModels(r)

	for i := range rawModels {
		rawModels[i].Enabled = h.modelStore == nil || !h.modelStore.IsDisabled("opencode", rawModels[i].ID)
		rawModels[i].ID = opencodePrefix + rawModels[i].ID
	}

	if h.modelStore != nil {
		for _, cm := range h.modelStore.ListCustomModels("opencode") {
			item := ModelItem{
				ID:                  opencodePrefix + cm.ID,
				Object:              "model",
				OwnedBy:             "opencode",
				Enabled:             !h.modelStore.IsDisabled("opencode", cm.ID),
				IsCustom:            true,
				Capabilities:        cm.Capabilities,
				ThinkingLevels:      cm.ThinkingLevels,
				ContextLength:       cm.ContextLength,
				MaxCompletionTokens: cm.MaxCompletionTokens,
			}
			if item.Capabilities == nil {
				model.EnrichModelItem(&item, "opencode")
			}
			rawModels = append(rawModels, item)
		}
	}

	version := opencode.DefaultVersion
	if h.client != nil && h.client.VersionManager() != nil {
		version = h.client.VersionManager().GetVersion()
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"provider": "opencode",
		"version":  version,
		"models":   rawModels,
	})
}

// GET /api/providers/zai/models (Provider view: returns all z.ai models with enabled status)
func (h *CompletionHandler) ZaiProviderModels(w http.ResponseWriter, r *http.Request) {
	var rawModels []ModelItem
	if h.zaiClient != nil {
		for _, m := range h.zaiClient.GetCatalogModels() {
			item := ModelItem{
				ID:      zaiPrefix + m,
				Object:  "model",
				OwnedBy: "zai",
				Enabled: h.modelStore == nil || !h.modelStore.IsDisabled("zai", m),
			}
			model.EnrichModelItem(&item, "zai")
			rawModels = append(rawModels, item)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"provider": "zai",
		"enabled":  h.zaiClient != nil && h.zaiClient.IsEnabled(),
		"models":   rawModels,
	})
}

// GET /api/providers/zai/status (provider state, browser usage and install progress)
func (h *CompletionHandler) ZaiStatus(w http.ResponseWriter, r *http.Request) {
	if h.zaiClient == nil {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"provider": "zai",
			"enabled":  false,
			"error":    "z.ai client not configured",
		})
		return
	}

	inst := h.zaiClient.Installer()
	browserPath := ""
	if exe := inst.DetectedExecutable(); exe != "" {
		browserPath = exe
	}

	mem := h.zaiClient.GetMemoryUsage()

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"provider":          "zai",
		"enabled":           h.zaiClient.IsEnabled(),
		"installed":         inst.IsInstalled(),
		"browser_path":      browserPath,
		"install_state":     inst.GetStatus().State,
		"install_progress":  inst.GetStatus().Progress,
		"install_error":     inst.GetStatus().Error,
		"browser_running":   h.zaiClient.BrowserRunning(),
		"browser_pid":       mem.PID,
		"browser_procs":     mem.ProcessNum,
		"browser_rss_mb":    mem.TotalRSSMB,
		"passthrough_tools": h.zaiClient.PassthroughTools(),
		"guest_mode":        true,
	})
}

// PUT /api/providers/zai/status (enable or disable the provider)
func (h *CompletionHandler) UpdateZaiStatus(w http.ResponseWriter, r *http.Request) {
	if h.zaiClient == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "z.ai client not configured"})
		return
	}
	var payload struct {
		Enabled *bool `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Invalid JSON body"})
		return
	}
	if payload.Enabled != nil {
		if err := h.zaiClient.SetEnabled(*payload.Enabled); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		if *payload.Enabled {
			// Launch the browser and mint one captcha token now so the first
			// completion does not pay the cold start.
			go h.zaiClient.WarmUp()
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"enabled":           h.zaiClient.IsEnabled(),
		"installed":         h.zaiClient.Installer().IsInstalled(),
		"passthrough_tools": h.zaiClient.PassthroughTools(),
	})
}

// POST /api/providers/zai/install (download the browser used for captcha minting)
func (h *CompletionHandler) InstallZaiBrowser(w http.ResponseWriter, r *http.Request) {
	if h.zaiClient == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "z.ai client not configured"})
		return
	}
	inst := h.zaiClient.Installer()
	if inst.IsInstalled() {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "state": "already_installed"})
		return
	}
	go func() {
		if err := inst.StartInstall(context.Background()); err != nil {
			fmt.Fprintf(os.Stderr, "[zai] browser install failed: %v\n", err)
		}
	}()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "state": inst.GetStatus().State})
}

// PUT /api/providers/zai/tools (toggle handing tool calls to the calling agent)
func (h *CompletionHandler) UpdateZaiTools(w http.ResponseWriter, r *http.Request) {
	if h.zaiClient == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "z.ai client not configured"})
		return
	}
	var payload struct {
		PassthroughTools *bool  `json:"passthrough_tools"`
		BrowserPath      string `json:"browser_path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Invalid JSON body"})
		return
	}
	if payload.PassthroughTools != nil {
		if err := h.zaiClient.SetPassthroughTools(*payload.PassthroughTools); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
	}
	if payload.BrowserPath != "" {
		if err := h.zaiClient.SetBrowserPath(payload.BrowserPath); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"passthrough_tools": h.zaiClient.PassthroughTools(),
	})
}

// GET /api/providers/chatgpt/models (Provider view: returns all ChatGPT models with enabled status)
func (h *CompletionHandler) ChatGPTProviderModels(w http.ResponseWriter, r *http.Request) {
	var rawModels []ModelItem
	if h.cgClient != nil {
		for _, m := range h.cgClient.GetCatalogModels() {
			item := ModelItem{
				ID:      chatgptPrefix + m,
				Object:  "model",
				OwnedBy: "chatgpt",
				Enabled: h.modelStore == nil || !h.modelStore.IsDisabled("chatgpt", m),
			}
			model.EnrichModelItem(&item, "chatgpt")
			rawModels = append(rawModels, item)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"provider": "chatgpt",
		"enabled":  h.cgClient != nil && h.cgClient.IsEnabled(),
		"models":   rawModels,
	})
}

// GET /api/providers/chatgpt/status (Returns provider enabled, browser installed, and install progress)
func (h *CompletionHandler) ChatGPTStatus(w http.ResponseWriter, r *http.Request) {
	if h.cgClient == nil {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"enabled":   false,
			"installed": false,
		})
		return
	}

	inst := h.cgClient.Installer()
	status := inst.GetStatus()
	execPath := inst.DetectedExecutable()
	installed := execPath != ""
	mem := h.cgClient.GetMemoryUsage()

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"enabled":           h.cgClient.IsEnabled(),
		"installed":         installed,
		"has_cookies":       h.cgClient.HasCookies(),
		"browser_path":      execPath,
		"install_state":     status.State,
		"install_progress":  status.Progress,
		"install_error":     status.Error,
		"browser_running":   mem.Running,
		"browser_pid":       mem.PID,
		"browser_procs":     mem.ProcessNum,
		"browser_rss_mb":    mem.TotalRSSMB,
		"shell_enabled":     h.cgClient.ShellEnabled(),
		"work_dir":          h.cgClient.ToolWorkdirPublic(),
		"passthrough_tools": h.cgClient.PassthroughTools(),
	})
}

// PUT /api/providers/chatgpt/tools (Toggle local tool execution settings)
func (h *CompletionHandler) UpdateChatGPTTools(w http.ResponseWriter, r *http.Request) {
	if h.cgClient == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "ChatGPT client not configured"})
		return
	}

	var payload struct {
		ShellEnabled     *bool   `json:"shell_enabled"`
		WorkDir          *string `json:"work_dir"`
		PassthroughTools *bool   `json:"passthrough_tools"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Invalid JSON body"})
		return
	}

	if payload.ShellEnabled != nil {
		if err := h.cgClient.SetShellEnabled(*payload.ShellEnabled); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
	}
	if payload.WorkDir != nil {
		if err := h.cgClient.SetWorkDir(*payload.WorkDir); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
	}
	if payload.PassthroughTools != nil {
		if err := h.cgClient.SetPassthroughTools(*payload.PassthroughTools); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"ok":                true,
		"shell_enabled":     h.cgClient.ShellEnabled(),
		"work_dir":          h.cgClient.ToolWorkdirPublic(),
		"passthrough_tools": h.cgClient.PassthroughTools(),
	})
}

// POST /api/providers/chatgpt/cookies (Upload cookies JSON)
func (h *CompletionHandler) UploadChatGPTCookies(w http.ResponseWriter, r *http.Request) {
	if h.cgClient == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "ChatGPT client not configured"})
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Failed to read request body"})
		return
	}

	if err := h.cgClient.SaveCookies(body); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"ok":          true,
		"has_cookies": true,
	})
}

// DELETE /api/providers/chatgpt/cookies (Clear uploaded cookies)
func (h *CompletionHandler) DeleteChatGPTCookies(w http.ResponseWriter, r *http.Request) {
	if h.cgClient == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "ChatGPT client not configured"})
		return
	}

	if err := h.cgClient.ClearCookies(); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"ok":          true,
		"has_cookies": false,
	})
}

// POST /api/providers/chatgpt/install (Starts downloading and extracting CloakBrowser)
func (h *CompletionHandler) InstallChatGPTBrowser(w http.ResponseWriter, r *http.Request) {
	if h.cgClient == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "ChatGPT client not configured"})
		return
	}

	if err := h.cgClient.Installer().StartInstall(r.Context()); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"ok":      true,
		"message": "Installation started",
	})
}

// PUT /api/providers/chatgpt/status (Toggle ChatGPT Web provider enabled/disabled)
func (h *CompletionHandler) UpdateChatGPTStatus(w http.ResponseWriter, r *http.Request) {
	if h.cgClient == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "ChatGPT client not configured"})
		return
	}

	var payload struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Invalid JSON body"})
		return
	}

	if err := h.cgClient.SetEnabled(payload.Enabled); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"ok":      true,
		"enabled": h.cgClient.IsEnabled(),
	})
}

// PUT /api/providers/chatgpt/models (Toggle single or batch ChatGPT model enabled/disabled)
// PUT /api/providers/zai/models (Toggle single or batch z.ai model enabled/disabled)
func (h *CompletionHandler) UpdateZaiProviderModels(w http.ResponseWriter, r *http.Request) {
	if h.modelStore == nil {
		http.Error(w, `{"error":"Model store not configured"}`, http.StatusInternalServerError)
		return
	}

	var payload struct {
		Model   string   `json:"model"`
		Models  []string `json:"models"`
		Enabled bool     `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Invalid JSON body"})
		return
	}

	for i := range payload.Models {
		payload.Models[i] = zai.CleanModelID(payload.Models[i])
	}
	payload.Model = zai.CleanModelID(payload.Model)

	var err error
	if len(payload.Models) > 0 {
		err = h.modelStore.SetBatchDisabled("zai", payload.Models, !payload.Enabled)
	} else if payload.Model != "" {
		err = h.modelStore.SetDisabled("zai", payload.Model, !payload.Enabled)
	} else {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Either model or models required"})
		return
	}
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "enabled": payload.Enabled})
}

func (h *CompletionHandler) UpdateChatGPTProviderModels(w http.ResponseWriter, r *http.Request) {
	if h.modelStore == nil {
		http.Error(w, `{"error":"Model store not configured"}`, http.StatusInternalServerError)
		return
	}

	var payload struct {
		Model   string   `json:"model"`
		Models  []string `json:"models"`
		Enabled bool     `json:"enabled"`
	}

	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Invalid JSON body"})
		return
	}

	for i := range payload.Models {
		payload.Models[i] = chatgpt.CleanModelID(payload.Models[i])
	}
	payload.Model = chatgpt.CleanModelID(payload.Model)

	var err error
	if len(payload.Models) > 0 {
		err = h.modelStore.SetBatchDisabled("chatgpt", payload.Models, !payload.Enabled)
	} else if payload.Model != "" {
		err = h.modelStore.SetDisabled("chatgpt", payload.Model, !payload.Enabled)
	} else {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Either model or models required"})
		return
	}

	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"ok":      true,
		"enabled": payload.Enabled,
	})
}

// POST /api/providers/opencode/models/refresh (Sync latest models from OpenCode Free upstream)
func (h *CompletionHandler) RefreshOpenCodeModels(w http.ResponseWriter, r *http.Request) {
	if h.client == nil || h.client.ModelManager() == nil {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": "Model manager not initialized"})
		return
	}

	models, updated, err := h.client.ModelManager().CheckAndUpdate(r.Context())
	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":      false,
			"error":   err.Error(),
			"count":   len(models),
			"updated": false,
		})
		return
	}

	_ = json.NewEncoder(w).Encode(map[string]any{
		"ok":      true,
		"count":   len(models),
		"updated": updated,
	})
}

// POST /api/providers/opencode/version/refresh (Check upstream GitHub for version updates)
func (h *CompletionHandler) RefreshVersion(w http.ResponseWriter, r *http.Request) {
	if h.client == nil || h.client.VersionManager() == nil {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": "Version manager not initialized"})
		return
	}

	version, updated, err := h.client.VersionManager().CheckAndUpdate(r.Context())
	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":      false,
			"version": version,
			"error":   err.Error(),
		})
		return
	}

	_ = json.NewEncoder(w).Encode(map[string]any{
		"ok":      true,
		"version": version,
		"updated": updated,
	})
}

// GET /api/providers/opencode/proxy (Get proxy status and pool counts)
func (h *CompletionHandler) OpenCodeProxyStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if h.client == nil || h.client.ProxyManager() == nil {
		_ = json.NewEncoder(w).Encode(map[string]any{"enabled": false, "total_count": 0})
		return
	}
	_ = json.NewEncoder(w).Encode(h.client.ProxyManager().Status())
}

// PUT /api/providers/opencode/proxy (Enable / disable proxy routing)
func (h *CompletionHandler) UpdateOpenCodeProxy(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if h.client == nil || h.client.ProxyManager() == nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "proxy manager not configured"})
		return
	}
	var payload struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "invalid payload"})
		return
	}
	h.client.ProxyManager().SetEnabled(payload.Enabled)
	_ = json.NewEncoder(w).Encode(h.client.ProxyManager().Status())
}

// POST /api/providers/opencode/proxy/refresh (Fetch fresh proxies from Geonode)
func (h *CompletionHandler) RefreshOpenCodeProxy(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if h.client == nil || h.client.ProxyManager() == nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "proxy manager not configured"})
		return
	}
	err := h.client.ProxyManager().Fetch(r.Context())
	if err != nil {
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": err.Error()})
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "status": h.client.ProxyManager().Status()})
}

// PUT /api/providers/opencode/models (Toggle single or batch model enabled/disabled)
func (h *CompletionHandler) AddCustomModel(w http.ResponseWriter, r *http.Request) {
	handleAddCustomModel(h.modelStore, "opencode", w, r)
}

func (h *CompletionHandler) DeleteCustomModel(w http.ResponseWriter, r *http.Request) {
	handleDeleteCustomModel(h.modelStore, "opencode", w, r)
}

func (h *CompletionHandler) UpdateProviderModels(w http.ResponseWriter, r *http.Request) {
	if h.modelStore == nil {
		http.Error(w, `{"error":"Model store not configured"}`, http.StatusInternalServerError)
		return
	}

	var payload struct {
		Model   string   `json:"model"`
		Models  []string `json:"models"`
		Enabled bool     `json:"enabled"`
	}

	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Invalid JSON body"})
		return
	}

	for i := range payload.Models {
		payload.Models[i] = cleanOpenCodeID(payload.Models[i])
	}
	payload.Model = cleanOpenCodeID(payload.Model)

	var err error
	if len(payload.Models) > 0 {
		err = h.modelStore.SetBatchDisabled("opencode", payload.Models, !payload.Enabled)
	} else if payload.Model != "" {
		err = h.modelStore.SetDisabled("opencode", payload.Model, !payload.Enabled)
	} else {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Either model or models required"})
		return
	}

	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"ok":      true,
		"enabled": payload.Enabled,
	})
}

type ModelTestResult struct {
	OK        bool   `json:"ok"`
	Model     string `json:"model"`
	LatencyMs int64  `json:"latency_ms"`
	Error     string `json:"error,omitempty"`
	Status    int    `json:"status,omitempty"`
	Response  string `json:"response,omitempty"`
}

type ModelTestPayload struct {
	Model       string   `json:"model,omitempty"`
	Models      []string `json:"models,omitempty"`
	Prompt      string   `json:"prompt,omitempty"`
	Concurrency int      `json:"concurrency,omitempty"`
}

func (h *CompletionHandler) pingSingleModel(ctx context.Context, model string, prompt string, customSession string) ModelTestResult {
	if prompt == "" {
		prompt = "ping"
	}

	// Antigravity Model Test
	provider, bareModel, _ := h.splitProviderPrefix(model)
	if provider == "antigravity" || (provider == "" && h.isAntigravityModel(model)) {
		cleanModel := antigravity.CleanModelID(bareModel)
		if h.modelStore != nil && h.modelStore.IsDisabled("antigravity", cleanModel) {
			return ModelTestResult{
				OK:        false,
				Model:     model,
				Error:     fmt.Sprintf("Model '%s' is disabled for provider antigravity", model),
				LatencyMs: 0,
			}
		}

		if h.agClient == nil {
			return ModelTestResult{
				OK:    false,
				Model: model,
				Error: "Antigravity provider not initialized",
			}
		}

		mCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()

		start := time.Now()
		testBody := map[string]any{
			"model": cleanModel,
			"messages": []map[string]string{
				{"role": "user", "content": prompt},
			},
		}

		upResp, err := h.agClient.ExecuteChat(mCtx, testBody, false)
		latencyMs := time.Since(start).Milliseconds()
		if err != nil {
			return ModelTestResult{
				OK:        false,
				Model:     model,
				Error:     err.Error(),
				LatencyMs: latencyMs,
			}
		}
		defer upResp.Body.Close()

		if upResp.StatusCode != http.StatusOK {
			bodyBytes, _ := io.ReadAll(upResp.Body)
			return ModelTestResult{
				OK:        false,
				Model:     model,
				Error:     string(bodyBytes),
				LatencyMs: latencyMs,
				Status:    upResp.StatusCode,
			}
		}

		return ModelTestResult{
			OK:        true,
			Model:     model,
			LatencyMs: latencyMs,
		}
	}

	// Kilo Code Model Test
	if provider == "kilocode" || (provider == "" && h.isKilocodeModel(model)) {
		cleanModel := kilocode.CleanModelID(bareModel)
		if h.modelStore != nil && h.modelStore.IsDisabled("kilocode", cleanModel) {
			return ModelTestResult{
				OK:        false,
				Model:     model,
				Error:     fmt.Sprintf("Model '%s' is disabled for provider kilocode", model),
				LatencyMs: 0,
			}
		}

		if h.kcClient == nil {
			return ModelTestResult{
				OK:    false,
				Model: model,
				Error: "Kilo Code provider not initialized",
			}
		}

		mCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()

		start := time.Now()
		testBody := map[string]any{
			"model": cleanModel,
			"messages": []map[string]string{
				{"role": "user", "content": prompt},
			},
		}

		upResp, _, err := h.kcClient.ExecuteChat(mCtx, testBody, false)
		latencyMs := time.Since(start).Milliseconds()
		if err != nil {
			return ModelTestResult{
				OK:        false,
				Model:     model,
				Error:     err.Error(),
				LatencyMs: latencyMs,
			}
		}
		defer upResp.Body.Close()

		if upResp.StatusCode != http.StatusOK {
			bodyBytes, _ := io.ReadAll(upResp.Body)
			return ModelTestResult{
				OK:        false,
				Model:     model,
				Error:     string(bodyBytes),
				LatencyMs: latencyMs,
				Status:    upResp.StatusCode,
			}
		}

		raw, _ := io.ReadAll(io.LimitReader(upResp.Body, 1<<20))
		return ModelTestResult{
			OK:        true,
			Model:     model,
			LatencyMs: latencyMs,
			Response:  assistantTextFromResponse(raw),
		}
	}

	// z.ai Model Test
	if provider == "zai" || (provider == "" && h.isZaiModel(model)) {
		cleanModel := zai.CleanModelID(bareModel)
		if h.modelStore != nil && h.modelStore.IsDisabled("zai", cleanModel) {
			return ModelTestResult{
				OK:        false,
				Model:     model,
				Error:     fmt.Sprintf("Model '%s' is disabled for provider zai", model),
				LatencyMs: 0,
			}
		}
		if h.zaiClient == nil || !h.zaiClient.IsEnabled() {
			return ModelTestResult{
				OK:    false,
				Model: model,
				Error: "z.ai provider is not enabled",
			}
		}

		mCtx, cancel := context.WithTimeout(ctx, 240*time.Second)
		defer cancel()

		start := time.Now()
		testBody := map[string]any{
			"model": cleanModel,
			"messages": []map[string]string{
				{"role": "user", "content": prompt},
			},
		}
		upResp, err := h.zaiClient.ExecuteChat(mCtx, testBody, false)
		latencyMs := time.Since(start).Milliseconds()
		if err != nil {
			return ModelTestResult{OK: false, Model: model, Error: err.Error(), LatencyMs: latencyMs}
		}
		defer upResp.Body.Close()

		if upResp.StatusCode != http.StatusOK {
			bodyBytes, _ := io.ReadAll(upResp.Body)
			return ModelTestResult{
				OK:        false,
				Model:     model,
				Error:     string(bodyBytes),
				LatencyMs: latencyMs,
				Status:    upResp.StatusCode,
			}
		}

		// Surface the reply text so the UI can show what the model actually
		// said, rather than only reporting that the call succeeded.
		raw, _ := io.ReadAll(io.LimitReader(upResp.Body, 1<<20))
		return ModelTestResult{
			OK:        true,
			Model:     model,
			LatencyMs: latencyMs,
			Response:  assistantTextFromResponse(raw),
		}
	}

	// ChatGPT Model Test
	if provider == "chatgpt" || (provider == "" && h.isChatGPTModel(model)) {
		cleanModel := chatgpt.CleanModelID(bareModel)
		if h.modelStore != nil && h.modelStore.IsDisabled("chatgpt", cleanModel) {
			return ModelTestResult{
				OK:        false,
				Model:     model,
				Error:     fmt.Sprintf("Model '%s' is disabled for provider chatgpt", model),
				LatencyMs: 0,
			}
		}

		if h.cgClient == nil {
			return ModelTestResult{
				OK:    false,
				Model: model,
				Error: "ChatGPT provider not initialized (browser not available)",
			}
		}

		mCtx, cancel := context.WithTimeout(ctx, 25*time.Second)
		defer cancel()

		start := time.Now()
		testBody := map[string]any{
			"model": cleanModel,
			"messages": []map[string]string{
				{"role": "user", "content": prompt},
			},
		}

		upResp, err := h.cgClient.ExecuteChat(mCtx, testBody, false)
		latencyMs := time.Since(start).Milliseconds()
		if err != nil {
			return ModelTestResult{
				OK:        false,
				Model:     model,
				Error:     err.Error(),
				LatencyMs: latencyMs,
			}
		}
		defer upResp.Body.Close()

		if upResp.StatusCode != http.StatusOK {
			bodyBytes, _ := io.ReadAll(upResp.Body)
			return ModelTestResult{
				OK:        false,
				Model:     model,
				Error:     string(bodyBytes),
				LatencyMs: latencyMs,
				Status:    upResp.StatusCode,
			}
		}

		return ModelTestResult{
			OK:        true,
			Model:     model,
			LatencyMs: latencyMs,
		}
	}

	// Custom Provider Model Test
	if cp, cleanModel, isCustom := h.findCustomProvider(provider, bareModel); isCustom {
		if !cp.IsEnabled {
			return ModelTestResult{
				OK:        false,
				Model:     model,
				Error:     fmt.Sprintf("Provider '%s' is disabled", cp.Name),
				LatencyMs: 0,
			}
		}

		if h.modelStore != nil && h.modelStore.IsDisabled(cp.Prefix, cleanModel) {
			return ModelTestResult{
				OK:        false,
				Model:     model,
				Error:     fmt.Sprintf("Model '%s' is disabled for provider %s", model, cp.Prefix),
				LatencyMs: 0,
			}
		}

		if h.customClient == nil {
			return ModelTestResult{
				OK:    false,
				Model: model,
				Error: "Custom provider client not initialized",
			}
		}

		mCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()

		start := time.Now()
		testBody := map[string]any{
			"model": cleanModel,
			"messages": []map[string]string{
				{"role": "user", "content": prompt},
			},
		}

		upResp, err := h.customClient.ExecuteChat(mCtx, cp, testBody, false)
		latencyMs := time.Since(start).Milliseconds()
		if err != nil {
			return ModelTestResult{
				OK:        false,
				Model:     model,
				Error:     err.Error(),
				LatencyMs: latencyMs,
			}
		}
		defer upResp.Body.Close()

		if upResp.StatusCode != http.StatusOK {
			bodyBytes, _ := io.ReadAll(upResp.Body)
			return ModelTestResult{
				OK:        false,
				Model:     model,
				Error:     string(bodyBytes),
				LatencyMs: latencyMs,
				Status:    upResp.StatusCode,
			}
		}

		return ModelTestResult{
			OK:        true,
			Model:     model,
			LatencyMs: latencyMs,
		}
	}

	// OpenCode Model Test
	if h.modelStore != nil && h.modelStore.IsDisabled("opencode", bareModel) {
		return ModelTestResult{
			OK:        false,
			Model:     model,
			Error:     fmt.Sprintf("Model '%s' is disabled", model),
			LatencyMs: 0,
		}
	}

	mCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	start := time.Now()
	testBody := map[string]any{
		"model": bareModel,
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
		"stream":            true,
		"max_tokens":        16,
		"max_output_tokens": 16,
	}

	upResp, err := h.client.ExecuteChat(mCtx, testBody, "model-test", customSession)
	latencyMs := time.Since(start).Milliseconds()
	if err != nil {
		return ModelTestResult{
			OK:        false,
			Model:     model,
			Error:     err.Error(),
			LatencyMs: latencyMs,
		}
	}
	defer upResp.Body.Close()

	if upResp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(upResp.Body)
		var errObj struct {
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		errMsg := string(bodyBytes)
		if json.Unmarshal(bodyBytes, &errObj) == nil && errObj.Error.Message != "" {
			errMsg = errObj.Error.Message
		}
		return ModelTestResult{
			OK:        false,
			Model:     model,
			Error:     errMsg,
			LatencyMs: latencyMs,
			Status:    upResp.StatusCode,
		}
	}

	// Read initial stream byte to confirm TTFT
	reader := bufio.NewReader(upResp.Body)
	_, _ = reader.ReadBytes('\n')
	latencyMs = time.Since(start).Milliseconds()

	return ModelTestResult{
		OK:        true,
		Model:     model,
		LatencyMs: latencyMs,
	}
}

// POST /api/models/test (Ping model or batch of models)
func (h *CompletionHandler) TestModel(w http.ResponseWriter, r *http.Request) {
	var payload ModelTestPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": "Invalid request body"})
		return
	}

	customSession := r.Header.Get("x-opencode-session")
	if customSession == "" {
		customSession = r.Header.Get("x-opencode-session-id")
	}

	// Batch mode
	if len(payload.Models) > 0 {
		concurrency := payload.Concurrency
		if concurrency <= 0 {
			concurrency = 4
		} else if concurrency > 10 {
			concurrency = 10
		}

		results := make([]ModelTestResult, len(payload.Models))
		sem := make(chan struct{}, concurrency)
		var wg sync.WaitGroup

		for i, m := range payload.Models {
			wg.Add(1)
			go func(idx int, modelID string) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()

				results[idx] = h.pingSingleModel(r.Context(), modelID, payload.Prompt, customSession)
			}(i, m)
		}

		wg.Wait()

		passed := 0
		failed := 0
		for _, res := range results {
			if res.OK {
				passed++
			} else {
				failed++
			}
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":      failed == 0,
			"results": results,
			"total":   len(results),
			"passed":  passed,
			"failed":  failed,
		})
		return
	}

	// Single model mode
	if payload.Model == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": "Model or models identifier required"})
		return
	}

	res := h.pingSingleModel(r.Context(), payload.Model, payload.Prompt, customSession)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(res)
}

// opencodeModelList returns the OpenCode Free catalogue (cached, no network).
func opencodeModelList(client *opencode.Client) []string {
	if client != nil && client.ModelManager() != nil {
		return client.ModelManager().GetModels()
	}
	return opencode.FallbackModels
}

func containsModel(models []string, id string) bool {
	for _, m := range models {
		if m == id {
			return true
		}
	}
	return false
}

// isAntigravityModel resolves which provider owns a model ID. The opencode/ and
// antigravity/ prefixes are explicit; bare IDs fall back to catalog ownership, where
// a shared ID (gemini-3.8-flash, claude-sonnet-5-5, ...) stays with OpenCode Free.
func (h *CompletionHandler) isAntigravityModel(model string) bool {
	if provider, _, ok := splitProviderPrefix(model); ok {
		return provider == "antigravity"
	}
	clean := antigravity.CleanModelID(model)
	if h.modelStore != nil && h.modelStore.HasCustomModel("antigravity", clean) {
		return true
	}
	return antigravityShouldHandle(model, opencodeModelList(h.client))
}

func antigravityShouldHandle(model string, opencodeModels []string) bool {
	clean := antigravity.CleanModelID(model)
	if !antigravity.IsCatalogModel(clean) {
		return false
	}
	return !containsModel(opencodeModels, clean)
}

// isZaiModel resolves whether a model belongs to the z.ai provider.
func (h *CompletionHandler) isZaiModel(model string) bool {
	if provider, _, ok := splitProviderPrefix(model); ok {
		return provider == "zai"
	}
	return zai.IsCatalogModel(model)
}

// isChatGPTModel resolves whether a model belongs to the ChatGPT provider.
func (h *CompletionHandler) isChatGPTModel(model string) bool {
	if provider, _, ok := splitProviderPrefix(model); ok {
		return provider == "chatgpt"
	}
	return chatgpt.IsCatalogModel(model)
}

// isKilocodeModel resolves whether a model belongs to the Kilo Code provider.
func (h *CompletionHandler) isKilocodeModel(model string) bool {
	if provider, _, ok := splitProviderPrefix(model); ok {
		return provider == "kilocode"
	}
	clean := kilocode.CleanModelID(model)
	if h.modelStore != nil && h.modelStore.HasCustomModel("kilocode", clean) {
		return true
	}
	if h.kcClient == nil {
		return false
	}
	return h.kcClient.IsCatalogModel(clean)
}

// splitProviderPrefix resolves an explicit provider namespace. ok is false for a
// bare ID, which keeps catalog-based routing.
func splitProviderPrefix(model string) (provider, id string, ok bool) {
	return splitProviderPrefixWithStore(model, nil)
}

func (h *CompletionHandler) splitProviderPrefix(model string) (provider, id string, ok bool) {
	var store *customprovider.Store
	if h != nil {
		store = h.customStore
	}
	return splitProviderPrefixWithStore(model, store)
}

func splitProviderPrefixWithStore(model string, cpStore *customprovider.Store) (provider, id string, ok bool) {
	switch {
	case strings.HasPrefix(model, opencodePrefix):
		return "opencode", strings.TrimPrefix(model, opencodePrefix), true
	case antigravity.HasAntigravityPrefix(model):
		return "antigravity", antigravity.CleanModelID(model), true
	case strings.HasPrefix(model, chatgptPrefix):
		return "chatgpt", strings.TrimPrefix(model, chatgptPrefix), true
	case strings.HasPrefix(model, zaiPrefix):
		return "zai", strings.TrimPrefix(model, zaiPrefix), true
	case kilocode.HasKilocodePrefix(model):
		return "kilocode", kilocode.CleanModelID(model), true
	default:
		if cpStore != nil {
			if slashIdx := strings.Index(model, "/"); slashIdx > 0 {
				prefix := model[:slashIdx]
				if cp, ok := cpStore.GetByPrefix(prefix); ok {
					return cp.Prefix, model[slashIdx+1:], true
				}
			}
		}
		return "", model, false
	}
}

func (h *CompletionHandler) findCustomProvider(providerPrefix, model string) (customprovider.Provider, string, bool) {
	if h.customStore == nil {
		return customprovider.Provider{}, "", false
	}
	if providerPrefix != "" {
		if cp, ok := h.customStore.GetByPrefix(providerPrefix); ok {
			cleanModel := customprovider.CleanModelID(cp.Prefix, model)
			return cp, cleanModel, true
		}
	}
	// Bare model fallback: see if any enabled custom provider has it
	if cp, ok := h.customStore.FindByModel(model); ok {
		return cp, model, true
	}
	return customprovider.Provider{}, "", false
}

func cleanOpenCodeID(model string) string {
	return strings.TrimPrefix(model, opencodePrefix)
}

// isAntigravityFamilyModel reports Gemini/Claude IDs, which the Antigravity provider
// owns. OpenCode Free still accepts them, but does not advertise them.
func isAntigravityFamilyModel(id string) bool {
	clean := strings.ToLower(antigravity.CleanModelID(id))
	return strings.HasPrefix(clean, "gemini") || strings.HasPrefix(clean, "claude")
}

func (h *CompletionHandler) fetchAllModels(r *http.Request) []ModelItem {
	data, err := h.client.FetchModels(r.Context())
	if err == nil {
		var resp struct {
			Data []ModelItem `json:"data"`
		}
		if jsonErr := json.Unmarshal(data, &resp); jsonErr == nil && len(resp.Data) > 0 {
			var list []ModelItem
			for i := range resp.Data {
				if isAntigravityFamilyModel(resp.Data[i].ID) {
					// Antigravity serves Gemini/Claude; keep them out of the OpenCode Free list
					continue
				}
				resp.Data[i].OwnedBy = "opencode"
				model.EnrichModelItem(&resp.Data[i], "opencode")
				list = append(list, resp.Data[i])
			}
			return list
		}
	}

	// Fallback models from ModelManager or FallbackModels
	var fallbackIDs []string
	if h.client != nil && h.client.ModelManager() != nil {
		fallbackIDs = h.client.ModelManager().GetModels()
	} else {
		fallbackIDs = opencode.FallbackModels
	}

	var list []ModelItem
	for _, m := range fallbackIDs {
		if isAntigravityFamilyModel(m) {
			continue
		}
		item := ModelItem{
			ID:      m,
			Object:  "model",
			OwnedBy: "opencode",
		}
		model.EnrichModelItem(&item, "opencode")
		list = append(list, item)
	}
	return list
}

func (h *CompletionHandler) ChatCompletions(w http.ResponseWriter, r *http.Request) {
	// 1. Authenticate with router API key
	isInternalAPI := strings.HasPrefix(r.URL.Path, "/api/chat/completions")

	var keyID, keyName string

	authHeader := r.Header.Get("Authorization")
	token := strings.TrimPrefix(authHeader, "Bearer ")
	if token == "" || token == authHeader {
		token = r.URL.Query().Get("key")
	}

	if token == "" {
		if !isInternalAPI {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"error": map[string]string{
					"message": "Missing API key. Provide Authorization: Bearer <key>",
					"type":    "authentication_error",
				},
			})
			return
		}
		token = "web-ui-test"
	} else {
		key, ok := h.store.FindByKey(token)
		if ok {
			keyID, keyName = key.ID, key.Name
		}
		if (!ok || !key.IsActive) && !isInternalAPI {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"error": map[string]string{
					"message": "Invalid or deactivated API key",
					"type":    "authentication_error",
				},
			})
			return
		}
	}

	// 2. Decode incoming request body
	var bodyMap map[string]any
	if err := json.NewDecoder(r.Body).Decode(&bodyMap); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]string{
				"message": "Invalid JSON request body",
				"type":    "invalid_request_error",
			},
		})
		return
	}

	model, _ := bodyMap["model"].(string)
	if model == "" {
		model = "big-pickle"
	}

	if os.Getenv("CHATGPT_DEBUG_BODY") == "1" {
		if dbg, err := json.MarshalIndent(bodyMap, "", "  "); err == nil {
			fmt.Fprintf(os.Stderr, "[chatgpt-debug] request model=%s body:\n%s\n", model, string(dbg))
		}
	}
	requestedModel := model
	modelProvider, bareModel, prefixed := h.splitProviderPrefix(model)
	if prefixed {
		model = bareModel
	}
	bodyMap["model"] = model

	clientWantsStream, _ := bodyMap["stream"].(bool)

	// Usage monitoring: one event per request, written when the handler returns.
	rec := &statusRecorder{ResponseWriter: w}
	w = rec
	reqStart := time.Now()
	tokens := &usage.Tokens{}
	ev := usage.Event{
		KeyID:    keyID,
		KeyName:  keyName,
		Model:    model,
		ReqModel: requestedModel,
		Endpoint: r.URL.Path,
		Stream:   clientWantsStream,
		Agent:    r.Header.Get("User-Agent"),
		Sess:     clientSessionID(r),
	}
	defer func() {
		ev.Status = rec.status
		if ev.Status == 0 {
			ev.Status = http.StatusOK
		}
		ev.Ms = time.Since(reqStart).Milliseconds()
		ev.In, ev.Out, ev.Cached, ev.Reasoning = tokens.In, tokens.Out, tokens.Cached, tokens.Reasoning
		ev.TokSrc = tokens.Src
		if ev.TokSrc == "" {
			ev.TokSrc = "none"
		}
		if h.usage != nil {
			h.usage.Record(ev)
		}
	}()

	// Compactor: auto-compress tool output before dispatching to upstream LLMs
	// Client opt-out via X-Compact: off or ?compact=off
	clientOptOut := strings.EqualFold(r.Header.Get("X-Compact"), "off") ||
		strings.EqualFold(r.URL.Query().Get("compact"), "off")

	if h.compactorService != nil {
		if stats := h.compactorService.ProcessBody(bodyMap, clientOptOut); stats != nil {
			ev.Saved = stats.BytesBefore - stats.BytesAfter
			ev.Filters = stats.FilterNames()
		}
	}

	// Caveman voice & prompt optimization (say less, cut output tokens)
	headerCaveman := r.Header.Get("X-Caveman")
	headerMode := r.Header.Get("X-Caveman-Mode")
	if clientOptOut {
		headerCaveman = "off"
	}
	if h.cavemanService != nil {
		h.cavemanService.Apply(bodyMap, headerCaveman, headerMode)
	}

	// Ponytail lazy senior developer mode (minimal code, anti-bloat)
	headerPonytail := r.Header.Get("X-Ponytail")
	headerPonytailMode := r.Header.Get("X-Ponytail-Mode")
	if clientOptOut {
		headerPonytail = "off"
	}
	if h.ponytailService != nil {
		h.ponytailService.Apply(bodyMap, headerPonytail, headerPonytailMode)
	}

	// Check if this is an Antigravity model
	if modelProvider == "antigravity" || (modelProvider == "" && h.isAntigravityModel(model)) {
		cleanModel := antigravity.CleanModelID(model)
		if h.modelStore != nil && h.modelStore.IsDisabled("antigravity", cleanModel) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"error": map[string]string{
					"message": fmt.Sprintf("Model '%s' is disabled for provider antigravity", model),
					"type":    "invalid_request_error",
				},
			})
			return
		}

		if h.agClient == nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadGateway)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"error": map[string]string{
					"message": "Antigravity provider not initialized",
					"type":    "api_error",
				},
			})
			return
		}

		ev.Provider = "antigravity"
		if acc, ok := h.agClient.GetActiveForModel(model); ok {
			ev.Account = acc.Email
		}

		upResp, err := h.agClient.ExecuteChat(r.Context(), bodyMap, clientWantsStream)
		if err != nil {
			ev.Err = "upstream_error"
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadGateway)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"error": map[string]string{
					"message": fmt.Sprintf("Upstream Antigravity error: %v", err),
					"type":    "api_error",
				},
			})
			return
		}
		ev.UpMs = time.Since(reqStart).Milliseconds()
		defer upResp.Body.Close()

		if upResp.StatusCode != http.StatusOK {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(upResp.StatusCode)
			bodyBytes, _ := io.ReadAll(upResp.Body)
			_, _ = w.Write(bodyBytes)
			return
		}

		if clientWantsStream {
			w.Header().Set("Content-Type", "text/event-stream")
			w.Header().Set("Cache-Control", "no-cache")
			w.Header().Set("Connection", "keep-alive")
			flusher, canFlush := w.(http.Flusher)
			streamAntigravityAsOpenAI(w, flusher, canFlush, upResp.Body, model, tokens)
			return
		}

		handleAntigravityNonStreamResponse(w, upResp.Body, model, tokens)
		return
	}

	// Check if this is a ChatGPT model
	if modelProvider == "zai" || (modelProvider == "" && h.isZaiModel(model)) {
		cleanModel := zai.CleanModelID(model)
		if h.modelStore != nil && h.modelStore.IsDisabled("zai", cleanModel) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"error": map[string]string{
					"message": fmt.Sprintf("Model '%s' is disabled for provider zai", model),
					"type":    "invalid_request_error",
				},
			})
			return
		}

		if h.zaiClient == nil || !h.zaiClient.IsEnabled() {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadGateway)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"error": map[string]string{
					"message": "z.ai provider is not enabled. Enable it from the provider page.",
					"type":    "api_error",
				},
			})
			return
		}

		ev.Provider = "zai"
		upResp, err := h.zaiClient.ExecuteChat(r.Context(), bodyMap, clientWantsStream)
		if err != nil {
			ev.Err = "upstream_error"
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadGateway)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"error": map[string]string{
					"message": err.Error(),
					"type":    "api_error",
				},
			})
			return
		}
		ev.UpMs = time.Since(reqStart).Milliseconds()
		defer upResp.Body.Close()

		if upResp.StatusCode != http.StatusOK {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(upResp.StatusCode)
			bodyBytes, _ := io.ReadAll(upResp.Body)
			_, _ = w.Write(bodyBytes)
			return
		}

		if clientWantsStream {
			w.Header().Set("Content-Type", "text/event-stream")
			w.Header().Set("Cache-Control", "no-cache")
			w.Header().Set("Connection", "keep-alive")
			flusher, canFlush := w.(http.Flusher)
			scanner := bufio.NewScanner(upResp.Body)
			scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
			for scanner.Scan() {
				line := scanner.Bytes()
				_, _ = w.Write(line)
				_, _ = w.Write([]byte("\n"))
				if canFlush {
					flusher.Flush()
				}
			}
			return
		}

		bodyBytes, _ := io.ReadAll(upResp.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(bodyBytes)
		return
	}

	// Check if this is a Kilo Code model
	if modelProvider == "kilocode" || (modelProvider == "" && h.isKilocodeModel(model)) {
		cleanModel := kilocode.CleanModelID(model)
		if h.modelStore != nil && h.modelStore.IsDisabled("kilocode", cleanModel) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"error": map[string]string{
					"message": fmt.Sprintf("Model '%s' is disabled for provider kilocode", model),
					"type":    "invalid_request_error",
				},
			})
			return
		}

		if h.kcClient == nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadGateway)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"error": map[string]string{
					"message": "Kilo Code provider not initialized",
					"type":    "api_error",
				},
			})
			return
		}

		ev.Provider = "kilocode"
		upResp, acc, err := h.kcClient.ExecuteChat(r.Context(), bodyMap, clientWantsStream)
		if err != nil {
			ev.Err = "upstream_error"
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadGateway)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"error": map[string]string{
					"message": fmt.Sprintf("Upstream Kilo Code error: %v", err),
					"type":    "api_error",
				},
			})
			return
		}
		if acc != nil {
			ev.Account = acc.Email
		}
		ev.UpMs = time.Since(reqStart).Milliseconds()
		defer upResp.Body.Close()

		if upResp.StatusCode != http.StatusOK {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(upResp.StatusCode)
			bodyBytes, _ := io.ReadAll(upResp.Body)
			_, _ = w.Write(bodyBytes)
			return
		}

		if clientWantsStream {
			w.Header().Set("Content-Type", "text/event-stream")
			w.Header().Set("Cache-Control", "no-cache")
			w.Header().Set("Connection", "keep-alive")
			flusher, canFlush := w.(http.Flusher)
			reader := bufio.NewReader(upResp.Body)
			for {
				line, err := reader.ReadBytes('\n')
				if len(line) > 0 {
					captureStreamUsage(line, tokens)
					_, _ = w.Write(line)
					if canFlush {
						flusher.Flush()
					}
				}
				if err != nil {
					break
				}
			}
			return
		}

		bodyBytes, _ := io.ReadAll(upResp.Body)
		captureNonStreamUsage(bodyBytes, tokens)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(bodyBytes)
		return
	}

	if modelProvider == "chatgpt" || (modelProvider == "" && h.isChatGPTModel(model)) {
		cleanModel := chatgpt.CleanModelID(model)
		if h.modelStore != nil && h.modelStore.IsDisabled("chatgpt", cleanModel) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"error": map[string]string{
					"message": fmt.Sprintf("Model '%s' is disabled for provider chatgpt", model),
					"type":    "invalid_request_error",
				},
			})
			return
		}

		if h.cgClient == nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadGateway)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"error": map[string]string{
					"message": "ChatGPT provider not initialized (browser not available)",
					"type":    "api_error",
				},
			})
			return
		}

		ev.Provider = "chatgpt"
		upResp, err := h.cgClient.ExecuteChat(r.Context(), bodyMap, clientWantsStream)
		if err != nil {
			ev.Err = "upstream_error"
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadGateway)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"error": map[string]string{
					"message": fmt.Sprintf("Upstream ChatGPT error: %v", err),
					"type":    "api_error",
				},
			})
			return
		}
		ev.UpMs = time.Since(reqStart).Milliseconds()
		defer upResp.Body.Close()

		if upResp.StatusCode != http.StatusOK {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(upResp.StatusCode)
			bodyBytes, _ := io.ReadAll(upResp.Body)
			_, _ = w.Write(bodyBytes)
			return
		}

		if clientWantsStream {
			w.Header().Set("Content-Type", "text/event-stream")
			w.Header().Set("Cache-Control", "no-cache")
			w.Header().Set("Connection", "keep-alive")
			flusher, canFlush := w.(http.Flusher)
			scanner := bufio.NewScanner(upResp.Body)
			for scanner.Scan() {
				line := scanner.Bytes()
				_, _ = w.Write(line)
				_, _ = w.Write([]byte("\n"))
				if canFlush {
					flusher.Flush()
				}
			}
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = io.Copy(w, upResp.Body)
		return
	}

	// Check if this is a custom provider model
	if cp, cleanModel, isCustom := h.findCustomProvider(modelProvider, model); isCustom {
		if !cp.IsEnabled {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"error": map[string]string{
					"message": fmt.Sprintf("Provider '%s' is disabled", cp.Name),
					"type":    "invalid_request_error",
				},
			})
			return
		}

		if h.modelStore != nil && h.modelStore.IsDisabled(cp.Prefix, cleanModel) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"error": map[string]string{
					"message": fmt.Sprintf("Model '%s' is disabled for provider %s", cleanModel, cp.Prefix),
					"type":    "invalid_request_error",
				},
			})
			return
		}

		if h.customClient == nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadGateway)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"error": map[string]string{
					"message": "Custom provider client not initialized",
					"type":    "api_error",
				},
			})
			return
		}

		ev.Provider = cp.Prefix
		ev.Account = cp.Name

		upResp, err := h.customClient.ExecuteChat(r.Context(), cp, bodyMap, clientWantsStream)
		if err != nil {
			ev.Err = "upstream_error"
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadGateway)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"error": map[string]string{
					"message": fmt.Sprintf("Upstream %s error: %v", cp.Name, err),
					"type":    "api_error",
				},
			})
			return
		}
		ev.UpMs = time.Since(reqStart).Milliseconds()
		defer upResp.Body.Close()

		if upResp.StatusCode != http.StatusOK {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(upResp.StatusCode)
			bodyBytes, _ := io.ReadAll(upResp.Body)
			_, _ = w.Write(bodyBytes)
			return
		}

		if clientWantsStream {
			w.Header().Set("Content-Type", "text/event-stream")
			w.Header().Set("Cache-Control", "no-cache")
			w.Header().Set("Connection", "keep-alive")
			flusher, canFlush := w.(http.Flusher)
			reader := bufio.NewReader(upResp.Body)
			for {
				line, err := reader.ReadBytes('\n')
				if len(line) > 0 {
					captureStreamUsage(line, tokens)
					_, _ = w.Write(line)
					if canFlush {
						flusher.Flush()
					}
				}
				if err != nil {
					break
				}
			}
			return
		}

		bodyBytes, _ := io.ReadAll(upResp.Body)
		captureNonStreamUsage(bodyBytes, tokens)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(bodyBytes)
		return
	}

	ev.Provider = "opencode"

	// Check if requested model is disabled for OpenCode
	if h.modelStore != nil && h.modelStore.IsDisabled("opencode", model) {
		ev.Err = "model_disabled"
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]string{
				"message": fmt.Sprintf("Model '%s' is disabled for provider opencode", model),
				"type":    "invalid_request_error",
			},
		})
		return
	}

	// 3. Execute with OpenCode client
	customSession := r.Header.Get("x-opencode-session")
	if customSession == "" {
		customSession = r.Header.Get("x-opencode-session-id")
	}

	upResp, err := h.client.ExecuteChat(r.Context(), bodyMap, token, customSession)
	if err != nil {
		ev.Err = "upstream_error"
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]string{
				"message": fmt.Sprintf("Upstream OpenCode error: %v", err),
				"type":    "api_error",
			},
		})
		return
	}
	defer upResp.Body.Close()
	ev.UpMs = time.Since(reqStart).Milliseconds()

	if upResp.StatusCode != http.StatusOK {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(upResp.StatusCode)
		scanner := bufio.NewScanner(upResp.Body)
		for scanner.Scan() {
			_, _ = w.Write(scanner.Bytes())
		}
		return
	}

	// 4. Stream response to client if requested
	if clientWantsStream {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		flusher, canFlush := w.(http.Flusher)

		isResponses := strings.HasPrefix(model, "muse-spark")
		if isResponses {
			streamResponsesAsOpenAI(w, flusher, canFlush, upResp.Body, model, tokens)
			return
		}

		reader := bufio.NewReader(upResp.Body)
		for {
			line, err := reader.ReadBytes('\n')
			if len(line) > 0 {
				captureStreamUsage(line, tokens)
				_, _ = w.Write(line)
				if canFlush {
					flusher.Flush()
				}
			}
			if err != nil {
				break
			}
		}
		return
	}

	// 5. Aggregate streaming chunks into full JSON response
	fullResp, err := opencode.AggregateSSEStream(upResp.Body, model, tokens)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]string{
				"message": fmt.Sprintf("Failed to aggregate stream: %v", err),
				"type":    "api_error",
			},
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(fullResp)
}

func streamResponsesAsOpenAI(w io.Writer, flusher http.Flusher, canFlush bool, body io.Reader, model string, tk *usage.Tokens) {
	reader := bufio.NewReader(body)
	streamID := fmt.Sprintf("chatcmpl-%d", time.Now().UnixNano())
	created := time.Now().Unix()

	sentRole := false

	for {
		line, err := reader.ReadBytes('\n')
		if len(line) > 0 {
			trimmed := strings.TrimSpace(string(line))
			if strings.HasPrefix(trimmed, "data: ") {
				payload := strings.TrimPrefix(trimmed, "data: ")
				if payload == "[DONE]" {
					_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
					if canFlush {
						flusher.Flush()
					}
					break
				}

				var raw map[string]any
				if json.Unmarshal([]byte(payload), &raw) == nil {
					if _, hasChoices := raw["choices"]; hasChoices {
						_, _ = fmt.Fprintf(w, "data: %s\n\n", payload)
						if canFlush {
							flusher.Flush()
						}
					} else {
						msgType, _ := raw["type"].(string)
						if msgType == "response.completed" {
							if respMap, ok := raw["response"].(map[string]any); ok {
								(&usageEnvelope{}).applyAny(respMap["usage"], tk)
							}
							(&usageEnvelope{}).applyAny(raw["usage"], tk)
						}

						// 1. First assistant delta: send role chunk if needed
						if !sentRole && (msgType == "response.output_text.delta" || msgType == "response.output_item.added") {
							roleChunk, _ := json.Marshal(map[string]any{
								"id":      streamID,
								"object":  "chat.completion.chunk",
								"created": created,
								"model":   model,
								"choices": []map[string]any{
									{
										"index": 0,
										"delta": map[string]string{
											"role": "assistant",
										},
									},
								},
							})
							_, _ = fmt.Fprintf(w, "data: %s\n\n", roleChunk)
							if canFlush {
								flusher.Flush()
							}
							sentRole = true
						}

						// 2. Text delta
						if msgType == "response.output_text.delta" {
							var textDelta string
							if dStr, ok := raw["delta"].(string); ok {
								textDelta = dStr
							} else if dMap, ok := raw["delta"].(map[string]any); ok {
								if t, ok := dMap["text"].(string); ok {
									textDelta = t
								}
							}

							if textDelta != "" {
								chunk, _ := json.Marshal(map[string]any{
									"id":      streamID,
									"object":  "chat.completion.chunk",
									"created": created,
									"model":   model,
									"choices": []map[string]any{
										{
											"index": 0,
											"delta": map[string]string{
												"content": textDelta,
											},
										},
									},
								})
								_, _ = fmt.Fprintf(w, "data: %s\n\n", chunk)
								if canFlush {
									flusher.Flush()
								}
							}
						} else if msgType == "response.completed" {
							// 3. Final chunk with finish_reason: "stop"
							finishChunk, _ := json.Marshal(map[string]any{
								"id":      streamID,
								"object":  "chat.completion.chunk",
								"created": created,
								"model":   model,
								"choices": []map[string]any{
									{
										"index":         0,
										"delta":         map[string]string{},
										"finish_reason": "stop",
									},
								},
							})
							_, _ = fmt.Fprintf(w, "data: %s\n\n", finishChunk)
							_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
							if canFlush {
								flusher.Flush()
							}
						}
					}
				}
			}
		}
		if err != nil {
			break
		}
	}
}

func streamAntigravityAsOpenAI(w io.Writer, flusher http.Flusher, canFlush bool, body io.Reader, model string, tk *usage.Tokens) {
	reader := bufio.NewReader(body)
	streamID := fmt.Sprintf("chatcmpl-%d", time.Now().UnixNano())
	created := time.Now().Unix()

	sentRole := false
	toolCallIndex := 0
	toolCallCount := 0
	var pendingThoughtSignature string

	for {
		line, err := reader.ReadBytes('\n')
		if len(line) > 0 {
			trimmed := strings.TrimSpace(string(line))
			if strings.HasPrefix(trimmed, "data: ") {
				payload := strings.TrimPrefix(trimmed, "data: ")
				if payload == "[DONE]" {
					if tk.Src == "upstream" {
						usageChunk, _ := json.Marshal(map[string]any{
							"id":      streamID,
							"object":  "chat.completion.chunk",
							"created": created,
							"model":   model,
							"choices": []any{},
							"usage":   usagePayload(tk),
						})
						_, _ = fmt.Fprintf(w, "data: %s\n\n", usageChunk)
					}
					_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
					if canFlush {
						flusher.Flush()
					}
					break
				}

				var data map[string]any
				if jsonErr := json.Unmarshal([]byte(payload), &data); jsonErr == nil {
					// Handle Antigravity envelope
					respMap, ok := data["response"].(map[string]any)
					if !ok {
						respMap = data
					}

					(&usageEnvelope{}).applyAny(respMap["usageMetadata"], tk)

					candidates, _ := respMap["candidates"].([]any)
					if len(candidates) > 0 {
						if cand, ok := candidates[0].(map[string]any); ok {
							// 1. Initial role chunk
							if !sentRole {
								sentRole = true
								roleChunk, _ := json.Marshal(map[string]any{
									"id":      streamID,
									"object":  "chat.completion.chunk",
									"created": created,
									"model":   model,
									"choices": []map[string]any{
										{
											"index": 0,
											"delta": map[string]string{
												"role": "assistant",
											},
										},
									},
								})
								_, _ = fmt.Fprintf(w, "data: %s\n\n", roleChunk)
								if canFlush {
									flusher.Flush()
								}
							}

							// 2. Process parts
							if content, ok := cand["content"].(map[string]any); ok {
								if parts, ok := content["parts"].([]any); ok {
									for _, pItem := range parts {
										if pMap, ok := pItem.(map[string]any); ok {
											// Check thought signature
											if sig, ok := pMap["thoughtSignature"].(string); ok && sig != "" {
												pendingThoughtSignature = sig
											} else if sig, ok := pMap["thought_signature"].(string); ok && sig != "" {
												pendingThoughtSignature = sig
											}

											isThought, _ := pMap["thought"].(bool)
											text, hasText := pMap["text"].(string)

											// A. Text / Reasoning delta
											if hasText && text != "" {
												delta := map[string]any{}
												if isThought {
													delta["reasoning_content"] = text
												} else {
													delta["content"] = text
												}
												chunk, _ := json.Marshal(map[string]any{
													"id":      streamID,
													"object":  "chat.completion.chunk",
													"created": created,
													"model":   model,
													"choices": []map[string]any{
														{
															"index": 0,
															"delta": delta,
														},
													},
												})
												_, _ = fmt.Fprintf(w, "data: %s\n\n", chunk)
												if canFlush {
													flusher.Flush()
												}
											}

											// B. Function call (tool call)
											if fc, ok := pMap["functionCall"].(map[string]any); ok {
												fcName, _ := fc["name"].(string)
												fcArgs, _ := fc["args"].(map[string]any)
												if fcArgs == nil {
													fcArgs = map[string]any{}
												}
												argsBytes, _ := json.Marshal(fcArgs)
												callID, _ := fc["id"].(string)
												if callID == "" {
													callID = fmt.Sprintf("%s-%d-%d", fcName, time.Now().UnixMilli(), toolCallIndex)
												}

												if pendingThoughtSignature != "" {
													antigravity.StoreThoughtSignature(callID, pendingThoughtSignature)
													pendingThoughtSignature = ""
												}

												chunk, _ := json.Marshal(map[string]any{
													"id":      streamID,
													"object":  "chat.completion.chunk",
													"created": created,
													"model":   model,
													"choices": []map[string]any{
														{
															"index": 0,
															"delta": map[string]any{
																"tool_calls": []map[string]any{
																	{
																		"index": toolCallIndex,
																		"id":    callID,
																		"type":  "function",
																		"function": map[string]string{
																			"name":      fcName,
																			"arguments": string(argsBytes),
																		},
																	},
																},
															},
														},
													},
												})
												_, _ = fmt.Fprintf(w, "data: %s\n\n", chunk)
												if canFlush {
													flusher.Flush()
												}
												toolCallIndex++
												toolCallCount++
											}
										}
									}
								}
							}

							// 3. Finish reason
							finishReason, _ := cand["finishReason"].(string)
							if finishReason != "" {
								fr := "stop"
								if toolCallCount > 0 {
									fr = "tool_calls"
								}
								finishChunk, _ := json.Marshal(map[string]any{
									"id":      streamID,
									"object":  "chat.completion.chunk",
									"created": created,
									"model":   model,
									"choices": []map[string]any{
										{
											"index":         0,
											"delta":         map[string]string{},
											"finish_reason": fr,
										},
									},
								})
								_, _ = fmt.Fprintf(w, "data: %s\n\n", finishChunk)
								_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
								if canFlush {
									flusher.Flush()
								}
								break
							}
						}
					}
				}
			}
		}
		if err != nil {
			break
		}
	}
}

func handleAntigravityNonStreamResponse(w http.ResponseWriter, body io.Reader, model string, tk *usage.Tokens) {
	bodyBytes, err := io.ReadAll(body)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Failed to read response"})
		return
	}

	var data map[string]any
	if jsonErr := json.Unmarshal(bodyBytes, &data); jsonErr != nil {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(bodyBytes)
		return
	}

	respMap, ok := data["response"].(map[string]any)
	if !ok {
		respMap = data
	}

	(&usageEnvelope{}).applyAny(respMap["usageMetadata"], tk)

	var fullText string
	var reasoningText string
	var toolCalls []map[string]any

	if candidates, ok := respMap["candidates"].([]any); ok && len(candidates) > 0 {
		if cand, ok := candidates[0].(map[string]any); ok {
			if content, ok := cand["content"].(map[string]any); ok {
				if parts, ok := content["parts"].([]any); ok {
					for idx, pItem := range parts {
						if pMap, ok := pItem.(map[string]any); ok {
							isThought, _ := pMap["thought"].(bool)
							if text, ok := pMap["text"].(string); ok {
								if isThought {
									reasoningText += text
								} else {
									fullText += text
								}
							}
							if fc, ok := pMap["functionCall"].(map[string]any); ok {
								fcName, _ := fc["name"].(string)
								fcArgs, _ := fc["args"].(map[string]any)
								if fcArgs == nil {
									fcArgs = map[string]any{}
								}
								argsBytes, _ := json.Marshal(fcArgs)
								callID, _ := fc["id"].(string)
								if callID == "" {
									callID = fmt.Sprintf("%s-%d-%d", fcName, time.Now().UnixMilli(), idx)
								}
								if sig, ok := pMap["thoughtSignature"].(string); ok && sig != "" {
									antigravity.StoreThoughtSignature(callID, sig)
								}
								toolCalls = append(toolCalls, map[string]any{
									"id":   callID,
									"type": "function",
									"function": map[string]string{
										"name":      fcName,
										"arguments": string(argsBytes),
									},
								})
							}
						}
					}
				}
			}
		}
	}

	messageObj := map[string]any{
		"role":    "assistant",
		"content": fullText,
	}
	if reasoningText != "" {
		messageObj["reasoning_content"] = reasoningText
	}
	finishReason := "stop"
	if len(toolCalls) > 0 {
		messageObj["tool_calls"] = toolCalls
		finishReason = "tool_calls"
	}

	openAIResp := map[string]any{
		"id":      fmt.Sprintf("chatcmpl-%d", time.Now().UnixNano()),
		"object":  "chat.completion",
		"created": time.Now().Unix(),
		"model":   model,
		"choices": []map[string]any{
			{
				"index":         0,
				"message":       messageObj,
				"finish_reason": finishReason,
			},
		},
	}
	if tk.Src == "upstream" {
		openAIResp["usage"] = usagePayload(tk)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(openAIResp)
}

// assistantTextFromResponse pulls the assistant message content out of an
// OpenAI-shaped completion body, tolerating both the streaming and
// non-streaming renderings.
func assistantTextFromResponse(raw []byte) string {
	var body struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
			Text string `json:"text"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(raw, &body); err != nil || len(body.Choices) == 0 {
		return ""
	}
	if body.Choices[0].Message.Content != "" {
		return body.Choices[0].Message.Content
	}
	return body.Choices[0].Text
}
