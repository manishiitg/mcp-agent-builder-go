package step_based_workflow

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/fsutil"
	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/pulsemodules"

	_ "modernc.org/sqlite"
)

// Durable record of the `CONCERNS:` lines steps emit.
//
// Why this exists: a concern used to live only as a substring inside the step's
// completion summary. Pulse Gate grepped that text for the literal marker, and
// if the Gate's cooldown said the matching module wasn't due, the concern was
// gone. Nothing could answer "has this been reported before?" — pulse_module_state
// keeps only `last_*` fields, so run N overwrites run N-1, and pulse_module_audit
// (the table that was meant to hold history) is empty in most workflows because
// it depends on an agent remembering to call a tool.
//
// That last point is the design constraint here: these rows are written by Go at
// the moment the completion summary is composed, never by an agent calling a
// tool. There is no call for an agent to skip, so the table cannot quietly end up
// empty the way pulse_module_audit did.

const runConcernsSchema = `CREATE TABLE IF NOT EXISTS run_concerns (
	fingerprint TEXT PRIMARY KEY,
	step_id TEXT NOT NULL DEFAULT '',
	phase TEXT NOT NULL DEFAULT '',
	group_name TEXT NOT NULL DEFAULT '',
	text TEXT NOT NULL DEFAULT '',
	first_seen_run TEXT NOT NULL DEFAULT '',
	first_seen_at TEXT NOT NULL DEFAULT '',
	last_seen_run TEXT NOT NULL DEFAULT '',
	last_seen_at TEXT NOT NULL DEFAULT '',
	seen_count INTEGER NOT NULL DEFAULT 0,
	status TEXT NOT NULL DEFAULT 'open',
	resolved_at TEXT NOT NULL DEFAULT '',
	resolved_by TEXT NOT NULL DEFAULT '',
	resolution_note TEXT NOT NULL DEFAULT '',
	first_seen_platform_version TEXT NOT NULL DEFAULT ''
)`

// Phases a concern can be raised from. The step body and its two closing turns
// are separate sources: knowing a contradiction came from the learnings turn
// rather than the task itself is what tells a reviewer where to look.
const (
	ConcernPhaseExecution       = "execution"
	ConcernPhaseLearnings       = "learnings"
	ConcernPhaseKBReview        = "kb-review"
	ConcernPhaseMessageSequence = "message-sequence"
	// ConcernPhaseReview covers Pulse reviewer artifacts. The "step" for these is
	// the module name (bug_review, db_health, ...) rather than a plan step.
	ConcernPhaseReview = "review"
	// ConcernPhasePreValidation is filed by Go itself (SavePreValidationLog),
	// not by an agent — there is no step turn to emit a CONCERNS: line, since a
	// failing gate produces a repair turn, not a completion summary. Written at
	// the moment a validation_schema check fails so a step whose schema demands
	// an undocumented field, or rejects a legitimate null, surfaces as evidence
	// even on a run where it eventually passed after repair and left no other
	// trace. See ParseConcernLines and RecordRunConcerns.
	ConcernPhasePreValidation = "prevalidation"
)

const (
	ConcernStatusOpen                   = "open"
	ConcernStatusAcknowledged           = "acknowledged"
	ConcernStatusExternalActionRequired = "external_action_required"
	ConcernStatusResolved               = "resolved"
	ConcernStatusRejected               = "rejected"
)

const concernLinePrefix = "CONCERNS:"

// RunConcern is one deduplicated concern with its recurrence history.
type RunConcern struct {
	Fingerprint    string `json:"fingerprint"`
	StepID         string `json:"step_id"`
	Phase          string `json:"phase"`
	GroupName      string `json:"group_name,omitempty"`
	Text           string `json:"text"`
	FirstSeenRun   string `json:"first_seen_run,omitempty"`
	FirstSeenAt    string `json:"first_seen_at,omitempty"`
	LastSeenRun    string `json:"last_seen_run,omitempty"`
	LastSeenAt     string `json:"last_seen_at,omitempty"`
	SeenCount      int    `json:"seen_count"`
	Status         string `json:"status"`
	ResolvedAt     string `json:"resolved_at,omitempty"`
	ResolvedBy     string `json:"resolved_by,omitempty"`
	ResolutionNote string `json:"resolution_note,omitempty"`
}

