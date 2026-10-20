package commandcode

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"
)

// Usage mirrors the totals Command Code reports on the finish event.
type Usage struct {
	In     int
	Out    int
	Cached int
}

// ToolCall is one completed tool call emitted by the upstream stream.
type ToolCall struct {
	ID   string
	Name string
	Args string
}

// Reply is everything the upstream stream produced, so non-streaming callers can
// build a single chat.completion out of the same event flow.
type Reply struct {
	Text         strings.Builder
	Reasoning    strings.Builder
	ToolCalls    []ToolCall
	FinishReason string
	Usage        *Usage
}

type streamEvent struct {
	Type            string `json:"type"`
	Text            string `json:"text"`
	ToolCallID      string `json:"toolCallId"`
	ToolName        string `json:"toolName"`
	Input           any    `json:"input"`
	FinishReason    string `json:"finishReason"`
	RawFinishReason string `json:"rawFinishReason"`
	TotalUsage      *struct {
		InputTokens      int `json:"inputTokens"`
		OutputTokens     int `json:"outputTokens"`
		InputTokenDetail struct {
			CacheReadTokens  int `json:"cacheReadTokens"`
			CacheWriteTokens int `json:"cacheWriteTokens"`
		} `json:"inputTokenDetails"`
	} `json:"totalUsage"`
	Error any `json:"error"`
}

// Relay consumes the upstream NDJSON event stream and, when dst is not nil, writes
// OpenAI chat.completion.chunk SSE frames to it. It always returns the accumulated
// reply so the caller can serve a non-streaming request from the same pass.
func Relay(dst io.Writer, flush func(), src io.Reader, model string) (*Reply, error) {
	rep := &Reply{}
	if model == "" {
		model = "commandcode"
	}

	id := "chatcmpl-" + strings.ReplaceAll(newUUID(), "-", "")[:24]
	created := time.Now().Unix()
	sentRole := false
	finished := false

	emit := func(delta map[string]any, finish any) {
		if dst == nil {
			return
		}
		chunk := map[string]any{
			"id":      id,
			"object":  "chat.completion.chunk",
			"created": created,
			"model":   model,
			"choices": []any{map[string]any{
				"index":         0,
				"delta":         delta,
				"finish_reason": finish,
			}},
		}
		data, err := json.Marshal(chunk)
		if err != nil {
			return
		}
		fmt.Fprintf(dst, "data: %s\n\n", data)
		if flush != nil {
			flush()
		}
	}

	emitDelta := func(delta map[string]any) {
		if !sentRole {
			delta["role"] = "assistant"
			sentRole = true
		}
		emit(delta, nil)
	}

	scanner := bufio.NewScanner(src)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		line = strings.TrimPrefix(line, "data:")
		line = strings.TrimSpace(line)
		if line == "" || line == "[DONE]" {
			continue
		}

		var ev streamEvent
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			continue
		}

		switch ev.Type {
		case "text-delta":
			if ev.Text == "" {
				break
			}
			rep.Text.WriteString(ev.Text)
			emitDelta(map[string]any{"content": ev.Text})

		case "reasoning-delta":
			if ev.Text == "" {
				break
			}
			rep.Reasoning.WriteString(ev.Text)
			emitDelta(map[string]any{"reasoning_content": ev.Text})

		case "tool-call":
			args, err := json.Marshal(ev.Input)
			if err != nil || string(args) == "null" {
				args = []byte("{}")
			}
			index := len(rep.ToolCalls)
			rep.ToolCalls = append(rep.ToolCalls, ToolCall{ID: ev.ToolCallID, Name: ev.ToolName, Args: string(args)})
			emitDelta(map[string]any{
				"tool_calls": []any{map[string]any{
					"index": index,
					"id":    ev.ToolCallID,
					"type":  "function",
					"function": map[string]any{
						"name":      ev.ToolName,
						"arguments": string(args),
					},
				}},
			})

		case "finish":
			rep.FinishReason = openAIFinishReason(ev.RawFinishReason, ev.FinishReason)
			if ev.TotalUsage != nil {
				rep.Usage = &Usage{
					In:     ev.TotalUsage.InputTokens,
					Out:    ev.TotalUsage.OutputTokens,
					Cached: ev.TotalUsage.InputTokenDetail.CacheReadTokens,
				}
			}
			if !sentRole {
				sentRole = true
			}
			emit(map[string]any{}, rep.FinishReason)
			if dst != nil && rep.Usage != nil {
				usageChunk := map[string]any{
					"id":      id,
					"object":  "chat.completion.chunk",
					"created": created,
					"model":   model,
					"choices": []any{},
					"usage": map[string]any{
						"prompt_tokens":         rep.Usage.In,
						"completion_tokens":     rep.Usage.Out,
						"total_tokens":          rep.Usage.In + rep.Usage.Out,
						"prompt_tokens_details": map[string]any{"cached_tokens": rep.Usage.Cached},
					},
				}
				if data, err := json.Marshal(usageChunk); err == nil {
					fmt.Fprintf(dst, "data: %s\n\n", data)
					if flush != nil {
						flush()
					}
				}
			}
			finished = true

		case "error":
			msg, _ := ev.Error.(string)
			if msg == "" {
				raw, _ := json.Marshal(ev.Error)
				msg = string(raw)
			}
			return rep, fmt.Errorf("commandcode stream error: %s", msg)
		}

		if finished {
			break
		}
	}

	if err := scanner.Err(); err != nil {
		return rep, err
	}

	if !finished && dst != nil {
		emit(map[string]any{}, "stop")
	}
	return rep, nil
}

