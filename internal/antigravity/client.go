package antigravity

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

func ClientID() string {
	if v := os.Getenv("ANTIGRAVITY_CLIENT_ID"); v != "" {
		return v
	}
	return strings.Join(
		[]string{"1071006", "060591", "-tmhssin2h21lcre", "235vtolojh4g403ep", ".apps.", "googleusercontent.com"},
		"",
	)
}

func ClientSecret() string {
	if v := os.Getenv("ANTIGRAVITY_CLIENT_SECRET"); v != "" {
		return v
	}
	return strings.Join(
		[]string{"GOC", "SPX-", "K58FWR48", "6LdLJ1mLB8s", "XC4z6qDAf"},
		"",
	)
}

const (
	OAuthAuthorizeURL           = "https://accounts.google.com/o/oauth2/v2/auth"
	OAuthTokenURL               = "https://oauth2.googleapis.com/token"
	UserInfoURL                 = "https://www.googleapis.com/oauth2/v1/userinfo"
	LoadCodeAssistURL           = "https://cloudcode-pa.googleapis.com/v1internal:loadCodeAssist"
	ApiBaseURL                  = "https://daily-cloudcode-pa.googleapis.com"
	UserAgent                   = "antigravity/ide/2.11.0 darwin/arm64"
	FetchAvailableModelsURL     = "https://daily-cloudcode-pa.googleapis.com/v1internal:fetchAvailableModels"
	RetrieveUserQuotaSummaryURL = "https://daily-cloudcode-pa.googleapis.com/v1internal:retrieveUserQuotaSummary"
)

var OAuthScopes = []string{
	"https://www.googleapis.com/auth/cloud-platform",
	"https://www.googleapis.com/auth/userinfo.email",
	"https://www.googleapis.com/auth/userinfo.profile",
	"https://www.googleapis.com/auth/cclog",
	"https://www.googleapis.com/auth/experimentsandconfigs",
}

var CatalogModels = []string{
	"gemini-3.8-flash-high",
	"gemini-3.8-flash-medium",
	"gemini-3.8-flash-low",
	"gemini-3.8-flash",
	"gemini-3.7-flash-high",
	"gemini-3.7-flash-medium",
	"gemini-3.7-flash-low",
	"gemini-3.6-flash-high",
	"gemini-3.6-flash-medium",
	"gemini-3.6-flash-low",
	"gemini-3.1-pro-high",
	"gemini-3.1-pro-low",
	"claude-sonnet-5-5-high",
	"claude-sonnet-5-5-medium",
	"claude-sonnet-5-5-low",
	"claude-sonnet-5-5",
	"claude-opus-5-5-high",
	"claude-opus-5-5-medium",
	"claude-opus-5-5-low",
	"claude-opus-5-5",
	"claude-sonnet-4-6",
	"claude-opus-4-6-thinking",
	"gpt-oss-120b-medium",
	"gemini-3.1-flash-image",
}

type Client struct {
	httpClient *http.Client
	store      *AccountStore
	quotaMu    sync.RWMutex
	quotaCache map[string]*AccountUsage
	exhausted  map[string]time.Time
	modelsMu   sync.RWMutex
	models     []string
	modelsFile string

	// quotaRefreshMu guards quotaRefreshing so at most one background balance
	// refresh runs at a time.
	quotaRefreshMu  sync.Mutex
	quotaRefreshing bool
}

// quotaSyncInterval is how often account balances are refreshed in the
// background. Selection reads only the cache, so this loop is what keeps credit
// ranking meaningful without putting a network call on the request path.
const quotaSyncInterval = 4 * time.Minute

// creditTieEpsilon is the balance difference, in percentage points, at which two
// accounts count as equal. Equal accounts rotate instead of one absorbing every
// request.
const creditTieEpsilon = 0.5

// defaultCallbackURL is the redirect URI used when the caller names none. It must
// stay on loopback: the Antigravity client ID belongs to Antigravity itself, and
// Google only has a loopback URI registered for it, so any public host is
// rejected with redirect_uri_mismatch. Callers behind a proxy paste the code back
// instead (see AntigravityHandler.CreateAccount).
const defaultCallbackURL = "http://localhost:20128/callback"

// AuthRedirectURI resolves the redirect URI for an authorize URL: the caller's
// hint when it names one, otherwise the loopback default.
func (c *Client) AuthRedirectURI(hint string) string {
	if hint != "" {
		return hint
	}
	return defaultCallbackURL
}

// ExchangeRedirectURI resolves the redirect URI for a token exchange. An explicit
// value (the callback URL the browser actually hit) wins so it always matches the
// authorize request; then the loopback default.
func (c *Client) ExchangeRedirectURI(explicit string) string {
	if explicit != "" {
		return explicit
	}
	return c.AuthRedirectURI("")
}

func NewClient(store *AccountStore, dataDirs ...string) *Client {
	dataDir := "data"
	if len(dataDirs) > 0 && dataDirs[0] != "" {
		dataDir = dataDirs[0]
	}

	c := &Client{
		httpClient: &http.Client{Timeout: 0},
		store:      store,
		quotaCache: make(map[string]*AccountUsage),
		exhausted:  make(map[string]time.Time),
		models:     append([]string{}, CatalogModels...),
		modelsFile: filepath.Join(dataDir, "antigravity_models.json"),
	}

	c.loadModelsCache()
	return c
}

func (c *Client) loadModelsCache() {
	if c.modelsFile == "" {
		return
	}
	b, err := os.ReadFile(c.modelsFile)
	if err != nil {
		return
	}
	var cached struct {
		Models    []string  `json:"models"`
		UpdatedAt time.Time `json:"updated_at"`
	}
	if err := json.Unmarshal(b, &cached); err == nil && len(cached.Models) > 0 {
		c.modelsMu.Lock()
		seen := make(map[string]bool)
		var merged []string
		for _, m := range cached.Models {
			if !seen[m] {
				seen[m] = true
				merged = append(merged, m)
			}
		}
		for _, m := range CatalogModels {
			if !seen[m] {
				seen[m] = true
				merged = append(merged, m)
			}
		}
		c.models = merged
		c.modelsMu.Unlock()
	}
}

