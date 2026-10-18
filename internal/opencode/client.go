package opencode

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"lun-router/internal/usage"
)

const (
	BaseURL    = "https://opencode.ai"
	UA         = "opencode/1.18.35"
	ClientType = "desktop"
	Project    = "global"
	Request    = "_msg"
)

var FallbackModels = []string{
	"big-pickle",
	"ling-3.1-flash-free",
	"cat-jam-free",
	"mimo-v2.6-flash-free",
	"nemotron-3.5-lightning-free",
	"longcat-2.5-preview-free",
	"space-bunny-free",
	"muse-spark-1.2-contributor-free",
	"muse-spark-1.3-contributor-free",
}

type Client struct {
	baseURL        string
	httpClient     *http.Client
	sessionManager *SessionManager
	versionManager *VersionManager
	modelManager   *ModelManager
	proxyManager   *ProxyManager
}

func NewClient(baseURL string, vm *VersionManager) *Client {
	if baseURL == "" {
		baseURL = BaseURL
	}
	if vm == nil {
		vm = NewVersionManager("", 0)
	}
	return &Client{
		baseURL:        baseURL,
		httpClient:     http.DefaultClient,
		sessionManager: NewSessionManager(30 * time.Minute),
		versionManager: vm,
	}
}

func (c *Client) SessionManager() *SessionManager {
	return c.sessionManager
}

func (c *Client) VersionManager() *VersionManager {
	return c.versionManager
}

func (c *Client) ModelManager() *ModelManager {
	return c.modelManager
}

func (c *Client) SetModelManager(mm *ModelManager) {
	c.modelManager = mm
}

func (c *Client) ProxyManager() *ProxyManager {
	return c.proxyManager
}

func (c *Client) SetProxyManager(pm *ProxyManager) {
	c.proxyManager = pm
}

// FetchModels retrieves the catalog from OpenCode Free (uses cached if available)
func (c *Client) FetchModels(ctx context.Context) ([]byte, error) {
	if c.modelManager != nil {
		if raw := c.modelManager.GetRawJSON(); len(raw) > 0 {
			return raw, nil
		}
	}
	return c.FetchModelsDirect(ctx)
}

// FetchModelsDirect directly queries upstream OpenCode Free models endpoint
func (c *Client) FetchModelsDirect(ctx context.Context) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/zen/v1/models", nil)
	if err != nil {
		return nil, err
	}
	ua := UA
	if c.versionManager != nil {
		ua = c.versionManager.GetUserAgent()
	}
	req.Header.Set("User-Agent", ua)
	req.Header.Set("x-opencode-client", ClientType)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("upstream models status: %d", resp.StatusCode)
	}

	return io.ReadAll(resp.Body)
}

