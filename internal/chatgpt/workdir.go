package chatgpt

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// projectMarkers identify a repository/project root during upward search.
var projectMarkers = []string{
	".git", "go.mod", "package.json", "pnpm-lock.yaml", "package-lock.json",
	"yarn.lock", "bun.lockb", "Cargo.toml", "pyproject.toml", "requirements.txt",
	"pom.xml", "build.gradle", "build.gradle.kts", "composer.json", "Gemfile",
	"mix.exs", "deno.json", "deno.jsonc", ".svelte-kit", "Makefile", "CMakeLists.txt",
}

const maxClimbLevels = 8

var (
	// Explicit "where am I" hints commonly present in agent system prompts.
	workdirHintRes = []*regexp.Regexp{
		regexp.MustCompile(`(?i)(?:working\s+directory|working\s+dir|workdir|cwd|workspace\s+root|workspace|project\s+root|project\s+directory|project\s+dir|root\s+directory)\s*[:=]\s*["'\x60]?([^\s"'\x60<>,;)\]]+)`),
		regexp.MustCompile(`(?i)<cwd>\s*([^<]+?)\s*</cwd>`),
		regexp.MustCompile(`(?i)<(?:working_directory|workspace|project_root|root)>\s*([^<]+?)\s*</(?:working_directory|workspace|project_root|root)>`),
		regexp.MustCompile(`(?i)(?:directory|folder)\s*[:=]\s*["'\x60]?(/[^\s"'\x60<>,;)\]]+)`),
	}

	// Absolute POSIX and Windows paths embedded anywhere in the prompt.
	absPathRe = regexp.MustCompile(`(?:^|[\s"'\x60(\[])((?:[A-Za-z]:\\|/)[A-Za-z0-9._@\-]+(?:[\\/][A-Za-z0-9._@\-]+)+)`)

	urlPrefixRe = regexp.MustCompile(`[a-zA-Z][a-zA-Z0-9+.\-]*://$`)
)

// CandidateWorkDirs returns the ordered list of directories tools should try.
//
// Resolution order:
//  1. explicit cwd/workspace hints in the request (system prompt or metadata)
//  2. project roots inferred from absolute paths in the request
//  3. the configured work directory
//  4. the process working directory
//
// The result is de-duplicated, existing-directory-only, and never empty.
func (c *Client) CandidateWorkDirs(prompt string, body map[string]any) []string {
	var out []string
	seen := map[string]bool{}

	add := func(dir string) {
		if dir == "" {
			return
		}
		dir = strings.TrimSuffix(filepath.Clean(dir), string(os.PathSeparator))
		if dir == "" {
			return
		}
		if seen[dir] {
			return
		}
		info, err := os.Stat(dir)
		if err != nil || !info.IsDir() {
			return
		}
		seen[dir] = true
		out = append(out, dir)
	}

	// 1. Explicit hints.
	for _, hint := range extractWorkdirHints(prompt) {
		add(hint)
		add(climbToProjectRoot(hint))
	}
	for _, hint := range extractWorkdirHintsFromBody(body) {
		add(hint)
		add(climbToProjectRoot(hint))
	}

	// 2. Project roots inferred from absolute paths mentioned in the request.
	for _, p := range extractAbsolutePaths(prompt) {
		dir := p
		if info, err := os.Stat(p); err == nil && !info.IsDir() {
			dir = filepath.Dir(p)
		}
		add(climbToProjectRoot(dir))
		add(dir)
	}

	// 3. Configured directory.
	add(c.cfg.WorkDir)

	// 4. Process working directory.
	if wd, err := os.Getwd(); err == nil {
		add(wd)
	}

	if len(out) == 0 {
		out = append(out, ".")
	}
	return out
}

// DetectWorkDir returns the best single guess for the request's project directory.
func (c *Client) DetectWorkDir(prompt string, body map[string]any) string {
	return c.CandidateWorkDirs(prompt, body)[0]
}