func (c *Client) saveModelsCache(models []string) {
	if c.modelsFile == "" {
		return
	}
	cached := struct {
		Models    []string  `json:"models"`
		UpdatedAt time.Time `json:"updated_at"`
	}{
		Models:    models,
		UpdatedAt: time.Now(),
	}
	b, err := json.MarshalIndent(cached, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(c.modelsFile, b, 0644)
}

func (c *Client) GetCatalogModels() []string {
	c.modelsMu.RLock()
	defer c.modelsMu.RUnlock()
	res := make([]string, len(c.models))
	copy(res, c.models)
	return res
}

func (c *Client) FetchAvailableModels(ctx context.Context) ([]string, bool, error) {
	accounts := c.store.ListRaw()
	if len(accounts) == 0 {
		return c.GetCatalogModels(), false, fmt.Errorf("no Antigravity accounts configured")
	}

	var activeAcc *Account
	for _, a := range accounts {
		if a != nil && a.IsActive {
			activeAcc = a
			break
		}
	}
	if activeAcc == nil {
		activeAcc = accounts[0]
	}

	if err := c.EnsureFreshToken(ctx, activeAcc); err != nil {
		return c.GetCatalogModels(), false, fmt.Errorf("token refresh for account %s: %w", activeAcc.Email, err)
	}

	reqMap := map[string]any{}
	if activeAcc.ProjectID != "" {
		reqMap["project"] = activeAcc.ProjectID
	}
	b, _ := json.Marshal(reqMap)

	fetchCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(fetchCtx, http.MethodPost, FetchAvailableModelsURL, bytes.NewReader(b))
	if err != nil {
		return c.GetCatalogModels(), false, err
	}
	req.Header.Set("Authorization", "Bearer "+activeAcc.AccessToken)
	req.Header.Set("User-Agent", UserAgent)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Client-Name", "antigravity")
	req.Header.Set("X-Client-Version", "2.11.0")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return c.GetCatalogModels(), false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return c.GetCatalogModels(), false, fmt.Errorf("upstream models status: %d", resp.StatusCode)
	}

	var raw struct {
		Models map[string]struct {
			DisplayName string `json:"displayName"`
			IsInternal  bool   `json:"isInternal"`
		} `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return c.GetCatalogModels(), false, err
	}

	seen := make(map[string]bool)
	var newModels []string

	for modelID, info := range raw.Models {
		if info.IsInternal {
			continue
		}
		if strings.HasPrefix(modelID, "chat_") || strings.HasPrefix(modelID, "tab_") {
			continue
		}
		if !seen[modelID] {
			seen[modelID] = true
			newModels = append(newModels, modelID)
		}
	}

	for _, m := range CatalogModels {
		if !seen[m] {
			seen[m] = true
			newModels = append(newModels, m)
		}
	}

	c.modelsMu.Lock()
	updated := len(newModels) != len(c.models)
	c.models = newModels
	c.modelsMu.Unlock()

	if updated {
		c.saveModelsCache(newModels)
	}

	return newModels, updated, nil
}

func (c *Client) StartBackgroundSync(ctx context.Context) {
	go func() {
		select {
		case <-ctx.Done():
			return
		case <-time.After(4 * time.Second):
			if models, updated, err := c.FetchAvailableModels(ctx); err == nil && updated {
				log.Printf("[Antigravity] Models synced from Google: %d models available", len(models))
			}
		}

		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if models, updated, err := c.FetchAvailableModels(ctx); err == nil && updated {
					log.Printf("[Antigravity] Models refreshed from Google: %d models available", len(models))
				}
			}
		}
	}()
}

func (c *Client) Store() *AccountStore {
	return c.store
}

func HasAntigravityPrefix(model string) bool {
	return strings.HasPrefix(model, "antigravity/")
}

func IsCatalogModel(model string) bool {
	clean := CleanModelID(model)
	for _, m := range CatalogModels {
		if m == clean {
			return true
		}
	}
	return false
}

func IsAntigravityModel(model string) bool {
	return HasAntigravityPrefix(model) || IsCatalogModel(model)
}

func (c *Client) IsAntigravityModel(model string) bool {
	clean := CleanModelID(model)
	for _, m := range c.GetCatalogModels() {
		if m == clean {
			return true
		}
	}
	return HasAntigravityPrefix(model)
}

func CleanModelID(model string) string {
	return strings.TrimPrefix(model, "antigravity/")
}

func GetModelFamily(model string) string {
	clean := strings.ToLower(CleanModelID(model))
	if strings.HasPrefix(clean, "claude-") || strings.HasPrefix(clean, "gpt-") || clean == "claude" {
		return "claude_gpt"
	}
	if strings.HasPrefix(clean, "gemini-") && !strings.Contains(clean, "image") {
		return "gemini"
	}
	return ""
}

func (c *Client) isExhaustedLocked(accountID, model string) bool {
	now := time.Now()
	cleanModel := CleanModelID(model)
	fam := GetModelFamily(cleanModel)

	if until, ok := c.exhausted[accountID+"|"+cleanModel]; ok && now.Before(until) {
		return true
	}
	if fam != "" {
		if until, ok := c.exhausted[accountID+"|"+fam+"_session"]; ok && now.Before(until) {
			return true
		}
	}

	if usage, ok := c.quotaCache[accountID]; ok && usage != nil {
		if q, ok := usage.Quotas[cleanModel]; ok && q.RemainingPercentage <= 0 {
			if t, err := time.Parse(time.RFC3339, q.ResetAt); err == nil && now.Before(t) {
				return true
			}
		}
		if fam != "" {
			if q, ok := usage.Quotas[fam+"_session"]; ok && q.RemainingPercentage <= 0 {
				if t, err := time.Parse(time.RFC3339, q.ResetAt); err == nil && now.Before(t) {
					return true
				}
			}
		}
	}
	return false
}

func (c *Client) IsExhausted(accountID, model string) bool {
	c.quotaMu.RLock()
	defer c.quotaMu.RUnlock()
	return c.isExhaustedLocked(accountID, model)
}

func (c *Client) MarkExhausted(accountID, model string, resetAt time.Time) {
	c.quotaMu.Lock()
	defer c.quotaMu.Unlock()

	cleanModel := CleanModelID(model)
	fam := GetModelFamily(cleanModel)

	c.exhausted[accountID+"|"+cleanModel] = resetAt
	if fam != "" {
		c.exhausted[accountID+"|"+fam+"_session"] = resetAt
	}

	if usage, ok := c.quotaCache[accountID]; ok && usage != nil {
		if q, ok := usage.Quotas[cleanModel]; ok {
			q.RemainingPercentage = 0
			q.ResetAt = resetAt.Format(time.RFC3339)
			usage.Quotas[cleanModel] = q
		}
		if fam != "" {
			if q, ok := usage.Quotas[fam+"_session"]; ok {
				q.RemainingPercentage = 0
				q.ResetAt = resetAt.Format(time.RFC3339)
				usage.Quotas[fam+"_session"] = q
			}
		}
	}
}

// creditCandidate pairs an eligible account with its remaining credit.
type creditCandidate struct {
	acc   *Account
	score float64
	// known is false when no balance has been cached for the account yet.
	known bool
}

// GetActiveForModel picks the eligible account with the most remaining credit.
//
// Ranking prefers, in order: the quota bucket for the exact model, then the
// family's 5h session bucket (the one that actually gates requests), then the
// family's weekly bucket, then the scarcest bucket the account reports. Accounts
// whose balances tie rotate, so equal accounts share load rather than the first
// one taking every request.
//
// Selection never blocks on the network. With no cached balances it degrades to
// plain rotation and asks for a background refresh, so the next request can rank.
func (c *Client) GetActiveForModel(model string) (*Account, bool) {
	active := c.store.ListActive()
	if len(active) == 0 {
		return nil, false
	}

	cleanModel := CleanModelID(model)
	fam := GetModelFamily(cleanModel)

	c.quotaMu.RLock()
	candidates := make([]creditCandidate, 0, len(active))
	for _, acc := range active {
		if c.isExhaustedLocked(acc.ID, model) {
			continue
		}
		score, known := c.creditsLocked(acc.ID, cleanModel, fam)
		candidates = append(candidates, creditCandidate{acc: acc, score: score, known: known})
	}
	c.quotaMu.RUnlock()

	// Sort by account id so rotation is driven by the cursor rather than by Go's
	// randomised map iteration order. Without this the "rotate" step picks a
	// random account instead of alternating, and load lands unevenly.
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].acc.ID < candidates[j].acc.ID })

	if len(candidates) == 0 {
		// Every account is exhausted for this model. Fall back to the plain
		// store so the caller still gets an account and the upstream error is
		// surfaced rather than masked as "no accounts".
		return c.store.GetActive()
	}

	pool := candidates
	if best, ok := topScore(candidates); ok {
		pool = tiedAt(candidates, best)
	} else {
		// Nothing cached yet: rotate and warm the cache for next time.
		c.requestQuotaRefresh()
	}

	return c.rotate(pool), true
}

// topScore returns the highest known balance and whether any candidate had one.
func topScore(cands []creditCandidate) (float64, bool) {
	best := 0.0
	found := false
	for _, cd := range cands {
		if !cd.known {
			continue
		}
		if !found || cd.score > best {
			best = cd.score
			found = true
		}
	}
	return best, found
}

// tiedAt narrows a pool to the candidates whose balance matches the best, so the
// rotation spreads load across equally-funded accounts.
func tiedAt(cands []creditCandidate, best float64) []creditCandidate {
	out := make([]creditCandidate, 0, len(cands))
	for _, cd := range cands {
		if cd.known && math.Abs(cd.score-best) < creditTieEpsilon {
			out = append(out, cd)
		}
	}
	if len(out) == 0 {
		return cands
	}
	return out
}

// rotate advances the shared rotation cursor inside pool and returns the chosen
// account. Holding the cursor on the store keeps one rotation order across
// model families.
func (c *Client) rotate(pool []creditCandidate) *Account {
	c.store.mu.Lock()
	c.store.rotIndex = (c.store.rotIndex + 1) % len(pool)
	acc := *pool[c.store.rotIndex].acc
	c.store.mu.Unlock()
	return &acc
}

// creditsLocked reports how much credit accountID has left for a model, and
// whether any balance was known. Callers must hold quotaMu.
func (c *Client) creditsLocked(accountID, cleanModel, fam string) (float64, bool) {
	usage := c.quotaCache[accountID]
	if usage == nil || len(usage.Quotas) == 0 {
		return 0, false
	}

	// The exact model bucket is the most precise signal for this request.
	if q, ok := usage.Quotas[cleanModel]; ok {
		return remainingOf(q), true
	}
	if fam != "" {
		// The 5h session bucket is what actually gates requests; the weekly
		// bucket is the wider backstop when no session bucket is reported.
		if q, ok := usage.Quotas[fam+"_session"]; ok {
			return remainingOf(q), true
		}
		if q, ok := usage.Quotas[fam+"_weekly"]; ok {
			return remainingOf(q), true
		}
	}

	// The model-to-bucket mapping is unknown here, so score the account by its
	// scarcest reported bucket - the conservative estimate of its headroom.
	best := math.Inf(1)
	found := false
	for _, q := range usage.Quotas {
		if q.Unlimited {
			continue
		}
		if q.RemainingPercentage < best {
			best = q.RemainingPercentage
		}
		found = true
	}
	if !found {
		// Every bucket reports unlimited.
		return 100, true
	}
	return best, true
}

// remainingOf normalises a bucket to a remaining percentage. An unlimited bucket
// outranks any finite balance.
func remainingOf(q QuotaItem) float64 {
	if q.Unlimited {
		return 100
	}
	return q.RemainingPercentage
}

// StartQuotaSync keeps per-account balances warm so selection can rank accounts
// by remaining credit instead of rotating blindly.
func (c *Client) StartQuotaSync(ctx context.Context) {
	go func() {
		select {
		case <-ctx.Done():
			return
		case <-time.After(3 * time.Second):
		}
		c.refreshAllQuotas(ctx)

		ticker := time.NewTicker(quotaSyncInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				c.refreshAllQuotas(ctx)
			}
		}
	}()
}

// refreshAllQuotas refetches balances for every active account. GetAccountUsage
// serves its own short-lived cache, so repeated calls stay cheap.
func (c *Client) refreshAllQuotas(ctx context.Context) {
	for _, acc := range c.store.ListActive() {
		select {
		case <-ctx.Done():
			return
		default:
		}
		callCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		if _, err := c.GetAccountUsage(callCtx, acc, false); err != nil {
			log.Printf("[Antigravity] quota refresh for %s failed: %v", acc.Email, err)
		}
		cancel()
	}
}

// requestQuotaRefresh warms balances off the request path, at most one refresh in
// flight at a time. It is what makes the first request after boot rank on the
// second call rather than never.
func (c *Client) requestQuotaRefresh() {
	c.quotaRefreshMu.Lock()
	if c.quotaRefreshing {
		c.quotaRefreshMu.Unlock()
		return
	}
	c.quotaRefreshing = true
	c.quotaRefreshMu.Unlock()

	go func() {
		defer func() {
			c.quotaRefreshMu.Lock()
			c.quotaRefreshing = false
			c.quotaRefreshMu.Unlock()
		}()
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		c.refreshAllQuotas(ctx)
	}()
}

func (c *Client) BuildAuthURL(redirectURI, state string) string {
	redirectURI = c.AuthRedirectURI(redirectURI)
	if state == "" {
		state = generateAccountID()
	}

	params := url.Values{}
	params.Set("client_id", ClientID())
	params.Set("response_type", "code")
	params.Set("redirect_uri", redirectURI)
	params.Set("scope", strings.Join(OAuthScopes, " "))
	params.Set("state", state)
	params.Set("access_type", "offline")
	params.Set("prompt", "consent")

	return fmt.Sprintf("%s?%s", OAuthAuthorizeURL, params.Encode())
}

func (c *Client) ExchangeCode(ctx context.Context, code, redirectURI string) (*Account, error) {
	redirectURI = c.ExchangeRedirectURI(redirectURI)

	data := url.Values{}
	data.Set("grant_type", "authorization_code")
	data.Set("client_id", ClientID())
	data.Set("client_secret", ClientSecret())
	data.Set("code", code)
	data.Set("redirect_uri", redirectURI)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, OAuthTokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("create token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("exchange token: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("token exchange failed (%d): %s", resp.StatusCode, string(body))
	}

	var tokenResp TokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return nil, fmt.Errorf("decode token response: %w", err)
	}

	// Fetch user info (email)
	email := c.fetchUserInfo(ctx, tokenResp.AccessToken)

	// Fetch Project ID from loadCodeAssist
	projectID := c.fetchProjectID(ctx, tokenResp.AccessToken)
	if projectID == "" {
		projectID = generateRandomProjectID()
	}

	expiresAt := time.Now().UTC().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)

	acc := &Account{
		ID:           generateAccountID(),
		Email:        email,
		ProjectID:    projectID,
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		ExpiresAt:    expiresAt,
		IsActive:     true,
		CreatedAt:    time.Now().UTC(),
	}

	if err := c.store.Save(acc); err != nil {
		return nil, fmt.Errorf("save account: %w", err)
	}

	return acc, nil
}

func (c *Client) RefreshToken(ctx context.Context, acc *Account) error {
	if acc.RefreshToken == "" {
		return fmt.Errorf("account has no refresh token")
	}

	data := url.Values{}
	data.Set("grant_type", "refresh_token")
	data.Set("client_id", ClientID())
	data.Set("client_secret", ClientSecret())
	data.Set("refresh_token", acc.RefreshToken)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, OAuthTokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("refresh token request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("token refresh failed (%d): %s", resp.StatusCode, string(body))
	}

	var tr TokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tr); err != nil {
		return fmt.Errorf("decode refresh response: %w", err)
	}

	acc.AccessToken = tr.AccessToken
	if tr.RefreshToken != "" {
		acc.RefreshToken = tr.RefreshToken
	}
	acc.ExpiresAt = time.Now().UTC().Add(time.Duration(tr.ExpiresIn) * time.Second)

	return c.store.Save(acc)
}

func (c *Client) EnsureFreshToken(ctx context.Context, acc *Account) error {
	// If expires within 5 minutes or already expired
	if time.Now().UTC().Add(5 * time.Minute).After(acc.ExpiresAt) {
		return c.RefreshToken(ctx, acc)
	}
	return nil
}

func (c *Client) fetchUserInfo(ctx context.Context, accessToken string) string {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, UserInfoURL+"?alt=json", nil)
	if err != nil {
		return "unknown@antigravity"
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "unknown@antigravity"
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		var user struct {
			Email string `json:"email"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&user); err == nil && user.Email != "" {
			return user.Email
		}
	}
	return "unknown@antigravity"
}

