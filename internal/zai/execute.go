package zai

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"lun-router/internal/chatgpt"
)

var responseIDCounter atomic.Int64

// ExecuteChat runs one chat completion against chat.z.ai and returns an
// OpenAI-shaped response body for the handler to relay.
//
// The upstream service takes no `tools` field - sending one fails with
// INTERNAL_ERROR - so tool use is driven by a prompt protocol instead. When the
// caller supplied tools, the model's call is parsed out and handed back as a
// standard tool_calls response, matching how the ChatGPT provider behaves.
func (c *Client) ExecuteChat(reqCtx context.Context, body map[string]any, stream bool) (*http.Response, error) {
	ctx, cancel := context.WithTimeout(reqCtx, turnTimout)

	model := ResolveModel(stringField(body, "model"))
	prompt := buildPrompt(body)
	specs := chatgpt.ParseToolSpecs(body)
	executed := chatgpt.ExecutedToolCalls(body)
	passthrough := c.PassthroughTools() && len(specs) > 0

	think, effort := thinkOptions(body, model)

	transcript := prompt
	if passthrough {
		// The preamble is sent on every turn, not only the first. Withholding it
		// once a tool result is present makes the model announce that it has no
		// tools at all - it then refuses to call again and answers from the
		// pasted result only.
		transcript = toolPreamble(specs) + prompt
	}

	res, err := c.sendTurn(ctx, model.ID, transcript, "", think, effort)
	if err != nil {
		cancel()
		return nil, err
	}

	// A rejected captcha token is the one upstream failure worth retrying: the
	// token is single-use, so a fresh one is all that is needed.
	if res.Err != "" && isCaptchaError(res.Err) {
		if retry, retryErr := c.sendTurn(ctx, model.ID, transcript, "", think, effort); retryErr == nil {
			res = retry
		}
	}

	if res.Err != "" {
		cancel()
		return nil, fmt.Errorf("Upstream z.ai error: %s", res.Err)
	}

	if passthrough {
		call := extractCall(res.Text, prompt, specs, executed)
		if call != nil {
			resp := buildToolCallResponse(call, model.ID, stream)
			resp.Body = &cancelOnClose{ReadCloser: resp.Body, cancel: cancel}
			return resp, nil
		}
	}

	resp := buildChatResponse(res.Text, res.Reasoning, model.ID, res.Usage, stream)
	resp.Body = &cancelOnClose{ReadCloser: resp.Body, cancel: cancel}
	return resp, nil
}

// extractCall pulls a tool call out of the model reply and normalises it onto
// the caller's advertised schema.
func extractCall(reply, userPrompt string, specs []chatgpt.ToolSpec, executed []chatgpt.ToolCall) *chatgpt.ToolCall {
	call, _ := chatgpt.ExtractToolCall(reply)
	if call == nil {
		// GLM sometimes answers with the bare JSON object and no wrapper tag.
		call = parseBareCall(reply)
	}
	if call == nil {
		return nil
	}
	if norm := chatgpt.NormalizeToolCall(specs, call); norm != nil {
		call = norm
	}
	for _, done := range executed {
		if chatgpt.SameToolCall(&done, call) {
			return nil
		}
	}
	return call
}

// parseBareCall accepts a reply that is just `{"name": ..., "arguments": ...}`,
// which is how the service delivers a tool call once it has stripped the
// <tool_call> wrapper.
func parseBareCall(reply string) *chatgpt.ToolCall {
	trimmed := strings.TrimSpace(reply)
	if !strings.HasPrefix(trimmed, "{") {
		return nil
	}
	var envelope struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
		Function  *struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		} `json:"function"`
	}
	if json.Unmarshal([]byte(trimmed), &envelope) != nil {
		return nil
	}

	name := envelope.Name
	rawArgs := envelope.Arguments
	if envelope.Function != nil && envelope.Function.Name != "" {
		name = envelope.Function.Name
		rawArgs = envelope.Function.Arguments
	}
	if name == "" {
		return nil
	}

	args := map[string]any{}
	if len(rawArgs) > 0 {
		if json.Unmarshal(rawArgs, &args) != nil {
			var asString string
			if json.Unmarshal(rawArgs, &asString) == nil {
				_ = json.Unmarshal([]byte(asString), &args)
			}
		}
	}
	if len(args) == 0 {
		return nil
	}
	return &chatgpt.ToolCall{Name: name, Arguments: args}
}