// ParseConcernLines pulls the `CONCERNS:` payloads out of a summary blob.
//
// The marker is matched case-insensitively at the start of a trimmed line, which
// is the shape the prompts ask for and the shape the message-sequence aggregator
// already assumed. Everything after the prefix on that line is the concern.
func ParseConcernLines(summary string) []string {
	var out []string
	for _, line := range strings.Split(summary, "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(strings.ToUpper(trimmed), concernLinePrefix) {
			continue
		}
		body := strings.TrimSpace(trimmed[len(concernLinePrefix):])
		// A bare "CONCERNS:" with no payload carries no information.
		if body == "" {
			continue
		}
		out = append(out, body)
	}
	return out
}

// concernFingerprint identifies "the same concern raised again".
//
// Normalization is deliberately conservative: lowercase and collapse whitespace,
// but do NOT strip digits. Stripping them would merge "12 items stale" with
// "3 items stale" — the same underlying issue at a different magnitude, where the
// magnitude is the interesting part. Under-merging shows up as visible
// near-duplicates and is recoverable; over-merging silently hides a changing
// problem behind one row.
func concernFingerprint(stepID, text string) string {
	normalized := strings.ToLower(strings.Join(strings.Fields(text), " "))
	sum := sha256.Sum256([]byte(strings.TrimSpace(stepID) + "\x00" + normalized))
	return hex.EncodeToString(sum[:])[:16]
}

// preValidationConcernFingerprint identifies the step's output-contract gate,
// not an individual JSON path. The detailed failed checks remain in the concern
// text and the per-run pre_validation.json evidence, while one lifecycle row
// represents the repair the Fixer must make.
func preValidationConcernFingerprint(stepID string) string {
	return concernFingerprint(stepID, "prevalidation:step-output-contract")
}

// existingCanonicalReviewFingerprint preserves the identity of an exact finding
// in the same current review lane.
func existingCanonicalReviewFingerprint(ctx context.Context, db pulseFindingLifecycleDB, module, text string) string {
	wanted := strings.ToLower(strings.Join(strings.Fields(text), " "))
	args := []interface{}{ConcernPhaseReview, module}
	rows, err := db.QueryContext(ctx, `SELECT fingerprint, text FROM run_concerns
		WHERE phase=? AND step_id=?`, args...)
	if err != nil {
		return ""
	}
	defer rows.Close()
	for rows.Next() {
		var fingerprint, existingText string
		if rows.Scan(&fingerprint, &existingText) == nil &&
			strings.ToLower(strings.Join(strings.Fields(existingText), " ")) == wanted {
			return fingerprint
		}
	}
	return ""
}

func runConcernsDBPath(workspacePath string) string {
	return filepath.Join(fsutil.WorkspaceDocsRoot(), filepath.FromSlash(strings.Trim(strings.TrimSpace(workspacePath), "/")), "db", "db.sqlite")
}

func openRunConcernsDB(ctx context.Context, workspacePath string, create bool) (*sql.DB, error) {
	dbPath := runConcernsDBPath(workspacePath)
	if create {
		if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
			return nil, err
		}
	} else if _, err := os.Stat(dbPath); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}
	if _, err := db.ExecContext(ctx, "PRAGMA busy_timeout = 5000"); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

