package chatgpt

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// ToolSpec is a normalized view of an OpenAI-style function tool definition.
type ToolSpec struct {
	Name        string
	Description string
	Parameters  map[string]any
}

// ToolCall is a single tool invocation requested by the model.
type ToolCall struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

var (
	toolCallTagRe = regexp.MustCompile(`(?s)<tool_call>\s*(\{.*?\})\s*</tool_call>`)
	toolCallFence = regexp.MustCompile("(?s)```(?:json)?\\s*(\\{\\s*\"(?:name|tool|tool_call)\"\\s*:.*?)\\s*```")
)

// ParseToolSpecs extracts function tool definitions from an OpenAI chat request body.
func ParseToolSpecs(body map[string]any) []ToolSpec {
	raw, ok := body["tools"].([]any)
	if !ok || len(raw) == 0 {
		return nil
	}
	var specs []ToolSpec
	for _, item := range raw {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		// OpenAI chat shape: {"type":"function","function":{...}}
		var fn map[string]any
		if f, ok := m["function"].(map[string]any); ok {
			fn = f
		} else if _, hasName := m["name"]; hasName {
			// Responses-style flat shape.
			fn = m
		} else {
			continue
		}
		name, _ := fn["name"].(string)
		if name == "" {
			continue
		}
		desc, _ := fn["description"].(string)
		params, _ := fn["parameters"].(map[string]any)
		specs = append(specs, ToolSpec{Name: name, Description: desc, Parameters: params})
	}
	return specs
}

// toolPreamble describes the available tools and the required reply protocol.
// ChatGPT web cannot emit native tool_calls, so the model is asked to wrap a
// single JSON invocation in a <tool_call> tag when it needs a tool.
func toolPreamble(specs []ToolSpec) string {
	var sb strings.Builder
	sb.WriteString("[TOOL USE PROTOCOL - READ THIS FIRST]\n")
	sb.WriteString("You ARE equipped with the tools listed below. The API host executes them for you on a real machine.\n")
	sb.WriteString("They are NOT unavailable. Never reply that you lack a tool, lack file access, lack shell access, or lack network: call the tool instead.\n")
	sb.WriteString("Writing a command or file path inside a code block does NOT run it. Only a <tool_call> tag causes execution.\n\n")
	sb.WriteString("To call a tool, output EXACTLY this and nothing else:\n")
	sb.WriteString("<tool_call>{\"name\": \"<tool>\", \"arguments\": { ... }}</tool_call>\n\n")
	sb.WriteString("Rules:\n")
	sb.WriteString("1. One tool call per reply.\n")
	sb.WriteString("2. Whenever the user refers to a file, directory, repository, command, or test, you MUST emit a tool call before answering.\n")
	sb.WriteString("3. After the host returns a result, either call another tool or reply with the final answer as plain prose.\n")
	sb.WriteString("4. Never emit <tool_call> in a final answer.\n")
	if hasToolNamed(specs, "write", "create_file", "write_file", "save") ||
		hasToolNamed(specs, "edit", "str_replace", "replace", "apply_patch", "patch", "modify_file") {
		sb.WriteString("5. FILE MODIFICATION: when the user asks you to create, update, add to, fix, or edit a file, you MUST emit a tool call for the write/edit tool carrying the COMPLETE final file contents (not a diff, not a snippet).\n")
		sb.WriteString("   - Never reply that the file \"already documents this\", never print the new contents as prose instead of writing, and never say you cannot modify files.\n")
		sb.WriteString("   - Write the whole file, even for a small addition, so the host can save it verbatim.\n")
		sb.WriteString("   - If you believe the file needs no change, say so plainly; do not invent a change.\n")
		sb.WriteString("   - If for any reason you cannot emit the tool call, output the COMPLETE new file contents in a fenced block whose info string is `file:<path>`, for example:\n")
		sb.WriteString("     ```file:README.md\n     <complete new contents>\n     ```\n")
		sb.WriteString("     The host performs the write from that block, so never put the new contents anywhere else.\n")
		sb.WriteString("   - Summarise what changed only AFTER the host confirms the write succeeded.\n")
	}
	sb.WriteString("\nAvailable tools:\n")
	for _, s := range specs {
		sb.WriteString("- ")
		sb.WriteString(s.Name)
		if s.Description != "" {
			sb.WriteString(": ")
			sb.WriteString(firstLine(s.Description))
		}
		sb.WriteString("\n")
		if len(s.Parameters) > 0 {
			if b, err := json.Marshal(s.Parameters); err == nil {
				fmt.Fprintf(&sb, "  parameters: %s\n", string(b))
			}
		}
	}
	sb.WriteString("\nWorked examples (copy this style exactly):\n")
	// Examples MUST use the caller's own tool and argument names. Hardcoding them
	// made the model emit calls like {"name":"shell"} for a client that only
	// advertises "bash", and the client then failed with an unresolvable tool.
	if spec, ok := specForName(specs, "read", "cat", "view", "open", "read_file", "readfile"); ok {
		pk := specParamName(spec, "filePath", "file_path", "path", "filename", "file")
		sb.WriteString("User: what is in README.md?\n")
		fmt.Fprintf(&sb, "You: <tool_call>{\"name\": \"%s\", \"arguments\": {\"%s\": \"README.md\"}}</tool_call>\n", spec.Name, pk)
	}
	if spec, ok := specForName(specs, "shell", "bash", "run", "run_command", "exec", "terminal"); ok {
		ck := specParamName(spec, "command", "cmd", "script", "input")
		sb.WriteString("User: run git status\n")
		fmt.Fprintf(&sb, "You: <tool_call>{\"name\": \"%s\", \"arguments\": {\"%s\": \"git status\"}}</tool_call>\n", spec.Name, ck)
	}
	if spec, ok := specForName(specs, "write", "create_file", "write_file", "save"); ok {
		pk := specParamName(spec, "filePath", "file_path", "path", "filename", "file")
		ck := specParamName(spec, "content", "text", "contents", "fileText", "data")
		sb.WriteString("User: add a usage section to README.md\n")
		fmt.Fprintf(&sb, "You: <tool_call>{\"name\": \"%s\", \"arguments\": {\"%s\": \"README.md\", \"%s\": \"<the COMPLETE new file contents here>\"}}</tool_call>\n", spec.Name, pk, ck)
	}
	if spec, ok := specForName(specs, "edit", "str_replace", "replace", "apply_patch", "patch", "modify_file"); ok {
		pk := specParamName(spec, "filePath", "file_path", "path", "filename", "file")
		ok2 := specParamName(spec, "oldString", "old_string", "oldText", "old", "find")
		nk := specParamName(spec, "newString", "new_string", "newText", "new", "replace")
		sb.WriteString("User: rename the title in README.md\n")
		fmt.Fprintf(&sb, "You: <tool_call>{\"name\": \"%s\", \"arguments\": {\"%s\": \"README.md\", \"%s\": \"# sv\", \"%s\": \"# lun-router\"}}</tool_call>\n", spec.Name, pk, ok2, nk)
	}
	if spec, ok := specForName(specs, "grep", "search", "search_files", "ripgrep", "rg"); ok {
		qk := specParamName(spec, "pattern", "query", "regex", "text")
		sb.WriteString("User: find where the port is configured\n")
		fmt.Fprintf(&sb, "You: <tool_call>{\"name\": \"%s\", \"arguments\": {\"%s\": \"listen\"}}</tool_call>\n", spec.Name, qk)
	}
	if spec, ok := specForName(specs, "glob", "find", "find_files", "list", "ls", "list_dir"); ok {
		pk := specParamName(spec, "pattern", "glob", "path", "query")
		sb.WriteString("User: list every Go file\n")
		fmt.Fprintf(&sb, "You: <tool_call>{\"name\": \"%s\", \"arguments\": {\"%s\": \"**/*.go\"}}</tool_call>\n", spec.Name, pk)
	}
	sb.WriteString("WRONG (never do this): \"I cannot run commands.\" / \"the file already documents this.\" / \"here is the updated file:\" followed by a code block.\n")
	sb.WriteString("The host has already granted these tools; refuse only if the tool list above does not contain a suitable tool.\n")
	sb.WriteString("[END TOOL USE PROTOCOL]\n\n")
	return sb.String()
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return strings.TrimSpace(s[:i])
	}
	return strings.TrimSpace(s)
}

