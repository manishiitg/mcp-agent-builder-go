package step_based_workflow

import (
	"fmt"
	"strings"
)

// A plan has at most one routing step (PLAT-294).
//
// Since PLAT-259, `routing` means the workflow's mode selector: the single
// major fork whose route a schedule or caller picks via route_selections
// (full_audit vs apply_approved_fixes, post vs engage vs measure, ...). Every
// other fixed choice inside the flow is a `branch`. Reviewing the workflows
// that grew a second routing step showed each one was really a branch (a
// skip/continue gate, a publish/redraft/hold decision, a router whose routes
// all land on the same step), and the agentic routing_best_practices check
// passed some of them anyway -- so this one rule is enforced deterministically.
//
// Deliberately enforced only on mutations that would introduce a routing step
// (add_routing_step, convert_routing_branch_step_type to routing), never on
// plan load or plan write in general: plans authored before the rule keep
// their extra routing steps untouched, and every other tool keeps working on
// them. There is no migration.

// existingRoutingStepID returns the id of the first routing step in the plan
// other than excludeID, or "" when there is none. It looks at top-level and
// orphan steps (an orphan can be re-attached later), not at sub-agent steps
// nested inside orchestrator routes, which are their own scope.
func existingRoutingStepID(plan *PlanningResponse, excludeID string) string {
	if plan == nil {
		return ""
	}
	excludeID = strings.TrimSpace(excludeID)
	scan := func(steps []PlanStepInterface) string {
		for _, step := range steps {
			routing, ok := step.(*RoutingPlanStep)
			if !ok {
				continue
			}
			if id := strings.TrimSpace(routing.GetID()); id != "" && id != excludeID {
				return id
			}
		}
		return ""
	}
	if id := scan(plan.Steps); id != "" {
		return id
	}
	return scan(plan.OrphanSteps)
}

// validateRoutingRoutesStartSubWorkflows rejects a routing step whose route
// points straight at "end". A routing step is the mode selector and each of
// its routes is a sub-workflow of the main work -- many steps -- so every route
// must start at a real step. A "mode" that does nothing is a simple
// if-condition (skip/continue, probe ok/failed, hold/publish), which is what
// `branch` is for. Across the reviewed workflows this was a clean tell: every
// real mode selector had zero routes to end, every misclassified second
// routing step had one. Target existence is already enforced by the plan-write
// graph validator (PLAN_GRAPH_INVALID), so only the end sentinel is checked
// here.
func validateRoutingRoutesStartSubWorkflows(stepID string, routes []RoutingRoute) error {
	for _, route := range routes {
		if strings.EqualFold(strings.TrimSpace(route.NextStepID), "end") {
			return fmt.Errorf("routing step %q route %q points at end; a routing step is the mode selector and every route must start a sub-workflow (a real step with its own downstream work). A route that does nothing is a simple if-condition -- add %q as a branch step (add_branch_step) instead, or route this option to the step that begins its sub-workflow", stepID, strings.TrimSpace(route.RouteID), stepID)
		}
	}
	return nil
}

// validateSingleRoutingStepForMutation rejects a mutation that would leave the
// plan with two routing steps. newStepID is the step being added or converted;
// it is excluded from the scan so converting a step to routing does not trip
// over itself. The error is written for the builder agent: it names the
// existing routing step and the two legitimate ways forward.
func validateSingleRoutingStepForMutation(plan *PlanningResponse, newStepID string) error {
	existing := existingRoutingStepID(plan, newStepID)
	if existing == "" {
		return nil
	}
	return fmt.Errorf("plan already has a routing step %q; a plan has at most one routing step -- the workflow's mode selector, the step whose route a schedule or caller picks via route_selections. Add %q as a branch step (add_branch_step) instead: every fixed choice below the mode selector is an in-flow decision. If %q really is the mode selector, first convert %q to branch with convert_routing_branch_step_type, then retry. Existing plans keep any extra routing steps they already have; only new routing steps are checked", existing, newStepID, newStepID, existing)
}