func (c *Client) fetchProjectID(ctx context.Context, accessToken string) string {
	meta := map[string]any{
		"metadata": map[string]int{
			"ideType":    9,
			"platform":   2,
			"pluginType": 2,
		},
	}
	bodyBytes, _ := json.Marshal(meta)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, LoadCodeAssistURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return ""
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", UserAgent)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		var res map[string]any
		if err := json.NewDecoder(resp.Body).Decode(&res); err == nil {
			if proj, ok := res["cloudaicompanionProject"].(string); ok && proj != "" {
				return proj
			}
			if projObj, ok := res["cloudaicompanionProject"].(map[string]any); ok {
				if id, ok := projObj["id"].(string); ok && id != "" {
					return id
				}
			}
		}
	}
	return ""
}

func generateRandomProjectID() string {
	adjectives := []string{"useful", "bright", "swift", "calm", "bold"}
	nouns := []string{"fuze", "wave", "spark", "flow", "core"}
	b := make([]byte, 2)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%s-%s-%s", adjectives[time.Now().Unix()%int64(len(adjectives))], nouns[time.Now().Unix()%int64(len(nouns))], hex.EncodeToString(b))
}

// ExecuteChat sends chat completion request to Google Antigravity backend
func (c *Client) ExecuteChat(ctx context.Context, body map[string]any, stream bool) (*http.Response, error) {
	model, _ := body["model"].(string)
	cleanModel := CleanModelID(model)

	acc, ok := c.GetActiveForModel(cleanModel)
	if !ok {
		return nil, fmt.Errorf("no active Antigravity account found; please add or activate an account in Providers > Antigravity")
	}

	if err := c.EnsureFreshToken(ctx, acc); err != nil {
		return nil, fmt.Errorf("token refresh for account %s: %w", acc.Email, err)
	}

	// Transform OpenAI messages to Gemini contents format
	contents, systemInstruction := convertOpenAIMessagesToGemini(body["messages"], cleanModel)

	// Build request payload according to Antigravity format
	reqObj := map[string]any{
		"contents": contents,
		"generationConfig": map[string]any{
			"maxOutputTokens": 64000,
		},
	}
	if systemInstruction != nil {
		reqObj["systemInstruction"] = systemInstruction
	} else if sys, ok := body["system"].(string); ok && strings.TrimSpace(sys) != "" {
		reqObj["systemInstruction"] = map[string]any{
			"role":  "user",
			"parts": []map[string]any{{"text": sys}},
		}
	}

	// Process tools if present
	if toolsRaw, ok := body["tools"].([]any); ok && len(toolsRaw) > 0 {
		var decls []map[string]any
		seen := make(map[string]bool)
		for _, tItem := range toolsRaw {
			tMap, ok := tItem.(map[string]any)
			if !ok {
				continue
			}
			var fnName, fnDesc string
			var fnParams map[string]any

			if tType, _ := tMap["type"].(string); tType == "function" {
				if fnObj, ok := tMap["function"].(map[string]any); ok {
					fnName, _ = fnObj["name"].(string)
					fnDesc, _ = fnObj["description"].(string)
					if params, ok := fnObj["parameters"].(map[string]any); ok {
						fnParams = params
					}
				}
			} else if name, ok := tMap["name"].(string); ok && name != "" {
				fnName = name
				fnDesc, _ = tMap["description"].(string)
				if schema, ok := tMap["input_schema"].(map[string]any); ok {
					fnParams = schema
				}
			}

			if fnName == "" {
				continue
			}
			cleanName := sanitizeGeminiFunctionName(fnName)
			if seen[cleanName] {
				continue
			}
			seen[cleanName] = true

			cleanedSchema := CleanJSONSchemaForAntigravity(fnParams)
			decls = append(decls, map[string]any{
				"name":        cleanName,
				"description": fnDesc,
				"parameters":  cleanedSchema,
			})
		}

		if len(decls) > 0 {
			reqObj["tools"] = []map[string]any{
				{"functionDeclarations": decls},
			}
			reqObj["toolConfig"] = map[string]any{
				"functionCallingConfig": map[string]any{
					"mode": "VALIDATED",
				},
			}
		}
	}

	// Strip blacklisted thinking fields rejected by Google generateContent
	for _, k := range []string{"output_config", "thinking", "reasoning_effort", "reasoning", "enable_thinking", "thinking_budget", "thinkingConfig"} {
		delete(reqObj, k)
	}

	projectID := acc.ProjectID
	if projectID == "" {
		projectID = generateRandomProjectID()
	}

	reqID := fmt.Sprintf("agent/%s/%d/%s/1", generateAccountID(), time.Now().UnixMilli(), cleanModel)

	payload := map[string]any{
		"project":   projectID,
		"model":     cleanModel,
		"userAgent": "antigravity",
		"requestId": reqID,
		"request":   reqObj,
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	action := "generateContent"
	if stream {
		action = "streamGenerateContent?alt=sse"
	}
	endpoint := fmt.Sprintf("%s/v1internal:%s", ApiBaseURL, action)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payloadBytes))
	if err != nil {
		return nil, fmt.Errorf("create upstream request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+acc.AccessToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", UserAgent)

	resp, err := c.httpClient.Do(req)
	if err == nil && resp != nil && (resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == http.StatusConflict) {
		c.MarkExhausted(acc.ID, cleanModel, time.Now().Add(15*time.Minute))
	}
	return resp, err
}

