package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/loopclosure"
	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/orchestrator/agents/workflow/step_based_workflow"
	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/pulsemodules"
	mcpexecutor "github.com/manishiitg/mcpagent/executor"
	"github.com/manishiitg/multi-llm-provider-go/llmtypes"
	htmlpkg "golang.org/x/net/html"
)

const (
	pulseModuleWorkflowReview = pulsemodules.WorkflowReviewID
	// Historical operational identities remain available to legacy read paths.
	// Current worklists and writers use workflow_review instead.
	pulseModuleBugReview      = pulsemodules.BugReviewID
	pulseModuleArtifactReview = pulsemodules.ArtifactReviewID
	pulseModuleReportHealth   = pulsemodules.ReportHealthID
	pulseModuleEvalHealth     = pulsemodules.EvalHealthID
	// pulseModuleStoresHealth replaces the former separate learning_health,
	// knowledgebase_health, and db_health modules. All three shared the same
	// due-cadence mechanism (Reviewed-baseline rule, no special throttling),
	// the same freshness-recency check pattern, the same plan_change_backlog
	// trigger, and the same bounded-fix authority — mechanically identical,
	// only the content domain (learnings HOW / KB facts / DB schema) differed.
	// One due-decision and one Fixer pass now covers all three, each with its
	// own small checklist inside.
	pulseModuleStoresHealth = pulsemodules.StoresHealthID
	// pulseModuleLLMOpsReview owns cost, timing, tool-call operations,
	// model/tier/catalog review, and plan-design hygiene (step-type fitness,
	// prevalidation fitness, schema/description drift). These checks need one
	// agentic judgment pass over the same runtime and goal evidence.
	pulseModuleLLMOpsReview = pulsemodules.LLMOpsReviewID
	// pulseModuleStrategyAuditor owns read-only improvement of the current
	// strategy across retained runs. Goal Advisor independently explores
	// materially different approaches outside the current plan.
	pulseModuleStrategyAuditor = pulsemodules.StrategyAuditorID
	pulseModuleGoalAdvisor     = pulsemodules.GoalAdvisorID
)

// Derived from the canonical registry — see pkg/pulsemodules. Do not restate
// the module set here; a hand-maintained second copy is exactly what caused
// the 2026-07-29 desync.
var pulseModuleOrder = pulsemodules.IDs()

