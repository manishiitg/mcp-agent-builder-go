package virtualtools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/common"
	"github.com/manishiitg/multi-llm-provider-go/llmtypes"
)

// RunConcernToolCategory groups the step-facing concern tool.
const RunConcernToolCategory = "workflow_db"

// PLAT-055. Before this, a step's only way to report anything that was not a
// learning was a `CONCERNS:` line in its completion summary — one unstructured
// string, scraped by Go. That is why 39 of 103 active Upwork concerns carry no
// severity, classification, or evidence, and why a multi-paragraph
// harness-bug analysis ended up filed in a learnings file instead: the
// sanctioned outlet had no room for it.
//
// Identity is resolved from trusted session state rather than tool arguments,
// exactly as the workflow-DB tools resolve their workspace. run_concerns dedups
// on a fingerprint of (step_id, concern), and Pulse Gate weighs seen_count when
// deciding whether a root cause deserves repair — so letting a model name its
// own step would split one recurring defect into several apparent one-offs.

// RunConcernRecorder files one structured concern. Injected so the tool layer
// does not depend on the workflow package.
type RunConcernRecorder func(ctx context.Context, workspacePath, runFolder, groupName, stepID, phase string, payload map[string]any) (string, error)

func recordRunConcernToolDefinition() llmtypes.Tool {
	return llmtypes.Tool{Type: "function", Function: &llmtypes.FunctionDefinition{
		Name: "record_run_concern",
		Description: "Report a defect, contradiction, or platform problem you observed during this step, as a structured record Pulse consumes. " +
			"Use it for anything that is NOT reusable how-to knowledge: a harness or tool that behaved wrongly, an artifact contract that contradicts itself, " +
			"a step description that conflicts with a binding constraint, or a store that disagrees with another store. " +
			"You are the only party that saw this run happen, so a problem you do not report here is lost. " +
			"Do NOT put such findings in learnings — learnings is for reusable execution technique only. " +
			"Your step identity, run folder, and phase are supplied by the runtime; you do not pass them. " +
			"Filing the same concern twice in one run is safe and does not inflate its recurrence count.",
		Parameters: llmtypes.NewParameters(map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"properties": map[string]any{
				"concern": map[string]any{
					"type":        "string",
					"description": "One line naming the defect itself, not this occurrence of it. This is the dedup key across runs, so describe the problem ('bid-pick-job's declared enum contract is unreachable from the turn that writes the file'), never the incident ('run 14 failed again').",
				},
				"issue_kind": map[string]any{
					"type":        "string",
					"enum":        []string{"workflow_issue", "harness_issue"},
					"description": "workflow_issue: this workflow's own plan, steps, artifacts, or data. harness_issue: the platform behaved wrongly and no workflow-level change can fix it.",
				},
				"classification": map[string]any{
					"type":        "string",
					"description": "Short category, e.g. correctness_bug, artifact_drift, measurement_gap, reliability, store_integrity, efficiency_or_coaching.",
				},
				"severity": map[string]any{
					"type": "string",
					"enum": []string{"critical", "high", "medium", "low"},
				},
				"summary": map[string]any{
					"type":        "string",
					"description": "What is wrong, stated so a reader who did not watch this run can understand it.",
				},
				"impact": map[string]any{
					"type":        "string",
					"description": "What this costs if left unfixed — wrong output, wasted retries, unmeasurable results, silent data loss.",
				},
				"evidence": map[string]any{
					"type":        "array",
					"minItems":    1,
					"items":       map[string]any{"type": "string"},
					"description": "Concrete artifact paths, table rows, error strings, or tool responses that show the problem. Never a claim without a source.",
				},
				"target_key": map[string]any{
					"type":        "string",
					"description": "Stable identifier for the thing that is wrong, e.g. planning/plan.json:bid-pick-job or harness:run_full_workflow. Required for harness_issue.",
				},
				"workaround": map[string]any{
					"type":        "string",
					"description": "Optional. What a future run should do until this is fixed.",
				},
				"next_check": map[string]any{
					"type":        "string",
					"description": "Optional. The exact future evidence that would prove this resolved.",
				},
				"reproduction": map[string]any{
					"type":                 "object",
					"additionalProperties": false,
					"description":          "How to see it again. expected/observed are required for harness_issue.",
					"properties": map[string]any{
						"safe":        map[string]any{"type": "boolean", "description": "True when reproducing causes no outward side effect."},
						"setup":       map[string]any{"type": "string"},
						"action":      map[string]any{"type": "string"},
						"expected":    map[string]any{"type": "string"},
						"observed":    map[string]any{"type": "string"},
						"limitations": map[string]any{"type": "string", "description": "What you could not establish, so a reader does not over-read this."},
					},
				},
			},
			"required": []string{"concern", "issue_kind", "classification", "severity", "summary", "impact", "evidence"},
		}),
	}}
}

// CreateRunConcernToolRegistry builds the step-facing concern tool.
func CreateRunConcernToolRegistry(fallbackSessionID string, record RunConcernRecorder) WorkflowDBToolRegistry {
	executor := func(ctx context.Context, args map[string]any) (string, error) {
		if record == nil {
			return "", fmt.Errorf("record_run_concern is not wired in this session")
		}
		sessionID := strings.TrimSpace(fallbackSessionID)
		if ctx != nil {
			if current, ok := ctx.Value(common.ChatSessionIDKey).(string); ok && strings.TrimSpace(current) != "" {
				sessionID = strings.TrimSpace(current)
			}
		}
		identity, ok := common.GetRunConcernSessionContext(sessionID)
		if !ok || strings.TrimSpace(identity.StepID) == "" {
			// Fail loudly rather than filing against an empty step: a concern
			// with no owner is worse than none, because it still consumes
			// backlog attention while pointing nowhere.
			//
			// Name the destination that does exist. Refusing without one is a
			// dead end, and the caller has already done the expensive part —
			// on 2026-08-17 a Pulse pass lost a fully-evidenced finding ("the
			// evaluator grades an older batch, so a stale 20/20 makes an
			// unevaluated run look fully verified") to this refusal, in a
			// session that called record_pulse_finding successfully 160 times.
			// A refusal the caller cannot act on discards the work silently.
			return "", fmt.Errorf(
				"record_run_concern is unavailable here: it files against the workflow step that raised the concern, and this session has no step identity — it is a Pulse or maintenance session rather than a step. Do not discard this concern: record it with record_pulse_finding, which owns findings raised outside a step. (session %q)",
				sessionID,
			)
		}
		payload := map[string]any{}
		for key, value := range args {
			payload[key] = value
		}
		return record(ctx, identity.WorkspacePath, identity.RunFolder, identity.GroupName, identity.StepID, identity.Phase, payload)
	}

	return WorkflowDBToolRegistry{
		Tools:      []llmtypes.Tool{recordRunConcernToolDefinition()},
		Executors:  map[string]func(context.Context, map[string]any) (string, error){"record_run_concern": executor},
		Categories: map[string]string{"record_run_concern": RunConcernToolCategory},
	}
}

// EncodeRunConcernResult renders a recorder result for the agent.
func EncodeRunConcernResult(fingerprint, phase, stepID string, duplicate bool) string {
	payload := map[string]any{
		"status":      "recorded",
		"fingerprint": fingerprint,
		"phase":       phase,
		"step_id":     stepID,
	}
	if duplicate {
		payload["status"] = "already_recorded_this_run"
		payload["note"] = "This concern was already filed for this run; its recurrence count was not incremented."
	}
	encoded, _ := json.Marshal(payload)
	return string(encoded)
}
