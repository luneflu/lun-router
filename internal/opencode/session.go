package opencode

import (
	"crypto/rand"
	"fmt"
	"strings"
	"time"
)

const base62Chars = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

// GenerateSessionID generates canonical OpenCode session: ses_<12 hex><14 base62>
// Fix for GitHub Issue #4781 (Bypass HTTP 403 FreeTierError)
func GenerateSessionID() string {
	nowMs := time.Now().UnixMilli()
	val := (^uint64(nowMs*4096 + 1)) & 0xFFFFFFFFFFFF
	timePart := fmt.Sprintf("%012x", val)

	randomBytes := make([]byte, 14)
	_, _ = rand.Read(randomBytes)
	var randomPart strings.Builder
	for _, b := range randomBytes {
		randomPart.WriteByte(base62Chars[int(b)%62])
	}

	return fmt.Sprintf("ses_%s%s", timePart, randomPart.String())
}