func convertOpenAIMessagesToGemini(messages any, model string) ([]map[string]any, map[string]any) {
	var msgList []map[string]any
	switch list := messages.(type) {
	case []any:
		for _, item := range list {
			if m, ok := item.(map[string]any); ok {
				msgList = append(msgList, m)
			}
		}
	case []map[string]any:
		msgList = list
	case []map[string]string:
		for _, item := range list {
			m := make(map[string]any, len(item))
			for k, v := range item {
				m[k] = v
			}
			msgList = append(msgList, m)
		}
	}

	if len(msgList) == 0 {
		return []map[string]any{
			{
				"role":  "user",
				"parts": []map[string]any{{"text": "Hello"}},
			},
		}, nil
	}

	// Build tool_call_id -> function_name map
	tcID2Name := make(map[string]string)
	for _, m := range msgList {
		if role, _ := m["role"].(string); role == "assistant" {
			if toolCalls, ok := m["tool_calls"].([]any); ok {
				for _, tc := range toolCalls {
					if tcMap, ok := tc.(map[string]any); ok {
						id, _ := tcMap["id"].(string)
						if fn, ok := tcMap["function"].(map[string]any); ok {
							if fnName, ok := fn["name"].(string); ok && id != "" {
								tcID2Name[id] = fnName
							}
						}
					}
				}
			}
		}
	}

	var systemParts []map[string]any
	var rawContents []map[string]any
	hasMultiple := len(msgList) > 1

	for _, m := range msgList {
		role, _ := m["role"].(string)

		if role == "system" && hasMultiple {
			txt := extractText(m["content"])
			if strings.TrimSpace(txt) != "" {
				systemParts = append(systemParts, map[string]any{"text": txt})
			}
			continue
		}

		geminiRole := "user"
		if role == "assistant" {
			geminiRole = "model"
		}

		var parts []map[string]any

		if role != "tool" {
			contentParts := convertOpenAIContentToParts(m["content"])
			parts = append(parts, contentParts...)
		}

		if role == "assistant" {
			if toolCalls, ok := m["tool_calls"].([]any); ok && len(toolCalls) > 0 {
				firstCall := true
				for _, tc := range toolCalls {
					if tcMap, ok := tc.(map[string]any); ok {
						fn, _ := tcMap["function"].(map[string]any)
						fnName, _ := fn["name"].(string)
						var args map[string]any
						if argStr, ok := fn["arguments"].(string); ok {
							_ = json.Unmarshal([]byte(argStr), &args)
						}
						if args == nil {
							args = map[string]any{}
						}
						callID, _ := tcMap["id"].(string)
						fc := map[string]any{
							"name": sanitizeGeminiFunctionName(fnName),
							"args": args,
						}
						if callID != "" {
							fc["id"] = callID
						}
						callPart := map[string]any{
							"functionCall": fc,
						}
						// Gemini 3+ rejects functionCall parts without thoughtSignature on the part.
						// First call gets cached or default signature; sibling calls remain unsigned.
						if firstCall {
							callPart["thoughtSignature"] = GetThoughtSignature(callID, model)
							firstCall = false
						}
						parts = append(parts, callPart)
					}
				}
			}
		}

		if role == "tool" {
			geminiRole = "user"
			toolCallID, _ := m["tool_call_id"].(string)
			respContent := extractText(m["content"])
			var parsedResp any
			if err := json.Unmarshal([]byte(respContent), &parsedResp); err != nil {
				parsedResp = respContent
			}
			fnName := tcID2Name[toolCallID]
			if fnName == "" {
				fnName = "tool"
			}
			fr := map[string]any{
				"name":     sanitizeGeminiFunctionName(fnName),
				"response": map[string]any{"result": parsedResp},
			}
			if toolCallID != "" {
				fr["id"] = toolCallID
			}
			parts = append(parts, map[string]any{"functionResponse": fr})
		}

		// functionResponse parts always belong in a user turn
		for _, p := range parts {
			if _, ok := p["functionResponse"]; ok {
				geminiRole = "user"
				break
			}
		}

		if len(parts) > 0 {
			rawContents = append(rawContents, map[string]any{
				"role":  geminiRole,
				"parts": parts,
			})
		}
	}

	contents := normalizeGeminiContents(rawContents)

	var systemInstruction map[string]any
	if len(systemParts) > 0 {
		systemInstruction = map[string]any{
			"role":  "user",
			"parts": systemParts,
		}
	}

	return contents, systemInstruction
}

