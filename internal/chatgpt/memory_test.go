package chatgpt

import "testing"

// TestUsageFromTree covers the stopped-browser cases. A process that has exited
// but not been reaped still has a ps row with zero RSS, and treating that as
// "running" surfaced a stale pid plus "1 procs" on a provider whose browser was
// already dead.
func TestUsageFromTree(t *testing.T) {
	tests := []struct {
		name     string
		rootPID  int
		totalKB  int64
		count    int
		measured bool
		wantRun  bool
		wantProg int
		wantRSS  float64
	}{
		{
			name: "healthy tree",
			// 7 processes totalling ~1016 MB, matching a warm Chromium.
			rootPID: 4242, totalKB: 1040384, count: 7, measured: true,
			wantRun: true, wantProg: 7, wantRSS: 1016.0,
		},
		{
			name:    "no rows for the pid at all",
			rootPID: 4242, totalKB: 0, count: 0, measured: true,
			wantRun: false,
		},
		{
			name:    "zombie: a row exists but RSS is zero",
			rootPID: 4242, totalKB: 0, count: 1, measured: true,
			wantRun: false,
		},
		{
			name:    "unmeasurable platform keeps reporting alive",
			rootPID: 4242, totalKB: 0, count: 1, measured: false,
			wantRun: true, wantProg: 1, wantRSS: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := usageFromTree(tc.rootPID, tc.totalKB, tc.count, tc.measured)
			if got.Running != tc.wantRun {
				t.Fatalf("Running = %v, want %v", got.Running, tc.wantRun)
			}
			if !tc.wantRun {
				// A stopped browser must not leak a stale pid to the UI.
				if got.PID != 0 || got.ProcessNum != 0 || got.TotalRSSMB != 0 {
					t.Errorf("stopped browser reported pid=%d procs=%d rss=%.1f, want all zero",
						got.PID, got.ProcessNum, got.TotalRSSMB)
				}
				return
			}
			if got.PID != tc.rootPID {
				t.Errorf("PID = %d, want %d", got.PID, tc.rootPID)
			}
			if got.ProcessNum != tc.wantProg {
				t.Errorf("ProcessNum = %d, want %d", got.ProcessNum, tc.wantProg)
			}
			if got.TotalRSSMB != tc.wantRSS {
				t.Errorf("TotalRSSMB = %.1f, want %.1f", got.TotalRSSMB, tc.wantRSS)
			}
		})
	}
}

// TestProcessTreeUsageRejectsBadPID guards the entry point against a zero pid,
// which reachable code can produce when no browser has been launched.
func TestProcessTreeUsageRejectsBadPID(t *testing.T) {
	for _, pid := range []int{0, -1} {
		if got := ProcessTreeUsage(pid); got.Running {
			t.Errorf("ProcessTreeUsage(%d) reported running", pid)
		}
	}
}

// TestParseUnixTreeRSS walks a synthetic ps table, including a grandchild, so the
// traversal cannot silently degrade to only direct children.
func TestParseUnixTreeRSS(t *testing.T) {
	ps := []byte("  PID  PPID    RSS\n" +
		"    1     0  15264\n" +
		" 4242     1 161296\n" +
		" 4243  4242  82304\n" +
		" 4244  4243  47952\n" +
		" 9999     1  13664\n")

	totalKB, count := parseUnixTreeRSS(ps, 4242)
	if count != 3 {
		t.Fatalf("count = %d, want 3 (root plus a child and a grandchild)", count)
	}
	want := int64(161296 + 82304 + 47952)
	if totalKB != want {
		t.Errorf("totalKB = %d, want %d", totalKB, want)
	}
}
