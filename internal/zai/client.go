package zai

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"lun-router/internal/chatgpt"
)

const (
	baseURL   = "https://chat.z.ai"
	feVersion = "prod-fe-1.1.107"
	userAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10.15; rv:158.0) Gecko/20100101 Firefox/158.0"

	debugPort  = "9333"
	turnTimout = 240 * time.Second
)

// ProviderConfig is persisted to data/zai.json.
type ProviderConfig struct {
	Enabled     bool   `json:"enabled"`
	BrowserPath string `json:"browser_path,omitempty"`
	// InternalTools opts out of handing tool calls to the calling agent. Stored
	// inverted so the zero value keeps passthrough enabled, matching the
	// ChatGPT provider's convention.
	InternalTools bool `json:"internal_tools,omitempty"`
}

// Config configures a Client.
type Config struct {
	DataDir     string
	BrowserPath string
	// CDPPort overrides the browser debug port.
	CDPPort string
}

// Client talks to chat.z.ai.
//
// Chat traffic is plain HTTP: the guest token, the x-signature HMAC and the SSE
// stream are all reproduced natively. A browser is used for one thing only -
// minting Aliyun captcha params, which are single-use and cannot be forged.
type Client struct {
	cfg       Config
	installer *chatgpt.Installer
	http      *http.Client

	mu       sync.RWMutex
	conf     ProviderConfig
	confPath string

	creds   Credentials
	credsMu sync.RWMutex

	browserMu  sync.Mutex
	browser    *browser
	profileDir string
	debugPort  string

	pool paramPool

	// blockMu guards lastBlockRotate, which throttles identity rotation while
	// the upstream is blocking this client.
	blockMu         sync.Mutex
	lastBlockRotate time.Time

	// turnMu paces outgoing turns. The endpoint sits behind an Aliyun (ESA) WAF
	// that blocks bursts: ~100 requests in a few minutes got this client cut off
	// with an edge-generated 405, and replaying a previously-successful captured
	// request verbatim was still refused. Pacing is the cheap way to stay a
	// well-behaved client.
	turnMu      sync.Mutex
	lastTurnEnd time.Time

	reqID int64
}

// blockCooldown stops the throttle-recovery path from thrashing. The block was
// measured to be IP-level: rotating the device identity and retrying still
// returned 405, and it persisted minutes later. Since relaunching the browser per
// request buys nothing while the block holds, recovery is attempted at most once
// per this interval.
const blockCooldown = 3 * time.Minute

// minTurnGap is the minimum spacing between upstream turns. The captcha mint is
// already serialized through the browser, so this only stops bursts.
const minTurnGap = 1200 * time.Millisecond

// NewClient builds a client and loads persisted config.
func NewClient(cfg Config) *Client {
	if cfg.DataDir == "" {
		cfg.DataDir = "data"
	}
	abs, err := filepath.Abs(cfg.DataDir)
	if err == nil {
		cfg.DataDir = abs
	}
	c := &Client{
		cfg:        cfg,
		installer:  installerFor(cfg.DataDir),
		http:       &http.Client{Timeout: 0},
		confPath:   filepath.Join(cfg.DataDir, "zai.json"),
		profileDir: filepath.Join(os.TempDir(), "lun_zai_profile"),
		debugPort:  debugPort,
	}
	if cfg.CDPPort != "" {
		c.debugPort = cfg.CDPPort
	}
	_ = c.loadConfig()
	return c
}

func (c *Client) loadConfig() error {
	raw, err := os.ReadFile(c.confPath)
	if err != nil {
		return err
	}
	var conf ProviderConfig
	if err := json.Unmarshal(raw, &conf); err != nil {
		return err
	}
	c.mu.Lock()
	c.conf = conf
	c.mu.Unlock()
	return nil
}