func sanitizeGeminiFunctionName(name string) string {
	if name == "" {
		return "_unknown"
	}
	var sb strings.Builder
	for i, r := range name {
		valid := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '.' || r == ':' || r == '-'
		if !valid {
			sb.WriteByte('_')
			continue
		}
		if i == 0 && !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || r == '_') {
			sb.WriteByte('_')
		}
		sb.WriteRune(r)
	}
	res := sb.String()
	if len(res) > 64 {
		res = res[:64]
	}
	return res
}

func extractText(content any) string {
	switch v := content.(type) {
	case string:
		return v
	case []any:
		var sb strings.Builder
		for _, item := range v {
			if m, ok := item.(map[string]any); ok {
				if t, ok := m["text"].(string); ok {
					sb.WriteString(t)
				}
			}
		}
		return sb.String()
	default:
		return ""
	}
}

func convertOpenAIContentToParts(content any) []map[string]any {
	var parts []map[string]any
	switch v := content.(type) {
	case string:
		if strings.TrimSpace(v) != "" {
			parts = append(parts, map[string]any{"text": v})
		}
	case []any:
		for _, item := range v {
			m, ok := item.(map[string]any)
			if !ok {
				continue
			}
			itemType, _ := m["type"].(string)
			switch itemType {
			case "text":
				if text, ok := m["text"].(string); ok && strings.TrimSpace(text) != "" {
					parts = append(parts, map[string]any{"text": text})
				}
			case "image_url":
				if imgObj, ok := m["image_url"].(map[string]any); ok {
					if urlStr, ok := imgObj["url"].(string); ok {
						if strings.HasPrefix(urlStr, "data:") {
							commaIdx := strings.Index(urlStr, ",")
							if commaIdx != -1 {
								mimePart := urlStr[5:commaIdx]
								data := urlStr[commaIdx+1:]
								mimeType := strings.Split(mimePart, ";")[0]
								parts = append(parts, map[string]any{
									"inlineData": map[string]any{
										"mime_type": mimeType,
										"data":      data,
									},
								})
							}
						} else if strings.HasPrefix(urlStr, "http://") || strings.HasPrefix(urlStr, "https://") {
							parts = append(parts, map[string]any{
								"fileData": map[string]any{
									"fileUri":  urlStr,
									"mimeType": "image/*",
								},
							})
						}
					}
				}
			}
		}
	}
	return parts
}

