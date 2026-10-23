package opencode

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

const (
	// ConsoleBaseURL is the OpenCode Console origin used by V2 OAuth.
	ConsoleBaseURL = "https://opencode.ai/console"
	// OAuthClientID is the public CLI client used by OpenCode V2.
	OAuthClientID = "opencode-cli"

	deviceCodePath  = "/auth/device/code"
	deviceTokenPath = "/auth/device/token"
	consoleUserPath = "/api/user"
	consoleOrgsPath = "/api/orgs"

	deviceGrantType = "urn:ietf:params:oauth:grant-type:device_code"
)

// DeviceAuthorization is the payload the dashboard needs to complete login.
type DeviceAuthorization struct {
	Server                  string `json:"server"`
	DeviceCode              string `json:"device_code"`
	UserCode                string `json:"user_code"`
	VerificationURI         string `json:"verification_uri,omitempty"`
	VerificationURIComplete string `json:"verification_uri_complete"`
	ExpiresIn               int    `json:"expires_in"`
	Interval                int    `json:"interval"`
}

type deviceTokenResponse struct {
	AccessToken      string `json:"access_token"`
	RefreshToken     string `json:"refresh_token"`
	ExpiresIn        int    `json:"expires_in"`
	OrgID            string `json:"org_id"`
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
}

type consoleUser struct {
	ID    string `json:"id"`
	Email string `json:"email"`
}

type consoleOrg struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func (c *Client) consoleURL() string {
	if c.consoleBaseURL != "" {
		return strings.TrimRight(c.consoleBaseURL, "/")
	}
	return ConsoleBaseURL
}

func (c *Client) oauthUserAgent() string {
	if c.versionManager != nil {
		return c.versionManager.GetUserAgent()
	}
	return UA
}