// RecordRunConcerns extracts every concern from summary and upserts it.
//
// Best-effort by contract: a step that did its work must not fail because its
// concern could not be filed. Callers log and continue.
func RecordRunConcerns(ctx context.Context, workspacePath, runFolder, groupName, stepID, phase, summary string) (int, error) {
	if phase == ConcernPhaseReview && pulsemodules.IsValid(stepID) {
		stepID = pulsemodules.Normalize(stepID)
	}
	lines := ParseConcernLines(summary)
	if len(lines) == 0 {
		return 0, nil
	}
	if phase == ConcernPhaseReview {
		if err := validatePulseAdvisorFindingRoutes(stepID, summary); err != nil {
			return 0, err
		}
	}
	db, err := openRunConcernsDB(ctx, workspacePath, true)
	if err != nil || db == nil {
		return 0, err
	}
	defer db.Close()
	if err := ensurePulseFindingLifecycleSchema(ctx, db); err != nil {
		return 0, err
	}
	observedAt := time.Now().UTC().Format(time.RFC3339Nano)
	fingerprints := pulseFindingFingerprintsByConcern(summary, stepID)
	recorded, err := recordRunConcernLinesAtWithFingerprints(
		ctx, db, runFolder, groupName, stepID, phase, lines,
		observedAt, fingerprints,
	)
	if err != nil {
		return recorded, err
	}
	if err := recordPulseFindingDetailsAt(
		ctx, db, workspacePath, runFolder, stepID, summary, observedAt, lines, fingerprints,
	); err != nil {
		return recorded, err
	}
	return recorded, nil
}

func recordRunConcernLinesAtWithFingerprints(
	ctx context.Context,
	db pulseFindingLifecycleDB,
	runFolder, groupName, stepID, phase string,
	lines []string,
	observedAt string,
	fingerprints map[string]string,
) (int, error) {
	if strings.TrimSpace(observedAt) == "" {
		observedAt = time.Now().UTC().Format(time.RFC3339Nano)
	}
	recorded := 0
	for _, text := range lines {
		normalizedText := strings.ToLower(strings.Join(strings.Fields(text), " "))
		fp := fingerprints[normalizedText]
		if phase == ConcernPhasePreValidation {
			fp = preValidationConcernFingerprint(stepID)
		} else if fp == "" {
			fp = concernFingerprint(stepID, text)
		}
		if phase == ConcernPhaseReview && pulsemodules.IsValid(stepID) {
			if historical := existingCanonicalReviewFingerprint(ctx, db, stepID, text); historical != "" {
				fp = historical
			}
		}
		previousStatus := ""
		if err := db.QueryRowContext(ctx, `SELECT status FROM run_concerns WHERE fingerprint=?`, fp).Scan(&previousStatus); err != nil && err != sql.ErrNoRows {
			return recorded, err
		}
		// A concern that recurs after being marked resolved reopens: the fix did
		// not hold, and that is strictly more important than the original report.
		// A concern recurring while awaiting verification is the failed
		// verification signal itself, so it also returns to open.
		// A concern still recurring after the run it was waiting for means that
		// run happened and did not resolve it, so awaiting_run reopens too.
		// "rejected" is deliberately sticky — someone judged it a non-issue, and
		// recurrence is not new evidence against that judgement.
		// first_seen_platform_version is written on INSERT only; the ON CONFLICT
		// branch deliberately leaves it alone. It answers "what was this first
		// observed against", so a recurrence must not overwrite it with a newer
		// revision — that would erase exactly the signal a staleness sweep reads.
		_, err := db.ExecContext(ctx, `INSERT INTO run_concerns
			(fingerprint, step_id, phase, group_name, text, first_seen_run, first_seen_at, last_seen_run, last_seen_at, seen_count, status, first_seen_platform_version)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 1, ?, ?)
			ON CONFLICT(fingerprint) DO UPDATE SET
				text = excluded.text,
				phase = excluded.phase,
				group_name = excluded.group_name,
				last_seen_run = excluded.last_seen_run,
				last_seen_at = excluded.last_seen_at,
				seen_count = run_concerns.seen_count + CASE
					WHEN excluded.phase = 'prevalidation' AND run_concerns.last_seen_run = excluded.last_seen_run THEN 0
					ELSE 1
				END,
				status = CASE WHEN run_concerns.status IN (?, ?, ?) THEN ? ELSE run_concerns.status END`,
			fp, stepID, phase, groupName, text, runFolder, observedAt, runFolder, observedAt, ConcernStatusOpen, PlatformVersion(),
			ConcernStatusResolved, ConcernStatusAwaitingVerification, ConcernStatusAwaitingRun, ConcernStatusOpen)
		if err != nil {
			return recorded, err
		}
		eventType := "observed_again"
		switch previousStatus {
		case "":
			eventType = "filed"
		case ConcernStatusResolved, ConcernStatusAwaitingVerification, ConcernStatusAwaitingRun:
			eventType = "reopened"
		}
		metadata, _ := json.Marshal(map[string]string{
			"previous_status": previousStatus,
			"phase":           phase,
			"step_id":         stepID,
		})
		if _, err := db.ExecContext(ctx, `INSERT INTO pulse_finding_events
			(fingerprint, pulse_run_id, event_type, summary, metadata_json, recorded_at)
			VALUES (?, ?, ?, ?, ?, ?)
			ON CONFLICT(fingerprint, pulse_run_id, attempt_id, event_type) DO NOTHING`,
			fp, runFolder, eventType, text, string(metadata), observedAt); err != nil {
			return recorded, err
		}
		recorded++
	}
	return recorded, nil
}

