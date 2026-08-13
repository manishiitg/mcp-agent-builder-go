package server

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/mux"
	mcpexecutor "github.com/manishiitg/mcpagent/executor"
	"github.com/manishiitg/multi-llm-provider-go/llmtypes"
	_ "modernc.org/sqlite"
)

var reportHumanInputStoreMu sync.Mutex

type ReportHumanInputOption struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
}

type ReportHumanInput struct {
	ID                string                   `json:"id"`
	WorkspacePath     string                   `json:"workspace_path"`
	Source            string                   `json:"source"`
	Priority          string                   `json:"priority"`
	Question          string                   `json:"question"`
	Context           string                   `json:"context,omitempty"`
	Options           []ReportHumanInputOption `json:"options"`
	AllowFreeText     bool                     `json:"allow_free_text"`
	Status            string                   `json:"status"`
	SelectedOptionID  string                   `json:"selected_option_id,omitempty"`
	Note              string                   `json:"note,omitempty"`
	RunID             string                   `json:"run_id,omitempty"`
	Evidence          string                   `json:"evidence,omitempty"`
	CreatedBy         string                   `json:"created_by,omitempty"`
	AnsweredBy        string                   `json:"answered_by,omitempty"`
	AnsweredByKind    string                   `json:"answered_by_kind,omitempty"`
	AnsweredVia       string                   `json:"answered_via,omitempty"`
	AnsweredSessionID string                   `json:"answered_session_id,omitempty"`
	ConsumedBy        string                   `json:"consumed_by,omitempty"`
	OutcomeSummary    string                   `json:"outcome_summary,omitempty"`
	CreatedAt         string                   `json:"created_at"`
	UpdatedAt         string                   `json:"updated_at"`
	AnsweredAt        string                   `json:"answered_at,omitempty"`
	ConsumedAt        string                   `json:"consumed_at,omitempty"`
	DismissedAt       string                   `json:"dismissed_at,omitempty"`
	ClaimToken        string                   `json:"claim_token,omitempty"`
	ClaimedAt         string                   `json:"claimed_at,omitempty"`
	ClaimExpiresAt    string                   `json:"claim_expires_at,omitempty"`
}

type ReportHumanInputCreateRequest struct {
	WorkspacePath string                   `json:"workspace_path"`
	InputID       string                   `json:"input_id"`
	Source        string                   `json:"source"`
	Priority      string                   `json:"priority"`
	Question      string                   `json:"question"`
	Context       string                   `json:"context"`
	Options       []ReportHumanInputOption `json:"options"`
	AllowFreeText bool                     `json:"allow_free_text"`
	RunID         string                   `json:"run_id"`
	Evidence      string                   `json:"evidence"`
	CreatedBy     string                   `json:"created_by"`
	CreatedByKind string                   `json:"-"`
	CreatedVia    string                   `json:"-"`
	SessionID     string                   `json:"-"`
}

type ReportHumanInputAnswerRequest struct {
	WorkspacePath    string `json:"workspace_path"`
	SelectedOptionID string `json:"selected_option_id"`
	Note             string `json:"note"`
	AnsweredBy       string `json:"answered_by"`
	AnsweredByKind   string `json:"-"`
	AnsweredVia      string `json:"-"`
	SessionID        string `json:"-"`
}

type ReportHumanInputConsumeRequest struct {
	WorkspacePath  string `json:"workspace_path"`
	OutcomeSummary string `json:"outcome_summary"`
	ConsumedBy     string `json:"consumed_by"`
	ConsumedByKind string `json:"-"`
	ConsumedVia    string `json:"-"`
	SessionID      string `json:"-"`
}

type reportHumanInputEvent struct {
	InputID   string
	EventType string
	Status    string
	ActorID   string
	ActorKind string
	Channel   string
	SessionID string
	Details   string
	CreatedAt string
}

func normalizeReportHumanInputWorkspacePath(workspacePath string) (string, error) {
	cleaned := filepath.ToSlash(filepath.Clean(strings.Trim(strings.TrimSpace(workspacePath), "/")))
	if cleaned == "." || cleaned == "" {
		return "", fmt.Errorf("workspace_path is required")
	}
	if strings.HasPrefix(cleaned, "/") || strings.Contains(cleaned, "\x00") {
		return "", fmt.Errorf("workspace_path must be workspace-relative")
	}
	for _, part := range strings.Split(cleaned, "/") {
		if part == ".." {
			return "", fmt.Errorf("workspace_path cannot contain ..")
		}
	}
	return cleaned, nil
}

func reportHumanInputDBPath(workspacePath string) (string, string, error) {
	normalized, err := normalizeReportHumanInputWorkspacePath(workspacePath)
	if err != nil {
		return "", "", err
	}
	root, err := filepath.Abs(getWorkspaceDocsAbsPath())
	if err != nil {
		return "", "", err
	}
	dbPath, err := filepath.Abs(filepath.Join(root, filepath.FromSlash(normalized), "db", "db.sqlite"))
	if err != nil {
		return "", "", err
	}
	rel, err := filepath.Rel(root, dbPath)
	if err != nil || strings.HasPrefix(rel, "..") || filepath.IsAbs(rel) {
		return "", "", fmt.Errorf("workspace_path escapes workspace docs root")
	}
	return normalized, dbPath, nil
}

func openReportHumanInputDB(ctx context.Context, workspacePath string, create bool) (string, *sql.DB, error) {
	normalized, dbPath, err := reportHumanInputDBPath(workspacePath)
	if err != nil {
		return "", nil, err
	}
	if create {
		if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
			return "", nil, err
		}
	} else if _, err := os.Stat(dbPath); err != nil {
		if os.IsNotExist(err) {
			return normalized, nil, nil
		}
		return "", nil, err
	}
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return "", nil, err
	}
	if _, err := db.ExecContext(ctx, "PRAGMA busy_timeout = 5000"); err != nil {
		_ = db.Close()
		return "", nil, err
	}
	if err := ensureReportHumanInputSchema(ctx, db); err != nil {
		_ = db.Close()
		return "", nil, err
	}
	return normalized, db, nil
}

