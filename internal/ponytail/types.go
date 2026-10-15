package ponytail

import "time"

type Config struct {
	Enabled             bool              `json:"enabled"`
	Mode                string            `json:"mode"` // "full", "lite", "ultra", "review", "custom"
	AutoUpdate          bool              `json:"auto_update"`
	CheckIntervalHours  int               `json:"check_interval_hours"`
	LastSync            time.Time         `json:"last_sync"`
	UpstreamCommit      string            `json:"upstream_commit"`
	UpstreamCommitMsg   string            `json:"upstream_commit_msg"`
	UpstreamCommitDate  string            `json:"upstream_commit_date"`
	CustomPrompt        string            `json:"custom_prompt,omitempty"`
	Rules               map[string]string `json:"rules"` // mode -> prompt text
}

type ModeInfo struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

type Stats struct {
	TotalRequests    int64 `json:"total_requests"`
	InjectedRequests int64 `json:"injected_requests"`
}

type StatusResponse struct {
	Enabled            bool       `json:"enabled"`
	Mode               string     `json:"mode"`
	AutoUpdate         bool       `json:"auto_update"`
	LastSync           *time.Time `json:"last_sync"`
	UpstreamCommit     string     `json:"upstream_commit"`
	UpstreamCommitMsg  string     `json:"upstream_commit_msg"`
	UpstreamCommitDate string     `json:"upstream_commit_date"`
	UpstreamURL        string     `json:"upstream_url"`
	ActivePrompt       string     `json:"active_prompt"`
	AvailableModes     []ModeInfo `json:"available_modes"`
	CustomPrompt       string     `json:"custom_prompt,omitempty"`
	Stats              Stats      `json:"stats"`
}
