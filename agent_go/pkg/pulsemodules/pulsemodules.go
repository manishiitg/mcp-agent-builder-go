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

// All is the canonical, ordered module set. Order is the Pulse worklist order
// and is part of the contract — the scheduler and UI both rely on it.
var All = []Module{
	{
		ID:        "bug_review",
		Label:     "Bug review",
		StepLabel: "bug-review",
	},
	{
		ID:        "artifact_review",
		Label:     "Plan drift",
		StepLabel: "artifact",
		Aliases:   []string{"artifact", "artifact_drift"},
	},
	{
		ID:        "report_health",
		Label:     "Report health",
		StepLabel: "report-health",
		Aliases:   []string{"report", "reporting", "report_repair"},
	},
	{
		ID:        "eval_health",
		Label:     "Eval health",
		StepLabel: "eval-health",
		Aliases:   []string{"eval", "evaluation", "evaluation_health", "eval_repair"},
	},
	{
		// stores_health replaced three separate modules that shared one
		// due-cadence mechanism, one freshness check, one plan_change_backlog
		// trigger, and one bounded-fix authority. Only the content domain
		// differed (learnings HOW / KB facts / DB schema).
		ID:        "stores_health",
		Label:     "Stores health",
		StepLabel: "stores-health",
		Aliases: []string{
			"learnings", "learning", "learning_policy", "learning_health",
			"kb", "knowledgebase", "knowledgebase_health",
			"db", "database", "db_health",
		},
	},
	{
		ID:        "cost_llm_time",
		Label:     "Cost + time",
		StepLabel: "cost-llm-time",
		Aliases:   []string{"cost", "llm_cost", "cost_time"},
	},
	{
		// Also owns plan-design hygiene (step-type fitness, prevalidation
		// fitness, schema/description drift), which Goal Advisor's contract
		// explicitly excludes.
		ID:        "llm_ops_review",
		Label:     "Steps & setup",
		StepLabel: "llm-ops-review",
	},
	{
		ID:        "goal_advisor",
		Label:     "Goal Advisor",
		StepLabel: "goal-advisor",
		Aliases:   []string{"advisor"},
	},
}

// RetiredIDs were once canonical module IDs. They are no longer scheduled, but
// historical reviewer artifacts under pulse/reviews/<run>/<module>.md and old
// builder/improve.html cards still carry them, so read paths must keep
// accepting them. They must never be written by current runs.
var RetiredIDs = []string{"learning_health", "knowledgebase_health", "db_health"}

// PseudoIDs are data-module values that appear in builder/improve.html but are
// not scheduled review modules: run_summary covers Gate and run rows,
// pulse_fixer covers applied fixes. Consumers that classify HTML must accept
// them; the scheduler must not treat them as modules.
var PseudoIDs = []string{"run_summary", "pulse_fixer"}

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

// AcceptedForReviewArtifacts is the set of module IDs a reviewer artifact path
// may use: every current module plus every retired one, so historical results
// stay readable. Omitting a current module here silently breaks that module's
// result persistence — the exact 2026-07-29 stores_health defect.
func AcceptedForReviewArtifacts() []string {
	out := append(IDs(), RetiredIDs...)
	return out
}