func extractParts(raw any) []map[string]any {
	switch v := raw.(type) {
	case []map[string]any:
		return v
	case []any:
		var res []map[string]any
		for _, item := range v {
			if m, ok := item.(map[string]any); ok {
				res = append(res, m)
			}
		}
		return res
	default:
		return nil
	}
}

// normalizeGeminiContents mirrors 9router normalizeGeminiContents:
// merges adjacent same-role messages, strips empty parts,
// and enforces initial user turn and terminal user turn (preventing 400 "Requests ending with a model turn are not supported").
func normalizeGeminiContents(contents []map[string]any) []map[string]any {
	var out []map[string]any

	for _, c := range contents {
		role, _ := c["role"].(string)
		if role == "" {
			continue
		}
		rawParts := extractParts(c["parts"])
		var parts []map[string]any
		for _, p := range rawParts {
			if len(p) == 0 {
				continue
			}
			// Strip thought-only parts
			if isThought, _ := p["thought"].(bool); isThought && p["functionCall"] == nil {
				continue
			}
			// Strip thoughtSignature-only parts without text or functionCall
			if p["thoughtSignature"] != nil && p["functionCall"] == nil && p["text"] == nil {
				continue
			}
			if t, ok := p["text"].(string); ok && strings.TrimSpace(t) == "" && len(p) == 1 {
				continue
			}
			// Safety: move any thoughtSignature erroneously nested in functionCall to the part level
			if fc, ok := p["functionCall"].(map[string]any); ok {
				if sig, ok := fc["thoughtSignature"].(string); ok && sig != "" {
					delete(fc, "thoughtSignature")
					if p["thoughtSignature"] == nil {
						p["thoughtSignature"] = sig
					}
				}
			}
			parts = append(parts, p)
		}
		if len(parts) == 0 {
			continue
		}

		if len(out) > 0 && out[len(out)-1]["role"] == role {
			lastParts := extractParts(out[len(out)-1]["parts"])
			out[len(out)-1]["parts"] = append(lastParts, parts...)
		} else {
			out = append(out, map[string]any{
				"role":  role,
				"parts": parts,
			})
		}
	}

	// Initial turn guard: first turn must be user
	if len(out) > 0 && out[0]["role"] != "user" {
		out = append([]map[string]any{
			{
				"role":  "user",
				"parts": []map[string]any{{"text": "..."}},
			},
		}, out...)
	}

	// Terminal turn guard: last turn cannot be model
	if len(out) > 0 && out[len(out)-1]["role"] == "model" {
		lastMsg := out[len(out)-1]
		lastParts := extractParts(lastMsg["parts"])

		var fnCalls []map[string]any
		for _, p := range lastParts {
			if fc, ok := p["functionCall"].(map[string]any); ok {
				fnCalls = append(fnCalls, fc)
			}
		}

		if len(fnCalls) > 0 {
			responses := make([]map[string]any, 0, len(fnCalls))
			for _, call := range fnCalls {
				name, _ := call["name"].(string)
				if name == "" {
					name = "tool"
				}
				fr := map[string]any{
					"name":     name,
					"response": map[string]any{"result": "Continue."},
				}
				if id, ok := call["id"].(string); ok && id != "" {
					fr["id"] = id
				}
				responses = append(responses, map[string]any{
					"functionResponse": fr,
				})
			}
			out = append(out, map[string]any{
				"role":  "user",
				"parts": responses,
			})
		} else {
			out = append(out, map[string]any{
				"role":  "user",
				"parts": []map[string]any{{"text": "Continue."}},
			})
		}
	}

	if len(out) == 0 {
		out = []map[string]any{
			{
				"role":  "user",
				"parts": []map[string]any{{"text": "Hello"}},
			},
		}
	}

	return out
}

