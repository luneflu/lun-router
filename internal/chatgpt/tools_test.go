package chatgpt

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestParseToolSpecs(t *testing.T) {
	body := map[string]any{
		"tools": []any{
			map[string]any{
				"type": "function",
				"function": map[string]any{
					"name":        "read",
					"description": "Read a file",
					"parameters": map[string]any{
						"type":       "object",
						"properties": map[string]any{"path": map[string]any{"type": "string"}},
					},
				},
			},
		},
	}
	specs := ParseToolSpecs(body)
	if len(specs) != 1 || specs[0].Name != "read" {
		t.Fatalf("unexpected specs: %+v", specs)
	}
	if ParseToolSpecs(map[string]any{}) != nil {
		t.Fatalf("expected nil specs when tools absent")
	}
}

func TestExtractToolCall(t *testing.T) {
	cases := []struct {
		reply    string
		wantName string
		wantPath string
	}{
		{`<tool_call>{"name":"read","arguments":{"path":"README.md"}}</tool_call>`, "read", "README.md"},
		{"Sure, let me look.\n<tool_call>{\"tool\":\"read\",\"parameters\":{\"path\":\"go.mod\"}}</tool_call>", "read", "go.mod"},
		{"```json\n{\"tool_call\":{\"name\":\"shell\",\"arguments\":{\"command\":\"ls\"}}}\n```", "shell", ""},
		{`{"name":"read","arguments":{"path":"main.go"}}`, "read", "main.go"},
	}
	for _, tc := range cases {
		call, clean := ExtractToolCall(tc.reply)
		if call == nil {
			t.Errorf("expected tool call from %q", tc.reply)
			continue
		}
		if call.Name != tc.wantName {
			t.Errorf("got name %q want %q", call.Name, tc.wantName)
		}
		if tc.wantPath != "" {
			if got, _ := call.Arguments["path"].(string); got != tc.wantPath {
				t.Errorf("got path %q want %q", got, tc.wantPath)
			}
		}
		if strings.Contains(clean, "<tool_call>") {
			t.Errorf("expected tool markup removed, got %q", clean)
		}
	}

	call, clean := ExtractToolCall("The README is a SvelteKit scaffold doc.")
	if call != nil {
		t.Errorf("expected no tool call, got %+v", call)
	}
	if clean == "" {
		t.Errorf("expected clean prose preserved")
	}
}

func TestExecuteToolRead(t *testing.T) {
	c := NewClient(Config{DataDir: t.TempDir()})
	c.cfg.WorkDir = "../../"

	out, err := c.ExecuteTool(t.Context(), &ToolCall{
		Name:      "read",
		Arguments: map[string]any{"path": "go.mod", "limit": float64(2)},
	})
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !strings.HasPrefix(out, "1: ") {
		t.Errorf("expected line-numbered output, got %q", out)
	}

	if _, err := c.ExecuteTool(t.Context(), &ToolCall{
		Name:      "read",
		Arguments: map[string]any{"path": "does-not-exist-xyz.txt"},
	}); err == nil {
		t.Errorf("expected error for missing file")
	}
}

func TestExecuteToolShellGated(t *testing.T) {
	c := NewClient(Config{DataDir: t.TempDir()})
	c.cfg.WorkDir = "../../"

	_, err := c.ExecuteTool(t.Context(), &ToolCall{
		Name:      "shell",
		Arguments: map[string]any{"command": "echo hi"},
	})
	if err == nil {
		t.Fatalf("expected shell to be disabled by default")
	}
	if !strings.Contains(err.Error(), "disabled") {
		t.Errorf("expected disabled error, got %v", err)
	}

	c.cfg.EnableShell = true
	out, err := c.ExecuteTool(t.Context(), &ToolCall{
		Name:      "shell",
		Arguments: map[string]any{"command": "echo hi"},
	})
	if err != nil {
		t.Fatalf("shell: %v", err)
	}
	if strings.TrimSpace(out) != "hi" {
		t.Errorf("got %q want hi", out)
	}
}

