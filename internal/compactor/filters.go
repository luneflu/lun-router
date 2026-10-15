package compactor

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

var (
	reLsDate          = regexp.MustCompile(`\s+(Jan|Feb|Mar|Apr|May|Jun|Jul|Aug|Sep|Oct|Nov|Dec)\s+\d{1,2}\s+(\d{4}|\d{2}:\d{2})\s+`)
	reSearchListHdr   = regexp.MustCompile(`^Result of search in '[^']*' \(total (\d+) files?\):`)
	reReadNumberedLn  = regexp.MustCompile(`^\s*\d+\|`)
	reCargoErrCont    = regexp.MustCompile(`^\s*(-->|\||\d+\s*\||=)`)
	reCommitHeader    = regexp.MustCompile(`(?i)^[*|/\\ ]*commit [0-9a-f]{7,40}`)
	reCommitGraph     = regexp.MustCompile(`(?i)^[*|/\\ ]+([0-9a-f]{7,40}\s+.+)`)
	reCommitPlain     = regexp.MustCompile(`^[0-9a-f]{7,40}\s+`)
	reCommitPureGraph = regexp.MustCompile(`^[*|/\\ ]+$`)
	// rePorcelainLine requires a path, so indented text (pretty JSON,
	// wrapped logs) is not mistaken for `git status --porcelain`.
	rePorcelainLine = regexp.MustCompile(`^[ MADRCU?!][ MADRCU?!] \S`)
	reISOTime       = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}[T ]\d{2}:\d{2}`)
	reErrorSignal   = regexp.MustCompile(`(?i)\b(fatal|panic|traceback|unhandled|segmentation fault|sigsegv)\b`)
	reLongStatus    = regexp.MustCompile(`^\s*(modified|new file|deleted|renamed|both modified):\s+(.+)$`)
	reHunkHeader    = regexp.MustCompile(`^@@\s+-[0-9,]+\s+\+[0-9,]+\s+@@(.*)$`)
)

func diffHeaderPath(line string) string {
	if strings.HasPrefix(line, "diff --git") {
		parts := strings.Split(line, " b/")
		if len(parts) > 1 {
			return strings.Join(parts[1:], " b/")
		}
	} else if strings.HasPrefix(line, "diff --cc ") {
		return strings.TrimSpace(strings.TrimPrefix(line, "diff --cc "))
	} else if strings.HasPrefix(line, "diff --combined ") {
		return strings.TrimSpace(strings.TrimPrefix(line, "diff --combined "))
	}
	return "unknown"
}

func hunkTruncationNote(deletions, additions int) string {
	count := func(n int, noun string) string {
		if n == 1 {
			return fmt.Sprintf("1 %s", noun)
		}
		return fmt.Sprintf("%d %ss", n, noun)
	}
	if deletions == 0 && additions == 0 {
		return ""
	}
	if deletions == 0 {
		return fmt.Sprintf("  ... (%s truncated)", count(additions, "addition"))
	}
	if additions == 0 {
		return fmt.Sprintf("  ... (%s truncated)", count(deletions, "deletion"))
	}
	return fmt.Sprintf("  ... (%s, %s truncated)", count(deletions, "deletion"), count(additions, "addition"))
}

// GitDiff compacts unified or combined diffs: headers, hunk-level truncation, leading context, +/- counting
func GitDiff(diff string, maxLines int) string {
	diff = StripAnsi(diff)
	if maxLines <= 0 {
		maxLines = GitDiffMaxLines
	}
	var result []string
	currentFile := ""
	added := 0
	removed := 0
	inHunk := false
	hunkShown := 0
	skippedAdd := 0
	skippedDel := 0
	wasTruncated := false
	var leadingContext []string
	const maxLeadingContext = 3

	flushLeading := func() {
		for _, ctxLine := range leadingContext {
			result = append(result, ctxLine)
		}
		leadingContext = nil
	}

	flushHunkTruncation := func() {
		if note := hunkTruncationNote(skippedDel, skippedAdd); note != "" {
			result = append(result, note)
			wasTruncated = true
			skippedDel = 0
			skippedAdd = 0
		}
	}

	lines := strings.Split(diff, "\n")

outer:
	for _, line := range lines {
		if strings.HasPrefix(line, "diff --") {
			flushLeading()
			flushHunkTruncation()
			if currentFile != "" && (added > 0 || removed > 0) {
				result = append(result, fmt.Sprintf("  +%d -%d", added, removed))
			}
			currentFile = diffHeaderPath(line)
			result = append(result, "\n"+currentFile)
			added = 0
			removed = 0
			inHunk = false
			hunkShown = 0
		} else if strings.HasPrefix(line, "@@") {
			flushLeading()
			flushHunkTruncation()
			inHunk = true
			hunkShown = 0
			result = append(result, "  "+line)
		} else if inHunk {
			isAdd := strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++")
			isDel := strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---")

			if isAdd || isDel {
				if isAdd {
					added++
				}
				if isDel {
					removed++
				}
				if hunkShown < GitDiffHunkMaxLines {
					flushLeading()
					result = append(result, "  "+line)
					hunkShown++
				} else if isDel {
					skippedDel++
				} else {
					skippedAdd++
				}
				leadingContext = nil
			} else if hunkShown > 0 {
				if hunkShown < GitDiffHunkMaxLines && !strings.HasPrefix(line, "\\") {
					result = append(result, "  "+line)
					hunkShown++
				}
			} else if !strings.HasPrefix(line, "\\") {
				if len(leadingContext) == maxLeadingContext {
					leadingContext = leadingContext[1:]
				}
				leadingContext = append(leadingContext, "  "+line)
			}
		}

		if len(result) >= maxLines {
			result = append(result, "\n... (more changes truncated)")
			wasTruncated = true
			break outer
		}
	}

	flushLeading()
	flushHunkTruncation()

	if currentFile != "" && (added > 0 || removed > 0) {
		result = append(result, fmt.Sprintf("  +%d -%d", added, removed))
	}
	if wasTruncated {
		result = append(result, "[full diff: rtk git diff --no-compact]")
	}

	return strings.Join(result, "\n")
}

// GitStatus produces compact git status summary
func GitStatus(input string) string {
	clean := StripAnsi(input)
	if strings.TrimSpace(clean) == "" {
		return "Clean working tree"
	}
	// Only summarize real git status output; never fabricate "Clean working tree"
	// for content that merely shares the leading whitespace pattern.
	if !looksLikeGitStatus(clean) {
		return input
	}
	input = clean
	lines := strings.Split(input, "\n")
	if len(lines) == 0 || (len(lines) == 1 && strings.TrimSpace(lines[0]) == "") {
		return "Clean working tree"
	}

	branch := ""
	var stagedFiles, modifiedFiles, untrackedFiles []string
	var states []string
	staged := 0
	modified := 0
	untracked := 0
	conflicts := 0

	for _, raw := range lines {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			continue
		}

		if strings.Contains(raw, "rebase in progress") || strings.Contains(raw, "You are currently rebasing") {
			states = append(states, "rebase in progress")
		} else if strings.Contains(raw, "You have unmerged paths") {
			states = append(states, "merge in progress. unresolved conflicts")
		} else if strings.Contains(raw, "All conflicts fixed but you are still merging") {
			states = append(states, "merge in progress. no conflicts")
		} else if strings.Contains(raw, "You are currently cherry-picking") {
			states = append(states, "cherry-pick in progress")
		}

		if strings.HasPrefix(raw, "On branch ") {
			fields := strings.Fields(raw)
			if len(fields) >= 3 {
				branch = fields[2]
			}
			continue
		}

		if strings.HasPrefix(raw, "##") {
			branch = strings.TrimSpace(strings.TrimPrefix(raw, "##"))
			continue
		}

		if rePorcelainLine.MatchString(raw) {
			x := raw[0]
			y := raw[1]
			file := raw[3:]

			if raw[:2] == "??" {
				untracked++
				untrackedFiles = append(untrackedFiles, file)
				continue
			}

			if strings.ContainsRune("MADRC", rune(x)) {
				staged++
				stagedFiles = append(stagedFiles, file)
			} else if x == 'U' {
				conflicts++
			}

			if y == 'M' || y == 'D' {
				modified++
				modifiedFiles = append(modifiedFiles, file)
			}
			continue
		}

		if m := reLongStatus.FindStringSubmatch(raw); len(m) == 3 {
			kind := m[1]
			path := strings.TrimSpace(m[2])
			switch kind {
			case "both modified":
				conflicts++
			case "modified", "deleted":
				modified++
				modifiedFiles = append(modifiedFiles, path)
			case "new file", "renamed":
				staged++
				stagedFiles = append(stagedFiles, path)
			}
			continue
		}
	}

	var out strings.Builder
	if branch == "" && len(states) == 0 && staged+modified+untracked+conflicts == 0 {
		// Nothing parsed as status output: pass the original through rather than
		// claiming the tree is clean.
		return input
	}

	if branch != "" {
		out.WriteString(fmt.Sprintf("* %s\n", branch))
	}
	for _, st := range states {
		out.WriteString(fmt.Sprintf("! %s\n", st))
	}

	if staged > 0 {
		out.WriteString(fmt.Sprintf("+ Staged: %d files\n", staged))
		limit := StatusMaxFiles
		if len(stagedFiles) < limit {
			limit = len(stagedFiles)
		}
		for _, f := range stagedFiles[:limit] {
			out.WriteString(fmt.Sprintf("   %s\n", f))
		}
		if len(stagedFiles) > StatusMaxFiles {
			out.WriteString(fmt.Sprintf("   ... +%d more\n", len(stagedFiles)-StatusMaxFiles))
		}
	}

	if modified > 0 {
		out.WriteString(fmt.Sprintf("~ Modified: %d files\n", modified))
		limit := StatusMaxFiles
		if len(modifiedFiles) < limit {
			limit = len(modifiedFiles)
		}
		for _, f := range modifiedFiles[:limit] {
			out.WriteString(fmt.Sprintf("   %s\n", f))
		}
		if len(modifiedFiles) > StatusMaxFiles {
			out.WriteString(fmt.Sprintf("   ... +%d more\n", len(modifiedFiles)-StatusMaxFiles))
		}
	}

	if untracked > 0 {
		out.WriteString(fmt.Sprintf("? Untracked: %d files\n", untracked))
		limit := StatusMaxUntracked
		if len(untrackedFiles) < limit {
			limit = len(untrackedFiles)
		}
		for _, f := range untrackedFiles[:limit] {
			out.WriteString(fmt.Sprintf("   %s\n", f))
		}
		if len(untrackedFiles) > StatusMaxUntracked {
			out.WriteString(fmt.Sprintf("   ... +%d more\n", len(untrackedFiles)-StatusMaxUntracked))
		}
	}

	if conflicts > 0 {
		out.WriteString(fmt.Sprintf("conflicts: %d files\n", conflicts))
	}

	res := strings.TrimRight(out.String(), "\n")
	if res == "" || res == fmt.Sprintf("* %s", branch) {
		if branch != "" {
			return fmt.Sprintf("* %s\nclean — nothing to commit", branch)
		}
		return "Clean working tree"
	}
	return res
}

// GitLog compresses git log output: keeps commit headers, subjects, Author/Date; drops diffs/body
func GitLog(text string, maxLines int) string {
	text = StripAnsi(text)
	if text == "" {
		return ""
	}
	if maxLines <= 0 {
		maxLines = GitLogMaxLines
	}

	lines := strings.Split(text, "\n")
	var out []string
	skipped := 0
	inCommit := false
	subjectSeen := false

	pushLine := func(l string) bool {
		if len(out) < maxLines {
			out = append(out, l)
			return true
		}
		skipped++
		return false
	}

	for _, raw := range lines {
		line := strings.TrimRight(raw, " \t\r")
		trimmed := strings.TrimSpace(line)

		if reCommitHeader.MatchString(trimmed) {
			inCommit = true
			subjectSeen = false
			pushLine(line)
			continue
		}

		if inCommit {
			if strings.HasPrefix(trimmed, "Author:") || strings.HasPrefix(trimmed, "Date:") {
				pushLine(trimmed)
				continue
			}
			if trimmed == "" {
				continue
			}
			if !subjectSeen && (strings.HasPrefix(line, "    ") || strings.Contains(line, "    ")) {
				pushLine("  Subject: " + trimmed)
				subjectSeen = true
				continue
			}
			if strings.Contains(trimmed, " changed, ") && (strings.Contains(trimmed, " insertion") || strings.Contains(trimmed, " deletion")) {
				pushLine("  " + trimmed)
				continue
			}
			if strings.HasPrefix(trimmed, "diff --git ") {
				pushLine("  ... diff body omitted")
				continue
			}
			continue
		}

		if m := reCommitGraph.FindStringSubmatch(trimmed); len(m) == 2 {
			pushLine(m[1])
			continue
		}

		if reCommitPlain.MatchString(trimmed) {
			pushLine(trimmed)
			continue
		}

		if reCommitPureGraph.MatchString(trimmed) && (strings.ContainsAny(trimmed, "*|/\\")) {
			continue
		}

		pushLine(trimmed)
	}

	if skipped > 0 {
		out = append(out, fmt.Sprintf("... (%d more lines)", skipped))
	}

	result := strings.Join(out, "\n")
	if result == "" && text != "" {
		return text
	}
	if len(result) > len(text) {
		return text
	}
	return result
}

// Grep groups matches by file and caps lines per file (RTK format)
func Grep(input string) string {
	input = StripAnsi(input)
	byFile := make(map[string][][2]string)
	var fileOrder []string
	total := 0

	for _, line := range strings.Split(input, "\n") {
		first := strings.Index(line, ":")
		if first == -1 {
			continue
		}
		second := strings.Index(line[first+1:], ":")
		if second == -1 {
			continue
		}
		second += first + 1

		file := line[:first]
		lineNumStr := line[first+1 : second]
		content := line[second+1:]

		if _, err := strconv.Atoi(lineNumStr); err != nil {
			continue
		}

		total++
		if _, exists := byFile[file]; !exists {
			fileOrder = append(fileOrder, file)
		}
		byFile[file] = append(byFile[file], [2]string{lineNumStr, content})
	}

	if total == 0 {
		return input
	}

	sort.Strings(fileOrder)
	var out strings.Builder
	out.WriteString(fmt.Sprintf("%d matches in %d files:\n\n", total, len(fileOrder)))

	for _, file := range fileOrder {
		matches := byFile[file]
		out.WriteString(fmt.Sprintf("[file] %s (%d):\n", file, len(matches)))
		limit := GrepPerFileMax
		if len(matches) < limit {
			limit = len(matches)
		}
		for _, m := range matches[:limit] {
			out.WriteString(fmt.Sprintf("  %4s: %s\n", m[0], strings.TrimSpace(m[1])))
		}
		if len(matches) > GrepPerFileMax {
			out.WriteString(fmt.Sprintf("  +%d more in %s\n", len(matches)-GrepPerFileMax, file))
		}
		out.WriteString("\n")
	}

	return out.String()
}

// Find groups paths by directory, shows basenames, caps 10/dir and 20 dirs (RTK format)
func Find(input string) string {
	input = StripAnsi(input)
	lines := strings.Split(input, "\n")
	var valid []string
	for _, l := range lines {
		if strings.TrimSpace(l) != "" {
			valid = append(valid, l)
		}
	}
	if len(valid) == 0 {
		return input
	}

	byDir := make(map[string][]string)
	for _, path := range valid {
		lastSep := strings.LastIndexAny(path, "/\\")
		var dir, basename string
		if lastSep == -1 {
			dir = "."
			basename = path
		} else {
			dir = path[:lastSep]
			if dir == "" {
				dir = "/"
			}
			basename = path[lastSep+1:]
		}
		byDir[dir] = append(byDir[dir], basename)
	}

	dirs := make([]string, 0, len(byDir))
	for d := range byDir {
		dirs = append(dirs, d)
	}
	sort.Strings(dirs)

	var out strings.Builder
	out.WriteString(fmt.Sprintf("%d files in %d dirs:\n\n", len(valid), len(dirs)))

	limit := FindTotalDirMax
	if len(dirs) < limit {
		limit = len(dirs)
	}

	for _, dir := range dirs[:limit] {
		files := byDir[dir]
		dirLabel := strings.ReplaceAll(dir, "\\", "/")
		out.WriteString(fmt.Sprintf("%s/  (%d)\n", dirLabel, len(files)))
		fLimit := FindPerDirMax
		if len(files) < fLimit {
			fLimit = len(files)
		}
		for _, f := range files[:fLimit] {
			out.WriteString(fmt.Sprintf("  %s\n", f))
		}
		if len(files) > FindPerDirMax {
			out.WriteString(fmt.Sprintf("  +%d\n", len(files)-FindPerDirMax))
		}
	}

	if len(dirs) > FindTotalDirMax {
		out.WriteString(fmt.Sprintf("\n+%d more dirs\n", len(dirs)-FindTotalDirMax))
	}

	return out.String()
}

func humanSize(bytes int64) string {
	if bytes >= 1048576 {
		return fmt.Sprintf("%.1fM", float64(bytes)/1048576.0)
	}
	if bytes >= 1024 {
		return fmt.Sprintf("%.1fK", float64(bytes)/1024.0)
	}
	return fmt.Sprintf("%dB", bytes)
}

// Ls produces compact ls -la output
func Ls(input string) string {
	input = StripAnsi(input)
	var dirs []string
	var files [][2]string
	byExt := make(map[string]int)

	for _, line := range strings.Split(input, "\n") {
		if strings.HasPrefix(line, "total ") || len(line) == 0 {
			continue
		}

		m := reLsDate.FindStringIndex(line)
		if m == nil {
			continue
		}

		name := line[m[1]:]
		beforeDate := line[:m[0]]
		parts := strings.Fields(beforeDate)
		if len(parts) < 4 {
			continue
		}

		perms := parts[0]
		fileType := string(perms[0])

		var size int64
		for i := len(parts) - 1; i >= 0; i-- {
			if n, err := strconv.ParseInt(parts[i], 10, 64); err == nil {
				size = n
				break
			}
		}

		if name == "." || name == ".." || LsNoiseDirs[name] {
			continue
		}

		if fileType == "d" {
			dirs = append(dirs, name)
		} else if fileType == "-" || fileType == "l" {
			dot := strings.LastIndex(name, ".")
			ext := "no ext"
			if dot > 0 {
				ext = name[dot:]
			}
			byExt[ext]++
			files = append(files, [2]string{name, humanSize(size)})
		}
	}

	if len(dirs) == 0 && len(files) == 0 {
		return input
	}

	var out strings.Builder
	for _, d := range dirs {
		out.WriteString(d + "/\n")
	}
	for _, f := range files {
		out.WriteString(fmt.Sprintf("%s  %s\n", f[0], f[1]))
	}

	out.WriteString(fmt.Sprintf("\nSummary: %d files, %d dirs", len(files), len(dirs)))
	if len(byExt) > 0 {
		type extEntry struct {
			ext string
			cnt int
		}
		var list []extEntry
		for e, c := range byExt {
			list = append(list, extEntry{e, c})
		}
		sort.Slice(list, func(i, j int) bool {
			return list[i].cnt > list[j].cnt
		})

		top := LsExtSummaryTop
		if len(list) < top {
			top = len(list)
		}
		var parts []string
		for _, e := range list[:top] {
			parts = append(parts, fmt.Sprintf("%d %s", e.cnt, e.ext))
		}
		out.WriteString(" (" + strings.Join(parts, ", "))
		if len(list) > LsExtSummaryTop {
			out.WriteString(fmt.Sprintf(", +%d more", len(list)-LsExtSummaryTop))
		}
		out.WriteString(")")
	}

	return out.String()
}

// Tree strips summary line and caps long trees
func Tree(input string) string {
	input = StripAnsi(input)
	lines := strings.Split(input, "\n")
	if len(lines) == 0 {
		return input
	}

	var filtered []string
	for _, line := range lines {
		if strings.Contains(line, "director") && strings.Contains(line, "file") {
			continue
		}
		if strings.TrimSpace(line) == "" && len(filtered) == 0 {
			continue
		}
		filtered = append(filtered, line)
	}

	for len(filtered) > 0 && strings.TrimSpace(filtered[len(filtered)-1]) == "" {
		filtered = filtered[:len(filtered)-1]
	}

	if len(filtered) > TreeMaxLines {
		cut := len(filtered) - TreeMaxLines
		return strings.Join(filtered[:TreeMaxLines], "\n") + fmt.Sprintf("\n... +%d more lines", cut)
	}

	return strings.Join(filtered, "\n")
}

// DedupLog collapses consecutive duplicate lines
func DedupLog(input string) string {
	input = StripAnsi(input)
	lines := strings.Split(input, "\n")
	var out []string
	var prev *string
	runCount := 0
	blankStreak := 0

	flushRun := func() {
		if prev != nil && runCount > 1 {
			out = append(out, fmt.Sprintf("  ... (%d duplicate lines)", runCount-1))
		}
	}

	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			if blankStreak < 1 {
				out = append(out, line)
			}
			blankStreak++
			flushRun()
			prev = nil
			runCount = 0
			continue
		}
		blankStreak = 0
		if prev != nil && line == *prev {
			runCount++
			continue
		}
		flushRun()
		out = append(out, line)
		p := line
		prev = &p
		runCount = 1

		if len(out) >= DedupLineMax {
			out = append(out, fmt.Sprintf("... (truncated at %d lines)", DedupLineMax))
			return strings.Join(out, "\n")
		}
	}
	flushRun()
	return strings.Join(out, "\n")
}

// SmartTruncate keeps head and tail, drops middle
func SmartTruncate(input string) string {
	input = StripAnsi(input)
	lines := strings.Split(input, "\n")
	if len(lines) < SmartTruncateMinLines {
		return input
	}

	head := lines[:SmartTruncateHead]
	tail := lines[len(lines)-SmartTruncateTail:]
	cut := len(lines) - len(head) - len(tail)
	return strings.Join(append(append(head, fmt.Sprintf("... +%d lines truncated", cut)), tail...), "\n")
}

// ReadNumbered truncates line-numbered files
func ReadNumbered(input string) string {
	input = StripAnsi(input)
	lines := strings.Split(input, "\n")
	if len(lines) < SmartTruncateMinLines {
		return input
	}

	head := lines[:SmartTruncateHead]
	tail := lines[len(lines)-SmartTruncateTail:]
	cut := len(lines) - len(head) - len(tail)
	return strings.Join(append(append(head, fmt.Sprintf("... +%d lines truncated (file continues)", cut)), tail...), "\n")
}

// SearchList compacts Cursor Glob search list
func SearchList(input string) string {
	input = StripAnsi(input)
	lines := strings.Split(input, "\n")
	if len(lines) == 0 {
		return input
	}

	header := lines[0]
	var paths []string
	for _, raw := range lines[1:] {
		t := strings.TrimSpace(raw)
		if strings.HasPrefix(t, "- ") {
			paths = append(paths, t[2:])
		}
	}
	if len(paths) == 0 {
		return input
	}

	byDir := make(map[string][]string)
	for _, p := range paths {
		slash := strings.LastIndex(p, "/")
		var dir, name string
		if slash == -1 {
			dir = "."
			name = p
		} else {
			dir = p[:slash]
			if dir == "" {
				dir = "/"
			}
			name = p[slash+1:]
		}
		byDir[dir] = append(byDir[dir], name)
	}

	dirs := make([]string, 0, len(byDir))
	for d := range byDir {
		dirs = append(dirs, d)
	}
	sort.Strings(dirs)

	var out strings.Builder
	out.WriteString(fmt.Sprintf("%s\n%d files in %d dirs:\n\n", header, len(paths), len(dirs)))

	limit := SearchListTotalDirMax
	if len(dirs) < limit {
		limit = len(dirs)
	}

	for _, dir := range dirs[:limit] {
		names := byDir[dir]
		out.WriteString(fmt.Sprintf("%s/ (%d):\n", dir, len(names)))
		nLimit := SearchListPerDirMax
		if len(names) < nLimit {
			nLimit = len(names)
		}
		for _, n := range names[:nLimit] {
			out.WriteString(fmt.Sprintf("  %s\n", n))
		}
		if len(names) > SearchListPerDirMax {
			out.WriteString(fmt.Sprintf("  +%d\n", len(names)-SearchListPerDirMax))
		}
		out.WriteString("\n")
	}

	if len(dirs) > SearchListTotalDirMax {
		out.WriteString(fmt.Sprintf("+%d more dirs\n", len(dirs)-SearchListTotalDirMax))
	}

	return strings.TrimRight(out.String(), "\n")
}

// CargoTest compresses cargo test output (RTK port)
func CargoTest(input string) string {
	input = StripAnsi(input)
	lines := strings.Split(input, "\n")
	var failures []string
	var summaryLines []string
	inFailureSection := false
	var currentFailure []string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		if strings.HasPrefix(trimmed, "Compiling ") || strings.HasPrefix(trimmed, "Downloading ") ||
			strings.HasPrefix(trimmed, "Downloaded ") || strings.HasPrefix(trimmed, "Finished ") {
			continue
		}

		if strings.HasPrefix(line, "running ") || (strings.HasPrefix(line, "test ") && strings.HasSuffix(line, "... ok")) {
			continue
		}

		if line == "failures:" {
			inFailureSection = true
			continue
		}

		if inFailureSection {
			if strings.HasPrefix(line, "test result:") {
				inFailureSection = false
				summaryLines = append(summaryLines, line)
			} else if strings.HasPrefix(line, "    ") || strings.HasPrefix(line, "---- ") {
				currentFailure = append(currentFailure, line)
			} else if trimmed == "" && len(currentFailure) > 0 {
				failures = append(failures, strings.Join(currentFailure, "\n"))
				currentFailure = nil
			} else if trimmed != "" {
				currentFailure = append(currentFailure, line)
			}
		}

		if !inFailureSection && strings.HasPrefix(line, "test result:") {
			summaryLines = append(summaryLines, line)
		}
	}

	if len(currentFailure) > 0 {
		failures = append(failures, strings.Join(currentFailure, "\n"))
	}

	var out strings.Builder
	if len(failures) > 0 {
		out.WriteString("failures:\n\n")
		for _, f := range failures {
			out.WriteString(f + "\n\n")
		}
	}
	for _, s := range summaryLines {
		out.WriteString(s + "\n")
	}

	res := strings.TrimRight(out.String(), "\n")
	if res == "" {
		return input
	}
	return res
}

// Pytest compresses pytest output (RTK port)
func Pytest(input string) string {
	input = StripAnsi(input)
	lines := strings.Split(input, "\n")
	var failures []string
	var currentFailure []string
	summaryLine := ""
	inFailures := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		if strings.HasPrefix(trimmed, "===") && strings.Contains(trimmed, "FAILURES") {
			inFailures = true
			continue
		} else if strings.HasPrefix(trimmed, "===") && strings.Contains(trimmed, "short test summary") {
			inFailures = false
			if len(currentFailure) > 0 {
				failures = append(failures, strings.Join(currentFailure, "\n"))
				currentFailure = nil
			}
			continue
		} else if strings.HasPrefix(trimmed, "===") && (strings.Contains(trimmed, "passed") || strings.Contains(trimmed, "failed") || strings.Contains(trimmed, "skipped")) {
			summaryLine = trimmed
			continue
		} else if summaryLine == "" && (strings.Contains(trimmed, " passed") || strings.Contains(trimmed, " failed")) && strings.Contains(trimmed, " in ") {
			summaryLine = trimmed
			continue
		}

		if inFailures {
			if strings.HasPrefix(trimmed, "___") {
				if len(currentFailure) > 0 {
					failures = append(failures, strings.Join(currentFailure, "\n"))
					currentFailure = nil
				}
			}
			currentFailure = append(currentFailure, line)
		}
	}

	if len(currentFailure) > 0 {
		failures = append(failures, strings.Join(currentFailure, "\n"))
	}

	var out strings.Builder
	if len(failures) > 0 {
		out.WriteString("FAILURES:\n")
		for _, f := range failures {
			out.WriteString(f + "\n\n")
		}
	}
	if summaryLine != "" {
		out.WriteString(summaryLine + "\n")
	}

	res := strings.TrimRight(out.String(), "\n")
	if res == "" {
		return input
	}
	return res
}

// GoTest compresses go test output (RTK port)
func GoTest(input string) string {
	input = StripAnsi(input)
	lines := strings.Split(input, "\n")
	var failures []string
	var currentFailure []string
	var packageResults []string
	inFail := false

	for _, line := range lines {
		if strings.HasPrefix(line, "=== RUN") || strings.HasPrefix(line, "--- PASS:") {
			if inFail && len(currentFailure) > 0 {
				failures = append(failures, strings.Join(currentFailure, "\n"))
				currentFailure = nil
				inFail = false
			}
			continue
		}

		if strings.HasPrefix(line, "--- FAIL:") {
			if inFail && len(currentFailure) > 0 {
				failures = append(failures, strings.Join(currentFailure, "\n"))
				currentFailure = nil
			}
			inFail = true
			currentFailure = append(currentFailure, line)
			continue
		}

		if inFail {
			if strings.HasPrefix(line, "FAIL") || strings.HasPrefix(line, "ok  ") {
				failures = append(failures, strings.Join(currentFailure, "\n"))
				currentFailure = nil
				inFail = false
				packageResults = append(packageResults, line)
				continue
			}
			currentFailure = append(currentFailure, line)
			continue
		}

		if strings.HasPrefix(line, "FAIL") || strings.HasPrefix(line, "ok  ") {
			packageResults = append(packageResults, line)
		}
	}

	if len(currentFailure) > 0 {
		failures = append(failures, strings.Join(currentFailure, "\n"))
	}

	var out strings.Builder
	for _, f := range failures {
		out.WriteString(f + "\n")
	}
	for _, p := range packageResults {
		out.WriteString(p + "\n")
	}

	res := strings.TrimRight(out.String(), "\n")
	if res == "" {
		return input
	}
	return res
}

// BuildOutput compresses build tool output (npm, cargo, etc.)
func BuildOutput(input string) string {
	input = StripAnsi(input)
	lines := strings.Split(input, "\n")
	if len(lines) == 0 {
		return input
	}

	var errors, warnings, deprecations []string
	summary := ""
	compilingCount := 0
	downloadingCount := 0
	inCargoError := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		if inCargoError {
			if trimmed == "" {
				inCargoError = false
				continue
			}
			if reCargoErrCont.MatchString(line) {
				errors = append(errors, line)
				continue
			}
			inCargoError = false
		}

		if trimmed == "" {
			continue
		}

		low := strings.ToLower(trimmed)

		if strings.HasPrefix(low, "npm err!") || strings.HasPrefix(low, "npm error") || strings.HasPrefix(low, "yarn error") {
			errors = append(errors, line)
			continue
		}

		if strings.HasPrefix(low, "npm warn deprecated") {
			deprecations = append(deprecations, line)
			continue
		}

		if strings.HasPrefix(low, "npm warn") || strings.HasPrefix(low, "yarn warn") {
			warnings = append(warnings, line)
			continue
		}

		if strings.HasPrefix(low, "error[") || strings.HasPrefix(low, "error:") || strings.HasPrefix(low, "error -->") {
			errors = append(errors, line)
			inCargoError = true
			continue
		}

		if strings.HasPrefix(low, "warning[") || strings.HasPrefix(low, "warning:") || strings.HasPrefix(low, "warning -->") {
			warnings = append(warnings, line)
			inCargoError = true
			continue
		}

		if strings.HasPrefix(trimmed, "ERROR:") || strings.HasPrefix(trimmed, "[ERROR]") || strings.HasPrefix(trimmed, "BUILD FAILED") {
			errors = append(errors, line)
			continue
		}

		if strings.HasPrefix(trimmed, "[WARNING]") {
			warnings = append(warnings, line)
			continue
		}

		if strings.HasPrefix(trimmed, "Compiling ") || strings.Contains(trimmed, " Compiling ") {
			compilingCount++
			continue
		}

		if strings.HasPrefix(trimmed, "Downloading ") || strings.Contains(trimmed, " Downloading ") || strings.HasPrefix(trimmed, "Fetching ") {
			downloadingCount++
			continue
		}

		if strings.HasPrefix(low, "added ") || strings.HasPrefix(low, "removed ") || strings.HasPrefix(low, "changed ") ||
			strings.HasPrefix(low, "audited ") || strings.HasPrefix(low, "installed ") || strings.HasPrefix(low, "finished ") ||
			strings.HasPrefix(trimmed, "BUILD SUCCESS") || strings.Contains(low, "vulnerabilities") ||
			strings.HasPrefix(low, "successfully installed") || strings.HasPrefix(low, "successfully built") {
			if summary == "" {
				summary = line
			} else {
				summary = summary + "\n" + line
			}
			continue
		}
	}

	var out strings.Builder

	depLimit := 3
	if len(deprecations) < depLimit {
		depLimit = len(deprecations)
	}
	for _, d := range deprecations[:depLimit] {
		out.WriteString(d + "\n")
	}
	if len(deprecations) > 3 {
		out.WriteString(fmt.Sprintf("... +%d more deprecated packages\n", len(deprecations)-3))
	}

	if compilingCount > 0 {
		out.WriteString(fmt.Sprintf("Compiled %d packages\n", compilingCount))
	}
	if downloadingCount > 0 {
		out.WriteString(fmt.Sprintf("Downloaded %d packages\n", downloadingCount))
	}

	for _, e := range errors {
		out.WriteString(e + "\n")
	}

	warnLimit := 5
	if len(warnings) < warnLimit {
		warnLimit = len(warnings)
	}
	for _, w := range warnings[:warnLimit] {
		out.WriteString(w + "\n")
	}
	if len(warnings) > 5 {
		out.WriteString(fmt.Sprintf("... +%d more warnings\n", len(warnings)-5))
	}

	if summary != "" {
		out.WriteString(summary + "\n")
	}

	res := strings.TrimRight(out.String(), "\n")
	if res == "" {
		return input
	}
	return res
}
