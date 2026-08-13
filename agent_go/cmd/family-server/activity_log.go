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
	// DurationSeconds accumulates every turn's real work time (see
	// turnTrace.duration) across however many turns happened on this
	// activity today. Approximate, not exact — see the caller in child.go
	// for what it does and doesn't capture.
	DurationSeconds int `json:"duration_seconds,omitempty"`
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

// recordActivityLogEntry accumulates duration onto today's entry for
// activityDir, creating one if today has no entry for it yet. Scans the
// TRAILING RUN of entries sharing today's date (not just the last entry) —
// a child bouncing between two activities in one sitting (Math -> English ->
// Math, completely normal) would otherwise have the second Math turn miss
// the last entry (English) and append a second, fragmented Math entry
// instead of accumulating onto the first. Best-effort: a failure here should
// never fail the real child turn it's called from, so errors are swallowed.
func recordActivityLogEntry(activityDir string, duration time.Duration) {
	if activityDir == "" {
		return
	}
	today := time.Now().Format("2006-01-02")
	activityLogMu.Lock()
	defer activityLogMu.Unlock()
	entries := loadActivityLog()
	seconds := int(duration.Seconds())
	for i := len(entries) - 1; i >= 0 && entries[i].Date == today; i-- {
		if entries[i].ActivityDir == activityDir {
			entries[i].DurationSeconds += seconds
			_ = saveActivityLog(entries)
			return
		}
	}
	title := activityDir
	if act, ok := loadActivity(activityDir); ok && act.Title != "" {
		title = act.Title
	}
	entries = append(entries, ActivityLogEntry{Date: today, ActivityDir: activityDir, Title: title, DurationSeconds: seconds})
	_ = saveActivityLog(entries)
}
