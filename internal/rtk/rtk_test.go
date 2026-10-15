package rtk

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

func makeLongDiff() string {
	var lines []string
	lines = append(lines, "diff --git a/foo.js b/foo.js", "index abc..def 100644", "--- a/foo.js", "+++ b/foo.js", "@@ -1,3 +1,200 @@")
	for i := 0; i < 200; i++ {
		lines = append(lines, fmt.Sprintf("+added line %d %s", i, strings.Repeat("x", 20)))
	}
	return strings.Join(lines, "\n")
}

func makeGitStatus() string {
	return strings.Join([]string{
		"On branch main",
		"Your branch is up to date with 'origin/main'.",
		"",
		"Changes not staged for commit:",
		"  (use \"git add <file>...\" to update what will be committed)",
		"\tmodified:   src/a.js",
		"\tmodified:   src/b.js",
		"\tnew file:   src/c.js",
		"\tdeleted:    src/old.js",
		"",
		"Untracked files:",
		"\tnotes.txt",
		"",
		"no changes added to commit",
	}, "\n")
}

func makeGrepOutput() string {
	var lines []string
	for i := 1; i <= 40; i++ {
		lines = append(lines, fmt.Sprintf("src/foo.js:%d:const x%d = \"some value here with padding text padding text\"", i, i))
	}
	for i := 1; i <= 10; i++ {
		lines = append(lines, fmt.Sprintf("src/bar.js:%d:const y%d = \"another value here with padding padding padding\"", i, i))
	}
	return strings.Join(lines, "\n")
}

func makeFindOutput() string {
	var lines []string
	for i := 0; i < 30; i++ {
		lines = append(lines, fmt.Sprintf("./src/a/%d.js", i))
	}
	for i := 0; i < 20; i++ {
		lines = append(lines, fmt.Sprintf("./src/b/%d.js", i))
	}
	for i := 0; i < 5; i++ {
		lines = append(lines, fmt.Sprintf("./top%d.md", i))
	}
	return strings.Join(lines, "\n")
}

func TestGitDiff(t *testing.T) {
	input := makeLongDiff()
	out := GitDiff(input, 500)
	if !strings.Contains(out, "foo.js") {
		t.Errorf("expected foo.js in output")
	}
	if !strings.Contains(out, "lines truncated") {
		t.Errorf("expected lines truncated in output")
	}
	if len(out) >= len(input) {
		t.Errorf("expected compressed diff to be smaller than input")
	}
}

func TestGitStatus(t *testing.T) {
	input := makeGitStatus()
	out := GitStatus(input)
	if !strings.Contains(out, "* main") {
		t.Errorf("expected * main in output: %s", out)
	}
	if !strings.Contains(out, "~ Modified:") {
		t.Errorf("expected ~ Modified: in output: %s", out)
	}
	if !strings.Contains(out, "src/a.js") {
		t.Errorf("expected src/a.js in output")
	}
	if len(out) >= len(input) {
		t.Errorf("expected compressed status to be smaller than input")
	}
}

func TestGrep(t *testing.T) {
	input := makeGrepOutput()
	out := Grep(input)
	if !strings.Contains(out, "50 matches in 2F:") {
		t.Errorf("expected '50 matches in 2F:' in output: %s", out)
	}
	if !strings.Contains(out, "[file] src/foo.js (40):") {
		t.Errorf("expected '[file] src/foo.js (40):' in output: %s", out)
	}
	if len(out) >= len(input) {
		t.Errorf("expected compressed grep to be smaller than input")
	}
}

func TestFind(t *testing.T) {
	input := makeFindOutput()
	out := Find(input)
	if !strings.Contains(out, "55 files in 3 dirs:") {
		t.Errorf("expected '55 files in 3 dirs:' in output: %s", out)
	}
	if !strings.Contains(out, "./src/a/  (30)") {
		t.Errorf("expected './src/a/  (30)' in output: %s", out)
	}
	if len(out) >= len(input) {
		t.Errorf("expected compressed find to be smaller than input")
	}
}