// LoadOpenRunConcerns returns concerns still needing Pulse attention. A negative
// limit returns the complete active backlog; zero preserves the bounded default
// for callers that only need a preview.
func LoadOpenRunConcerns(ctx context.Context, workspacePath string, limit int) ([]RunConcern, error) {
	db, err := openRunConcernsDB(ctx, workspacePath, false)
	if err != nil || db == nil {
		return nil, err
	}
	defer db.Close()
	if limit == 0 {
		limit = 50
	}
	// Rank by how many active concerns a step has before ranking by how often
	// any one of them recurred. Prevalidation is already one root concern per
	// step; this clustering remains useful for distinct execution/review concerns
	// that share a step and should be read together.
	query := `SELECT c.fingerprint, c.step_id, c.phase, c.group_name, c.text,
			c.first_seen_run, c.first_seen_at, c.last_seen_run, c.last_seen_at, c.seen_count, c.status
		FROM run_concerns c
		JOIN (
			SELECT step_id, COUNT(*) AS active_count, MAX(seen_count) AS peak_seen
			FROM run_concerns
			WHERE status IN (?, ?, ?, ?, ?, ?)
			GROUP BY step_id
		) cluster ON cluster.step_id = c.step_id
		WHERE c.status IN (?, ?, ?, ?, ?, ?)
		ORDER BY cluster.active_count DESC, cluster.peak_seen DESC, c.step_id ASC,
			c.seen_count DESC, c.first_seen_at ASC, c.last_seen_at DESC`
	args := []interface{}{
		ConcernStatusOpen, ConcernStatusAcknowledged, ConcernStatusFixing, ConcernStatusAwaitingVerification, ConcernStatusAwaitingRun, ConcernStatusQueuedForEngineering,
		ConcernStatusOpen, ConcernStatusAcknowledged, ConcernStatusFixing, ConcernStatusAwaitingVerification, ConcernStatusAwaitingRun, ConcernStatusQueuedForEngineering,
	}
	if limit > 0 {
		query += ` LIMIT ?`
		args = append(args, limit)
	}
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		// Absent table just means no concern has been raised yet.
		if strings.Contains(strings.ToLower(err.Error()), "no such table") {
			return nil, nil
		}
		return nil, err
	}
	defer rows.Close()

	var out []RunConcern
	for rows.Next() {
		var c RunConcern
		if err := rows.Scan(&c.Fingerprint, &c.StepID, &c.Phase, &c.GroupName, &c.Text,
			&c.FirstSeenRun, &c.FirstSeenAt, &c.LastSeenRun, &c.LastSeenAt, &c.SeenCount, &c.Status); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// LoadExternallyOwnedRunConcerns returns findings Pulse has diagnosed as real
// but cannot act on in this workflow. Reviewers receive these as a suppression
// list; recurrence updates their audit history without putting them back in the
// active Pulse queue.
func LoadExternallyOwnedRunConcerns(ctx context.Context, workspacePath string) ([]RunConcern, error) {
	db, err := openRunConcernsDB(ctx, workspacePath, false)
	if err != nil || db == nil {
		return nil, err
	}
	defer db.Close()
	rows, err := db.QueryContext(ctx, `SELECT fingerprint, step_id, phase, group_name, text,
			first_seen_run, first_seen_at, last_seen_run, last_seen_at, seen_count, status,
			resolved_at, resolved_by, resolution_note
		FROM run_concerns
		WHERE status=?
		ORDER BY last_seen_at DESC, first_seen_at ASC`, ConcernStatusExternalActionRequired)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "no such table") {
			return nil, nil
		}
		return nil, err
	}
	defer rows.Close()
	var out []RunConcern
	for rows.Next() {
		var concern RunConcern
		if err := rows.Scan(
			&concern.Fingerprint, &concern.StepID, &concern.Phase, &concern.GroupName,
			&concern.Text, &concern.FirstSeenRun, &concern.FirstSeenAt,
			&concern.LastSeenRun, &concern.LastSeenAt, &concern.SeenCount,
			&concern.Status, &concern.ResolvedAt, &concern.ResolvedBy,
			&concern.ResolutionNote,
		); err != nil {
			return nil, err
		}
		out = append(out, concern)
	}
	return out, rows.Err()
}

