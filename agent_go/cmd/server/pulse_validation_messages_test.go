package server

import (
	"context"
	"strings"
	"testing"

	mcpexecutor "github.com/manishiitg/mcpagent/executor"
)

// These tests pin the information content of Pulse write-path rejections. The
// Fixer is the only stage that mutates state, so a rejection it cannot act on
// costs a completed fix that never gets recorded.
func assertRejectionContains(t *testing.T, err error, wants ...string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected a rejection, got nil")
	}
	for _, want := range wants {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("rejection %q is missing %q", err.Error(), want)
		}
	}
}

// A raw json.Unmarshal failure names an internal Go struct field and never the
// shape that would have worked, which is unusable mid-turn.
func TestPulseToolDecodeFailuresPublishTheExpectedShape(t *testing.T) {
	_, err := pulseFindingDispositionsFromToolArg([]map[string]interface{}{{
		"fingerprint": "fp-1", "finding_id": "PUL-1",
	}})
	assertRejectionContains(t, err,
		"finding_dispositions", "array of disposition objects",
		`"issue_id"`, `"disposition"`, `"summary"`, `"verification"`, `unknown field`)

	_, err = pulseFindingDispositionsFromToolArg("fixed_verified")
	assertRejectionContains(t, err, "array of disposition objects", "not an object or a string")

}

func TestRecordPulseImpactDecodeFailurePublishesTheExpectedShape(t *testing.T) {
	_, executor := createRecordPulseImpactTool()
	ctx := mcpexecutor.WithSessionID(context.Background(), "impact-shape-session")

	_, err := executor(ctx, map[string]interface{}{
		"workspace_path": "Workflow/testing",
		"pulse_run_id":   "impact-shape-run",
		"observations":   "posts_published went up",
	})
	assertRejectionContains(t, err,
		"interventions", "observations", "assessments", "array of objects",
		"never a string or a bare object", `"criterion_id"`, `"run_id"`, `"observed_at"`)
}

// The one message that already met the bar named the field the caller meant.
// Every unknown-field rejection must now carry the full allowed set as well.
func TestPulseWorklistUnknownFieldNamesAllowedSetAndSuggestsIntent(t *testing.T) {
	allowed := []string{
		"module", "due", "reason", "evidence",
		"next_check_at", "next_check_after_run_id", "cooldown_runs",
	}
	tests := []struct {
		name string
		item map[string]interface{}
		want []string
	}{
		{
			name: "decision alias still points at the boolean due field",
			item: map[string]interface{}{"module": pulseModuleWorkflowReview, "decision": "due", "reason": "test"},
			want: []string{`unknown field "decision"`, "use the required boolean field due"},
		},
		{
			name: "status alias points at due",
			item: map[string]interface{}{"module": pulseModuleWorkflowReview, "status": "due", "reason": "test"},
			want: []string{`unknown field "status"`, "use the required boolean field due"},
		},
		{
			name: "rationale points at reason",
			item: map[string]interface{}{"module": pulseModuleWorkflowReview, "due": true, "rationale": "test"},
			want: []string{`unknown field "rationale"`, `did you mean "reason"`},
		},
		{
			name: "near-miss spelling points at the field it nearly matched",
			item: map[string]interface{}{"module": pulseModuleWorkflowReview, "due": true, "reason": "r", "cooldown": 2},
			want: []string{`unknown field "cooldown"`, `did you mean "cooldown_runs"`},
		},
		{
			name: "unrecognized field still gets the complete allowed set",
			item: map[string]interface{}{"module": pulseModuleWorkflowReview, "due": true, "reason": "r", "zzz": 1},
			want: []string{`unknown field "zzz"`},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := pulseWorklistDecisionsFromArgs([]interface{}{tt.item})
			assertRejectionContains(t, err, append(tt.want, allowed...)...)
		})
	}
}

