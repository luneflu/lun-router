package customprovider

import "time"

// Provider represents a user-configured OpenAI-compatible provider.
type Provider struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Prefix    string    `json:"prefix"`
	BaseURL   string    `json:"baseUrl"`
	APIKey    string    `json:"apiKey,omitempty"`
	Models    []string  `json:"models"`
	IsEnabled bool      `json:"isEnabled"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// ProviderSummary is a public representation of Provider with masked APIKey.
type ProviderSummary struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Prefix    string    `json:"prefix"`
	BaseURL   string    `json:"baseUrl"`
	HasAPIKey bool      `json:"hasApiKey"`
	ApiKeyPreview string `json:"apiKeyPreview,omitempty"`
	Models    []string  `json:"models"`
	ModelCount int      `json:"modelCount"`
	IsEnabled bool      `json:"isEnabled"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

func MaskAPIKey(key string) (bool, string) {
	if key == "" {
		return false, ""
	}
	if len(key) <= 8 {
		return true, "••••••••"
	}
	return true, key[:3] + "••••" + key[len(key)-4:]
}

func (p *Provider) ToSummary() ProviderSummary {
	hasKey, preview := MaskAPIKey(p.APIKey)
	models := p.Models
	if models == nil {
		models = []string{}
	}
	return ProviderSummary{
		ID:            p.ID,
		Name:          p.Name,
		Prefix:        p.Prefix,
		BaseURL:       p.BaseURL,
		HasAPIKey:     hasKey,
		ApiKeyPreview: preview,
		Models:        models,
		ModelCount:    len(models),
		IsEnabled:     p.IsEnabled,
		CreatedAt:     p.CreatedAt,
		UpdatedAt:     p.UpdatedAt,
	}
}
