package kilocode

import "time"

// Account represents a connected Kilo Code account.
type Account struct {
	ID          string    `json:"id"`
	Email       string    `json:"email"`
	OrgID       string    `json:"orgId,omitempty"`
	AccessToken string    `json:"accessToken,omitempty"`
	IsActive    bool      `json:"isActive"`
	CreatedAt   time.Time `json:"createdAt"`
}

// DeviceCodeResponse represents the response when initiating device authorization.
type DeviceCodeResponse struct {
	DeviceCode              string `json:"device_code"`
	UserCode                string `json:"user_code"`
	VerificationURI         string `json:"verification_uri"`
	VerificationURIComplete string `json:"verification_uri_complete"`
	ExpiresIn               int    `json:"expires_in"`
	Interval                int    `json:"interval"`
}

// PollResponse represents poll status for a device code.
type PollResponse struct {
	Status    string   `json:"status"` // "approved", "pending", "access_denied", "expired_token", "poll_failed"
	Token     string   `json:"token,omitempty"`
	UserEmail string   `json:"userEmail,omitempty"`
	OrgID     string   `json:"orgId,omitempty"`
	Account   *Account `json:"account,omitempty"`
	Error     string   `json:"error,omitempty"`
}

// ProfileResponse represents the user profile from https://api.kilo.ai/api/profile.
type ProfileResponse struct {
	Email         string              `json:"email"`
	Organizations []OrganizationEntry `json:"organizations"`
}

// OrganizationEntry represents an organization within the user profile.
type OrganizationEntry struct {
	ID   string `json:"id"`
	Name string `json:"name,omitempty"`
}

// GatewayModelItem represents a model object returned from https://api.kilo.ai/api/gateway/models.
type GatewayModelItem struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Description   string `json:"description,omitempty"`
	ContextLength int    `json:"context_length,omitempty"`
	IsFree        bool   `json:"isFree,omitempty"`
	TopProvider   struct {
		MaxCompletionTokens int `json:"max_completion_tokens,omitempty"`
		ContextLength       int `json:"context_length,omitempty"`
	} `json:"top_provider,omitempty"`
}

// GatewayModelsResponse represents the outer structure of the gateway models response.
type GatewayModelsResponse struct {
	Data []GatewayModelItem `json:"data"`
}
