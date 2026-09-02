package main

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// An "activity" is the unit of child-facing content in the new workspace layout
// (see the workspace-redesign plan): a self-contained folder nested by
// subject/topic — `<Subject>/<Topic>/<slug>/` — holding its own content files,
// an `activity.json` manifest, its conversation, and the child's attempts. It
// replaces the old shared/study + shared/tests + shared/packages/*.json model.
// The child is sandboxed to exactly ONE activity folder at a time.
const activityManifestName = "activity.json"

// activityManifest is the on-disk `activity.json` inside each activity folder.
// Items are BARE filenames within the folder, in the order the child works
// through them (empty = an instruction-only / dynamically-generated activity).
// The answer key (`*-KEY.md`) lives in the folder but is deliberately NOT in
// Items — it's the parent's, and what the tutor reveals to the child is the
// tutor's judgment in the moment, not something the file's presence decides.
type activityManifest struct {
	Title   string   `json:"title"`
	Subject string   `json:"subject,omitempty"`
	Topic   string   `json:"topic,omitempty"`
	Items   []string `json:"items,omitempty"`
	// Goal is WHAT the activity is for — what the child should get out of it,
	// in the parent's own words. HOW to get there is deliberately not stored:
	// it's the tutor's judgment, live in the conversation.
	//
	// This replaced two earlier fields that both tried to pin the "how" down in
	// advance — guide_note (pacing/order/what-to-do-if-stuck) and teaching_mode
	// (a beginner|graduated|strict enum with an hints_before_answer count).
	// Both were decided before anyone knew how the session would actually go,
	// and then could not bend when it went differently: a child who is
	// genuinely stuck, out of time, or facing a question that turned out to be
	// wrong needs a tutor that adapts, not one honoring a setting made
	// yesterday. The teaching judgment those encoded now lives in
	// childSystemPrompt as default behavior the tutor applies with sense, and
	// Goal steers it where the parent actually wants something specific.
	Goal      string `json:"goal,omitempty"`
	Persona   string `json:"persona,omitempty"`
	CreatedAt string `json:"created_at,omitempty"`
}

// Activity is a loaded activity: its workspace-relative folder Dir plus the
// manifest fields (flattened for JSON so API responses are one object).
type Activity struct {
	Dir string `json:"dir"`
	activityManifest
}

// reservedTopLevel are the top-level workspace entries that are NOT subjects —
// infrastructure/areas that must never be treated as a Subject folder when
// discovering activities or listing subjects in the Files tab.
var reservedTopLevel = map[string]bool{
	"materials":               true,
	"inbox":                   true,
	"reports":                 true,
	"memory":                  true,
	"conversations":           true,
	"skills":                  true,
	".agents":                 true,
	"_legacy":                 true,
	"backup":                  true,
	"publish":                 true,
	"workspace.pre-v2-backup": true,
	archiveDir:                true, // see archive.go — retired activities, excluded from routine scans
	// Platform folders that exist when this workspace is (or is shared with)
	// an AgentWorks workspace root. None is a Subject, and archiveStaleActivities
	// os.Rename()s any non-reserved top-level folder idle for a week — a
	// misconfigured FAMILY_DATA_DIR would otherwise silently relocate the
	// main server's Workflow/ or pulse/ trees into archive/.
	"_users":                true,
	"Workflow":              true,
	"pulse":                 true,
	"memories":              true,
	"config":                true,
	"chat_history":          true,
	"Chats":                 true,
	"Downloads":             true,
	"current-activity.json": true,
}

// isSubjectDir reports whether a top-level entry name is a Subject folder
// (i.e. an ordinary directory that isn't reserved infrastructure or hidden).
func isSubjectDir(name string) bool {
	if name == "" || strings.HasPrefix(name, ".") {
		return false
	}
	return !reservedTopLevel[name]
}

// loadActivity reads `<dir>/activity.json` and returns the loaded Activity.
// dir is workspace-relative (e.g. "Maths/Fractions/2026-07-24-quick-check").
func loadActivity(dir string) (Activity, bool) {
	dir = strings.Trim(strings.TrimSpace(dir), "/")
	if dir == "" {
		return Activity{}, false
	}
	abs, ok := resolveWorkspacePath(filepath.Join(dir, activityManifestName))
	if !ok {
		return Activity{}, false
	}
	b, err := os.ReadFile(abs)
	if err != nil {
		return Activity{}, false
	}
	var m activityManifest
	if err := json.Unmarshal(b, &m); err != nil {
		return Activity{}, false
	}
	return Activity{Dir: filepath.ToSlash(dir), activityManifest: m}, true
}

// listActivities walks the workspace (skipping reserved top-level areas) and
// returns every activity — any directory containing an `activity.json`. It
// does not recurse into a found activity. Newest first. Replaces listPackages.
func listActivities() []Activity {
	root := workspaceRoot()
	out := []Activity{}
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || !d.IsDir() {
			return nil
		}
		rel, relErr := filepath.Rel(root, p)
		if relErr != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if rel == "." {
			return nil // the root itself
		}
		// Skip reserved top-level areas entirely (materials/, memory/, etc.).
		top := strings.SplitN(rel, "/", 2)[0]
		if !isSubjectDir(top) {
			return filepath.SkipDir
		}
		if _, statErr := os.Stat(filepath.Join(p, activityManifestName)); statErr == nil {
			if act, ok := loadActivity(rel); ok {
				out = append(out, act)
			}
			return filepath.SkipDir // activities don't nest
		}
		return nil
	})
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt > out[j].CreatedAt })
	return out
}

// findActivityForPath returns the activity dir that a workspace-relative path
// belongs to — either the path IS an activity dir, or it's a file inside one.
// Returns "" if the path isn't within any activity.
func findActivityForPath(path string) string {
	path = strings.Trim(strings.TrimSpace(path), "/")
	if path == "" {
		return ""
	}
	for _, act := range listActivities() {
		if path == act.Dir || strings.HasPrefix(path, act.Dir+"/") {
			return act.Dir
		}
	}
	return ""
}

// --- current-activity pointer (replaces child/current-task.json) ------------

// currentActivityPointer is the tiny root-level `current-activity.json` naming
// which activity the child session is currently bound to. Everything else the
// child needs (items, goal, persona) it reads from that
// activity's own activity.json, which it has full access to.
type currentActivityPointer struct {
	Dir string `json:"dir"`
}

func currentActivityPointerPath() (string, bool) {
	return resolveWorkspacePath("current-activity.json")
}

func currentActivityDir() string {
	abs, ok := currentActivityPointerPath()
	if !ok {
		return ""
	}
	b, err := os.ReadFile(abs)
	if err != nil {
		return ""
	}
	var ptr currentActivityPointer
	if err := json.Unmarshal(b, &ptr); err != nil {
		return ""
	}
	return strings.Trim(strings.TrimSpace(ptr.Dir), "/")
}

// loadCurrentActivity returns the full Activity the child is currently bound
// to, or ok=false when nothing is handed off (or the pointer is stale).
func loadCurrentActivity() (Activity, bool) {
	dir := currentActivityDir()
	if dir == "" {
		return Activity{}, false
	}
	return loadActivity(dir)
}

func saveCurrentActivity(dir string) {
	abs, ok := currentActivityPointerPath()
	if !ok {
		return
	}
	_ = os.MkdirAll(filepath.Dir(abs), 0o700)
	b, _ := json.Marshal(currentActivityPointer{Dir: filepath.ToSlash(strings.Trim(strings.TrimSpace(dir), "/"))})
	_ = os.WriteFile(abs, b, 0o600)
}