var (
	fencedBlockRe  = regexp.MustCompile("(?s)```(?:sh|bash|shell|zsh|console|terminal)?[ \t]*\r?\n(.*?)```")
	inlineTickRe   = regexp.MustCompile("`([^`\n]{2,200})`")
	atMentionRe    = regexp.MustCompile(`@([A-Za-z0-9_./\-]+\.[A-Za-z0-9_]+)`)
	bareFileRe     = regexp.MustCompile(`(?i)(?:^|[\s"'\x60(:])([A-Za-z0-9_][A-Za-z0-9_./\-]*\.(?:md|mod|sum|ts|tsx|js|jsx|mjs|cjs|json|jsonc|yaml|yml|toml|ini|cfg|txt|sh|bash|zsh|py|rs|java|kt|kts|php|rb|c|h|cpp|hpp|cc|cs|swift|sql|html|htm|css|scss|sass|less|svelte|vue|lock|env|conf|xml|gradle|properties|proto|graphql|tf|hcl))`)
	barePathRe     = regexp.MustCompile(`(?:^|[\s"'\x60(])([A-Za-z0-9_][A-Za-z0-9_.\-]*(?:/[A-Za-z0-9_.\-]+)+)`)
	knownFileRe    = regexp.MustCompile(`(?i)(?:^|[\s"'\x60(])(readme|license|licence|changelog|contributing|authors|notice|makefile|dockerfile|codeowners|go\.mod|procfile|justfile|rakefile|gemfile|brewfile)\b`)
	shellRefusalRe = regexp.MustCompile(`(?i)(cannot|can not|can't|unable to|no|without|lack[s]?|not)\s+(run|execute|executing|shell|command|access)`)
	runIntentRe    = regexp.MustCompile(`(?i)\b(run|running|execute|executing|shell|command|terminal|bash)\b`)
	commandWordRe  = regexp.MustCompile(`(?i)^\s*(sudo\s+)?(git|ls|cat|head|tail|npm|pnpm|yarn|bun|node|npx|go|make|python3?|pip3?|curl|wget|grep|rg|find|docker|compose|cargo|rustc|sed|awk|echo|ps|pwd|mkdir|rm|cp|mv|touch|chmod|chown|tar|zip|unzip|ssh|scp|rsync|env|export|bash|sh|zsh|kubectl|helm|terraform|aws|gh|jq|yq|sort|uniq|wc|diff|patch|test|which|whoami|date|df|du|kill|systemctl|brew|apt|apt-get|dnf|pacman)\b`)
)