func ensureReportHumanInputSchema(ctx context.Context, db *sql.DB) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS report_human_inputs (
			id TEXT PRIMARY KEY,
			workspace_path TEXT NOT NULL,
			source TEXT NOT NULL,
			priority TEXT NOT NULL DEFAULT 'medium',
			question TEXT NOT NULL,
			context TEXT NOT NULL DEFAULT '',
			options_json TEXT NOT NULL DEFAULT '[]',
			allow_free_text INTEGER NOT NULL DEFAULT 0,
			status TEXT NOT NULL,
			selected_option_id TEXT NOT NULL DEFAULT '',
			note TEXT NOT NULL DEFAULT '',
			run_id TEXT NOT NULL DEFAULT '',
			evidence TEXT NOT NULL DEFAULT '',
			created_by TEXT NOT NULL DEFAULT '',
			answered_by TEXT NOT NULL DEFAULT '',
			answered_by_kind TEXT NOT NULL DEFAULT '',
			answered_via TEXT NOT NULL DEFAULT '',
			answered_session_id TEXT NOT NULL DEFAULT '',
			consumed_by TEXT NOT NULL DEFAULT '',
			outcome_summary TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			answered_at TEXT NOT NULL DEFAULT '',
			consumed_at TEXT NOT NULL DEFAULT '',
			dismissed_at TEXT NOT NULL DEFAULT '',
			claim_token TEXT NOT NULL DEFAULT '',
			claimed_at TEXT NOT NULL DEFAULT '',
			claim_expires_at TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE INDEX IF NOT EXISTS idx_report_human_inputs_status ON report_human_inputs(status, updated_at)`,
		`CREATE INDEX IF NOT EXISTS idx_report_human_inputs_source ON report_human_inputs(source, status, updated_at)`,
		`CREATE TABLE IF NOT EXISTS report_human_input_events (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			workspace_path TEXT NOT NULL,
			input_id TEXT NOT NULL,
			event_type TEXT NOT NULL,
			status TEXT NOT NULL,
			actor_id TEXT NOT NULL DEFAULT '',
			actor_kind TEXT NOT NULL DEFAULT '',
			channel TEXT NOT NULL DEFAULT '',
			session_id TEXT NOT NULL DEFAULT '',
			details TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_report_human_input_events_lookup
			ON report_human_input_events(workspace_path, input_id, id)`,
	}
	for _, stmt := range stmts {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}
	for name, definition := range map[string]string{
		"claim_token":         "TEXT NOT NULL DEFAULT ''",
		"claimed_at":          "TEXT NOT NULL DEFAULT ''",
		"claim_expires_at":    "TEXT NOT NULL DEFAULT ''",
		"answered_by_kind":    "TEXT NOT NULL DEFAULT ''",
		"answered_via":        "TEXT NOT NULL DEFAULT ''",
		"answered_session_id": "TEXT NOT NULL DEFAULT ''",
	} {
		if err := ensureReportHumanInputColumn(ctx, db, name, definition); err != nil {
			return err
		}
	}
	// Historical rows predate trusted channel attribution. Preserve their user
	// label but classify the provenance honestly instead of leaving an empty
	// value that callers may mistake for a current verified UI answer.
	if _, err := db.ExecContext(ctx, `UPDATE report_human_inputs
		SET answered_by_kind='legacy_unattributed', answered_via='legacy'
		WHERE answered_by<>'' AND answered_by_kind=''`); err != nil {
		return err
	}
	return nil
}

type reportHumanInputEventExecer interface {
	ExecContext(context.Context, string, ...interface{}) (sql.Result, error)
}

func writeReportHumanInputEvent(ctx context.Context, execer reportHumanInputEventExecer, workspacePath string, event reportHumanInputEvent) error {
	event.ActorKind = normalizeReportHumanInputActorKind(event.ActorKind)
	event.Channel = strings.TrimSpace(event.Channel)
	if event.Channel == "" {
		event.Channel = "unknown"
	}
	if event.CreatedAt == "" {
		event.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	}
	_, err := execer.ExecContext(ctx, `INSERT INTO report_human_input_events
		(workspace_path, input_id, event_type, status, actor_id, actor_kind, channel, session_id, details, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, workspacePath, strings.TrimSpace(event.InputID),
		strings.TrimSpace(event.EventType), strings.TrimSpace(event.Status), strings.TrimSpace(event.ActorID),
		event.ActorKind, event.Channel, strings.TrimSpace(event.SessionID), strings.TrimSpace(event.Details), event.CreatedAt)
	return err
}

func normalizeReportHumanInputActorKind(kind string) string {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "human_ui", "human_via_chat", "agent", "system", "migration":
		return strings.ToLower(strings.TrimSpace(kind))
	default:
		return "legacy_unattributed"
	}
}

