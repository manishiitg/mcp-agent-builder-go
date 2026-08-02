package loopclosure

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func ts(t *testing.T, s string) time.Time {
	t.Helper()
	v, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatalf("bad fixture time %q: %v", s, err)
	}
	return v.UTC()
}

// TestStaleAnswerUsesRealSocialMediaShape reproduces the exact situation
// measured on 2026-07-29: social-media's operator answers landed at 11:29-11:30
// and a full Pulse Gate pass completed at 14:57 without consuming them. That
// pass had the answer available and did not act on it.
func TestStaleAnswerUsesRealSocialMediaShape(t *testing.T) {
	now := ts(t, "2026-07-29T22:00:00Z")
	lastGate := ts(t, "2026-07-29T14:57:51Z")

	inputs := []HumanInput{{
		ID:        "pulse-rolling-window-cap",
		Source:    "pulse",
		Question:  "Your rules require a limit on total actions within a rolling time window, but no such limit exists.",
		Status:    "answered",
		CreatedAt: ts(t, "2026-07-28T18:17:26Z"),
		UpdatedAt: ts(t, "2026-07-29T11:30:17Z"),
	}}

	got := Evaluate(now, lastGate, inputs, nil, DefaultConfig())
	if len(got) != 1 {
		t.Fatalf("expected 1 stale-answer finding, got %d: %+v", len(got), got)
	}
	if got[0].Kind != KindStaleAnswer {
		t.Fatalf("Kind = %q, want %q", got[0].Kind, KindStaleAnswer)
	}
	if got[0].Severity != SeverityHigh {
		t.Fatalf("an unapplied operator answer must be high severity, got %q", got[0].Severity)
	}
	if got[0].ID != "pulse-rolling-window-cap" {
		t.Fatalf("ID not carried through: %q", got[0].ID)
	}
}

// An answer given AFTER the last Gate pass is not stale — no pass has had the
// chance to consume it yet. Reporting it would manufacture a false failure.
func TestAnswerNewerThanLastGatePassIsNotStale(t *testing.T) {
	now := ts(t, "2026-07-29T22:00:00Z")
	lastGate := ts(t, "2026-07-29T14:57:51Z")

	inputs := []HumanInput{{
		ID:        "answered-after-the-pass",
		Status:    "answered",
		Question:  "Answered after the most recent pass ran.",
		CreatedAt: ts(t, "2026-07-29T15:00:00Z"),
		UpdatedAt: ts(t, "2026-07-29T16:00:00Z"),
	}}

	if got := Evaluate(now, lastGate, inputs, nil, DefaultConfig()); len(got) != 0 {
		t.Fatalf("expected no finding, got %+v", got)
	}
}

// TestAnswerDuringAnInFlightPassIsNotStale locks in a false positive caught by
// running against production: upwork answers landed at 16:52:06 and 16:52:12
// against a Gate pass completing 16:52:34 — 22 and 28 seconds later. The pass
// was already underway and never had a fair chance to consume them. Without a
// grace window this fires on every pass the operator answers during, which
// would make the whole layer noise.
func TestAnswerDuringAnInFlightPassIsNotStale(t *testing.T) {
	now := ts(t, "2026-07-29T22:00:00Z")
	lastGate := ts(t, "2026-07-29T16:52:34Z")

	inputs := []HumanInput{
		{ID: "answered-22s-before-pass-completed", Status: "answered",
			Question:  "One more job is stuck mid-bid. Was a proposal actually sent for it?",
			CreatedAt: ts(t, "2026-07-29T16:00:00Z"), UpdatedAt: ts(t, "2026-07-29T16:52:06Z")},
		{ID: "answered-28s-before-pass-completed", Status: "answered",
			Question:  "Most of your steps are in an old format. May I convert them?",
			CreatedAt: ts(t, "2026-07-29T16:00:00Z"), UpdatedAt: ts(t, "2026-07-29T16:52:12Z")},
	}

	if got := Evaluate(now, lastGate, inputs, nil, DefaultConfig()); len(got) != 0 {
		t.Fatalf("an answer given during an in-flight pass must not count as ignored, got %+v", got)
	}

	// But one answered comfortably before the same pass is genuinely stale.
	inputs = append(inputs, HumanInput{
		ID: "answered-hours-earlier", Status: "answered",
		Question:  "Approve a four-producing-run Upwork search experiment?",
		CreatedAt: ts(t, "2026-07-20T17:00:00Z"), UpdatedAt: ts(t, "2026-07-20T17:44:09Z"),
	})
	got := Evaluate(now, lastGate, inputs, nil, DefaultConfig())
	if len(got) != 1 || got[0].ID != "answered-hours-earlier" {
		t.Fatalf("expected only the genuinely stale answer, got %+v", got)
	}
}