func TestExecuteToolGlobAndGrep(t *testing.T) {
	c := NewClient(Config{DataDir: t.TempDir()})
	c.cfg.WorkDir = "../../"

	globOut, err := c.ExecuteTool(t.Context(), &ToolCall{
		Name:      "glob",
		Arguments: map[string]any{"pattern": "go.*"},
	})
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	if !strings.Contains(globOut, "go.mod") {
		t.Errorf("expected go.mod in glob output, got %q", globOut)
	}

	grepOut, err := c.ExecuteTool(t.Context(), &ToolCall{
		Name:      "grep",
		Arguments: map[string]any{"pattern": "module lun-router", "path": "go.mod"},
	})
	if err != nil {
		t.Fatalf("grep: %v", err)
	}
	if !strings.Contains(grepOut, "module lun-router") {
		t.Errorf("expected grep hit, got %q", grepOut)
	}
}

func TestExtractFilePath(t *testing.T) {
	bases := []string{"../../"}

	if got := ExtractFilePath("Tell me what is @README.md is about", bases); got != "README.md" {
		t.Errorf("at-mention: got %q", got)
	}
	if got := ExtractFilePath("summarize `go.mod` for me", bases); got != "go.mod" {
		t.Errorf("backtick path: got %q", got)
	}
	if got := ExtractFilePath("read @missing-file-xyz.md please", bases); got != "" {
		t.Errorf("expected no path for missing file, got %q", got)
	}
	if got := ExtractFilePath("hello there", bases); got != "" {
		t.Errorf("expected no path, got %q", got)
	}
}

func TestCandidateWorkDirsDetectsHints(t *testing.T) {
	root, err := filepath.Abs("../../")
	if err != nil {
		t.Fatal(err)
	}
	c := NewClient(Config{DataDir: t.TempDir()})

	prompt := "<env>\nWorking directory: " + root + "\n</env>\nhello"
	bases := c.CandidateWorkDirs(prompt, nil)
	if len(bases) == 0 || bases[0] != root {
		t.Fatalf("expected first base %q, got %v", root, bases)
	}

	// Absolute file path in the prompt should resolve to its project root.
	prompt2 := "look at " + filepath.Join(root, "internal", "chatgpt", "client.go")
	bases2 := c.CandidateWorkDirs(prompt2, nil)
	found := false
	for _, b := range bases2 {
		if b == root {
			found = true
		}
	}
	if !found {
		t.Errorf("expected project root %q among %v", root, bases2)
	}
}

func TestCandidateWorkDirsFromBody(t *testing.T) {
	root, _ := filepath.Abs("../../")
	c := NewClient(Config{DataDir: t.TempDir()})
	body := map[string]any{
		"metadata": map[string]any{"cwd": root},
	}
	bases := c.CandidateWorkDirs("hi", body)
	if len(bases) == 0 || bases[0] != root {
		t.Fatalf("expected cwd from body metadata, got %v", bases)
	}
}

func TestExtractAbsolutePathsSkipsURLs(t *testing.T) {
	paths := extractAbsolutePaths("see https://example.com/foo/bar and /usr/local/share/thing")
	for _, p := range paths {
		if strings.Contains(p, "example.com") {
			t.Errorf("URL parsed as path: %q", p)
		}
	}
	found := false
	for _, p := range paths {
		if p == "/usr/local/share/thing" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected real path in %v", paths)
	}
}

func TestExtractShellCommand(t *testing.T) {
	refusal := "Cannot run `git status`: no shell execution tool is available."

	if got := ExtractShellCommand(refusal+"\n\n```sh\ngit status\n```", "please run git status"); got != "git status" {
		t.Errorf("fenced block: got %q", got)
	}
	if got := ExtractShellCommand(refusal, "Run the shell command `git log --oneline -3` please"); got != "git log --oneline -3" {
		t.Errorf("user fallback: got %q", got)
	}
	if got := ExtractShellCommand(refusal, "what is this project?"); got != "" {
		t.Errorf("expected no command without run intent, got %q", got)
	}
	if got := ExtractShellCommand("All good, no tools needed.", "run `ls`"); got != "" {
		t.Errorf("expected no fallback without refusal, got %q", got)
	}
	if got := ExtractShellCommand("I can't do that", "the word `hello` appears"); got != "" {
		t.Errorf("expected non-command backticks ignored, got %q", got)
	}

	// A normal answer that merely quotes commands must never be executed.
	answer := "Here is how to start it:\n\n```bash\nnpm run dev\n```\n"
	if got := ExtractShellCommand(answer, "run npm run dev"); got != "" {
		t.Errorf("expected dev-server command refused, got %q", got)
	}
	if got := ExtractShellCommand(answer, "what does the readme say?"); got != "" {
		t.Errorf("expected no execution without refusal+intent, got %q", got)
	}
}