func (c *Client) doConsoleJSON(ctx context.Context, path string, payload any, accessToken string) (int, []byte, error) {
	var body io.Reader
	if payload != nil {
		raw, err := json.Marshal(payload)
		if err != nil {
			return 0, nil, fmt.Errorf("marshal console request: %w", err)
		}
		body = bytes.NewReader(raw)
	}

	method := http.MethodPost
	if payload == nil {
		method = http.MethodGet
	}
	req, err := http.NewRequestWithContext(ctx, method, c.consoleURL()+path, body)
	if err != nil {
		return 0, nil, fmt.Errorf("create console request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", c.oauthUserAgent())
	if accessToken != "" {
		req.Header.Set("Authorization", "Bearer "+accessToken)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("execute console request: %w", err)
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	return resp.StatusCode, raw, nil
}

// StartOAuth begins the OpenCode Console device-code flow.
func (c *Client) StartOAuth(ctx context.Context) (*DeviceAuthorization, error) {
	status, raw, err := c.doConsoleJSON(ctx, deviceCodePath, map[string]any{
		"client_id":          OAuthClientID,
		"supports_org_scope": true,
	}, "")
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK && status != http.StatusCreated {
		return nil, fmt.Errorf("device code request failed (%d): %s", status, consoleError(raw))
	}

	var res DeviceAuthorization
	if err := json.Unmarshal(raw, &res); err != nil {
		return nil, fmt.Errorf("decode device code response: %w", err)
	}
	if res.DeviceCode == "" || res.UserCode == "" {
		return nil, errors.New("device code response missing device_code/user_code")
	}
	if res.ExpiresIn <= 0 {
		res.ExpiresIn = 600
	}
	if res.Interval <= 0 {
		res.Interval = 5
	}
	res.Server = c.consoleURL()
	res.VerificationURIComplete = c.resolveVerificationURL(res.VerificationURIComplete)
	if res.VerificationURIComplete == "" {
		res.VerificationURIComplete = c.resolveVerificationURL(res.VerificationURI)
	}
	if res.VerificationURIComplete == "" {
		return nil, errors.New("device code response missing verification URL")
	}
	return &res, nil
}

func (c *Client) resolveVerificationURL(raw string) string {
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	if u.IsAbs() {
		if u.Scheme != "http" && u.Scheme != "https" {
			return ""
		}
		return u.String()
	}
	base, err := url.Parse(c.consoleURL() + "/")
	if err != nil {
		return ""
	}
	return base.ResolveReference(u).String()
}

// PollOAuth checks one device-code poll. status is "pending", "approved" or
// "error"; approved returns the saved account.
func (c *Client) PollOAuth(ctx context.Context, deviceCode string) (*Account, string, error) {
	deviceCode = strings.TrimSpace(deviceCode)
	if deviceCode == "" {
		return nil, "error", errors.New("device code is required")
	}

	token, status, err := c.deviceToken(ctx, map[string]any{
		"grant_type":  deviceGrantType,
		"device_code": deviceCode,
		"client_id":   OAuthClientID,
	})
	if err != nil {
		return nil, "error", err
	}

	switch {
	case token.AccessToken != "":
		acc, err := c.finishOAuth(ctx, token)
		if err != nil {
			return nil, "error", err
		}
		return acc, "approved", nil
	case token.Error == "authorization_pending", token.Error == "slow_down":
		return nil, "pending", nil
	case status == http.StatusBadRequest && token.Error == "":
		return nil, "pending", nil
	case token.Error != "":
		return nil, "error", fmt.Errorf("device authorization failed: %s", token.Error)
	default:
		return nil, "pending", nil
	}
}

func (c *Client) deviceToken(ctx context.Context, payload map[string]any) (*deviceTokenResponse, int, error) {
	status, raw, err := c.doConsoleJSON(ctx, deviceTokenPath, payload, "")
	if err != nil {
		return nil, 0, err
	}

	var token deviceTokenResponse
	if err := json.Unmarshal(raw, &token); err != nil {
		return nil, status, fmt.Errorf("decode token response: %w", err)
	}
	return &token, status, nil
}

func (c *Client) finishOAuth(ctx context.Context, token *deviceTokenResponse) (*Account, error) {
	user, err := c.fetchConsoleUser(ctx, token.AccessToken)
	if err != nil {
		return nil, err
	}
	orgs, err := c.fetchConsoleOrgs(ctx, token.AccessToken)
	if err != nil {
		return nil, err
	}
	org := pickOrg(orgs, token.OrgID)
	if token.OrgID != "" && org == nil {
		return nil, fmt.Errorf("OpenCode organization not found: %s", token.OrgID)
	}

	acc := &Account{
		ID:           user.ID,
		Email:        user.Email,
		Server:       c.consoleURL(),
		AccessToken:  token.AccessToken,
		RefreshToken: token.RefreshToken,
		ExpiresAt:    time.Now().Add(time.Duration(token.ExpiresIn) * time.Second),
		IsActive:     true,
	}
	if acc.ID == "" {
		acc.ID = user.Email
	}
	if org != nil {
		acc.OrgID = org.ID
		acc.OrgName = org.Name
	}

	if c.accountStore == nil {
		return nil, errors.New("opencode account store not initialized")
	}
	if err := c.accountStore.Save(acc); err != nil {
		return nil, fmt.Errorf("save account: %w", err)
	}
	return acc, nil
}

func (c *Client) fetchConsoleUser(ctx context.Context, accessToken string) (*consoleUser, error) {
	status, raw, err := c.doConsoleJSON(ctx, consoleUserPath, nil, accessToken)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("user request failed (%d): %s", status, consoleError(raw))
	}
	var user consoleUser
	if err := json.Unmarshal(raw, &user); err != nil {
		return nil, fmt.Errorf("decode user response: %w", err)
	}
	return &user, nil
}

func (c *Client) fetchConsoleOrgs(ctx context.Context, accessToken string) ([]consoleOrg, error) {
	status, raw, err := c.doConsoleJSON(ctx, consoleOrgsPath, nil, accessToken)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("orgs request failed (%d): %s", status, consoleError(raw))
	}
	var orgs []consoleOrg
	if err := json.Unmarshal(raw, &orgs); err != nil {
		return nil, fmt.Errorf("decode orgs response: %w", err)
	}
	return orgs, nil
}

