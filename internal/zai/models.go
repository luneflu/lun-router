package zai

// Model is one selectable chat.z.ai model.
type Model struct {
	ID          string
	Name        string
	ContextSize int
	MaxOutput   int
	Vision      bool
	Reasoning   bool
	// Guest reports whether an anonymous (not signed-in) token may use it.
	// chat.z.ai answers 403 "Model not available for current user level" for
	// non-guest models when the token is anonymous.
	Guest bool
}

// Catalog is the set of models this provider knows about. It is deliberately
// limited to the agent-capable ones (returnFc/agent_mode), because the rest
// cannot drive a tool loop.
//
// The Guest flag was verified against a live anonymous token: only x-preview-l
// and glm-4.7 answer. Every other id returns 403 "Model not available for
// current user level", so advertising them would hand clients models that can
// never respond.
var Catalog = []Model{
	{ID: "x-preview-l", Name: "GLM-5.3-Flash", ContextSize: 200000, MaxOutput: 128000, Vision: true, Reasoning: true, Guest: true},
	{ID: "glm-4.7", Name: "GLM-4.7", ContextSize: 200000, MaxOutput: 40000, Reasoning: true, Guest: true},
	{ID: "glm-5.3", Name: "GLM-5.3", ContextSize: 200000, MaxOutput: 128000, Reasoning: true, Guest: false},
	{ID: "glm-5.2", Name: "GLM-5.2", ContextSize: 200000, MaxOutput: 64064, Reasoning: true, Guest: false},
	{ID: "GLM-5-Turbo", Name: "GLM-5-Turbo", ContextSize: 200000, MaxOutput: 32000, Reasoning: true, Guest: false},
	{ID: "GLM-5v-Turbo", Name: "GLM-5V-Turbo", ContextSize: 200000, MaxOutput: 32000, Vision: true, Reasoning: true, Guest: false},
	{ID: "GLM-4.1V-Thinking-FlashX", Name: "GLM-4.1V-9B-Thinking", ContextSize: 128000, MaxOutput: 16000, Vision: true, Reasoning: true, Guest: false},
	{ID: "0727-106B-API", Name: "GLM-4.5-Air", ContextSize: 128000, MaxOutput: 80000, Reasoning: true, Guest: false},
	{ID: "0727-360B-API", Name: "GLM-4.5", ContextSize: 128000, MaxOutput: 80000, Reasoning: true, Guest: false},
	{ID: "0808-360B-DR", Name: "GLM-4.6-DR", ContextSize: 128000, MaxOutput: 32000, Reasoning: true, Guest: false},
}

// DefaultModel is used when a caller asks for "auto" or an unknown id.
const DefaultModel = "x-preview-l"

// ProviderPrefix is the model namespace this provider owns.
const ProviderPrefix = "zai/"

// IsCatalogModel reports whether model belongs to this provider.
func IsCatalogModel(model string) bool {
	clean := CleanModelID(model)
	for _, m := range Catalog {
		if m.ID == clean {
			return true
		}
	}
	return false
}

// GuestUsable reports whether an anonymous token may use this model.
func (m Model) GuestUsable() bool { return m.Guest }

// UsableModels returns the models the current session can actually call. An
// anonymous session is limited to the guest-usable subset; the rest answer 403.
func UsableModels(signedIn bool) []Model {
	if signedIn {
		return Catalog
	}
	out := make([]Model, 0, len(Catalog))
	for _, m := range Catalog {
		if m.Guest {
			out = append(out, m)
		}
	}
	return out
}

// CleanModelID strips the provider prefix.
func CleanModelID(model string) string {
	if len(model) > len(ProviderPrefix) && model[:len(ProviderPrefix)] == ProviderPrefix {
		return model[len(ProviderPrefix):]
	}
	return model
}

// ResolveModel maps a requested id onto a catalog model, falling back to the
// default when the id is unknown or the caller asked for "auto".
func ResolveModel(model string) Model {
	clean := CleanModelID(model)
	if clean == "" || clean == "auto" {
		return LookupModel(DefaultModel)
	}
	return LookupModel(clean)
}

// LookupModel returns the catalog entry for id, or the default model.
func LookupModel(id string) Model {
	for _, m := range Catalog {
		if m.ID == id {
			return m
		}
	}
	return Catalog[0]
}