// ExecuteChat sends chat request to OpenCode Free with Issue #4781 bypass headers.
// If customSession is provided, it uses it; otherwise uses managed session.
// If upstream returns HTTP 429 Too Many Requests (session quota limit),
// it automatically rotates the session for this identity and retries once.
func (c *Client) ExecuteChat(ctx context.Context, bodyMap map[string]any, identity string, customSession string) (*http.Response, error) {
	if identity == "" {
		identity = "default"
	}

	model, _ := bodyMap["model"].(string)
	if model == "" {
		model = "big-pickle"
		bodyMap["model"] = model
	}

	// Normalize thinking suffix if present, e.g. "muse-spark-1.3-contributor-free(high)"
	var thinkingEffort string
	if idx := strings.LastIndex(model, "("); idx != -1 && strings.HasSuffix(model, ")") {
		thinkingEffort = strings.ToLower(strings.TrimSpace(model[idx+1 : len(model)-1]))
		model = strings.TrimSpace(model[:idx])
		bodyMap["model"] = model
	}
	if effortVal, ok := bodyMap["reasoning_effort"].(string); ok && effortVal != "" {
		thinkingEffort = strings.ToLower(strings.TrimSpace(effortVal))
	}

	// Upstream always requires stream: true
	bodyMap["stream"] = true

	isResponses := strings.Contains(model, "muse-spark")
	if isResponses {
		if thinkingEffort != "" {
			if thinkingEffort == "max" || thinkingEffort == "ultra" {
				thinkingEffort = "xhigh"
			}
			bodyMap["reasoning"] = map[string]any{
				"effort":  thinkingEffort,
				"summary": "auto",
			}
			delete(bodyMap, "reasoning_effort")
		}

		if maxTok, hasMax := bodyMap["max_tokens"]; hasMax {
			if _, hasOutput := bodyMap["max_output_tokens"]; !hasOutput {
				bodyMap["max_output_tokens"] = maxTok
			}
			delete(bodyMap, "max_tokens")
		}
		delete(bodyMap, "max_completion_tokens")

		var input []map[string]any

		switch msgs := bodyMap["messages"].(type) {
		case []any:
			for _, m := range msgs {
				if mMap, ok := m.(map[string]any); ok {
					role, _ := mMap["role"].(string)
					content, _ := mMap["content"].(string)
					if item, valid := toResponsesMessage(role, content); valid {
						input = append(input, item)
					}
				}
			}
		case []map[string]string:
			for _, mMap := range msgs {
				if item, valid := toResponsesMessage(mMap["role"], mMap["content"]); valid {
					input = append(input, item)
				}
			}
		case []map[string]any:
			for _, mMap := range msgs {
				role, _ := mMap["role"].(string)
				content, _ := mMap["content"].(string)
				if item, valid := toResponsesMessage(role, content); valid {
					input = append(input, item)
				}
			}
		}

		if len(input) == 0 {
			input = []map[string]any{
				{
					"type": "message",
					"role": "user",
					"content": []map[string]any{
						{"type": "input_text", "text": ""},
					},
				},
			}
		}

		bodyMap["input"] = input
		delete(bodyMap, "messages")
		bodyMap["store"] = false
	} else {
		if thinkingEffort != "" {
			bodyMap["reasoning_effort"] = thinkingEffort
		}
		delete(bodyMap, "max_output_tokens")
	}

	// Apply required fingerprint tools (read and shell for v2)
	ApplyFingerprint(bodyMap, isResponses)

	targetURL := c.baseURL + "/inference/v1/chat/completions"
	if isResponses {
		targetURL = c.baseURL + "/inference/v1/responses"
	}

	payloadBytes, err := json.Marshal(bodyMap)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	sessionID := customSession
	if sessionID == "" {
		sessionID = c.sessionManager.Get(identity)
	}

	// Pick initial HTTP client (direct connection)
	var initialClient *http.Client
	/* Proxy rotation commented out - keeping original IP
	if c.canUseProxy(targetURL) && c.proxyManager.IsEnabled() {
		initialClient = c.proxyManager.GetActiveClient()
		if initialClient == nil {
			if nextP := c.proxyManager.NextProxy(); nextP != nil {
				initialClient, _ = c.proxyManager.ClientForProxy(nextP)
			}
		}
	}
	*/

	resp, err := c.doRequest(ctx, targetURL, payloadBytes, sessionID, initialClient)
	if err != nil {
		/* Proxy retry commented out
		if c.canUseProxy(targetURL) && initialClient != nil {
			if act := c.proxyManager.ActiveProxy(); act != nil {
				c.proxyManager.MarkFailed(act, err.Error())
			}
			return c.executeWithProxyRetry(ctx, targetURL, payloadBytes, sessionID)
		}
		*/
		return nil, err
	}

	// If rate-limited (429 / free usage limit error): rotate session ID and retry once directly
	if c.isFreeLimit(resp) {
		resp.Body.Close()

		newSessionID := c.sessionManager.Rotate(identity)
		log.Printf("[opencode] rate limit hit (429). Rotated session to %s, retrying directly...", newSessionID)

		/* Proxy failover commented out
		if c.canUseProxy(targetURL) {
			log.Printf("[opencode] free limit or IP gate hit (%d). Switching to proxy pool (DE/Europe priority)...", resp.StatusCode)
			c.proxyManager.EnsureLoaded(ctx)
			return c.executeWithProxyRetry(ctx, targetURL, payloadBytes, newSessionID)
		}
		*/

		// Direct retry with fresh session ID
		return c.doRequest(ctx, targetURL, payloadBytes, newSessionID, nil)
	}

	return resp, nil
}

func (c *Client) isFreeLimit(resp *http.Response) bool {
	if resp == nil {
		return false
	}
	return resp.StatusCode == http.StatusTooManyRequests
}

func (c *Client) isFreeLimitOrBlocked(resp *http.Response) bool {
	if resp == nil {
		return false
	}
	return resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == http.StatusForbidden
}

func isLocalhost(rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	h := u.Hostname()
	return h == "localhost" || h == "127.0.0.1" || h == "::1"
}

func (c *Client) canUseProxy(targetURL string) bool {
	if c.proxyManager == nil {
		return false
	}
	// If target is localhost, only allow if configured proxy is also localhost (e.g. in tests)
	if isLocalhost(targetURL) {
		p := c.proxyManager.ActiveProxy()
		if p == nil {
			p = c.proxyManager.NextProxy()
		}
		if p == nil || !isLocalhost("http://"+p.Key()) {
			return false
		}
	}
	return true
}