// A workflow where Pulse has never run cannot have a stale answer: there has
// been no opportunity to consume anything. Guards against a fleet of brand-new
// workflows all reporting false stalls on their first pass.
func TestNoGatePassYetMeansNoStaleAnswers(t *testing.T) {
	now := ts(t, "2026-07-29T22:00:00Z")

	inputs := []HumanInput{{
		ID:        "answered-but-pulse-never-ran",
		Status:    "answered",
		Question:  "Answered in a workflow with no Pulse history.",
		CreatedAt: ts(t, "2026-07-01T10:00:00Z"),
		UpdatedAt: ts(t, "2026-07-01T10:00:00Z"),
	}}

	if got := Evaluate(now, time.Time{}, inputs, nil, DefaultConfig()); len(got) != 0 {
		t.Fatalf("expected no finding when Pulse has never run, got %+v", got)
	}
}

// The linkedin case: a question the operator has not answered for 8 days,
// which was blocking all publishing.
func TestAgedPendingDecisionIsReported(t *testing.T) {
	now := ts(t, "2026-07-29T22:00:00Z")
	lastGate := ts(t, "2026-07-29T14:57:51Z")

	inputs := []HumanInput{
		{
			ID:        "plan-proposal-enable-live-publish",
			Source:    "goal_advisor",
			Question:  "Turn on live publishing now, so an approved post can actually go out?",
			Status:    "pending",
			CreatedAt: ts(t, "2026-07-21T10:27:00Z"),
			UpdatedAt: ts(t, "2026-07-21T10:27:00Z"),
		},
		{
			// Asked today — working as designed, must not be reported.
			ID:        "fresh-question",
			Status:    "pending",
			Question:  "Asked a few hours ago.",
			CreatedAt: ts(t, "2026-07-29T15:33:56Z"),
			UpdatedAt: ts(t, "2026-07-29T15:33:56Z"),
		},
	}

	got := Evaluate(now, lastGate, inputs, nil, DefaultConfig())
	if len(got) != 1 {
		t.Fatalf("expected only the aged question, got %d: %+v", len(got), got)
	}
	if got[0].Kind != KindPendingAged || got[0].ID != "plan-proposal-enable-live-publish" {
		t.Fatalf("wrong finding: %+v", got[0])
	}
	if got[0].AgeDays != 8 {
		t.Fatalf("AgeDays = %d, want 8", got[0].AgeDays)
	}
}

// Recurrence is the signal a concern was never really handled. social-media's
// worst open concern had been reported on 3 separate runs.
func TestRecurringOpenConcernIsReportedAtThreshold(t *testing.T) {
	now := ts(t, "2026-07-29T22:00:00Z")
	lastGate := ts(t, "2026-07-29T14:57:51Z")

	concerns := []Concern{
		{Fingerprint: "a1", Text: "db_health rows have NULL pnl_inr", Status: "open", SeenCount: 3,
			LastSeenAt: ts(t, "2026-07-29T10:54:16Z")},
		{Fingerprint: "b2", Text: "seen once, not yet a pattern", Status: "open", SeenCount: 1,
			LastSeenAt: ts(t, "2026-07-29T09:49:15Z")},
		{Fingerprint: "c3", Text: "recurred but already resolved", Status: "resolved", SeenCount: 9,
			LastSeenAt: ts(t, "2026-07-29T09:49:15Z")},
		{Fingerprint: "d4", Text: "triaged and intentionally deferred", Status: "acknowledged", SeenCount: 12,
			LastSeenAt: ts(t, "2026-07-29T09:49:15Z")},
		{Fingerprint: "e5", Text: "owned by the platform", Status: "external_action_required", SeenCount: 20,
			LastSeenAt: ts(t, "2026-07-29T09:49:15Z")},
	}

	got := Evaluate(now, lastGate, nil, concerns, DefaultConfig())
	if len(got) != 1 {
		t.Fatalf("expected only the recurring open concern, got %d: %+v", len(got), got)
	}
	if got[0].Kind != KindRecurringConcern || got[0].ID != "a1" {
		t.Fatalf("wrong finding: %+v", got[0])
	}
}