// thinkOptions decides whether reasoning is requested and at what effort.
func thinkOptions(body map[string]any, model Model) (bool, string) {
	if !model.Reasoning {
		return false, ""
	}
	thinking := true
	if tc, ok := body["thinking"].(map[string]any); ok {
		if v, ok := tc["type"].(string); ok && (v == "disabled" || v == "off" || v == "none") {
			thinking = false
		}
	}
	if raw, ok := body["reasoning_effort"].(string); ok && raw != "" {
		switch raw {
		case "none", "off", "disabled":
			return false, ""
		case "minimal", "low":
			return thinking, "low"
		case "medium":
			return thinking, "medium"
		case "high", "xhigh", "max":
			return thinking, "max"
		}
	}
	if thinking {
		return true, "max"
	}
	return false, ""
}

// buildPrompt flattens the caller's message history into the single transcript
// the upstream request carries.
func buildPrompt(body map[string]any) string {
	var sb strings.Builder
	msgs, ok := body["messages"].([]any)
	if !ok {
		if s, ok := body["prompt"].(string); ok {
			return strings.TrimSpace(s)
		}
		return ""
	}
	for _, m := range msgs {
		msg, ok := m.(map[string]any)
		if !ok {
			continue
		}
		role, _ := msg["role"].(string)
		content := contentToText(msg["content"])
		if content == "" && msg["tool_calls"] != nil {
			content = toolCallsToText(msg["tool_calls"])
		}
		switch role {
		case "system":
			sb.WriteString("[system] " + content + "\n\n")
		case "assistant":
			sb.WriteString("Assistant: " + content + "\n\n")
		case "tool", "function":
			name, _ := msg["name"].(string)
			sb.WriteString(fmt.Sprintf("Tool result (%s): %s\n\n", name, content))
		default:
			sb.WriteString(content + "\n\n")
		}
	}
	return strings.TrimSpace(sb.String())
}

// contentToText renders a message content value, which may be a plain string or
// a multimodal array.
func contentToText(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case []any:
		var parts []string
		for _, item := range t {
			if obj, ok := item.(map[string]any); ok {
				if txt, ok := obj["text"].(string); ok {
					parts = append(parts, txt)
					continue
				}
				if typ, ok := obj["type"].(string); ok {
					parts = append(parts, "["+typ+"]")
				}
			} else if s, ok := item.(string); ok {
				parts = append(parts, s)
			}
		}
		return strings.Join(parts, "\n")
	default:
		buf, _ := json.Marshal(v)
		return string(buf)
	}
}

// toolCallsToText renders prior assistant tool calls into the transcript so the
// model can see what it already asked for.
func toolCallsToText(v any) string {
	var sb strings.Builder
	if list, ok := v.([]any); ok {
		for _, item := range list {
			obj, ok := item.(map[string]any)
			if !ok {
				continue
			}
			fn, _ := obj["function"].(map[string]any)
			name, _ := fn["name"].(string)
			if name == "" {
				name, _ = obj["name"].(string)
				fn = obj
			}
			args := fn["arguments"]
			if s, ok := args.(string); ok {
				sb.WriteString(fmt.Sprintf("[called tool %s with %s]\n", name, s))
			} else {
				buf, _ := json.Marshal(args)
				sb.WriteString(fmt.Sprintf("[called tool %s with %s]\n", name, string(buf)))
			}
		}
	}
	return strings.TrimSpace(sb.String())
}

