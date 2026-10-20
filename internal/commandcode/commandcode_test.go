package commandcode

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestBuildBody(t *testing.T) {
	body := map[string]any{
		"model":  "commandcode/deepseek/deepseek-v4-flash",
		"stream": false,
		"messages": []any{
			map[string]any{"role": "system", "content": "be terse"},
			map[string]any{"role": "user", "content": "hi"},
			map[string]any{"role": "assistant", "content": "calling", "tool_calls": []any{
				map[string]any{"id": "call_1", "type": "function", "function": map[string]any{"name": "read_file", "arguments": `{"path":"a.go"}`}},
			}},
			map[string]any{"role": "tool", "tool_call_id": "call_1", "content": "package a"},
		},
		"tools": []any{map[string]any{"type": "function", "function": map[string]any{
			"name": "read_file", "description": "read", "parameters": map[string]any{"type": "object"},
		}}},
	}

	wire, err := BuildBody(body, "/tmp/project")
	if err != nil {
		t.Fatalf("BuildBody: %v", err)
	}

	if wire["permissionMode"] != "standard" {
		t.Errorf("permissionMode = %v", wire["permissionMode"])
	}
	config, _ := wire["config"].(map[string]any)
	if config["workingDir"] != "/tmp/project" || config["isGitRepo"] != false {
		t.Errorf("config = %v", config)
	}

	params, _ := wire["params"].(map[string]any)
	if params["model"] != "deepseek/deepseek-v4-flash" {
		t.Errorf("model = %v", params["model"])
	}
	if params["stream"] != true {
		t.Errorf("stream must be forced true, got %v", params["stream"])
	}
	if params["max_tokens"] != defaultMaxTokens {
		t.Errorf("max_tokens = %v", params["max_tokens"])
	}

	system, _ := params["system"].([]any)
	if len(system) != 1 || system[0].(map[string]any)["text"] != "be terse" {
		t.Errorf("system = %v", params["system"])
	}

	messages, _ := params["messages"].([]any)
	if len(messages) != 3 {
		t.Fatalf("want 3 wire messages (system inlined), got %d: %v", len(messages), messages)
	}

	toolResult := messages[2].(map[string]any)
	block := toolResult["content"].([]any)[0].(map[string]any)
	if toolResult["role"] != "tool" || block["toolName"] != "read_file" || block["toolCallId"] != "call_1" {
		t.Errorf("tool result block = %v", block)
	}
	if block["output"].(map[string]any)["value"] != "package a" {
		t.Errorf("tool output = %v", block["output"])
	}

	assistant := messages[1].(map[string]any)
	callBlock := assistant["content"].([]any)[1].(map[string]any)
	if callBlock["type"] != "tool-call" || callBlock["toolName"] != "read_file" {
		t.Errorf("tool-call block = %v", callBlock)
	}
	if callBlock["input"].(map[string]any)["path"] != "a.go" {
		t.Errorf("tool-call input = %v", callBlock["input"])
	}

	tools := params["tools"].([]any)
	if tools[0].(map[string]any)["name"] != "read_file" {
		t.Errorf("tools = %v", tools)
	}
}

func TestRelayStreamAndReply(t *testing.T) {
	upstream := strings.Join([]string{
		`{"type":"start"}`,
		`{"type":"reasoning-delta","text":"thinking"}`,
		`{"type":"text-delta","text":"hello "}`,
		`{"type":"text-delta","text":"world"}`,
		`{"type":"tool-call","toolCallId":"call_9","toolName":"read_file","input":{"path":"a.go"}}`,
		`{"type":"finish","finishReason":"tool-calls","rawFinishReason":"tool-calls","totalUsage":{"inputTokens":12,"outputTokens":7,"inputTokenDetails":{"cacheReadTokens":3}}}`,
	}, "\n")

	var sse bytes.Buffer
	reply, err := Relay(&sse, nil, strings.NewReader(upstream), "deepseek/deepseek-v4-flash")
	if err != nil {
		t.Fatalf("Relay: %v", err)
	}

	if reply.Text.String() != "hello world" {
		t.Errorf("text = %q", reply.Text.String())
	}
	if reply.Reasoning.String() != "thinking" {
		t.Errorf("reasoning = %q", reply.Reasoning.String())
	}
	if reply.FinishReason != "tool_calls" {
		t.Errorf("finish = %q", reply.FinishReason)
	}
	if reply.Usage == nil || reply.Usage.In != 12 || reply.Usage.Out != 7 || reply.Usage.Cached != 3 {
		t.Errorf("usage = %+v", reply.Usage)
	}
	if len(reply.ToolCalls) != 1 || reply.ToolCalls[0].Name != "read_file" || reply.ToolCalls[0].Args != `{"path":"a.go"}` {
		t.Errorf("tool calls = %+v", reply.ToolCalls)
	}

	out := sse.String()
	for _, want := range []string{`"object":"chat.completion.chunk"`, `"content":"hello "`, `"reasoning_content":"thinking"`, `"finish_reason":"tool_calls"`, `"cached_tokens":3`} {
		if !strings.Contains(out, want) {
			t.Errorf("SSE missing %s\n%s", want, out)
		}
	}

	nonStream, err := Relay(nil, nil, strings.NewReader(`{"type":"text-delta","text":"ok"}`+"\n"+`{"type":"finish","finishReason":"stop"}`), "m")
	if err != nil {
		t.Fatalf("Relay non-stream: %v", err)
	}
	payload := nonStream.ChatCompletion("m")
	choice := payload["choices"].([]any)[0].(map[string]any)
	if choice["message"].(map[string]any)["content"] != "ok" || choice["finish_reason"] != "stop" {
		t.Errorf("chat.completion = %v", payload)
	}
	if _, err := json.Marshal(payload); err != nil {
		t.Errorf("marshal reply: %v", err)
	}
}

func TestRelayStreamError(t *testing.T) {
	_, err := Relay(nil, nil, strings.NewReader(`{"type":"error","error":{"message":"boom","statusCode":429}}`), "m")
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("want stream error, got %v", err)
	}
}

func TestCleanModelID(t *testing.T) {
	for in, want := range map[string]string{
		"commandcode/deepseek/deepseek-v4-flash": "deepseek/deepseek-v4-flash",
		"cmc/zai-org/GLM-5.2":                    "zai-org/GLM-5.2",
		"zai-org/GLM-5.2":                        "zai-org/GLM-5.2",
	} {
		if got := CleanModelID(in); got != want {
			t.Errorf("CleanModelID(%q) = %q, want %q", in, got, want)
		}
	}
}

// Regression: the model-test path builds messages as []map[string]string, which
// used to reach the wire as "messages": null.
func TestBuildBodyTypedSliceMessages(t *testing.T) {
	wire, err := BuildBody(map[string]any{
		"model":    "deepseek/deepseek-v4-flash",
		"messages": []map[string]string{{"role": "user", "content": "ping"}},
	}, "/tmp/p")
	if err != nil {
		t.Fatalf("BuildBody: %v", err)
	}
	params := wire["params"].(map[string]any)
	messages, ok := params["messages"].([]any)
	if !ok || len(messages) != 1 {
		t.Fatalf("messages = %v", params["messages"])
	}
	if messages[0].(map[string]any)["role"] != "user" {
		t.Errorf("message = %v", messages[0])
	}
}