// ExtractFilePath finds a file the user referenced with @path, with backticks,
// or as a bare filename with a recognised extension, and that exists on disk.
// Each candidate is tried against the ordered base-directory list.
func ExtractFilePath(userPrompt string, bases []string) string {
	candidates := []string{}
	for _, m := range atMentionRe.FindAllStringSubmatch(userPrompt, -1) {
		candidates = append(candidates, m[1])
	}
	for _, m := range inlineTickRe.FindAllStringSubmatch(userPrompt, -1) {
		t := strings.TrimSpace(m[1])
		if strings.ContainsAny(t, "/.") && !strings.ContainsAny(t, " \n") {
			candidates = append(candidates, strings.TrimPrefix(t, "@"))
		}
	}
	for _, m := range bareFileRe.FindAllStringSubmatch(userPrompt, -1) {
		candidates = append(candidates, strings.TrimSpace(m[1]))
	}
	for _, m := range knownFileRe.FindAllStringSubmatch(userPrompt, -1) {
		name := strings.TrimSpace(m[1])
		base := strings.ToUpper(name[:1]) + strings.ToLower(name[1:])
		// Try the literal spelling and canonical names, with and without the
		// extensions these files commonly carry.
		for _, variant := range []string{name, strings.ToUpper(name), base} {
			candidates = append(candidates, variant,
				variant+".md", variant+".MD", variant+".rst", variant+".txt")
		}
	}
	for _, cand := range candidates {
		p, base := resolveExistingPathBase(bases, cand)
		if p == "" {
			continue
		}
		if info, err := os.Stat(p); err == nil && !info.IsDir() {
			// Return the path as it exists on disk so case-sensitive filesystems
			// receive README.md rather than the user's lowercase spelling. On a
			// case-insensitive filesystem Stat alone cannot tell us the true
			// casing, so resolve each segment against the directory listing.
			if base != "" {
				if rel, err := filepath.Rel(base, p); err == nil && !strings.HasPrefix(rel, "..") {
					return realCasePath(base, rel)
				}
			}
			return filepath.Base(p)
		}
	}
	return ""
}

// realCasePath rewrites rel so each segment matches the true on-disk entry name.
func realCasePath(base, rel string) string {
	segments := strings.Split(filepath.ToSlash(rel), "/")
	cur := base
	out := make([]string, 0, len(segments))
	for _, seg := range segments {
		entries, err := os.ReadDir(cur)
		if err != nil {
			out = append(out, seg)
			cur = filepath.Join(cur, seg)
			continue
		}
		name := seg
		for _, e := range entries {
			if strings.EqualFold(e.Name(), seg) {
				name = e.Name()
				break
			}
		}
		out = append(out, name)
		cur = filepath.Join(cur, name)
	}
	return strings.Join(out, "/")
}

// ExtractFileBlock pulls a path and full file contents out of a `file:<path>`
// fenced block. The body is scanned to the LAST fence in the message because the
// file contents themselves usually contain nested fenced code blocks.
func ExtractFileBlock(reply string) (string, string) {
	const opener = "```file:"
	idx := strings.Index(reply, opener)
	if idx < 0 {
		return "", ""
	}
	rest := reply[idx+len(opener):]
	nl := strings.IndexByte(rest, '\n')
	if nl < 0 {
		return "", ""
	}
	path := strings.Trim(strings.TrimSpace(rest[:nl]), "\"'`")
	body := rest[nl+1:]

	// Remove the closing fence: the last triple-backtick in the remainder.
	if last := strings.LastIndex(body, "```"); last >= 0 {
		body = body[:last]
	}
	body = strings.TrimPrefix(body, "\r")
	body = strings.TrimPrefix(body, "\n")

	if path == "" || strings.TrimSpace(body) == "" {
		return "", ""
	}
	return path, body
}

// toolCallTarget returns the path-like argument of a call, for dedup comparison.
func toolCallTarget(args map[string]any) string {
	for _, key := range []string{"filePath", "file_path", "path", "filename", "file"} {
		if v, ok := args[key].(string); ok && v != "" {
			return strings.TrimPrefix(v, "@")
		}
	}
	return ""
}

// ExecutedToolCalls extracts the tool calls the agent has already made, read from
// the assistant messages of the incoming conversation. Used to avoid reissuing a
// call the client already ran, which previously caused an infinite read loop.
func ExecutedToolCalls(body map[string]any) []ToolCall {
	raw := messageList(body)
	var out []ToolCall
	for _, m := range raw {
		mm, ok := m.(map[string]any)
		if !ok {
			continue
		}
		role, _ := mm["role"].(string)
		if role != "assistant" {
			continue
		}
		list, ok := mm["tool_calls"].([]any)
		if !ok {
			continue
		}
		for _, item := range list {
			im, ok := item.(map[string]any)
			if !ok {
				continue
			}
			fn, ok := im["function"].(map[string]any)
			if !ok {
				continue
			}
			name, _ := fn["name"].(string)
			if name == "" {
				continue
			}
			args := map[string]any{}
			switch a := fn["arguments"].(type) {
			case string:
				_ = json.Unmarshal([]byte(a), &args)
			case map[string]any:
				args = a
			}
			out = append(out, ToolCall{Name: name, Arguments: args})
		}
	}
	return out
}

// callAlreadyMade reports whether this call (same tool, same target) was already
// executed earlier in the conversation.
func callAlreadyMade(executed []ToolCall, call *ToolCall) bool {
	if call == nil {
		return false
	}
	target := toolCallTarget(call.Arguments)
	for _, prev := range executed {
		if !strings.EqualFold(prev.Name, call.Name) {
			continue
		}
		prevTarget := toolCallTarget(prev.Arguments)
		if target == "" || prevTarget == "" {
			return true
		}
		if strings.EqualFold(prevTarget, target) {
			return true
		}
	}
	return false
}

// SameToolCall reports whether two tool calls target the same tool and the same
// path-like argument. It is the exported form of callAlreadyMade and exists so
// other providers can reuse the dedup rule.
func SameToolCall(prev, call *ToolCall) bool {
	if prev == nil || call == nil {
		return false
	}
	if !strings.EqualFold(prev.Name, call.Name) {
		return false
	}
	target := toolCallTarget(call.Arguments)
	prevTarget := toolCallTarget(prev.Arguments)
	if target == "" || prevTarget == "" {
		return true
	}
	return strings.EqualFold(prevTarget, target)
}

// writeIntentRe detects a request that asks for a file to be created or changed.
var writeIntentRe = regexp.MustCompile(`(?i)\b(write|rewrite|update|edit|modify|change|create|add|append|insert|remove|delete|rename|fix|apply|patch)\b`)