func TestDedupLog(t *testing.T) {
	var lines []string
	for i := 0; i < 20; i++ {
		lines = append(lines, "repeated log line A")
	}
	lines = append(lines, "unique")
	for i := 0; i < 10; i++ {
		lines = append(lines, "another dup")
	}
	input := strings.Join(lines, "\n")
	out := DedupLog(input)
	if !strings.Contains(out, "repeated log line A") {
		t.Errorf("expected repeated line in output")
	}
	if !strings.Contains(out, "duplicate lines") {
		t.Errorf("expected 'duplicate lines' in output")
	}
	if len(out) >= len(input) {
		t.Errorf("expected compressed dedup log to be smaller than input")
	}
}

func TestAutoDetect(t *testing.T) {
	_, name := AutoDetectFilter("diff --git a/x b/x\n@@ -1 +1 @@\n+a")
	if name != "git-diff" {
		t.Errorf("expected git-diff, got %s", name)
	}

	_, name = AutoDetectFilter("On branch main\n  modified:   x.js\n")
	if name != "git-status" {
		t.Errorf("expected git-status, got %s", name)
	}

	_, name = AutoDetectFilter("a.js:1:hello\nb.js:2:world\nc.js:3:foo")
	if name != "grep" {
		t.Errorf("expected grep, got %s", name)
	}

	_, name = AutoDetectFilter("./a/b.js\n./a/c.js\n./a/d.js")
	if name != "find" {
		t.Errorf("expected find, got %s", name)
	}

	_, name = AutoDetectFilter("commit abc1234def5678abc1234def5678abc1234def5\nAuthor: Dev <dev@example.com>\nDate: 2026-01-01\n\n    Initial commit\n")
	if name != "git-log" {
		t.Errorf("expected git-log, got %s", name)
	}
}

func TestCompressMessages(t *testing.T) {
	body := map[string]any{
		"messages": []any{
			map[string]any{
				"role":    "user",
				"content": "check status",
			},
			map[string]any{
				"role":    "tool",
				"content": makeLongDiff(),
			},
		},
	}

	stats := CompressMessages(body, true)
	if stats == nil {
		t.Fatalf("expected stats, got nil")
	}
	if len(stats.Hits) == 0 {
		t.Fatalf("expected at least 1 hit")
	}
	if stats.Hits[0].Filter != "git-diff" {
		t.Errorf("expected filter git-diff, got %s", stats.Hits[0].Filter)
	}
	if stats.BytesAfter >= stats.BytesBefore {
		t.Errorf("expected bytes after (%d) < bytes before (%d)", stats.BytesAfter, stats.BytesBefore)
	}

	// Verify in-place mutation
	msgs := body["messages"].([]any)
	toolMsg := msgs[1].(map[string]any)
	content := toolMsg["content"].(string)
	if !strings.Contains(content, "lines truncated") {
		t.Errorf("expected content to be compressed in-place")
	}
}

func TestService(t *testing.T) {
	tmp, err := os.MkdirTemp("", "rtk-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmp)

	s, err := NewService(tmp)
	if err != nil {
		t.Fatal(err)
	}

	if !s.IsRTKEnabled() {
		t.Errorf("expected RTK to be enabled by default")
	}

	body := map[string]any{
		"messages": []any{
			map[string]any{
				"role":    "tool",
				"content": makeLongDiff(),
			},
		},
	}

	stats := s.ProcessBody(body, false)
	if stats == nil {
		t.Fatalf("expected non-nil stats")
	}

	st := s.GetStatus()
	if st["rtk_enabled"] != true {
		t.Errorf("expected rtk_enabled to be true")
	}

	stMap := st["stats"].(map[string]any)
	if stMap["compressed_requests"].(int64) != 1 {
		t.Errorf("expected 1 compressed request, got %v", stMap["compressed_requests"])
	}

	// Test bypass
	body2 := map[string]any{
		"messages": []any{
			map[string]any{
				"role":    "tool",
				"content": makeLongDiff(),
			},
		},
	}
	stats2 := s.ProcessBody(body2, true)
	if stats2 != nil {
		t.Errorf("expected nil stats on bypass")
	}
}
