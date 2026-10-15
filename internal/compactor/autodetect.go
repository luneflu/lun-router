package compactor

import (
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

var (
	reGitDiffHead   = regexp.MustCompile(`(?m)^diff --(git|cc|combined) `)
	reGitDiffHunk   = regexp.MustCompile(`(?m)^@@ `)
	reGitStatusHead = regexp.MustCompile(`(?m)^On branch |^nothing to commit|^Changes (not |to be )|^Untracked files:`)
	reBuildOutput   = regexp.MustCompile(`(?im)^(npm (warn|error|ERR!)|yarn (warn|error)|\s*Compiling\s+\S+|\s*Downloading\s+\S+|added \d+ package|\[ERROR\]|BUILD (SUCCESS|FAILED)|\s*Finished\s+|Successfully (installed|built)|ERROR:)`)
	reTreeGlyph     = regexp.MustCompile(`[├└]──|│  `)
	reLsRow         = regexp.MustCompile(`(?m)^[-dlbcps][rwx-]{9}`)
	reLsTotal       = regexp.MustCompile(`(?m)^total \d+$`)
	reWindowsDrive  = regexp.MustCompile(`^[A-Za-z]:[\\/]`)
)

type FilterFunc func(string) string

func safeHead(s string, maxBytes int) string {
	if len(s) <= maxBytes {
		return s
	}
	for maxBytes > 0 && !utf8.RuneStart(s[maxBytes]) {
		maxBytes--
	}
	return s[:maxBytes]
}

func AutoDetectFilter(text string) (FilterFunc, string) {
	clean := StripAnsi(text)

	// JSON is handled by the structure-based JSON path, never by the shell-output
	// filters below.
	if IsJSONLike(clean) {
		return nil, ""
	}

	head := safeHead(clean, DetectWindow)

	// Test output autodetection (matching official RTK pipe_cmd.rs)
	if strings.Contains(head, "test result:") && strings.Contains(head, "passed;") {
		return CargoTest, "cargo-test"
	}
	if strings.Contains(head, "=== test session starts") || (strings.Contains(head, "===") && strings.Contains(head, "FAILURES")) {
		return Pytest, "pytest"
	}
	if (strings.Contains(head, "=== RUN") && (strings.Contains(head, "--- PASS:") || strings.Contains(head, "--- FAIL:"))) ||
		(strings.HasPrefix(strings.TrimSpace(head), "--- FAIL:") || (strings.Contains(head, "--- FAIL:") && strings.Contains(head, "FAIL\t"))) {
		return GoTest, "go-test"
	}

	if reCommitHeader.MatchString(head) {
		return func(s string) string { return GitLog(s, GitLogMaxLines) }, "git-log"
	}

	if reGitDiffHead.MatchString(head) || reGitDiffHunk.MatchString(head) {
		return func(s string) string { return GitDiff(s, GitDiffMaxLines) }, "git-diff"
	}

	if reGitStatusHead.MatchString(head) {
		return GitStatus, "git-status"
	}

	// Build output BEFORE porcelain check: prevents cargo "Compiling" misdetection as git-status
	if reBuildOutput.MatchString(head) {
		return BuildOutput, "build-output"
	}

	if isMostlyPorcelain(head) {
		return GitStatus, "git-status"
	}

	lines := strings.Split(head, "\n")
	var nonEmpty []string
	for _, l := range lines {
		if strings.TrimSpace(l) != "" {
			nonEmpty = append(nonEmpty, l)
		}
	}

	// First 5 non-empty lines, ANY matches "file:number:content"
	first5 := nonEmpty
	if len(first5) > 5 {
		first5 = first5[:5]
	}
	for _, l := range first5 {
		if isGrepLine(l) {
			return Grep, "grep"
		}
	}

	// find: ALL non-empty lines path-like, >= 3 lines
	if len(nonEmpty) >= 3 {
		allPath := true
		for _, l := range nonEmpty {
			if !isPathLike(l) {
				allPath = false
				break
			}
		}
		if allPath {
			return Find, "find"
		}
	}

	// Tree glyphs
	if reTreeGlyph.MatchString(head) {
		return Tree, "tree"
	}

	// ls -la
	if reLsTotal.MatchString(head) || len(reLsRow.FindAllString(head, -1)) >= 3 {
		return Ls, "ls"
	}

	// Search list header
	if reSearchListHdr.MatchString(head) {
		return SearchList, "search-list"
	}

	allLines := strings.Split(clean, "\n")
	if len(allLines) >= SmartTruncateMinLines && isLineNumbered(allLines) {
		return ReadNumbered, "read-numbered"
	}

	// dedupLog for generic multi-line noise
	if len(nonEmpty) >= 5 {
		return DedupLog, "dedup-log"
	}

	// Last resort: big blob with no structure
	if len(allLines) >= SmartTruncateMinLines {
		return SmartTruncate, "smart-truncate"
	}

	return nil, ""
}

func isGrepLine(line string) bool {
	// Timestamped logs ("2026-10-15T10:00:00Z INFO ...") look like grep output
	// ("file:number:content") but are not.
	if reISOTime.MatchString(line) {
		return false
	}
	first := strings.Index(line, ":")
	if first == -1 {
		return false
	}
	second := strings.Index(line[first+1:], ":")
	if second == -1 {
		return false
	}
	second += first + 1
	lineno := line[first+1 : second]
	_, err := strconv.Atoi(lineno)
	return err == nil
}

func isPathLike(line string) bool {
	t := strings.TrimSpace(line)
	if t == "" {
		return false
	}
	if reWindowsDrive.MatchString(t) {
		return true
	}
	if strings.Contains(t, ":") {
		return false
	}
	return strings.HasPrefix(t, ".") || strings.HasPrefix(t, "/") || strings.Contains(t, "/")
}

func looksLikeGitStatus(text string) bool {
	head := safeHead(text, DetectWindow)
	if reGitStatusHead.MatchString(head) {
		return true
	}
	for _, line := range strings.Split(head, "\n") {
		if rePorcelainLine.MatchString(line) {
			return true
		}
	}
	return false
}

func isMostlyPorcelain(head string) bool {
	lines := strings.Split(head, "\n")
	var valid []string
	for _, l := range lines {
		if strings.TrimSpace(l) != "" {
			valid = append(valid, l)
		}
	}
	if len(valid) < 3 {
		return false
	}

	hits := 0
	for _, l := range valid {
		if rePorcelainLine.MatchString(l) {
			hits++
		}
	}
	return float64(hits)/float64(len(valid)) >= 0.6
}

func isLineNumbered(lines []string) bool {
	hits := 0
	nonEmpty := 0
	sample := lines
	if len(sample) > 100 {
		sample = sample[:100]
	}
	for _, l := range sample {
		if len(l) == 0 {
			continue
		}
		nonEmpty++
		if reReadNumberedLn.MatchString(l) {
			hits++
		}
	}
	if nonEmpty < 5 {
		return false
	}
	return float64(hits)/float64(nonEmpty) >= ReadNumberedMinRatio
}