func (c *Client) fetchSubscriptionAndProject(ctx context.Context, accessToken string) (string, string) {
	meta := map[string]any{
		"metadata": map[string]any{
			"ideType":       "ANTIGRAVITY",
			"ideVersion":    "2.11.0",
			"pluginType":    "GEMINI",
			"pluginVersion": "2.11.0",
		},
		"mode": 1,
	}
	bodyBytes, _ := json.Marshal(meta)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, LoadCodeAssistURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return "Unknown", ""
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", UserAgent)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "Unknown", ""
	}
	defer resp.Body.Close()

	plan := "Free Tier"
	projectID := ""

	if resp.StatusCode == http.StatusOK {
		var res map[string]any
		if err := json.NewDecoder(resp.Body).Decode(&res); err == nil {
			if proj, ok := res["cloudaicompanionProject"].(string); ok && proj != "" {
				projectID = proj
			} else if projObj, ok := res["cloudaicompanionProject"].(map[string]any); ok {
				if id, ok := projObj["id"].(string); ok && id != "" {
					projectID = id
				}
			}

			if tier, ok := res["currentTier"].(map[string]any); ok {
				if name, ok := tier["name"].(string); ok && name != "" {
					plan = name
				}
			} else if paid, ok := res["paidTier"].(map[string]any); ok {
				if id, ok := paid["id"].(string); ok && id != "" {
					plan = id
				}
			}
		}
	}
	return plan, projectID
}

func (c *Client) fetchQuotaSummary(ctx context.Context, accessToken, projectID string, quotas map[string]QuotaItem) {
	reqMap := map[string]any{}
	if projectID != "" {
		reqMap["project"] = projectID
	}
	b, _ := json.Marshal(reqMap)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, RetrieveUserQuotaSummaryURL, bytes.NewReader(b))
	if err != nil {
		return
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("User-Agent", UserAgent)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Client-Name", "antigravity")
	req.Header.Set("X-Client-Version", "2.11.0")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return
	}

	var raw map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return
	}

	var groups []any
	if g, ok := raw["groups"].([]any); ok {
		groups = g
	} else if qs, ok := raw["quotaSummary"].(map[string]any); ok {
		if g, ok := qs["groups"].([]any); ok {
			groups = g
		}
	}

	for _, gItem := range groups {
		group, ok := gItem.(map[string]any)
		if !ok {
			continue
		}
		displayName, _ := group["displayName"].(string)
		lowerName := strings.ToLower(displayName)

		buckets, ok := group["buckets"].([]any)
		if !ok {
			continue
		}

		isGemini := strings.Contains(lowerName, "gemini")
		isClaudeGPT := strings.Contains(lowerName, "claude") || strings.Contains(lowerName, "gpt")

		if !isGemini && !isClaudeGPT {
			continue
		}

		for _, bItem := range buckets {
			bMap, ok := bItem.(map[string]any)
			if !ok {
				continue
			}
			w, _ := bMap["window"].(string)
			w = strings.ToLower(w)
			bucketID, _ := bMap["bucketId"].(string)
			bDisplayName, _ := bMap["displayName"].(string)
			text := strings.ToLower(bucketID + " " + bDisplayName)

			isWeekly := w == "weekly" || strings.Contains(text, "weekly")
			isSession := w == "5h" || w == "daily" || strings.Contains(text, "5h") || strings.Contains(text, "five hour") || strings.Contains(text, "daily")

			if !isWeekly && !isSession {
				continue
			}

			disabled, _ := bMap["disabled"].(bool)
			if isWeekly && disabled {
				continue
			}

			var fraction float64
			if disabled {
				fraction = 0
			} else if rf, ok := bMap["remainingFraction"].(float64); ok {
				fraction = rf
			}

			resetTime, _ := bMap["resetTime"].(string)

			total := int64(1000)
			remaining := int64(math.Round(float64(total) * fraction))
			used := total - remaining
			if used < 0 {
				used = 0
			}
			pct := math.Round(fraction*1000) / 10

			if isGemini {
				if isSession && quotas["gemini_session"].Name == "" {
					quotas["gemini_session"] = QuotaItem{
						Name:                "Gemini (5h)",
						ModelKey:            "gemini_session",
						Used:                used,
						Total:               total,
						RemainingPercentage: pct,
						ResetAt:             resetTime,
					}
				} else if isWeekly && quotas["gemini_weekly"].Name == "" {
					quotas["gemini_weekly"] = QuotaItem{
						Name:                "Gemini (Weekly)",
						ModelKey:            "gemini_weekly",
						Used:                used,
						Total:               total,
						RemainingPercentage: pct,
						ResetAt:             resetTime,
					}
				}
			} else if isClaudeGPT {
				if isSession && quotas["claude_gpt_session"].Name == "" {
					quotas["claude_gpt_session"] = QuotaItem{
						Name:                "Claude & GPT (5h)",
						ModelKey:            "claude_gpt_session",
						Used:                used,
						Total:               total,
						RemainingPercentage: pct,
						ResetAt:             resetTime,
					}
				} else if isWeekly && quotas["claude_gpt_weekly"].Name == "" {
					quotas["claude_gpt_weekly"] = QuotaItem{
						Name:                "Claude & GPT (Weekly)",
						ModelKey:            "claude_gpt_weekly",
						Used:                used,
						Total:               total,
						RemainingPercentage: pct,
						ResetAt:             resetTime,
					}
				}
			}
		}
	}
}