// ResolveRunConcern records a terminal judgement on a concern.
//
// Absence never resolves a concern: a report that stops appearing may just mean
// the step did not run, or took a different path. Auto-closing on absence would
// quietly delete real findings, so closing one is always an explicit act.
func ResolveRunConcern(ctx context.Context, workspacePath, fingerprint, status, resolvedBy, note string) error {
	switch status {
	case ConcernStatusAcknowledged, ConcernStatusResolved, ConcernStatusRejected:
	default:
		return fmt.Errorf("invalid concern status %q", status)
	}
	db, err := openRunConcernsDB(ctx, workspacePath, false)
	if err != nil {
		return err
	}
	if db == nil {
		return fmt.Errorf("no workflow database at %s", runConcernsDBPath(workspacePath))
	}
	defer db.Close()

	res, err := db.ExecContext(ctx, `UPDATE run_concerns
		SET status = ?, resolved_at = ?, resolved_by = ?, resolution_note = ?
		WHERE fingerprint = ?`,
		status, time.Now().UTC().Format(time.RFC3339), resolvedBy, note, fingerprint)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("no concern with fingerprint %q", fingerprint)
	}
	return nil
}

// recordStepConcerns files concerns from each named phase summary.
//
// Best-effort on purpose: a step that completed its work must never be failed by
// a bookkeeping write. A failure here is logged and the run continues — the
// concern still appears inline in the completion summary either way, so the
// worst case is losing recurrence tracking for one occurrence, not the report.
func (hcpo *StepBasedWorkflowOrchestrator) recordStepConcerns(ctx context.Context, stepID string, summariesByPhase map[string]string) error {
	workspacePath := strings.TrimSpace(hcpo.GetWorkspacePath())
	if workspacePath == "" {
		return nil
	}
	var recordErrors []string
	for phase, summary := range summariesByPhase {
		if strings.TrimSpace(summary) == "" {
			continue
		}
		n, err := RecordRunConcerns(ctx, workspacePath, hcpo.selectedRunFolder, hcpo.currentGroupName, stepID, phase, summary)
		if err != nil {
			hcpo.GetLogger().Warn(fmt.Sprintf("⚠️ Failed to record %s concerns for step %s: %v", phase, stepID, err))
			recordErrors = append(recordErrors, phase+": "+err.Error())
			continue
		}
		if n > 0 {
			hcpo.GetLogger().Info(fmt.Sprintf("📌 Recorded %d %s concern(s) for step %s", n, phase, stepID))
		}
	}
	if len(recordErrors) > 0 {
		return fmt.Errorf("record concerns for %s: %s", stepID, strings.Join(recordErrors, "; "))
	}
	return nil
}