func ensureReportHumanInputColumn(ctx context.Context, db *sql.DB, column, definition string) error {
	rows, err := db.QueryContext(ctx, "PRAGMA table_info(report_human_inputs)")
	if err != nil {
		return err
	}
	found := false
	for rows.Next() {
		var cid int
		var name, kind string
		var notNull, primaryKey int
		var defaultValue interface{}
		if err := rows.Scan(&cid, &name, &kind, &notNull, &defaultValue, &primaryKey); err != nil {
			_ = rows.Close()
			return err
		}
		if strings.EqualFold(name, column) {
			found = true
		}
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if found {
		return nil
	}
	_, err = db.ExecContext(ctx, fmt.Sprintf("ALTER TABLE report_human_inputs ADD COLUMN %s %s", column, definition))
	return err
}

func createReportHumanInput(ctx context.Context, workspacePath string, req ReportHumanInputCreateRequest) (*ReportHumanInput, error) {
	reportHumanInputStoreMu.Lock()
	defer reportHumanInputStoreMu.Unlock()

	normalized, db, err := openReportHumanInputDB(ctx, workspacePath, true)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	question := strings.TrimSpace(req.Question)
	if question == "" {
		return nil, fmt.Errorf("question is required")
	}
	source := normalizeReportHumanInputSource(req.Source)
	priority := normalizeReportHumanInputPriority(req.Priority)
	options, err := normalizeReportHumanInputOptions(req.Options)
	if err != nil {
		return nil, err
	}
	allowFreeText := req.AllowFreeText || len(options) == 0
	id := strings.TrimSpace(req.InputID)
	if id == "" {
		id = newReportHumanInputID(question)
	}
	id = normalizeReportHumanInputID(id)
	if id == "" {
		return nil, fmt.Errorf("input_id is invalid")
	}

	existing, err := getReportHumanInputByID(ctx, db, normalized, id)
	if err != nil {
		return nil, err
	}
	if existing != nil && existing.Status != "pending" {
		return nil, fmt.Errorf("input_id %q already exists with status %q", id, existing.Status)
	}

	now := time.Now().UTC().Format(time.RFC3339)
	optionsJSON, _ := json.Marshal(options)
	input := &ReportHumanInput{
		ID:            id,
		WorkspacePath: normalized,
		Source:        source,
		Priority:      priority,
		Question:      question,
		Context:       strings.TrimSpace(req.Context),
		AllowFreeText: allowFreeText,
		RunID:         strings.TrimSpace(req.RunID),
		Evidence:      strings.TrimSpace(req.Evidence),
		CreatedBy:     strings.TrimSpace(req.CreatedBy),
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	eventType := "created"
	if existing == nil {
		_, err = tx.ExecContext(ctx, `INSERT INTO report_human_inputs
			(id, workspace_path, source, priority, question, context, options_json, allow_free_text, status, run_id, evidence, created_by, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'pending', ?, ?, ?, ?, ?)`,
			input.ID, input.WorkspacePath, input.Source, input.Priority, input.Question, input.Context, string(optionsJSON), boolToInt(input.AllowFreeText),
			input.RunID, input.Evidence, input.CreatedBy, input.CreatedAt, input.UpdatedAt)
	} else {
		eventType = "refreshed"
		_, err = tx.ExecContext(ctx, `UPDATE report_human_inputs
			SET source=?, priority=?, question=?, context=?, options_json=?, allow_free_text=?, run_id=?, evidence=?, created_by=?, updated_at=?
			WHERE id=? AND workspace_path=? AND status='pending'`,
			input.Source, input.Priority, input.Question, input.Context, string(optionsJSON), boolToInt(input.AllowFreeText),
			input.RunID, input.Evidence, input.CreatedBy, input.UpdatedAt, input.ID, input.WorkspacePath)
		input.CreatedAt = existing.CreatedAt
	}
	if err != nil {
		return nil, err
	}
	if err := writeReportHumanInputEvent(ctx, tx, normalized, reportHumanInputEvent{
		InputID: input.ID, EventType: eventType, Status: "pending", ActorID: input.CreatedBy,
		ActorKind: req.CreatedByKind, Channel: req.CreatedVia, SessionID: req.SessionID, CreatedAt: now,
	}); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return getReportHumanInputByID(ctx, db, normalized, id)
}

func listReportHumanInputs(ctx context.Context, workspacePath, status, source string) ([]ReportHumanInput, error) {
	reportHumanInputStoreMu.Lock()
	defer reportHumanInputStoreMu.Unlock()

	normalized, db, err := openReportHumanInputDB(ctx, workspacePath, false)
	if err != nil {
		return nil, err
	}
	if db == nil {
		return []ReportHumanInput{}, nil
	}
	defer db.Close()

	clauses := []string{"workspace_path = ?"}
	args := []interface{}{normalized}
	if s := strings.TrimSpace(status); s != "" {
		clauses = append(clauses, "status = ?")
		args = append(args, s)
	}
	if s := strings.TrimSpace(source); s != "" {
		clauses = append(clauses, "source = ?")
		args = append(args, normalizeReportHumanInputSource(s))
	}
	query := `SELECT id, workspace_path, source, priority, question, context, options_json, allow_free_text, status,
		selected_option_id, note, run_id, evidence, created_by, answered_by, answered_by_kind, answered_via, answered_session_id, consumed_by, outcome_summary,
		created_at, updated_at, answered_at, consumed_at, dismissed_at, claim_token, claimed_at, claim_expires_at
		FROM report_human_inputs WHERE ` + strings.Join(clauses, " AND ") + `
		ORDER BY CASE status WHEN 'pending' THEN 0 WHEN 'answered' THEN 1 WHEN 'claimed' THEN 2 WHEN 'dismissed' THEN 3 ELSE 4 END,
			datetime(updated_at) DESC, id DESC`
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		if isReportHumanInputsMissingTable(err) {
			return []ReportHumanInput{}, nil
		}
		return nil, err
	}
	defer rows.Close()

	var inputs []ReportHumanInput
	for rows.Next() {
		input, err := scanReportHumanInput(rows)
		if err != nil {
			return nil, err
		}
		inputs = append(inputs, *input)
	}
	return inputs, rows.Err()
}

func answerReportHumanInput(ctx context.Context, workspacePath, inputID string, req ReportHumanInputAnswerRequest) (*ReportHumanInput, error) {
	reportHumanInputStoreMu.Lock()
	defer reportHumanInputStoreMu.Unlock()

	normalized, db, err := openReportHumanInputDB(ctx, workspacePath, false)
	if err != nil {
		return nil, err
	}
	if db == nil {
		return nil, fmt.Errorf("input_id %q not found", inputID)
	}
	defer db.Close()

	input, err := getReportHumanInputByID(ctx, db, normalized, inputID)
	if err != nil {
		return nil, err
	}
	if input == nil {
		return nil, fmt.Errorf("input_id %q not found", inputID)
	}
	if input.Status == "consumed" || input.Status == "dismissed" || input.Status == "claimed" {
		return nil, fmt.Errorf("input_id %q is %s", inputID, input.Status)
	}

	selected := strings.TrimSpace(req.SelectedOptionID)
	note := strings.TrimSpace(req.Note)
	if selected != "" && !reportHumanInputOptionExists(input.Options, selected) {
		return nil, fmt.Errorf("selected_option_id %q is not valid for input_id %q", selected, inputID)
	}
	if !input.AllowFreeText && len(input.Options) > 0 {
		note = ""
	}
	if selected == "" && note == "" {
		if len(input.Options) > 0 {
			if input.AllowFreeText {
				return nil, fmt.Errorf("select an option or provide a note")
			}
			return nil, fmt.Errorf("selected_option_id is required")
		}
		return nil, fmt.Errorf("note is required for free-text input")
	}

	now := time.Now().UTC().Format(time.RFC3339)
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	// The WHERE clause re-checks status inside the same statement that writes
	// it (PLAT-073 cluster I, cf457bdd/7602e2ac): the in-process mutex above
	// only serializes goroutines in this server — a concurrent writer in
	// another process (the documented chat/schedule concurrency contract) can
	// still consume or dismiss this row between the read above and this
	// write. Without this guard that write silently reverts status back to
	// 'answered' while leaving the prior consumed_at/outcome_summary in
	// place, producing a row that is simultaneously "answered" and
	// "consumed" — exactly the state loop_closure observed live.
	result, err := tx.ExecContext(ctx, `UPDATE report_human_inputs
		SET status='answered', selected_option_id=?, note=?, answered_by=?, answered_by_kind=?, answered_via=?, answered_session_id=?, answered_at=?, updated_at=?
		WHERE id=? AND workspace_path=? AND status NOT IN ('consumed', 'dismissed', 'claimed')`,
		selected, note, strings.TrimSpace(req.AnsweredBy), normalizeReportHumanInputActorKind(req.AnsweredByKind),
		strings.TrimSpace(req.AnsweredVia), strings.TrimSpace(req.SessionID), now, now, input.ID, normalized)
	if err != nil {
		return nil, err
	}
	if affected, err := result.RowsAffected(); err != nil {
		return nil, err
	} else if affected == 0 {
		return nil, fmt.Errorf("input_id %q was consumed, dismissed, or claimed by another writer before this answer could be saved", inputID)
	}
	// The current row already owns the answer. The append-only audit trail keeps
	// provenance, not a second permanent copy of potentially sensitive free text.
	details, _ := json.Marshal(map[string]interface{}{
		"selected_option_id": selected,
		"note_present":       note != "",
	})
	if err := writeReportHumanInputEvent(ctx, tx, normalized, reportHumanInputEvent{
		InputID: input.ID, EventType: "answered", Status: "answered", ActorID: req.AnsweredBy,
		ActorKind: req.AnsweredByKind, Channel: req.AnsweredVia, SessionID: req.SessionID,
		Details: string(details), CreatedAt: now,
	}); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return getReportHumanInputByID(ctx, db, normalized, input.ID)
}

func dismissReportHumanInput(ctx context.Context, workspacePath, inputID string, req ReportHumanInputAnswerRequest) (*ReportHumanInput, error) {
	reportHumanInputStoreMu.Lock()
	defer reportHumanInputStoreMu.Unlock()

	normalized, db, err := openReportHumanInputDB(ctx, workspacePath, false)
	if err != nil {
		return nil, err
	}
	if db == nil {
		return nil, fmt.Errorf("input_id %q not found", inputID)
	}
	defer db.Close()

	input, err := getReportHumanInputByID(ctx, db, normalized, inputID)
	if err != nil {
		return nil, err
	}
	if input == nil {
		return nil, fmt.Errorf("input_id %q not found", inputID)
	}
	if input.Status == "consumed" || input.Status == "claimed" {
		return nil, fmt.Errorf("input_id %q is %s and cannot be dismissed", inputID, input.Status)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	// Same concurrent-writer guard as answerReportHumanInput (PLAT-073 cluster I).
	result, err := tx.ExecContext(ctx, `UPDATE report_human_inputs
		SET status='dismissed', answered_by=?, answered_by_kind=?, answered_via=?, answered_session_id=?, dismissed_at=?, updated_at=?
		WHERE id=? AND workspace_path=? AND status NOT IN ('consumed', 'claimed')`,
		strings.TrimSpace(req.AnsweredBy), normalizeReportHumanInputActorKind(req.AnsweredByKind),
		strings.TrimSpace(req.AnsweredVia), strings.TrimSpace(req.SessionID), now, now, input.ID, normalized)
	if err != nil {
		return nil, err
	}
	if affected, err := result.RowsAffected(); err != nil {
		return nil, err
	} else if affected == 0 {
		return nil, fmt.Errorf("input_id %q was consumed or claimed by another writer before it could be dismissed", inputID)
	}
	if err := writeReportHumanInputEvent(ctx, tx, normalized, reportHumanInputEvent{
		InputID: input.ID, EventType: "dismissed", Status: "dismissed", ActorID: req.AnsweredBy,
		ActorKind: req.AnsweredByKind, Channel: req.AnsweredVia, SessionID: req.SessionID, CreatedAt: now,
	}); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return getReportHumanInputByID(ctx, db, normalized, input.ID)
}

func consumeReportHumanInput(ctx context.Context, workspacePath, inputID string, req ReportHumanInputConsumeRequest) (*ReportHumanInput, error) {
	reportHumanInputStoreMu.Lock()
	defer reportHumanInputStoreMu.Unlock()

	normalized, db, err := openReportHumanInputDB(ctx, workspacePath, false)
	if err != nil {
		return nil, err
	}
	if db == nil {
		return nil, fmt.Errorf("input_id %q not found", inputID)
	}
	defer db.Close()

	input, err := getReportHumanInputByID(ctx, db, normalized, inputID)
	if err != nil {
		return nil, err
	}
	if input == nil {
		return nil, fmt.Errorf("input_id %q not found", inputID)
	}
	if input.Status != "answered" && input.Status != "claimed" {
		return nil, fmt.Errorf("input_id %q must be answered or claimed before it can be consumed; current status=%q", inputID, input.Status)
	}
	outcome := strings.TrimSpace(req.OutcomeSummary)
	if outcome == "" {
		return nil, fmt.Errorf("outcome_summary is required")
	}
	now := time.Now().UTC().Format(time.RFC3339)
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	result, err := tx.ExecContext(ctx, `UPDATE report_human_inputs
		SET status='consumed', consumed_by=?, outcome_summary=?, consumed_at=?, updated_at=?,
		    claim_token='', claimed_at='', claim_expires_at=''
		WHERE id=? AND workspace_path=? AND status IN ('answered', 'claimed')`,
		strings.TrimSpace(req.ConsumedBy), outcome, now, now, input.ID, normalized)
	if err != nil {
		return nil, err
	}
	// The WHERE guard above already existed but its zero-row case was never
	// checked, so a concurrent writer racing this one could make the guard
	// silently no-op while this call still reported success and wrote a
	// "consumed" audit event for a row it never actually changed (PLAT-073
	// cluster I).
	if affected, err := result.RowsAffected(); err != nil {
		return nil, err
	} else if affected == 0 {
		return nil, fmt.Errorf("input_id %q was not in 'answered' or 'claimed' state when this consumption was applied (concurrent writer): current status=%q", inputID, input.Status)
	}
	if err := writeReportHumanInputEvent(ctx, tx, normalized, reportHumanInputEvent{
		InputID: input.ID, EventType: "consumed", Status: "consumed", ActorID: req.ConsumedBy,
		ActorKind: req.ConsumedByKind, Channel: req.ConsumedVia, SessionID: req.SessionID,
		Details: outcome, CreatedAt: now,
	}); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return getReportHumanInputByID(ctx, db, normalized, input.ID)
}

func getReportHumanInputByID(ctx context.Context, db *sql.DB, workspacePath, inputID string) (*ReportHumanInput, error) {
	row := db.QueryRowContext(ctx, `SELECT id, workspace_path, source, priority, question, context, options_json, allow_free_text, status,
		selected_option_id, note, run_id, evidence, created_by, answered_by, answered_by_kind, answered_via, answered_session_id, consumed_by, outcome_summary,
		created_at, updated_at, answered_at, consumed_at, dismissed_at, claim_token, claimed_at, claim_expires_at
		FROM report_human_inputs WHERE workspace_path=? AND id=?`, workspacePath, strings.TrimSpace(inputID))
	input, err := scanReportHumanInput(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if isReportHumanInputsMissingTable(err) {
		return nil, nil
	}
	return input, err
}

type reportHumanInputScanner interface {
	Scan(dest ...interface{}) error
}

func scanReportHumanInput(row reportHumanInputScanner) (*ReportHumanInput, error) {
	var input ReportHumanInput
	var optionsJSON string
	var allowFreeText int
	if err := row.Scan(
		&input.ID, &input.WorkspacePath, &input.Source, &input.Priority, &input.Question, &input.Context,
		&optionsJSON, &allowFreeText, &input.Status, &input.SelectedOptionID, &input.Note, &input.RunID,
		&input.Evidence, &input.CreatedBy, &input.AnsweredBy, &input.AnsweredByKind, &input.AnsweredVia, &input.AnsweredSessionID, &input.ConsumedBy, &input.OutcomeSummary,
		&input.CreatedAt, &input.UpdatedAt, &input.AnsweredAt, &input.ConsumedAt, &input.DismissedAt,
		&input.ClaimToken, &input.ClaimedAt, &input.ClaimExpiresAt,
	); err != nil {
		return nil, err
	}
	_ = json.Unmarshal([]byte(optionsJSON), &input.Options)
	if input.Options == nil {
		input.Options = []ReportHumanInputOption{}
	}
	input.AllowFreeText = allowFreeText != 0
	return &input, nil
}

func createReportHumanInputTools() ([]llmtypes.Tool, map[string]interface{}, map[string]string) {
	createTool := llmtypes.Tool{
		Type: "function",
		Function: &llmtypes.FunctionDefinition{
			Name:        "create_human_input_request",
			Description: "Create or refresh a structured non-blocking question for the user. Pulse, Strategy Auditor, and Goal Advisor store workflow questions in that workflow's db/db.sqlite; Chief of Staff may use workspace_path=\"pulse\" for an org-wide question, stored in pulse/db/db.sqlite, or a Workflow/<name> path for a workflow-specific question. The user answers inside Runloop's Pulse/report panel; published static reports should only show the question and tell the user to open Runloop to answer. For an in-strategy business proposal use source=\"strategy_auditor\" and a stable input_id prefixed with \"strategy-proposal-\". For a materially different plan proposal use source=\"goal_advisor\" and a stable input_id prefixed with \"plan-proposal-\". Use approve/reject/defer options and put the exact proposed changes, rationale, expected impact, risk, and evidence in context so a later Pulse pass can apply an approved proposal with normal plan tools.",
			Parameters: llmtypes.NewParameters(map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"workspace_path": map[string]interface{}{"type": "string", "description": "Workflow-relative path, for example Workflow/social-media. Chief of Staff may use pulse for an org-wide question. Required; requests are stored in that scope's db/db.sqlite."},
					"input_id":       map[string]interface{}{"type": "string", "description": "Optional stable id. Reuse this for the same still-open question so Pulse refreshes it instead of duplicating it."},
					"source":         map[string]interface{}{"type": "string", "enum": []string{"pulse", "strategy_auditor", "goal_advisor", "chief_of_staff"}, "description": "Who is asking. Defaults to pulse."},
					"priority":       map[string]interface{}{"type": "string", "enum": []string{"low", "medium", "high"}, "description": "How important the answer is. Defaults to medium."},
					"question":       map[string]interface{}{"type": "string", "description": "The exact user-facing question in ONE short plain sentence -- the kind a busy operator reads in three seconds, not an analyst's framing of the problem."},
					"context":        map[string]interface{}{"type": "string", "description": "Short explanation of why this matters and what will happen next, for a non-technical operator, not a technical report. One to three short sentences PER SECTION, plain language, no jargon, no walked-through derivation -- state the single number or fact that matters and the conclusion, not how you got there; the full analysis belongs in the reviewer's findings file, not this question. For plan-change proposals, use newline-separated labeled sections exactly like: Proposal:\n...\nExact intended edits if approved:\n(1) ...\n(2) ...\nRationale:\n...\nExpected impact:\n...\nRisk:\n... -- each section still capped at one to three short sentences. Keep evidence paths in the separate evidence field, never inline citations or file paths in context."},
					"options": map[string]interface{}{
						"type": "array",
						"items": map[string]interface{}{
							"type": "object",
							"properties": map[string]interface{}{
								"id":          map[string]interface{}{"type": "string", "description": "Stable machine id, e.g. approve, wait, use_email."},
								"title":       map[string]interface{}{"type": "string", "description": "Short option label shown to the user."},
								"description": map[string]interface{}{"type": "string", "description": "One short sentence explaining the tradeoff."},
							},
							"required": []string{"id", "title"},
						},
						"description": "Optional choice list. Each option needs an id, title, and ideally a short description.",
					},
					"allow_free_text": map[string]interface{}{"type": "boolean", "description": "Allow the user to write a custom answer instead of selecting an option, or add a note alongside an option. If no options are provided, free text is automatically allowed."},
					"run_id":          map[string]interface{}{"type": "string", "description": "Optional schedule/run id connected to the request."},
					"evidence":        map[string]interface{}{"type": "string", "description": "Evidence paths/ids that justify the question."},
				},
				"required": []string{"workspace_path", "question"},
			}),
		},
	}
	getTool := llmtypes.Tool{
		Type: "function",
		Function: &llmtypes.FunctionDefinition{
			Name:        "get_human_input_request",
			Description: "Read one existing Pulse/report decision by its exact workflow path and input id. Use this before applying an answered decision so its context, selected option, evidence, and lifecycle status are read from the canonical typed record rather than inferred from a summary or queried directly from SQLite.",
			Parameters: llmtypes.NewParameters(map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"workspace_path": map[string]interface{}{"type": "string", "description": "Exact workflow-relative path, for example Workflow/rtslatency, or pulse for an org-wide Chief of Staff decision."},
					"input_id":       map[string]interface{}{"type": "string", "description": "Exact decision id supplied by the scheduler, Pulse state, or chat context."},
				},
				"required": []string{"workspace_path", "input_id"},
			}),
		},
	}
	consumeTool := llmtypes.Tool{
		Type: "function",
		Function: &llmtypes.FunctionDefinition{
			Name:        "mark_human_input_consumed",
			Description: "Mark an answered report human input as consumed after Pulse/Goal Advisor/Chief of Staff has used the answer and recorded the outcome. This keeps history but removes it from the pending-for-agent queue.",
			Parameters: llmtypes.NewParameters(map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"workspace_path":  map[string]interface{}{"type": "string", "description": "Workflow-relative path, for example Workflow/social-media."},
					"input_id":        map[string]interface{}{"type": "string", "description": "The input id returned by create_human_input_request."},
					"outcome_summary": map[string]interface{}{"type": "string", "description": "What you did with the answer, in one sentence."},
					"consumed_by":     map[string]interface{}{"type": "string", "description": "Optional actor label. Defaults to agent."},
				},
				"required": []string{"workspace_path", "input_id", "outcome_summary"},
			}),
		},
	}
	answerTool := llmtypes.Tool{
		Type: "function",
		Function: &llmtypes.FunctionDefinition{
			Name:        "answer_human_input_request",
			Description: "Record the current user's explicit answer to an existing pending Pulse/report decision. Call this only after the user clearly selects an option or gives a final free-text answer; never infer an answer from discussion. This changes the request to answered so a later Pulse, Goal Advisor, or Chief of Staff run can apply it. It does not apply the decision or mark it consumed.",
			Parameters: llmtypes.NewParameters(map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"workspace_path":     map[string]interface{}{"type": "string", "description": "Workflow-relative path, for example Workflow/social-media, or pulse for an org-wide Chief of Staff decision."},
					"input_id":           map[string]interface{}{"type": "string", "description": "Exact decision id supplied in the chat context."},
					"selected_option_id": map[string]interface{}{"type": "string", "description": "Exact option id supplied in the chat context when the user selects an option."},
					"note":               map[string]interface{}{"type": "string", "description": "The user's final free-text answer or optional note. Free text is accepted only when the request allows it."},
				},
				"required": []string{"workspace_path", "input_id"},
			}),
		},
	}
	executors := map[string]interface{}{
		"get_human_input_request": func(ctx context.Context, args map[string]interface{}) (string, error) {
			workspacePath, _ := args["workspace_path"].(string)
			inputID, _ := args["input_id"].(string)
			if strings.TrimSpace(workspacePath) == "" || strings.TrimSpace(inputID) == "" {
				return "", fmt.Errorf("workspace_path and input_id are required")
			}
			normalized, db, err := openReportHumanInputDB(ctx, workspacePath, false)
			if err != nil {
				return "", err
			}
			if db == nil {
				return "", fmt.Errorf("human input %q was not found in %s", strings.TrimSpace(inputID), strings.TrimSpace(workspacePath))
			}
			defer db.Close()
			input, err := getReportHumanInputByID(ctx, db, normalized, inputID)
			if err != nil {
				return "", err
			}
			if input == nil {
				return "", fmt.Errorf("human input %q was not found in %s", strings.TrimSpace(inputID), normalized)
			}
			return marshalReportHumanInputToolResult("read", input)
		},
		"create_human_input_request": func(ctx context.Context, args map[string]interface{}) (string, error) {
			req, err := reportHumanInputCreateRequestFromToolArgs(args)
			if err != nil {
				return "", err
			}
			if req.CreatedBy == "" {
				req.CreatedBy = "agent"
			}
			req.CreatedByKind = "agent"
			req.CreatedVia = "agent_tool"
			req.SessionID = mcpexecutor.SessionIDFromContext(ctx)
			input, err := createReportHumanInput(ctx, req.WorkspacePath, req)
			if err != nil {
				return "", err
			}
			return marshalReportHumanInputToolResult("created", input)
		},
		"answer_human_input_request": func(ctx context.Context, args map[string]interface{}) (string, error) {
			workspacePath, _ := args["workspace_path"].(string)
			inputID, _ := args["input_id"].(string)
			req := ReportHumanInputAnswerRequest{}
			req.SelectedOptionID, _ = args["selected_option_id"].(string)
			req.Note, _ = args["note"].(string)
			req.AnsweredBy = GetUserIDFromContext(ctx)
			req.AnsweredByKind = "human_via_chat"
			req.AnsweredVia = "agent_chat"
			req.SessionID = mcpexecutor.SessionIDFromContext(ctx)
			input, err := answerReportHumanInput(ctx, workspacePath, inputID, req)
			if err != nil {
				return "", err
			}
			return marshalReportHumanInputToolResult("answered", input)
		},
		"mark_human_input_consumed": func(ctx context.Context, args map[string]interface{}) (string, error) {
			workspacePath, _ := args["workspace_path"].(string)
			inputID, _ := args["input_id"].(string)
			req := ReportHumanInputConsumeRequest{}
			req.OutcomeSummary, _ = args["outcome_summary"].(string)
			req.ConsumedBy, _ = args["consumed_by"].(string)
			if req.ConsumedBy == "" {
				req.ConsumedBy = "agent"
			}
			req.ConsumedByKind = "agent"
			req.ConsumedVia = "agent_tool"
			req.SessionID = mcpexecutor.SessionIDFromContext(ctx)
			input, err := consumeReportHumanInput(ctx, workspacePath, inputID, req)
			if err != nil {
				return "", err
			}
			return marshalReportHumanInputToolResult("consumed", input)
		},
	}
	categories := map[string]string{
		"get_human_input_request":    "human_tools",
		"create_human_input_request": "human_tools",
		"answer_human_input_request": "human_tools",
		"mark_human_input_consumed":  "human_tools",
	}
	return []llmtypes.Tool{getTool, createTool, answerTool, consumeTool}, executors, categories
}