// toolPreamble teaches the model the tool protocol. Unlike ChatGPT web, GLM
// follows this reliably, so it stays short and concrete.
func toolPreamble(specs []chatgpt.ToolSpec) string {
	var sb strings.Builder
	sb.WriteString("[TOOL USE PROTOCOL]\n")
	sb.WriteString("You have tools. They are the ONLY way you can touch the filesystem or run commands.\n")
	sb.WriteString("To use one, reply with exactly one tag and nothing else:\n\n")
	sb.WriteString(`<tool_call>{"name": "<tool>", "arguments": { ... }}</tool_call>`)
	sb.WriteString("\n\nAvailable tools:\n")
	for _, spec := range specs {
		sb.WriteString("- " + spec.Name)
		if spec.Description != "" {
			sb.WriteString(": " + firstLine(spec.Description))
		}
		if args := describeParams(spec.Parameters); args != "" {
			sb.WriteString(" arguments: {" + args + "}")
		}
		sb.WriteString("\n")
	}
	sb.WriteString("\nRules:\n")
	sb.WriteString("1. Never answer a question about file contents from memory. Call the tool.\n")
	sb.WriteString("2. Emit ONE tool call per reply. The result arrives in the next turn.\n")
	sb.WriteString("3. Once a tool result is present, answer the user in plain prose and do not call again.\n")
	sb.WriteString("[END TOOL USE PROTOCOL]\n\n")
	return sb.String()
}

func firstLine(s string) string {
	if i := strings.IndexAny(s, "\n"); i != -1 {
		s = s[:i]
	}
	s = strings.TrimSpace(s)
	if len(s) > 200 {
		s = s[:200] + "..."
	}
	return s
}

// describeParams renders the argument names a tool accepts, preferring the
// caller's own schema spellings.
func describeParams(params map[string]any) string {
	props, ok := params["properties"].(map[string]any)
	if !ok {
		return ""
	}
	var required map[string]bool
	if list, ok := params["required"].([]any); ok {
		required = make(map[string]bool, len(list))
		for _, r := range list {
			if s, ok := r.(string); ok {
				required[s] = true
			}
		}
	}
	names := make([]string, 0, len(props))
	for name := range props {
		if required == nil || required[name] {
			names = append(names, fmt.Sprintf("%q: <value>", name))
		}
	}
	if len(names) == 0 {
		for name := range props {
			names = append(names, fmt.Sprintf("%q: <value>", name))
		}
	}
	return strings.Join(names, ", ")
}

func stringField(body map[string]any, key string) string {
	if s, ok := body[key].(string); ok {
		return s
	}
	return ""
}

// cancelOnClose ties the request context to the response body so the deadline
// cannot outlive the caller.
type cancelOnClose struct {
	io.ReadCloser
	cancel context.CancelFunc
}

func (c *cancelOnClose) Close() error {
	err := c.ReadCloser.Close()
	c.cancel()
	return err
}

