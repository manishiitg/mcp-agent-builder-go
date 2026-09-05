package server

import "testing"

// PLAT-284: a workflow's .sandbox-cache holds installed packages (hundreds of
// MB of numpy/pandas any run can recreate); it must never count as backup
// content, exactly like the rotating runs/ folders.
func TestBackupHashSkipsTheSandboxCache(t *testing.T) {
	for _, relPath := range []string{
		".sandbox-cache/python/lib/python3.12/site-packages/yfinance/__init__.py",
		"Workflow/x/.sandbox-cache/bin/tool",
		"runs/iteration-0/execution/step/out.json",
	} {
		if !shouldSkipBackupHashFile(relPath) {
			t.Errorf("expected %q to be skipped", relPath)
		}
	}
	for _, relPath := range []string{
		"planning/plan.json",
		"db/assets/report.pdf",
	} {
		if shouldSkipBackupHashFile(relPath) {
			t.Errorf("expected %q to be tracked", relPath)
		}
	}
}