func reportHumanInputCreateRequestFromToolArgs(args map[string]interface{}) (ReportHumanInputCreateRequest, error) {
	var req ReportHumanInputCreateRequest
	req.WorkspacePath, _ = args["workspace_path"].(string)
	req.InputID, _ = args["input_id"].(string)
	req.Source, _ = args["source"].(string)
	req.Priority, _ = args["priority"].(string)
	req.Question, _ = args["question"].(string)
	req.Context, _ = args["context"].(string)
	req.RunID, _ = args["run_id"].(string)
	req.Evidence, _ = args["evidence"].(string)
	req.CreatedBy, _ = args["created_by"].(string)
	req.AllowFreeText, _ = args["allow_free_text"].(bool)
	if raw, ok := args["options"]; ok {
		b, _ := json.Marshal(raw)
		if err := json.Unmarshal(b, &req.Options); err != nil {
			return req, fmt.Errorf("options must be an array of {id,title,description}")
		}
	}
	return req, nil
}

func marshalReportHumanInputToolResult(status string, input *ReportHumanInput) (string, error) {
	payload := map[string]interface{}{
		"status": status,
		"input":  input,
		"note":   "Stored in the workflow-local db/db.sqlite report_human_inputs table.",
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func (api *StreamingAPI) handleListReportHumanInputs(w http.ResponseWriter, r *http.Request) {
	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}
	workspacePath := r.URL.Query().Get("workspace_path")
	inputs, err := listReportHumanInputs(r.Context(), workspacePath, r.URL.Query().Get("status"), r.URL.Query().Get("source"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "inputs": inputs})
}

func (api *StreamingAPI) handleListReportHumanInputsAggregate(w http.ResponseWriter, r *http.Request) {
	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}
	workspacePaths := r.URL.Query()["workspace_path"]
	if len(workspacePaths) == 0 {
		workspacePaths = strings.Split(r.URL.Query().Get("workspace_paths"), ",")
	}
	seen := make(map[string]struct{}, len(workspacePaths))
	inputs := make([]ReportHumanInput, 0)
	for _, workspacePath := range workspacePaths {
		workspacePath = strings.TrimSpace(workspacePath)
		if workspacePath == "" {
			continue
		}
		normalized, err := normalizeReportHumanInputWorkspacePath(workspacePath)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if _, exists := seen[normalized]; exists {
			continue
		}
		seen[normalized] = struct{}{}
		workspaceInputs, err := listReportHumanInputs(r.Context(), normalized, r.URL.Query().Get("status"), r.URL.Query().Get("source"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		inputs = append(inputs, workspaceInputs...)
	}
	sort.SliceStable(inputs, func(i, j int) bool {
		return inputs[i].UpdatedAt > inputs[j].UpdatedAt
	})
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "inputs": inputs})
}

