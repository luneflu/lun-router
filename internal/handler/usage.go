package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strconv"

	"lun-router/internal/usage"
)

type UsageHandler struct {
	service *usage.Service
}

func NewUsageHandler(s *usage.Service) *UsageHandler {
	return &UsageHandler{service: s}
}

// GET /api/usage?days=7
func (h *UsageHandler) Summary(w http.ResponseWriter, r *http.Request) {
	if h.service == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "usage service not available"})
		return
	}

	days := 7
	if raw := r.URL.Query().Get("days"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil {
			days = n
		}
	}

	f := usage.Filter{
		Provider: r.URL.Query().Get("prov"),
		KeyID:    r.URL.Query().Get("key_id"),
		Model:    r.URL.Query().Get("model"),
	}

	sum, err := h.service.Summary(days, f)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(sum)
}

// statusRecorder captures the status code and keeps http.Flusher working for the
// streaming handlers.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) {
	if s.status == 0 {
		s.status = code
	}
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusRecorder) Write(b []byte) (int, error) {
	if s.status == 0 {
		s.status = http.StatusOK
	}
	return s.ResponseWriter.Write(b)
}

func (s *statusRecorder) Flush() {
	if f, ok := s.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// usageEnvelope accepts every usage shape the upstreams produce: OpenAI
// prompt_tokens/completion_tokens, Gemini usageMetadata counts, and the Responses
// API input_tokens/output_tokens form.
type usageEnvelope struct {
	PromptTokens            int           `json:"prompt_tokens"`
	CompletionTokens        int           `json:"completion_tokens"`
	PromptTokensDetails     *usageDetails `json:"prompt_tokens_details"`
	CompletionTokensDetails *usageDetails `json:"completion_tokens_details"`
	PromptTokenCount        int           `json:"promptTokenCount"`
	CandidatesTokenCount    int           `json:"candidatesTokenCount"`
	ThoughtsTokenCount      int           `json:"thoughtsTokenCount"`
	CachedContentTokenCount int           `json:"cachedContentTokenCount"`
	InputTokens             int           `json:"input_tokens"`
	OutputTokens            int           `json:"output_tokens"`
	InputTokensDetails      *usageDetails `json:"input_tokens_details"`
	OutputTokensDetails     *usageDetails `json:"output_tokens_details"`
}

type usageDetails struct {
	CachedTokens    int `json:"cached_tokens"`
	ReasoningTokens int `json:"reasoning_tokens"`
}

// apply writes the counts into tk, preferring the OpenAI shape and falling back to
// the Gemini and Responses shapes. It is a no-op when nothing was reported.
func (u *usageEnvelope) apply(tk *usage.Tokens) {
	if u == nil || tk == nil {
		return
	}

	in, out, cached, reasoning := u.PromptTokens, u.CompletionTokens, 0, 0
	if u.PromptTokensDetails != nil {
		cached = u.PromptTokensDetails.CachedTokens
	}
	if u.CompletionTokensDetails != nil {
		reasoning = u.CompletionTokensDetails.ReasoningTokens
	}
	if in == 0 && out == 0 {
		in, out = u.PromptTokenCount, u.CandidatesTokenCount
		cached, reasoning = u.CachedContentTokenCount, u.ThoughtsTokenCount
	}
	if in == 0 && out == 0 {
		in, out = u.InputTokens, u.OutputTokens
		if u.InputTokensDetails != nil {
			cached = u.InputTokensDetails.CachedTokens
		}
		if u.OutputTokensDetails != nil {
			reasoning = u.OutputTokensDetails.ReasoningTokens
		}
	}
	if in == 0 && out == 0 {
		return
	}
	tk.Set(in, out, cached, reasoning)
}

// applyAny re-encodes an arbitrary decoded JSON value into a usageEnvelope.
func (u *usageEnvelope) applyAny(raw any, tk *usage.Tokens) {
	if raw == nil {
		return
	}
	if m, ok := raw.(map[string]any); ok {
		u.applyMap(m, tk)
		return
	}
	b, err := json.Marshal(raw)
	if err != nil {
		return
	}
	var env usageEnvelope
	if json.Unmarshal(b, &env) == nil {
		env.apply(tk)
	}
}

func (u *usageEnvelope) applyMap(m map[string]any, tk *usage.Tokens) {
	b, err := json.Marshal(m)
	if err != nil {
		return
	}
	var env usageEnvelope
	if json.Unmarshal(b, &env) == nil {
		env.apply(tk)
	}
}

// usagePayload renders tk in the OpenAI usage shape for clients.
func usagePayload(tk *usage.Tokens) map[string]any {
	return map[string]any{
		"prompt_tokens":     tk.In,
		"completion_tokens": tk.Out,
		"total_tokens":      tk.In + tk.Out,
		"prompt_tokens_details": map[string]any{
			"cached_tokens": tk.Cached,
		},
		"completion_tokens_details": map[string]any{
			"reasoning_tokens": tk.Reasoning,
		},
	}
}

// clientSessionID returns the caller's session identifier when it sends one.
func clientSessionID(r *http.Request) string {
	if s := r.Header.Get("x-opencode-session"); s != "" {
		return s
	}
	return r.Header.Get("x-opencode-session-id")
}

// captureStreamUsage pulls the provider usage chunk out of one SSE line.
func captureStreamUsage(line []byte, tk *usage.Tokens) {
	if tk == nil || !bytes.Contains(line, []byte(`"usage"`)) {
		return
	}
	payload := bytes.TrimSpace(line)
	payload = bytes.TrimPrefix(payload, []byte("data: "))
	if len(payload) == 0 || payload[0] != '{' {
		return
	}
	var chunk struct {
		Usage json.RawMessage `json:"usage"`
	}
	if err := json.Unmarshal(payload, &chunk); err != nil || len(chunk.Usage) == 0 {
		return
	}
	var env usageEnvelope
	if err := json.Unmarshal(chunk.Usage, &env); err != nil {
		return
	}
	env.apply(tk)
}

// captureNonStreamUsage pulls the usage object out of a full JSON response.
func captureNonStreamUsage(body []byte, tk *usage.Tokens) {
	if tk == nil || !bytes.Contains(body, []byte(`"usage"`)) {
		return
	}
	var resp struct {
		Usage json.RawMessage `json:"usage"`
	}
	if err := json.Unmarshal(body, &resp); err != nil || len(resp.Usage) == 0 {
		return
	}
	var env usageEnvelope
	if err := json.Unmarshal(resp.Usage, &env); err != nil {
		return
	}
	env.apply(tk)
}
