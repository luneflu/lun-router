package caveman

// Config is the on-disk state. The rule text is not part of it: built-in modes resolve
// from the constants in defaults.go, so a redeploy changes what gets injected.
type Config struct {
	Enabled      bool   `json:"enabled"`
	Mode         string `json:"mode"` // "caveman", "ultracave", "megacave", "custom"
	CustomPrompt string `json:"custom_prompt,omitempty"`
}

type ModeInfo struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	TokensEst   int    `json:"tokens_est"`
}

type Stats struct {
	TotalRequests     int64 `json:"total_requests"`
	InjectedRequests  int64 `json:"injected_requests"`
	EstimatedSavedPct int   `json:"estimated_saved_pct"`
}

type StatusResponse struct {
	Enabled        bool       `json:"enabled"`
	Mode           string     `json:"mode"`
	ActivePrompt   string     `json:"active_prompt"`
	AvailableModes []ModeInfo `json:"available_modes"`
	CustomPrompt   string     `json:"custom_prompt,omitempty"`
	Stats          Stats      `json:"stats"`
}