// Gate reads these top-down, so ordering is part of the contract.
func TestFindingsAreOrderedBySeverityThenAge(t *testing.T) {
	now := ts(t, "2026-07-29T22:00:00Z")
	lastGate := ts(t, "2026-07-29T14:57:51Z")

	inputs := []HumanInput{
		{ID: "pending-old", Status: "pending", Question: "medium severity, very old",
			CreatedAt: ts(t, "2026-07-01T00:00:00Z"), UpdatedAt: ts(t, "2026-07-01T00:00:00Z")},
		{ID: "answer-recent", Status: "answered", Question: "high severity, recent",
			CreatedAt: ts(t, "2026-07-29T09:00:00Z"), UpdatedAt: ts(t, "2026-07-29T10:00:00Z")},
	}

	got := Evaluate(now, lastGate, inputs, nil, DefaultConfig())
	if len(got) != 2 {
		t.Fatalf("expected 2 findings, got %d", len(got))
	}
	// High severity outranks age: an unapplied answer is actionable now.
	if got[0].ID != "answer-recent" {
		t.Fatalf("expected the high-severity finding first, got %q", got[0].ID)
	}
}

// A fully closed loop must produce silence. If this layer reports on healthy
// workflows it becomes noise and gets ignored.
func TestHealthyStateProducesNoFindings(t *testing.T) {
	now := ts(t, "2026-07-29T22:00:00Z")
	lastGate := ts(t, "2026-07-29T14:57:51Z")

	inputs := []HumanInput{
		{ID: "done", Status: "consumed", Question: "already applied",
			CreatedAt: ts(t, "2026-07-01T00:00:00Z"), UpdatedAt: ts(t, "2026-07-02T00:00:00Z")},
		{ID: "dropped", Status: "dismissed", Question: "operator dismissed it",
			CreatedAt: ts(t, "2026-07-01T00:00:00Z"), UpdatedAt: ts(t, "2026-07-02T00:00:00Z")},
	}
	concerns := []Concern{
		{Fingerprint: "ok", Text: "handled", Status: "resolved", SeenCount: 5,
			LastSeenAt: ts(t, "2026-07-20T00:00:00Z")},
	}

	if got := Evaluate(now, lastGate, inputs, concerns, DefaultConfig()); len(got) != 0 {
		t.Fatalf("healthy state must be silent, got %+v", got)
	}
}

func TestSummarizeStripsWhitespaceAndTruncates(t *testing.T) {
	long := ""
	for i := 0; i < 40; i++ {
		long += "word "
	}
	got := summarize(long)
	if len(got) > 120 {
		t.Fatalf("summary too long: %d chars", len(got))
	}
	if summarize("  a\n\tb  ") != "a b" {
		t.Fatalf("whitespace not normalized: %q", summarize("  a\n\tb  "))
	}
}

func TestCheckDistinguishesMissingDatabaseFromClean(t *testing.T) {
	t.Setenv("WORKSPACE_DOCS_PATH", t.TempDir())

	result := Check(context.Background(), "Workflow/new", ts(t, "2026-07-29T22:00:00Z"))
	if result.CoverageStatus != CoverageNotInstrumented {
		t.Fatalf("coverage = %q, want %q: %+v", result.CoverageStatus, CoverageNotInstrumented, result)
	}
	if len(result.Findings) != 0 {
		t.Fatalf("missing database produced findings: %+v", result.Findings)
	}
	if result.DetectorVersion != DetectorVersion {
		t.Fatalf("detector version = %q, want %q", result.DetectorVersion, DetectorVersion)
	}
}

