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
// A 2026-07-29 refactor merged several maintenance lenses without updating all
// consumers. Two production failures followed: a missing ToolCategories entry
// blocked every todo_task orchestrator, and a missing reviewer-whitelist entry
// silently discarded a whole review result. Both built cleanly and passed the
// full test suite.
//
// The registry is a leaf package with no imports because both cmd/server and
// pkg/orchestrator/.../step_based_workflow must consume it, and server already
// imports step_based_workflow — so the shared truth cannot live in either.
package pulsemodules

import "strings"

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
	LLMOpsReviewID    = "llm_ops_review"
	StrategyAuditorID = "strategy_auditor"
	GoalAdvisorID     = "goal_advisor"
)

// HTML-only classifications. They are not scheduled review modules.
const (
	PseudoRunSummaryID = "run_summary"
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
		Aliases:   []string{"workflow", "review", "engineering", "engineering_review", "correctness", "correctness_review"},
	},
	{
		ID:        LLMOpsReviewID,
		Label:     "LLM & operations",
		StepLabel: "llm-ops-review",
		Aliases:   []string{"ops", "operations"},
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

// PseudoIDs are data-module values that appear in builder/improve.html but are
// not scheduled review modules. run_summary covers Gate and run rows; fixes
// and decisions belong to their actual Engineering, Operations, Strategy, or
// Goal Advisor source rather than a synthetic "Pulse fixer" lane.
var PseudoIDs = []string{PseudoRunSummaryID}

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

// Normalize maps current shorthand and loosely-cased spellings onto a canonical
// ID. Retired module identities are deliberately not translated.
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

// AcceptedForReviewReceipts is the current module set accepted for durable
// reviewer receipts.
func AcceptedForReviewReceipts() []string {
	return IDs()
}
