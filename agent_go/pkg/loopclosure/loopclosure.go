// Package loopclosure deterministically finds Pulse work that has stalled
// past its own deadline.
//
// Why this exists, measured rather than assumed. A 2026-07-29 census of
// report_human_inputs across every workflow found:
//
//	56 consumed | 28 answered | 6 dismissed | 5 pending
//
// `answered` means the operator already replied and nothing has acted on it.
// The system owed the operator 5.6x more than the operator owed the system.
// Six of those answers were 5-9 days old in workflows that had completed a
// full Pulse pass within the previous two hours — the answer was sitting
// there, a pass ran, and it was not consumed.
//
// Every check here is arithmetic over state that already exists. None of it
// needs an LLM. The system currently relies on an agent *noticing* a stalled
// item, and agents do not reliably notice.
//
// This is deliberately NOT a scheduler and NOT a replacement for Pulse Gate.
// Gate decides which reviewers run; this reports which loops failed to close,
// as a mandatory input to that decision.
package loopclosure

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/fsutil"

	_ "modernc.org/sqlite"
)

// Kinds of stalled loop. Stable strings — they appear in Gate evidence and in
// operator-facing Pulse text.
const (
	// KindStaleAnswer is the dominant measured failure: the operator answered,
	// a later Pulse pass completed, and the answer was still not consumed.
	KindStaleAnswer = "answer_not_applied"
	// KindPendingAged is a question the operator has not answered for long
	// enough that it is probably blocking something.
	KindPendingAged = "decision_waiting_on_user"
	// KindRecurringConcern is a concern re-reported across many runs while
	// still open — recurrence is the signal that it was never really handled.
	KindRecurringConcern = "concern_keeps_recurring"
)

const (
	SeverityHigh   = "high"
	SeverityMedium = "medium"
)

// Config holds the thresholds. Defaults are deliberately conservative: this
// layer should report obvious stalls, not manufacture urgency.
type Config struct {
	// PendingAgedAfter is how long an unanswered question may sit before it is
	// reported. It is not a failure — the operator may simply be busy — but a
	// question nobody surfaces again is one nobody answers.
	PendingAgedAfter time.Duration
	// ConcernRecurrenceThreshold is the seen_count at or above which a still-open
	// concern is reported.
	ConcernRecurrenceThreshold int
	// StaleAnswerGrace is how far an answer must predate a completed Gate pass
	// before that pass counts as having ignored it.
	//
	// Without this the check is wrong in a specific, common way: an operator
	// answering while Pulse is mid-run produces an answer timestamp seconds
	// before the pass's completion timestamp. Running against production found
	// exactly that — upwork answers at 16:52:06 and 16:52:12 against a pass
	// completing 16:52:34, 22 seconds later. That pass was already underway and
	// never had a fair chance to consume them. Reporting it would generate a
	// false stall on every pass the operator happens to answer during.
	StaleAnswerGrace time.Duration
}

// DefaultConfig reflects the observed data: real answers were stalling at
// 5-9 days, and the worst recurring concern had been seen 3 times.
func DefaultConfig() Config {
	return Config{
		PendingAgedAfter:           72 * time.Hour,
		ConcernRecurrenceThreshold: 3,
		StaleAnswerGrace:           10 * time.Minute,
	}
}

// Finding is one loop that did not close.
type Finding struct {
	Kind     string `json:"kind"`
	Severity string `json:"severity"`
	// Subject identifies the stalled item in operator language, never a raw id.
	Subject string `json:"subject"`
	// Detail says what should have happened and did not.
	Detail string `json:"detail"`
	// Evidence is the arithmetic that proves it, so Gate can cite it without
	// re-deriving anything.
	Evidence string `json:"evidence"`
	AgeDays  int    `json:"age_days"`
	// ID is the durable identifier for the underlying row, kept out of Subject
	// so operator-facing text stays clean.
	ID string `json:"id,omitempty"`
}