// LoadPriorPreValidationFailures returns this step's still-open prevalidation
// concerns, newest first, so the next run can be told what its last output got
// wrong.
//
// Why this exists. A scripted step that fails gets its own error handed back:
// controller_execution.go renders ScriptedPriorScript and ScriptedPriorError
// into the prompt, and the LLM repairs the script. An agentic step had no
// equivalent. It receives ValidationSchema — so it knows the shape it must
// produce — but prevalidation runs *after* it, and nothing carried the verdict
// forward. The step therefore read an identical prompt every run and wrote the
// same wrong shape every run.
//
// tectonicusadaytrading's deliver-briefing is the worked example: five concerns
// filed 2026-07-29, still open at seen_count 3 on 2026-07-30, all of the form
// "$.delivery_status must exist but was not found: unknown key". A stable
// contract mismatch that recurred purely because the only party able to fix it
// was never told.
//
// Reads run_concerns rather than the prevalidation log because these rows are
// already durable, already deduplicated by fingerprint, and already carry
// seen_count — recurrence is the strongest signal that the step is not learning,
// and it is free here.
func LoadPriorPreValidationFailures(ctx context.Context, workspacePath, stepID string, limit int) ([]RunConcern, error) {
	stepID = strings.TrimSpace(stepID)
	if stepID == "" {
		return nil, nil
	}
	db, err := openRunConcernsDB(ctx, workspacePath, false)
	if err != nil || db == nil {
		return nil, err
	}
	defer db.Close()
	if limit <= 0 {
		limit = 10
	}
	rows, err := db.QueryContext(ctx, `SELECT fingerprint, step_id, phase, group_name, text,
			first_seen_run, first_seen_at, last_seen_run, last_seen_at, seen_count, status,
			resolved_at, resolved_by, resolution_note
		FROM run_concerns
		WHERE step_id=? AND phase=? AND status NOT IN (?, ?, ?)
		ORDER BY seen_count DESC, last_seen_at DESC
		LIMIT ?`,
		stepID, ConcernPhasePreValidation,
		ConcernStatusResolved, ConcernStatusRejected, ConcernStatusExternalActionRequired,
		limit)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "no such table") {
			return nil, nil
		}
		return nil, err
	}
	defer rows.Close()
	var out []RunConcern
	for rows.Next() {
		var concern RunConcern
		if err := rows.Scan(
			&concern.Fingerprint, &concern.StepID, &concern.Phase, &concern.GroupName,
			&concern.Text, &concern.FirstSeenRun, &concern.FirstSeenAt,
			&concern.LastSeenRun, &concern.LastSeenAt, &concern.SeenCount,
			&concern.Status, &concern.ResolvedAt, &concern.ResolvedBy,
			&concern.ResolutionNote,
		); err != nil {
			return nil, err
		}
		out = append(out, concern)
	}
	return out, rows.Err()
}

// FormatPriorPreValidationFailures renders concerns for a step prompt. Returns
// "" when there is nothing to say, so the caller can leave the template variable
// empty and the block disappears entirely rather than rendering an empty header.
func FormatPriorPreValidationFailures(concerns []RunConcern) string {
	if len(concerns) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("A previous run of this step produced output that failed its own validation schema. ")
	b.WriteString("These are unresolved. Produce output that satisfies them this time; if the schema itself is wrong, say so explicitly in your summary as a CONCERNS: line rather than writing output you know will fail.\n")
	for _, concern := range concerns {
		text := strings.TrimSpace(concern.Text)
		if text == "" {
			continue
		}
		// seen_count is the part that matters most: "failed once" is noise, and
		// "failed on three consecutive runs" means the step has been reading its
		// schema and getting it wrong repeatedly.
		if concern.SeenCount > 1 {
			fmt.Fprintf(&b, "- %s (seen on %d runs)\n", text, concern.SeenCount)
			continue
		}
		fmt.Fprintf(&b, "- %s\n", text)
	}
	return strings.TrimRight(b.String(), "\n")
}