func (c *Client) executeWithProxyRetry(ctx context.Context, targetURL string, payloadBytes []byte, sessionID string) (*http.Response, error) {
	const maxProxyAttempts = 5
	var lastErr error

	for attempt := 0; attempt < maxProxyAttempts; attempt++ {
		proxy := c.proxyManager.NextProxy()
		if proxy == nil {
			break
		}

		proxyClient, err := c.proxyManager.ClientForProxy(proxy)
		if err != nil {
			c.proxyManager.MarkFailed(proxy, err.Error())
			continue
		}

		log.Printf("[opencode-proxy] attempt %d/%d via %s (%s, prio=%d, uptime=%.1f%%, resp=%.0fms)",
			attempt+1, maxProxyAttempts, proxy.Key(), proxy.Country, proxy.Priority, proxy.UpTime, proxy.EffectiveLatency())

		resp, err := c.doRequest(ctx, targetURL, payloadBytes, sessionID, proxyClient)
		if err != nil {
			log.Printf("[opencode-proxy] proxy %s failed: %v", proxy.Key(), err)
			c.proxyManager.MarkFailed(proxy, err.Error())
			lastErr = err
			continue
		}

		if c.isFreeLimitOrBlocked(resp) {
			resp.Body.Close()
			log.Printf("[opencode-proxy] proxy %s also returned %d", proxy.Key(), resp.StatusCode)
			c.proxyManager.MarkFailed(proxy, fmt.Sprintf("status %d", resp.StatusCode))
			sessionID = c.sessionManager.Rotate(sessionID)
			continue
		}

		// Succeeded!
		c.proxyManager.MarkSuccess(proxy)
		log.Printf("[opencode-proxy] success via proxy %s (%s)", proxy.Key(), proxy.Country)
		return resp, nil
	}

	if lastErr != nil {
		return nil, fmt.Errorf("all proxy attempts failed: %w", lastErr)
	}
	return nil, fmt.Errorf("all proxy attempts returned rate limit (429)")
}

func generateRequestID() string {
	nowMs := time.Now().UnixMilli()
	val := uint64(nowMs*4096 + 1)
	timePart := fmt.Sprintf("%012x", val&0xFFFFFFFFFFFF)

	randomBytes := make([]byte, 14)
	_, _ = rand.Read(randomBytes)
	var randomPart strings.Builder
	for _, b := range randomBytes {
		randomPart.WriteByte(base62Chars[int(b)%62])
	}

	return fmt.Sprintf("msg_%s%s", timePart, randomPart.String())
}

func (c *Client) doRequest(ctx context.Context, targetURL string, payloadBytes []byte, sessionID string, httpClient *http.Client) (*http.Response, error) {
	upReq, err := http.NewRequestWithContext(ctx, "POST", targetURL, bytes.NewReader(payloadBytes))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	reqID := generateRequestID()

	// Aligned Custom Headers for OpenCode Free v2 (/inference/v1)
	upReq.Header.Set("User-Agent", c.versionManager.GetUserAgent())
	upReq.Header.Set("x-opencode-client", ClientType)
	upReq.Header.Set("x-opencode-project", Project)
	upReq.Header.Set("x-opencode-session", sessionID)
	upReq.Header.Set("x-opencode-session-id", sessionID)
	upReq.Header.Set("x-opencode-request", reqID)
	upReq.Header.Set("x-session-affinity", sessionID)
	upReq.Header.Set("X-Session-Id", sessionID)
	upReq.Header.Set("Authorization", "Bearer public")
	upReq.Header.Set("Content-Type", "application/json")
	upReq.Header.Set("Accept", "text/event-stream")

	if httpClient == nil {
		httpClient = c.httpClient
	}
	return httpClient.Do(upReq)
}

