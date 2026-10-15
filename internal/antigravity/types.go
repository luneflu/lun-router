package antigravity

import "time"

type Account struct {
	ID           string    `json:"id"`
	Email        string    `json:"email"`
	ProjectID    string    `json:"projectId"`
	AccessToken  string    `json:"accessToken,omitempty"`
	RefreshToken string    `json:"refreshToken,omitempty"`
	ExpiresAt    time.Time `json:"expiresAt"`
	IsActive     bool      `json:"isActive"`
	CreatedAt    time.Time `json:"createdAt"`
}

type ModelInfo struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Object  string `json:"object"`
	OwnedBy string `json:"owned_by"`
	Enabled bool   `json:"enabled"`
}

type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	TokenType    string `json:"token_type"`
	Scope        string `json:"scope"`
}

type QuotaItem struct {
	Name                string  `json:"name"`
	ModelKey            string  `json:"modelKey,omitempty"`
	Used                int64   `json:"used"`
	Total               int64   `json:"total"`
	RemainingPercentage float64 `json:"remainingPercentage"`
	ResetAt             string  `json:"resetAt,omitempty"`
	Unlimited           bool    `json:"unlimited,omitempty"`
}

type AccountUsage struct {
	AccountID string               `json:"accountId"`
	Email     string               `json:"email"`
	Plan      string               `json:"plan"`
	Message   string               `json:"message,omitempty"`
	Quotas    map[string]QuotaItem `json:"quotas"`
	FetchedAt time.Time            `json:"fetchedAt"`
}

