package main

import (
	"log"
	"os"
	"path/filepath"
	"time"
)

// staleActivityAge is how long an activity can go untouched before Pulse
// archives it out of the live tree — keeps the set routine Pulse checks
// (learning-review, academic-map) scan bounded to roughly the last week or
// two of real use, rather than growing forever.
const staleActivityAge = 7 * 24 * time.Hour

// archiveDir is the reserved top-level folder retired activities move into.
// Still real files on disk — nothing here is ever deleted — and still
// visible via the "All files" browser; just excluded from routine Pulse
// scans and the normal Workspace tab, which both go through
// listActivities()/reservedTopLevel.
const archiveDir = "archive"

// archiveStaleActivities moves any activity untouched for staleActivityAge
// out of the live tree into archive/<Subject>/<Topic>/<slug>/, mirroring its
// original path exactly. This exists because every Pulse cycle's
// learning-review and academic-map checks re-scan EVERY activity ever
// created, and that set only grows over months/years of real use — a real,
// unbounded cost. Archiving is always a MOVE, never a delete: the parent can
// still find an old one via "All files", and create-progress-report's
// cumulative Overall section explicitly still reads archive/ too, so
// lifetime totals never silently shrink just because something aged out of
// the routine scans.
//
// Purely mechanical (age, plus "is this what the child is on right now") —
// no judgment call for a model to make, so this runs as plain Go
// housekeeping rather than an agent turn.
func archiveStaleActivities() {
	root := workspaceRoot()
	current := currentActivityDir()
	cutoff := time.Now().Add(-staleActivityAge)

	for _, act := range listActivities() {
		if act.Dir == current {
			continue // never archive what the child is actively on
		}
		lastTouched, ok := activityLastTouched(filepath.Join(root, act.Dir))
		if !ok || lastTouched.After(cutoff) {
			continue
		}
		dest := filepath.Join(root, archiveDir, filepath.FromSlash(act.Dir))
		if err := os.MkdirAll(filepath.Dir(dest), 0o700); err != nil {
			log.Printf("[archive] mkdir failed for %s: %v", act.Dir, err)
			continue
		}
		if err := os.Rename(filepath.Join(root, act.Dir), dest); err != nil {
			log.Printf("[archive] move failed for %s: %v", act.Dir, err)
			continue
		}
		log.Printf("[archive] moved stale activity %q (last touched %s) to %s/%s",
			act.Dir, lastTouched.Format("2006-01-02"), archiveDir, act.Dir)
	}
}

// activityLastTouched is the most recent modification time across an
// activity's manifest, conversation, and any saved attempts — a proxy for
// "when was this genuinely last engaged with," not just when it was created.
func activityLastTouched(absDir string) (time.Time, bool) {
	var latest time.Time
	found := false
	consider := func(path string) {
		info, err := os.Stat(path)
		if err != nil {
			return
		}
		found = true
		if info.ModTime().After(latest) {
			latest = info.ModTime()
		}
	}
	consider(filepath.Join(absDir, activityManifestName))
	consider(filepath.Join(absDir, "conversation.json"))
	if entries, err := os.ReadDir(filepath.Join(absDir, "attempts")); err == nil {
		for _, e := range entries {
			consider(filepath.Join(absDir, "attempts", e.Name()))
		}
	}
	return latest, found
}
