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
	"net/http"
	"regexp"
	"strings"
	"sync"
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
	consoleBaseURL string
	httpClient     *http.Client
	sessionManager *SessionManager
	versionManager *VersionManager
	modelManager   *ModelManager
	accountStore   *AccountStore
	refreshMu      sync.Mutex
	quotaMu        sync.RWMutex
	quotaCache     map[string]*AccountUsage
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
		consoleBaseURL: ConsoleBaseURL,
		httpClient:     http.DefaultClient,
		sessionManager: NewSessionManager(30 * time.Minute),
		versionManager: vm,
		quotaCache:     make(map[string]*AccountUsage),
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

func (c *Client) AccountStore() *AccountStore {
	return c.accountStore
}

func (c *Client) SetAccountStore(store *AccountStore) {
	c.accountStore = store
}

// IsFreeTierModel reports whether a model is served by the attested free-tier
// identity (Bearer public) instead of the OpenCode Go keyed lane.
func IsFreeTierModel(model string) bool {
	clean := baseModelID(model)
	if slash := strings.LastIndex(clean, "/"); slash != -1 {
		clean = clean[slash+1:]
	}
	clean = strings.ToLower(clean)
	// big-pickle has no "-free" suffix but lives on the free catalog.
	return clean == "big-pickle" || strings.HasSuffix(clean, "-free")
}

// IsResponsesModel reports whether a model is served by the Responses endpoint.
func IsResponsesModel(model string) bool {
	clean := strings.ToLower(baseModelID(model))
	return strings.Contains(clean, "muse-spark") ||
		strings.HasPrefix(clean, "grok-") ||
		strings.HasPrefix(clean, "gpt-")
}

func baseModelID(model string) string {
	clean := strings.TrimSpace(model)
	if idx := strings.LastIndex(clean, "("); idx != -1 && strings.HasSuffix(clean, ")") {
		clean = strings.TrimSpace(clean[:idx])
	}
	return clean
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

type modelCatalog struct {
	Object string           `json:"object"`
	Data   []map[string]any `json:"data"`
}

func (c *Client) fetchModelCatalog(ctx context.Context, path string) (*modelCatalog, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+path, nil)
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

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var catalog modelCatalog
	if err := json.Unmarshal(raw, &catalog); err != nil {
		return nil, fmt.Errorf("decode models response: %w", err)
	}
	return &catalog, nil
}

// FetchModelsDirect queries the free catalog and, when an OAuth account is
// configured, merges the OpenCode Go catalog so Go-only models are selectable.
func (c *Client) FetchModelsDirect(ctx context.Context) ([]byte, error) {
	catalog, err := c.fetchModelCatalog(ctx, "/zen/v1/models")
	if err != nil {
		return nil, err
	}

	if c.accountStore != nil && c.accountStore.FirstActive() != nil {
		if goCatalog, err := c.fetchModelCatalog(ctx, "/zen/go/v1/models"); err == nil {
			seen := make(map[string]bool, len(catalog.Data))
			for _, model := range catalog.Data {
				if id, ok := model["id"].(string); ok {
					seen[id] = true
				}
			}
			for _, model := range goCatalog.Data {
				id, _ := model["id"].(string)
				if id != "" && !seen[id] {
					seen[id] = true
					catalog.Data = append(catalog.Data, model)
				}
			}
		}
	}

	catalog.Object = "list"
	return json.Marshal(catalog)
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

	isResponses := IsResponsesModel(model)
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
		sanitizeAssistantReasoning(bodyMap)
	}

	sessionID := customSession
	if sessionID == "" {
		sessionID = c.sessionManager.Get(identity)
	}

	// Accounts are sticky per session. With an OAuth login, paid models use the
	// OpenCode Go lane; free-only models and anonymous requests stay on the
	// public free lane.
	account := c.selectAccount(sessionID)
	lane := "/inference/v1"
	auth := "Bearer public"
	withRequestID := true
	headers := map[string]string{}
	if account != nil && !IsFreeTierModel(model) {
		fresh, err := c.ensureFreshAccount(ctx, account)
		if err != nil {
			return nil, err
		}
		lane = "/zen/go/v1"
		auth = "Bearer " + fresh.AccessToken
		withRequestID = false
		if fresh.OrgID != "" {
			headers["x-opencode-org-id"] = fresh.OrgID
		}
	} else {
		// The free-tier gate requires the file-search quartet.
		ApplyFingerprint(bodyMap, isResponses)
	}

	payloadBytes, err := json.Marshal(bodyMap)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	targetURL := c.baseURL + lane + "/chat/completions"
	if isResponses {
		targetURL = c.baseURL + lane + "/responses"
	}

	return c.doRequest(ctx, targetURL, payloadBytes, sessionID, auth, withRequestID, headers)
}