// specParamName returns the argument name the client's tool schema expects.
// Tools differ on naming (filePath vs path vs filename), so inferred calls must
// use the schema's own key rather than a guessed one.
func specParamName(spec ToolSpec, candidates ...string) string {
	props := map[string]any(nil)
	if spec.Parameters != nil {
		if p, ok := spec.Parameters["properties"].(map[string]any); ok {
			props = p
		}
	}
	for _, c := range candidates {
		if props != nil {
			for k := range props {
				if strings.EqualFold(k, c) {
					return k
				}
			}
		}
	}
	// Fall back to the schema's declared required/first property.
	if spec.Parameters != nil {
		if req, ok := spec.Parameters["required"].([]any); ok {
			for _, r := range req {
				if name, ok := r.(string); ok && name != "" {
					if props == nil {
						return name
					}
					if _, exists := props[name]; exists {
						return name
					}
				}
			}
		}
		for k := range props {
			return k
		}
	}
	return candidates[0]
}

// specForName returns the advertised spec for an alias group.
func specForName(specs []ToolSpec, aliases ...string) (ToolSpec, bool) {
	for _, s := range specs {
		for _, a := range aliases {
			if strings.EqualFold(s.Name, a) {
				return s, true
			}
		}
	}
	return ToolSpec{}, false
}

// argKeyGroups lists argument names that mean the same thing across tool
// schemas, so a call can be reconciled with the client's own schema.
var argKeyGroups = [][]string{
	{"filepath", "file_path", "path", "filename", "file", "targetfile", "target_file", "absolute_path", "absolutepath"},
	{"command", "cmd", "script", "shell_command", "bash"},
	{"content", "contents", "text", "data", "filetext", "file_text", "newcontent", "new_content"},
	{"oldstring", "old_string", "oldtext", "old_text", "old"},
	{"newstring", "new_string", "newtext", "new_text", "new"},
	{"pattern", "query", "regex", "glob", "search", "needle"},
}

// toolAliasGroups lists tool names that are equivalent.
var toolAliasGroups = [][]string{
	{"read", "cat", "view", "open", "read_file", "readfile"},
	{"shell", "bash", "run", "run_command", "exec", "terminal", "execute_bash"},
	{"write", "create_file", "write_file", "save", "create"},
	{"edit", "str_replace", "replace", "apply_patch", "patch", "modify_file", "str_replace_editor"},
	{"grep", "search", "search_files", "ripgrep", "rg", "search_text"},
	{"glob", "find", "find_files", "ls", "list", "list_dir", "list_files"},
}

func inGroup(name string, group []string) bool {
	for _, g := range group {
		if strings.EqualFold(strings.ReplaceAll(name, "-", "_"), strings.ReplaceAll(g, "-", "_")) {
			return true
		}
	}
	return false
}

// mapArgKeys rewrites argument keys to the ones the client's schema declares,
// preserving values for keys that already match.
func mapArgKeys(spec ToolSpec, args map[string]any) map[string]any {
	if len(args) == 0 || spec.Parameters == nil {
		return args
	}
	props, ok := spec.Parameters["properties"].(map[string]any)
	if !ok || len(props) == 0 {
		return args
	}
	out := make(map[string]any, len(args))
	for k, v := range args {
		if _, exists := props[k]; exists {
			out[k] = v
			continue
		}
		mapped := ""
		for _, group := range argKeyGroups {
			if !inGroup(k, group) {
				continue
			}
			for propKey := range props {
				if inGroup(propKey, group) {
					mapped = propKey
					break
				}
			}
			break
		}
		if mapped != "" {
			out[mapped] = v
		} else {
			out[k] = v
		}
	}
	return out
}

// NormalizeToolCall reconciles a model-produced call with the client's own tool
// schema. ChatGPT web drifts towards names it saw elsewhere ("shell" for a
// client that only offers "bash", or "path" for "filePath"), and the client then
// fails with an unresolvable tool binding. Unknown tools are returned unchanged.
func NormalizeToolCall(specs []ToolSpec, call *ToolCall) *ToolCall {
	if call == nil || len(specs) == 0 {
		return call
	}
	if spec, ok := specForName(specs, call.Name); ok {
		return &ToolCall{Name: spec.Name, Arguments: mapArgKeys(spec, call.Arguments)}
	}
	for _, group := range toolAliasGroups {
		if !inGroup(call.Name, group) {
			continue
		}
		if spec, ok := specForName(specs, group...); ok {
			return &ToolCall{Name: spec.Name, Arguments: mapArgKeys(spec, call.Arguments)}
		}
	}
	return call
}

// actionIntentRe detects a request that implies tool use is needed.
var actionIntentRe = regexp.MustCompile(`(?i)\b(inspect|check|look|find|search|continue|keep going|proceed|implement|refactor|add|create|fix|update|edit|modify|remove|delete|rename|move|read|open|run|execute|test|build|install|analyze|analyse|review|explore|list|show me|what is in|how many|where is)\b`)

// refusalRe detects a reply that declined to act for lack of a tool.
var refusalRe = regexp.MustCompile(`(?i)(no (\w+ )?(tool|access|file|shell|command)|not (available|exposed|accessible)|cannot (access|inspect|read|run|execute)|can't (access|inspect|read|run|execute)|unable to (access|inspect|read|run|execute)|need(s)? (to )?(inspect|access|read|run)|tool binding|without (file|shell|tool))`)

// wantsToolUse reports whether a turn should be pushed for a tool call. It fires
// when the request implies investigation or action, or when the reply itself
// explains that it could not proceed without a tool.
func wantsToolUse(userPrompt, reply string) bool {
	if refusalRe.MatchString(reply) {
		return true
	}
	return actionIntentRe.MatchString(userPrompt)
}