func TestIsLongRunningCommand(t *testing.T) {
	long := []string{"npm run dev", "pnpm dev", "npm start", "yarn preview", "vite", "nodemon x", "tail -f log"}
	for _, c := range long {
		if !isLongRunningCommand(c) {
			t.Errorf("expected %q to be long-running", c)
		}
	}
	short := []string{"git log --oneline -3", "go test ./...", "ls -la", "npm run build", "go build -o x ."}
	for _, c := range short {
		if isLongRunningCommand(c) {
			t.Errorf("did not expect %q to be long-running", c)
		}
	}
}

func TestInferToolCall(t *testing.T) {
	bases := []string{"../../"}
	specs := []ToolSpec{
		{Name: "read", Parameters: map[string]any{
			"type":       "object",
			"properties": map[string]any{"filePath": map[string]any{"type": "string"}},
			"required":   []any{"filePath"},
		}},
		{Name: "bash", Parameters: map[string]any{
			"type":       "object",
			"properties": map[string]any{"command": map[string]any{"type": "string"}},
			"required":   []any{"command"},
		}},
	}

	call := InferToolCall("Read the file README.md and summarize it", specs, bases)
	if call == nil || call.Name != "read" {
		t.Fatalf("expected inferred read call, got %+v", call)
	}
	// Must use the schema's own argument name, not a guessed one.
	if p, _ := call.Arguments["filePath"].(string); p != "README.md" {
		t.Errorf("expected filePath=README.md, got %+v", call.Arguments)
	}
	if _, wrong := call.Arguments["path"]; wrong {
		t.Errorf("must not emit a guessed 'path' key: %+v", call.Arguments)
	}

	call = InferToolCall("Tell me what @README.md is about", specs, bases)
	if call == nil || call.Name != "read" {
		t.Fatalf("expected inferred read call for @mention, got %+v", call)
	}

	call = InferToolCall("Run `git log --oneline -3` for me", specs, bases)
	if call == nil || call.Name != "bash" {
		t.Fatalf("expected inferred bash call, got %+v", call)
	}
	if c, _ := call.Arguments["command"].(string); c != "git log --oneline -3" {
		t.Errorf("expected command arg, got %+v", call.Arguments)
	}

	if call := InferToolCall("hello, how are you?", specs, bases); call != nil {
		t.Errorf("expected no inference, got %+v", call)
	}
}

func TestSpecParamName(t *testing.T) {
	spec := ToolSpec{Name: "read", Parameters: map[string]any{
		"type":       "object",
		"properties": map[string]any{"filePath": map[string]any{"type": "string"}, "offset": map[string]any{"type": "integer"}},
		"required":   []any{"filePath"},
	}}
	if got := specParamName(spec, "filePath", "path", "filename"); got != "filePath" {
		t.Errorf("got %q", got)
	}
	empty := ToolSpec{Name: "x"}
	if got := specParamName(empty, "command", "cmd"); got != "command" {
		t.Errorf("fallback: got %q", got)
	}
}

func TestPassthroughDefaultsEnabled(t *testing.T) {
	c := NewClient(Config{DataDir: t.TempDir()})
	t.Setenv("CHATGPT_INTERNAL_TOOLS", "")
	if !c.PassthroughTools() {
		t.Fatalf("expected passthrough enabled by default")
	}
	if err := c.SetPassthroughTools(false); err != nil {
		t.Fatal(err)
	}
	if c.PassthroughTools() {
		t.Fatalf("expected passthrough disabled after toggle")
	}
}

func TestHasToolNamed(t *testing.T) {
	specs := []ToolSpec{{Name: "Read"}, {Name: "run_command"}}
	if !hasToolNamed(specs, "read") {
		t.Errorf("expected case-insensitive match")
	}
	if !hasToolNamed(specs, "shell", "run_command") {
		t.Errorf("expected alias match")
	}
	if hasToolNamed(specs, "shell", "bash") {
		t.Errorf("expected no match")
	}
}