func (c *Client) selectAccount(sessionID string) *Account {
	if c.accountStore == nil {
		return nil
	}
	return c.accountStore.SelectForSession(sessionID)
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

func (c *Client) doRequest(ctx context.Context, targetURL string, payloadBytes []byte, sessionID, auth string, withRequestID bool, extraHeaders map[string]string) (*http.Response, error) {
	upReq, err := http.NewRequestWithContext(ctx, "POST", targetURL, bytes.NewReader(payloadBytes))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	// Aligned Custom Headers for OpenCode Zen/Go v2.
	upReq.Header.Set("User-Agent", c.versionManager.GetUserAgent())
	upReq.Header.Set("x-opencode-client", ClientType)
	upReq.Header.Set("x-opencode-project", Project)
	upReq.Header.Set("x-opencode-session", sessionID)
	upReq.Header.Set("x-opencode-session-id", sessionID)
	if withRequestID {
		upReq.Header.Set("x-opencode-request", generateRequestID())
	}
	upReq.Header.Set("x-session-affinity", sessionID)
	upReq.Header.Set("X-Session-Id", sessionID)
	upReq.Header.Set("Authorization", auth)
	upReq.Header.Set("Content-Type", "application/json")
	upReq.Header.Set("Accept", "text/event-stream")
	for k, v := range extraHeaders {
		upReq.Header.Set(k, v)
	}

	return c.httpClient.Do(upReq)
}

// AggregateSSEStream parses incoming SSE stream into a standard OpenAI ChatCompletion response
func AggregateSSEStream(r io.Reader, fallbackModel string, tk *usage.Tokens) (map[string]any, error) {
	var fullText strings.Builder
	var fullReasoning strings.Builder
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
					Role             string `json:"role"`
					Content          string `json:"content"`
					ReasoningContent string `json:"reasoning_content"`
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
				if chunk.Choices[0].Delta.ReasoningContent != "" {
					fullReasoning.WriteString(chunk.Choices[0].Delta.ReasoningContent)
				}
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

	assistantMsg := map[string]any{
		"role":    "assistant",
		"content": fullText.String(),
	}
	if fullReasoning.Len() > 0 {
		assistantMsg["reasoning_content"] = fullReasoning.String()
	}

	return map[string]any{
		"id":      lastID,
		"object":  "chat.completion",
		"created": lastCreated,
		"model":   lastModel,
		"choices": []map[string]any{
			{
				"index":         0,
				"message":       assistantMsg,
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

var thinkTagRe = regexp.MustCompile(`(?s)<think>(.*?)</think>`)

// sanitizeAssistantReasoning ensures assistant messages satisfy upstream thinking mode
// requirements (e.g. SiliconFlow/DeepSeek-R1 requiring non-null reasoning_content).
func sanitizeAssistantReasoning(bodyMap map[string]any) {
	raw, ok := bodyMap["messages"]
	if !ok {
		return
	}

	sanitizeMsg := func(m map[string]any) {
		role, _ := m["role"].(string)
		if role != "assistant" {
			return
		}

		rc, hasRC := m["reasoning_content"]
		rcStr, rcIsStr := rc.(string)
		if hasRC && rcIsStr && rcStr != "" {
			return
		}

		switch content := m["content"].(type) {
		case string:
			if matches := thinkTagRe.FindAllStringSubmatch(content, -1); len(matches) > 0 {
				var thoughts []string
				for _, match := range matches {
					if len(match) > 1 && strings.TrimSpace(match[1]) != "" {
						thoughts = append(thoughts, strings.TrimSpace(match[1]))
					}
				}
				if len(thoughts) > 0 {
					m["reasoning_content"] = strings.Join(thoughts, "\n")
					m["content"] = strings.TrimSpace(thinkTagRe.ReplaceAllString(content, ""))
					return
				}
			}
		case []any:
			var thoughts []string
			for _, part := range content {
				if pMap, ok := part.(map[string]any); ok {
					if t, _ := pMap["type"].(string); t == "text" {
						if textStr, _ := pMap["text"].(string); textStr != "" {
							if matches := thinkTagRe.FindAllStringSubmatch(textStr, -1); len(matches) > 0 {
								for _, match := range matches {
									if len(match) > 1 && strings.TrimSpace(match[1]) != "" {
										thoughts = append(thoughts, strings.TrimSpace(match[1]))
									}
								}
								pMap["text"] = strings.TrimSpace(thinkTagRe.ReplaceAllString(textStr, ""))
							}
						}
					}
				}
			}
			if len(thoughts) > 0 {
				m["reasoning_content"] = strings.Join(thoughts, "\n")
				return
			}
		}

		m["reasoning_content"] = ""
	}

	switch msgs := raw.(type) {
	case []any:
		for _, item := range msgs {
			if m, ok := item.(map[string]any); ok {
				sanitizeMsg(m)
			}
		}
	case []map[string]any:
		for _, m := range msgs {
			sanitizeMsg(m)
		}
	case []map[string]string:
		newMsgs := make([]map[string]any, len(msgs))
		for i, m := range msgs {
			newM := make(map[string]any, len(m)+1)
			for k, v := range m {
				newM[k] = v
			}
			sanitizeMsg(newM)
			newMsgs[i] = newM
		}
		bodyMap["messages"] = newMsgs
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
