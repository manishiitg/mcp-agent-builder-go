package step_based_workflow

import (
	"reflect"
	"strings"
	"testing"
)

func TestParseWorkflowStepStringMapAcceptsBothDecodedShapes(t *testing.T) {
	tests := []struct {
		name string
		raw  interface{}
	}{
		{name: "JSON decoder", raw: map[string]interface{}{"step-a": "alpha", "step-b": "beta"}},
		{name: "internal caller", raw: map[string]string{"step-a": "alpha", "step-b": "beta"}},
	}
	want := map[string]string{"step-a": "alpha", "step-b": "beta"}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseWorkflowStepStringMap(map[string]interface{}{"human_inputs": tt.raw}, "human_inputs")
			if err != nil {
				t.Fatalf("parseWorkflowStepStringMap returned error: %v", err)
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("parsed values = %#v, want %#v", got, want)
			}
		})
	}
}

func TestParseWorkflowStepStringMapFailsClosed(t *testing.T) {
	tests := []struct {
		name string
		raw  interface{}
		want string
	}{
		{name: "not an object", raw: "step-a=alpha", want: "must be an object"},
		{name: "non string value", raw: map[string]interface{}{"step-a": 42}, want: "must be a string"},
		{name: "empty value", raw: map[string]interface{}{"step-a": "  "}, want: "must be a non-empty string"},
		{name: "empty key", raw: map[string]interface{}{"  ": "alpha"}, want: "non-empty step IDs"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseWorkflowStepStringMap(map[string]interface{}{"human_inputs": tt.raw}, "human_inputs")
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want substring %q", err, tt.want)
			}
		})
	}
}

func TestExecutionContextForStepScopesInputsWithoutLeakage(t *testing.T) {
	base := &ExecutionContext{
		SkipHumanInput: true,
		HumanInputs: map[string]string{
			"step-a": "alpha-only",
			"step-b": "beta-only",
		},
	}

	stepA := executionContextForStep(base, "step-a")
	stepB := executionContextForStep(base, "step-b")
	stepC := executionContextForStep(base, "step-c")

	if got := stepA.WorkshopHumanInput; got != "alpha-only" {
		t.Fatalf("step-a input = %q, want alpha-only", got)
	}
	if got := stepB.WorkshopHumanInput; got != "beta-only" {
		t.Fatalf("step-b input = %q, want beta-only", got)
	}
	if got := stepC.WorkshopHumanInput; got != "" {
		t.Fatalf("unkeyed step input leaked: %q", got)
	}
	if base.WorkshopHumanInput != "" {
		t.Fatalf("base context was mutated: %q", base.WorkshopHumanInput)
	}

	stepA.HumanInputs["step-b"] = "mutated"
	if base.HumanInputs["step-b"] != "beta-only" || stepB.HumanInputs["step-b"] != "beta-only" {
		t.Fatal("step-scoped context shares its HumanInputs map")
	}
}

func TestUnknownWorkflowStepInputIDs(t *testing.T) {
	steps := []PlanStepInterface{
		&MessageSequencePlanStep{CommonStepFields: CommonStepFields{ID: "step-a"}},
		&MessageSequencePlanStep{CommonStepFields: CommonStepFields{ID: "step-b"}},
	}
	got := unknownWorkflowStepInputIDs(steps, map[string]string{
		"step-b":        "known",
		"missing-zeta":  "unknown",
		"missing-alpha": "unknown",
	})
	want := []string{"missing-alpha", "missing-zeta"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unknown IDs = %#v, want %#v", got, want)
	}
}

func TestExecutionContextForStepPreservesExplicitSingleStepInput(t *testing.T) {
	base := &ExecutionContext{
		WorkshopHumanInput: "explicit execute_step input",
		HumanInputs:        map[string]string{"step-a": "full-run input"},
	}
	got := executionContextForStep(base, "step-a")
	if got.WorkshopHumanInput != "explicit execute_step input" {
		t.Fatalf("explicit input was overwritten: %q", got.WorkshopHumanInput)
	}
}