func pickOrg(orgs []consoleOrg, wanted string) *consoleOrg {
	if len(orgs) == 0 {
		return nil
	}
	if wanted != "" {
		for i := range orgs {
			if orgs[i].ID == wanted {
				return &orgs[i]
			}
		}
		return nil
	}
	sorted := append([]consoleOrg{}, orgs...)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].Name != sorted[j].Name {
			return sorted[i].Name < sorted[j].Name
		}
		return sorted[i].ID < sorted[j].ID
	})
	return &sorted[0]
}

func consoleError(raw []byte) string {
	var nested struct {
		Error struct {
			Message string `json:"message"`
			Type    string `json:"type"`
		} `json:"error"`
	}
	if json.Unmarshal(raw, &nested) == nil && nested.Error.Message != "" {
		if nested.Error.Type != "" {
			return nested.Error.Type + ": " + nested.Error.Message
		}
		return nested.Error.Message
	}

	var flat struct {
		Error            string `json:"error"`
		ErrorDescription string `json:"error_description"`
	}
	if json.Unmarshal(raw, &flat) == nil {
		if flat.ErrorDescription != "" {
			return flat.ErrorDescription
		}
		if flat.Error != "" {
			return flat.Error
		}
	}

	text := strings.TrimSpace(string(raw))
	if len(text) > 300 {
		text = text[:300]
	}
	if text == "" {
		text = "empty response body"
	}
	return text
}

// ensureFreshAccount refreshes the OAuth access token shortly before expiry.
func (c *Client) ensureFreshAccount(ctx context.Context, acc *Account) (*Account, error) {
	if acc == nil || c.accountStore == nil {
		return acc, nil
	}
	if acc.ExpiresAt.IsZero() || time.Until(acc.ExpiresAt) > 90*time.Second {
		return acc, nil
	}

	c.refreshMu.Lock()
	defer c.refreshMu.Unlock()

	// Another request may have refreshed while we waited for the lock.
	current, ok := c.accountStore.Get(acc.ID)
	if ok && !current.ExpiresAt.IsZero() && time.Until(current.ExpiresAt) > 90*time.Second {
		return current, nil
	}
	if !ok {
		current = acc
	}
	if current.RefreshToken == "" {
		return nil, errors.New("OpenCode login expired; reconnect the account")
	}

	token, status, err := c.deviceToken(ctx, map[string]any{
		"grant_type":    "refresh_token",
		"refresh_token": current.RefreshToken,
		"client_id":     OAuthClientID,
	})
	if err != nil {
		return nil, err
	}
	if token.AccessToken == "" {
		if token.Error == "invalid_grant" || status == http.StatusBadRequest {
			return nil, errors.New("OpenCode login expired; reconnect the account")
		}
		return nil, fmt.Errorf("refresh OpenCode login failed (%d): %s", status, consoleErrorFromToken(token))
	}

	expires := time.Now().Add(time.Duration(token.ExpiresIn) * time.Second)
	return c.accountStore.UpdateTokens(current.ID, token.AccessToken, token.RefreshToken, expires, token.OrgID, "")
}

func consoleErrorFromToken(token *deviceTokenResponse) string {
	if token.ErrorDescription != "" {
		return token.ErrorDescription
	}
	if token.Error != "" {
		return token.Error
	}
	return "empty response body"
}

// TestAccount refreshes if needed and checks /api/user with the access token.
func (c *Client) TestAccount(ctx context.Context, id string) (int64, error) {
	if c.accountStore == nil {
		return 0, errors.New("opencode account store not initialized")
	}
	acc, ok := c.accountStore.Get(id)
	if !ok {
		return 0, fmt.Errorf("account not found: %s", id)
	}

	start := time.Now()
	fresh, err := c.ensureFreshAccount(ctx, acc)
	if err != nil {
		return time.Since(start).Milliseconds(), err
	}
	if _, err := c.fetchConsoleUser(ctx, fresh.AccessToken); err != nil {
		return time.Since(start).Milliseconds(), err
	}
	return time.Since(start).Milliseconds(), nil
}
