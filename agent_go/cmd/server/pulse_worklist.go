package server

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/loopclosure"
	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/orchestrator/agents/workflow/step_based_workflow"
	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/pulsemodules"
	"github.com/manishiitg/multi-llm-provider-go/llmtypes"
)

const (
	pulseModuleWorkflowReview = pulsemodules.WorkflowReviewID
	// pulseModuleLLMOpsReview owns cost, timing, tool-call operations,
	// model/tier/catalog review, and plan-design hygiene (step-type fitness,
	// prevalidation fitness, schema/description drift). These checks need one
	// agentic judgment pass over the same runtime and goal evidence.
	pulseModuleLLMOpsReview = pulsemodules.LLMOpsReviewID
	// pulseModuleStrategicReview owns both hidden-mechanism review of the
	// current strategy and conditional discovery of materially different
	// approaches. Those are sequence turns, not independent modules.
	pulseModuleStrategicReview = pulsemodules.StrategicReviewID
)

// Derived from the canonical registry — see pkg/pulsemodules. Do not restate
// the module set here; a hand-maintained second copy is exactly what caused
// the 2026-07-29 desync.
var pulseModuleOrder = pulsemodules.IDs()

var validPulseModules = func() map[string]bool {
	m := make(map[string]bool, len(pulsemodules.All))
	for _, id := range pulsemodules.IDs() {
		m[id] = true
	}
	return m
}()

// pulseModuleList renders the closed module set for rejection messages. A
// rejection that names the bad value but not the set it had to come from
// leaves the caller guessing, which is how the same invalid value gets retried.
func pulseModuleList() string {
	return strings.Join(pulseModuleOrder, ", ")
}

func pulseModuleExample() string {
	if len(pulseModuleOrder) > 0 {
		return pulseModuleOrder[0]
	}
	return pulseModuleWorkflowReview
}

// pulseModuleResultValues is the module-audit result set shared by the accept
// check and its rejection message.
var pulseModuleResultValues = []string{"done", "changed", "blocked", "failed", "skipped"}

// pulseSchedulerModuleResultValues additionally accepts the scheduler-only
// timed_out result, which agents cannot report for themselves.
var pulseSchedulerModuleResultValues = append(append([]string(nil), pulseModuleResultValues...), "timed_out")

const pulseModuleStateSchema = `CREATE TABLE IF NOT EXISTS pulse_module_state (
	workspace_path TEXT NOT NULL,
	module TEXT NOT NULL,
	last_pulse_run_id TEXT NOT NULL DEFAULT '',
	last_checked_at TEXT NOT NULL DEFAULT '',
	last_ran_at TEXT NOT NULL DEFAULT '',
	last_decision TEXT NOT NULL DEFAULT '',
	last_reason TEXT NOT NULL DEFAULT '',
	last_gate_decision TEXT NOT NULL DEFAULT '',
	last_result TEXT NOT NULL DEFAULT '',
	last_result_reason TEXT NOT NULL DEFAULT '',
	next_check_at TEXT NOT NULL DEFAULT '',
	next_check_after_run_id TEXT NOT NULL DEFAULT '',
	cooldown_runs INTEGER NOT NULL DEFAULT 0,
	evidence_json TEXT NOT NULL DEFAULT '[]',
	updated_at TEXT NOT NULL,
	PRIMARY KEY (workspace_path, module)
)`

const pulseModuleAuditSchema = `CREATE TABLE IF NOT EXISTS pulse_module_audit (
	workspace_path TEXT NOT NULL,
	module TEXT NOT NULL,
	pulse_run_id TEXT NOT NULL,
	result TEXT NOT NULL,
	reason TEXT NOT NULL,
	evidence_json TEXT NOT NULL DEFAULT '[]',
	changed_files_json TEXT NOT NULL DEFAULT '[]',
	verification_json TEXT NOT NULL DEFAULT '[]',
	before_refs_json TEXT NOT NULL DEFAULT '[]',
	after_refs_json TEXT NOT NULL DEFAULT '[]',
	recorded_at TEXT NOT NULL,
	PRIMARY KEY (workspace_path, module, pulse_run_id)
)`

// pulseRunModeSchema stores the Gate's agent-selected shape for one Pulse pass.
// It is deliberately separate from per-module cadence: mode explains *how* the
// selected work should run, while module rows explain *which* perspectives are due.
const pulseRunModeSchema = `CREATE TABLE IF NOT EXISTS pulse_run_mode (
	workspace_path TEXT NOT NULL,
	pulse_run_id TEXT NOT NULL,
	mode TEXT NOT NULL,
	reason TEXT NOT NULL,
	recorded_at TEXT NOT NULL,
	PRIMARY KEY (workspace_path, pulse_run_id)
)`

const (
	pulseRunModeBacklogDrain = "backlog_drain"
	pulseRunModeDiscovery    = "discovery"
	pulseRunModeStrategy     = "strategy"
	pulseRunModeObserve      = "observe"
)

var pulseRunModeValues = []string{
	pulseRunModeBacklogDrain,
	pulseRunModeDiscovery,
	pulseRunModeStrategy,
	pulseRunModeObserve,
}

const pulseShadowSignalObservationSchema = `CREATE TABLE IF NOT EXISTS pulse_shadow_signal_observation (
	workspace_path TEXT NOT NULL,
	pulse_run_id TEXT NOT NULL,
	detector TEXT NOT NULL,
	detector_version TEXT NOT NULL,
	observed_at TEXT NOT NULL,
	coverage_status TEXT NOT NULL,
	coverage_reason TEXT NOT NULL DEFAULT '',
	signals_json TEXT NOT NULL DEFAULT '[]',
	gate_decisions_json TEXT NOT NULL DEFAULT '[]',
	recorded_at TEXT NOT NULL,
	PRIMARY KEY (workspace_path, pulse_run_id, detector)
)`

type PulseModuleState struct {
	WorkspacePath       string   `json:"workspace_path"`
	Module              string   `json:"module"`
	LastPulseRunID      string   `json:"last_pulse_run_id,omitempty"`
	LastCheckedAt       string   `json:"last_checked_at,omitempty"`
	LastRanAt           string   `json:"last_ran_at,omitempty"`
	LastDecision        string   `json:"last_decision,omitempty"`
	LastReason          string   `json:"last_reason,omitempty"`
	LastGateDecision    string   `json:"last_gate_decision,omitempty"`
	LastResult          string   `json:"last_result,omitempty"`
	LastResultReason    string   `json:"last_result_reason,omitempty"`
	NextCheckAt         string   `json:"next_check_at,omitempty"`
	NextCheckAfterRunID string   `json:"next_check_after_run_id,omitempty"`
	CooldownRuns        int      `json:"cooldown_runs,omitempty"`
	Evidence            []string `json:"evidence,omitempty"`
	UpdatedAt           string   `json:"updated_at,omitempty"`
}

type PulseWorklistDecision struct {
	Module              string   `json:"module"`
	Due                 bool     `json:"due"`
	Reason              string   `json:"reason"`
	Evidence            []string `json:"evidence"`
	NextCheckAt         string   `json:"next_check_at"`
	NextCheckAfterRunID string   `json:"next_check_after_run_id"`
	CooldownRuns        int      `json:"cooldown_runs"`
}

// PulseRunMode is the durable, human-readable Gate decision for a single pass.
// Go validates the finite vocabulary but never selects a mode.
type PulseRunMode struct {
	WorkspacePath string `json:"workspace_path"`
	PulseRunID    string `json:"pulse_run_id"`
	Mode          string `json:"mode"`
	Reason        string `json:"reason"`
	RecordedAt    string `json:"recorded_at"`
}

type PulseModuleAudit struct {
	WorkspacePath string   `json:"workspace_path"`
	Module        string   `json:"module"`
	PulseRunID    string   `json:"pulse_run_id"`
	Result        string   `json:"result"`
	Reason        string   `json:"reason"`
	Evidence      []string `json:"evidence,omitempty"`
	ChangedFiles  []string `json:"changed_files,omitempty"`
	Verification  []string `json:"verification,omitempty"`
	BeforeRefs    []string `json:"before_refs,omitempty"`
	AfterRefs     []string `json:"after_refs,omitempty"`
	RecordedAt    string   `json:"recorded_at"`
}

type PulseModuleAuditInput struct {
	ChangedFiles []string
	Verification []string
	BeforeRefs   []string
	AfterRefs    []string
}

// PulseShadowSignalObservation is retained experimental evidence. It is
// deliberately not returned by get_pulse_state(view="module"), so the live Gate cannot
// condition its worklist on a shadow detector before Stage B approval.
type PulseShadowSignalObservation struct {
	WorkspacePath   string                  `json:"workspace_path"`
	PulseRunID      string                  `json:"pulse_run_id"`
	Detector        string                  `json:"detector"`
	DetectorVersion string                  `json:"detector_version"`
	ObservedAt      string                  `json:"observed_at"`
	CoverageStatus  string                  `json:"coverage_status"`
	CoverageReason  string                  `json:"coverage_reason,omitempty"`
	Signals         []loopclosure.Finding   `json:"signals"`
	GateDecisions   []PulseWorklistDecision `json:"gate_decisions"`
	RecordedAt      string                  `json:"recorded_at"`
}

func ensurePulseModuleStateSchema(ctx context.Context, db *sql.DB) error {
	if err := migratePulseModuleStateSchema(ctx, db); err != nil {
		return err
	}
	stmts := []string{
		pulseModuleStateSchema,
		pulseModuleAuditSchema,
		pulseRunModeSchema,
		pulseShadowSignalObservationSchema,
		pulseFinalCommandStateSchema,
		backgroundAgentLogSchema,
	}
	for _, stmt := range stmts {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}
	if err := ensurePulseModuleStateColumns(ctx, db); err != nil {
		return err
	}
	if err := migrateMergedStrategicReviewRows(ctx, db); err != nil {
		return err
	}
	stmts = []string{
		`CREATE INDEX IF NOT EXISTS idx_pulse_module_state_run ON pulse_module_state(last_pulse_run_id, last_decision)`,
		`CREATE INDEX IF NOT EXISTS idx_pulse_module_audit_recorded ON pulse_module_audit(workspace_path, recorded_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_pulse_run_mode_recorded ON pulse_run_mode(workspace_path, recorded_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_pulse_shadow_signal_observed ON pulse_shadow_signal_observation(workspace_path, observed_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_background_agent_log_session ON background_agent_log(workspace_path, session_id, started_at)`,
	}
	for _, stmt := range stmts {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}
	return nil
}

// migrateMergedStrategicReviewRows collapses the two retired advisor lanes
// into the one live Strategic Review identity. The newest state wins when an
// old workflow database has both rows; audit history preserves one receipt per
// Pulse run, which is the new module contract.
func migrateMergedStrategicReviewRows(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	legacyStrategy := pulsemodules.LegacyStrategyAuditorID
	legacyGoal := pulsemodules.LegacyGoalAdvisorID
	if _, err := tx.ExecContext(ctx, `INSERT INTO pulse_module_state (
		workspace_path, module, last_pulse_run_id, last_checked_at, last_ran_at,
		last_decision, last_reason, last_gate_decision, last_result, last_result_reason,
		next_check_at, next_check_after_run_id, cooldown_runs, evidence_json, updated_at)
		SELECT workspace_path, ?, last_pulse_run_id, last_checked_at, last_ran_at,
		last_decision, last_reason, last_gate_decision, last_result, last_result_reason,
		next_check_at, next_check_after_run_id, cooldown_runs, evidence_json, updated_at
		FROM pulse_module_state old
		WHERE module IN (?, ?)
		AND updated_at=(SELECT MAX(updated_at) FROM pulse_module_state newer
			WHERE newer.workspace_path=old.workspace_path AND newer.module IN (?, ?))
		ORDER BY updated_at DESC
		ON CONFLICT(workspace_path, module) DO UPDATE SET
			last_pulse_run_id=excluded.last_pulse_run_id,
			last_checked_at=excluded.last_checked_at,
			last_ran_at=excluded.last_ran_at,
			last_decision=excluded.last_decision,
			last_reason=excluded.last_reason,
			last_gate_decision=excluded.last_gate_decision,
			last_result=excluded.last_result,
			last_result_reason=excluded.last_result_reason,
			next_check_at=excluded.next_check_at,
			next_check_after_run_id=excluded.next_check_after_run_id,
			cooldown_runs=excluded.cooldown_runs,
			evidence_json=excluded.evidence_json,
			updated_at=excluded.updated_at
		WHERE excluded.updated_at > pulse_module_state.updated_at`, pulsemodules.StrategicReviewID, legacyStrategy, legacyGoal, legacyStrategy, legacyGoal); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM pulse_module_state WHERE module IN (?, ?)`, legacyStrategy, legacyGoal); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO pulse_module_audit (
		workspace_path, module, pulse_run_id, result, reason, evidence_json,
		changed_files_json, verification_json, before_refs_json, after_refs_json, recorded_at)
		SELECT workspace_path, ?, pulse_run_id, result, reason, evidence_json,
		changed_files_json, verification_json, before_refs_json, after_refs_json, recorded_at
		FROM pulse_module_audit WHERE module IN (?, ?) ORDER BY recorded_at DESC`,
		pulsemodules.StrategicReviewID, legacyStrategy, legacyGoal); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM pulse_module_audit WHERE module IN (?, ?)`, legacyStrategy, legacyGoal); err != nil {
		return err
	}
	return tx.Commit()
}

func migratePulseModuleStateSchema(ctx context.Context, db *sql.DB) error {
	rows, err := db.QueryContext(ctx, `PRAGMA table_info(pulse_module_state)`)
	if err != nil {
		return err
	}
	defer rows.Close()

	pk := map[string]int{}
	hasTable := false
	for rows.Next() {
		hasTable = true
		var cid, notNull, pkIndex int
		var name, colType string
		var defaultValue sql.NullString
		if err := rows.Scan(&cid, &name, &colType, &notNull, &defaultValue, &pkIndex); err != nil {
			return err
		}
		if pkIndex > 0 {
			pk[name] = pkIndex
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if !hasTable {
		return nil
	}
	if pk["workspace_path"] > 0 && pk["module"] > 0 {
		return nil
	}
	if pk["module"] == 0 {
		return nil
	}

	legacyTable := fmt.Sprintf("pulse_module_state_legacy_%d", time.Now().UnixNano())
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, fmt.Sprintf(`ALTER TABLE pulse_module_state RENAME TO %s`, legacyTable)); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DROP INDEX IF EXISTS idx_pulse_module_state_run`); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, pulseModuleStateSchema); err != nil {
		return err
	}
	insert := fmt.Sprintf(`INSERT OR REPLACE INTO pulse_module_state (
			workspace_path, module, last_pulse_run_id, last_checked_at, last_ran_at,
			last_decision, last_reason, last_gate_decision, last_result, last_result_reason,
			next_check_at, next_check_after_run_id, cooldown_runs, evidence_json, updated_at
		)
		SELECT workspace_path, module, last_pulse_run_id, last_checked_at, last_ran_at,
			last_decision, last_reason,
			CASE WHEN last_decision IN ('due', 'skipped') THEN last_decision ELSE '' END,
			CASE WHEN last_decision IN ('done', 'changed', 'blocked', 'failed') THEN last_decision ELSE '' END,
			CASE WHEN last_decision IN ('done', 'changed', 'blocked', 'failed') THEN last_reason ELSE '' END,
			next_check_at, next_check_after_run_id,
			cooldown_runs, evidence_json, updated_at
		FROM %s`, legacyTable)
	if _, err := tx.ExecContext(ctx, insert); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, fmt.Sprintf(`DROP TABLE %s`, legacyTable)); err != nil {
		return err
	}
	return tx.Commit()
}