// forcedToolDirective is appended to the transcript when the model answered
// without calling a tool. Agent clients stop their loop as soon as a reply has no
// tool call, so a single explicit re-ask is what keeps the agent running.
const forcedToolDirective = "\n\n[FORCED TOOL CALL]\n" +
	"Your previous reply contained no <tool_call> tag, so the caller received no action to perform and stopped.\n" +
	"If the request refers to specific files whose contents you have NOT been shown, you MUST call the read tool for them.\n" +
	"Never describe a file from memory or guess its contents.\n" +
	"Do not apologise, do not explain, do not summarise, do not answer from memory.\n" +
	"Reply now with EXACTLY ONE <tool_call> for the single most useful tool to make progress on the request above, and nothing else.\n" +
	"Example shape: <tool_call>{\"name\": \"<tool>\", \"arguments\": { ... }}</tool_call>"

// ExtractExistingPath finds a file or directory the user named that exists on
// disk. Unlike ExtractFilePath it accepts directories and extensionless paths.
func ExtractExistingPath(userPrompt string, bases []string) (string, bool) {
	var candidates []string
	for _, re := range []*regexp.Regexp{atMentionRe, inlineTickRe, bareFileRe, barePathRe} {
		for _, m := range re.FindAllStringSubmatch(userPrompt, -1) {
			if len(m) < 2 {
				continue
			}
			c := strings.Trim(strings.TrimSpace(m[1]), "\"'`")
			if c != "" {
				candidates = append(candidates, c)
			}
		}
	}
	for _, cand := range candidates {
		p, base := resolveExistingPathBase(bases, cand)
		if p == "" || base == "" {
			continue
		}
		info, err := os.Stat(p)
		if err != nil {
			continue
		}
		rel := realCasePath(base, cand)
		return rel, info.IsDir()
	}
	return "", false
}

// shellQuote wraps a path for safe use inside a shell command.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// InferProgressCall makes one safe, bounded attempt to keep an agent loop alive
// when the model refused to call a tool twice. It only ever targets a path that
// exists on disk, and only tools the client itself advertised. Listing a real
// directory is preferable to terminating the agent's run.
func InferProgressCall(userPrompt string, specs []ToolSpec, bases []string) *ToolCall {
	path, isDir := ExtractExistingPath(userPrompt, bases)
	if path == "" {
		return nil
	}

	if isDir {
		if spec, ok := specForName(specs, "list", "ls", "list_dir", "list_files", "glob", "find", "find_files"); ok {
			key := specParamName(spec, "path", "filePath", "file_path", "directory", "dir", "pattern", "glob")
			return &ToolCall{Name: spec.Name, Arguments: map[string]any{key: path}}
		}
		if spec, ok := specForName(specs, "shell", "bash", "run", "run_command", "exec", "terminal"); ok {
			key := specParamName(spec, "command", "cmd", "script", "input")
			return &ToolCall{Name: spec.Name, Arguments: map[string]any{key: "ls -la " + shellQuote(path)}}
		}
	}

	if spec, ok := specForName(specs, "read", "cat", "view", "open", "read_file", "readfile"); ok {
		key := specParamName(spec, "filePath", "file_path", "path", "filename", "file")
		return &ToolCall{Name: spec.Name, Arguments: map[string]any{key: path}}
	}
	return nil
}

// InferBuildCall decides which tool call to synthesise for the client when the
// model declined to emit one. Order matters: an explicit file-change request
// wins, then a file read, then a command.
func InferBuildCall(userPrompt, reply string, specs []ToolSpec, bases []string, executed []ToolCall) *ToolCall {
	if w := InferWriteCall(userPrompt, reply, specs, bases); w != nil && !callAlreadyMade(executed, w) {
		return w
	}
	if r := InferToolCall(userPrompt, specs, bases); r != nil && !callAlreadyMade(executed, r) {
		return r
	}
	return nil
}

// InferWriteCall synthesises a write/edit tool call from a `file:<path>` block so
// the calling agent can perform the write with its own tooling. It only fires
// when the user asked for a change and the target path resolves on disk.
func InferWriteCall(userPrompt, reply string, specs []ToolSpec, bases []string) *ToolCall {
	if !writeIntentRe.MatchString(userPrompt) {
		return nil
	}
	path, content := ExtractFileBlock(reply)
	if path == "" {
		return nil
	}
	resolved, base := resolveExistingPathBase(bases, path)
	if resolved == "" || base == "" {
		return nil
	}
	if _, err := os.Stat(resolved); err != nil {
		return nil
	}
	// Never write back identical content.
	if existing, err := os.ReadFile(resolved); err == nil && string(existing) == content {
		return nil
	}
	rel := realCasePath(base, path)
	if spec, ok := specForName(specs, "write", "create_file", "write_file", "save"); ok {
		pathKey := specParamName(spec, "filePath", "file_path", "path", "filename", "file")
		contentKey := specParamName(spec, "content", "text", "contents", "fileText", "data")
		return &ToolCall{Name: spec.Name, Arguments: map[string]any{pathKey: rel, contentKey: content}}
	}
	if spec, ok := specForName(specs, "edit", "str_replace", "replace", "apply_patch", "patch", "modify_file"); ok {
		pathKey := specParamName(spec, "filePath", "file_path", "path", "filename", "file")
		contentKey := specParamName(spec, "content", "text", "contents", "newString", "data")
		return &ToolCall{Name: spec.Name, Arguments: map[string]any{pathKey: rel, contentKey: content}}
	}
	return nil
}

// toolNameFor returns the caller's own spelling of the first advertised tool
// matching any alias, so a synthesised call uses a name the client recognises.
func toolNameFor(specs []ToolSpec, aliases ...string) string {
	for _, s := range specs {
		for _, a := range aliases {
			if strings.EqualFold(s.Name, a) {
				return s.Name
			}
		}
	}
	return ""
}

