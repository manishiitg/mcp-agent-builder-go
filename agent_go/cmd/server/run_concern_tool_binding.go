package server

import (
	"context"
	"fmt"

	virtualtools "github.com/manishiitg/coding-agent-loop/agent_go/cmd/server/virtual-tools"
	step_based_workflow "github.com/manishiitg/coding-agent-loop/agent_go/pkg/orchestrator/agents/workflow/step_based_workflow"
)

// PLAT-055. Bridges the step-facing record_run_concern tool to the workflow
// package's lifecycle writer. The tool layer stays free of a workflow-package
// dependency; this file owns the one translation between them.

func recordStepRunConcernFromTool(
	ctx context.Context,
	workspacePath, runFolder, groupName, stepID, phase string,
	payload map[string]any,
) (string, error) {
	input := step_based_workflow.StepRunConcernInput{
		Concern: stringToolArg(payload, "concern"),
		PulseFindingDetails: step_based_workflow.PulseFindingDetails{
			TargetKey:      stringToolArg(payload, "target_key"),
			IssueKind:      stringToolArg(payload, "issue_kind"),
			Classification: stringToolArg(payload, "classification"),
			Severity:       stringToolArg(payload, "severity"),
			Summary:        stringToolArg(payload, "summary"),
			Impact:         stringToolArg(payload, "impact"),
			Workaround:     stringToolArg(payload, "workaround"),
			NextCheck:      stringToolArg(payload, "next_check"),
			Evidence:       stringSliceFromToolArg(payload["evidence"]),
		},
	}
	if raw, ok := payload["reproduction"].(map[string]interface{}); ok {
		input.Reproduction.Safe, _ = raw["safe"].(bool)
		input.Reproduction.Setup, _ = raw["setup"].(string)
		input.Reproduction.Action, _ = raw["action"].(string)
		input.Reproduction.Expected, _ = raw["expected"].(string)
		input.Reproduction.Observed, _ = raw["observed"].(string)
		input.Reproduction.Limitations, _ = raw["limitations"].(string)
	}

	record, err := step_based_workflow.RecordStepRunConcern(ctx, workspacePath, runFolder, groupName, stepID, phase, input)
	if err != nil {
		return "", fmt.Errorf("record_run_concern failed: %w", err)
	}
	return virtualtools.EncodeRunConcernResult(record.Fingerprint, record.Phase, record.StepID, record.Duplicate), nil
}