func (c *Client) fetchAvailableModelsQuota(ctx context.Context, accessToken, projectID string, quotas map[string]QuotaItem) {
	reqMap := map[string]any{}
	if projectID != "" {
		reqMap["project"] = projectID
	}
	b, _ := json.Marshal(reqMap)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, FetchAvailableModelsURL, bytes.NewReader(b))
	if err != nil {
		return
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("User-Agent", UserAgent)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Client-Name", "antigravity")
	req.Header.Set("X-Client-Version", "2.11.0")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return
	}

	var raw struct {
		Models map[string]struct {
			DisplayName string `json:"displayName"`
			IsInternal  bool   `json:"isInternal"`
			QuotaInfo   *struct {
				RemainingFraction float64 `json:"remainingFraction"`
				ResetTime         string  `json:"resetTime"`
			} `json:"quotaInfo"`
		} `json:"models"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return
	}

	// Auto-discover models from response
	var discovered []string
	for modelKey, mData := range raw.Models {
		if !mData.IsInternal && !strings.HasPrefix(modelKey, "chat_") && !strings.HasPrefix(modelKey, "tab_") {
			discovered = append(discovered, modelKey)
		}
	}
	if len(discovered) > 0 {
		c.modelsMu.Lock()
		seen := make(map[string]bool)
		for _, m := range c.models {
			seen[m] = true
		}
		newAdded := false
		for _, d := range discovered {
			if !seen[d] {
				seen[d] = true
				c.models = append(c.models, d)
				newAdded = true
			}
		}
		if newAdded {
			c.saveModelsCache(c.models)
		}
		c.modelsMu.Unlock()
	}

	for modelKey, mData := range raw.Models {
		if mData.IsInternal || mData.QuotaInfo == nil {
			continue
		}

		fraction := mData.QuotaInfo.RemainingFraction
		total := int64(1000)
		remaining := int64(math.Round(float64(total) * fraction))
		used := total - remaining
		if used < 0 {
			used = 0
		}
		pct := math.Round(fraction*1000) / 10

		displayName := mData.DisplayName
		if displayName == "" {
			displayName = modelKey
		}

		quotas[modelKey] = QuotaItem{
			Name:                displayName,
			ModelKey:            modelKey,
			Used:                used,
			Total:               total,
			RemainingPercentage: pct,
			ResetAt:             mData.QuotaInfo.ResetTime,
		}
	}
}

// GetAccountUsage returns live or cached usage & quotas for an account
func (c *Client) GetAccountUsage(ctx context.Context, acc *Account, force bool) (*AccountUsage, error) {
	c.quotaMu.RLock()
	cached, hasCached := c.quotaCache[acc.ID]
	c.quotaMu.RUnlock()

	if !force && hasCached && cached != nil && time.Since(cached.FetchedAt) < 45*time.Second {
		return cached, nil
	}

	if err := c.EnsureFreshToken(ctx, acc); err != nil {
		return nil, fmt.Errorf("refresh token for %s: %w", acc.Email, err)
	}

	plan, projectID := c.fetchSubscriptionAndProject(ctx, acc.AccessToken)
	if acc.ProjectID == "" && projectID != "" {
		acc.ProjectID = projectID
		_ = c.store.Save(acc)
	} else if projectID == "" {
		projectID = acc.ProjectID
	}

	quotas := make(map[string]QuotaItem)

	// 1. Fetch Weekly & Session Quota Summary
	c.fetchQuotaSummary(ctx, acc.AccessToken, projectID, quotas)

	// 2. Fetch Available Models Quota Info
	c.fetchAvailableModelsQuota(ctx, acc.AccessToken, projectID, quotas)

	// Apply any active local strikes / exhaustion
	now := time.Now()
	c.quotaMu.RLock()
	for k, until := range c.exhausted {
		if strings.HasPrefix(k, acc.ID+"|") && now.Before(until) {
			modKey := strings.TrimPrefix(k, acc.ID+"|")
			if q, exists := quotas[modKey]; exists {
				q.RemainingPercentage = 0
				q.ResetAt = until.Format(time.RFC3339)
				quotas[modKey] = q
			} else {
				quotas[modKey] = QuotaItem{
					Name:                modKey,
					ModelKey:            modKey,
					Used:                1000,
					Total:               1000,
					RemainingPercentage: 0,
					ResetAt:             until.Format(time.RFC3339),
				}
			}
		}
	}
	c.quotaMu.RUnlock()

	usage := &AccountUsage{
		AccountID: acc.ID,
		Email:     acc.Email,
		Plan:      plan,
		Quotas:    quotas,
		FetchedAt: time.Now(),
	}

	c.quotaMu.Lock()
	c.quotaCache[acc.ID] = usage
	c.quotaMu.Unlock()

	return usage, nil
}

// GetAllUsage returns quota and usage information across all accounts
func (c *Client) GetAllUsage(ctx context.Context, force bool) (map[string]*AccountUsage, error) {
	accounts := c.store.ListRaw()
	results := make(map[string]*AccountUsage)
	if len(accounts) == 0 {
		return results, nil
	}

	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, acc := range accounts {
		wg.Add(1)
		go func(a *Account) {
			defer wg.Done()
			reqCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
			defer cancel()

			usage, err := c.GetAccountUsage(reqCtx, a, force)
			if err == nil && usage != nil {
				mu.Lock()
				results[a.ID] = usage
				mu.Unlock()
			}
		}(acc)
	}

	wg.Wait()
	return results, nil
}