func TestContentToText(t *testing.T) {
	if got := contentToText("plain"); got != "plain" {
		t.Errorf("string: %q", got)
	}
	arr := []any{
		map[string]any{"type": "text", "text": "hello"},
		map[string]any{"type": "text", "text": "world"},
	}
	if got := contentToText(arr); got != "hello\nworld" {
		t.Errorf("parts: %q", got)
	}
	if got := contentToText(nil); got != "" {
		t.Errorf("nil: %q", got)
	}
	img := []any{map[string]any{"type": "image_url", "image_url": map[string]any{"url": "x"}}}
	if got := contentToText(img); got != "[image_url]" {
		t.Errorf("image: %q", got)
	}
}

func TestToolCallsToText(t *testing.T) {
	calls := []any{
		map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":      "read",
				"arguments": `{"path":"README.md"}`,
			},
		},
	}
	got := toolCallsToText(calls)
	if !strings.Contains(got, "read") || !strings.Contains(got, "README.md") {
		t.Errorf("got %q", got)
	}
}

func TestBuildPromptKeepsAgentHistory(t *testing.T) {
	body := map[string]any{
		"messages": []any{
			map[string]any{"role": "system", "content": "You are an agent."},
			map[string]any{"role": "user", "content": "read the file"},
			map[string]any{
				"role":    "assistant",
				"content": "I will read it",
				"tool_calls": []any{
					map[string]any{
						"type":     "function",
						"function": map[string]any{"name": "read", "arguments": `{"path":"a.txt"}`},
					},
				},
			},
			map[string]any{"role": "tool", "name": "read", "content": "file contents here"},
			map[string]any{"role": "user", "content": []any{map[string]any{"type": "text", "text": "now summarize"}}},
		},
	}
	p := buildPrompt(body)
	for _, want := range []string{
		"System instruction: You are an agent.",
		"User: read the file",
		"called tool read",
		"Tool result (read): file contents here",
		"User: now summarize",
		"Assistant:",
	} {
		if !strings.Contains(p, want) {
			t.Errorf("missing %q in:\n%s", want, p)
		}
	}
}

func TestToolPreambleListsTools(t *testing.T) {
	p := toolPreamble([]ToolSpec{{Name: "read", Description: "Read a file\nmore"}})
	if !strings.Contains(p, "- read: Read a file") {
		t.Errorf("expected tool listed, got %q", p)
	}
	if !strings.Contains(p, "<tool_call>") {
		t.Errorf("expected protocol instructions")
	}
	if strings.Contains(p, "more") {
		t.Errorf("expected description folded to first line")
	}
}

func TestResolvePathStripsAt(t *testing.T) {
	got := resolvePath([]string{"/base"}, map[string]any{"path": "@README.md"})
	if got != "/base/README.md" {
		t.Errorf("got %q", got)
	}
	if got := resolvePath([]string{"/base"}, map[string]any{}); got != "/base" {
		t.Errorf("expected workdir default, got %q", got)
	}
}

func TestTruncateOutput(t *testing.T) {
	big := strings.Repeat("x", maxToolOutput+10)
	out := truncateOutput(big)
	if !strings.Contains(out, "truncated") {
		t.Errorf("expected truncation marker")
	}
}

func TestNormalizeToolCall(t *testing.T) {
	// Pi-style schemas: bash + read with filePath, no "shell" tool at all.
	specs := []ToolSpec{
		{Name: "read", Parameters: map[string]any{
			"type":       "object",
			"properties": map[string]any{"filePath": map[string]any{"type": "string"}},
			"required":   []any{"filePath"},
		}},
		{Name: "bash", Parameters: map[string]any{
			"type":       "object",
			"properties": map[string]any{"command": map[string]any{"type": "string"}, "timeout": map[string]any{"type": "integer"}},
			"required":   []any{"command"},
		}},
	}

	got := NormalizeToolCall(specs, &ToolCall{Name: "shell", Arguments: map[string]any{"command": "ls"}})
	if got.Name != "bash" {
		t.Errorf("expected shell remapped to bash, got %q", got.Name)
	}
	if c, _ := got.Arguments["command"].(string); c != "ls" {
		t.Errorf("command lost: %+v", got.Arguments)
	}

	got = NormalizeToolCall(specs, &ToolCall{Name: "read", Arguments: map[string]any{"path": "README.md"}})
	if got.Name != "read" {
		t.Errorf("name: %q", got.Name)
	}
	if p, _ := got.Arguments["filePath"].(string); p != "README.md" {
		t.Errorf("expected path remapped to filePath, got %+v", got.Arguments)
	}
	if _, bad := got.Arguments["path"]; bad {
		t.Errorf("stale key kept: %+v", got.Arguments)
	}

	// An unknown tool is passed through untouched so the client can report it.
	unknown := &ToolCall{Name: "totally_unknown", Arguments: map[string]any{"x": 1}}
	if got := NormalizeToolCall(specs, unknown); got.Name != "totally_unknown" {
		t.Errorf("unknown tool must pass through, got %q", got.Name)
	}
}

