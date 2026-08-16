package server

import (
	"context"
	"log"
	"strings"
	"time"

	orchEvents "github.com/manishiitg/coding-agent-loop/agent_go/pkg/orchestrator/events"
)

// PLAT-114. Every background agent (Pulse's Gate/reviewers/Fixer, and a
// workflow's own todo-task sub-agents) previously had exactly two records of
// what it did: the typed pulse_review_log receipt, which a module's own Fixer
// turn may simply never write, and the chat session's ui_events cache, which
// is hard-capped at the last 200 UI events and gets silently overwritten once
// a session is reused for a later run. Neither is durable. Workflow steps, by
// contrast, get a real per-run record for free (runs/iteration-N/default/).
// This table gives background agents the equivalent: one durable row per
// agent, upserted on start and again on completion, independent of both.
const backgroundAgentLogSchema = `CREATE TABLE IF NOT EXISTS background_agent_log (
	workspace_path TEXT NOT NULL,
	session_id TEXT NOT NULL,
	agent_id TEXT NOT NULL,
	name TEXT NOT NULL DEFAULT '',
	kind TEXT NOT NULL DEFAULT '',
	parent_execution_id TEXT NOT NULL DEFAULT '',
	status TEXT NOT NULL DEFAULT '',
	result TEXT NOT NULL DEFAULT '',
	error TEXT NOT NULL DEFAULT '',
	duration TEXT NOT NULL DEFAULT '',
	started_at TEXT NOT NULL DEFAULT '',
	completed_at TEXT NOT NULL DEFAULT '',
	updated_at TEXT NOT NULL,
	PRIMARY KEY (workspace_path, session_id, agent_id)
)`

// BackgroundAgentLogEntry is the durable, queryable record of one background
// agent's lifecycle within one workflow-scoped session.
type BackgroundAgentLogEntry struct {
	WorkspacePath     string `json:"workspace_path"`
	SessionID         string `json:"session_id"`
	AgentID           string `json:"agent_id"`
	Name              string `json:"name"`
	Kind              string `json:"kind,omitempty"`
	ParentExecutionID string `json:"parent_execution_id,omitempty"`
	Status            string `json:"status"`
	Result            string `json:"result,omitempty"`
	Error             string `json:"error,omitempty"`
	Duration          string `json:"duration,omitempty"`
	StartedAt         string `json:"started_at,omitempty"`
	CompletedAt       string `json:"completed_at,omitempty"`
	UpdatedAt         string `json:"updated_at"`
}

// backgroundAgentLogWorkspacePath resolves which workflow's database a
// session's background-agent activity belongs to. A session with no
// workspace (plain chat) has nothing to log against — that is intentional,
// not a gap: this table exists to give workflow-scoped runs the durable
// record workflow steps already get, not to log every chat delegation.
func (api *StreamingAPI) backgroundAgentLogWorkspacePath(sessionID string) string {
	if api == nil || strings.TrimSpace(sessionID) == "" {
		return ""
	}
	session, ok := api.getActiveSession(sessionID)
	if !ok || session == nil {
		return ""
	}
	return strings.TrimSpace(session.WorkspacePath)
}

// recordBackgroundAgentLogStarted durably records a background agent
// beginning work. Best-effort by design: a write failure is logged and
// swallowed, never returned to the caller — this is an observability record
// of the agent's execution, not a gate on it. Blocking or failing a
// background agent because its own audit log couldn't be written would be a
// strictly worse failure mode than the one this table exists to fix.
func (api *StreamingAPI) recordBackgroundAgentLogStarted(sessionID, agentID, name string, kind orchEvents.ExecutionKind, parentExecutionID string, startedAt time.Time) {
	workspacePath := api.backgroundAgentLogWorkspacePath(sessionID)
	if workspacePath == "" {
		return
	}
	ctx := context.Background()
	_, db, err := openPulseModuleStateDB(ctx, workspacePath, true)
	if err != nil || db == nil {
		if err != nil {
			log.Printf("[BG_AGENT_LOG] failed to open db for %s: %v", workspacePath, err)
		}
		return
	}
	defer db.Close()
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := db.ExecContext(ctx, `INSERT INTO background_agent_log
		(workspace_path, session_id, agent_id, name, kind, parent_execution_id, status, started_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, 'running', ?, ?)
		ON CONFLICT(workspace_path, session_id, agent_id) DO UPDATE SET
			name=excluded.name, kind=excluded.kind, parent_execution_id=excluded.parent_execution_id,
			status='running', started_at=excluded.started_at, updated_at=excluded.updated_at`,
		workspacePath, sessionID, agentID, name, string(kind), parentExecutionID, startedAt.UTC().Format(time.RFC3339), now,
	); err != nil {
		log.Printf("[BG_AGENT_LOG] failed to record start for session=%s agent=%s: %v", sessionID, agentID, err)
	}
}