func TestCheckReportsVerifiedCoverageAndFindings(t *testing.T) {
	root := t.TempDir()
	t.Setenv("WORKSPACE_DOCS_PATH", root)
	db := openLoopClosureFixtureDB(t, root, "Workflow/example")
	defer db.Close()

	for _, stmt := range []string{
		`CREATE TABLE pulse_module_state (
			workspace_path TEXT NOT NULL, last_checked_at TEXT NOT NULL
		)`,
		`CREATE TABLE report_human_inputs (
			id TEXT NOT NULL, source TEXT NOT NULL, question TEXT NOT NULL,
			status TEXT NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL
		)`,
		`CREATE TABLE run_concerns (
			fingerprint TEXT NOT NULL, text TEXT NOT NULL, status TEXT NOT NULL,
			seen_count INTEGER NOT NULL, last_seen_at TEXT NOT NULL
		)`,
		`INSERT INTO pulse_module_state(workspace_path, last_checked_at)
		 VALUES ('Workflow/example', '2026-07-29T14:57:51Z')`,
		`INSERT INTO report_human_inputs
			(id, source, question, status, created_at, updated_at)
		 VALUES
			('answered-1', 'pulse', 'Apply the approved destination.', 'answered',
			 '2026-07-29T10:00:00Z', '2026-07-29T11:00:00Z')`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("fixture statement %q: %v", stmt, err)
		}
	}

	result := Check(context.Background(), "Workflow/example", ts(t, "2026-07-29T22:00:00Z"))
	if result.CoverageStatus != CoverageVerified {
		t.Fatalf("coverage = %q, want %q: %s", result.CoverageStatus, CoverageVerified, result.CoverageReason)
	}
	if len(result.Findings) != 1 || result.Findings[0].ID != "answered-1" {
		t.Fatalf("findings = %+v, want answered-1", result.Findings)
	}
}

func TestCheckMakesPartialSchemaExplicit(t *testing.T) {
	root := t.TempDir()
	t.Setenv("WORKSPACE_DOCS_PATH", root)
	db := openLoopClosureFixtureDB(t, root, "Workflow/example")
	defer db.Close()

	for _, stmt := range []string{
		`CREATE TABLE pulse_module_state (
			workspace_path TEXT NOT NULL, last_checked_at TEXT NOT NULL
		)`,
		`CREATE TABLE report_human_inputs (
			id TEXT NOT NULL, source TEXT NOT NULL, question TEXT NOT NULL,
			status TEXT NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL
		)`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("fixture statement %q: %v", stmt, err)
		}
	}

	result := Check(context.Background(), "Workflow/example", ts(t, "2026-07-29T22:00:00Z"))
	if result.CoverageStatus != CoveragePartial {
		t.Fatalf("coverage = %q, want %q: %+v", result.CoverageStatus, CoveragePartial, result)
	}
	if !strings.Contains(result.CoverageReason, "run_concerns") {
		t.Fatalf("coverage reason does not identify missing source: %q", result.CoverageReason)
	}
}

func TestCheckRejectsWorkspaceTraversal(t *testing.T) {
	t.Setenv("WORKSPACE_DOCS_PATH", t.TempDir())

	result := Check(context.Background(), "../../outside", ts(t, "2026-07-29T22:00:00Z"))
	if result.CoverageStatus != CoverageUnavailable {
		t.Fatalf("coverage = %q, want %q", result.CoverageStatus, CoverageUnavailable)
	}
	if !strings.Contains(result.CoverageReason, "workspace root") {
		t.Fatalf("unexpected coverage reason: %q", result.CoverageReason)
	}
}

func TestOpenReadOnlyObservesCheckpointedWALWithoutSidecars(t *testing.T) {
	root := t.TempDir()
	t.Setenv("WORKSPACE_DOCS_PATH", root)
	workspacePath := "Workflow/wal-loopclosure"
	fixture := openLoopClosureFixtureDB(t, root, workspacePath)
	if _, err := fixture.Exec(`PRAGMA journal_mode=WAL; CREATE TABLE marker(value TEXT); INSERT INTO marker VALUES ('observed')`); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.Exec(`PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
		t.Fatal(err)
	}
	if err := fixture.Close(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, filepath.FromSlash(workspacePath), "db", "db.sqlite")
	_ = os.Remove(path + "-wal")
	_ = os.Remove(path + "-shm")

	db, exists, err := openReadOnly(context.Background(), workspacePath)
	if err != nil || !exists {
		t.Fatalf("loop closure could not open checkpointed WAL without sidecars: exists=%v err=%v", exists, err)
	}
	defer db.Close()
	var value string
	if err := db.QueryRow(`SELECT value FROM marker`).Scan(&value); err != nil {
		t.Fatal(err)
	}
	if value != "observed" {
		t.Fatalf("value=%q, want observed", value)
	}
}

func openLoopClosureFixtureDB(t *testing.T, root, workspacePath string) *sql.DB {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(workspacePath), "db", "db.sqlite")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create fixture directory: %v", err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open fixture database: %v", err)
	}
	return db
}
