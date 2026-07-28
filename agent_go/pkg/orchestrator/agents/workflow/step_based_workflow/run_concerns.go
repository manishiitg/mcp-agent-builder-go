package step_based_workflow

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/fsutil"

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
	resolution_note TEXT NOT NULL DEFAULT ''
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
)

const (
	ConcernStatusOpen         = "open"
	ConcernStatusAcknowledged = "acknowledged"
	ConcernStatusResolved     = "resolved"
	ConcernStatusRejected     = "rejected"
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
	lines := ParseConcernLines(summary)
	if len(lines) == 0 {
		return 0, nil
	}
	db, err := openRunConcernsDB(ctx, workspacePath, true)
	if err != nil || db == nil {
		return 0, err
	}
	defer db.Close()
	if _, err := db.ExecContext(ctx, runConcernsSchema); err != nil {
		return 0, err
	}

	now := time.Now().UTC().Format(time.RFC3339)
	recorded := 0
	for _, text := range lines {
		fp := concernFingerprint(stepID, text)
		// A concern that recurs after being marked resolved reopens: the fix did
		// not hold, and that is strictly more important than the original report.
		// "rejected" is deliberately sticky — someone judged it a non-issue, and
		// recurrence is not new evidence against that judgement.
		_, err := db.ExecContext(ctx, `INSERT INTO run_concerns
			(fingerprint, step_id, phase, group_name, text, first_seen_run, first_seen_at, last_seen_run, last_seen_at, seen_count, status)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 1, ?)
			ON CONFLICT(fingerprint) DO UPDATE SET
				text = excluded.text,
				phase = excluded.phase,
				group_name = excluded.group_name,
				last_seen_run = excluded.last_seen_run,
				last_seen_at = excluded.last_seen_at,
				seen_count = run_concerns.seen_count + 1,
				status = CASE WHEN run_concerns.status = ? THEN ? ELSE run_concerns.status END`,
			fp, stepID, phase, groupName, text, runFolder, now, runFolder, now, ConcernStatusOpen,
			ConcernStatusResolved, ConcernStatusOpen)
		if err != nil {
			return recorded, err
		}
		recorded++
	}
	return recorded, nil
}

// LoadOpenRunConcerns returns concerns still needing attention, most-recurring
// first. Recurrence is the ranking signal: a contradiction reported on twelve
// consecutive runs matters more than one seen once, and that ordering is not
// derivable from anything else in the workflow today.
func LoadOpenRunConcerns(ctx context.Context, workspacePath string, limit int) ([]RunConcern, error) {
	db, err := openRunConcernsDB(ctx, workspacePath, false)
	if err != nil || db == nil {
		return nil, err
	}
	defer db.Close()
	if limit <= 0 {
		limit = 50
	}
	rows, err := db.QueryContext(ctx, `SELECT fingerprint, step_id, phase, group_name, text,
			first_seen_run, first_seen_at, last_seen_run, last_seen_at, seen_count, status
		FROM run_concerns
		WHERE status IN (?, ?)
		ORDER BY seen_count DESC, last_seen_at DESC
		LIMIT ?`, ConcernStatusOpen, ConcernStatusAcknowledged, limit)
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

// ResolveRunConcern records a terminal judgement on a concern.
//
// Absence never resolves a concern: a report that stops appearing may just mean
// the step did not run, or took a different path. Auto-closing on absence would
// quietly delete real findings, so closing one is always an explicit act.
func ResolveRunConcern(ctx context.Context, workspacePath, fingerprint, status, resolvedBy, note string) error {
	switch status {
	case ConcernStatusAcknowledged, ConcernStatusResolved, ConcernStatusRejected:
	default:
		return fmt.Errorf("invalid concern status %q (want acknowledged, resolved, or rejected)", status)
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
func (hcpo *StepBasedWorkflowOrchestrator) recordStepConcerns(ctx context.Context, stepID string, summariesByPhase map[string]string) {
	workspacePath := strings.TrimSpace(hcpo.GetWorkspacePath())
	if workspacePath == "" {
		return
	}
	for phase, summary := range summariesByPhase {
		if strings.TrimSpace(summary) == "" {
			continue
		}
		n, err := RecordRunConcerns(ctx, workspacePath, hcpo.selectedRunFolder, hcpo.currentGroupName, stepID, phase, summary)
		if err != nil {
			hcpo.GetLogger().Warn(fmt.Sprintf("⚠️ Failed to record %s concerns for step %s: %v", phase, stepID, err))
			continue
		}
		if n > 0 {
			hcpo.GetLogger().Info(fmt.Sprintf("📌 Recorded %d %s concern(s) for step %s", n, phase, stepID))
		}
	}
}