func TestToolPreambleUsesClientToolNames(t *testing.T) {
	specs := []ToolSpec{
		{Name: "read", Parameters: map[string]any{"type": "object", "properties": map[string]any{"filePath": map[string]any{"type": "string"}}}},
		{Name: "bash", Parameters: map[string]any{"type": "object", "properties": map[string]any{"command": map[string]any{"type": "string"}}}},
	}
	p := toolPreamble(specs)
	if strings.Contains(p, `"name": "shell"`) {
		t.Errorf("preamble must not advertise a tool the client lacks:\n%s", p)
	}
	if !strings.Contains(p, `"name": "bash"`) {
		t.Errorf("preamble must use the client's tool name 'bash':\n%s", p)
	}
	if !strings.Contains(p, `"filePath"`) {
		t.Errorf("preamble must use the client's argument name 'filePath':\n%s", p)
	}
}

func TestWantsToolUse(t *testing.T) {
	if !wantsToolUse("continue inspecting the package", "Sure, here is a summary.") {
		t.Errorf("action intent must trigger a tool push")
	}
	if !wantsToolUse("hello", "I have no filesystem tool available in this session.") {
		t.Errorf("refusal must trigger a tool push")
	}
	if wantsToolUse("what is 2+2?", "Four.") {
		t.Errorf("plain chat must not be forced into a tool call")
	}
}

func TestExtractExistingPath(t *testing.T) {
	bases := []string{"../../"}

	if got, isDir := ExtractExistingPath("continue inspecting internal/antigravity", bases); got != "internal/antigravity" || !isDir {
		t.Errorf("got %q dir=%v, want internal/antigravity dir=true", got, isDir)
	}
	if got, isDir := ExtractExistingPath("look at internal/antigravity/store.go", bases); got != "internal/antigravity/store.go" || isDir {
		t.Errorf("got %q dir=%v, want file", got, isDir)
	}
	if got, _ := ExtractExistingPath("nothing to see here", bases); got != "" {
		t.Errorf("expected no path, got %q", got)
	}
}

func TestInferProgressCall(t *testing.T) {
	bases := []string{"../../"}
	specs := []ToolSpec{
		{Name: "read", Parameters: map[string]any{"type": "object", "properties": map[string]any{"filePath": map[string]any{"type": "string"}}}},
		{Name: "bash", Parameters: map[string]any{"type": "object", "properties": map[string]any{"command": map[string]any{"type": "string"}}}},
	}

	// Directory with no list tool: fall back to a quoted shell listing.
	call := InferProgressCall("continue inspecting internal/antigravity", specs, bases)
	if call == nil || call.Name != "bash" {
		t.Fatalf("expected bash listing fallback, got %+v", call)
	}
	if c, _ := call.Arguments["command"].(string); c != "ls -la 'internal/antigravity'" {
		t.Errorf("unexpected command: %v", call.Arguments)
	}

	// File: read it.
	call = InferProgressCall("inspect internal/antigravity/store.go please", specs, bases)
	if call == nil || call.Name != "read" {
		t.Fatalf("expected read fallback, got %+v", call)
	}
	if p, _ := call.Arguments["filePath"].(string); p != "internal/antigravity/store.go" {
		t.Errorf("filePath: %v", call.Arguments)
	}

	// Nothing that exists on disk: no guessing.
	if call := InferProgressCall("inspect the mystery module", specs, bases); call != nil {
		t.Errorf("must not invent a target, got %+v", call)
	}
}
