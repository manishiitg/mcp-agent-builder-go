package step_based_workflow

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// A durable record of which Pulse reviewers actually ran and what they concluded.
//
// Pulse Gate chooses which modules to review each cycle. It had no way to tell a
// module that keeps finding real problems from one that never finds anything,
// because nothing recorded the outcome — so the choice drifted to habit in one
// workflow (the same four modules every run) and to everything-at-once in another.
//
// The evidence that this was costing real findings: confida ran seven reviewers
// in one cycle. All seven wrote substantive artifacts to disk — the knowledgebase
// reviewer caught a half-finished migration where two live consumers were reading
// a deleted path and silently getting nothing. Every one of those seven left
// `last_result` empty in pulse_module_state, because recording the outcome depends
// on the Pulse Fixer calling mark_pulse_module_result, and it did not. A cycle
// that found a live breakage left no trace for the next cycle to learn from.
//
// So this is written by Go at the moment the backend persists a reviewer artifact
// — the same choke point that files reviewer concerns. There is no call for an
// agent to skip.
//
// Kept separate from pulse_module_audit deliberately. That table records the
// FIXER's outcome for a module (done/changed/blocked). This records that a
// REVIEWER ran and what it concluded, which is a different fact: a review can
// find something real and still be followed by no fix at all.

const pulseReviewLogSchema = `CREATE TABLE IF NOT EXISTS pulse_review_log (
	_id INTEGER PRIMARY KEY AUTOINCREMENT,
	module TEXT NOT NULL,
	review_run_id TEXT NOT NULL DEFAULT '',
	pulse_run_id TEXT NOT NULL DEFAULT '',
	verdict TEXT NOT NULL DEFAULT '',
	artifact_path TEXT NOT NULL DEFAULT '',
	artifact_bytes INTEGER NOT NULL DEFAULT 0,
	recorded_at TEXT NOT NULL
)`

const pulseReviewLogIndex = `CREATE INDEX IF NOT EXISTS idx_pulse_review_log_module ON pulse_review_log(module, recorded_at)`

// maxVerdictChars keeps one recorded verdict to a scannable length. The full text
// stays in the reviewer artifact; this is the index into it.
const maxVerdictChars = 400

// ModuleReviewHistory summarizes what one reviewer has been finding.
type ModuleReviewHistory struct {
	Module        string   `json:"module"`
	RunCount      int      `json:"run_count"`
	LastRanAt     string   `json:"last_ran_at,omitempty"`
	RecentVerdict []string `json:"recent_verdicts,omitempty"`
}

// extractReviewVerdict pulls the reviewer's conclusion out of its artifact.
//
// Reviewers write markdown, and the verdict shows up as `## Verdict`, `Verdict:`,
// or `**Verdict:**` depending on the module. Handles the heading form (where the
// text is on the following lines) as well as the inline form.
//
// Returns "" when no verdict is found rather than guessing: an empty verdict with
// a recorded run is still useful — it says the reviewer ran — whereas inventing
// one would poison the history the Gate is meant to trust.
func extractReviewVerdict(artifact string) string {
	lines := strings.Split(artifact, "\n")
	for i, raw := range lines {
		line := strings.TrimSpace(raw)
		lower := strings.ToLower(strings.TrimLeft(line, "#* "))
		if !strings.HasPrefix(lower, "verdict") {
			continue
		}
		// Inline form: "Verdict: <text>" / "**Verdict:** <text>".
		if idx := strings.Index(line, ":"); idx >= 0 {
			if rest := strings.TrimSpace(strings.Trim(line[idx+1:], "* ")); rest != "" {
				return truncateVerdict(rest)
			}
		}
		// Heading form: text begins on the next non-empty line.
		for _, next := range lines[i+1:] {
			if candidate := strings.TrimSpace(next); candidate != "" {
				return truncateVerdict(candidate)
			}
		}
		return ""
	}
	return ""
}

func truncateVerdict(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) <= maxVerdictChars {
		return s
	}
	return s[:maxVerdictChars] + "…"
}

// RecordPulseReview logs that a reviewer ran and what it concluded.
//
// Best-effort: a reviewer whose artifact was persisted must not fail because the
// bookkeeping write did. Callers log and continue.
func RecordPulseReview(ctx context.Context, workspacePath, module, reviewRunID, pulseRunID, artifactPath, artifact string) error {
	module = strings.TrimSpace(module)
	if module == "" {
		return nil
	}
	db, err := openRunConcernsDB(ctx, workspacePath, true)
	if err != nil || db == nil {
		return err
	}
	defer db.Close()
	for _, ddl := range []string{pulseReviewLogSchema, pulseReviewLogIndex} {
		if _, err := db.ExecContext(ctx, ddl); err != nil {
			return err
		}
	}
	_, err = db.ExecContext(ctx, `INSERT INTO pulse_review_log
		(module, review_run_id, pulse_run_id, verdict, artifact_path, artifact_bytes, recorded_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		module, strings.TrimSpace(reviewRunID), strings.TrimSpace(pulseRunID),
		extractReviewVerdict(artifact), strings.TrimSpace(artifactPath),
		len(artifact), time.Now().UTC().Format(time.RFC3339))
	return err
}

// LoadModuleReviewHistory summarizes recent reviews per module, most recently run
// first, so Gate can weigh "this one keeps finding things" against "this one has
// come back clean five times".
func LoadModuleReviewHistory(ctx context.Context, workspacePath string, perModule int) ([]ModuleReviewHistory, error) {
	db, err := openRunConcernsDB(ctx, workspacePath, false)
	if err != nil || db == nil {
		return nil, err
	}
	defer db.Close()
	if perModule <= 0 {
		perModule = 3
	}

	rows, err := db.QueryContext(ctx, `SELECT module, COUNT(*), MAX(recorded_at)
		FROM pulse_review_log GROUP BY module ORDER BY MAX(recorded_at) DESC`)
	if err != nil {
		// No table means no reviewer has run yet — that is "nothing to report",
		// not an error the Gate has to handle.
		if strings.Contains(strings.ToLower(err.Error()), "no such table") {
			return nil, nil
		}
		return nil, err
	}
	var out []ModuleReviewHistory
	for rows.Next() {
		var h ModuleReviewHistory
		if err := rows.Scan(&h.Module, &h.RunCount, &h.LastRanAt); err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, h)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()

	for i := range out {
		verdicts, err := recentVerdictsForModule(ctx, db, out[i].Module, perModule)
		if err != nil {
			return nil, err
		}
		out[i].RecentVerdict = verdicts
	}
	return out, nil
}

func recentVerdictsForModule(ctx context.Context, db *sql.DB, module string, limit int) ([]string, error) {
	rows, err := db.QueryContext(ctx, `SELECT recorded_at, verdict FROM pulse_review_log
		WHERE module = ? ORDER BY recorded_at DESC LIMIT ?`, module, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var at, verdict string
		if err := rows.Scan(&at, &verdict); err != nil {
			return nil, err
		}
		if strings.TrimSpace(verdict) == "" {
			verdict = "(ran; no verdict line found in the artifact)"
		}
		out = append(out, fmt.Sprintf("%s — %s", at, verdict))
	}
	return out, rows.Err()
}