func extractWorkdirHints(text string) []string {
	var out []string
	for _, re := range workdirHintRes {
		for _, m := range re.FindAllStringSubmatch(text, -1) {
			if len(m) < 2 {
				continue
			}
			cand := strings.TrimSpace(m[1])
			cand = strings.Trim(cand, `"'`+"`"+`,`)
			if cand == "" || !filepath.IsAbs(cand) {
				continue
			}
			out = append(out, cand)
		}
	}
	return out
}

// extractWorkdirHintsFromBody looks for structured cwd metadata that agent
// clients often attach outside the message content (e.g. {"cwd": "..."}).
func extractWorkdirHintsFromBody(v any) []string {
	var out []string
	var walk func(node any, key string, depth int)
	walk = func(node any, key string, depth int) {
		if depth > 6 {
			return
		}
		switch t := node.(type) {
		case map[string]any:
			for k, val := range t {
				walk(val, k, depth+1)
			}
		case map[string]string:
			for k, val := range t {
				walk(val, k, depth+1)
			}
		case []any:
			for _, item := range t {
				walk(item, key, depth+1)
			}
		case string:
			lk := strings.ToLower(key)
			if lk == "cwd" || lk == "workdir" || lk == "working_directory" ||
				lk == "workspace" || lk == "project_root" || lk == "root" ||
				lk == "project_path" || lk == "directory" {
				if filepath.IsAbs(t) {
					out = append(out, t)
				}
			}
		}
	}
	walk(v, "", 0)
	return out
}

func extractAbsolutePaths(text string) []string {
	var out []string
	for _, m := range absPathRe.FindAllStringSubmatch(text, -1) {
		if len(m) < 2 {
			continue
		}
		cand := m[1]
		// Skip URL fragments such as https://example.com/path
		idx := strings.Index(text, cand)
		if idx > 0 {
			prefix := text[:idx]
			if urlPrefixRe.MatchString(prefix) {
				continue
			}
		}
		out = append(out, filepath.Clean(cand))
	}
	return out
}

// climbToProjectRoot walks up from dir until a directory containing a project
// marker is found. Returns dir unchanged when no marker exists.
func climbToProjectRoot(dir string) string {
	if dir == "" {
		return ""
	}
	cur := filepath.Clean(dir)
	if info, err := os.Stat(cur); err != nil || !info.IsDir() {
		cur = filepath.Dir(cur)
	}
	start := cur
	for i := 0; i < maxClimbLevels; i++ {
		if hasProjectMarker(cur) {
			return cur
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			break
		}
		cur = parent
	}
	return start
}

func hasProjectMarker(dir string) bool {
	for _, marker := range projectMarkers {
		if _, err := os.Stat(filepath.Join(dir, marker)); err == nil {
			return true
		}
	}
	return false
}

// resolveExistingPath resolves a candidate path against the ordered base list,
// returning the first form that exists on disk. Absolute candidates bypass the
// base list. When nothing exists, the first base is used so callers can produce
// a meaningful error message.
func resolveExistingPath(bases []string, candidate string) string {
	p, _ := resolveExistingPathBase(bases, candidate)
	return p
}

// resolveExistingPathBase also reports which base directory matched.
func resolveExistingPathBase(bases []string, candidate string) (string, string) {
	candidate = strings.TrimPrefix(strings.TrimSpace(candidate), "@")
	candidate = os.ExpandEnv(candidate)
	if candidate == "" {
		return "", ""
	}
	if filepath.IsAbs(candidate) {
		return filepath.Clean(candidate), filepath.Dir(candidate)
	}
	var fallback string
	var fallbackBase string
	for _, base := range bases {
		p := filepath.Clean(filepath.Join(base, candidate))
		if fallback == "" {
			fallback = p
			fallbackBase = base
		}
		if _, err := os.Stat(p); err == nil {
			return p, base
		}
	}
	return fallback, fallbackBase
}
