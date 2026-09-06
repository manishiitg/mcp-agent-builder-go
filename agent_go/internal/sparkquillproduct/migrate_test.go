package sparkquillproduct

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFixtureFile(t *testing.T, p, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

// legacyFixture builds a small but representative ~/.sunlit-learning: two
// activities sharing a slug under different subjects, an answer key and a
// child conversation inside one, an archived activity with its own key, the
// verbatim folders, the parent thread, CLI session handles, secrets at the
// root, and the standalone app's own scratch.
func legacyFixture(t *testing.T) string {
	t.Helper()
	src := filepath.Join(t.TempDir(), ".sunlit-learning")
	ws := filepath.Join(src, "workspace")
	writeFixtureFile(t, filepath.Join(src, "family.json"), `{
  "engine": "codex-cli",
  "child": {"name": "Maya", "grade": "10", "board": "CBSE", "language": "en", "created_at": "2026-07-01T00:00:00Z"},
  "parent_label": "mom",
  "pin_hash": "legacyhash",
  "pulse": {"enabled": true, "cadence_hours": 24, "watch_sites": ["https://a.example"], "school_portal_url": "https://portal.example", "last_run_at": "2026-09-01T00:00:00Z"},
  "schedule": {"weekdays": []},
  "whatsapp_voice_enabled": true,
  "child_fast_mode": false
}`)
	writeFixtureFile(t, filepath.Join(src, "secrets.key"), "k")
	writeFixtureFile(t, filepath.Join(src, "secrets.enc.json"), "{}")

	math := filepath.Join(ws, "Mathematics", "Fractions and Decimals", "2026-01-01-quiz")
	writeFixtureFile(t, filepath.Join(math, "activity.json"), `{"title":"Fractions Quiz","subject":"Mathematics","topic":"Fractions and Decimals","items":["quiz.html"],"goal":"Do it alone"}`)
	writeFixtureFile(t, filepath.Join(math, "quiz.html"), "<p>quiz</p>")
	writeFixtureFile(t, filepath.Join(math, "quiz-KEY.md"), "answers")
	writeFixtureFile(t, filepath.Join(math, "conversation.json"), `{"messages":[]}`)
	writeFixtureFile(t, filepath.Join(math, "quiz.session.json"), `{"id":"cli"}`)
	writeFixtureFile(t, filepath.Join(math, "attempts", "a.json"), `{"score":1}`)
	writeFixtureFile(t, filepath.Join(ws, "Mathematics", "stray.txt"), "stray")

	sci := filepath.Join(ws, "Science", "Fractions", "2026-01-01-quiz")
	writeFixtureFile(t, filepath.Join(sci, "activity.json"), `{"title":"Science Quiz","subject":"Science","topic":"Fractions","items":["sci.html"]}`)
	writeFixtureFile(t, filepath.Join(sci, "sci.html"), "<p>sci</p>")

	old := filepath.Join(ws, "archive", "Old", "2026-01-02-old")
	writeFixtureFile(t, filepath.Join(old, "activity.json"), `{"title":"Old"}`)
	writeFixtureFile(t, filepath.Join(old, "old-KEY.md"), "old answers")
	writeFixtureFile(t, filepath.Join(old, "old.html"), "<p>old</p>")

	writeFixtureFile(t, filepath.Join(ws, "materials", "m.txt"), "m")
	writeFixtureFile(t, filepath.Join(ws, "memory", "preferences.md"), "prefs")
	writeFixtureFile(t, filepath.Join(ws, "inbox", "i.txt"), "i")
	writeFixtureFile(t, filepath.Join(ws, "reports", "progress.html"), "<p>progress</p>")
	writeFixtureFile(t, filepath.Join(ws, "conversations", "parent.json"), `{"messages":[]}`)
	writeFixtureFile(t, filepath.Join(ws, "conversations", "parent.session.json"), `{"id":"cli"}`)
	writeFixtureFile(t, filepath.Join(ws, "conversations", "legacy-child", "x.json"), `{}`)
	writeFixtureFile(t, filepath.Join(ws, "current-activity.json"), `{"dir":"Mathematics/Fractions and Decimals/2026-01-01-quiz"}`)
	writeFixtureFile(t, filepath.Join(ws, "skills", "s.md"), "skill")
	writeFixtureFile(t, filepath.Join(ws, "whatsapp-routing.json"), `{}`)
	writeFixtureFile(t, filepath.Join(ws, ".migrated-v2"), "2026")
	writeFixtureFile(t, filepath.Join(ws, "_legacy", "old.txt"), "old")
	writeFixtureFile(t, filepath.Join(ws, "notes.txt"), "unplaceable")
	return src
}

func listFiles(t *testing.T, root string) []string {
	t.Helper()
	var out []string
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			rel, _ := filepath.Rel(root, p)
			out = append(out, filepath.ToSlash(rel))
		}
		return nil
	})
	return out
}