// buildChatResponse renders the OpenAI-shaped reply.
func buildChatResponse(text, reasoning, model string, usage json.RawMessage, stream bool) *http.Response {
	id := fmt.Sprintf("chatcmpl-zai-%d", responseIDCounter.Add(1))
	created := time.Now().Unix()
	if stream {
		pr, pw := io.Pipe()
		go func() {
			defer pw.Close()
			writeStreamChunk(pw, id, model, created, map[string]any{"role": "assistant", "content": ""}, nil)
			for _, part := range chunkString(text, 400) {
				writeStreamChunk(pw, id, model, created, map[string]any{"content": part}, nil)
			}
			writeStreamChunk(pw, id, model, created, map[string]any{}, "stop")
			if len(usage) > 0 {
				_, _ = fmt.Fprintf(pw, "data: %s\n\n", mustJSON(map[string]any{
					"id": id, "object": "chat.completion.chunk", "created": created,
					"model": model, "choices": []any{}, "usage": json.RawMessage(usage),
				}))
			}
			_, _ = io.WriteString(pw, "data: [DONE]\n\n")
		}()
		resp := &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: pr}
		resp.Header.Set("Content-Type", "text/event-stream")
		resp.Header.Set("Cache-Control", "no-cache")
		return resp
	}

	obj := map[string]any{
		"id": id, "object": "chat.completion", "created": created, "model": model,
		"choices": []map[string]any{{
			"index":         0,
			"message":       map[string]any{"role": "assistant", "content": text},
			"finish_reason": "stop",
		}},
	}
	if reasoning != "" {
		obj["choices"].([]map[string]any)[0]["message"].(map[string]any)["reasoning_content"] = reasoning
	}
	if len(usage) > 0 {
		obj["usage"] = json.RawMessage(usage)
	}
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(mustJSON(obj))),
	}
	resp.Header.Set("Content-Type", "application/json")
	return resp
}

// buildToolCallResponse renders a chat.completion carrying a tool_calls array.
func buildToolCallResponse(call *chatgpt.ToolCall, model string, stream bool) *http.Response {
	id := fmt.Sprintf("chatcmpl-zai-%d", responseIDCounter.Add(1))
	created := time.Now().Unix()
	toolCall := map[string]any{
		"id":   fmt.Sprintf("call_zai%d", responseIDCounter.Add(1)),
		"type": "function",
		"function": map[string]any{
			"name":      call.Name,
			"arguments": mustJSON(call.Arguments),
		},
	}

	if !stream {
		obj := map[string]any{
			"id": id, "object": "chat.completion", "created": created, "model": model,
			"choices": []map[string]any{{
				"index":         0,
				"message":       map[string]any{"role": "assistant", "content": nil, "tool_calls": []any{toolCall}},
				"finish_reason": "tool_calls",
			}},
		}
		resp := &http.Response{
			StatusCode: http.StatusOK, Header: make(http.Header),
			Body: io.NopCloser(strings.NewReader(mustJSON(obj))),
		}
		resp.Header.Set("Content-Type", "application/json")
		return resp
	}

	pr, pw := io.Pipe()
	go func() {
		defer pw.Close()
		writeStreamChunk(pw, id, model, created, map[string]any{"role": "assistant", "content": nil}, nil)
		writeStreamChunk(pw, id, model, created, map[string]any{
			"tool_calls": []any{map[string]any{
				"index":    0,
				"id":       toolCall["id"],
				"type":     "function",
				"function": toolCall["function"],
			}},
		}, nil)
		writeStreamChunk(pw, id, model, created, map[string]any{}, "tool_calls")
		_, _ = io.WriteString(pw, "data: [DONE]\n\n")
	}()
	resp := &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: pr}
	resp.Header.Set("Content-Type", "text/event-stream")
	resp.Header.Set("Cache-Control", "no-cache")
	return resp
}

func writeStreamChunk(w io.Writer, id, model string, created int64, delta map[string]any, finish any) {
	choice := map[string]any{"index": 0, "delta": delta}
	if finish != nil {
		choice["finish_reason"] = finish
	}
	_, _ = fmt.Fprintf(w, "data: %s\n\n", mustJSON(map[string]any{
		"id": id, "object": "chat.completion.chunk", "created": created,
		"model": model, "choices": []any{choice},
	}))
}

// chunkString splits text into pieces so a streamed reply still arrives as
// multiple deltas rather than one large frame.
func chunkString(s string, size int) []string {
	if s == "" {
		return nil
	}
	var out []string
	for len(s) > size {
		out = append(out, s[:size])
		s = s[size:]
	}
	return append(out, s)
}

func mustJSON(v any) string {
	buf, _ := json.Marshal(v)
	return string(buf)
}

func debugEnabled() bool { return os.Getenv("ZAI_DEBUG") == "1" }
