// Package pulsemodules is the single canonical registry of Pulse review
// module identity.
//
// Why this package exists, and why it is a dependency-free leaf:
//
// Module identity was previously restated in nine independently maintained
// places — scheduler constants, an order slice, a validity map, an alias
// normalizer, a scheduler step-label switch, a tool enum, that tool's prose
// description, a reviewer artifact-path whitelist, and two frontend files.
// Nothing asserted they agreed.
//
// On 2026-07-29 a single refactor (merging learning_health,
// knowledgebase_health, and db_health into stores_health) desynchronized four
// of them at once. Two caused production failures: a missing ToolCategories
// entry blocked every todo_task orchestrator, and a missing entry in the
// reviewer whitelist made every stores_health review silently fail to persist
// its result. Both built cleanly and passed the full test suite.
//
// The registry is a leaf package with no imports because both cmd/server and
// pkg/orchestrator/.../step_based_workflow must consume it, and server already
// imports step_based_workflow — so the shared truth cannot live in either.
package pulsemodules

import "strings"

// ReviewerMaxConcurrency is the shared bound for scheduled reviewer stages and
// their read-only child agents. Keeping the scheduler and child runtime on the
// same value prevents queued parent turns from defeating the intended bounded
// parallel review phase.
const ReviewerMaxConcurrency = 3

// Module is one scheduled Pulse review module.
type Module struct {
	// ID is the canonical identifier used in db state, tool payloads,
	// reviewer artifact paths, and HTML data-module attributes.
	ID string
	// Label is the plain-language name shown to operators.
	Label string
	// StepLabel is the scheduler's per-stage label for this module.
	StepLabel string
	// Aliases are shorthand or superseded spellings that normalize to ID.
	// These are accepted as input; they are never emitted.
	Aliases []string
}

// Canonical module IDs. Consumers that need compile-time constants must alias
// these values rather than restating their string literals.
const (
	WorkflowReviewID  = "workflow_review"
	BugReviewID       = "bug_review"
	ArtifactReviewID  = "artifact_review"
	ReportHealthID    = "report_health"
	EvalHealthID      = "eval_health"
	StoresHealthID    = "stores_health"
	LLMOpsReviewID    = "llm_ops_review"
	StrategyAuditorID = "strategy_auditor"
	GoalAdvisorID     = "goal_advisor"
)

// Historical IDs remain constants because read paths must recognize them, but
// current writers must never emit them.
const (
	RetiredLearningHealthID      = "learning_health"
	RetiredKnowledgebaseHealthID = "knowledgebase_health"
	RetiredDBHealthID            = "db_health"
	// CostLLMTimeID remains readable for historical Pulse state and artifacts.
	// New runs fold cost, timing, and tool/LLM operations into LLMOpsReviewID.
	CostLLMTimeID = "cost_llm_time"
)

// HTML-only classifications. They are not scheduled review modules.
const (
	PseudoRunSummaryID = "run_summary"
	PseudoPulseFixerID = "pulse_fixer"
)

// All is the canonical, ordered module set. Order is the Pulse worklist order
// and is part of the contract — the scheduler and UI both rely on it.
var All = []Module{
	{
		// WorkflowReviewID is retained as the durable identity for Engineering
		// Review. Artifact names are evidence packs, not reviewer perspectives:
		// execution, report/eval implementation, plan-change impact, artifact
		// consistency, and store-integrity defects all belong here.
		ID:        WorkflowReviewID,
		Label:     "Engineering review",
		StepLabel: "workflow-review",
		Aliases: []string{
			"workflow", "review", "engineering", "engineering_review", "correctness", "correctness_review",
			"bug", BugReviewID,
			"artifact", "artifact_drift", ArtifactReviewID,
			"report", "reporting", "report_repair", ReportHealthID,
			"eval", "evaluation", "evaluation_health", "eval_repair", EvalHealthID,
			"learnings", "learning", "learning_policy", RetiredLearningHealthID,
			"kb", "knowledgebase", RetiredKnowledgebaseHealthID,
			"db", "database", RetiredDBHealthID, StoresHealthID,
		},
	},
	{
		ID:        LLMOpsReviewID,
		Label:     "LLM & operations",
		StepLabel: "llm-ops-review",
		Aliases:   []string{"ops", "operations", "cost", "llm_cost", "cost_time", CostLLMTimeID},
	},
	{
		// Strategy Auditor improves the selected strategy by finding missing
		// causal stages, weak assumptions, concentration, saturation, and other
		// plan-versus-goal gaps. It is independent from Goal Advisor, which uses
		// a blank-sheet lens to propose materially different approaches.
		ID:        StrategyAuditorID,
		Label:     "Strategy Auditor",
		StepLabel: "strategy-auditor",
		Aliases:   []string{"strategy", "strategy_review", "plan_effectiveness"},
	},
	{
		ID:        GoalAdvisorID,
		Label:     "Goal Advisor",
		StepLabel: "goal-advisor",
		Aliases:   []string{"advisor"},
	},
}

// RetiredIDs were once canonical module IDs. They are no longer scheduled, but
// historical reviewer artifacts under pulse/reviews/<run>/<module>.md and old
// builder/improve.html cards still carry them, so read paths must keep
// accepting them. They must never be written by current runs.
var RetiredIDs = []string{
	BugReviewID,
	ArtifactReviewID,
	ReportHealthID,
	EvalHealthID,
	StoresHealthID,
	RetiredLearningHealthID,
	RetiredKnowledgebaseHealthID,
	RetiredDBHealthID,
	CostLLMTimeID,
}

// PseudoIDs are data-module values that appear in builder/improve.html but are
// not scheduled review modules: run_summary covers Gate and run rows,
// pulse_fixer covers applied fixes. Consumers that classify HTML must accept
// them; the scheduler must not treat them as modules.
var PseudoIDs = []string{PseudoRunSummaryID, PseudoPulseFixerID}

// IDs returns the canonical module IDs in worklist order.
func IDs() []string {
	out := make([]string, 0, len(All))
	for _, m := range All {
		out = append(out, m.ID)
	}
	return out
}

// IsValid reports whether id is a currently scheduled canonical module.
func IsValid(id string) bool {
	for _, m := range All {
		if m.ID == id {
			return true
		}
	}
	return false
}

// IsRetired reports whether id was a canonical module that no longer schedules.
func IsRetired(id string) bool {
	for _, r := range RetiredIDs {
		if r == id {
			return true
		}
	}
	return false
}

// Normalize maps shorthand, superseded, and loosely-cased spellings onto a
// canonical ID. An unrecognized value is returned normalized but unchanged, so
// callers can still reject it explicitly rather than silently remapping it.
func Normalize(module string) string {
	module = strings.ToLower(strings.TrimSpace(module))
	module = strings.ReplaceAll(module, "-", "_")
	for _, m := range All {
		if m.ID == module {
			return m.ID
		}
		for _, a := range m.Aliases {
			if a == module {
				return m.ID
			}
		}
	}
	return module
}

// ForStepLabel maps a scheduler stage label to its canonical module ID, or ""
// when the label is not a module stage (for example "gate" or "finalize").
func ForStepLabel(label string) string {
	for _, m := range All {
		if m.StepLabel == label {
			return m.ID
		}
	}
	return ""
}

// AcceptedForReviewReceipts is the set of module IDs a durable reviewer
// receipt may use: every current module plus every retired one, so historical
// compact records stay readable. Omitting a current module here silently
// breaks that module's result persistence — the exact 2026-07-29 stores_health
// defect.
func AcceptedForReviewReceipts() []string {
	out := append(IDs(), RetiredIDs...)
	return out
}
