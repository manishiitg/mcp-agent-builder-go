package step_based_workflow

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/orchestrator"
	loggerv2 "github.com/manishiitg/mcpagent/logger/v2"
)

// PLAT-259: branch is a new step type, same shape/executor as routing (now
// the "route"/major-fork concept). These tests mirror the existing routing
// coverage, substituting BranchPlanStep/branch, to prove the shared
// routeSwitchStep interface refactor in controller_routing.go/
// controller_routing_deterministic.go genuinely works for branch and not
// just for routing.

func TestValidateBranchStepRejectsDescription(t *testing.T) {
	step := &BranchPlanStep{
		CommonStepFields: CommonStepFields{
			ID:          "branch-by-mode",
			Title:       "Branch by Mode",
			Description: "Decide the route first.",
		},
		BranchQuestion: "Which path should run?",
		Routes:         deterministicRoutingTestRoutes(),
	}

	err := validateBranchStepFieldsTyped(step)
	if err == nil {
		t.Fatal("expected branch description to be rejected")
	}
	if !strings.Contains(err.Error(), "must not set description") {
		t.Fatalf("expected deterministic-only description error, got %v", err)
	}
}

func TestValidateBranchStepRequiresTwoRoutes(t *testing.T) {
	step := &BranchPlanStep{
		CommonStepFields: CommonStepFields{ID: "branch-one-route", Title: "One Route"},
		BranchQuestion:   "Which path?",
		Routes:           []RoutingRoute{{RouteID: "only", RouteName: "Only", NextStepID: "step-only"}},
	}

	err := validateBranchStepFieldsTyped(step)
	if err == nil {
		t.Fatal("expected branch step with < 2 routes to be rejected")
	}
	if !strings.Contains(err.Error(), "at least 2 routes") {
		t.Fatalf("expected minimum-routes error, got %v", err)
	}
}

func TestValidateBranchStepAcceptsWellFormedStep(t *testing.T) {
	step := &BranchPlanStep{
		CommonStepFields: CommonStepFields{ID: "branch-ok", Title: "OK"},
		BranchQuestion:   "Which path?",
		Routes:           deterministicRoutingTestRoutes(),
		DefaultRouteID:   "route-search",
	}

	if err := validateBranchStepFieldsTyped(step); err != nil {
		t.Fatalf("expected well-formed branch step to pass, got %v", err)
	}
}

func TestBranchStepOwnRouteFileCandidatesIgnoresLegacyDescription(t *testing.T) {
	base, err := orchestrator.NewBaseOrchestrator(
		loggerv2.NewDefault(),
		nil,
		orchestrator.OrchestratorTypeWorkflow,
		"Workflow/demo",
		0,
		"",
		nil,
		nil,
		false,
		&orchestrator.LLMConfig{},
		1,
		nil,
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("NewBaseOrchestrator returned error: %v", err)
	}
	hcpo := &StepBasedWorkflowOrchestrator{
		BaseOrchestrator:  base,
		selectedRunFolder: "run-1",
	}
	step := &BranchPlanStep{
		CommonStepFields: CommonStepFields{
			ID:          "branch-by-mode",
			Description: "legacy agent body",
		},
	}

	candidates := hcpo.routingStepOwnRouteFileCandidates(step, 0, "step-1")
	if len(candidates) != 1 {
		t.Fatalf("expected one own route file candidate, got %d: %v", len(candidates), candidates)
	}
	if strings.Contains(candidates[0], "step-1-branch") {
		t.Fatalf("legacy execute path should not be considered, got %q", candidates[0])
	}
}

func TestIsRoutingStepRecognizesBranchStep(t *testing.T) {
	if !isRoutingStep(&BranchPlanStep{}) {
		t.Fatal("isRoutingStep should recognize *BranchPlanStep")
	}
	if !isRoutingStep(&RoutingPlanStep{}) {
		t.Fatal("isRoutingStep should still recognize *RoutingPlanStep")
	}
	if isRoutingStep(&RegularPlanStep{}) {
		t.Fatal("isRoutingStep should not recognize *RegularPlanStep")
	}
}

func TestBranchPlanStepMarshalJSONAlwaysSetsType(t *testing.T) {
	step := &BranchPlanStep{
		CommonStepFields: CommonStepFields{ID: "branch-a", Title: "Branch A"},
		BranchQuestion:   "Which path?",
		Routes:           deterministicRoutingTestRoutes(),
	}

	raw, err := json.Marshal(step)
	if err != nil {
		t.Fatalf("Marshal returned error: %v", err)
	}
	var decoded map[string]interface{}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("Unmarshal into map failed: %v", err)
	}
	if decoded["type"] != "branch" {
		t.Fatalf("decoded type = %v, want %q", decoded["type"], "branch")
	}
	if decoded["branch_question"] != "Which path?" {
		t.Fatalf("decoded branch_question = %v, want %q", decoded["branch_question"], "Which path?")
	}
}

func TestParseStepFromJSONHandlesBranchType(t *testing.T) {
	raw := `{"type":"branch","id":"branch-a","title":"Branch A","branch_question":"Which path?","routes":[
		{"route_id":"route-search","route_name":"Search","condition":"c","next_step_id":"step-search"},
		{"route_id":"route-save","route_name":"Save","condition":"c","next_step_id":"step-save"}
	]}`

	step, err := parseStepFromJSON(json.RawMessage(raw), 0, "step")
	if err != nil {
		t.Fatalf("parseStepFromJSON returned error: %v", err)
	}
	branchStep, ok := step.(*BranchPlanStep)
	if !ok {
		t.Fatalf("parseStepFromJSON returned %T, want *BranchPlanStep", step)
	}
	if branchStep.StepType() != StepTypeBranch {
		t.Fatalf("StepType() = %q, want %q", branchStep.StepType(), StepTypeBranch)
	}
	if len(branchStep.Routes) != 2 {
		t.Fatalf("expected 2 routes, got %d", len(branchStep.Routes))
	}
}