// HumanInput is the subset of report_human_inputs this package reasons about.
type HumanInput struct {
	ID        string
	Source    string
	Question  string
	Status    string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Concern is the subset of run_concerns this package reasons about.
type Concern struct {
	Fingerprint string
	Text        string
	Status      string
	SeenCount   int
	LastSeenAt  time.Time
}

// Evaluate is the pure decision core: given state, return stalled loops.
// Separated from SQL so the rules are testable without a database.
//
// lastGatePass is the most recent completed Pulse Gate pass. A zero value
// means Pulse has never run here, in which case no answer can be called
// stale — there has been no opportunity to consume it.
func Evaluate(now, lastGatePass time.Time, inputs []HumanInput, concerns []Concern, cfg Config) []Finding {
	if cfg.PendingAgedAfter <= 0 {
		cfg.PendingAgedAfter = DefaultConfig().PendingAgedAfter
	}
	if cfg.ConcernRecurrenceThreshold <= 0 {
		cfg.ConcernRecurrenceThreshold = DefaultConfig().ConcernRecurrenceThreshold
	}
	if cfg.StaleAnswerGrace <= 0 {
		cfg.StaleAnswerGrace = DefaultConfig().StaleAnswerGrace
	}

	var out []Finding

	for _, in := range inputs {
		switch strings.ToLower(strings.TrimSpace(in.Status)) {
		case "answered":
			// The core invariant. If a Gate pass completed after the operator
			// answered and the answer is still unconsumed, that pass had the
			// answer available and did not act on it. That is not a judgement
			// call — it is a timestamp comparison.
			if lastGatePass.IsZero() || !in.UpdatedAt.Before(lastGatePass.Add(-cfg.StaleAnswerGrace)) {
				continue
			}
			out = append(out, Finding{
				Kind:     KindStaleAnswer,
				Severity: SeverityHigh,
				Subject:  summarize(in.Question),
				Detail:   "You answered this, but no Pulse pass since then has applied it.",
				Evidence: fmt.Sprintf("answered %s; a Pulse pass completed %s and left it unapplied",
					in.UpdatedAt.UTC().Format(time.RFC3339), lastGatePass.UTC().Format(time.RFC3339)),
				AgeDays: daysBetween(in.UpdatedAt, now),
				ID:      in.ID,
			})
		case "pending":
			age := now.Sub(in.CreatedAt)
			if age < cfg.PendingAgedAfter {
				continue
			}
			out = append(out, Finding{
				Kind:     KindPendingAged,
				Severity: SeverityMedium,
				Subject:  summarize(in.Question),
				Detail:   "This is still waiting on your answer and may be blocking work.",
				Evidence: fmt.Sprintf("asked %s, unanswered for %d day(s)",
					in.CreatedAt.UTC().Format(time.RFC3339), daysBetween(in.CreatedAt, now)),
				AgeDays: daysBetween(in.CreatedAt, now),
				ID:      in.ID,
			})
		}
	}

	for _, c := range concerns {
		status := strings.ToLower(strings.TrimSpace(c.Status))
		if status != "open" && status != "acknowledged" {
			continue
		}
		if c.SeenCount < cfg.ConcernRecurrenceThreshold {
			continue
		}
		out = append(out, Finding{
			Kind:     KindRecurringConcern,
			Severity: SeverityHigh,
			Subject:  summarize(c.Text),
			Detail:   "A step has raised this on several runs and it is still open.",
			Evidence: fmt.Sprintf("reported on %d runs, still %s, last seen %s",
				c.SeenCount, status, c.LastSeenAt.UTC().Format(time.RFC3339)),
			AgeDays: daysBetween(c.LastSeenAt, now),
			ID:      c.Fingerprint,
		})
	}

	// Highest severity first, then oldest — the order Gate should read them in.
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Severity != out[j].Severity {
			return out[i].Severity == SeverityHigh
		}
		return out[i].AgeDays > out[j].AgeDays
	})
	return out
}

// Check reads the workflow's db.sqlite and reports stalled loops. Best-effort
// by contract: a missing database or table means "nothing to report", never an
// error that could block a Pulse pass.
func Check(ctx context.Context, workspacePath string, now time.Time) ([]Finding, error) {
	db, err := openReadOnly(workspacePath)
	if err != nil || db == nil {
		return nil, err
	}
	defer db.Close()

	lastGatePass := queryLastGatePass(ctx, db)
	inputs := queryHumanInputs(ctx, db)
	concerns := queryConcerns(ctx, db)

	return Evaluate(now, lastGatePass, inputs, concerns, DefaultConfig()), nil
}

func dbPath(workspacePath string) string {
	return filepath.Join(fsutil.WorkspaceDocsRoot(),
		filepath.FromSlash(strings.Trim(strings.TrimSpace(workspacePath), "/")), "db", "db.sqlite")
}

func openReadOnly(workspacePath string) (*sql.DB, error) {
	path := dbPath(workspacePath)
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	// Read-only: this layer observes, it never mutates workflow state.
	db, err := sql.Open("sqlite", path+"?mode=ro")
	if err != nil {
		return nil, err
	}
	return db, nil
}

// queryLastGatePass returns the most recent completed Gate pass.
// last_checked_at is the right field: Gate records a decision for every module
// each pass, so it advances even for skipped modules, whereas last_ran_at only
// advances for modules that actually ran.
func queryLastGatePass(ctx context.Context, db *sql.DB) time.Time {
	var raw sql.NullString
	err := db.QueryRowContext(ctx,
		`SELECT MAX(last_checked_at) FROM pulse_module_state WHERE last_checked_at != ''`).Scan(&raw)
	if err != nil || !raw.Valid {
		return time.Time{}
	}
	return parseTime(raw.String)
}

func queryHumanInputs(ctx context.Context, db *sql.DB) []HumanInput {
	rows, err := db.QueryContext(ctx,
		`SELECT id, source, question, status, created_at, updated_at
		 FROM report_human_inputs WHERE status IN ('answered','pending')`)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var out []HumanInput
	for rows.Next() {
		var in HumanInput
		var created, updated string
		if err := rows.Scan(&in.ID, &in.Source, &in.Question, &in.Status, &created, &updated); err != nil {
			return out
		}
		in.CreatedAt = parseTime(created)
		in.UpdatedAt = parseTime(updated)
		out = append(out, in)
	}
	return out
}

func queryConcerns(ctx context.Context, db *sql.DB) []Concern {
	rows, err := db.QueryContext(ctx,
		`SELECT fingerprint, text, status, seen_count, last_seen_at
		 FROM run_concerns WHERE status IN ('open','acknowledged')`)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var out []Concern
	for rows.Next() {
		var c Concern
		var lastSeen string
		if err := rows.Scan(&c.Fingerprint, &c.Text, &c.Status, &c.SeenCount, &lastSeen); err != nil {
			return out
		}
		c.LastSeenAt = parseTime(lastSeen)
		out = append(out, c)
	}
	return out
}

// parseTime accepts the RFC3339 shapes actually present in these tables, with
// and without a trailing Z, and returns a zero time for anything else.
func parseTime(s string) time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02T15:04:05"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC()
		}
	}
	return time.Time{}
}

func daysBetween(from, to time.Time) int {
	if from.IsZero() || to.Before(from) {
		return 0
	}
	return int(to.Sub(from).Hours() / 24)
}

// summarize keeps operator-facing text short and free of raw identifiers.
func summarize(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	const max = 110
	if len(s) <= max {
		return s
	}
	return strings.TrimSpace(s[:max]) + "…"
}