// maxPulseGateDueModules is a per-Pulse-pass cost cap. The Gate still records
// every lens' cadence decision, but may select only this many expensive review
// perspectives for execution; the rest remain explicitly deferred and due
// again at their recorded next-check boundary.
const maxPulseGateDueModules = 2

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
	return "bug_review"
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
		pulseShadowSignalObservationSchema,
		pulseFinalCommandStateSchema,
	}
	for _, stmt := range stmts {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}
	if err := ensurePulseModuleStateColumns(ctx, db); err != nil {
		return err
	}
	stmts = []string{
		`CREATE INDEX IF NOT EXISTS idx_pulse_module_state_run ON pulse_module_state(last_pulse_run_id, last_decision)`,
		`CREATE INDEX IF NOT EXISTS idx_pulse_module_audit_recorded ON pulse_module_audit(workspace_path, recorded_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_pulse_shadow_signal_observed ON pulse_shadow_signal_observation(workspace_path, observed_at DESC)`,
	}
	for _, stmt := range stmts {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}
	return nil
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
	pulseRunID = strings.TrimSpace(pulseRunID)
	if pulseRunID == "" {
		return nil, fmt.Errorf("pulse_run_id is required: pass the scheduler-provided Pulse run id exactly as it appears in the prompt")
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

// recordStandalonePulseFixerModules opens only the explicitly selected modules
// for a manual /pulse-fixer run. Unlike Gate's complete worklist write, it does
// not rewrite cadence or results for unrelated modules.
func recordStandalonePulseFixerModules(ctx context.Context, workspacePath, pulseRunID string, modules []string) ([]PulseModuleState, error) {
	pulseWorklistRecordMu.Lock()
	defer pulseWorklistRecordMu.Unlock()

	pulseRunID = strings.TrimSpace(pulseRunID)
	if pulseRunID == "" {
		return nil, fmt.Errorf("pulse_run_id is required: pass the scheduler-provided Pulse run id exactly as it appears in the prompt")
	}
	if len(modules) == 0 {
		return nil, fmt.Errorf("modules must not be empty; pass at least one module from: %s", pulseModuleList())
	}
	normalized, db, err := openPulseModuleStateDB(ctx, workspacePath, true)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	seen := map[string]bool{}
	canonical := make([]string, 0, len(modules))
	for _, raw := range modules {
		module := normalizePulseModule(raw)
		if !validPulseModules[module] {
			return nil, fmt.Errorf("module %q is not a valid Pulse module. Must be one of: %s", raw, pulseModuleList())
		}
		if seen[module] {
			return nil, fmt.Errorf("module %q appears more than once", module)
		}
		seen[module] = true
		canonical = append(canonical, module)
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	for _, module := range canonical {
		var activeRunID, decision, result string
		err := tx.QueryRowContext(ctx, `SELECT last_pulse_run_id, last_decision, last_result
			FROM pulse_module_state WHERE workspace_path = ? AND module = ?`, normalized, module).
			Scan(&activeRunID, &decision, &result)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
		// Refuse only a pass that is still running. An unresolved claim whose
		// run no longer holds authority was abandoned, and leaving it
		// untouchable strands the module permanently rather than protecting
		// anything.
		if err == nil && decision == "due" && strings.TrimSpace(result) == "" && activeRunID != pulseRunID {
			if isTrustedPulseRunLive(activeRunID) {
				return nil, fmt.Errorf("module %q already belongs to unresolved Pulse run %q", module, activeRunID)
			}
			log.Printf("[PULSE] standalone fixer taking over module %q from abandoned Pulse run %q", module, activeRunID)
		}
	}

	now := time.Now().UTC().Format(time.RFC3339)
	evidenceJSON, _ := json.Marshal([]string{"explicit standalone /pulse-fixer backlog drain"})
	for _, module := range canonical {
		_, err := tx.ExecContext(ctx, `INSERT INTO pulse_module_state (
				module, workspace_path, last_pulse_run_id, last_checked_at,
				last_decision, last_reason, last_gate_decision, last_result,
				last_result_reason, evidence_json, updated_at
			) VALUES (?, ?, ?, ?, 'due', ?, 'due', '', '', ?, ?)
			ON CONFLICT(workspace_path, module) DO UPDATE SET
				last_pulse_run_id=excluded.last_pulse_run_id,
				last_checked_at=excluded.last_checked_at,
				last_decision='due',
				last_reason=excluded.last_reason,
				last_gate_decision='due',
				last_result='',
				last_result_reason='',
				evidence_json=excluded.evidence_json,
				updated_at=excluded.updated_at`,
			module, normalized, pulseRunID, now, "explicit standalone /pulse-fixer backlog drain", string(evidenceJSON), now)
		if err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	states := make([]PulseModuleState, 0, len(canonical))
	for _, module := range canonical {
		state, err := getPulseModuleStateByModule(ctx, db, normalized, module)
		if err != nil {
			return nil, err
		}
		states = append(states, *state)
	}
	return states, nil
}

func recordTrustedPulseWorklistOnce(ctx context.Context, workspacePath, pulseRunID string, decisions []PulseWorklistDecision) ([]PulseModuleState, error) {
	return recordTrustedPulseWorklistOnceAfter(ctx, workspacePath, pulseRunID, decisions, nil)
}

func recordTrustedPulseWorklistOnceWithShadow(ctx context.Context, workspacePath, pulseRunID string, decisions []PulseWorklistDecision, shadowResult loopclosure.Result) ([]PulseModuleState, error) {
	return recordTrustedPulseWorklistOnceAfter(ctx, workspacePath, pulseRunID, decisions, func() {
		// Shadow instrumentation cannot block or modify live scheduling.
		// Coverage failures are retained in shadowResult rather than converted
		// to an empty, apparently-clean signal set.
		if err := recordPulseShadowSignalObservation(ctx, workspacePath, pulseRunID, shadowResult, decisions); err != nil {
			log.Printf("[PULSE] record shadow loop-closure observation for %s: %v", workspacePath, err)
		}
	})
}

func recordTrustedPulseWorklistOnceAfter(ctx context.Context, workspacePath, pulseRunID string, decisions []PulseWorklistDecision, afterRecord func()) ([]PulseModuleState, error) {
	if err := validatePulseWorklistDecisions(decisions); err != nil {
		return nil, err
	}

	pulseWorklistRecordMu.Lock()
	defer pulseWorklistRecordMu.Unlock()
	// Revalidate at the serialized write boundary. A session may have been
	// revoked after the tool call began but before argument parsing finished.
	if err := validateTrustedPulseToolRunID(ctx, pulseRunID); err != nil {
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
	states, err := recordPulseWorklist(ctx, workspacePath, pulseRunID, decisions)
	if err != nil {
		return nil, err
	}
	if afterRecord != nil {
		afterRecord()
	}
	return states, nil
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
	dueModules := make([]string, 0, maxPulseGateDueModules+1)
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
		if decision.Due {
			dueModules = append(dueModules, module)
		}
	}
	if len(dueModules) > maxPulseGateDueModules {
		return fmt.Errorf("Pulse Gate may select at most %d due modules per pass to cap review cost; got %d (%s). Defer the lower-priority modules with a reason and next-check boundary",
			maxPulseGateDueModules, len(dueModules), strings.Join(dueModules, ", "))
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
// output. Gate does not own builder/improve.html; coupling routing to an HTML
// write made a presentation failure discard an otherwise complete worklist.
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

// validatePulseDashboardArtifact proves that the dedicated Dashboard turn made
// a current, contract-compliant write. An agent-reported "done" is not enough:
// this check catches the production failure where Pulse completed while
// builder/improve.html remained in the retired Issues & fixes format.
func validatePulseDashboardArtifact(ctx context.Context, workspacePath, pulseRunID, previousHTML string, previousExists bool) error {
	htmlPath := strings.TrimSuffix(workspacePath, "/") + "/builder/improve.html"
	html, exists, err := readFileFromWorkspace(ctx, htmlPath)
	if err != nil {
		return fmt.Errorf("read Pulse dashboard artifact: %w", err)
	}
	if !exists || strings.TrimSpace(html) == "" {
		return fmt.Errorf("Pulse Dashboard did not write %s", htmlPath)
	}
	if previousExists && html == previousHTML {
		return fmt.Errorf("Pulse Dashboard left %s unchanged", htmlPath)
	}
	if !pulseDashboardHandoffContainsRunID(html, pulseRunID) {
		return fmt.Errorf("Pulse Dashboard handoff does not contain current pulse_run_id %q", pulseRunID)
	}
	if err := validatePulseImproveHTMLContract(html); err != nil {
		return fmt.Errorf("Pulse Dashboard wrote an outdated builder/improve.html: %w", err)
	}
	return nil
}

type pulseDashboardArtifactSnapshot struct {
	Path    string
	Content string
	Exists  bool
}

// capturePulseDashboardArtifacts snapshots the two files owned by the
// Dashboard stage so a timed-out or invalid write cannot leave a half-rendered
// pair behind.
func capturePulseDashboardArtifacts(ctx context.Context, workspacePath string) ([]pulseDashboardArtifactSnapshot, error) {
	base := strings.TrimSuffix(workspacePath, "/") + "/builder/"
	paths := []string{base + "improve.html", base + "card.health.html"}
	snapshots := make([]pulseDashboardArtifactSnapshot, 0, len(paths))
	for _, path := range paths {
		content, exists, err := readFileFromWorkspace(ctx, path)
		if err != nil {
			return nil, fmt.Errorf("snapshot %s: %w", path, err)
		}
		snapshots = append(snapshots, pulseDashboardArtifactSnapshot{
			Path: path, Content: content, Exists: exists,
		})
	}
	return snapshots, nil
}

func restorePulseDashboardArtifacts(ctx context.Context, snapshots []pulseDashboardArtifactSnapshot) error {
	var failures []string
	for _, snapshot := range snapshots {
		if snapshot.Exists {
			if err := writeFileToWorkspace(ctx, snapshot.Path, snapshot.Content); err != nil {
				failures = append(failures, snapshot.Path+": "+err.Error())
				continue
			}
		} else {
			_, exists, err := readFileFromWorkspace(ctx, snapshot.Path)
			if err != nil {
				failures = append(failures, snapshot.Path+": "+err.Error())
				continue
			}
			if exists {
				if err := deleteWorkspaceFile(ctx, snapshot.Path); err != nil {
					failures = append(failures, snapshot.Path+": "+err.Error())
					continue
				}
			}
		}
		content, exists, err := readFileFromWorkspace(ctx, snapshot.Path)
		if err != nil {
			failures = append(failures, snapshot.Path+": verify restore: "+err.Error())
			continue
		}
		if exists != snapshot.Exists || (snapshot.Exists && content != snapshot.Content) {
			failures = append(failures, snapshot.Path+": restore verification mismatch")
		}
	}
	if len(failures) > 0 {
		return fmt.Errorf("restore Pulse dashboard artifacts: %s", strings.Join(failures, "; "))
	}
	return nil
}

// validatePulseImproveHTMLContract checks the stable structural surface that
// distinguishes the current human-first Pulse page from retired or partially
// written shells. It intentionally does not validate prose or styling.
func validatePulseImproveHTMLContract(content string) error {
	if err := validatePulseHTMLTagBalance(content); err != nil {
		return err
	}
	if count := len(pulseLightweightSchemaRootPattern.FindAllString(content, -1)); count != 1 {
		return fmt.Errorf("expected exactly one data-pulse-schema=\"5\" html root (found %d)", count)
	}
	document, err := htmlpkg.Parse(strings.NewReader(content))
	if err != nil {
		return fmt.Errorf("parse HTML: %w", err)
	}

	brief, err := requireSinglePulseElement(document, "brief", "Latest Pulse brief")
	if err != nil {
		return err
	}
	briefGrids := pulseElementsWithClass(brief, "briefgrid")
	if len(briefGrids) != 1 {
		return fmt.Errorf("Latest Pulse must contain exactly one .briefgrid (found %d)", len(briefGrids))
	}
	briefItems := pulseDirectChildrenWithClass(briefGrids[0], "briefitem")
	expectedBriefLabels := []string{"Outcome", "Goal movement", "Next"}
	if len(briefItems) != len(expectedBriefLabels) {
		return fmt.Errorf("Latest Pulse must contain exactly %d brief cells (found %d)", len(expectedBriefLabels), len(briefItems))
	}
	expectedBrief := make(map[string]bool, len(expectedBriefLabels))
	for _, label := range expectedBriefLabels {
		expectedBrief[normalizePulseHTMLText(label)] = true
	}
	seenBrief := make(map[string]bool, len(briefItems))
	for _, item := range briefItems {
		headings := pulseElementsWithClass(item, "k")
		if len(headings) != 1 {
			return fmt.Errorf("each Latest Pulse cell must contain exactly one .k heading")
		}
		label := normalizePulseHTMLText(pulseHTMLText(headings[0]))
		if !expectedBrief[label] {
			return fmt.Errorf("Latest Pulse contains unknown cell heading %q", pulseHTMLText(headings[0]))
		}
		if seenBrief[label] {
			return fmt.Errorf("Latest Pulse contains duplicate cell heading %q", pulseHTMLText(headings[0]))
		}
		seenBrief[label] = true
	}

	for _, retiredClass := range []string{"coverage", "covitem", "assumptions", "worksummary", "workstats", "workstat", "workqueue", "workitem", "technical", "filters", "modfields", "agentlog"} {
		if len(pulseElementsWithClass(document, retiredClass)) > 0 {
			return fmt.Errorf("Lightweight Pulse report must not contain .%s blocks", retiredClass)
		}
	}
	activityEntries := pulseElementsWithClass(document, "entry")
	for _, entry := range activityEntries {
		if pulseHTMLHasClass(entry, "open") {
			return fmt.Errorf("Current Pulse report must not keep standing open-finding cards in Activity")
		}
	}

	if _, err := requireSinglePulseElementByID(document, "pulse-agent-handoff"); err != nil {
		return err
	}
	return nil
}

func pulseDashboardHandoffContainsRunID(content, pulseRunID string) bool {
	pulseRunID = strings.TrimSpace(pulseRunID)
	if pulseRunID == "" {
		return false
	}
	document, err := htmlpkg.Parse(strings.NewReader(content))
	if err != nil {
		return false
	}
	handoff, err := requireSinglePulseElementByID(document, "pulse-agent-handoff")
	if err != nil {
		return false
	}
	for _, attr := range handoff.Attr {
		if attr.Key == "data-pulse-run-id" {
			return strings.TrimSpace(attr.Val) == pulseRunID
		}
	}
	return false
}

// pulseGateHandoffContainsRunID remains a compatibility read for legacy
// recovery markers. Live Gate routing no longer depends on HTML.
func pulseGateHandoffContainsRunID(content, pulseRunID string) bool {
	pulseRunID = strings.TrimSpace(pulseRunID)
	if pulseRunID == "" {
		return false
	}
	document, err := htmlpkg.Parse(strings.NewReader(content))
	if err != nil {
		return false
	}
	handoff, err := requireSinglePulseElementByID(document, "pulse-agent-handoff")
	if err != nil {
		return false
	}
	for _, attr := range handoff.Attr {
		if strings.TrimSpace(attr.Val) == pulseRunID {
			return true
		}
	}
	return strings.Contains(pulseHTMLText(handoff), pulseRunID)
}

func validatePulseHTMLTagBalance(content string) error {
	tracked := map[string]bool{"div": true, "details": true, "summary": true}
	var stack []string
	tokenizer := htmlpkg.NewTokenizer(strings.NewReader(content))
	for {
		switch tokenizer.Next() {
		case htmlpkg.ErrorToken:
			if len(stack) > 0 {
				return fmt.Errorf("unclosed <%s> element", stack[len(stack)-1])
			}
			return nil
		case htmlpkg.StartTagToken:
			name, _ := tokenizer.TagName()
			tag := strings.ToLower(string(name))
			if tracked[tag] {
				stack = append(stack, tag)
			}
		case htmlpkg.EndTagToken:
			name, _ := tokenizer.TagName()
			tag := strings.ToLower(string(name))
			if !tracked[tag] {
				continue
			}
			if len(stack) == 0 {
				return fmt.Errorf("unexpected closing </%s> element", tag)
			}
			if stack[len(stack)-1] != tag {
				return fmt.Errorf("mismatched closing </%s>; expected </%s>", tag, stack[len(stack)-1])
			}
			stack = stack[:len(stack)-1]
		}
	}
}

func requireSinglePulseElement(root *htmlpkg.Node, className, description string) (*htmlpkg.Node, error) {
	nodes := pulseElementsWithClass(root, className)
	if len(nodes) != 1 {
		return nil, fmt.Errorf("%s must appear exactly once (found %d)", description, len(nodes))
	}
	return nodes[0], nil
}

func requireSinglePulseElementByID(root *htmlpkg.Node, id string) (*htmlpkg.Node, error) {
	var nodes []*htmlpkg.Node
	pulseWalkElements(root, func(node *htmlpkg.Node) {
		for _, attr := range node.Attr {
			if attr.Key == "id" && attr.Val == id {
				nodes = append(nodes, node)
				return
			}
		}
	})
	if len(nodes) != 1 {
		return nil, fmt.Errorf("#%s must appear exactly once (found %d)", id, len(nodes))
	}
	return nodes[0], nil
}

func pulseElementsWithClass(root *htmlpkg.Node, className string) []*htmlpkg.Node {
	var nodes []*htmlpkg.Node
	pulseWalkElements(root, func(node *htmlpkg.Node) {
		for _, attr := range node.Attr {
			if attr.Key != "class" {
				continue
			}
			for _, class := range strings.Fields(attr.Val) {
				if class == className {
					nodes = append(nodes, node)
					return
				}
			}
		}
	})
	return nodes
}

func pulseHTMLAttribute(node *htmlpkg.Node, key string) string {
	if node == nil {
		return ""
	}
	for _, attr := range node.Attr {
		if attr.Key == key {
			return attr.Val
		}
	}
	return ""
}

func pulseHTMLHasClass(node *htmlpkg.Node, className string) bool {
	for _, class := range strings.Fields(pulseHTMLAttribute(node, "class")) {
		if class == className {
			return true
		}
	}
	return false
}

func pulseDirectChildrenWithClass(root *htmlpkg.Node, className string) []*htmlpkg.Node {
	var nodes []*htmlpkg.Node
	for child := root.FirstChild; child != nil; child = child.NextSibling {
		if child.Type != htmlpkg.ElementNode {
			continue
		}
		for _, attr := range child.Attr {
			if attr.Key != "class" {
				continue
			}
			for _, class := range strings.Fields(attr.Val) {
				if class == className {
					nodes = append(nodes, child)
					break
				}
			}
		}
	}
	return nodes
}

func pulseWalkElements(root *htmlpkg.Node, visit func(*htmlpkg.Node)) {
	if root.Type == htmlpkg.ElementNode {
		visit(root)
	}
	for child := root.FirstChild; child != nil; child = child.NextSibling {
		pulseWalkElements(child, visit)
	}
}

func pulseHTMLText(root *htmlpkg.Node) string {
	var text strings.Builder
	var walk func(*htmlpkg.Node)
	walk = func(node *htmlpkg.Node) {
		if node.Type == htmlpkg.TextNode {
			text.WriteString(" ")
			text.WriteString(node.Data)
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(root)
	return strings.Join(strings.Fields(text.String()), " ")
}

func normalizePulseHTMLText(value string) string {
	return strings.ToLower(strings.Join(strings.Fields(value), " "))
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
	if rawID := strings.TrimSpace(r.URL.Query().Get("id")); rawID != "" {
		id, err := strconv.ParseInt(rawID, 10, 64)
		if err != nil || id <= 0 {
			http.Error(w, "id must be a positive integer", http.StatusBadRequest)
			return
		}
		artifact, err := step_based_workflow.LoadPulseReviewArtifact(r.Context(), workspacePath, id)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				http.Error(w, "Pulse review not found", http.StatusNotFound)
				return
			}
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "review": artifact})
		return
	}
	module := normalizePulseModule(r.URL.Query().Get("module"))
	if module != "" && !validPulseModules[module] {
		http.Error(w, fmt.Sprintf("module %q is not valid", module), http.StatusBadRequest)
		return
	}
	artifacts, err := step_based_workflow.LoadPulseReviewArtifacts(r.Context(), workspacePath, module, false, -1)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "reviews": artifacts, "total": len(artifacts)})
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
	if module != "" && module != "pulse_fixer" {
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
	beginFixerTool := llmtypes.Tool{
		Type: "function",
		Function: &llmtypes.FunctionDefinition{
			Name:        "begin_pulse_fixer_run",
			Description: "Begin an explicit standalone /pulse-fixer lifecycle run for existing SQLite-backed findings. Use only after get_pulse_state(view=\"module\") and only for modules whose retained backlog the user asked to fix. This does not run Gate or reviewers, does not alter unrelated module cadence, and refuses to take over a module already due in an unresolved Pulse run. It returns the pulse_run_id required by record_pulse_result.",
			Parameters: llmtypes.NewParameters(map[string]interface{}{
				"type":                 "object",
				"additionalProperties": false,
				"properties": map[string]interface{}{
					"workspace_path": map[string]interface{}{"type": "string", "description": "Workflow-relative path, e.g. Workflow/social-media."},
					"modules": map[string]interface{}{
						"type":        "array",
						"minItems":    1,
						"uniqueItems": true,
						"items":       map[string]interface{}{"type": "string", "enum": moduleEnum},
						"description": "Owning modules for the existing findings selected from get_pulse_state(view=\"module\").",
					},
				},
				"required": []string{"workspace_path", "modules"},
			}),
		},
	}
	recordTool := llmtypes.Tool{
		Type: "function",
		Function: &llmtypes.FunctionDefinition{
			Name:        "record_pulse_worklist",
			Description: fmt.Sprintf("Record the dynamic Pulse worklist for this run in the workflow's db/db.sqlite. Pulse Gate must call this exactly once after deciding which perspectives are due or skipped. The decisions array must contain exactly one entry for each current Pulse module: %s. To cap cost, select at most %d due modules in one Pulse pass; defer any lower-priority eligible module with its reason and next-check boundary. workflow_review is Engineering Review and conditionally covers execution, report/eval implementation, plan-change/artifact consistency, and store-integrity evidence. llm_ops_review owns efficiency and runtime operations. strategy_auditor owns product/business adequacy inside the current strategy; goal_advisor owns materially different approaches. Engineering and Ops may share one selected-perspective sequence; Strategy and Goal remain independent agents. Do not pass retired artifact-named modules and never make one reviewer depend on another. Every skipped module must include next_check_at, next_check_after_run_id, or a positive cooldown_runs value.", strings.Join(pulseModuleOrder, ", "), maxPulseGateDueModules),
			Parameters: llmtypes.NewParameters(map[string]interface{}{
				"type":                 "object",
				"additionalProperties": false,
				"properties": map[string]interface{}{
					"workspace_path": map[string]interface{}{"type": "string", "description": "Workflow-relative path, e.g. Workflow/social-media."},
					"pulse_run_id":   map[string]interface{}{"type": "string", "description": "Scheduler-provided Pulse run id. Use exactly the id in the prompt."},
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
				"required": []string{"workspace_path", "pulse_run_id", "decisions"},
			}),
		},
	}
	stateTool := llmtypes.Tool{
		Type: "function",
		Function: &llmtypes.FunctionDefinition{
			Name: "get_pulse_state",
			Description: fmt.Sprintf("Read Pulse state from the workflow's db/db.sqlite. One read tool with three views; never use Dashboard HTML as the source of truth.\n"+
				"view=\"module\": per-module cadence and results so Pulse Gate can decide what is due, plus the complete active concern backlog, externally owned suppressed concerns, plan-change backlog, reviewer history, impact ledger, and read-only loop-closure facts. Read this before record_pulse_worklist. Loop-closure findings are evidence Gate may weigh; they do not mandate a module or authorize mutation. A concern with a high seen_count has been reported on that many runs and should weigh heavily.\n"+
				"view=\"backlog\": the durable SDLC-style issue backlog — each compact issue, current lifecycle state, fix attempts, verification history, internal fingerprint, and external-action disposition. Optional module filter. issue.id is the stable human-facing finding_id; fingerprint is an internal lifecycle key. A fixer passes both from the same item and never derives sameness from either ID.\n"+
				"view=\"review\": one saved reviewer result as JSON with its markdown and validated structured verifications. Requires review_run_id and module exactly as reported by the call_generic_agent completion notification; Pulse review Markdown files are no longer created.\n"+
				"Close a real finding only through a verified finding_disposition on record_pulse_result; resolve_run_concern is limited to acknowledgment or rejection. Modules: %s.", pulseModuleList()),
			Parameters: llmtypes.NewParameters(map[string]interface{}{
				"type":                 "object",
				"additionalProperties": false,
				"properties": map[string]interface{}{
					"workspace_path": map[string]interface{}{"type": "string", "description": "Workflow-relative path, e.g. Workflow/social-media."},
					"view":           map[string]interface{}{"type": "string", "enum": pulseStateViewValues, "description": "Which Pulse state to read: module cadence, the finding backlog, or one saved review."},
					"module":         map[string]interface{}{"type": "string", "description": "Optional owning-module filter for view=\"backlog\" (omit for the complete backlog). Required for view=\"review\". Ignored for view=\"module\"."},
					"review_run_id":  map[string]interface{}{"type": "string", "description": "Required for view=\"review\": the review run id from the reviewer's completion notification."},
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
				"Fix attempts are opened by the backend from the disposition itself; there is no separate attempt tool and no attempt_id to carry. A fixed_verified finding needs changed_files plus only passed post-change checks. changed_unverified needs an inconclusive check plus next_check naming the run, table, or artifact whose arrival settles it, and remains open awaiting that evidence; the next review verifies it against that evidence rather than re-attempting the fix. external_action_required permanently removes a diagnosed real finding from Pulse's active queue and requires external_owner, reason_code, and reopen_condition; use it only when workflow tools cannot act. A failed check reopens the concern. awaiting_run is a real finding waiting only on a scheduled run to produce its evidence — no fix applied, nobody stuck — and requires next_check naming that run; use it instead of blocked, which means no action is available at all. awaiting_user requires human_input_id naming a still-pending create_human_input_request, so a finding cannot wait on a decision the operator was never asked for; escalate only when the goal does not already settle it and the cost of deciding is real, otherwise decide and record the reasoning. For strategy_auditor and goal_advisor, proposal_only is accepted only with a concrete next_check evidence boundary; an actionable recommendation must use awaiting_user linked to that module's pending decision, while safe technical prerequisites use the normal Fixer lifecycle. before_refs and after_refs are paired agent-supplied audit references; the backend preserves them but does not recompute arbitrary textual checks. The lifecycle is machine-validated for finding/module linkage and required evidence shape, not for the truth of an agent-authored verdict. Put exact technical failures in reason.",
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
								"fingerprint":      map[string]interface{}{"type": "string", "description": "Internal fingerprint from the selected get_pulse_state(view=\"backlog\") item."},
								"finding_id":       map[string]interface{}{"type": "string", "description": "That same backlog item's issue.id."},
								"disposition":      map[string]interface{}{"type": "string", "enum": []string{"fixed_verified", "verified_no_change", "changed_unverified", "proposal_only", "awaiting_user", "awaiting_run", "blocked", "external_action_required", "failed", "rejected"}},
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
							"required": []string{"fingerprint", "finding_id", "disposition", "summary"},
						},
					},
				},
				"required": []string{"workspace_path", "pulse_run_id", "result", "reason"},
			}),
		},
	}
	impactTool, impactExecutor := createRecordPulseImpactTool()

	executors := map[string]interface{}{
		"begin_pulse_fixer_run": func(ctx context.Context, args map[string]interface{}) (string, error) {
			workspacePath, _ := args["workspace_path"].(string)
			sessionID := strings.TrimSpace(mcpexecutor.SessionIDFromContext(ctx))
			if sessionID == "" {
				return "", fmt.Errorf("begin_pulse_fixer_run requires an active workshop session")
			}
			modules := stringSliceFromToolArg(args["modules"])
			pulseRunID := fmt.Sprintf("manual-fixer--%s-%d", time.Now().UTC().Format("20060102T150405Z"), time.Now().UTC().UnixNano())
			states, err := recordStandalonePulseFixerModules(ctx, workspacePath, pulseRunID, modules)
			if err != nil {
				return "", err
			}
			registerTemporaryTrustedPulseSession(sessionID, pulseRunID, 2*time.Hour)
			payload, _ := json.Marshal(map[string]interface{}{
				"status":       "started",
				"pulse_run_id": pulseRunID,
				"modules":      states,
				"expires_in":   "2h",
			})
			return string(payload), nil
		},
		"record_pulse_worklist": func(ctx context.Context, args map[string]interface{}) (string, error) {
			workspacePath, _ := args["workspace_path"].(string)
			pulseRunID, _ := args["pulse_run_id"].(string)
			if err := validateTrustedPulseToolRunID(ctx, pulseRunID); err != nil {
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
			states, err := recordTrustedPulseWorklistOnceWithShadow(ctx, normalized, pulseRunID, decisions, shadowResult)
			if err != nil {
				return "", err
			}
			payload, _ := json.Marshal(map[string]interface{}{"status": "recorded", "modules": states})
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
				return readPulseModuleStateView(ctx, workspacePath)
			case pulseStateViewBacklog:
				module, _ := args["module"].(string)
				return readPulseBacklogView(ctx, workspacePath, module)
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
		"begin_pulse_fixer_run": "workflow",
		"record_pulse_worklist": "workflow",
		"get_pulse_state":       "workflow",
		"record_pulse_result":   "workflow",
		"record_pulse_impact":   "workflow",
		"resolve_run_concern":   "workflow",
	}
	resolveConcernTool := llmtypes.Tool{
		Type: "function",
		Function: &llmtypes.FunctionDefinition{
			Name:        "resolve_run_concern",
			Description: "Acknowledge or reject a concern returned by get_pulse_state. Use rejected only when evidence shows it is not a problem; rejected concerns stay closed if the same text recurs. Use acknowledged when it is real but deliberately deferred. This tool cannot close a real bug as resolved: use record_pulse_result with a verified finding_disposition so changed files and test evidence are retained. Absence is never evidence of a fix.",
			Parameters: llmtypes.NewParameters(map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"workspace_path": map[string]interface{}{"type": "string", "description": "Workflow-relative path, e.g. Workflow/social-media."},
					"fingerprint":    map[string]interface{}{"type": "string", "description": "The concern's fingerprint from get_pulse_state."},
					"status":         map[string]interface{}{"type": "string", "enum": []string{"acknowledged", "rejected"}},
					"note":           map[string]interface{}{"type": "string", "description": "Short justification recorded with the judgement."},
				},
				"required": []string{"workspace_path", "fingerprint", "status"},
			}),
		},
	}
	executors["resolve_run_concern"] = func(ctx context.Context, args map[string]interface{}) (string, error) {
		workspacePath, _ := args["workspace_path"].(string)
		fingerprint, _ := args["fingerprint"].(string)
		status, _ := args["status"].(string)
		note, _ := args["note"].(string)
		if strings.EqualFold(strings.TrimSpace(status), step_based_workflow.ConcernStatusResolved) {
			return "", fmt.Errorf("resolve_run_concern cannot close a real finding; use record_pulse_result with verified finding_dispositions")
		}
		if err := step_based_workflow.ResolveRunConcern(ctx, workspacePath, fingerprint, status, "pulse", note); err != nil {
			return "", err
		}
		return fmt.Sprintf("Concern %s marked %s.", fingerprint, status), nil
	}

	return []llmtypes.Tool{beginFixerTool, recordTool, stateTool, resultTool, impactTool, resolveConcernTool}, executors, categories
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

func readPulseModuleStateView(ctx context.Context, workspacePath string) (string, error) {
	states, err := getPulseModuleStates(ctx, workspacePath)
	if err != nil {
		return "", err
	}
	// Open step concerns ride along with module state rather than needing
	// their own tool: the Gate already calls this immediately before
	// deciding what is due, which is exactly when a recurring concern
	// should influence that decision. Best-effort — a concerns read
	// failure must not block the Gate from scheduling modules.
	concerns, concernsErr := step_based_workflow.LoadOpenRunConcerns(ctx, workspacePath, -1)
	if concernsErr != nil {
		log.Printf("[PULSE] get_pulse_state(view=module): open concerns unavailable for %s: %v", workspacePath, concernsErr)
	}
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
	payload, _ := json.Marshal(map[string]interface{}{
		"modules":                  states,
		"open_concerns":            concerns,
		"open_concern_count":       len(concerns),
		"concerns_note":            "Complete active backlog. seen_count is how many runs reported the same thing; absence is not evidence of a fix. Use severity, age, recurrence, ownership, and starvation—not recurrence alone—when selecting work.",
		"suppressed_concerns":      suppressedConcerns,
		"suppressed_concern_count": len(suppressedConcerns),
		"suppressed_concerns_note": "Diagnosed real findings owned outside this workflow. Do not report an unchanged fingerprint as a new finding or spend active review effort on it. A materially changed target/evidence creates or reopens an active finding; the recorded reopen condition explains the boundary.",
		"plan_change_backlog":      planBacklog,
		"loop_closure":             loopClosure,
		"loop_closure_note":        "Read-only deterministic evidence. Gate may weigh verified findings alongside other facts, but they do not mandate a module or authorize mutation. coverage_status must be verified before an empty findings list means clean.",
		"module_review_history":    reviewHistory,
		"review_history_note":      "What each reviewer concluded the last few times it ran, most recently run first. A module absent from this list has not run in the retained window at all. Use it to justify each skip: a module that keeps returning real findings is a poor candidate for another cooldown, and one that has come back clean repeatedly is a good one. A verdict here is the reviewer's conclusion, which is not the same as whether anything was then fixed.",
		"impact_ledger":            impactLedger,
		"impact_ledger_note":       "Durable intervention, per-run success-criterion observation, and append-only before/after assessment history. Reliability or measurement work is not direct goal progress; inconclusive is correct until a comparable evidence window matures.",
	})
	return string(payload), nil
}

func readPulseBacklogView(ctx context.Context, workspacePath, module string) (string, error) {
	module = normalizePulseModule(module)
	if module != "" && !validPulseModules[module] {
		return "", fmt.Errorf("module %q is not a valid Pulse module. Must be one of: %s. Omit module entirely to load the complete backlog", module, pulseModuleList())
	}
	findings, err := step_based_workflow.LoadPulseFindingLifecycles(ctx, workspacePath, module, -1)
	if err != nil {
		return "", err
	}
	payload, _ := json.Marshal(map[string]interface{}{
		"findings": findings,
		"total":    len(findings),
		"note":     "Durable issue, attempt, verification, and disposition history. issue.id is the stable finding_id; fingerprint is internal lifecycle plumbing. A fixer passes both from the same item. Match by affected behavior, expected outcome, and observed failure—not either identifier.",
	})
	return string(payload), nil
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
	artifact, err := step_based_workflow.LoadPulseReviewArtifactForRun(ctx, workspacePath, reviewRunID, module)
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
		"module":        artifact.Module,
		"review_run_id": artifact.ReviewRunID,
		"pulse_run_id":  artifact.PulseRunID,
		"status":        artifact.Status,
		"verifications": artifact.Verifications,
		"markdown":      artifact.Markdown,
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
	if err := validateTrustedPulseToolRunID(ctx, pulseRunID); err != nil {
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
	if strings.HasPrefix(pulseRunID, "manual-fixer--") {
		worklist, exists, readErr := getPulseWorklistForRun(ctx, workspacePath, pulseRunID)
		if readErr == nil && exists {
			complete := true
			for _, selected := range worklist {
				if strings.TrimSpace(selected.LastResult) == "" {
					complete = false
					break
				}
			}
			if complete {
				releaseTrustedPulseSessionForRun(ctx, pulseRunID)
			}
		}
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
	`[{"fingerprint": "<backlog fingerprint>", "finding_id": "<backlog issue.id>", "disposition": "fixed_verified", "summary": "<one sentence>", ` +
	`"changed_files": ["path/to/file"], ` +
	`"verification": [{"check": "<what was run>", "verdict": "passed", "expected": "<expected>", "observed": "<observed>"}]}]`

func pulseFindingDispositionsFromToolArg(raw interface{}) ([]step_based_workflow.PulseFindingDisposition, error) {
	if raw == nil {
		return nil, nil
	}
	encoded, err := json.Marshal(raw)
	if err != nil {
		return nil, fmt.Errorf("encode finding_dispositions (%s): %w", pulseFindingDispositionsShape, err)
	}
	var dispositions []step_based_workflow.PulseFindingDisposition
	if err := json.Unmarshal(encoded, &dispositions); err != nil {
		return nil, fmt.Errorf("decode finding_dispositions (%s): %w", pulseFindingDispositionsShape, err)
	}
	for index := range dispositions {
		dispositions[index] = step_based_workflow.NormalizePulseFindingDisposition(dispositions[index])
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
			messages = append(messages, fmt.Sprintf("reviewer verification for finding %q (fingerprint %q) requires a matching finding_disposition before the module can be terminal", review.FindingID, review.Fingerprint))
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
