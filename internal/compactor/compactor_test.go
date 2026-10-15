package compactor

import (
	"encoding/json"
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
	if !strings.Contains(out, "truncated") {
		t.Errorf("expected truncated in output")
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
	if !strings.Contains(out, "50 matches in 2 files:") && !strings.Contains(out, "50 matches in 2F:") {
		t.Errorf("expected matches header in output: %s", out)
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
	if !strings.Contains(content, "truncated") {
		t.Errorf("expected content to be compressed in-place")
	}
}

func TestStripAnsi(t *testing.T) {
	colored := "\x1b[32mPASS\x1b[0m test_foo"
	clean := StripAnsi(colored)
	if clean != "PASS test_foo" {
		t.Errorf("expected 'PASS test_foo', got %q", clean)
	}
}

func TestCargoTest(t *testing.T) {
	input := strings.Join([]string{
		"   Compiling mypkg v0.1.0",
		"    Finished test [unoptimized + debuginfo] in 0.82s",
		"     Running unittests src/lib.rs",
		"running 20 tests",
		"test tests::test_1 ... ok",
		"test tests::test_2 ... ok",
		"test tests::test_fail ... FAILED",
		"",
		"failures:",
		"",
		"---- tests::test_fail stdout ----",
		"thread 'main' panicked at 'assertion failed: 1 == 2', src/lib.rs:42:9",
		"",
		"failures:",
		"    tests::test_fail",
		"",
		"test result: FAILED. 19 passed; 1 failed; 0 ignored; 0 measured; 0 filtered out",
	}, "\n")

	out := CargoTest(input)
	if !strings.Contains(out, "failures:") {
		t.Errorf("expected failures section in output")
	}
	if !strings.Contains(out, "tests::test_fail") {
		t.Errorf("expected failed test name in output")
	}
	if strings.Contains(out, "test tests::test_1 ... ok") {
		t.Errorf("expected passing test line to be omitted")
	}
	if strings.Contains(out, "Compiling mypkg") {
		t.Errorf("expected Compiling line to be omitted")
	}
}

func TestPytest(t *testing.T) {
	input := strings.Join([]string{
		"============================= test session starts ==============================",
		"platform darwin -- Python 3.11.0",
		"collected 10 items",
		"",
		"tests/test_basic.py .F.......                                            [100%]",
		"",
		"=================================== FAILURES ===================================",
		"__________________________________ test_fail ___________________________________",
		"    def test_fail():",
		">       assert 1 == 2",
		"E       assert 1 == 2",
		"",
		"tests/test_basic.py:5: AssertionError",
		"=========================== short test summary info ============================",
		"FAILED tests/test_basic.py::test_fail - assert 1 == 2",
		"========================= 1 failed, 9 passed in 0.12s ==========================",
	}, "\n")

	out := Pytest(input)
	if !strings.Contains(out, "FAILURES:") {
		t.Errorf("expected FAILURES section in output: %s", out)
	}
	if !strings.Contains(out, "assert 1 == 2") {
		t.Errorf("expected failure detail in output")
	}
	if strings.Contains(out, "test session starts") {
		t.Errorf("expected header to be omitted")
	}
}

func TestGoTest(t *testing.T) {
	input := strings.Join([]string{
		"=== RUN   TestPass1",
		"--- PASS: TestPass1 (0.00s)",
		"=== RUN   TestPass2",
		"--- PASS: TestPass2 (0.00s)",
		"=== RUN   TestFail",
		"--- FAIL: TestFail (0.00s)",
		"    main_test.go:42: expected true got false",
		"FAIL",
		"FAIL\tmypkg\t0.05s",
	}, "\n")

	out := GoTest(input)
	if !strings.Contains(out, "--- FAIL: TestFail") {
		t.Errorf("expected failure header in output: %s", out)
	}
	if strings.Contains(out, "--- PASS: TestPass1") {
		t.Errorf("expected passing test to be omitted")
	}
}

func TestService(t *testing.T) {
	tmp, err := os.MkdirTemp("", "compactor-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmp)

	s, err := NewService(tmp)
	if err != nil {
		t.Fatal(err)
	}

	if !s.Enabled() {
		t.Errorf("expected compactor to be enabled by default")
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
	if st["enabled"] != true {
		t.Errorf("expected enabled to be true")
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

func makeJSONToolOutput(n int) string {
	rows := make([]map[string]any, 0, n)
	for i := 0; i < n; i++ {
		status := "ok"
		snippet := "export let value = $state(0);"
		if i == n/2 {
			status = "error"
			snippet = "FATAL: unhandled rejection in transport layer"
		}
		rows = append(rows, map[string]any{
			"id":         i,
			"path":       fmt.Sprintf("src/lib/components/x%d.svelte", i%7),
			"lineNumber": 120 + i,
			"score":      float64(i) / float64(n),
			"snippet":    snippet,
			"status":     status,
		})
	}
	out, err := json.MarshalIndent(rows, "", "  ")
	if err != nil {
		panic(err)
	}
	return string(out)
}

func TestJSONNeverBecomesGitStatus(t *testing.T) {
	in := makeJSONToolOutput(120)
	out, filterName := compressOne(in)
	if filterName != "json" {
		t.Fatalf("expected json filter, got %q", filterName)
	}
	if strings.Contains(out, "Clean working tree") {
		t.Fatal("JSON tool output was summarized as a git status")
	}
	if !json.Valid([]byte(out)) {
		t.Fatalf("compressed output is not valid JSON: %s", out[:120])
	}
	if !strings.Contains(out, "FATAL") {
		t.Error("expected the error row to survive compression")
	}
	if len(out) >= len(in) {
		t.Errorf("expected savings, got %d -> %d bytes", len(in), len(out))
	}
}

func TestJSONTableFoldsUniformRows(t *testing.T) {
	const rows = 50
	in := makeJSONToolOutput(rows)
	out, filterName := compressOne(in)
	if filterName != "json" {
		t.Fatalf("expected json filter, got %q", filterName)
	}

	var parsed struct {
		Marker string   `json:"_compacted"`
		Fields []string `json:"fields"`
		Rows   [][]any  `json:"rows"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("decode folded table: %v", err)
	}
	if parsed.Marker != "table" {
		t.Errorf("expected _compacted=table, got %q", parsed.Marker)
	}
	if len(parsed.Rows) != rows {
		t.Errorf("expected %d rows, got %d", rows, len(parsed.Rows))
	}
	for i, row := range parsed.Rows {
		if len(row) != len(parsed.Fields) {
			t.Fatalf("row %d has %d values for %d fields", i, len(row), len(parsed.Fields))
		}
	}
	// Every original row must survive: same count, and the error row is present.
	found := false
	for _, row := range parsed.Rows {
		for _, v := range row {
			if s, ok := v.(string); ok && strings.HasPrefix(s, "FATAL:") {
				found = true
			}
		}
	}
	if !found {
		t.Error("expected the error row in the folded table")
	}

	// Lossless: rebuilding objects from fields+rows must equal the input.
	var original []map[string]any
	if err := json.Unmarshal([]byte(in), &original); err != nil {
		t.Fatal(err)
	}
	rebuilt := make([]map[string]any, 0, len(parsed.Rows))
	for _, row := range parsed.Rows {
		obj := make(map[string]any, len(parsed.Fields))
		for i, f := range parsed.Fields {
			obj[f] = row[i]
		}
		rebuilt = append(rebuilt, obj)
	}
	want, _ := json.Marshal(original)
	got, _ := json.Marshal(rebuilt)
	if string(want) != string(got) {
		t.Error("fold is not lossless")
	}
}

func TestJSONFallsBackToMinify(t *testing.T) {
	in, err := json.MarshalIndent([]any{
		map[string]any{"path": "a.go", "size": 12},
		map[string]any{"path": "b.go", "size": 34},
	}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	out, filterName := compressOne(string(in))
	if filterName != "json" {
		t.Fatalf("expected json filter, got %q", filterName)
	}
	if strings.Contains(out, "_compacted") {
		t.Error("expected minify-only for a short non-uniform array")
	}
	if !json.Valid([]byte(out)) {
		t.Fatalf("output is not valid JSON: %s", out)
	}
}

func TestJSONAlreadyCompactIsUnchanged(t *testing.T) {
	in := `{"a":1,"b":"two"}`
	out, filterName := compressOne(in)
	if filterName != "" {
		t.Errorf("expected passthrough, got filter %q", filterName)
	}
	if out != in {
		t.Errorf("expected unchanged input, got %q", out)
	}
}

func TestAutoDetectSkipsJSON(t *testing.T) {
	if _, name := AutoDetectFilter(makeJSONToolOutput(5)); name != "" {
		t.Errorf("expected no shell filter for JSON, got %q", name)
	}
}

func TestGitStatusPassesThroughNonStatusInput(t *testing.T) {
	cases := []string{
		"[\n   \"alpha\",\n   \"beta\",\n   \"gamma\"\n]",
		"context:\n   \"alpha\": 1,\n   \"beta\": 2,\n   \"gamma\": 3",
		"nonsense output\nfrom a tool\nwith indentation",
	}
	for _, in := range cases {
		if got := GitStatus(in); got != in {
			t.Errorf("GitStatus rewrote non-status input:\n in: %q\nout: %q", in, got)
		}
	}
}

func TestGrepDetectionRejectsTimestampedLogs(t *testing.T) {
	line := "2026-10-15T10:00:00Z INFO worker[3] fetch 3 ok 12ms"
	if isGrepLine(line) {
		t.Fatalf("timestamped log line detected as grep output: %q", line)
	}
	if _, name := AutoDetectFilter(line + "\n" + line + "\n" + line + "\n" + line + "\n" + line + "\n" + line); name == "grep" {
		t.Error("timestamped log detected as grep output")
	}
}

func TestTimestampedLogKeepsErrorLine(t *testing.T) {
	var b strings.Builder
	for i := 0; i < 200; i++ {
		fmt.Fprintf(&b, "2026-10-15T10:%02d:00Z INFO worker[%d] fetch %d ok 12ms\n", i%60, i%8, i)
	}
	b.WriteString("2026-10-15T10:59:00Z FATAL worker[3] connection reset by peer\n")
	in := b.String()

	out, _ := compressOne(in)
	if !strings.Contains(out, "FATAL worker[3] connection reset by peer") {
		t.Errorf("expected the FATAL line to survive, got %d bytes", len(out))
	}
}

func TestPreservesErrorSignals(t *testing.T) {
	if preservesErrorSignals("ok\nFATAL: boom", "all good") {
		t.Error("expected false when the output drops a FATAL line")
	}
	if !preservesErrorSignals("ok\nFATAL: boom", "1 error\nFATAL: boom") {
		t.Error("expected true when the FATAL line is kept")
	}
	if !preservesErrorSignals("nothing to see", "summary") {
		t.Error("expected true when the input has no error signals")
	}
}

func TestIsJSONLike(t *testing.T) {
	if !IsJSONLike("  [1,2,3]  ") {
		t.Error("expected array to be JSON-like")
	}
	if !IsJSONLike(`{"a":1}`) {
		t.Error("expected object to be JSON-like")
	}
	if IsJSONLike("42") || IsJSONLike(`"text"`) || IsJSONLike("{not json}") {
		t.Error("expected scalars and malformed JSON to be rejected")
	}
}