func mustExist(t *testing.T, p string) {
	t.Helper()
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("expected %s to exist: %v", p, err)
	}
}

func mustNotExist(t *testing.T, p string) {
	t.Helper()
	if _, err := os.Stat(p); err == nil {
		t.Fatalf("expected %s NOT to exist", p)
	}
}

func TestMigrateLegacyCopiesActivitiesKeysAndPointer(t *testing.T) {
	src := legacyFixture(t)
	before := listFiles(t, src)
	docs := t.TempDir()

	report, err := MigrateLegacy(LegacyMigrationOptions{SourceDir: src, DocsDir: docs, KnownEngines: []string{"claude-code", "codex-cli"}, Log: t.Logf})
	if err != nil {
		t.Fatal(err)
	}
	root := LegacyFamilyRoot(docs, "")
	if report.Activities != 2 || report.Archived != 1 || report.KeysMoved != 2 {
		t.Fatalf("report = %+v", report)
	}

	// Two activities with the same slug: the second gets a numeric suffix.
	mustExist(t, filepath.Join(root, "activities", "2026-01-01-quiz", "quiz.html"))
	mustExist(t, filepath.Join(root, "activities", "2026-01-01-quiz", "attempts", "a.json"))
	mustExist(t, filepath.Join(root, "activities", "2026-01-01-quiz-2", "sci.html"))

	// product.json synthesized for live activities, with a platform session id.
	var project activityProject
	b, err := os.ReadFile(filepath.Join(root, "activities", "2026-01-01-quiz", "product.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, &project); err != nil {
		t.Fatal(err)
	}
	if project.Product != ChildProfileID || project.ID != "2026-01-01-quiz" || project.Title != "Fractions Quiz" || !strings.HasPrefix(project.SessionID, "product-") {
		t.Fatalf("product.json = %+v", project)
	}
	mustNotExist(t, filepath.Join(root, "archive", "2026-01-02-old", "product.json"))

	// Keys never stay inside the child's sandbox.
	mustNotExist(t, filepath.Join(root, "activities", "2026-01-01-quiz", "quiz-KEY.md"))
	mustExist(t, filepath.Join(root, "keys", "2026-01-01-quiz-KEY.md"))
	mustNotExist(t, filepath.Join(root, "archive", "2026-01-02-old", "old-KEY.md"))
	mustExist(t, filepath.Join(root, "keys", "2026-01-02-old-KEY.md"))

	// Child conversation kept as evidence; CLI session handles dropped.
	mustExist(t, filepath.Join(root, "activities", "2026-01-01-quiz", "legacy-conversation.json"))
	for _, f := range listFiles(t, root) {
		if strings.HasSuffix(f, ".session.json") {
			t.Fatalf("session handle leaked into the target: %s", f)
		}
		if strings.Contains(f, "secrets") {
			t.Fatalf("secret file leaked into the target: %s", f)
		}
		if strings.HasPrefix(f, "skills/") || f == "whatsapp-routing.json" || f == ".migrated-v2" {
			t.Fatalf("standalone scratch copied: %s", f)
		}
	}

	// Verbatim folders, the parent thread and unplaceables.
	mustExist(t, filepath.Join(root, "materials", "m.txt"))
	mustExist(t, filepath.Join(root, "memory", "preferences.md"))
	mustExist(t, filepath.Join(root, "inbox", "i.txt"))
	mustExist(t, filepath.Join(root, "reports", "progress.html"))
	mustExist(t, filepath.Join(root, "_legacy", "conversations", "parent.json"))
	mustExist(t, filepath.Join(root, "_legacy", "conversations", "legacy-child", "x.json"))
	mustExist(t, filepath.Join(root, "_legacy", "Mathematics", "stray.txt"))
	mustExist(t, filepath.Join(root, "_legacy", "old.txt"))
	mustExist(t, filepath.Join(root, "_legacy", "notes.txt"))

	// The handoff pointer follows the allocated slug.
	var pointer struct{ Dir string }
	b, _ = os.ReadFile(filepath.Join(root, "current-activity.json"))
	_ = json.Unmarshal(b, &pointer)
	if pointer.Dir != "activities/2026-01-01-quiz" {
		t.Fatalf("current-activity.json dir = %q", pointer.Dir)
	}

	// family.json: the platform fields, watch sites folded, the rest dropped.
	var fam FamilyState
	b, _ = os.ReadFile(filepath.Join(root, FamilyFile))
	if err := json.Unmarshal(b, &fam); err != nil {
		t.Fatal(err)
	}
	if fam.Engine != "codex-cli" || fam.Child == nil || fam.Child.Name != "Maya" || fam.Child.Language != "en" || fam.ParentLabel != "mom" || fam.PinHash != "legacyhash" {
		t.Fatalf("family.json = %+v", fam)
	}
	if strings.Join(fam.WatchSites, ",") != "https://a.example,https://portal.example" {
		t.Fatalf("watch_sites = %v", fam.WatchSites)
	}
	if strings.Contains(string(b), "whatsapp_voice_enabled") || strings.Contains(string(b), "schedule") {
		t.Fatalf("legacy-only fields must not be carried over: %s", b)
	}

	mustExist(t, filepath.Join(root, LegacyMigrationMarker))

	// Copy, never move: the source is byte-for-byte untouched.
	after := listFiles(t, src)
	if strings.Join(before, "\n") != strings.Join(after, "\n") {
		t.Fatalf("source changed:\nbefore=%v\nafter=%v", before, after)
	}
}

func TestMigrateLegacyIsIdempotent(t *testing.T) {
	src := legacyFixture(t)
	docs := t.TempDir()
	if _, err := MigrateLegacy(LegacyMigrationOptions{SourceDir: src, DocsDir: docs}); err != nil {
		t.Fatal(err)
	}
	root := LegacyFamilyRoot(docs, "")
	first := listFiles(t, root)
	report, err := MigrateLegacy(LegacyMigrationOptions{SourceDir: src, DocsDir: docs})
	if err != nil {
		t.Fatal(err)
	}
	if report.Skipped == "" {
		t.Fatalf("second run must be skipped via the marker, got %+v", report)
	}
	if second := listFiles(t, root); strings.Join(first, "\n") != strings.Join(second, "\n") {
		t.Fatalf("second run changed the target")
	}
}

func TestMigrateLegacyRefusesNonEmptyUnmarkedTarget(t *testing.T) {
	src := legacyFixture(t)
	docs := t.TempDir()
	root := LegacyFamilyRoot(docs, "")
	writeFixtureFile(t, filepath.Join(root, FamilyFile), `{"child":{"name":"Someone"}}`)

	_, err := MigrateLegacy(LegacyMigrationOptions{SourceDir: src, DocsDir: docs})
	if !errors.Is(err, ErrTargetNotEmpty) {
		t.Fatalf("expected ErrTargetNotEmpty, got %v", err)
	}
	if files := listFiles(t, root); len(files) != 1 {
		t.Fatalf("a refused migration must write nothing, target now has %v", files)
	}
}

func TestMigrateLegacyAllowExistingMergesWithoutOverwriting(t *testing.T) {
	src := legacyFixture(t)
	docs := t.TempDir()
	root := LegacyFamilyRoot(docs, "")
	writeFixtureFile(t, filepath.Join(root, FamilyFile), `{"engine":"claude-code","child":{"name":"Zoe","grade":"5","board":"IB"}}`)
	writeFixtureFile(t, filepath.Join(root, "reports", "progress.html"), "<p>platform report</p>")

	if _, err := MigrateLegacy(LegacyMigrationOptions{SourceDir: src, DocsDir: docs, AllowExisting: true, KnownEngines: []string{"claude-code", "codex-cli"}}); err != nil {
		t.Fatal(err)
	}
	var fam FamilyState
	b, _ := os.ReadFile(filepath.Join(root, FamilyFile))
	_ = json.Unmarshal(b, &fam)
	if fam.Engine != "claude-code" || fam.Child == nil || fam.Child.Name != "Zoe" {
		t.Fatalf("existing choices must win on merge, got %+v", fam)
	}
	if fam.PinHash != "legacyhash" || fam.ParentLabel != "mom" {
		t.Fatalf("missing fields must be filled from legacy, got %+v", fam)
	}
	b, _ = os.ReadFile(filepath.Join(root, "reports", "progress.html"))
	if string(b) != "<p>platform report</p>" {
		t.Fatalf("an existing file was overwritten: %q", b)
	}
	mustExist(t, filepath.Join(root, "reports", "progress.html.migrated-dup"))
	mustExist(t, filepath.Join(root, "activities", "2026-01-01-quiz", "quiz.html"))
}

func TestMigrateLegacyDryRunWritesNothing(t *testing.T) {
	src := legacyFixture(t)
	docs := t.TempDir()
	report, err := MigrateLegacy(LegacyMigrationOptions{SourceDir: src, DocsDir: docs, DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if report.Activities != 2 || report.FilesCopied == 0 {
		t.Fatalf("dry run must still plan the work, got %+v", report)
	}
	if files := listFiles(t, docs); len(files) != 0 {
		t.Fatalf("dry run wrote %v", files)
	}
}

func TestMigrateLegacyDropsUnknownEngineAndSkipsMissingSource(t *testing.T) {
	src := legacyFixture(t)
	docs := t.TempDir()
	report, err := MigrateLegacy(LegacyMigrationOptions{SourceDir: src, DocsDir: docs, KnownEngines: []string{"claude-code"}})
	if err != nil {
		t.Fatal(err)
	}
	var fam FamilyState
	b, _ := os.ReadFile(filepath.Join(LegacyFamilyRoot(docs, ""), FamilyFile))
	_ = json.Unmarshal(b, &fam)
	if fam.Engine != "" {
		t.Fatalf("an engine the profile does not offer must be dropped, got %q", fam.Engine)
	}
	if len(report.Warnings) == 0 {
		t.Fatal("dropping the engine must be reported")
	}

	report, err = MigrateLegacy(LegacyMigrationOptions{SourceDir: filepath.Join(t.TempDir(), "nope"), DocsDir: t.TempDir()})
	if err != nil || report.Skipped == "" {
		t.Fatalf("a missing legacy home must be a no-op, got report=%+v err=%v", report, err)
	}
}

func TestLegacyPulseEnabled(t *testing.T) {
	src := legacyFixture(t)
	enabled, last := LegacyPulseEnabled(src)
	if !enabled || last != "2026-09-01T00:00:00Z" {
		t.Fatalf("enabled=%v last=%q", enabled, last)
	}
}
