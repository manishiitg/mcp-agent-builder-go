package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/loopclosure"
	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/orchestrator/agents/workflow/step_based_workflow"
	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/pulsemodules"
	"github.com/manishiitg/multi-llm-provider-go/llmtypes"
	htmlpkg "golang.org/x/net/html"
)

const (
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
	pulseModuleCostLLMTime  = pulsemodules.CostLLMTimeID
	// pulseModuleLLMOpsReview also owns plan-design hygiene (step-type
	// fitness, prevalidation fitness, schema/description drift) alongside its
	// original model/tier/catalog scope — see its due-trigger section in
	// post-run-monitor.md. This was previously unowned: Goal Advisor's own
	// contract explicitly excludes plan-cleanup content, so a plan-design
	// change trigger inside Goal Advisor was never actually consistent with
	// what Goal Advisor is for.
	pulseModuleLLMOpsReview = pulsemodules.LLMOpsReviewID
	// pulseModuleStrategyAuditor owns read-only plan-versus-goal diagnosis
	// across retained runs. Goal Advisor owns the resulting proposal,
	// experiment, approval, or plan mutation.
	pulseModuleStrategyAuditor = pulsemodules.StrategyAuditorID
	pulseModuleGoalAdvisor     = pulsemodules.GoalAdvisorID
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
// deliberately not returned by get_pulse_module_state, so the live Gate cannot
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
		return nil, fmt.Errorf("pulse_run_id is required")
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
			return nil, fmt.Errorf("module %q is not valid", decision.Module)
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
		return fmt.Errorf("decisions are required")
	}
	if len(decisions) != len(pulseModuleOrder) {
		return fmt.Errorf("decisions must include exactly one entry for each Pulse module; got %d want %d", len(decisions), len(pulseModuleOrder))
	}
	seen := map[string]bool{}
	for _, decision := range decisions {
		module := normalizePulseModule(decision.Module)
		if !validPulseModules[module] {
			return fmt.Errorf("module %q is not valid", decision.Module)
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
	for _, module := range pulseModuleOrder {
		if !seen[module] {
			return fmt.Errorf("decisions missing required module %q", module)
		}
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

func validatePulseGateCompletion(ctx context.Context, workspacePath, pulseRunID, previousHTML string, previousExists bool) error {
	worklist, exists, err := getPulseWorklistForRun(ctx, workspacePath, pulseRunID)
	if err != nil {
		return fmt.Errorf("read Pulse Gate worklist: %w", err)
	}
	if !exists || !pulseWorklistIsComplete(worklist) {
		return fmt.Errorf("Pulse Gate did not record a complete worklist for pulse_run_id %q", pulseRunID)
	}

	htmlPath := strings.TrimSuffix(workspacePath, "/") + "/builder/improve.html"
	html, htmlExists, err := readFileFromWorkspace(ctx, htmlPath)
	if err != nil {
		return fmt.Errorf("read Pulse Gate handoff: %w", err)
	}
	if !htmlExists || strings.TrimSpace(html) == "" {
		return fmt.Errorf("Pulse Gate did not write %s", htmlPath)
	}
	if previousExists && html == previousHTML {
		return fmt.Errorf("Pulse Gate left %s unchanged", htmlPath)
	}
	if !pulseGateHandoffContainsRunID(html, pulseRunID) {
		return fmt.Errorf("Pulse Gate handoff does not contain current pulse_run_id %q", pulseRunID)
	}
	return nil
}

func pulseGateHandoffContainsRunID(html, pulseRunID string) bool {
	pulseRunID = strings.TrimSpace(pulseRunID)
	if pulseRunID == "" {
		return false
	}
	tokenizer := htmlpkg.NewTokenizer(strings.NewReader(html))
	depth := 0
	for {
		switch tokenizer.Next() {
		case htmlpkg.ErrorToken:
			return false
		case htmlpkg.StartTagToken, htmlpkg.SelfClosingTagToken:
			token := tokenizer.Token()
			if depth == 0 {
				if !htmlTokenHasID(token, "pulse-agent-handoff") {
					continue
				}
				// Any attribute carrying this run's id counts. The canonical name is
				// data-pulse-run-id, but the .pulse-fixer-recovery ledger nested in
				// this same element is keyed by data-pulse-run, and Gate has written
				// the correct id under the ledger's name — one letter apart, same
				// element hierarchy. Rejecting that cost a full Pulse run and forced
				// a recovery session while the handoff was, substantively, right.
				//
				// What matters is that this handoff belongs to this run, which any
				// attribute holding the id establishes. A stale handoff still fails,
				// because no attribute would carry the current id.
				for _, attr := range token.Attr {
					if strings.TrimSpace(attr.Val) == pulseRunID {
						return true
					}
				}
				if token.Type == htmlpkg.StartTagToken {
					depth = 1
				}
				continue
			}
			if token.Type == htmlpkg.StartTagToken {
				depth++
			}
		case htmlpkg.TextToken:
			if depth > 0 && strings.Contains(tokenizer.Token().Data, pulseRunID) {
				return true
			}
		case htmlpkg.EndTagToken:
			if depth > 0 {
				depth--
			}
		}
	}
}

func markPulseModuleResult(ctx context.Context, workspacePath, module, pulseRunID, result, reason string, evidence []string) (*PulseModuleState, error) {
	module = normalizePulseModule(module)
	if !validPulseModules[module] {
		return nil, fmt.Errorf("module %q is not valid", module)
	}
	pulseRunID = strings.TrimSpace(pulseRunID)
	if pulseRunID == "" {
		return nil, fmt.Errorf("pulse_run_id is required")
	}
	result = strings.TrimSpace(strings.ToLower(result))
	if result == "" {
		return nil, fmt.Errorf("result is required")
	}
	switch result {
	case "done", "changed", "blocked", "failed", "skipped", "timed_out":
	default:
		return nil, fmt.Errorf("result must be one of done, changed, blocked, failed, skipped, timed_out")
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return nil, fmt.Errorf("reason is required")
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
	module = normalizePulseModule(module)
	if !validPulseModules[module] {
		return nil, fmt.Errorf("module %q is not valid", module)
	}
	pulseRunID = strings.TrimSpace(pulseRunID)
	result = strings.TrimSpace(strings.ToLower(result))
	switch result {
	case "done", "changed", "blocked", "failed", "skipped":
	default:
		return nil, fmt.Errorf("result must be one of done, changed, blocked, failed, skipped")
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return nil, fmt.Errorf("reason is required")
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
			if err := recordPulseModuleAudit(ctx, db, normalized, module, pulseRunID, result, reason, evidence, audit, now); err != nil {
				return nil, err
			}
			return existing, nil
		}
		return nil, fmt.Errorf("Pulse module %q for run %q is already terminal or belongs to another run", module, pulseRunID)
	}
	if err := recordPulseModuleAudit(ctx, tx, normalized, module, pulseRunID, result, reason, evidence, audit, now); err != nil {
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
		return fmt.Errorf("pulse_run_id is required")
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
	recordTool := llmtypes.Tool{
		Type: "function",
		Function: &llmtypes.FunctionDefinition{
			Name:        "record_pulse_worklist",
			Description: "Record the dynamic Pulse worklist for this run in the workflow's db/db.sqlite. Pulse Gate must call this exactly once after deciding which modules are due or skipped. The decisions array must contain exactly one entry for each Pulse module: bug_review, artifact_review, report_health, eval_health, stores_health, cost_llm_time, llm_ops_review, strategy_auditor, and goal_advisor. stores_health covers learnings, knowledgebase, and database freshness/quality — do not pass the old learning_health/knowledgebase_health/db_health names. strategy_auditor diagnoses plan-versus-goal from cross-run evidence; goal_advisor owns proposals and experiments. Every skipped module must include next_check_at, next_check_after_run_id, or a positive cooldown_runs value. The scheduler reads this table and only sends prompts for due modules.",
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
			Name:        "get_pulse_module_state",
			Description: "Read the workflow-local Pulse module state from db/db.sqlite so Pulse Gate can decide what is due this run, plus open concerns and read-only loop-closure facts. Use this before record_pulse_worklist. Loop-closure findings are evidence Gate may weigh; they do not mandate a module, override the 3-module cap, or authorize a mutation. A concern with a high seen_count has been reported on that many runs and should weigh heavily on which module you mark due; resolve or reject it with resolve_run_concern once a module has acted on it.",
			Parameters: llmtypes.NewParameters(map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"workspace_path": map[string]interface{}{"type": "string", "description": "Workflow-relative path, e.g. Workflow/social-media."},
				},
				"required": []string{"workspace_path"},
			}),
		},
	}
	resultTool := llmtypes.Tool{
		Type: "function",
		Function: &llmtypes.FunctionDefinition{
			Name:        "mark_pulse_module_result",
			Description: "Mark a selected Pulse module as done, changed, blocked, failed, or skipped after its module review and Pulse Fixer work complete. This also writes the durable internal audit row for that run and module. For result=changed, changed_files and verification are required. before_refs and after_refs carry only useful hashes, versions, or cursors when they exist. Put an exact technical failure in reason for failed/blocked results. Scheduler timeouts are recorded by the scheduler and cannot be overwritten by an agent.",
			Parameters: llmtypes.NewParameters(map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"workspace_path": map[string]interface{}{"type": "string", "description": "Workflow-relative path, e.g. Workflow/social-media."},
					"pulse_run_id":   map[string]interface{}{"type": "string", "description": "Scheduler-provided Pulse run id."},
					"module":         map[string]interface{}{"type": "string", "enum": moduleEnum},
					"result":         map[string]interface{}{"type": "string", "enum": []string{"done", "changed", "blocked", "failed", "skipped"}},
					"reason":         map[string]interface{}{"type": "string", "description": "One-sentence result summary."},
					"evidence":       map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}},
					"changed_files":  map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}, "description": "Exact workspace-relative files changed by this module. Required when result=changed; otherwise omit."},
					"verification":   map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}, "description": "Exact checks performed and their factual outcomes. Required when result=changed; otherwise include only when useful."},
					"before_refs":    map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}, "description": "Optional pre-change hashes, versions, or cursors used to detect drift."},
					"after_refs":     map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}, "description": "Optional post-change hashes, versions, or cursors paired with before_refs."},
				},
				"required": []string{"workspace_path", "pulse_run_id", "module", "result", "reason"},
			}),
		},
	}
	finalCommandTool := llmtypes.Tool{
		Type: "function",
		Function: &llmtypes.FunctionDefinition{
			Name:        "mark_pulse_final_command_result",
			Description: "Record the live or final status of one Pulse final command in the workflow-local db/db.sqlite. The combined Pulse finalizer must mark each command running before work and then done, skipped, blocked, or failed immediately after the command finishes.",
			Parameters: llmtypes.NewParameters(map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"workspace_path": map[string]interface{}{"type": "string", "description": "Workflow-relative path, e.g. Workflow/social-media."},
					"pulse_run_id":   map[string]interface{}{"type": "string", "description": "Scheduler-provided Pulse run id."},
					"command":        map[string]interface{}{"type": "string", "enum": pulseFinalCommandOrder},
					"status":         map[string]interface{}{"type": "string", "enum": []string{"running", "done", "skipped", "blocked", "failed"}},
					"reason":         map[string]interface{}{"type": "string", "description": "Short factual status or outcome."},
				},
				"required": []string{"workspace_path", "pulse_run_id", "command", "status", "reason"},
			}),
		},
	}

	executors := map[string]interface{}{
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
		"get_pulse_module_state": func(ctx context.Context, args map[string]interface{}) (string, error) {
			workspacePath, _ := args["workspace_path"].(string)
			states, err := getPulseModuleStates(ctx, workspacePath)
			if err != nil {
				return "", err
			}
			// Open step concerns ride along with module state rather than needing
			// their own tool: the Gate already calls this immediately before
			// deciding what is due, which is exactly when a recurring concern
			// should influence that decision. Best-effort — a concerns read
			// failure must not block the Gate from scheduling modules.
			concerns, concernsErr := step_based_workflow.LoadOpenRunConcerns(ctx, workspacePath, 25)
			if concernsErr != nil {
				log.Printf("[PULSE] get_pulse_module_state: open concerns unavailable for %s: %v", workspacePath, concernsErr)
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
				log.Printf("[PULSE] get_pulse_module_state: review history unavailable for %s: %v", workspacePath, historyErr)
			}
			// This detector has already been validated against fleet state and
			// is supplied as a read-only fact feed, like open_concerns and
			// plan_change_backlog. Gate may weigh it but receives no mandatory
			// routing rule. A separate pre-decision snapshot is still retained
			// when Gate records its worklist so its handling remains auditable.
			loopClosure := loopclosure.Check(ctx, workspacePath, time.Now().UTC())
			payload, _ := json.Marshal(map[string]interface{}{
				"modules":               states,
				"open_concerns":         concerns,
				"concerns_note":         "Step-raised concerns, most-recurring first. seen_count is how many runs reported the same thing; a high count is a stronger signal than a fresh one. Absence of a previously-seen concern does NOT mean it was fixed.",
				"plan_change_backlog":   planBacklog,
				"loop_closure":          loopClosure,
				"loop_closure_note":     "Read-only deterministic evidence. Gate may weigh verified findings alongside other facts, but they do not mandate a module, override the 3-module cap, or authorize a mutation. coverage_status must be verified before an empty findings list means clean.",
				"module_review_history": reviewHistory,
				"review_history_note":   "What each reviewer concluded the last few times it ran, most recently run first. A module absent from this list has not run in the retained window at all. Use it to justify each skip: a module that keeps returning real findings is a poor candidate for another cooldown, and one that has come back clean repeatedly is a good one. A verdict here is the reviewer's conclusion, which is not the same as whether anything was then fixed.",
			})
			return string(payload), nil
		},
		"mark_pulse_module_result": func(ctx context.Context, args map[string]interface{}) (string, error) {
			workspacePath, _ := args["workspace_path"].(string)
			pulseRunID, _ := args["pulse_run_id"].(string)
			module, _ := args["module"].(string)
			result, _ := args["result"].(string)
			reason, _ := args["reason"].(string)
			if err := validateTrustedPulseToolRunID(ctx, pulseRunID); err != nil {
				return "", err
			}
			audit := PulseModuleAuditInput{
				ChangedFiles: stringSliceFromToolArg(args["changed_files"]),
				Verification: stringSliceFromToolArg(args["verification"]),
				BeforeRefs:   stringSliceFromToolArg(args["before_refs"]),
				AfterRefs:    stringSliceFromToolArg(args["after_refs"]),
			}
			if strings.EqualFold(strings.TrimSpace(result), "changed") {
				if len(audit.ChangedFiles) == 0 {
					return "", fmt.Errorf("changed_files is required when result=changed")
				}
				if len(audit.Verification) == 0 {
					return "", fmt.Errorf("verification is required when result=changed")
				}
			}
			state, err := markPulseModuleResultFromAgentWithAudit(ctx, workspacePath, module, pulseRunID, result, reason, stringSliceFromToolArg(args["evidence"]), audit)
			if err != nil {
				return "", err
			}
			payload, _ := json.Marshal(map[string]interface{}{"status": "updated", "module": state})
			return string(payload), nil
		},
		"mark_pulse_final_command_result": func(ctx context.Context, args map[string]interface{}) (string, error) {
			workspacePath, _ := args["workspace_path"].(string)
			pulseRunID, _ := args["pulse_run_id"].(string)
			command, _ := args["command"].(string)
			status, _ := args["status"].(string)
			reason, _ := args["reason"].(string)
			if err := validateTrustedPulseToolRunID(ctx, pulseRunID); err != nil {
				return "", err
			}
			state, err := markPulseFinalCommandStateFromAgent(ctx, workspacePath, command, pulseRunID, status, reason)
			if err != nil {
				return "", err
			}
			payload, _ := json.Marshal(map[string]interface{}{"status": "updated", "command": state})
			return string(payload), nil
		},
	}
	categories := map[string]string{
		"record_pulse_worklist":           "workflow",
		"get_pulse_module_state":          "workflow",
		"mark_pulse_module_result":        "workflow",
		"mark_pulse_final_command_result": "workflow",
		"resolve_run_concern":             "workflow",
	}
	resolveConcernTool := llmtypes.Tool{
		Type: "function",
		Function: &llmtypes.FunctionDefinition{
			Name:        "resolve_run_concern",
			Description: "Record a terminal judgement on a step-raised concern returned by get_pulse_module_state. Use 'resolved' only when the underlying problem was actually fixed — if it recurs afterwards the concern reopens automatically, because a fix that did not hold matters more than the original report. Use 'rejected' when it is genuinely not a problem; rejected concerns stay closed even if reported again. Use 'acknowledged' when it is real but deliberately deferred. Never leave a concern open simply because it stopped appearing: absence is not evidence of a fix.",
			Parameters: llmtypes.NewParameters(map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"workspace_path": map[string]interface{}{"type": "string", "description": "Workflow-relative path, e.g. Workflow/social-media."},
					"fingerprint":    map[string]interface{}{"type": "string", "description": "The concern's fingerprint from get_pulse_module_state."},
					"status":         map[string]interface{}{"type": "string", "enum": []string{"acknowledged", "resolved", "rejected"}},
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
		if err := step_based_workflow.ResolveRunConcern(ctx, workspacePath, fingerprint, status, "pulse", note); err != nil {
			return "", err
		}
		return fmt.Sprintf("Concern %s marked %s.", fingerprint, status), nil
	}

	return []llmtypes.Tool{recordTool, stateTool, resultTool, resolveConcernTool, finalCommandTool}, executors, categories
}

func pulseWorklistDecisionsFromArgs(raw interface{}) ([]PulseWorklistDecision, error) {
	arr, ok := raw.([]interface{})
	if !ok {
		return nil, fmt.Errorf("decisions must be an array")
	}
	out := make([]PulseWorklistDecision, 0, len(arr))
	allowed := map[string]bool{
		"module": true, "due": true, "reason": true, "evidence": true,
		"next_check_at": true, "next_check_after_run_id": true, "cooldown_runs": true,
	}
	for index, item := range arr {
		m, ok := item.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("decisions[%d] must be an object", index)
		}
		for key := range m {
			if !allowed[key] {
				return nil, fmt.Errorf("decisions[%d] contains unknown field %q; use the required boolean field due", index, key)
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
		return "", fmt.Errorf("decisions[%d].%s is required and must be a non-empty string", index, key)
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
		return "", fmt.Errorf("decisions[%d].%s must be a string", index, key)
	}
	return value, nil
}

func strictStringSliceToolArg(raw interface{}) ([]string, error) {
	arr, ok := raw.([]interface{})
	if !ok {
		return nil, fmt.Errorf("must be an array of strings")
	}
	out := make([]string, 0, len(arr))
	for index, item := range arr {
		value, ok := item.(string)
		if !ok {
			return nil, fmt.Errorf("item %d must be a string", index)
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
			return 0, fmt.Errorf("must be an integer")
		}
		return integer, nil
	default:
		return 0, fmt.Errorf("must be an integer")
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
