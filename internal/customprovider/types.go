package customprovider

import (
	"encoding/json"
	"time"
)

// Model represents model metadata saved from /v1/models (matching Pi model format).
type Model struct {
	ID            string   `json:"id"`
	Name          string   `json:"name,omitempty"`
	ContextWindow int      `json:"contextWindow,omitempty"`
	MaxTokens     int      `json:"maxTokens,omitempty"`
	Input         []string `json:"input,omitempty"`
	Reasoning     *bool    `json:"reasoning,omitempty"`
}

func (m *Model) UnmarshalJSON(data []byte) error {
	var id string
	if err := json.Unmarshal(data, &id); err == nil {
		m.ID = id
		m.Name = id
		m.Input = []string{"text"}
		return nil
	}

	type Alias Model
	var aux Alias
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	*m = Model(aux)
	return nil
}

// ProviderCompat specifies provider-level compatibility overrides.
type ProviderCompat struct {
	Reasoning *bool `json:"reasoning,omitempty"`
}

func (pc *ProviderCompat) UnmarshalJSON(data []byte) error {
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return err
	}
	if r, ok := m["reasoning"]; ok {
		pc.Reasoning = capabilitySupported(r)
	}
	return nil
}

// Provider represents a user-configured OpenAI-compatible provider.
type Provider struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Prefix    string          `json:"prefix"`
	BaseURL   string          `json:"baseUrl"`
	APIKey    string          `json:"apiKey,omitempty"`
	Compat    *ProviderCompat `json:"compat,omitempty"`
	Models    []Model         `json:"models"`
	IsEnabled bool            `json:"isEnabled"`
	CreatedAt time.Time       `json:"createdAt"`
	UpdatedAt time.Time       `json:"updatedAt"`
}

// ProviderSummary is a public representation of Provider with masked APIKey.
type ProviderSummary struct {
	ID            string          `json:"id"`
	Name          string          `json:"name"`
	Prefix        string          `json:"prefix"`
	BaseURL       string          `json:"baseUrl"`
	HasAPIKey     bool            `json:"hasApiKey"`
	ApiKeyPreview string          `json:"apiKeyPreview,omitempty"`
	Compat        *ProviderCompat `json:"compat,omitempty"`
	Models        []string        `json:"models"`
	ModelDetails  []Model         `json:"modelDetails,omitempty"`
	ModelCount    int             `json:"modelCount"`
	IsEnabled     bool            `json:"isEnabled"`
	CreatedAt     time.Time       `json:"createdAt"`
	UpdatedAt     time.Time       `json:"updatedAt"`
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

func (p *Provider) ModelIDs() []string {
	if p.Models == nil {
		return []string{}
	}
	ids := make([]string, len(p.Models))
	for i, m := range p.Models {
		ids[i] = m.ID
	}
	return ids
}

func (p *Provider) ToSummary() ProviderSummary {
	hasKey, preview := MaskAPIKey(p.APIKey)
	models := p.ModelIDs()
	details := p.Models
	if details == nil {
		details = []Model{}
	}
	return ProviderSummary{
		ID:            p.ID,
		Name:          p.Name,
		Prefix:        p.Prefix,
		BaseURL:       p.BaseURL,
		HasAPIKey:     hasKey,
		ApiKeyPreview: preview,
		Compat:        p.Compat,
		Models:        models,
		ModelDetails:  details,
		ModelCount:    len(models),
		IsEnabled:     p.IsEnabled,
		CreatedAt:     p.CreatedAt,
		UpdatedAt:     p.UpdatedAt,
	}
}