// recordBackgroundAgentLogCompleted durably records a background agent's
// outcome. Upserts the same row recordBackgroundAgentLogStarted created, so a
// row that only ever reaches "running" is itself a real signal: that agent
// started and this record never saw it finish.
func (api *StreamingAPI) recordBackgroundAgentLogCompleted(sessionID, agentID, name, kind, status, result, errMsg, duration string, completedAt time.Time) {
	workspacePath := api.backgroundAgentLogWorkspacePath(sessionID)
	if workspacePath == "" {
		return
	}
	ctx := context.Background()
	_, db, err := openPulseModuleStateDB(ctx, workspacePath, true)
	if err != nil || db == nil {
		if err != nil {
			log.Printf("[BG_AGENT_LOG] failed to open db for %s: %v", workspacePath, err)
		}
		return
	}
	defer db.Close()
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := db.ExecContext(ctx, `INSERT INTO background_agent_log
		(workspace_path, session_id, agent_id, name, kind, status, result, error, duration, completed_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(workspace_path, session_id, agent_id) DO UPDATE SET
			name=excluded.name, status=excluded.status, result=excluded.result, error=excluded.error,
			duration=excluded.duration, completed_at=excluded.completed_at, updated_at=excluded.updated_at`,
		workspacePath, sessionID, agentID, name, kind, status, result, errMsg, duration, completedAt.UTC().Format(time.RFC3339), now,
	); err != nil {
		log.Printf("[BG_AGENT_LOG] failed to record completion for session=%s agent=%s: %v", sessionID, agentID, err)
	}
}

// backgroundAgentLogForSession reads every background agent durably recorded
// for one session, oldest first. Used to answer "what did this Pulse run's
// (or workflow run's) background agents actually do" without depending on
// pulse_review_log (which a module's Fixer turn may never write) or
// ui_events (capped at 200, overwritten on session reuse).
func backgroundAgentLogForSession(ctx context.Context, workspacePath, sessionID string) ([]BackgroundAgentLogEntry, error) {
	_, db, err := openPulseModuleStateDB(ctx, workspacePath, false)
	if err != nil {
		return nil, err
	}
	if db == nil {
		return nil, nil
	}
	defer db.Close()

	rows, err := db.QueryContext(ctx, `SELECT workspace_path, session_id, agent_id, name, kind, parent_execution_id,
		status, result, error, duration, started_at, completed_at, updated_at
		FROM background_agent_log WHERE workspace_path = ? AND session_id = ?
		ORDER BY started_at ASC, updated_at ASC`, workspacePath, sessionID)
	if err != nil {
		if isMissingTableError(err) {
			return nil, nil
		}
		return nil, err
	}
	defer rows.Close()

	var out []BackgroundAgentLogEntry
	for rows.Next() {
		var entry BackgroundAgentLogEntry
		if err := rows.Scan(&entry.WorkspacePath, &entry.SessionID, &entry.AgentID, &entry.Name, &entry.Kind,
			&entry.ParentExecutionID, &entry.Status, &entry.Result, &entry.Error, &entry.Duration,
			&entry.StartedAt, &entry.CompletedAt, &entry.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, entry)
	}
	return out, rows.Err()
}

func isMissingTableError(err error) bool {
	return err != nil && strings.Contains(err.Error(), "no such table")
}