func openAIFinishReason(raw, finish string) string {
	reason := strings.ToLower(raw)
	if reason == "" {
		reason = strings.ToLower(finish)
	}
	switch reason {
	case "tool-calls", "tool_calls", "tool_use":
		return "tool_calls"
	case "length", "max_tokens", "max-tokens":
		return "length"
	case "refusal", "content_filter", "content-filter":
		return "content_filter"
	default:
		return "stop"
	}
}

// ChatCompletion renders the accumulated reply as a non-streaming response.
func (r *Reply) ChatCompletion(model string) map[string]any {
	message := map[string]any{"role": "assistant", "content": r.Text.String()}
	if r.Reasoning.Len() > 0 {
		message["reasoning_content"] = r.Reasoning.String()
	}
	if len(r.ToolCalls) > 0 {
		calls := make([]any, 0, len(r.ToolCalls))
		for _, call := range r.ToolCalls {
			calls = append(calls, map[string]any{
				"id":   call.ID,
				"type": "function",
				"function": map[string]any{
					"name":      call.Name,
					"arguments": call.Args,
				},
			})
		}
		message["tool_calls"] = calls
	}

	finish := r.FinishReason
	if finish == "" {
		finish = "stop"
	}

	resp := map[string]any{
		"id":      "chatcmpl-" + strings.ReplaceAll(newUUID(), "-", "")[:24],
		"object":  "chat.completion",
		"created": time.Now().Unix(),
		"model":   model,
		"choices": []any{map[string]any{
			"index":         0,
			"message":       message,
			"finish_reason": finish,
		}},
	}

	if r.Usage != nil {
		resp["usage"] = map[string]any{
			"prompt_tokens":     r.Usage.In,
			"completion_tokens": r.Usage.Out,
			"total_tokens":      r.Usage.In + r.Usage.Out,
			"prompt_tokens_details": map[string]any{
				"cached_tokens": r.Usage.Cached,
			},
		}
	}
	return resp
}

// ErrorPayload is the OpenAI-shaped error object this router returns to clients.
func ErrorPayload(message string) map[string]any {
	return map[string]any{"error": map[string]any{"message": message, "type": "api_error"}}
}