// AggregateSSEStream parses incoming SSE stream into a standard OpenAI ChatCompletion response
func AggregateSSEStream(r io.Reader, fallbackModel string, tk *usage.Tokens) (map[string]any, error) {
	var fullText strings.Builder
	var lastID string
	var lastModel = fallbackModel
	var lastCreated = time.Now().Unix()
	var finishReason = "stop"
	var lastUsage *usageEnvelope

	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || !strings.HasPrefix(line, "data: ") {
			continue
		}

		payload := strings.TrimPrefix(line, "data: ")
		if payload == "[DONE]" {
			break
		}

		var chunk struct {
			ID      string `json:"id"`
			Model   string `json:"model"`
			Created int64  `json:"created"`
			Choices []struct {
				Delta struct {
					Role    string `json:"role"`
					Content string `json:"content"`
				} `json:"delta"`
				FinishReason *string `json:"finish_reason"`
			} `json:"choices"`
			Usage *usageEnvelope `json:"usage"`
			// Responses API fields
			Type     string `json:"type"`
			Delta    any    `json:"delta"`
			Response struct {
				ID    string         `json:"id"`
				Usage *usageEnvelope `json:"usage"`
			} `json:"response"`
		}

		if err := json.Unmarshal([]byte(payload), &chunk); err == nil {
			if chunk.Usage != nil {
				lastUsage = chunk.Usage
			}
			if chunk.Response.Usage != nil {
				lastUsage = chunk.Response.Usage
			}
			if chunk.ID != "" {
				lastID = chunk.ID
			}
			if chunk.Response.ID != "" {
				lastID = chunk.Response.ID
			}
			if chunk.Model != "" {
				lastModel = chunk.Model
			}
			if chunk.Created > 0 {
				lastCreated = chunk.Created
			}
			if len(chunk.Choices) > 0 {
				fullText.WriteString(chunk.Choices[0].Delta.Content)
				if chunk.Choices[0].FinishReason != nil {
					finishReason = *chunk.Choices[0].FinishReason
				}
			}
			// Parse Responses API stream deltas
			if chunk.Delta != nil {
				if dMap, ok := chunk.Delta.(map[string]any); ok {
					if t, ok := dMap["text"].(string); ok {
						fullText.WriteString(t)
					}
				} else if dStr, ok := chunk.Delta.(string); ok {
					fullText.WriteString(dStr)
				}
			}
		}
	}

	if err := scanner.Err(); err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("read stream: %w", err)
	}

	if lastID == "" {
		b := make([]byte, 8)
		_, _ = rand.Read(b)
		lastID = "chatcmpl-" + hex.EncodeToString(b)
	}

	usageField := map[string]int{
		"prompt_tokens":     0,
		"completion_tokens": len(fullText.String()) / 4,
		"total_tokens":      len(fullText.String()) / 4,
	}
	if lastUsage != nil {
		lastUsage.apply(tk)
		usageField = lastUsage.toMap()
	}

	return map[string]any{
		"id":      lastID,
		"object":  "chat.completion",
		"created": lastCreated,
		"model":   lastModel,
		"choices": []map[string]any{
			{
				"index": 0,
				"message": map[string]string{
					"role":    "assistant",
					"content": fullText.String(),
				},
				"finish_reason": finishReason,
			},
		},
		"usage": usageField,
	}, nil
}

// usageEnvelope is the subset of provider usage counters this client understands.
type usageEnvelope struct {
	PromptTokens            int `json:"prompt_tokens"`
	CompletionTokens        int `json:"completion_tokens"`
	PromptTokenCount        int `json:"promptTokenCount"`
	CandidatesTokenCount    int `json:"candidatesTokenCount"`
	ThoughtsTokenCount      int `json:"thoughtsTokenCount"`
	CachedContentTokenCount int `json:"cachedContentTokenCount"`
	InputTokens             int `json:"input_tokens"`
	OutputTokens            int `json:"output_tokens"`
	PromptTokensDetails     *struct {
		CachedTokens int `json:"cached_tokens"`
	} `json:"prompt_tokens_details"`
	CompletionTokensDetails *struct {
		ReasoningTokens int `json:"reasoning_tokens"`
	} `json:"completion_tokens_details"`
}

func (u *usageEnvelope) counters() (in, out, cached, reasoning int) {
	in, out = u.PromptTokens, u.CompletionTokens
	if in == 0 && out == 0 {
		in, out = u.PromptTokenCount, u.CandidatesTokenCount
		reasoning = u.ThoughtsTokenCount
		cached = u.CachedContentTokenCount
	}
	if in == 0 && out == 0 {
		in, out = u.InputTokens, u.OutputTokens
	}
	if u.PromptTokensDetails != nil {
		cached = u.PromptTokensDetails.CachedTokens
	}
	if u.CompletionTokensDetails != nil {
		reasoning = u.CompletionTokensDetails.ReasoningTokens
	}
	return in, out, cached, reasoning
}

func (u *usageEnvelope) apply(tk *usage.Tokens) {
	in, out, cached, reasoning := u.counters()
	tk.Set(in, out, cached, reasoning)
}

func (u *usageEnvelope) toMap() map[string]int {
	in, out, cached, reasoning := u.counters()
	return map[string]int{
		"prompt_tokens":     in,
		"completion_tokens": out,
		"total_tokens":      in + out,
		"cached_tokens":     cached,
		"reasoning_tokens":  reasoning,
	}
}

func toResponsesMessage(role, content string) (map[string]any, bool) {
	if role == "assistant" && strings.TrimSpace(content) == "" {
		return nil, false
	}
	contentType := "input_text"
	if role == "assistant" {
		contentType = "output_text"
	}
	return map[string]any{
		"type": "message",
		"role": role,
		"content": []map[string]any{
			{"type": contentType, "text": content},
		},
	}, true
}
