package commandcode

import "time"

// Account is one Command Code CLI API key (user_xxx from ~/.commandcode/auth.json).
type Account struct {
	ID        string    `json:"id"`
	Name      string    `json:"name,omitempty"`
	APIKey    string    `json:"apiKey,omitempty"`
	IsActive  bool      `json:"isActive"`
	CreatedAt time.Time `json:"createdAt"`
}

// CatalogModel is one row of GET /provider/v1/models?view=client.
type CatalogModel struct {
	ID              string   `json:"id"`
	Name            string   `json:"name"`
	ContextLength   int      `json:"context_length"`
	MaxOutput       int      `json:"maxOutputTokens"`
	InputModalities []string `json:"inputModalities"`
	Hidden          bool     `json:"hidden"`
}