func ensurePulseModuleStateColumns(ctx context.Context, db *sql.DB) error {
	rows, err := db.QueryContext(ctx, `PRAGMA table_info(pulse_module_state)`)
	if err != nil {
		return err
	}
	defer rows.Close()
	cols := map[string]bool{}
	for rows.Next() {
		var cid, notNull, pkIndex int
		var name, colType string
		var defaultValue sql.NullString
		if err := rows.Scan(&cid, &name, &colType, &notNull, &defaultValue, &pkIndex); err != nil {
			return err
		}
		cols[name] = true
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, col := range []string{"last_gate_decision", "last_result", "last_result_reason"} {
		if cols[col] {
			continue
		}
		if _, err := db.ExecContext(ctx, fmt.Sprintf(`ALTER TABLE pulse_module_state ADD COLUMN %s TEXT NOT NULL DEFAULT ''`, col)); err != nil {
			return err
		}
	}
	return nil
}

func openPulseModuleStateDB(ctx context.Context, workspacePath string, create bool) (string, *sql.DB, error) {
	normalized, db, err := openReportHumanInputDB(ctx, workspacePath, create)
	if err != nil || db == nil {
		return normalized, db, err
	}
	if create {
		if err := ensurePulseModuleStateSchema(ctx, db); err != nil {
			_ = db.Close()
			return "", nil, err
		}
	}
	return normalized, db, nil
}

func recordPulseWorklist(ctx context.Context, workspacePath, pulseRunID string, decisions []PulseWorklistDecision) ([]PulseModuleState, error) {
	return recordPulseWorklistWithMode(ctx, workspacePath, pulseRunID, pulseRunModeDiscovery, "Direct worklist call; Gate mode was not supplied.", decisions)
}

func recordPulseWorklistWithMode(ctx context.Context, workspacePath, pulseRunID, mode, modeReason string, decisions []PulseWorklistDecision) ([]PulseModuleState, error) {
	pulseRunID = strings.TrimSpace(pulseRunID)
	if pulseRunID == "" {
		return nil, fmt.Errorf("pulse_run_id is required: pass the scheduler-provided Pulse run id exactly as it appears in the prompt")
	}
	mode, modeReason, err := normalizePulseRunMode(mode, modeReason)
	if err != nil {
		return nil, err
	}
	if err := validatePulseWorklistDecisions(decisions); err != nil {
		return nil, err
	}
	normalized, db, err := openPulseModuleStateDB(ctx, workspacePath, true)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := tx.ExecContext(ctx, `INSERT INTO pulse_run_mode (workspace_path, pulse_run_id, mode, reason, recorded_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(workspace_path, pulse_run_id) DO UPDATE SET
			mode=excluded.mode, reason=excluded.reason, recorded_at=excluded.recorded_at`,
		normalized, pulseRunID, mode, modeReason, now); err != nil {
		return nil, err
	}
	states := make([]PulseModuleState, 0, len(decisions))
	for _, decision := range decisions {
		module := normalizePulseModule(decision.Module)
		if !validPulseModules[module] {
			return nil, fmt.Errorf("module %q is not a valid Pulse module. Must be one of: %s", decision.Module, pulseModuleList())
		}
		reason := strings.TrimSpace(decision.Reason)
		if reason == "" {
			return nil, fmt.Errorf("reason is required for module %q", module)
		}
		lastDecision := "skipped"
		if decision.Due {
			lastDecision = "due"
		}
		evidence := normalizePulseEvidence(decision.Evidence)
		evidenceJSON, _ := json.Marshal(evidence)
		state := PulseModuleState{
			WorkspacePath:       normalized,
			Module:              module,
			LastPulseRunID:      pulseRunID,
			LastCheckedAt:       now,
			LastDecision:        lastDecision,
			LastReason:          reason,
			LastGateDecision:    lastDecision,
			NextCheckAt:         strings.TrimSpace(decision.NextCheckAt),
			NextCheckAfterRunID: strings.TrimSpace(decision.NextCheckAfterRunID),
			CooldownRuns:        decision.CooldownRuns,
			Evidence:            evidence,
			UpdatedAt:           now,
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO pulse_module_state (
				module, workspace_path, last_pulse_run_id, last_checked_at, last_decision,
				last_reason, last_gate_decision, last_result, last_result_reason,
				next_check_at, next_check_after_run_id, cooldown_runs,
				evidence_json, updated_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, '', '', ?, ?, ?, ?, ?)
			ON CONFLICT(workspace_path, module) DO UPDATE SET
				last_pulse_run_id=excluded.last_pulse_run_id,
				last_checked_at=excluded.last_checked_at,
				last_decision=excluded.last_decision,
				last_reason=excluded.last_reason,
				last_gate_decision=excluded.last_gate_decision,
				last_result='',
				last_result_reason='',
				next_check_at=excluded.next_check_at,
				next_check_after_run_id=excluded.next_check_after_run_id,
				cooldown_runs=excluded.cooldown_runs,
				evidence_json=excluded.evidence_json,
				updated_at=excluded.updated_at`,
			module, normalized, pulseRunID, now, lastDecision, reason,
			lastDecision,
			state.NextCheckAt, state.NextCheckAfterRunID, state.CooldownRuns,
			string(evidenceJSON), now,
		)
		if err != nil {
			return nil, err
		}
		states = append(states, state)
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return states, nil
}

func recordPulseWorklistOnce(ctx context.Context, workspacePath, pulseRunID string, decisions []PulseWorklistDecision) ([]PulseModuleState, error) {
	return recordPulseWorklistOnceAfter(ctx, workspacePath, pulseRunID, pulseRunModeDiscovery, "Direct worklist call; Gate mode was not supplied.", decisions, nil)
}

func recordPulseWorklistOnceWithShadowAndMode(ctx context.Context, workspacePath, pulseRunID, mode, modeReason string, decisions []PulseWorklistDecision, shadowResult loopclosure.Result) ([]PulseModuleState, error) {
	return recordPulseWorklistOnceAfter(ctx, workspacePath, pulseRunID, mode, modeReason, decisions, func() {
		// Shadow instrumentation cannot block or modify live scheduling.
		// Coverage failures are retained in shadowResult rather than converted
		// to an empty, apparently-clean signal set.
		if err := recordPulseShadowSignalObservation(ctx, workspacePath, pulseRunID, shadowResult, decisions); err != nil {
			log.Printf("[PULSE] record shadow loop-closure observation for %s: %v", workspacePath, err)
		}
	})
}

func recordPulseWorklistOnceAfter(ctx context.Context, workspacePath, pulseRunID, mode, modeReason string, decisions []PulseWorklistDecision, afterRecord func()) ([]PulseModuleState, error) {
	if _, _, err := normalizePulseRunMode(mode, modeReason); err != nil {
		return nil, err
	}
	if err := validatePulseWorklistDecisions(decisions); err != nil {
		return nil, err
	}

	pulseWorklistRecordMu.Lock()
	defer pulseWorklistRecordMu.Unlock()
	// Revalidate at the serialized write boundary. A session may have been
	// revoked after the tool call began but before argument parsing finished.
	if err := validatePulseToolRunID(ctx, pulseRunID); err != nil {
		return nil, err
	}

	existing, exists, err := getPulseWorklistForRun(ctx, workspacePath, pulseRunID)
	if err != nil {
		return nil, err
	}
	if exists && pulseWorklistIsComplete(existing) {
		states := make([]PulseModuleState, 0, len(pulseModuleOrder))
		for _, module := range pulseModuleOrder {
			states = append(states, existing[module])
		}
		return states, nil
	}
	states, err := recordPulseWorklistWithMode(ctx, workspacePath, pulseRunID, mode, modeReason, decisions)
	if err != nil {
		return nil, err
	}
	if afterRecord != nil {
		afterRecord()
	}
	return states, nil
}

func normalizePulseRunMode(mode, reason string) (string, string, error) {
	mode = strings.ToLower(strings.TrimSpace(mode))
	reason = strings.TrimSpace(reason)
	valid := false
	for _, value := range pulseRunModeValues {
		if mode == value {
			valid = true
			break
		}
	}
	if !valid {
		return "", "", fmt.Errorf("mode %q is not valid; choose one of: %s", mode, strings.Join(pulseRunModeValues, ", "))
	}
	if reason == "" {
		return "", "", fmt.Errorf("mode_reason is required: explain why this pass is %q", mode)
	}
	return mode, reason, nil
}

func validatePulseWorklistDecisions(decisions []PulseWorklistDecision) error {
	if len(decisions) == 0 {
		return fmt.Errorf("decisions are required: exactly one entry for each Pulse module (%s), each with module, due, and reason", pulseModuleList())
	}
	if len(decisions) != len(pulseModuleOrder) {
		return fmt.Errorf("decisions must include exactly one entry for each Pulse module; got %d, want %d covering: %s",
			len(decisions), len(pulseModuleOrder), pulseModuleList())
	}
	seen := map[string]bool{}
	for _, decision := range decisions {
		module := normalizePulseModule(decision.Module)
		if !validPulseModules[module] {
			return fmt.Errorf("module %q is not a valid Pulse module. Must be one of: %s", decision.Module, pulseModuleList())
		}
		if seen[module] {
			return fmt.Errorf("module %q appears more than once", module)
		}
		seen[module] = true
		if strings.TrimSpace(decision.Reason) == "" {
			return fmt.Errorf("reason is required for module %q", module)
		}
		if decision.CooldownRuns < 0 {
			return fmt.Errorf("cooldown_runs must be non-negative for module %q", module)
		}
		nextCheckAt := strings.TrimSpace(decision.NextCheckAt)
		if nextCheckAt != "" {
			if _, err := time.Parse(time.RFC3339Nano, nextCheckAt); err != nil {
				if _, dateErr := time.Parse("2006-01-02", nextCheckAt); dateErr != nil {
					return fmt.Errorf("next_check_at must be RFC3339 or YYYY-MM-DD for module %q", module)
				}
			}
		}
		if !decision.Due && nextCheckAt == "" && strings.TrimSpace(decision.NextCheckAfterRunID) == "" && decision.CooldownRuns == 0 {
			return fmt.Errorf("skipped module %q must include next_check_at, next_check_after_run_id, or cooldown_runs", module)
		}
	}
	missing := []string{}
	for _, module := range pulseModuleOrder {
		if !seen[module] {
			missing = append(missing, module)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("decisions is missing required module(s) %s; every Pulse module needs its own entry and the complete set is: %s",
			strings.Join(missing, ", "), pulseModuleList())
	}
	return nil
}

func pulseWorklistIsComplete(worklist map[string]PulseModuleState) bool {
	if len(worklist) != len(pulseModuleOrder) {
		return false
	}
	for _, module := range pulseModuleOrder {
		if _, ok := worklist[module]; !ok {
			return false
		}
	}
	return true
}

// validatePulseGateCompletion validates only Gate's durable control-plane
// output. Gate does not own any presentation artifact; coupling routing to a
// UI write made a presentation failure discard an otherwise complete worklist.
func validatePulseGateCompletion(ctx context.Context, workspacePath, pulseRunID string) error {
	worklist, exists, err := getPulseWorklistForRun(ctx, workspacePath, pulseRunID)
	if err != nil {
		return fmt.Errorf("read Pulse Gate worklist: %w", err)
	}
	if !exists || !pulseWorklistIsComplete(worklist) {
		return fmt.Errorf("Pulse Gate did not record a complete worklist for pulse_run_id %q", pulseRunID)
	}
	return nil
}

func markPulseModuleResult(ctx context.Context, workspacePath, module, pulseRunID, result, reason string, evidence []string) (*PulseModuleState, error) {
	module = normalizePulseModule(module)
	if !validPulseModules[module] {
		return nil, fmt.Errorf("module %q is not a valid Pulse module. Must be one of: %s", module, pulseModuleList())
	}
	pulseRunID = strings.TrimSpace(pulseRunID)
	if pulseRunID == "" {
		return nil, fmt.Errorf("pulse_run_id is required: pass the scheduler-provided Pulse run id exactly as it appears in the prompt")
	}
	result = strings.TrimSpace(strings.ToLower(result))
	if result == "" {
		return nil, fmt.Errorf("result is required. Must be one of: %s", strings.Join(pulseSchedulerModuleResultValues, ", "))
	}
	switch result {
	case "done", "changed", "blocked", "failed", "skipped", "timed_out":
	default:
		return nil, fmt.Errorf("result %q is not valid. Must be one of: %s", result, strings.Join(pulseSchedulerModuleResultValues, ", "))
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return nil, fmt.Errorf("reason is required: one sentence stating the module's outcome")
	}

	normalized, db, err := openPulseModuleStateDB(ctx, workspacePath, true)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	now := time.Now().UTC().Format(time.RFC3339)
	evidence = normalizePulseEvidence(evidence)
	evidenceJSON, _ := json.Marshal(evidence)
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `INSERT INTO pulse_module_state (
			module, workspace_path, last_pulse_run_id, last_checked_at, last_ran_at,
			last_result, last_result_reason, evidence_json, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(workspace_path, module) DO UPDATE SET
			last_pulse_run_id=excluded.last_pulse_run_id,
			last_ran_at=excluded.last_ran_at,
			last_result=excluded.last_result,
			last_result_reason=excluded.last_result_reason,
			evidence_json=excluded.evidence_json,
			updated_at=excluded.updated_at`,
		module, normalized, pulseRunID, now, now, result, reason, string(evidenceJSON), now,
	)
	if err != nil {
		return nil, err
	}
	if err := recordPulseModuleAudit(ctx, tx, normalized, module, pulseRunID, result, reason, evidence, PulseModuleAuditInput{}, now); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	state, err := getPulseModuleStateByModule(ctx, db, normalized, module)
	if err != nil {
		return nil, err
	}
	return state, nil
}

func markPulseModuleResultFromAgent(ctx context.Context, workspacePath, module, pulseRunID, result, reason string, evidence []string) (*PulseModuleState, error) {
	return markPulseModuleResultFromAgentWithAudit(ctx, workspacePath, module, pulseRunID, result, reason, evidence, PulseModuleAuditInput{})
}

func markPulseModuleResultFromAgentWithAudit(ctx context.Context, workspacePath, module, pulseRunID, result, reason string, evidence []string, audit PulseModuleAuditInput) (*PulseModuleState, error) {
	return markPulseModuleResultFromAgentWithAuditAndFindings(
		ctx, workspacePath, module, pulseRunID, result, reason, evidence, audit, nil,
	)
}

func markPulseModuleResultFromAgentWithAuditAndFindings(
	ctx context.Context,
	workspacePath, module, pulseRunID, result, reason string,
	evidence []string,
	audit PulseModuleAuditInput,
	dispositions []step_based_workflow.PulseFindingDisposition,
) (*PulseModuleState, error) {
	module = normalizePulseModule(module)
	if !validPulseModules[module] {
		return nil, fmt.Errorf("module %q is not a valid Pulse module. Must be one of: %s", module, pulseModuleList())
	}
	pulseRunID = strings.TrimSpace(pulseRunID)
	result = strings.TrimSpace(strings.ToLower(result))
	switch result {
	case "done", "changed", "blocked", "failed", "skipped":
	default:
		return nil, fmt.Errorf("result %q is not valid. Must be one of: %s. Use changed only when files were modified, and pair it with changed_files, verification, and finding_dispositions",
			result, strings.Join(pulseModuleResultValues, ", "))
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return nil, fmt.Errorf("reason is required: one sentence stating the module's outcome, including the exact technical failure when result is blocked or failed")
	}

	normalized, db, err := openPulseModuleStateDB(ctx, workspacePath, true)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	now := time.Now().UTC().Format(time.RFC3339)
	evidence = normalizePulseEvidence(evidence)
	evidenceJSON, _ := json.Marshal(evidence)
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	resultExec, err := tx.ExecContext(ctx, `UPDATE pulse_module_state SET
			last_ran_at = ?, last_result = ?, last_result_reason = ?, evidence_json = ?, updated_at = ?
		WHERE workspace_path = ? AND module = ? AND last_pulse_run_id = ?
			AND last_decision = 'due' AND last_result = ''`,
		now, result, reason, string(evidenceJSON), now, normalized, module, pulseRunID)
	if err != nil {
		return nil, err
	}
	if changed, err := resultExec.RowsAffected(); err != nil {
		return nil, err
	} else if changed == 0 {
		_ = tx.Rollback()
		existing, readErr := getPulseModuleStateByModule(ctx, db, normalized, module)
		if readErr != nil {
			return nil, fmt.Errorf("Pulse module %q is not an unresolved due module for run %q", module, pulseRunID)
		}
		if existing.LastPulseRunID == pulseRunID && existing.LastResult == result {
			retryTx, err := db.BeginTx(ctx, nil)
			if err != nil {
				return nil, err
			}
			defer retryTx.Rollback()
			if err := recordPulseModuleAudit(ctx, retryTx, normalized, module, pulseRunID, result, reason, evidence, audit, now); err != nil {
				return nil, err
			}
			if err := step_based_workflow.RecordPulseFindingDispositionsTx(
				ctx, retryTx, module, pulseRunID, dispositions, now,
			); err != nil {
				return nil, err
			}
			if err := retryTx.Commit(); err != nil {
				return nil, err
			}
			return existing, nil
		}
		return nil, fmt.Errorf("Pulse module %q for run %q is already terminal or belongs to another run", module, pulseRunID)
	}
	if err := recordPulseModuleAudit(ctx, tx, normalized, module, pulseRunID, result, reason, evidence, audit, now); err != nil {
		return nil, err
	}
	if err := step_based_workflow.RecordPulseFindingDispositionsTx(
		ctx, tx, module, pulseRunID, dispositions, now,
	); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return getPulseModuleStateByModule(ctx, db, normalized, module)
}

type pulseModuleAuditExecer interface {
	ExecContext(context.Context, string, ...interface{}) (sql.Result, error)
}

func recordPulseModuleAudit(ctx context.Context, execer pulseModuleAuditExecer, workspacePath, module, pulseRunID, result, reason string, evidence []string, audit PulseModuleAuditInput, recordedAt string) error {
	marshal := func(values []string) string {
		encoded, _ := json.Marshal(normalizePulseEvidence(values))
		return string(encoded)
	}
	_, err := execer.ExecContext(ctx, `INSERT INTO pulse_module_audit (
			workspace_path, module, pulse_run_id, result, reason, evidence_json,
			changed_files_json, verification_json, before_refs_json, after_refs_json, recorded_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(workspace_path, module, pulse_run_id) DO UPDATE SET
			result=excluded.result,
			reason=excluded.reason,
			evidence_json=excluded.evidence_json,
			changed_files_json=excluded.changed_files_json,
			verification_json=excluded.verification_json,
			before_refs_json=excluded.before_refs_json,
			after_refs_json=excluded.after_refs_json,
			recorded_at=excluded.recorded_at`,
		workspacePath, module, pulseRunID, result, reason, marshal(evidence),
		marshal(audit.ChangedFiles), marshal(audit.Verification), marshal(audit.BeforeRefs), marshal(audit.AfterRefs), recordedAt)
	return err
}

const pulseShadowDetectorLoopClosure = "loop_closure"

func recordPulseShadowSignalObservation(ctx context.Context, workspacePath, pulseRunID string, result loopclosure.Result, decisions []PulseWorklistDecision) error {
	pulseRunID = strings.TrimSpace(pulseRunID)
	if pulseRunID == "" {
		return fmt.Errorf("pulse_run_id is required: pass the scheduler-provided Pulse run id exactly as it appears in the prompt")
	}
	normalized, db, err := openPulseModuleStateDB(ctx, workspacePath, true)
	if err != nil {
		return err
	}
	defer db.Close()

	signalsJSON, err := json.Marshal(result.Findings)
	if err != nil {
		return err
	}
	decisionsJSON, err := json.Marshal(decisions)
	if err != nil {
		return err
	}
	recordedAt := time.Now().UTC().Format(time.RFC3339)
	_, err = db.ExecContext(ctx, `INSERT INTO pulse_shadow_signal_observation (
			workspace_path, pulse_run_id, detector, detector_version, observed_at,
			coverage_status, coverage_reason, signals_json, gate_decisions_json, recorded_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(workspace_path, pulse_run_id, detector) DO NOTHING`,
		normalized, pulseRunID, pulseShadowDetectorLoopClosure, result.DetectorVersion,
		result.ObservedAt, result.CoverageStatus, result.CoverageReason,
		string(signalsJSON), string(decisionsJSON), recordedAt)
	return err
}

func getPulseShadowSignalObservations(ctx context.Context, workspacePath string, limit int) ([]PulseShadowSignalObservation, error) {
	if limit <= 0 {
		limit = 20
	}
	normalized, db, err := openPulseModuleStateDB(ctx, workspacePath, false)
	if err != nil {
		return nil, err
	}
	if db == nil {
		return []PulseShadowSignalObservation{}, nil
	}
	defer db.Close()
	if err := ensurePulseModuleStateSchema(ctx, db); err != nil {
		return nil, err
	}
	rows, err := db.QueryContext(ctx, `SELECT workspace_path, pulse_run_id, detector,
			detector_version, observed_at, coverage_status, coverage_reason,
			signals_json, gate_decisions_json, recorded_at
		FROM pulse_shadow_signal_observation
		WHERE workspace_path = ?
		ORDER BY observed_at DESC
		LIMIT ?`, normalized, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []PulseShadowSignalObservation{}
	for rows.Next() {
		var observation PulseShadowSignalObservation
		var signalsJSON, decisionsJSON string
		if err := rows.Scan(
			&observation.WorkspacePath,
			&observation.PulseRunID,
			&observation.Detector,
			&observation.DetectorVersion,
			&observation.ObservedAt,
			&observation.CoverageStatus,
			&observation.CoverageReason,
			&signalsJSON,
			&decisionsJSON,
			&observation.RecordedAt,
		); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(signalsJSON), &observation.Signals); err != nil {
			return nil, fmt.Errorf("decode shadow signals for %s: %w", observation.PulseRunID, err)
		}
		if err := json.Unmarshal([]byte(decisionsJSON), &observation.GateDecisions); err != nil {
			return nil, fmt.Errorf("decode shadow Gate decisions for %s: %w", observation.PulseRunID, err)
		}
		if observation.Signals == nil {
			observation.Signals = []loopclosure.Finding{}
		}
		if observation.GateDecisions == nil {
			observation.GateDecisions = []PulseWorklistDecision{}
		}
		out = append(out, observation)
	}
	return out, rows.Err()
}

func getPulseModuleStates(ctx context.Context, workspacePath string) ([]PulseModuleState, error) {
	normalized, db, err := openPulseModuleStateDB(ctx, workspacePath, false)
	if err != nil {
		return nil, err
	}
	if db == nil {
		return []PulseModuleState{}, nil
	}
	defer db.Close()
	if err := ensurePulseModuleStateSchema(ctx, db); err != nil {
		return nil, err
	}
	rows, err := db.QueryContext(ctx, `SELECT module, workspace_path, last_pulse_run_id, last_checked_at, last_ran_at,
		last_decision, last_reason, last_gate_decision, last_result, last_result_reason,
		next_check_at, next_check_after_run_id, cooldown_runs, evidence_json, updated_at
		FROM pulse_module_state WHERE workspace_path = ? ORDER BY module`, normalized)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var states []PulseModuleState
	for rows.Next() {
		state, err := scanPulseModuleState(rows)
		if err != nil {
			return nil, err
		}
		states = append(states, *state)
	}
	return states, rows.Err()
}

func (api *StreamingAPI) handleGetPulseModuleState(w http.ResponseWriter, r *http.Request) {
	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}
	states, err := getPulseModuleStates(r.Context(), r.URL.Query().Get("workspace_path"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	commands, err := getPulseFinalCommandStates(r.Context(), r.URL.Query().Get("workspace_path"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	gateMode, err := getLatestPulseRunMode(r.Context(), r.URL.Query().Get("workspace_path"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	shadowObservations, shadowErr := getPulseShadowSignalObservations(r.Context(), r.URL.Query().Get("workspace_path"), 20)
	shadowCoverage := map[string]string{}
	if shadowErr != nil {
		log.Printf("[PULSE] shadow signal observations unavailable: %v", shadowErr)
		shadowObservations = []PulseShadowSignalObservation{}
		shadowCoverage = map[string]string{"status": "unavailable", "reason": shadowErr.Error()}
	} else if len(shadowObservations) == 0 {
		shadowCoverage = map[string]string{"status": "not_instrumented", "reason": "no shadow observations recorded yet"}
	} else {
		shadowCoverage["status"] = shadowObservations[0].CoverageStatus
		if shadowObservations[0].CoverageReason != "" {
			shadowCoverage["reason"] = shadowObservations[0].CoverageReason
		}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success":                    true,
		"modules":                    states,
		"commands":                   commands,
		"gate_mode":                  gateMode,
		"shadow_signal_observations": shadowObservations,
		"shadow_signal_coverage":     shadowCoverage,
	})
}

func (api *StreamingAPI) handleGetPulseFindings(w http.ResponseWriter, r *http.Request) {
	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}
	workspacePath, err := normalizeReportHumanInputWorkspacePath(r.URL.Query().Get("workspace_path"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	module := normalizePulseModule(r.URL.Query().Get("module"))
	if module != "" && !validPulseModules[module] {
		http.Error(w, fmt.Sprintf("module %q is not valid", module), http.StatusBadRequest)
		return
	}
	findings, err := step_based_workflow.LoadPulseFindingLifecycles(
		r.Context(), workspacePath, module, -1,
	)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success":  true,
		"findings": findings,
		"total":    len(findings),
	})
}

func (api *StreamingAPI) handleGetPulseReviews(w http.ResponseWriter, r *http.Request) {
	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}
	workspacePath, err := normalizeReportHumanInputWorkspacePath(r.URL.Query().Get("workspace_path"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	module := normalizePulseModule(r.URL.Query().Get("module"))
	if module != "" && !validPulseModules[module] {
		http.Error(w, fmt.Sprintf("module %q is not valid", module), http.StatusBadRequest)
		return
	}
	receipts, err := step_based_workflow.LoadPulseReviewReceipts(r.Context(), workspacePath, module, -1)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "reviews": receipts, "total": len(receipts)})
}

func (api *StreamingAPI) handleGetPulseAgentMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}
	workspacePath, err := normalizeReportHumanInputWorkspacePath(r.URL.Query().Get("workspace_path"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	module := strings.TrimSpace(r.URL.Query().Get("module"))
	if module != "" {
		module = normalizePulseModule(module)
		if !validPulseModules[module] {
			http.Error(w, fmt.Sprintf("module %q is not valid", module), http.StatusBadRequest)
			return
		}
	}
	role := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("role")))
	if role != "" && role != "reviewer" && role != "fixer" {
		http.Error(w, "role must be reviewer or fixer", http.StatusBadRequest)
		return
	}
	metrics, err := step_based_workflow.LoadPulseAgentMetrics(
		r.Context(), workspacePath, r.URL.Query().Get("pulse_run_id"), module, role, -1,
	)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"metrics": metrics,
		"total":   len(metrics),
	})
}

func (api *StreamingAPI) handleGetPulseImpact(w http.ResponseWriter, r *http.Request) {
	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}
	workspacePath, err := normalizeReportHumanInputWorkspacePath(r.URL.Query().Get("workspace_path"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	ledger, err := step_based_workflow.LoadPulseImpactLedger(r.Context(), workspacePath, 500)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "impact": ledger})
}

func (api *StreamingAPI) handleGetPulseContext(w http.ResponseWriter, r *http.Request) {
	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}
	workspacePath, err := normalizeReportHumanInputWorkspacePath(r.URL.Query().Get("workspace_path"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	records, err := step_based_workflow.LoadPulseContextRecords(r.Context(), workspacePath, 100)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "records": records, "total": len(records)})
}

func getPulseWorklistForRun(ctx context.Context, workspacePath, pulseRunID string) (map[string]PulseModuleState, bool, error) {
	normalized, db, err := openPulseModuleStateDB(ctx, workspacePath, false)
	if err != nil {
		return nil, false, err
	}
	if db == nil {
		return map[string]PulseModuleState{}, false, nil
	}
	defer db.Close()
	if err := ensurePulseModuleStateSchema(ctx, db); err != nil {
		return nil, false, err
	}
	rows, err := db.QueryContext(ctx, `SELECT module, workspace_path, last_pulse_run_id, last_checked_at, last_ran_at,
		last_decision, last_reason, last_gate_decision, last_result, last_result_reason,
		next_check_at, next_check_after_run_id, cooldown_runs, evidence_json, updated_at
		FROM pulse_module_state WHERE workspace_path = ? AND last_pulse_run_id = ?`, normalized, strings.TrimSpace(pulseRunID))
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	out := map[string]PulseModuleState{}
	for rows.Next() {
		state, err := scanPulseModuleState(rows)
		if err != nil {
			return nil, false, err
		}
		out[state.Module] = *state
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}
	return out, len(out) > 0, nil
}

func getPulseRunMode(ctx context.Context, workspacePath, pulseRunID string) (*PulseRunMode, error) {
	normalized, db, err := openPulseModuleStateDB(ctx, workspacePath, false)
	if err != nil || db == nil {
		return nil, err
	}
	defer db.Close()
	if err := ensurePulseModuleStateSchema(ctx, db); err != nil {
		return nil, err
	}
	mode := &PulseRunMode{}
	err = db.QueryRowContext(ctx, `SELECT workspace_path, pulse_run_id, mode, reason, recorded_at
		FROM pulse_run_mode WHERE workspace_path = ? AND pulse_run_id = ?`, normalized, strings.TrimSpace(pulseRunID)).Scan(
		&mode.WorkspacePath, &mode.PulseRunID, &mode.Mode, &mode.Reason, &mode.RecordedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return mode, nil
}

func getLatestPulseRunMode(ctx context.Context, workspacePath string) (*PulseRunMode, error) {
	normalized, db, err := openPulseModuleStateDB(ctx, workspacePath, false)
	if err != nil || db == nil {
		return nil, err
	}
	defer db.Close()
	if err := ensurePulseModuleStateSchema(ctx, db); err != nil {
		return nil, err
	}
	mode := &PulseRunMode{}
	err = db.QueryRowContext(ctx, `SELECT workspace_path, pulse_run_id, mode, reason, recorded_at
		FROM pulse_run_mode WHERE workspace_path = ? ORDER BY recorded_at DESC LIMIT 1`, normalized).Scan(
		&mode.WorkspacePath, &mode.PulseRunID, &mode.Mode, &mode.Reason, &mode.RecordedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return mode, nil
}

type pulseModuleScanner interface {
	Scan(dest ...interface{}) error
}

func getPulseModuleStateByModule(ctx context.Context, db *sql.DB, workspacePath, module string) (*PulseModuleState, error) {
	row := db.QueryRowContext(ctx, `SELECT module, workspace_path, last_pulse_run_id, last_checked_at, last_ran_at,
		last_decision, last_reason, last_gate_decision, last_result, last_result_reason,
		next_check_at, next_check_after_run_id, cooldown_runs, evidence_json, updated_at
		FROM pulse_module_state WHERE workspace_path = ? AND module = ?`, workspacePath, module)
	return scanPulseModuleState(row)
}

func scanPulseModuleState(row pulseModuleScanner) (*PulseModuleState, error) {
	var state PulseModuleState
	var evidenceJSON string
	if err := row.Scan(&state.Module, &state.WorkspacePath, &state.LastPulseRunID, &state.LastCheckedAt, &state.LastRanAt,
		&state.LastDecision, &state.LastReason, &state.LastGateDecision, &state.LastResult, &state.LastResultReason,
		&state.NextCheckAt, &state.NextCheckAfterRunID, &state.CooldownRuns,
		&evidenceJSON, &state.UpdatedAt); err != nil {
		return nil, err
	}
	if state.LastGateDecision == "" {
		state.LastGateDecision = state.LastDecision
	}
	if strings.TrimSpace(evidenceJSON) != "" {
		_ = json.Unmarshal([]byte(evidenceJSON), &state.Evidence)
	}
	if state.Evidence == nil {
		state.Evidence = []string{}
	}
	return &state, nil
}

func createPulseWorklistTools() ([]llmtypes.Tool, map[string]interface{}, map[string]string) {
	moduleEnum := append([]string(nil), pulseModuleOrder...)
	reviewIdentityProperties := map[string]interface{}{
		"workspace_path": map[string]interface{}{"type": "string", "description": "Workflow-relative path, e.g. Workflow/social-media."},
		"pulse_run_id":   map[string]interface{}{"type": "string", "description": "Use \"current\" for this active Pulse conversation."},
		"module":         map[string]interface{}{"type": "string", "enum": moduleEnum},
	}
	findingProperties := map[string]interface{}{}
	for key, value := range reviewIdentityProperties {
		findingProperties[key] = value
	}
	for key, value := range map[string]interface{}{
		"issue_id":          map[string]interface{}{"type": "string", "description": "Optional visible PUL issue_id from get_pulse_state(view=\"backlog\", detail=\"compact\"). Supply it when this is new evidence for an existing root cause; omit only for a genuinely new root issue."},
		"concern":           map[string]interface{}{"type": "string", "description": "Current concise root-cause statement. It may be reworded when issue_id is supplied; wording never creates a new identity by itself."},
		"issue_kind":        map[string]interface{}{"type": "string", "enum": []string{"workflow_issue", "harness_issue"}},
		"classification":    map[string]interface{}{"type": "string"},
		"severity":          map[string]interface{}{"type": "string", "enum": []string{"low", "medium", "high", "critical"}},
		"summary":           map[string]interface{}{"type": "string"},
		"impact":            map[string]interface{}{"type": "string"},
		"evidence":          map[string]interface{}{"type": "array", "minItems": 1, "items": map[string]interface{}{"type": "string"}},
		"recommended_route": map[string]interface{}{"type": "string", "enum": []string{"decision_required", "evidence_wait", "fixer_handoff"}},
		"human_input_id":    map[string]interface{}{"type": "string", "description": "Required with recommended_route=decision_required. Create the pending reviewer-owned decision first and pass its exact id so the finding is atomically linked as awaiting_user."},
		"next_check":        map[string]interface{}{"type": "string"},
		"workaround":        map[string]interface{}{"type": "string"},
		"target_key":        map[string]interface{}{"type": "string"},
		"reproduction": map[string]interface{}{
			"type": "object", "additionalProperties": false,
			"properties": map[string]interface{}{
				"safe": map[string]interface{}{"type": "boolean"}, "setup": map[string]interface{}{"type": "string"},
				"action": map[string]interface{}{"type": "string"}, "expected": map[string]interface{}{"type": "string"},
				"observed": map[string]interface{}{"type": "string"}, "limitations": map[string]interface{}{"type": "string"},
			},
		},
	} {
		findingProperties[key] = value
	}
	recordFindingTool := llmtypes.Tool{Type: "function", Function: &llmtypes.FunctionDefinition{
		Name:        "record_pulse_finding",
		Description: "Persist one complete Pulse reviewer finding directly to the SQLite lifecycle. First inspect the compact active backlog index and reason about semantic sameness; request targeted full detail only for candidate issue_ids. For an existing root cause, supply its issue_id and update it; do not create a second issue because wording, evidence paths, or symptoms differ. Omit issue_id only for a genuinely distinct root cause with a different repair, owner, or verification boundary. Do not encode findings in the final response. Backup, publish, and notify waiting for their ordered finalizer stage are not findings; report only a real terminal failure after that command ran.",
		Parameters:  llmtypes.NewParameters(map[string]interface{}{"type": "object", "additionalProperties": false, "properties": findingProperties, "required": []string{"workspace_path", "pulse_run_id", "module", "concern", "issue_kind", "classification", "severity", "summary", "impact", "evidence"}}),
	}}
	verificationProperties := map[string]interface{}{}
	for key, value := range reviewIdentityProperties {
		verificationProperties[key] = value
	}
	for key, value := range map[string]interface{}{
		"issue_id": map[string]interface{}{"type": "string", "description": "The visible issue_id from get_pulse_state(view=\"backlog\", detail=\"compact\"). The backend resolves the pending internal attempt."},
		"verdict":  map[string]interface{}{"type": "string", "enum": []string{"passed", "failed", "inconclusive"}},
		"expected": map[string]interface{}{"type": "string"}, "observed": map[string]interface{}{"type": "string"},
		"evidence": map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}}, "next_check": map[string]interface{}{"type": "string"},
	} {
		verificationProperties[key] = value
	}
	recordVerificationTool := llmtypes.Tool{Type: "function", Function: &llmtypes.FunctionDefinition{
		Name:        "record_pulse_verification",
		Description: "Persist one reviewer judgment for a pending changed_unverified issue selected from get_pulse_state(view=\"backlog\", detail=\"compact\"). Send only issue_id; the backend resolves the exact eligible internal attempt.",
		Parameters:  llmtypes.NewParameters(map[string]interface{}{"type": "object", "additionalProperties": false, "properties": verificationProperties, "required": []string{"workspace_path", "pulse_run_id", "module", "issue_id", "verdict", "expected", "observed"}}),
	}}
	completeReviewTool := llmtypes.Tool{Type: "function", Function: &llmtypes.FunctionDefinition{
		Name:        "complete_pulse_review",
		Description: "Finalize compact SQLite receipts after all findings and verifications have been recorded through their tools. Finding and verification counts are computed by the backend. Call exactly once before the review's brief final message; final response text is not persisted or parsed.",
		Parameters: llmtypes.NewParameters(map[string]interface{}{"type": "object", "additionalProperties": false, "properties": map[string]interface{}{
			"workspace_path": reviewIdentityProperties["workspace_path"], "pulse_run_id": reviewIdentityProperties["pulse_run_id"],
			"modules": map[string]interface{}{"type": "array", "minItems": 1, "uniqueItems": true, "items": map[string]interface{}{"type": "string", "enum": moduleEnum}},
			"verdict": map[string]interface{}{"type": "string", "description": "Compact overall judgment; not a findings transport."}, "status": map[string]interface{}{"type": "string", "enum": []string{"completed", "failed"}},
		}, "required": []string{"workspace_path", "pulse_run_id", "modules", "verdict", "status"}}),
	}}
	mergeIssuesTool := llmtypes.Tool{Type: "function", Function: &llmtypes.FunctionDefinition{
		Name:        "merge_pulse_issues",
		Description: "Merge symptom-level duplicate Pulse issues into one canonical root-cause issue after a semantic backlog review. This never deletes history: it retires each duplicate from the active queue, links it to the canonical PUL issue, and preserves all attempts, events, and verification records. Use only when the issues share the same causal defect and compatible repair/verification boundary; never merge merely because they involve the same file or module.",
		Parameters: llmtypes.NewParameters(map[string]interface{}{"type": "object", "additionalProperties": false, "properties": map[string]interface{}{
			"workspace_path":      map[string]interface{}{"type": "string"},
			"canonical_issue_id":  map[string]interface{}{"type": "string", "description": "The one visible PUL issue that remains active."},
			"duplicate_issue_ids": map[string]interface{}{"type": "array", "minItems": 1, "items": map[string]interface{}{"type": "string"}, "description": "Visible PUL issue IDs to retire into the canonical issue."},
			"reason":              map[string]interface{}{"type": "string", "description": "One sentence naming the shared root cause and why one repair covers the duplicates."},
		}, "required": []string{"workspace_path", "canonical_issue_id", "duplicate_issue_ids", "reason"}}),
	}}
	recordTool := llmtypes.Tool{
		Type: "function",
		Function: &llmtypes.FunctionDefinition{
			Name:        "record_pulse_worklist",
			Description: fmt.Sprintf("Record the dynamic Pulse worklist for this run in the workflow's db/db.sqlite. Pulse Gate must call this exactly once after choosing the agent-owned pass mode and deciding which perspectives are due or skipped. backlog_drain verifies and repairs retained issues without broad discovery; discovery investigates materially new evidence; strategy is for selected product/goal work; observe runs no reviewer or fixer. Go validates the declared mode but never selects it. The decisions array must contain exactly one entry for each current Pulse module: %s. Select only work justified by evidence and expected value; explicitly defer lower-priority lenses with a reason and next-check boundary. workflow_review is Engineering Review and conditionally covers execution, report/eval implementation, plan-change/artifact consistency, and store-integrity evidence. When store integrity is selected, name the Stores Health lens (learnings, knowledgebase, and/or DB) in the workflow_review reason/evidence. llm_ops_review owns efficiency and runtime operations. strategic_review owns both adequacy of the current strategy and conditional discovery of materially different approaches as turns in one sequence. Engineering and Ops share one ordered sequence; Strategic Review uses a separate ordered sequence. Do not pass retired module names. Every skipped module must include next_check_at, next_check_after_run_id, or a positive cooldown_runs value.", strings.Join(pulseModuleOrder, ", ")),
			Parameters: llmtypes.NewParameters(map[string]interface{}{
				"type":                 "object",
				"additionalProperties": false,
				"properties": map[string]interface{}{
					"workspace_path": map[string]interface{}{"type": "string", "description": "Workflow-relative path, e.g. Workflow/social-media."},
					"pulse_run_id":   map[string]interface{}{"type": "string", "description": "Scheduler-provided Pulse run id. Use exactly the id in the prompt."},
					"mode":           map[string]interface{}{"type": "string", "enum": pulseRunModeValues, "description": "Agent-selected Pulse pass shape."},
					"mode_reason":    map[string]interface{}{"type": "string", "description": "Evidence why this mode is the cheapest sufficient next action."},
					"decisions": map[string]interface{}{
						"type": "array",
						"items": map[string]interface{}{
							"type":                 "object",
							"additionalProperties": false,
							"properties": map[string]interface{}{
								"module":                  map[string]interface{}{"type": "string", "enum": moduleEnum},
								"due":                     map[string]interface{}{"type": "boolean"},
								"reason":                  map[string]interface{}{"type": "string", "description": "Plain-language reason with the evidence basis."},
								"evidence":                map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}},
								"next_check_at":           map[string]interface{}{"type": "string", "description": "Optional RFC3339 timestamp or YYYY-MM-DD date for the next normal check."},
								"next_check_after_run_id": map[string]interface{}{"type": "string", "description": "Optional run id/folder after which to check again."},
								"cooldown_runs":           map[string]interface{}{"type": "integer", "minimum": 0, "description": "Optional number of future runs to skip unless new evidence overrides it."},
							},
							"required": []string{"module", "due", "reason"},
						},
					},
				},
				"required": []string{"workspace_path", "pulse_run_id", "mode", "mode_reason", "decisions"},
			}),
		},
	}
	stateTool := llmtypes.Tool{
		Type: "function",
		Function: &llmtypes.FunctionDefinition{
			Name: "get_pulse_state",
			Description: fmt.Sprintf("Read Pulse state from the workflow's db/db.sqlite. One read tool with three views; typed SQLite state is the only source of truth.\n"+
				"view=\"module\": per-module cadence and results so Pulse Gate can decide what is due, plus the complete active concern backlog, externally owned suppressed concerns, plan-change backlog, reviewer history, impact ledger, and read-only loop-closure facts. Read this before record_pulse_worklist. Loop-closure findings are evidence Gate may weigh; they do not mandate a module or authorize mutation. A concern with a high seen_count has been reported on that many runs and should weigh heavily.\n"+
				"view=\"backlog\": a compact active issue/observation index by default. Read it once to select relevant public PUL ids. To inspect lifecycle evidence, call again with detail=\"full\" and 1-20 exact issue_ids; broad full-history reads are rejected. Optional module filter. The backend resolves internal fingerprints; never copy, invent, or submit one.\n"+
				"view=\"review\": one compact reviewer receipt as JSON with validated structured verifications. Requires the stored pulse-run receipt id and module; reviewer prose and Markdown are not stored.\n"+
				"Close a real finding only through a verified finding_disposition on record_pulse_result; resolve_run_concern is limited to acknowledgment or rejection. Modules: %s.", pulseModuleList()),
			Parameters: llmtypes.NewParameters(map[string]interface{}{
				"type":                 "object",
				"additionalProperties": false,
				"properties": map[string]interface{}{
					"workspace_path": map[string]interface{}{"type": "string", "description": "Workflow-relative path, e.g. Workflow/social-media."},
					"view":           map[string]interface{}{"type": "string", "enum": pulseStateViewValues, "description": "Which Pulse state to read: module cadence, the finding backlog, or one saved review."},
					"pulse_run_id":   map[string]interface{}{"type": "string", "description": "Optional current Pulse run id. With view=module, returns that Gate's persisted pass mode."},
					"module":         map[string]interface{}{"type": "string", "description": "Optional owning-module filter for view=\"backlog\" (omit for the complete backlog). Required for view=\"review\". Ignored for view=\"module\"."},
					"review_run_id":  map[string]interface{}{"type": "string", "description": "Required for view=\"review\": the review run id from the reviewer's completion notification."},
					"detail":         map[string]interface{}{"type": "string", "enum": []string{"compact", "full"}, "description": "For view=\"backlog\" only. compact (default) returns the bounded active index. full requires issue_ids and returns complete lifecycle evidence only for those ids."},
					"issue_ids": map[string]interface{}{
						"type": "array", "minItems": 1, "maxItems": 20, "uniqueItems": true,
						"items":       map[string]interface{}{"type": "string", "pattern": "^PUL-[A-Za-z0-9]+$"},
						"description": "For view=\"backlog\" with detail=\"full\": 1-20 exact public PUL ids selected from the compact issues or observations index.",
					},
				},
				"required": []string{"workspace_path", "view"},
			}),
		},
	}
	resultTool := llmtypes.Tool{
		Type: "function",
		Function: &llmtypes.FunctionDefinition{
			Name: "record_pulse_result",
			Description: fmt.Sprintf("Record one Pulse outcome in the workflow's db/db.sqlite. Pass exactly one of module or command.\n"+
				"module (%s): the terminal result of a selected Pulse module after its review and Fixer work complete — done, changed, blocked, failed, or skipped. This writes the module audit and per-finding lifecycle atomically. For result=changed, changed_files, verification, and finding_dispositions are required.\n"+
				"command (%s): the live or final status of one Pulse final command — running, done, skipped, blocked, or failed. The combined Pulse finalizer marks each command running before work and then terminal immediately after it finishes.\n"+
				"Fix attempts are opened by the backend from the disposition itself; there is no separate attempt tool and no attempt_id to carry. A fixed_verified finding needs changed_files plus only passed post-change checks. changed_unverified needs an inconclusive check plus next_check naming the run, table, or artifact whose arrival settles it, and remains open awaiting that evidence; the next review verifies it against that evidence rather than re-attempting the fix. queued_for_engineering means a safe workflow repair exists but was deliberately not attempted in this pass; it requires next_check naming the next Engineering/Pulse pass and remains in Gate's active queue. external_action_required permanently removes a diagnosed real finding from Pulse's active queue and requires external_owner, reason_code, and reopen_condition; use it only when workflow tools cannot act. A failed check reopens the concern. awaiting_run is a real finding waiting only on a scheduled run to produce its evidence — no fix applied, nobody stuck — and requires next_check naming that run. Use it rather than blocked whenever the answer is \"the data does not exist yet\". blocked means there is genuinely no safe action at all; never use it merely because work was deferred, deprioritized, or not selected in this pass. awaiting_user requires human_input_id naming a still-pending create_human_input_request, so a finding cannot wait on a decision the operator was never asked for; escalate only when the goal does not already settle it and the cost of deciding is real, otherwise decide and record the reasoning. For strategic_review, proposal_only is accepted only with a concrete next_check evidence boundary; an actionable recommendation must use awaiting_user linked to a pending strategic_review decision, while safe technical prerequisites use the normal Fixer lifecycle. before_refs and after_refs are paired agent-supplied audit references; the backend preserves them but does not recompute arbitrary textual checks. The lifecycle is machine-validated for finding/module linkage and required evidence shape, not for the truth of an agent-authored verdict. Put exact technical failures in reason.",
				strings.Join(pulseModuleOrder, ", "), strings.Join(pulseFinalCommandOrder, ", ")),
			Parameters: llmtypes.NewParameters(map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"workspace_path": map[string]interface{}{"type": "string", "description": "Workflow-relative path, e.g. Workflow/social-media."},
					"pulse_run_id":   map[string]interface{}{"type": "string", "description": "Scheduler-provided Pulse run id."},
					"module":         map[string]interface{}{"type": "string", "enum": moduleEnum, "description": "The Pulse module this result belongs to. Pass this or command, never both."},
					"command":        map[string]interface{}{"type": "string", "enum": pulseFinalCommandOrder, "description": "The Pulse final command this status belongs to. Pass this or module, never both."},
					"result":         map[string]interface{}{"type": "string", "enum": pulseResultValues, "description": "For a module: done, changed, blocked, failed, or skipped. For a command: running, done, skipped, blocked, or failed."},
					"reason":         map[string]interface{}{"type": "string", "description": "One-sentence result summary."},
					"evidence":       map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}, "description": "Module results only."},
					"changed_files":  map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}, "description": "Exact workspace-relative files changed by this module. Required when result=changed; otherwise omit."},
					"verification":   map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}, "description": "Agent-reported checks performed and their observed outcomes. Required when result=changed; otherwise include only when useful. These records are audit evidence, not backend-executed proof."},
					"before_refs":    map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}, "description": "Optional agent-supplied pre-change hashes, versions, or cursors. When present, provide an equal-length after_refs audit pair."},
					"after_refs":     map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}, "description": "Optional agent-supplied post-change hashes, versions, or cursors paired positionally with before_refs."},
					"finding_dispositions": map[string]interface{}{
						"type":        "array",
						"description": "Per-finding outcome. Required for changed modules and whenever a reviewer returned trackable findings.",
						"items": map[string]interface{}{
							"type":                 "object",
							"additionalProperties": false,
							"properties": map[string]interface{}{
								"issue_id":         map[string]interface{}{"type": "string", "description": "The visible issue_id (for example PUL-DBA2B19E) from get_pulse_state(view=\"backlog\", detail=\"compact\"). This is the only finding identity to send; the backend resolves the internal fingerprint."},
								"disposition":      map[string]interface{}{"type": "string", "enum": []string{"fixed_verified", "verified_no_change", "changed_unverified", "proposal_only", "awaiting_user", "queued_for_engineering", "awaiting_run", "blocked", "external_action_required", "failed", "rejected"}},
								"summary":          map[string]interface{}{"type": "string"},
								"changed_files":    map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}},
								"before_refs":      map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}},
								"after_refs":       map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}},
								"next_check":       map[string]interface{}{"type": "string"},
								"external_owner":   map[string]interface{}{"type": "string", "enum": []string{"platform", "user", "vendor", "workflow_owner"}, "description": "Required for external_action_required."},
								"reason_code":      map[string]interface{}{"type": "string", "description": "Required stable reason such as missing_platform_tool, permission_boundary, vendor_issue, policy, or accepted_risk."},
								"reopen_condition": map[string]interface{}{"type": "string", "description": "Required evidence/capability/user boundary that makes this actionable again."},
								"human_input_id":   map[string]interface{}{"type": "string", "description": "Required for awaiting_user: the id returned by create_human_input_request for the decision this finding is waiting on. The request must exist and still be pending, so the operator has something real to answer."},
								"verification": map[string]interface{}{
									"type": "array",
									"items": map[string]interface{}{
										"type":                 "object",
										"additionalProperties": false,
										"properties": map[string]interface{}{
											"check":    map[string]interface{}{"type": "string"},
											"verdict":  map[string]interface{}{"type": "string", "enum": []string{"passed", "failed", "inconclusive"}},
											"expected": map[string]interface{}{"type": "string"},
											"observed": map[string]interface{}{"type": "string"},
											"evidence": map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}},
										},
										"required": []string{"check", "verdict"},
									},
								},
							},
							"required": []string{"issue_id", "disposition", "summary"},
						},
					},
				},
				"required": []string{"workspace_path", "pulse_run_id", "result", "reason"},
			}),
		},
	}
	impactTool, impactExecutor := createRecordPulseImpactTool()

	executors := map[string]interface{}{
		"record_pulse_finding": func(ctx context.Context, args map[string]interface{}) (string, error) {
			workspacePath, _ := args["workspace_path"].(string)
			pulseRunID, _ := args["pulse_run_id"].(string)
			pulseRunID = pulseRunIDForSession(ctx, pulseRunID)
			module, _ := args["module"].(string)
			if err := validatePulseToolRunID(ctx, pulseRunID); err != nil {
				return "", err
			}
			reproduction := step_based_workflow.PulseFindingReproduction{}
			if raw, ok := args["reproduction"].(map[string]interface{}); ok {
				reproduction.Safe, _ = raw["safe"].(bool)
				reproduction.Setup, _ = raw["setup"].(string)
				reproduction.Action, _ = raw["action"].(string)
				reproduction.Expected, _ = raw["expected"].(string)
				reproduction.Observed, _ = raw["observed"].(string)
				reproduction.Limitations, _ = raw["limitations"].(string)
			}
			input := step_based_workflow.PulseReviewFindingInput{IssueID: stringToolArg(args, "issue_id"), Concern: stringToolArg(args, "concern"), Module: module, HumanInputID: stringToolArg(args, "human_input_id"), PulseFindingDetails: step_based_workflow.PulseFindingDetails{
				TargetKey: stringToolArg(args, "target_key"), IssueKind: stringToolArg(args, "issue_kind"),
				RecommendedRoute: stringToolArg(args, "recommended_route"), NextCheck: stringToolArg(args, "next_check"), Classification: stringToolArg(args, "classification"),
				Severity: stringToolArg(args, "severity"), Summary: stringToolArg(args, "summary"), Impact: stringToolArg(args, "impact"), Workaround: stringToolArg(args, "workaround"),
				Evidence: stringSliceFromToolArg(args["evidence"]), Reproduction: reproduction,
			}}
			record, err := step_based_workflow.RecordPulseReviewFinding(ctx, workspacePath, pulseRunID, pulseRunID, input)
			if err != nil {
				return "", err
			}
			encoded, _ := json.Marshal(record)
			return string(encoded), nil
		},
		"merge_pulse_issues": func(ctx context.Context, args map[string]interface{}) (string, error) {
			workspacePath, _ := args["workspace_path"].(string)
			merged, err := step_based_workflow.MergePulseFindingIssues(ctx, workspacePath,
				stringToolArg(args, "canonical_issue_id"), stringSliceFromToolArg(args["duplicate_issue_ids"]), stringToolArg(args, "reason"))
			if err != nil {
				return "", err
			}
			payload, _ := json.Marshal(map[string]interface{}{"status": "merged", "merged_count": merged, "canonical_issue_id": stringToolArg(args, "canonical_issue_id")})
			return string(payload), nil
		},
		"record_pulse_verification": func(ctx context.Context, args map[string]interface{}) (string, error) {
			workspacePath, _ := args["workspace_path"].(string)
			pulseRunID, _ := args["pulse_run_id"].(string)
			pulseRunID = pulseRunIDForSession(ctx, pulseRunID)
			if err := validatePulseToolRunID(ctx, pulseRunID); err != nil {
				return "", err
			}
			candidate, err := step_based_workflow.ResolvePulseReviewVerificationIssueID(
				ctx, workspacePath, stringToolArg(args, "module"), stringToolArg(args, "issue_id"),
			)
			if err != nil {
				return "", err
			}
			verification := step_based_workflow.PulseReviewVerificationResult{
				FindingID: candidate.FindingID, Fingerprint: candidate.Fingerprint, AttemptID: candidate.AttemptID,
				Verdict: stringToolArg(args, "verdict"), Expected: stringToolArg(args, "expected"), Observed: stringToolArg(args, "observed"),
				Evidence: stringSliceFromToolArg(args["evidence"]), NextCheck: stringToolArg(args, "next_check"),
			}
			if err := step_based_workflow.RecordPulseReviewVerification(ctx, workspacePath, stringToolArg(args, "module"), pulseRunID, pulseRunID, verification); err != nil {
				return "", err
			}
			return `{"status":"recorded"}`, nil
		},
		"complete_pulse_review": func(ctx context.Context, args map[string]interface{}) (string, error) {
			workspacePath, _ := args["workspace_path"].(string)
			pulseRunID, _ := args["pulse_run_id"].(string)
			pulseRunID = pulseRunIDForSession(ctx, pulseRunID)
			verdict := strings.TrimSpace(stringToolArg(args, "verdict"))
			if verdict == "" {
				return "", fmt.Errorf("complete_pulse_review requires a non-empty verdict: summarize the overall judgment after recording findings and verifications")
			}
			modules := stringSliceFromToolArg(args["modules"])
			if err := validatePulseToolRunID(ctx, pulseRunID); err != nil {
				return "", err
			}
			if err := step_based_workflow.CompletePulseReview(ctx, workspacePath, modules, pulseRunID, pulseRunID, verdict, stringToolArg(args, "status")); err != nil {
				return "", err
			}
			return `{"status":"completed"}`, nil
		},
		"record_pulse_worklist": func(ctx context.Context, args map[string]interface{}) (string, error) {
			workspacePath, _ := args["workspace_path"].(string)
			pulseRunID, _ := args["pulse_run_id"].(string)
			pulseRunID = pulseRunIDForSession(ctx, pulseRunID)
			if err := validatePulseToolRunID(ctx, pulseRunID); err != nil {
				return "", err
			}
			decisions, err := pulseWorklistDecisionsFromArgs(args["decisions"])
			if err != nil {
				return "", err
			}
			normalized, err := normalizeReportHumanInputWorkspacePath(workspacePath)
			if err != nil {
				return "", err
			}
			// Capture the deterministic result before writing this Gate pass so
			// answer_not_applied is evaluated against the preceding completed
			// pass. The result is retained only for shadow comparison and is
			// never returned to the Gate that made these decisions.
			shadowResult := loopclosure.Check(ctx, normalized, time.Now().UTC())
			mode := stringToolArg(args, "mode")
			modeReason := stringToolArg(args, "mode_reason")
			states, err := recordPulseWorklistOnceWithShadowAndMode(ctx, normalized, pulseRunID, mode, modeReason, decisions, shadowResult)
			if err != nil {
				return "", err
			}
			payload, _ := json.Marshal(map[string]interface{}{"status": "recorded", "mode": mode, "mode_reason": modeReason, "modules": states})
			return string(payload), nil
		},
		"get_pulse_state": func(ctx context.Context, args map[string]interface{}) (string, error) {
			workspacePath, _ := args["workspace_path"].(string)
			view, _ := args["view"].(string)
			view = strings.ToLower(strings.TrimSpace(view))
			if strings.TrimSpace(workspacePath) == "" {
				// Every view opens the workflow's own db/db.sqlite. Without the
				// path there is no database to read, and the empty-result answer
				// reads as "nothing is tracked" rather than "you addressed nothing".
				return "", fmt.Errorf("get_pulse_state requires workspace_path: the workflow-relative path, e.g. Workflow/social-media")
			}
			switch view {
			case pulseStateViewModule:
				return readPulseModuleStateView(ctx, workspacePath, stringToolArg(args, "pulse_run_id"))
			case pulseStateViewBacklog:
				module, _ := args["module"].(string)
				return readPulseBacklogViewWithOptions(ctx, workspacePath, module, stringToolArg(args, "detail"), stringSliceFromToolArg(args["issue_ids"]))
			case pulseStateViewReview:
				module, _ := args["module"].(string)
				reviewRunID, _ := args["review_run_id"].(string)
				return readPulseReviewView(ctx, workspacePath, reviewRunID, module)
			default:
				return "", fmt.Errorf("view %q is not a valid Pulse state view. Must be one of: %s. Use %q for module cadence and the active concern backlog, %q for the durable finding backlog, and %q for one saved reviewer result (which also needs review_run_id and module)",
					view, strings.Join(pulseStateViewValues, ", "),
					pulseStateViewModule, pulseStateViewBacklog, pulseStateViewReview)
			}
		},
		"record_pulse_result": func(ctx context.Context, args map[string]interface{}) (string, error) {
			return recordPulseResultFromToolArgs(ctx, args)
		},
		"record_pulse_impact": impactExecutor,
	}
	categories := map[string]string{
		"record_pulse_finding":      "workflow",
		"record_pulse_verification": "workflow",
		"merge_pulse_issues":        "workflow",
		"complete_pulse_review":     "workflow",
		"record_pulse_worklist":     "workflow",
		"get_pulse_state":           "workflow",
		"record_pulse_result":       "workflow",
		"record_pulse_impact":       "workflow",
		"resolve_run_concern":       "workflow",
	}
	resolveConcernTool := llmtypes.Tool{
		Type: "function",
		Function: &llmtypes.FunctionDefinition{
			Name:        "resolve_run_concern",
			Description: "Acknowledge or reject a Pulse issue returned by get_pulse_state(view=\"backlog\"). Use rejected only when evidence shows it is not a problem; rejected concerns stay closed if the same behavior recurs. Use acknowledged when it is real but deliberately deferred. This tool cannot close a real bug as resolved: use record_pulse_result with a verified finding_disposition so changed files and test evidence are retained. Absence is never evidence of a fix.",
			Parameters: llmtypes.NewParameters(map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"workspace_path": map[string]interface{}{"type": "string", "description": "Workflow-relative path, e.g. Workflow/social-media."},
					"issue_id":       map[string]interface{}{"type": "string", "description": "The visible issue_id from get_pulse_state(view=\"backlog\", detail=\"compact\")."},
					"status":         map[string]interface{}{"type": "string", "enum": []string{"acknowledged", "rejected"}},
					"note":           map[string]interface{}{"type": "string", "description": "Short justification recorded with the judgement."},
				},
				"required": []string{"workspace_path", "issue_id", "status"},
			}),
		},
	}
	executors["resolve_run_concern"] = func(ctx context.Context, args map[string]interface{}) (string, error) {
		workspacePath, _ := args["workspace_path"].(string)
		issueID := stringToolArg(args, "issue_id")
		status, _ := args["status"].(string)
		note, _ := args["note"].(string)
		if strings.EqualFold(strings.TrimSpace(status), step_based_workflow.ConcernStatusResolved) {
			return "", fmt.Errorf("resolve_run_concern cannot close a real finding; use record_pulse_result with verified finding_dispositions")
		}
		finding, err := step_based_workflow.ResolvePulseFindingIssueID(ctx, workspacePath, issueID)
		if err != nil {
			return "", err
		}
		if err := step_based_workflow.ResolveRunConcern(ctx, workspacePath, finding.Fingerprint, status, "pulse", note); err != nil {
			return "", err
		}
		return fmt.Sprintf("Issue %s marked %s.", step_based_workflow.NewPulseIssue(finding).ID, status), nil
	}

	return []llmtypes.Tool{recordFindingTool, recordVerificationTool, completeReviewTool, mergeIssuesTool, recordTool, stateTool, resultTool, impactTool, resolveConcernTool}, executors, categories
}

func stringToolArg(args map[string]interface{}, key string) string {
	value, _ := args[key].(string)
	return strings.TrimSpace(value)
}

// The three reads get_pulse_state merges. One tool with a named view rather
// than three tools whose names shared no rule: the agent that invented
// close_pulse_fix_attempt and resolve_human_input was guessing across a surface
// larger than the number of concepts in it.
const (
	pulseStateViewModule  = "module"
	pulseStateViewBacklog = "backlog"
	pulseStateViewReview  = "review"
)

// pulseStateViewValues is the closed view set shared by the schema enum, the
// accept check, and the rejection message.
var pulseStateViewValues = []string{pulseStateViewBacklog, pulseStateViewModule, pulseStateViewReview}

// pulseResultValues is the union of the module and final-command result sets.
// Each target validates its own subset and names it on rejection; the schema
// enum is the union so a valid value is never rejected by the transport before
// the executor can explain which subset applies.
var pulseResultValues = []string{"running", "done", "changed", "skipped", "blocked", "failed"}

func readPulseModuleStateView(ctx context.Context, workspacePath, pulseRunID string) (string, error) {
	states, err := getPulseModuleStates(ctx, workspacePath)
	if err != nil {
		return "", err
	}
	// Workflow observations and accepted Pulse issues share one durable ledger,
	// but they are different queues. Gate needs both: issues are lifecycle work;
	// observations are evidence it may ask a reviewer to classify. Flattening
	// them made raw CONCERNS lines look like 100 confirmed bugs and sent Fixer
	// into broad rediscovery loops.
	lifecycles, concernsErr := step_based_workflow.LoadPulseFindingLifecycles(ctx, workspacePath, "", -1)
	if concernsErr != nil {
		log.Printf("[PULSE] get_pulse_state(view=module): open concerns unavailable for %s: %v", workspacePath, concernsErr)
	}
	issues, observations := step_based_workflow.SplitPulseFindingLifecycles(lifecycles)
	issues = activePulseFindingLifecycles(issues)
	observations = activePulseFindingLifecycles(observations)
	suppressedConcerns, suppressedErr := step_based_workflow.LoadExternallyOwnedRunConcerns(ctx, workspacePath)
	if suppressedErr != nil {
		log.Printf("[PULSE] get_pulse_state(view=module): suppressed concerns unavailable for %s: %v", workspacePath, suppressedErr)
	}
	// Plan changes whose knock-on effects have not been traced yet. The
	// changelog already records every plan-mod call and artifact_review
	// already stamps the ones that have been reconciled; nothing counted
	// the remainder, so Gate had to derive it from the files each run.
	planBacklog := step_based_workflow.CollectPlanChangeBacklog(workspacePath)
	// What each reviewer has actually been finding. Without this the choice
	// between modules is a guess: nothing distinguished a module that keeps
	// surfacing real breakage from one that returns clean every time.
	reviewHistory, historyErr := step_based_workflow.LoadModuleReviewHistory(ctx, workspacePath, 3)
	if historyErr != nil {
		log.Printf("[PULSE] get_pulse_state(view=module): review history unavailable for %s: %v", workspacePath, historyErr)
	}
	// This detector has already been validated against fleet state and
	// is supplied as a read-only fact feed, like open_concerns and
	// plan_change_backlog. Gate may weigh it but receives no mandatory
	// routing rule. A separate pre-decision snapshot is still retained
	// when Gate records its worklist so its handling remains auditable.
	loopClosure := loopclosure.Check(ctx, workspacePath, time.Now().UTC())
	impactLedger, impactErr := step_based_workflow.LoadPulseImpactLedger(ctx, workspacePath, 100)
	if impactErr != nil {
		log.Printf("[PULSE] get_pulse_state(view=module): impact ledger unavailable for %s: %v", workspacePath, impactErr)
	}
	var runMode *PulseRunMode
	if strings.TrimSpace(pulseRunID) != "" {
		var modeErr error
		runMode, modeErr = getPulseRunMode(ctx, workspacePath, pulseRunID)
		if modeErr != nil {
			return "", fmt.Errorf("read persisted Pulse mode: %w", modeErr)
		}
	}
	payload, _ := json.Marshal(map[string]interface{}{
		"modules": states,
		// Backward-compatible aliases now intentionally mean canonical issues.
		"open_concerns":              pulseLifecycleAgentProjection(issues, "issue_id"),
		"open_concern_count":         len(issues),
		"canonical_issues":           pulseLifecycleAgentProjection(issues, "issue_id"),
		"canonical_issue_count":      len(issues),
		"concerns_note":              "Complete active canonical issue backlog. These have been accepted by a reviewer or entered lifecycle work; they are not raw step emissions.",
		"workflow_observations":      pulseLifecycleAgentProjection(observations, "observation_id"),
		"workflow_observation_count": len(observations),
		"workflow_observations_note": "Unclassified evidence emitted by workflow steps. Gate may select a relevant cluster for review. A reviewer must link, reject, or promote it agentically; recurrence alone does not make it a bug. To promote one, submit its observation_id unchanged as issue_id to record_pulse_finding.",
		"suppressed_concerns":        pulseConcernAgentProjection(suppressedConcerns),
		"suppressed_concern_count":   len(suppressedConcerns),
		"suppressed_concerns_note":   "Diagnosed real findings owned outside this workflow. Do not report an unchanged fingerprint as a new finding or spend active review effort on it. A materially changed target/evidence creates or reopens an active finding; the recorded reopen condition explains the boundary.",
		"plan_change_backlog":        planBacklog,
		"loop_closure":               loopClosure,
		"loop_closure_note":          "Read-only deterministic evidence. Gate may weigh verified findings alongside other facts, but they do not mandate a module or authorize mutation. coverage_status must be verified before an empty findings list means clean.",
		"module_review_history":      reviewHistory,
		"review_history_note":        "What each reviewer concluded the last few times it ran, most recently run first. A module absent from this list has not run in the retained window at all. Use it to justify each skip: a module that keeps returning real findings is a poor candidate for another cooldown, and one that has come back clean repeatedly is a good one. A verdict here is the reviewer's conclusion, which is not the same as whether anything was then fixed.",
		"impact_ledger":              impactLedger,
		"impact_ledger_note":         "Durable intervention, per-run success-criterion observation, and append-only before/after assessment history. Reliability or measurement work is not direct goal progress; inconclusive is correct until a comparable evidence window matures.",
		"context_records":            loadPulseContextRecordsForState(ctx, workspacePath),
		"context_records_note":       "User-confirmed workflow rules captured through capture_context. The context file is the runtime source; these immutable records show who captured what and when.",
		"gate_mode":                  runMode,
		"gate_mode_note":             "The Gate-selected pass shape for the supplied pulse_run_id. Go records it but does not choose it; the following message sequence must follow it.",
	})
	return string(payload), nil
}

func loadPulseContextRecordsForState(ctx context.Context, workspacePath string) []step_based_workflow.PulseContextRecord {
	records, err := step_based_workflow.LoadPulseContextRecords(ctx, workspacePath, 100)
	if err != nil {
		log.Printf("[PULSE] get_pulse_state(view=module): context records unavailable for %s: %v", workspacePath, err)
		return []step_based_workflow.PulseContextRecord{}
	}
	return records
}

const maxPulseBacklogDetailIDs = 20

// readPulseBacklogView is retained as the compact-default entry point used by
// focused tests and internal callers. Agent tool calls flow through the options
// variant so a full lifecycle response is impossible without explicit ids.
func readPulseBacklogView(ctx context.Context, workspacePath, module string) (string, error) {
	return readPulseBacklogViewWithOptions(ctx, workspacePath, module, "compact", nil)
}

func readPulseBacklogViewWithOptions(ctx context.Context, workspacePath, module, detail string, issueIDs []string) (string, error) {
	module = normalizePulseModule(module)
	if module != "" && !validPulseModules[module] {
		return "", fmt.Errorf("module %q is not a valid Pulse module. Must be one of: %s. Omit module entirely to load the complete backlog", module, pulseModuleList())
	}
	detail = strings.ToLower(strings.TrimSpace(detail))
	if detail == "" {
		detail = "compact"
	}
	if detail != "compact" && detail != "full" {
		return "", fmt.Errorf("detail %q is not valid for get_pulse_state(view=\"backlog\"); use compact or full", detail)
	}
	issueIDs = normalizePulseIssueIDs(issueIDs)
	if detail == "compact" && len(issueIDs) > 0 {
		return "", fmt.Errorf("issue_ids is only valid with detail=\"full\"; read the compact index without ids first")
	}
	if detail == "full" {
		if len(issueIDs) == 0 {
			return "", fmt.Errorf("detail=\"full\" requires 1-%d issue_ids selected from the compact backlog index; broad full-history reads are intentionally disabled", maxPulseBacklogDetailIDs)
		}
		if len(issueIDs) > maxPulseBacklogDetailIDs {
			return "", fmt.Errorf("detail=\"full\" accepts at most %d issue_ids, got %d", maxPulseBacklogDetailIDs, len(issueIDs))
		}
	}
	findings, err := step_based_workflow.LoadPulseFindingLifecycles(ctx, workspacePath, module, -1)
	if err != nil {
		return "", err
	}
	issues, observations := step_based_workflow.SplitPulseFindingLifecycles(findings)
	if detail == "full" {
		selected, missing, ambiguous := selectPulseLifecyclesByIssueID(findings, issueIDs)
		if len(missing) > 0 {
			return "", fmt.Errorf("unknown issue_ids: %s; refresh get_pulse_state(view=\"backlog\", detail=\"compact\") and use exact ids", strings.Join(missing, ", "))
		}
		if len(ambiguous) > 0 {
			return "", fmt.Errorf("ambiguous issue_ids: %s; these public prefixes match multiple lifecycle rows and require backend identity repair before mutation", strings.Join(ambiguous, ", "))
		}
		payload, marshalErr := json.Marshal(map[string]interface{}{
			"detail":  "full",
			"records": pulseBacklogAgentProjection(selected),
			"total":   len(selected),
			"note":    "Complete lifecycle evidence for only the requested public PUL ids. No unrequested issue or observation history was loaded into the agent response.",
		})
		if marshalErr != nil {
			return "", marshalErr
		}
		return string(payload), nil
	}

	activeIssues := activePulseFindingLifecycles(issues)
	activeObservations := activePulseFindingLifecycles(observations)
	payload, marshalErr := json.Marshal(map[string]interface{}{
		"detail":            "compact",
		"issues":            pulseLifecycleAgentProjection(activeIssues, "issue_id"),
		"observations":      pulseLifecycleAgentProjection(activeObservations, "observation_id"),
		"total":             len(activeIssues),
		"issue_total":       len(activeIssues),
		"observation_total": len(activeObservations),
		"summary":           pulseBacklogSummary(issues, observations),
		"note":              "Bounded active index only: choose relevant public PUL ids here, then request detail=\"full\" for those ids. Canonical issues are the repair queue; observations remain reviewer evidence until linked, rejected, or promoted.",
	})
	if marshalErr != nil {
		return "", marshalErr
	}
	return string(payload), nil
}

func normalizePulseIssueIDs(issueIDs []string) []string {
	seen := make(map[string]bool, len(issueIDs))
	normalized := make([]string, 0, len(issueIDs))
	for _, issueID := range issueIDs {
		issueID = strings.ToUpper(strings.TrimSpace(issueID))
		if issueID == "" || seen[issueID] {
			continue
		}
		seen[issueID] = true
		normalized = append(normalized, issueID)
	}
	return normalized
}

func selectPulseLifecyclesByIssueID(findings []step_based_workflow.PulseFindingLifecycle, issueIDs []string) ([]step_based_workflow.PulseFindingLifecycle, []string, []string) {
	byID := make(map[string][]step_based_workflow.PulseFindingLifecycle, len(findings))
	for _, finding := range findings {
		issueID := strings.ToUpper(step_based_workflow.NewPulseIssue(finding).ID)
		byID[issueID] = append(byID[issueID], finding)
	}
	selected := make([]step_based_workflow.PulseFindingLifecycle, 0, len(issueIDs))
	missing := make([]string, 0)
	ambiguous := make([]string, 0)
	for _, issueID := range issueIDs {
		matches := byID[issueID]
		if len(matches) == 0 {
			missing = append(missing, issueID)
			continue
		}
		if len(matches) > 1 {
			ambiguous = append(ambiguous, issueID)
			continue
		}
		selected = append(selected, matches[0])
	}
	return selected, missing, ambiguous
}

// pulseBacklogSummary is intentionally derived from durable lifecycle rows,
// not a model receipt. It lets a reviewer and the consolidation command report
// whether the active backlog actually moved rather than celebrating activity
// while the number of unresolved root causes rises.
func pulseBacklogSummary(issues, observations []step_based_workflow.PulseFindingLifecycle) map[string]interface{} {
	byStatus := map[string]int{}
	active, merged := 0, 0
	for _, finding := range issues {
		status := strings.TrimSpace(finding.Status)
		if status == "" {
			status = "unknown"
		}
		byStatus[status]++
		if finding.Details != nil && strings.TrimSpace(finding.Details.MergedIntoIssueID) != "" {
			merged++
		}
		switch status {
		case step_based_workflow.ConcernStatusResolved, step_based_workflow.ConcernStatusRejected, step_based_workflow.ConcernStatusExternalActionRequired:
		default:
			active++
		}
	}
	observationByStatus := map[string]int{}
	activeObservations := 0
	for _, observation := range observations {
		status := strings.TrimSpace(observation.Status)
		if status == "" {
			status = "unknown"
		}
		observationByStatus[status]++
		if pulseFindingLifecycleActive(observation) {
			activeObservations++
		}
	}
	return map[string]interface{}{
		"active_count": active, "terminal_count": len(issues) - active,
		"merged_duplicate_count": merged, "by_status": byStatus,
		"active_observation_count":   activeObservations,
		"terminal_observation_count": len(observations) - activeObservations,
		"observations_by_status":     observationByStatus,
	}
}

func pulseFindingLifecycleActive(finding step_based_workflow.PulseFindingLifecycle) bool {
	switch strings.TrimSpace(finding.Status) {
	case step_based_workflow.ConcernStatusResolved, step_based_workflow.ConcernStatusRejected, step_based_workflow.ConcernStatusExternalActionRequired:
		return false
	default:
		return true
	}
}

func activePulseFindingLifecycles(findings []step_based_workflow.PulseFindingLifecycle) []step_based_workflow.PulseFindingLifecycle {
	active := make([]step_based_workflow.PulseFindingLifecycle, 0, len(findings))
	for _, finding := range findings {
		if pulseFindingLifecycleActive(finding) {
			active = append(active, finding)
		}
	}
	return active
}

func pulseLifecycleAgentProjection(findings []step_based_workflow.PulseFindingLifecycle, idKey string) []map[string]interface{} {
	projected := make([]map[string]interface{}, 0, len(findings))
	for _, finding := range findings {
		issue := step_based_workflow.NewPulseIssue(finding)
		card := map[string]interface{}{
			idKey: issue.ID, "kind": finding.Kind, "step_id": finding.StepID,
			"phase": finding.Phase, "group_name": finding.GroupName, "summary": issue.Title,
			"first_seen_run": finding.FirstSeenRun, "first_seen_at": finding.FirstSeenAt,
			"last_seen_run": finding.LastSeenRun, "last_seen_at": finding.LastSeenAt,
			"seen_count": finding.SeenCount, "status": finding.Status, "priority": issue.Priority,
			"module": issue.Module, "repair_eligible": finding.Kind == step_based_workflow.PulseFindingKindIssue && issue.Status == "backlog",
		}
		if finding.Details != nil {
			card["target_key"] = finding.Details.TargetKey
			card["next_check"] = finding.Details.NextCheck
		}
		if len(finding.Verification) > 0 {
			latest := finding.Verification[0]
			card["latest_verification"] = map[string]interface{}{
				"verdict": latest.Verdict, "check": latest.Check, "verified_at": latest.At,
			}
		}
		projected = append(projected, card)
	}
	return projected
}

// pulseConcernAgentProjection keeps the Gate's issue feed on the same public
// identity contract as the detailed backlog. A raw fingerprint is a database
// join key, not an instruction for an LLM to copy into a later write.
func pulseConcernAgentProjection(concerns []step_based_workflow.RunConcern) []map[string]interface{} {
	projected := make([]map[string]interface{}, 0, len(concerns))
	for _, concern := range concerns {
		issue := step_based_workflow.NewPulseIssue(step_based_workflow.PulseFindingLifecycle{
			Fingerprint: concern.Fingerprint, StepID: concern.StepID, Phase: concern.Phase,
			GroupName: concern.GroupName, Text: concern.Text, FirstSeenRun: concern.FirstSeenRun,
			FirstSeenAt: concern.FirstSeenAt, LastSeenRun: concern.LastSeenRun, LastSeenAt: concern.LastSeenAt,
			SeenCount: concern.SeenCount, Status: concern.Status,
		})
		projected = append(projected, map[string]interface{}{
			"issue_id": issue.ID, "step_id": concern.StepID, "phase": concern.Phase,
			"group_name": concern.GroupName, "text": concern.Text, "first_seen_run": concern.FirstSeenRun,
			"first_seen_at": concern.FirstSeenAt, "last_seen_run": concern.LastSeenRun,
			"last_seen_at": concern.LastSeenAt, "seen_count": concern.SeenCount, "status": concern.Status,
			"resolution_note": concern.ResolutionNote,
		})
	}
	return projected
}

// pulseBacklogAgentProjection removes lifecycle implementation keys from the
// coding-agent tool response. The REST/UI projection can retain them as stable
// React keys while the agent receives only the one public PUL issue identity.
func pulseBacklogAgentProjection(findings []step_based_workflow.PulseFindingLifecycle) interface{} {
	raw, err := json.Marshal(findings)
	if err != nil {
		return []interface{}{}
	}
	var projected interface{}
	if err := json.Unmarshal(raw, &projected); err != nil {
		return []interface{}{}
	}
	var redact func(interface{})
	redact = func(value interface{}) {
		switch typed := value.(type) {
		case map[string]interface{}:
			delete(typed, "fingerprint")
			delete(typed, "attempt_id")
			delete(typed, "finding_id")
			for _, nested := range typed {
				redact(nested)
			}
		case []interface{}:
			for _, nested := range typed {
				redact(nested)
			}
		}
	}
	redact(projected)
	return projected
}

func readPulseReviewView(ctx context.Context, workspacePath, reviewRunID, module string) (string, error) {
	reviewRunID = strings.TrimSpace(reviewRunID)
	module = strings.TrimSpace(module)
	if reviewRunID == "" || module == "" {
		return "", fmt.Errorf("get_pulse_state(view=%q) requires both review_run_id and module (got %s); use the pair exactly as reported by the call_generic_agent completion notification",
			pulseStateViewReview,
			pulseArgArrivalReport("review_run_id", reviewRunID, "module", module))
	}
	if err := step_based_workflow.ValidatePulseReviewIdentity(reviewRunID, module); err != nil {
		return "", err
	}
	receipt, err := step_based_workflow.LoadPulseReviewReceiptForRun(ctx, workspacePath, reviewRunID, module)
	if errors.Is(err, sql.ErrNoRows) {
		// Absence is the expected answer here, not a fault. The review stage
		// prompt (scheduler.go) tells a reviewer to reconcile "any already-saved
		// SQLite result before discovery" so it does not launch a duplicate — on
		// a fresh run that pre-check must miss, because the caller is the thing
		// that will write the row.
		//
		// The previous wording offered "or this identity pair is wrong" as a
		// co-equal explanation. ValidatePulseReviewIdentity ran immediately
		// above and passed, so the code had already disproved that: the format
		// is valid and the module is in the canonical registry. Naming it anyway
		// sent reviewers hunting for a different id — 8 of these on 2026-08-04
		// across 3 sessions, on identities that were correct and seconds old.
		//
		// So: state the normal case first, and do not offer a cause this
		// function has already ruled out.
		return "", fmt.Errorf(
			"no Pulse review is saved for review_run_id=%q module=%q yet. This identity is well-formed and the module is valid — "+
				"do not look for a different review_run_id. "+
				"If you are the reviewer for this run, nothing is saved until you save it: this is the expected result of the "+
				"pre-discovery check, so proceed with discovery and record your result. "+
				"If you are waiting on another stage's reviewer, it has not finished — resume from its completion notification "+
				"rather than polling",
			reviewRunID, module,
		)
	}
	if err != nil {
		return "", err
	}
	payload, err := json.Marshal(map[string]interface{}{
		"module":             receipt.Module,
		"review_run_id":      receipt.ReviewRunID,
		"pulse_run_id":       receipt.PulseRunID,
		"status":             receipt.Status,
		"verdict":            receipt.Verdict,
		"finding_count":      receipt.FindingCount,
		"verification_count": receipt.VerificationCount,
		"verifications":      receipt.Verifications,
		"note":               "Review findings are stored only in the structured lifecycle backlog; load view=module or view=backlog for actionable issues.",
	})
	if err != nil {
		return "", err
	}
	return string(payload), nil
}

// pulseArgArrivalReport names what actually arrived for each of a pair of
// required arguments. "requires both" alone does not say which one was missing.
func pulseArgArrivalReport(nameA, valueA, nameB, valueB string) string {
	state := func(value string) string {
		if strings.TrimSpace(value) == "" {
			return "missing"
		}
		return "set"
	}
	return fmt.Sprintf("%s=%s, %s=%s", nameA, state(valueA), nameB, state(valueB))
}

// recordPulseResultFromToolArgs writes one Pulse outcome. Exactly one of module
// or command selects which durable record is being written; the two used to be
// separate tools whose names differed by more than the concept did.
func recordPulseResultFromToolArgs(ctx context.Context, args map[string]interface{}) (string, error) {
	workspacePath, _ := args["workspace_path"].(string)
	pulseRunID, _ := args["pulse_run_id"].(string)
	pulseRunID = pulseRunIDForSession(ctx, pulseRunID)
	module, _ := args["module"].(string)
	command, _ := args["command"].(string)
	result, _ := args["result"].(string)
	reason, _ := args["reason"].(string)
	module = strings.TrimSpace(module)
	command = strings.TrimSpace(command)
	if (module == "") == (command == "") {
		return "", fmt.Errorf("record_pulse_result requires exactly one of module or command (got %s). Pass module (one of: %s) to record a Pulse module's terminal result, or command (one of: %s) to record a Pulse final command's status",
			pulseArgArrivalReport("module", module, "command", command),
			pulseModuleList(), strings.Join(pulseFinalCommandOrder, ", "))
	}
	if err := validatePulseToolRunID(ctx, pulseRunID); err != nil {
		return "", err
	}
	if command != "" {
		normalizedResult := strings.ToLower(strings.TrimSpace(result))
		if !pulseValueIsOneOf(normalizedResult, pulseFinalCommandResultValues) {
			return "", fmt.Errorf("result %q is not valid for command %q. Must be one of: %s. Mark a command running before its work and terminal immediately after it finishes",
				result, command, strings.Join(pulseFinalCommandResultValues, ", "))
		}
		state, err := markPulseFinalCommandStateFromAgent(ctx, workspacePath, command, pulseRunID, normalizedResult, reason)
		if err != nil {
			return "", err
		}
		payload, _ := json.Marshal(map[string]interface{}{"status": "updated", "command": state})
		return string(payload), nil
	}

	if !pulseValueIsOneOf(strings.ToLower(strings.TrimSpace(result)), pulseModuleResultValues) {
		return "", fmt.Errorf("result %q is not valid for module %q. Must be one of: %s. Use changed only when files were modified, and pair it with changed_files, verification, and finding_dispositions",
			result, module, strings.Join(pulseModuleResultValues, ", "))
	}
	audit := PulseModuleAuditInput{
		ChangedFiles: stringSliceFromToolArg(args["changed_files"]),
		Verification: stringSliceFromToolArg(args["verification"]),
		BeforeRefs:   stringSliceFromToolArg(args["before_refs"]),
		AfterRefs:    stringSliceFromToolArg(args["after_refs"]),
	}
	if len(audit.BeforeRefs) != len(audit.AfterRefs) {
		return "", fmt.Errorf("before_refs and after_refs must be equal-length positional audit pairs (got before_refs=%d, after_refs=%d); supply the matching after_ref for each before_ref, or omit both arrays",
			len(audit.BeforeRefs), len(audit.AfterRefs))
	}
	dispositions, err := pulseFindingDispositionsFromToolArg(args["finding_dispositions"])
	if err != nil {
		return "", err
	}
	dispositions, err = step_based_workflow.ResolvePulseFindingDispositionIssueIDs(ctx, workspacePath, dispositions)
	if err != nil {
		return "", err
	}
	reviewerVerifications, err := step_based_workflow.LoadPulseReviewVerificationsForPulseRun(
		ctx, workspacePath, pulseRunID, module,
	)
	if err != nil {
		return "", fmt.Errorf("load structured reviewer verifications: %w", err)
	}
	if err := validateReviewerVerificationDispositions(reviewerVerifications, dispositions); err != nil {
		return "", err
	}
	if strings.EqualFold(strings.TrimSpace(result), "changed") {
		// One rejection per missing array taught the caller the contract one
		// failed write at a time. Name the whole required set and what
		// actually arrived, so a single retry can satisfy all three.
		changedRequirement := fmt.Sprintf("result=changed requires changed_files, verification, and finding_dispositions together (got changed_files=%d items, verification=%d items, finding_dispositions=%d items)",
			len(audit.ChangedFiles), len(audit.Verification), len(dispositions))
		if len(audit.ChangedFiles) == 0 {
			return "", fmt.Errorf("changed_files is required when result=changed; %s. changed_files lists the exact workspace-relative files this module changed", changedRequirement)
		}
		if len(audit.Verification) == 0 {
			return "", fmt.Errorf("verification is required when result=changed; %s. verification lists the checks performed and their observed outcomes", changedRequirement)
		}
		if len(dispositions) == 0 {
			return "", fmt.Errorf("finding_dispositions is required when result=changed; %s. %s", changedRequirement, pulseFindingDispositionsShape)
		}
	}
	state, err := markPulseModuleResultFromAgentWithAuditAndFindings(
		ctx, workspacePath, module, pulseRunID, result, reason,
		stringSliceFromToolArg(args["evidence"]), audit, dispositions,
	)
	if err != nil {
		return "", err
	}
	payload, _ := json.Marshal(map[string]interface{}{"status": "updated", "module": state})
	return string(payload), nil
}

func pulseValueIsOneOf(value string, allowed []string) bool {
	for _, candidate := range allowed {
		if value == candidate {
			return true
		}
	}
	return false
}

// pulseFindingDispositionsShape is appended to decode failures. A raw
// json.Unmarshal message names a Go struct field the agent has never seen and
// says nothing about the shape that would have worked, so a mistyped payload
// cannot be corrected from the rejection alone.
const pulseFindingDispositionsShape = `finding_dispositions takes an array of disposition objects, not an object or a string: ` +
	`[{"issue_id": "<backlog issue_id>", "disposition": "fixed_verified", "summary": "<one sentence>", ` +
	`"changed_files": ["path/to/file"], ` +
	`"verification": [{"check": "<what was run>", "verdict": "passed", "expected": "<expected>", "observed": "<observed>"}]}]`

type pulseFindingDispositionToolArg struct {
	IssueID         string                                         `json:"issue_id"`
	Disposition     string                                         `json:"disposition"`
	Summary         string                                         `json:"summary"`
	ChangedFiles    []string                                       `json:"changed_files,omitempty"`
	BeforeRefs      []string                                       `json:"before_refs,omitempty"`
	AfterRefs       []string                                       `json:"after_refs,omitempty"`
	NextCheck       string                                         `json:"next_check,omitempty"`
	ExternalOwner   string                                         `json:"external_owner,omitempty"`
	ReasonCode      string                                         `json:"reason_code,omitempty"`
	ReopenCondition string                                         `json:"reopen_condition,omitempty"`
	HumanInputID    string                                         `json:"human_input_id,omitempty"`
	Verification    []step_based_workflow.PulseFindingVerification `json:"verification,omitempty"`
}

func pulseFindingDispositionsFromToolArg(raw interface{}) ([]step_based_workflow.PulseFindingDisposition, error) {
	if raw == nil {
		return nil, nil
	}
	encoded, err := json.Marshal(raw)
	if err != nil {
		return nil, fmt.Errorf("encode finding_dispositions (%s): %w", pulseFindingDispositionsShape, err)
	}
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	var toolArgs []pulseFindingDispositionToolArg
	if err := decoder.Decode(&toolArgs); err != nil {
		return nil, fmt.Errorf("decode finding_dispositions (%s): %w", pulseFindingDispositionsShape, err)
	}
	dispositions := make([]step_based_workflow.PulseFindingDisposition, len(toolArgs))
	for index, input := range toolArgs {
		dispositions[index] = step_based_workflow.NormalizePulseFindingDisposition(step_based_workflow.PulseFindingDisposition{
			FindingID:       input.IssueID,
			Disposition:     input.Disposition,
			Summary:         input.Summary,
			ChangedFiles:    input.ChangedFiles,
			BeforeRefs:      input.BeforeRefs,
			AfterRefs:       input.AfterRefs,
			NextCheck:       input.NextCheck,
			ExternalOwner:   input.ExternalOwner,
			ReasonCode:      input.ReasonCode,
			ReopenCondition: input.ReopenCondition,
			HumanInputID:    input.HumanInputID,
			Verification:    input.Verification,
		})
	}
	return dispositions, nil
}

// validateReviewerVerificationDispositions reports every mismatch between the
// saved reviewer verdicts and the submitted dispositions in one pass, not one
// mismatch per call.
//
// It used to return on the first mismatch within the first review. On
// 2026-08-04 finding PUL-70B1057E took three separate record_pulse_result
// rejections to get through this single function alone — wrong disposition
// value, then a verification proof that didn't match the reviewer's evidence,
// then a next_check that didn't match the reviewer's boundary text — because
// each rejection only revealed the next check once the previous one was fixed.
// All three checks read independent fields of the same matched disposition, so
// none of them needs the others to have passed first; they can all run and all
// report in one message. validateFindingDisposition got the same fix for the
// structural checks that follow this one in the same call.
func validateReviewerVerificationDispositions(
	reviews []step_based_workflow.PulseReviewVerificationResult,
	dispositions []step_based_workflow.PulseFindingDisposition,
) error {
	var messages []string
	for _, review := range reviews {
		var matched *step_based_workflow.PulseFindingDisposition
		for index := range dispositions {
			candidate := &dispositions[index]
			// The reviewer names the attempt it checked, but the fixer no longer
			// carries an attempt_id: the backend resolves the module's own open
			// attempt for this finding. Match on the identity the fixer does hold,
			// and only compare attempt_id when one was supplied anyway.
			if candidate.FindingID != review.FindingID || candidate.Fingerprint != review.Fingerprint {
				continue
			}
			if candidate.AttemptID != "" && candidate.AttemptID != review.AttemptID {
				continue
			}
			matched = candidate
			break
		}
		if matched == nil {
			messages = append(messages, fmt.Sprintf("reviewer verification for issue %q requires a matching issue_id disposition before the module can be terminal", review.FindingID))
			continue
		}

		var findingProblems []string
		wantDisposition := map[string]string{
			step_based_workflow.VerificationPassed:       step_based_workflow.FindingDispositionFixedVerified,
			step_based_workflow.VerificationFailed:       step_based_workflow.FindingDispositionFailed,
			step_based_workflow.VerificationInconclusive: step_based_workflow.FindingDispositionChangedUnverified,
		}[review.Verdict]
		if matched.Disposition != wantDisposition {
			findingProblems = append(findingProblems, fmt.Sprintf("reviewer verification %s requires disposition %q, got %q", review.Verdict, wantDisposition, matched.Disposition))
		}
		verificationMatched := false
		for _, proof := range matched.Verification {
			if proof.Verdict == review.Verdict && strings.TrimSpace(proof.Expected) == review.Expected && strings.TrimSpace(proof.Observed) == review.Observed {
				verificationMatched = true
				break
			}
		}
		if !verificationMatched {
			findingProblems = append(findingProblems, fmt.Sprintf("disposition must carry the reviewer's structured %s proof with identical expected and observed evidence", review.Verdict))
		}
		if review.Verdict == step_based_workflow.VerificationInconclusive && strings.TrimSpace(matched.NextCheck) != review.NextCheck {
			findingProblems = append(findingProblems, fmt.Sprintf("inconclusive disposition next_check must match the reviewer boundary %q", review.NextCheck))
		}
		if len(findingProblems) > 0 {
			messages = append(messages, step_based_workflow.FormatPulseDispositionProblems(review.FindingID, findingProblems).Error())
		}
	}
	if len(messages) == 0 {
		return nil
	}
	if len(messages) == 1 {
		return errors.New(messages[0])
	}
	return fmt.Errorf("%d findings failed reviewer-verification cross-check:\n%s", len(messages), strings.Join(messages, "\n"))
}

// pulseDecisionFields is the complete decision contract in schema order. It
// backs both the unknown-field check and the message that reports it, so a
// field can never be silently accepted without appearing in the list an agent
// is shown.
var pulseDecisionFields = []string{
	"module", "due", "reason", "evidence",
	"next_check_at", "next_check_after_run_id", "cooldown_runs",
}

// pulseDecisionFieldAliases maps the wrong names agents actually reach for onto
// the field they meant. "decision" carrying the due/skip verdict is the observed
// case; without the pointer the rejection only says a field is unknown, which
// does not tell the caller which of the seven it should have used.
var pulseDecisionFieldAliases = map[string]string{
	"decision": "due", "decisions": "due", "is_due": "due", "status": "due",
	"verdict": "due", "state": "due", "rationale": "reason", "justification": "reason",
	"why": "reason", "notes": "reason", "note": "reason", "proof": "evidence",
	"cooldown": "cooldown_runs", "next_check": "next_check_at",
}

func pulseDecisionFieldSuggestion(key string) string {
	if suggestion, ok := pulseDecisionFieldAliases[strings.ToLower(strings.TrimSpace(key))]; ok {
		return suggestion
	}
	// A near-miss spelling of a real field is still recoverable, but only if the
	// rejection says which field it was close to.
	normalized := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(key), "-", "_"))
	for _, field := range pulseDecisionFields {
		if normalized == field || strings.HasPrefix(field, normalized) || strings.HasPrefix(normalized, field) {
			return field
		}
	}
	return ""
}

func pulseWorklistDecisionsFromArgs(raw interface{}) ([]PulseWorklistDecision, error) {
	arr, ok := raw.([]interface{})
	if !ok {
		return nil, fmt.Errorf("decisions must be an array with one object per Pulse module: [{\"module\": %q, \"due\": true, \"reason\": \"<why>\"}]; module, due (boolean), and reason are required on every entry",
			pulseModuleExample())
	}
	out := make([]PulseWorklistDecision, 0, len(arr))
	allowed := map[string]bool{}
	for _, field := range pulseDecisionFields {
		allowed[field] = true
	}
	for index, item := range arr {
		m, ok := item.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("decisions[%d] must be an object shaped {\"module\": %q, \"due\": true, \"reason\": \"<why>\"}, got %T",
				index, pulseModuleExample(), item)
		}
		for key := range m {
			if !allowed[key] {
				message := fmt.Sprintf("decisions[%d] contains unknown field %q. Allowed fields: %s; module, due, and reason are required",
					index, key, strings.Join(pulseDecisionFields, ", "))
				if suggestion := pulseDecisionFieldSuggestion(key); suggestion != "" {
					if suggestion == "due" {
						message += fmt.Sprintf("; use the required boolean field %s", suggestion)
					} else {
						message += fmt.Sprintf("; did you mean %q?", suggestion)
					}
				}
				return nil, fmt.Errorf("%s", message)
			}
		}
		decision := PulseWorklistDecision{}
		var err error
		if decision.Module, err = requiredStringToolArg(m, "module", index); err != nil {
			return nil, err
		}
		if decision.Due, ok = m["due"].(bool); !ok {
			return nil, fmt.Errorf("decisions[%d].due is required and must be boolean", index)
		}
		if decision.Reason, err = requiredStringToolArg(m, "reason", index); err != nil {
			return nil, err
		}
		if rawEvidence, exists := m["evidence"]; exists {
			if decision.Evidence, err = strictStringSliceToolArg(rawEvidence); err != nil {
				return nil, fmt.Errorf("decisions[%d].evidence: %w", index, err)
			}
		}
		if decision.NextCheckAt, err = optionalStringToolArg(m, "next_check_at", index); err != nil {
			return nil, err
		}
		if decision.NextCheckAfterRunID, err = optionalStringToolArg(m, "next_check_after_run_id", index); err != nil {
			return nil, err
		}
		if rawCooldown, exists := m["cooldown_runs"]; exists {
			if decision.CooldownRuns, err = strictIntToolArg(rawCooldown); err != nil {
				return nil, fmt.Errorf("decisions[%d].cooldown_runs: %w", index, err)
			}
		}
		out = append(out, decision)
	}
	return out, nil
}

func requiredStringToolArg(m map[string]interface{}, key string, index int) (string, error) {
	value, ok := m[key].(string)
	if !ok || strings.TrimSpace(value) == "" {
		return "", fmt.Errorf("decisions[%d].%s is required and must be a non-empty string (got %s); every decision entry needs module, due, and reason",
			index, key, pulseArgTypeName(m[key]))
	}
	return value, nil
}

func optionalStringToolArg(m map[string]interface{}, key string, index int) (string, error) {
	raw, exists := m[key]
	if !exists {
		return "", nil
	}
	value, ok := raw.(string)
	if !ok {
		return "", fmt.Errorf("decisions[%d].%s must be a string (got %s)", index, key, pulseArgTypeName(raw))
	}
	return value, nil
}

// pulseArgTypeName describes what actually arrived for a tool argument. "must
// be a string" alone does not say whether the caller sent a number, an object,
// or nothing at all.
func pulseArgTypeName(raw interface{}) string {
	switch value := raw.(type) {
	case nil:
		return "nothing"
	case string:
		if strings.TrimSpace(value) == "" {
			return "an empty string"
		}
		return "a string"
	case bool:
		return "a boolean"
	case float64, int, int64:
		return "a number"
	case []interface{}:
		return "an array"
	case map[string]interface{}:
		return "an object"
	default:
		return fmt.Sprintf("%T", raw)
	}
}

func strictStringSliceToolArg(raw interface{}) ([]string, error) {
	arr, ok := raw.([]interface{})
	if !ok {
		return nil, fmt.Errorf("must be an array of strings, got %s", pulseArgTypeName(raw))
	}
	out := make([]string, 0, len(arr))
	for index, item := range arr {
		value, ok := item.(string)
		if !ok {
			return nil, fmt.Errorf("item %d must be a string, got %s", index, pulseArgTypeName(item))
		}
		if value = strings.TrimSpace(value); value != "" {
			out = append(out, value)
		}
	}
	return out, nil
}

func strictIntToolArg(raw interface{}) (int, error) {
	switch value := raw.(type) {
	case int:
		return value, nil
	case int64:
		return int(value), nil
	case float64:
		integer := int(value)
		if float64(integer) != value {
			return 0, fmt.Errorf("must be a whole number of runs, got the fractional value %v", value)
		}
		return integer, nil
	default:
		return 0, fmt.Errorf("must be a non-negative integer number of runs, got %s", pulseArgTypeName(raw))
	}
}

func stringSliceFromToolArg(raw interface{}) []string {
	if values, ok := raw.([]string); ok {
		return normalizePulseEvidence(values)
	}
	arr, ok := raw.([]interface{})
	if !ok {
		return nil
	}
	out := make([]string, 0, len(arr))
	for _, item := range arr {
		if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
			out = append(out, strings.TrimSpace(s))
		}
	}
	return out
}

func normalizePulseModule(module string) string {
	return pulsemodules.Normalize(module)
}

func normalizePulseEvidence(evidence []string) []string {
	out := make([]string, 0, len(evidence))
	seen := map[string]bool{}
	for _, item := range evidence {
		item = strings.TrimSpace(item)
		if item == "" || seen[item] {
			continue
		}
		seen[item] = true
		out = append(out, item)
	}
	return out
}
