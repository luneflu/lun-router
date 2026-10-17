package chatgpt

import (
	"bufio"
	"bytes"
	"fmt"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
)

// MemoryUsage reports live RSS memory metrics for the browser process tree.
type MemoryUsage struct {
	Running    bool    `json:"running"`
	PID        int     `json:"pid"`
	ProcessNum int     `json:"process_count"`
	TotalRSSMB float64 `json:"total_rss_mb"`
}

// GetMemoryUsage inspects the process tree rooted at cmd.Process.Pid.
func (c *Client) GetMemoryUsage() MemoryUsage {
	c.mu.Lock()
	cmd := c.cmd
	c.mu.Unlock()

	if cmd == nil || cmd.Process == nil || cmd.Process.Pid <= 0 {
		return MemoryUsage{Running: false}
	}

	rootPID := cmd.Process.Pid
	totalKB, count := inspectTreeRSS(rootPID)
	if count == 0 {
		return MemoryUsage{Running: false}
	}

	return MemoryUsage{
		Running:    true,
		PID:        rootPID,
		ProcessNum: count,
		TotalRSSMB: float64(totalKB) / 1024.0,
	}
}

func inspectTreeRSS(rootPID int) (int64, int) {
	switch runtime.GOOS {
	case "darwin", "linux":
		// ps -eo pid,ppid,rss
		out, err := exec.Command("ps", "-eo", "pid,ppid,rss").Output()
		if err != nil {
			return 0, 0
		}
		return parseUnixTreeRSS(out, rootPID)
	case "windows":
		// wmic or tasklist fallback
		return 0, 1
	default:
		return 0, 0
	}
}

func parseUnixTreeRSS(output []byte, rootPID int) (int64, int) {
	kids := make(map[int][]int)
	rssMap := make(map[int]int64)

	scanner := bufio.NewScanner(bytes.NewReader(output))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 3 {
			continue
		}
		pid, err1 := strconv.Atoi(fields[0])
		ppid, err2 := strconv.Atoi(fields[1])
		rss, err3 := strconv.ParseInt(fields[2], 10, 64)
		if err1 != nil || err2 != nil || err3 != nil {
			continue
		}
		kids[ppid] = append(kids[ppid], pid)
		rssMap[pid] = rss
	}

	// Traverse tree starting from rootPID
	seen := make(map[int]bool)
	queue := []int{rootPID}
	var totalKB int64
	var count int

	for len(queue) > 0 {
		curr := queue[0]
		queue = queue[1:]
		if seen[curr] {
			continue
		}
		seen[curr] = true

		if rss, ok := rssMap[curr]; ok {
			totalKB += rss
			count++
		}
		for _, child := range kids[curr] {
			if !seen[child] {
				queue = append(queue, child)
			}
		}
	}

	return totalKB, count
}

func formatMB(mb float64) string {
	return fmt.Sprintf("%.1f MB", mb)
}
