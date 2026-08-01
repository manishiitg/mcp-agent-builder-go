package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// activity_log.go — a genuine append-only history of which activity the
// child actually worked on, on which day. Unlike memory/preferences.md or
// memory/interests.md (rewritten in place each time, "current state" only),
// this one needs real history across weeks — powers the "This Week" view's
// past-weeks navigation (see week.go). Written from Go code on every real
// child turn, not by the agent.

// ActivityLogEntry is one day's worth of work on one activity.
type ActivityLogEntry struct {
	Date        string `json:"date"` // "2026-07-28", LOCAL time.Now()
	ActivityDir string `json:"activity_dir"`
	Title       string `json:"title"`
}

var activityLogMu sync.Mutex

func activityLogPath() (string, bool) { return resolveWorkspacePath("memory/activity-log.json") }

func loadActivityLog() []ActivityLogEntry {
	abs, ok := activityLogPath()
	if !ok {
		return nil
	}
	b, err := os.ReadFile(abs)
	if err != nil {
		return nil
	}
	var entries []ActivityLogEntry
	if err := json.Unmarshal(b, &entries); err != nil {
		return nil
	}
	return entries
}

func saveActivityLog(entries []ActivityLogEntry) error {
	abs, ok := activityLogPath()
	if !ok {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(abs, b, 0o600)
}

// recordActivityLogEntry appends one entry for (today, activityDir) — unless
// the LAST entry already matches both, so a burst of turns in one sitting
// doesn't spam duplicates. Best-effort: a failure here should never fail the
// real child turn it's called from, so errors are swallowed.
func recordActivityLogEntry(activityDir string) {
	if activityDir == "" {
		return
	}
	today := time.Now().Format("2006-01-02")
	activityLogMu.Lock()
	defer activityLogMu.Unlock()
	entries := loadActivityLog()
	if n := len(entries); n > 0 && entries[n-1].Date == today && entries[n-1].ActivityDir == activityDir {
		return // already logged today for this activity
	}
	title := activityDir
	if act, ok := loadActivity(activityDir); ok && act.Title != "" {
		title = act.Title
	}
	entries = append(entries, ActivityLogEntry{Date: today, ActivityDir: activityDir, Title: title})
	_ = saveActivityLog(entries)
}