// InferToolCall derives the tool call a user's request unambiguously requires,
// for when the model declines to emit one itself. Only clear-cut cases are
// inferred: a file that exists on disk, or a command the user asked to run.
// Handing this back lets the calling agent display and run the step with its own
// tools instead of the router silently doing the work.
func InferToolCall(userPrompt string, specs []ToolSpec, bases []string) *ToolCall {
	if spec, ok := specForName(specs, "read", "cat", "view", "open", "read_file", "readfile"); ok {
		if path := ExtractFilePath(userPrompt, bases); path != "" {
			key := specParamName(spec, "filePath", "file_path", "path", "filename", "file")
			return &ToolCall{Name: spec.Name, Arguments: map[string]any{key: path}}
		}
	}
	if spec, ok := specForName(specs, "shell", "bash", "run", "run_command", "exec", "terminal"); ok {
		if runIntentRe.MatchString(userPrompt) {
			if cmd := firstCommandLike(userPrompt); cmd != "" {
				key := specParamName(spec, "command", "cmd", "script", "input")
				return &ToolCall{Name: spec.Name, Arguments: map[string]any{key: cmd}}
			}
		}
	}
	if spec, ok := specForName(specs, "grep", "search", "search_files", "ripgrep", "rg"); ok {
		if strings.Contains(strings.ToLower(userPrompt), "grep") {
			if m := inlineTickRe.FindStringSubmatch(userPrompt); len(m) == 2 {
				if q := strings.TrimSpace(m[1]); q != "" {
					key := specParamName(spec, "pattern", "query", "regex", "text")
					return &ToolCall{Name: spec.Name, Arguments: map[string]any{key: q}}
				}
			}
		}
	}
	return nil
}

// ExtractShellCommand finds a shell command to run when the model refused shell
// tool use instead of emitting a <tool_call>. Execution requires BOTH a refusal
// in the reply and a run request from the user, and long-running server
// commands are rejected outright, so a normal answer that merely quotes a
// command block is never executed.
func ExtractShellCommand(reply, userPrompt string) string {
	if !shellRefusalRe.MatchString(reply) {
		return ""
	}
	if !runIntentRe.MatchString(userPrompt) {
		return ""
	}
	var cmd string
	if m := fencedBlockRe.FindStringSubmatch(reply); len(m) == 2 {
		cmd = cleanCommandBlock(m[1])
	}
	if cmd == "" {
		cmd = firstCommandLike(userPrompt)
	}
	if cmd == "" {
		return ""
	}
	if isLongRunningCommand(cmd) {
		return ""
	}
	return cmd
}

// longRunningCommandRe matches commands that start a server or watcher and would
// block the request until timeout. RE2 has no lookahead, so one-off exceptions
// (vite build) are handled in isLongRunningCommand.
var longRunningCommandRe = regexp.MustCompile(`(?i)\b(npm|pnpm|yarn|bun)\s+(run\s+)?(dev|start|serve|watch|preview)\b|\bvite\b|\bnodemon\b|\bnext\s+dev\b|\bpython3?\s+-m\s+http\.server\b|\bng\s+serve\b|\btail\s+-f\b|\bwatch\b|\bserve\b`)

func isLongRunningCommand(cmd string) bool {
	for _, line := range strings.Split(cmd, "\n") {
		l := strings.ToLower(strings.TrimSpace(line))
		if l == "" {
			continue
		}
		// One-shot forms are fine even though they name a dev tool.
		if strings.Contains(l, "vite build") || strings.Contains(l, "run build") ||
			strings.Contains(l, "build --") || strings.HasSuffix(l, "--help") {
			continue
		}
		if longRunningCommandRe.MatchString(l) {
			return true
		}
	}
	return false
}

func cleanCommandBlock(block string) string {
	lines := strings.Split(block, "\n")
	out := make([]string, 0, len(lines))
	for _, l := range lines {
		t := strings.TrimSpace(l)
		if t == "" || strings.HasPrefix(t, "#") {
			continue
		}
		out = append(out, strings.TrimPrefix(t, "$ "))
	}
	if len(out) == 0 {
		return ""
	}
	return strings.Join(out, "\n")
}

func firstCommandLike(s string) string {
	for _, m := range inlineTickRe.FindAllStringSubmatch(s, -1) {
		candidate := strings.TrimSpace(m[1])
		if commandWordRe.MatchString(candidate) {
			return candidate
		}
	}
	return ""
}

// hasToolNamed reports whether the caller advertised a tool under any of the
// given names (matched case-insensitively, allowing aliases).
func hasToolNamed(specs []ToolSpec, names ...string) bool {
	for _, s := range specs {
		for _, n := range names {
			if strings.EqualFold(s.Name, n) {
				return true
			}
		}
	}
	return false
}

// ExtractToolCall pulls a tool invocation out of a model reply.
// It returns the parsed call (nil when the reply is a final answer) and the
// reply text with any tool-call markup removed.
func ExtractToolCall(reply string) (*ToolCall, string) {
	if m := toolCallTagRe.FindStringSubmatch(reply); len(m) == 2 {
		if call := parseToolCallJSON(m[1]); call != nil {
			return call, strings.TrimSpace(toolCallTagRe.ReplaceAllString(reply, ""))
		}
	}
	if m := toolCallFence.FindStringSubmatch(reply); len(m) == 2 {
		if call := parseToolCallJSON(m[1]); call != nil {
			return call, strings.TrimSpace(toolCallFence.ReplaceAllString(reply, ""))
		}
	}
	// Bare JSON object anywhere in the reply.
	trimmed := strings.TrimSpace(reply)
	if strings.HasPrefix(trimmed, "{") {
		if call := parseToolCallJSON(trimmed); call != nil {
			return call, ""
		}
	}
	return nil, strings.TrimSpace(reply)
}