func (api *StreamingAPI) handleCreateReportHumanInput(w http.ResponseWriter, r *http.Request) {
	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}
	var req ReportHumanInputCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("Invalid request body: %v", err), http.StatusBadRequest)
		return
	}
	// Actor identity is server-derived. A request body must not be able to forge
	// who created or answered an operator-facing decision.
	req.CreatedBy = GetUserIDFromContext(r.Context())
	req.CreatedByKind = "human_ui"
	req.CreatedVia = "report_ui"
	req.SessionID = reportHumanInputRequestSessionID(r)
	input, err := createReportHumanInput(r.Context(), req.WorkspacePath, req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "input": input})
}

func (api *StreamingAPI) handleAnswerReportHumanInput(w http.ResponseWriter, r *http.Request) {
	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}
	var req ReportHumanInputAnswerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("Invalid request body: %v", err), http.StatusBadRequest)
		return
	}
	if req.WorkspacePath == "" {
		req.WorkspacePath = r.URL.Query().Get("workspace_path")
	}
	req.AnsweredBy = GetUserIDFromContext(r.Context())
	req.AnsweredByKind = "human_ui"
	req.AnsweredVia = "report_ui"
	req.SessionID = reportHumanInputRequestSessionID(r)
	input, err := answerReportHumanInput(r.Context(), req.WorkspacePath, mux.Vars(r)["input_id"], req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "input": input})
}