func TestPulseWorklistArgumentShapeRejectionsAreActionable(t *testing.T) {
	_, err := pulseWorklistDecisionsFromArgs("workflow_review")
	assertRejectionContains(t, err, "decisions must be an array", `"module"`, `"due"`, `"reason"`)

	_, err = pulseWorklistDecisionsFromArgs([]interface{}{"workflow_review"})
	assertRejectionContains(t, err, "decisions[0] must be an object", `"module"`, `"due"`, `"reason"`, "string")

	_, err = pulseWorklistDecisionsFromArgs([]interface{}{
		map[string]interface{}{"module": pulseModuleWorkflowReview, "due": true},
	})
	assertRejectionContains(t, err, "decisions[0].reason", "module, due, and reason", "nothing")

	_, err = pulseWorklistDecisionsFromArgs([]interface{}{
		map[string]interface{}{"module": pulseModuleWorkflowReview, "due": true, "reason": "r", "cooldown_runs": "two"},
	})
	assertRejectionContains(t, err, "decisions[0].cooldown_runs", "integer", "a string")

	_, err = pulseWorklistDecisionsFromArgs([]interface{}{
		map[string]interface{}{"module": pulseModuleWorkflowReview, "due": true, "reason": "r", "evidence": "one thing"},
	})
	assertRejectionContains(t, err, "decisions[0].evidence", "array of strings", "a string")
}

// Every closed set in the worklist path must print its members.
func TestPulseWorklistClosedSetRejectionsPrintTheirMembers(t *testing.T) {
	ctx := context.Background()

	decisions := completePulseWorklistDecisions(nil)
	decisions[0].Module = "bugs"
	err := validatePulseWorklistDecisions(decisions)
	assertRejectionContains(t, err, append([]string{`module "bugs" is not a valid Pulse module`, "Must be one of:"}, pulseModuleOrder...)...)

	err = validatePulseWorklistDecisions(nil)
	assertRejectionContains(t, err, append([]string{"decisions are required"}, pulseModuleOrder...)...)

	err = validatePulseWorklistDecisions(completePulseWorklistDecisions(nil)[:1])
	assertRejectionContains(t, err, append([]string{"exactly one entry for each Pulse module"}, pulseModuleOrder...)...)

	_, err = markPulseModuleResult(ctx, t.TempDir(), pulseModuleWorkflowReview, "pulse-1", "finished", "reason", nil)
	assertRejectionContains(t, err, `result "finished" is not valid`, "Must be one of:",
		"done", "changed", "blocked", "failed", "skipped", "timed_out")

	_, err = markPulseModuleResultFromAgentWithAuditAndFindings(
		ctx, t.TempDir(), pulseModuleWorkflowReview, "pulse-1", "finished", "reason", nil,
		PulseModuleAuditInput{}, nil,
	)
	assertRejectionContains(t, err, `result "finished" is not valid`, "Must be one of:",
		"done", "changed", "blocked", "failed", "skipped")

	_, err = markPulseModuleResult(ctx, t.TempDir(), "bugs", "pulse-1", "done", "reason", nil)
	assertRejectionContains(t, err, append([]string{"is not a valid Pulse module"}, pulseModuleOrder...)...)
}

// result=changed used to teach its contract one rejected write at a time.
func TestMarkPulseModuleResultChangedNamesTheWholeRequiredSet(t *testing.T) {
	_, executors, _ := createPulseWorklistTools()
	execute := executors["record_pulse_result"].(func(context.Context, map[string]interface{}) (string, error))
	ctx := mcpexecutor.WithSessionID(context.Background(), "changed-set-session")

	base := map[string]interface{}{
		"workspace_path": "Workflow/testing",
		"pulse_run_id":   "changed-set-run",
		"module":         pulseModuleWorkflowReview,
		"result":         "changed",
		"reason":         "fixed the selector",
	}
	_, err := execute(ctx, base)
	assertRejectionContains(t, err,
		"changed_files", "verification", "finding_dispositions",
		"changed_files=0 items", "verification=0 items", "finding_dispositions=0 items")

	withFiles := map[string]interface{}{}
	for key, value := range base {
		withFiles[key] = value
	}
	withFiles["changed_files"] = []interface{}{"planning/step_config.json"}
	_, err = execute(ctx, withFiles)
	assertRejectionContains(t, err,
		"verification is required", "changed_files=1 items", "verification=0 items", "finding_dispositions=0 items")

	withVerification := map[string]interface{}{}
	for key, value := range withFiles {
		withVerification[key] = value
	}
	withVerification["verification"] = []interface{}{"ran the suite"}
	_, err = execute(ctx, withVerification)
	assertRejectionContains(t, err,
		"finding_dispositions is required", "changed_files=1 items", "verification=1 items",
		"array of disposition objects", `"disposition"`)

	unpaired := map[string]interface{}{}
	for key, value := range base {
		unpaired[key] = value
	}
	unpaired["before_refs"] = []interface{}{"a", "b"}
	_, err = execute(ctx, unpaired)
	assertRejectionContains(t, err, "before_refs", "after_refs", "before_refs=2", "after_refs=0")
}