func parseToolCallJSON(raw string) *ToolCall {
	var envelope struct {
		Name      string         `json:"name"`
		Tool      string         `json:"tool"`
		Arguments map[string]any `json:"arguments"`
		Params    map[string]any `json:"parameters"`
		ToolCall  *struct {
			Name      string         `json:"name"`
			Arguments map[string]any `json:"arguments"`
		} `json:"tool_call"`
	}
	if err := json.Unmarshal([]byte(raw), &envelope); err != nil {
		return nil
	}
	if envelope.ToolCall != nil && envelope.ToolCall.Name != "" {
		return &ToolCall{Name: envelope.ToolCall.Name, Arguments: envelope.ToolCall.Arguments}
	}
	name := envelope.Name
	if name == "" {
		name = envelope.Tool
	}
	if name == "" {
		return nil
	}
	args := envelope.Arguments
	if args == nil {
		args = envelope.Params
	}
	if args == nil {
		args = map[string]any{}
	}
	return &ToolCall{Name: name, Arguments: args}
}

// ToolWorkdir resolves the directory tools run relative to. Defaults to the
// process working directory so @file mentions resolve like the client expects.
func (c *Client) ToolWorkdir() string {
	if c.cfg.WorkDir != "" {
		return c.cfg.WorkDir
	}
	wd, err := os.Getwd()
	if err != nil {
		return "."
	}
	return wd
}

// ExecuteTool runs one tool call locally and returns its textual result.
func (c *Client) ExecuteTool(ctx context.Context, call *ToolCall) (string, error) {
	return c.ExecuteToolIn(ctx, []string{c.ToolWorkdir()}, call)
}

// ExecuteToolIn runs one tool call, resolving relative paths against the ordered
// base-directory list. The first base is used as the shell working directory.
func (c *Client) ExecuteToolIn(ctx context.Context, bases []string, call *ToolCall) (string, error) {
	if len(bases) == 0 {
		bases = []string{c.ToolWorkdir()}
	}
	name := strings.ToLower(strings.TrimSpace(call.Name))
	workdir := bases[0]

	switch name {
	case "read", "cat", "view", "open", "read_file", "readfile":
		return toolRead(bases, call.Arguments)
	case "list", "ls", "list_dir", "listdir", "list_files":
		return toolList(bases, call.Arguments)
	case "grep", "search", "search_files", "ripgrep", "rg":
		return toolGrep(ctx, workdir, bases, call.Arguments)
	case "glob", "find", "find_files":
		return toolGlob(bases, call.Arguments)
	case "shell", "bash", "run", "run_command", "exec", "terminal":
		return c.toolShell(ctx, workdir, call.Arguments)
	default:
		return "", fmt.Errorf("unsupported tool %q", call.Name)
	}
}

// ShellEnabled reports whether local command execution is permitted.
func (c *Client) ShellEnabled() bool {
	if c.cfg.EnableShell {
		return true
	}
	return os.Getenv("CHATGPT_ENABLE_SHELL") == "1"
}

// SetShellEnabled persists the local command execution toggle.
func (c *Client) SetShellEnabled(enabled bool) error {
	c.cfg.EnableShell = enabled
	c.configMu.Lock()
	defer c.configMu.Unlock()
	c.provConfig.ShellEnabled = enabled
	return c.saveConfigLocked()
}

// PassthroughTools reports whether tool calls are handed back to the calling
// agent (so its UI shows the step) rather than executed inside the router.
// Enabled by default; set CHATGPT_INTERNAL_TOOLS=1 to force in-process execution.
func (c *Client) PassthroughTools() bool {
	if os.Getenv("CHATGPT_INTERNAL_TOOLS") == "1" {
		return false
	}
	c.configMu.RLock()
	defer c.configMu.RUnlock()
	return !c.provConfig.InternalTools
}

// SetPassthroughTools persists the tool-call routing toggle.
func (c *Client) SetPassthroughTools(enabled bool) error {
	c.configMu.Lock()
	defer c.configMu.Unlock()
	c.provConfig.InternalTools = !enabled
	return c.saveConfigLocked()
}

// ToolWorkdirPublic exposes the resolved tool working directory for status reporting.
func (c *Client) ToolWorkdirPublic() string {
	return c.ToolWorkdir()
}

// SetWorkDir overrides the directory tools resolve paths against.
func (c *Client) SetWorkDir(dir string) error {
	c.cfg.WorkDir = dir
	c.configMu.Lock()
	defer c.configMu.Unlock()
	c.provConfig.WorkDir = dir
	return c.saveConfigLocked()
}

const maxToolOutput = 60000

func truncateOutput(s string) string {
	if len(s) <= maxToolOutput {
		return s
	}
	return s[:maxToolOutput] + fmt.Sprintf("\n... [truncated, %d bytes total]", len(s))
}

func resolvePath(bases []string, args map[string]any) string {
	p, _ := args["path"].(string)
	if p == "" {
		p, _ = args["file"].(string)
	}
	if p == "" {
		p, _ = args["file_path"].(string)
	}
	if p == "" {
		p, _ = args["filename"].(string)
	}
	if p == "" {
		if len(bases) > 0 {
			return bases[0]
		}
		return "."
	}
	return resolveExistingPath(bases, p)
}

func argInt(args map[string]any, key string, def int) int {
	switch v := args[key].(type) {
	case float64:
		return int(v)
	case int:
		return v
	case json.Number:
		if i, err := v.Int64(); err == nil {
			return int(i)
		}
	}
	return def
}