func (api *StreamingAPI) handleDismissReportHumanInput(w http.ResponseWriter, r *http.Request) {
	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}
	var body struct {
		WorkspacePath string `json:"workspace_path"`
		AnsweredBy    string `json:"answered_by"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	if body.WorkspacePath == "" {
		body.WorkspacePath = r.URL.Query().Get("workspace_path")
	}
	req := ReportHumanInputAnswerRequest{
		WorkspacePath:  body.WorkspacePath,
		AnsweredBy:     GetUserIDFromContext(r.Context()),
		AnsweredByKind: "human_ui",
		AnsweredVia:    "report_ui",
		SessionID:      reportHumanInputRequestSessionID(r),
	}
	input, err := dismissReportHumanInput(r.Context(), req.WorkspacePath, mux.Vars(r)["input_id"], req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "input": input})
}

func (api *StreamingAPI) handleConsumeReportHumanInput(w http.ResponseWriter, r *http.Request) {
	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}
	var req ReportHumanInputConsumeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("Invalid request body: %v", err), http.StatusBadRequest)
		return
	}
	if req.WorkspacePath == "" {
		req.WorkspacePath = r.URL.Query().Get("workspace_path")
	}
	req.ConsumedBy = GetUserIDFromContext(r.Context())
	req.ConsumedByKind = "human_ui"
	req.ConsumedVia = "report_ui"
	req.SessionID = reportHumanInputRequestSessionID(r)
	input, err := consumeReportHumanInput(r.Context(), req.WorkspacePath, mux.Vars(r)["input_id"], req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "input": input})
}

func reportHumanInputRequestSessionID(r *http.Request) string {
	if r == nil {
		return ""
	}
	for _, header := range []string{"X-AgentWorks-Session-ID", "X-Session-ID", "X-Request-ID"} {
		if value := strings.TrimSpace(r.Header.Get(header)); value != "" {
			return value
		}
	}
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err == nil {
		return "http-request-" + hex.EncodeToString(buf)
	}
	return fmt.Sprintf("http-request-%d", time.Now().UTC().UnixNano())
}

func formatAnsweredReportHumanInputsForAgent(ctx context.Context, workspacePath string) string {
	inputs, err := listReportHumanInputs(ctx, workspacePath, "answered", "")
	if err != nil || len(inputs) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("Answered human input requests waiting for this workflow pass:\n")
	for _, input := range inputs {
		answer := reportHumanInputAnswerForAgent(input)
		context := strings.TrimSpace(input.Context)
		if context != "" {
			context = " context=" + strconv.Quote(context)
		}
		b.WriteString(fmt.Sprintf("- input_id=%s source=%s priority=%s question=%q answer=%q answered_by=%q actor_kind=%s via=%s answered_at=%s evidence=%q%s\n",
			input.ID, input.Source, input.Priority, input.Question, answer, input.AnsweredBy, input.AnsweredByKind, input.AnsweredVia, input.AnsweredAt, input.Evidence, context))
	}
	b.WriteString("If an answered Goal Advisor plan proposal is approved, apply it only with normal plan modification/config/eval/report tools, then call mark_human_input_consumed with the concrete outcome. If it is rejected or deferred, record that outcome and consume it. The Pulse popup derives pending questions from typed state, so do not edit a presentation artifact or the SQLite table directly.\n")
	return strings.TrimSpace(b.String())
}

// formatAnsweredChiefOfStaffInputsForAgent gathers only Chief of Staff answers
// across the org-level pulse scope and discovered workflow scopes. Unlike a
// workflow Pulse pass, Chief of Staff has no single workspace DB, so each line
// carries the workspace_path required to record the outcome in the same scope.
func formatChiefOfStaffInputsForAgent(inputs []ReportHumanInput) string {
	if len(inputs) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("Answered Chief of Staff questions waiting for this run:\n")
	for _, input := range inputs {
		context := strings.TrimSpace(input.Context)
		if context != "" {
			context = " context=" + strconv.Quote(context)
		}
		b.WriteString(fmt.Sprintf("- workspace_path=%s input_id=%s priority=%s question=%q answer=%q answered_by=%q actor_kind=%s via=%s answered_at=%s evidence=%q%s\n",
			input.WorkspacePath, input.ID, input.Priority, input.Question, reportHumanInputAnswerForAgent(input), input.AnsweredBy, input.AnsweredByKind, input.AnsweredVia, input.AnsweredAt, input.Evidence, context))
	}
	b.WriteString("Use each answer when it is still relevant. Do not mark an answer consumed merely because you read it. After the requested action or a concrete no-action/deferred/stale decision is complete, call mark_human_input_consumed with the same workspace_path and input_id plus a truthful outcome_summary. If the action cannot be completed safely in this run, leave the answer unconsumed so a later Chief of Staff or workflow Pulse pass can handle it.\n")
	return strings.TrimSpace(b.String())
}

func claimAnsweredChiefOfStaffInputsForAgent(ctx context.Context, workspacePaths []string, claimToken string) string {
	claimToken = strings.TrimSpace(claimToken)
	if claimToken == "" {
		return ""
	}
	seen := make(map[string]struct{}, len(workspacePaths))
	claimed := make([]ReportHumanInput, 0)
	for _, workspacePath := range workspacePaths {
		workspacePath = strings.TrimSpace(workspacePath)
		if workspacePath == "" {
			continue
		}
		if _, exists := seen[workspacePath]; exists {
			continue
		}
		seen[workspacePath] = struct{}{}
		_ = releaseExpiredChiefOfStaffInputClaims(ctx, workspacePath)
		inputs, err := listReportHumanInputs(ctx, workspacePath, "answered", "chief_of_staff")
		if err != nil {
			continue
		}
		for _, input := range inputs {
			claimedInput, ok := claimReportHumanInput(ctx, workspacePath, input.ID, claimToken)
			if ok {
				claimed = append(claimed, *claimedInput)
			}
		}
	}
	return formatChiefOfStaffInputsForAgent(claimed)
}

func claimReportHumanInput(ctx context.Context, workspacePath, inputID, claimToken string) (*ReportHumanInput, bool) {
	reportHumanInputStoreMu.Lock()
	defer reportHumanInputStoreMu.Unlock()

	normalized, db, err := openReportHumanInputDB(ctx, workspacePath, false)
	if err != nil || db == nil {
		return nil, false
	}
	defer db.Close()
	now := time.Now().UTC()
	expiresAt := now.Add(6 * time.Hour)
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, false
	}
	defer func() { _ = tx.Rollback() }()
	result, err := tx.ExecContext(ctx, `UPDATE report_human_inputs
		SET status='claimed', claim_token=?, claimed_at=?, claim_expires_at=?, updated_at=?
		WHERE id=? AND workspace_path=? AND source='chief_of_staff' AND status='answered'`,
		claimToken, now.Format(time.RFC3339), expiresAt.Format(time.RFC3339), now.Format(time.RFC3339),
		strings.TrimSpace(inputID), normalized)
	if err != nil {
		return nil, false
	}
	affected, _ := result.RowsAffected()
	if affected != 1 {
		return nil, false
	}
	if err := writeReportHumanInputEvent(ctx, tx, normalized, reportHumanInputEvent{
		InputID: inputID, EventType: "claimed", Status: "claimed", ActorID: claimToken,
		ActorKind: "agent", Channel: "chief_of_staff_schedule", SessionID: claimToken,
		Details: "answer leased for one Chief of Staff run", CreatedAt: now.Format(time.RFC3339),
	}); err != nil {
		return nil, false
	}
	if err := tx.Commit(); err != nil {
		return nil, false
	}
	input, err := getReportHumanInputByID(ctx, db, normalized, inputID)
	return input, err == nil && input != nil
}

func releaseExpiredChiefOfStaffInputClaims(ctx context.Context, workspacePath string) error {
	reportHumanInputStoreMu.Lock()
	defer reportHumanInputStoreMu.Unlock()
	normalized, db, err := openReportHumanInputDB(ctx, workspacePath, false)
	if err != nil || db == nil {
		return err
	}
	defer db.Close()
	now := time.Now().UTC().Format(time.RFC3339)
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	ids, err := reportHumanInputIDs(ctx, tx, `SELECT id FROM report_human_inputs
		WHERE workspace_path=? AND status='claimed' AND claim_expires_at<>'' AND claim_expires_at<?`, normalized, now)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `UPDATE report_human_inputs
		SET status='answered', claim_token='', claimed_at='', claim_expires_at='', updated_at=?
		WHERE workspace_path=? AND status='claimed' AND claim_expires_at<>'' AND claim_expires_at<?`, now, normalized, now)
	if err != nil {
		return err
	}
	for _, inputID := range ids {
		if err := writeReportHumanInputEvent(ctx, tx, normalized, reportHumanInputEvent{
			InputID: inputID, EventType: "claim_released", Status: "answered", ActorID: "scheduler",
			ActorKind: "system", Channel: "claim_expiry", Details: "expired claim released", CreatedAt: now,
		}); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func releaseChiefOfStaffInputClaims(ctx context.Context, workspacePaths []string, claimToken string) {
	claimToken = strings.TrimSpace(claimToken)
	if claimToken == "" {
		return
	}
	seen := make(map[string]struct{}, len(workspacePaths))
	for _, workspacePath := range workspacePaths {
		workspacePath = strings.TrimSpace(workspacePath)
		if workspacePath == "" {
			continue
		}
		if _, exists := seen[workspacePath]; exists {
			continue
		}
		seen[workspacePath] = struct{}{}
		reportHumanInputStoreMu.Lock()
		normalized, db, err := openReportHumanInputDB(ctx, workspacePath, false)
		if err == nil && db != nil {
			now := time.Now().UTC().Format(time.RFC3339)
			tx, txErr := db.BeginTx(ctx, nil)
			if txErr == nil {
				ids, idsErr := reportHumanInputIDs(ctx, tx, `SELECT id FROM report_human_inputs
					WHERE workspace_path=? AND status='claimed' AND claim_token=?`, normalized, claimToken)
				if idsErr != nil {
					txErr = idsErr
				} else {
					_, txErr = tx.ExecContext(ctx, `UPDATE report_human_inputs
				SET status='answered', claim_token='', claimed_at='', claim_expires_at='', updated_at=?
				WHERE workspace_path=? AND status='claimed' AND claim_token=?`, now, normalized, claimToken)
					if txErr == nil {
						for _, inputID := range ids {
							txErr = writeReportHumanInputEvent(ctx, tx, normalized, reportHumanInputEvent{
								InputID: inputID, EventType: "claim_released", Status: "answered", ActorID: claimToken,
								ActorKind: "agent", Channel: "chief_of_staff_schedule", SessionID: claimToken,
								Details: "unconsumed answer released for a later run", CreatedAt: now,
							})
							if txErr != nil {
								break
							}
						}
					}
				}
				if txErr == nil {
					txErr = tx.Commit()
				}
				if txErr != nil {
					_ = tx.Rollback()
				}
			}
			_ = db.Close()
		}
		reportHumanInputStoreMu.Unlock()
	}
}

type reportHumanInputRowsQueryer interface {
	QueryContext(context.Context, string, ...interface{}) (*sql.Rows, error)
}

func reportHumanInputIDs(ctx context.Context, queryer reportHumanInputRowsQueryer, query string, args ...interface{}) ([]string, error) {
	rows, err := queryer.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func reportHumanInputAnswerForAgent(input ReportHumanInput) string {
	answer := input.Note
	if input.SelectedOptionID == "" {
		return answer
	}
	answer = fmt.Sprintf("option=%s", input.SelectedOptionID)
	if title := reportHumanInputOptionTitle(input.Options, input.SelectedOptionID); title != "" {
		answer += fmt.Sprintf(" (%s)", title)
	}
	if input.Note != "" {
		answer += "; note=" + input.Note
	}
	return answer
}

func normalizeReportHumanInputSource(source string) string {
	switch strings.ToLower(strings.TrimSpace(source)) {
	case "strategy_auditor", "strategy-auditor", "strategy auditor":
		return "strategy_auditor"
	case "goal_advisor", "goal-advisor", "goal advisor":
		return "goal_advisor"
	case "chief_of_staff", "chief-of-staff", "chief", "org_pulse", "org-pulse":
		return "chief_of_staff"
	default:
		return "pulse"
	}
}

func normalizeReportHumanInputPriority(priority string) string {
	switch strings.ToLower(strings.TrimSpace(priority)) {
	case "low", "high":
		return strings.ToLower(strings.TrimSpace(priority))
	default:
		return "medium"
	}
}

func normalizeReportHumanInputOptions(options []ReportHumanInputOption) ([]ReportHumanInputOption, error) {
	if len(options) == 0 {
		return []ReportHumanInputOption{}, nil
	}
	out := make([]ReportHumanInputOption, 0, len(options))
	seen := map[string]bool{}
	for _, option := range options {
		id := normalizeReportHumanInputID(option.ID)
		title := strings.TrimSpace(option.Title)
		if id == "" || title == "" {
			return nil, fmt.Errorf("each option requires id and title")
		}
		if seen[id] {
			return nil, fmt.Errorf("duplicate option id %q", id)
		}
		seen[id] = true
		out = append(out, ReportHumanInputOption{
			ID:          id,
			Title:       title,
			Description: strings.TrimSpace(option.Description),
		})
	}
	return out, nil
}

var reportHumanInputIDRe = regexp.MustCompile(`[^a-zA-Z0-9_-]+`)

func normalizeReportHumanInputID(id string) string {
	cleaned := strings.Trim(reportHumanInputIDRe.ReplaceAllString(strings.TrimSpace(id), "-"), "-_")
	if len(cleaned) > 96 {
		cleaned = cleaned[:96]
	}
	return cleaned
}

func newReportHumanInputID(question string) string {
	slug := strings.ToLower(normalizeReportHumanInputID(question))
	if len(slug) > 40 {
		slug = strings.Trim(slug[:40], "-_")
	}
	if slug == "" {
		slug = "question"
	}
	buf := make([]byte, 4)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("input-%s-%s", time.Now().UTC().Format("20060102-150405"), slug)
	}
	return fmt.Sprintf("input-%s-%s-%s", time.Now().UTC().Format("20060102-150405"), slug, hex.EncodeToString(buf))
}

func reportHumanInputOptionExists(options []ReportHumanInputOption, id string) bool {
	for _, option := range options {
		if option.ID == id {
			return true
		}
	}
	return false
}

func reportHumanInputOptionTitle(options []ReportHumanInputOption, id string) string {
	for _, option := range options {
		if option.ID == id {
			return option.Title
		}
	}
	return ""
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func isReportHumanInputsMissingTable(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "no such table: report_human_inputs")
}