func (c *Client) saveConfigLocked() error {
	if err := os.MkdirAll(c.cfg.DataDir, 0755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(c.conf, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(c.confPath, raw, 0644)
}

// Installer exposes the shared browser installer.
func (c *Client) Installer() *chatgpt.Installer { return c.installer }

// IsEnabled reports whether the provider is switched on.
func (c *Client) IsEnabled() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.conf.Enabled
}

// SetEnabled toggles the provider, shutting the browser down when disabled.
func (c *Client) SetEnabled(enabled bool) error {
	c.mu.Lock()
	c.conf.Enabled = enabled
	if !enabled {
		if c.cfg.BrowserPath == "" {
			c.conf.BrowserPath = ""
		}
	}
	err := c.saveConfigLocked()
	c.mu.Unlock()

	if !enabled {
		c.browserMu.Lock()
		c.dropBrowserLocked()
		c.browserMu.Unlock()
	}
	return err
}

// PassthroughTools reports whether tool calls are handed to the calling agent.
func (c *Client) PassthroughTools() bool {
	if os.Getenv("ZAI_INTERNAL_TOOLS") == "1" {
		return false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return !c.conf.InternalTools
}

// SetPassthroughTools stores the inverse of passthrough.
func (c *Client) SetPassthroughTools(enabled bool) error {
	c.mu.Lock()
	c.conf.InternalTools = !enabled
	err := c.saveConfigLocked()
	c.mu.Unlock()
	return err
}

// SetBrowserPath pins a browser executable, overriding auto-detection.
func (c *Client) SetBrowserPath(path string) error {
	c.mu.Lock()
	c.conf.BrowserPath = path
	err := c.saveConfigLocked()
	c.mu.Unlock()
	return err
}

// GetCatalogModels returns the model ids this session can actually call.
//
// An anonymous session is restricted to the guest-usable subset, because the
// service rejects everything else with 403 "Model not available for current
// user level" - advertising those would hand clients models that never answer.
func (c *Client) GetCatalogModels() []string {
	models := UsableModels(c.SignedIn())
	out := make([]string, 0, len(models))
	for _, m := range models {
		out = append(out, m.ID)
	}
	return out
}

// ResetSession discards the browser, its profile and the cached credentials so
// the next mint runs as a new device.
//
// The service throttles per device, so a fresh profile yields a new _arms_uid and
// a new guest token. It does NOT clear an IP-level block - that was measured to
// survive rotation - so callers gate this with shouldRotateOnBlock.
func (c *Client) ResetSession() {
	c.browserMu.Lock()
	c.dropBrowserLocked()
	c.browserMu.Unlock()

	c.credsMu.Lock()
	c.creds = Credentials{}
	c.credsMu.Unlock()

	c.pool.reset()
}

// than an anonymous guest. Only guest sessions are implemented today, so this is
// false until account tokens are supported.
// paceTurn blocks until at least minTurnGap has elapsed since the previous turn
// finished, so a retrying client cannot hammer the endpoint into blocking it.
func (c *Client) paceTurn() {
	c.turnMu.Lock()
	defer c.turnMu.Unlock()
	if wait := minTurnGap - time.Since(c.lastTurnEnd); wait > 0 {
		time.Sleep(wait)
	}
	c.lastTurnEnd = time.Now()
}

// shouldRotateOnBlock reports whether an upstream block warrants another identity
// rotation, and records the attempt. Without it a blocked client would relaunch
// the browser on every request and never make progress.
func (c *Client) shouldRotateOnBlock() bool {
	c.blockMu.Lock()
	defer c.blockMu.Unlock()
	if time.Since(c.lastBlockRotate) < blockCooldown {
		return false
	}
	c.lastBlockRotate = time.Now()
	return true
}

// SignedIn reports whether the cached session is an authenticated account rather
// than an anonymous guest. Only guest sessions are implemented today, so this is
// false until account tokens are supported.
func (c *Client) SignedIn() bool {
	c.credsMu.RLock()
	defer c.credsMu.RUnlock()
	if c.creds.Token == "" {
		return false
	}
	parts := strings.Split(c.creds.Token, ".")
	if len(parts) < 2 {
		return false
	}
	pad := parts[1] + strings.Repeat("=", (4-len(parts[1])%4)%4)
	raw, err := base64.URLEncoding.DecodeString(pad)
	if err != nil {
		return false
	}
	var claims struct {
		Email string `json:"email"`
	}
	if json.Unmarshal(raw, &claims) != nil {
		return false
	}
	return !strings.HasSuffix(claims.Email, "@guest.com")
}

// BrowserRunning reports whether a browser is currently attached.
func (c *Client) BrowserRunning() bool {
	c.browserMu.Lock()
	defer c.browserMu.Unlock()
	return c.browserAliveLocked()
}

// GetMemoryUsage reports live RSS metrics for the browser process tree.
//
// The browser is launched lazily and only for captcha minting, so an idle
// provider legitimately reports nothing running.
func (c *Client) GetMemoryUsage() chatgpt.MemoryUsage {
	c.browserMu.Lock()
	b := c.browser
	c.browserMu.Unlock()

	if b == nil {
		return chatgpt.MemoryUsage{Running: false}
	}
	return chatgpt.ProcessTreeUsage(b.pid())
}

// WarmUp mints one captcha param and pools it so the first real request is not
// cold. The browser launch dominates the cost (~7s), so doing it eagerly matters.
func (c *Client) WarmUp() {
	if !c.IsEnabled() {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	if _, err := c.credentials(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "[zai] warm-up failed: %v\n", err)
		return
	}
	fmt.Fprintln(os.Stderr, "[zai] warm-up complete")
}

// Close releases the browser and any pooled params.
func (c *Client) Close() {
	c.browserMu.Lock()
	c.dropBrowserLocked()
	c.browserMu.Unlock()
}

// captchaParam returns a single-use captcha param, preferring the pool and
// minting on demand otherwise. Tokens cannot be replayed, so every completion
// consumes exactly one.
func (c *Client) captchaParam(ctx context.Context) (captchaParam, error) {
	if p, ok := c.pool.take(); ok {
		return p, nil
	}
	res, err := c.mintCaptcha(ctx)
	if err != nil {
		return captchaParam{}, err
	}
	c.credsMu.Lock()
	c.creds = res.Credentials
	c.credsMu.Unlock()

	p, err := decodeParam(res.Param)
	if err != nil {
		// An undecodable param is still usable; keep the raw value.
		p = captchaParam{Raw: res.Param}
	}
	p.MintedAt = time.Now()
	return p, nil
}

// credentials returns cached session credentials, minting them if absent.
func (c *Client) credentials(ctx context.Context) (Credentials, error) {
	c.credsMu.RLock()
	creds := c.creds
	c.credsMu.RUnlock()
	if creds.Token != "" {
		return creds, nil
	}
	res, err := c.mintCaptcha(ctx)
	if err != nil {
		return Credentials{}, err
	}
	c.credsMu.Lock()
	c.creds = res.Credentials
	c.credsMu.Unlock()
	if p, err := decodeParam(res.Param); err == nil {
		p.MintedAt = time.Now()
		c.pool.put(p)
	} else {
		c.pool.put(captchaParam{Raw: res.Param, MintedAt: time.Now()})
	}
	return res.Credentials, nil
}

// fetchGuestToken requests an anonymous token without a browser. This is the
// same call the page makes on load.
func fetchGuestToken(ctx context.Context) (token, userID string, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/api/v1/auths/", nil)
	if err != nil {
		return "", "", err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Referer", baseURL+"/")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("guest token request returned %d", resp.StatusCode)
	}
	var d struct {
		ID    string `json:"id"`
		Token string `json:"token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&d); err != nil {
		return "", "", err
	}
	if d.Token == "" {
		return "", "", errors.New("guest token response carried no token")
	}
	return d.Token, d.ID, nil
}

// userIDFromToken reads the id claim out of the JWT payload.
func userIDFromToken(token string) string {
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return ""
	}
	pad := parts[1] + strings.Repeat("=", (4-len(parts[1])%4)%4)
	raw, err := base64.URLEncoding.DecodeString(pad)
	if err != nil {
		return ""
	}
	var claims struct {
		ID string `json:"id"`
	}
	if json.Unmarshal(raw, &claims) != nil {
		return ""
	}
	return claims.ID
}

// completionRequest is the upstream request body.
type completionRequest struct {
	Stream          bool             `json:"stream"`
	Model           string           `json:"model"`
	Messages        []map[string]any `json:"messages"`
	SignaturePrompt string           `json:"signature_prompt"`
	Params          map[string]any   `json:"params"`
	Extra           map[string]any   `json:"extra"`
	Features        map[string]any   `json:"features"`
	ChatID          string           `json:"chat_id"`
	ID              string           `json:"id"`
	UserMessageID   string           `json:"current_user_message_id"`
	ParentID        any              `json:"current_user_message_parent_id"`
	BackgroundTask  map[string]any   `json:"background_tasks"`
	Captcha         string           `json:"captcha_verify_param"`
}

// completionResult is the outcome of one upstream turn.
type completionResult struct {
	Text      string
	Reasoning string
	Usage     json.RawMessage
	Err       string
	Status    int
	// Blocked marks a reply that was not SSE at all, which is how the service
	// signals throttling. It is separated from Err because it is recoverable by
	// rotating the device identity.
	Blocked bool
}

// sendTurn performs one upstream turn and folds the SSE stream.
func (c *Client) sendTurn(ctx context.Context, model, prompt, captcha string, think bool, effort string) (completionResult, error) {
	c.paceTurn()
	creds, err := c.credentials(ctx)
	if err != nil {
		return completionResult{}, err
	}
	if captcha == "" {
		p, err := c.captchaParam(ctx)
		if err != nil {
			return completionResult{}, err
		}
		captcha = p.Raw
	}

	now := time.Now()
	ts := strconv.FormatInt(now.UnixMilli(), 10)
	requestID := newUUID()
	messageID := newUUID()

	body := completionRequest{
		Stream:          true,
		Model:           model,
		Messages:        []map[string]any{{"role": "user", "content": prompt}},
		SignaturePrompt: prompt,
		Params:          map[string]any{},
		Extra:           map[string]any{},
		Features: map[string]any{
			"image_generation": false,
			"web_search":       false,
			"auto_web_search":  false,
			"preview_mode":     true,
			"flags":            []string{},
			"enable_thinking":  think,
		},
		ChatID:         "",
		ID:             messageID,
		UserMessageID:  requestID,
		ParentID:       nil,
		BackgroundTask: map[string]any{"title_generation": false, "tags_generation": false},
		Captcha:        captcha,
	}
	if think && effort != "" {
		body.Features["reasoning_effort"] = effort
	}

	raw, err := json.Marshal(body)
	if err != nil {
		return completionResult{}, err
	}

	q := url.Values{}
	q.Set("timestamp", ts)
	q.Set("requestId", requestID)
	q.Set("version", "0.0.1")
	q.Set("platform", "web")
	q.Set("language", "en-US")
	q.Set("timezone", "Asia/Jakarta")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		baseURL+"/api/v2/chat/completions?"+q.Encode(), bytes.NewReader(raw))
	if err != nil {
		return completionResult{}, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Authorization", "Bearer "+creds.Token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Origin", baseURL)
	req.Header.Set("Referer", baseURL+"/")
	req.Header.Set("x-fe-version", feVersion)
	req.Header.Set("x-region", "overseas")
	if creds.DeviceID != "" {
		req.Header.Set("x-device-id", creds.DeviceID)
	}
	req.Header.Set("x-signature", Signature(ts, requestID, creds.UserID, prompt))

	resp, err := c.http.Do(req)
	if err != nil {
		return completionResult{}, err
	}
	defer resp.Body.Close()

	dec, err := readStream(resp.Body, nil)
	if err != nil && dec == nil {
		return completionResult{}, err
	}

	res := completionResult{Usage: dec.usage, Status: resp.StatusCode}
	res.Text, res.Reasoning = dec.Answer()
	if raw := dec.errRaw; len(raw) > 0 {
		res.Err = upstreamErrorMessage(raw)
	}
	if res.Err == "" && res.Text == "" {
		res.Err = describeEmptyReply(resp.StatusCode, dec)
		// A non-SSE body is a throttle, not a model failure.
		res.Blocked = dec.buf == "" && dec.rawSnippet != ""
	}
	return res, nil
}

// describeEmptyReply explains a reply that carried no usable content. A non-200
// status usually means throttling, and the service signals that with an HTML
// page rather than a JSON error, so the status and a body snippet are reported
// instead of a bare "empty reply".
func describeEmptyReply(status int, dec *decoder) string {
	if dec.buf != "" {
		// Content arrived, but nothing survived as a visible answer: the model's
		// reasoning block was never closed, so there is no reply to return.
		return "upstream produced reasoning but no answer"
	}
	switch {
	case status >= 500:
		return fmt.Sprintf("upstream returned %d with no content", status)
	case status == http.StatusTooManyRequests:
		return "upstream rate-limited this client (429). Wait before retrying."
	case status != http.StatusOK:
		msg := fmt.Sprintf("upstream returned %d with no SSE content", status)
		if dec.rawSnippet != "" {
			msg += ": " + summarizeHTML(dec.rawSnippet)
		}
		return msg
	case dec.rawSnippet != "":
		return "upstream replied without SSE: " + summarizeHTML(dec.rawSnippet)
	default:
		return "upstream returned an empty reply"
	}
}

// summarizeHTML strips a non-SSE body down to something readable. A throttle page
// is HTML, and dumping its markup into an error message helps nobody.
func summarizeHTML(snippet string) string {
	s := strings.TrimSpace(snippet)
	if strings.HasPrefix(strings.ToLower(s), "<!doctype") || strings.HasPrefix(strings.ToLower(s), "<html") {
		if start := strings.Index(s, "<title>"); start != -1 {
			if end := strings.Index(s[start:], "</title>"); end != -1 {
				title := s[start+len("<title>") : start+end]
				return fmt.Sprintf("an HTML block page (title %q) - the service is throttling this client", title)
			}
		}
		return "an HTML block page - the service is throttling this client"
	}
	return truncateRaw(s, 160)
}

// upstreamErrorMessage renders the service's error payload compactly.
func upstreamErrorMessage(raw json.RawMessage) string {
	var e struct {
		Code       any    `json:"code"`
		Detail     string `json:"detail"`
		Message    string `json:"message"`
		ErrorCode  string `json:"error_code"`
		VerifyCode string `json:"verify_code"`
		CaptchaErr string `json:"captcha_error_type"`
	}
	if err := json.Unmarshal(raw, &e); err != nil {
		return strings.TrimSpace(string(raw))
	}
	parts := make([]string, 0, 4)
	if s := fmt.Sprint(e.Code); s != "" && s != "<nil>" {
		parts = append(parts, "code "+s)
	}
	msg := e.Detail
	if msg == "" {
		msg = e.Message
	}
	if msg != "" {
		parts = append(parts, msg)
	}
	if e.VerifyCode != "" {
		parts = append(parts, "verify "+e.VerifyCode)
	}
	if e.CaptchaErr != "" {
		parts = append(parts, "captcha "+e.CaptchaErr)
	}
	if len(parts) == 0 {
		return strings.TrimSpace(string(raw))
	}
	return strings.Join(parts, ": ")
}

// isCaptchaError reports whether an upstream error means the captcha param was
// rejected, which is the one error worth retrying with a fresh token.
func isCaptchaError(msg string) bool {
	return strings.Contains(msg, "FRONTEND_CAPTCHA") || strings.Contains(msg, "captcha")
}

// newUUID returns a v4-shaped UUID. The service only needs it to be unique per
// request; it never correlates it with anything we control.
func newUUID() string {
	var b [16]byte
	seq := uuidCounter.Add(1)
	nano := uint64(time.Now().UnixNano())
	for i := 0; i < 16; i++ {
		// Mix a nanosecond clock with a counter so two calls in the same
		// nanosecond still differ.
		shift := uint((i % 8) * 8)
		b[i] = byte((nano>>shift)&0xff) ^ byte((uint64(seq)>>shift)&0xff)
		nano = nano*6364136223846793005 + 1442695040888963407
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

var uuidCounter atomic.Int64

var uuidMu sync.Mutex