func toolRead(bases []string, args map[string]any) (string, error) {
	target := resolvePath(bases, args)
	info, err := os.Stat(target)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", target, err)
	}

	offset := argInt(args, "offset", 1)
	if offset < 1 {
		offset = 1
	}
	limit := argInt(args, "limit", 2000)
	if limit <= 0 || limit > 2000 {
		limit = 2000
	}

	if info.IsDir() {
		entries, err := os.ReadDir(target)
		if err != nil {
			return "", err
		}
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			n := e.Name()
			if e.IsDir() {
				n += "/"
			}
			names = append(names, n)
		}
		sort.Strings(names)
		start := offset - 1
		if start >= len(names) {
			return "", nil
		}
		end := start + limit
		if end > len(names) {
			end = len(names)
		}
		return strings.Join(names[start:end], "\n"), nil
	}

	data, err := os.ReadFile(target)
	if err != nil {
		return "", err
	}
	lines := strings.Split(string(data), "\n")
	start := offset - 1
	if start >= len(lines) {
		return "", nil
	}
	end := start + limit
	if end > len(lines) {
		end = len(lines)
	}
	var sb strings.Builder
	for i := start; i < end; i++ {
		fmt.Fprintf(&sb, "%d: %s\n", i+1, lines[i])
	}
	return truncateOutput(sb.String()), nil
}

func toolList(bases []string, args map[string]any) (string, error) {
	target := resolvePath(bases, args)
	entries, err := os.ReadDir(target)
	if err != nil {
		return "", err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		n := e.Name()
		if e.IsDir() {
			n += "/"
		}
		names = append(names, n)
	}
	sort.Strings(names)
	return strings.Join(names, "\n"), nil
}

func toolGrep(ctx context.Context, workdir string, bases []string, args map[string]any) (string, error) {
	pattern, _ := args["pattern"].(string)
	if pattern == "" {
		pattern, _ = args["query"].(string)
	}
	if pattern == "" {
		pattern, _ = args["regex"].(string)
	}
	if pattern == "" {
		return "", fmt.Errorf("grep requires a pattern")
	}
	searchPath := workdir
	if p, _ := args["path"].(string); p != "" {
		searchPath = resolveExistingPath(bases, p)
	} else if p, _ := args["directory"].(string); p != "" {
		searchPath = resolveExistingPath(bases, p)
	}

	cctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	var out bytes.Buffer
	cmd := exec.CommandContext(cctx, "grep", "-rn", "-I", "--exclude-dir=.git", "--exclude-dir=node_modules", "-e", pattern, searchPath)
	cmd.Dir = workdir
	cmd.Stdout = &out
	cmd.Stderr = nil
	err := cmd.Run()
	if out.Len() == 0 && err != nil {
		// Fall back to a pure-Go walk so grep absence is not fatal.
		return fallbackGrep(workdir, searchPath, pattern), nil
	}
	return truncateOutput(out.String()), nil
}

func fallbackGrep(workdir, searchPath, pattern string) string {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return ""
	}
	var sb strings.Builder
	count := 0
	_ = filepath.WalkDir(searchPath, func(path string, d os.DirEntry, err error) error {
		if err != nil || count > 200 {
			return nil
		}
		if d.IsDir() {
			if d.Name() == ".git" || d.Name() == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil || bytes.IndexByte(data, 0) >= 0 {
			return nil
		}
		rel, _ := filepath.Rel(workdir, path)
		for i, line := range strings.Split(string(data), "\n") {
			if re.MatchString(line) {
				fmt.Fprintf(&sb, "%s:%d:%s\n", rel, i+1, line)
				count++
				if count > 200 {
					return nil
				}
			}
		}
		return nil
	})
	return truncateOutput(sb.String())
}

func toolGlob(bases []string, args map[string]any) (string, error) {
	pattern, _ := args["pattern"].(string)
	if pattern == "" {
		pattern, _ = args["glob"].(string)
	}
	if pattern == "" {
		return "", fmt.Errorf("glob requires a pattern")
	}
	base := ""
	if len(bases) > 0 {
		base = bases[0]
	}
	if p, _ := args["path"].(string); p != "" {
		base = resolveExistingPath(bases, p)
	}
	workdir := base
	full := pattern
	if !filepath.IsAbs(pattern) {
		full = filepath.Join(base, pattern)
	}
	matches, err := filepath.Glob(full)
	if err != nil {
		return "", err
	}
	rels := make([]string, 0, len(matches))
	for _, m := range matches {
		if rel, err := filepath.Rel(workdir, m); err == nil {
			rels = append(rels, rel)
		} else {
			rels = append(rels, m)
		}
	}
	sort.Strings(rels)
	return truncateOutput(strings.Join(rels, "\n")), nil
}

func (c *Client) toolShell(ctx context.Context, workdir string, args map[string]any) (string, error) {
	if !c.ShellEnabled() {
		return "", fmt.Errorf("shell tool is disabled; set CHATGPT_ENABLE_SHELL=1 or enable it in the provider config to allow local command execution")
	}
	command, _ := args["command"].(string)
	if command == "" {
		return "", fmt.Errorf("shell requires a command")
	}
	runDir := workdir
	if wd, _ := args["workdir"].(string); wd != "" {
		if filepath.IsAbs(wd) {
			runDir = wd
		} else {
			runDir = filepath.Join(workdir, wd)
		}
	}
	timeoutMs := argInt(args, "timeout", 120000)
	if timeoutMs <= 0 {
		timeoutMs = 120000
	}
	cctx, cancel := context.WithTimeout(ctx, time.Duration(timeoutMs)*time.Millisecond)
	defer cancel()

	var out bytes.Buffer
	cmd := exec.CommandContext(cctx, "bash", "-c", command)
	cmd.Dir = runDir
	cmd.Stdout = &out
	cmd.Stderr = &out
	configureProcessGroup(cmd)
	err := cmd.Run()
	res := truncateOutput(out.String())
	if err != nil && res == "" {
		return "", fmt.Errorf("shell: %w", err)
	}
	if err != nil {
		res += fmt.Sprintf("\n[exit error: %v]", err)
	}
	return res, nil
}
