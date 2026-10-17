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
	return ProcessTreeUsage(cmd.Process.Pid)
}

// ProcessTreeUsage sums resident memory over the process tree rooted at rootPID.
//
// It is exported because internal/zai launches the same Chromium build for
// captcha minting and needs identical numbers; duplicating the traversal would
// let the two providers drift.
func ProcessTreeUsage(rootPID int) MemoryUsage {
	if rootPID <= 0 {
		return MemoryUsage{Running: false}
	}
	totalKB, count, measured := inspectTreeRSS(rootPID)
	return usageFromTree(rootPID, totalKB, count, measured)
}

// usageFromTree decides whether a process tree reading describes a running
// browser.
//
// The zero-RSS case matters: a process that has exited but not yet been reaped
// still has a ps row, with RSS 0 and no children. Reporting that as "running, 0
// MB" both surfaces a stale pid and makes a stopped provider look alive. When the
// platform cannot measure RSS at all (measured false) the process is reported as
// alive with no memory figure rather than as stopped.
func usageFromTree(rootPID int, totalKB int64, count int, measured bool) MemoryUsage {
	if count == 0 {
		return MemoryUsage{Running: false}
	}
	if measured && totalKB <= 0 {
		return MemoryUsage{Running: false}
	}
	return MemoryUsage{
		Running:    true,
		PID:        rootPID,
		ProcessNum: count,
		TotalRSSMB: float64(totalKB) / 1024.0,
	}
}

// inspectTreeRSS reports summed RSS in KB, the process count, and whether the
// platform could actually measure memory. The last value is false when the
// reading is unavailable, so callers can distinguish "no processes" from "cannot
// tell" instead of treating zero as a measurement.
func inspectTreeRSS(rootPID int) (int64, int, bool) {
	switch runtime.GOOS {
	case "darwin", "linux":
		// ps -eo pid,ppid,rss
		out, err := exec.Command("ps", "-eo", "pid,ppid,rss").Output()
		if err != nil {
			return 0, 0, false
		}
		totalKB, count := parseUnixTreeRSS(out, rootPID)
		return totalKB, count, true
	case "windows":
		// wmic is gone on current Windows and tasklist reports no tree RSS, so the
		// process counts as alive without a memory figure.
		return 0, 1, false
	default:
		return 0, 0, false
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
