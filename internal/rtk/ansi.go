package rtk

import (
	"regexp"
	"strings"
)

var reAnsi = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]|\x1b\([a-zA-Z]|\x1b[=>]`)

// StripAnsi removes ANSI escape codes from terminal/command outputs
func StripAnsi(s string) string {
	if !strings.Contains(s, "\x1b") {
		return s
	}
	return reAnsi.ReplaceAllString(s, "")
}
