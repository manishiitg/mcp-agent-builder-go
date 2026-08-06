package step_based_workflow

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/orchestrator"
)

// This is the regression that the old run_folders schema could not express:
// two real executions reuse iteration-0/default, then rotation archives the
// first path. Their spend must remain two records, never one mixed aggregate.
func TestExecutionKeyedCostLedgerSeparatesIterationZeroReuse(t *testing.T) {
	hcpo, _ := newEvalReportPhaseWiringTestOrchestrator(t)
	ctx := context.Background()
	runFolder := "iteration-0/default"

	for _, event := range []struct {
		executionID string
		input       int
	}{
		{executionID: "execution-A", input: 100},
		{executionID: "execution-B", input: 250},
	} {
		err := hcpo.PersistTokenUsage(ctx, runFolder,
			&orchestrator.StepTokenData{Phase: "execution_only", StepID: "collect", ExecutionID: event.executionID},
			&orchestrator.ModelTokenData{Provider: "codex-cli", ModelID: "gpt-5.6-terra", InputTokens: event.input, LLMCallCount: 1},
		)
		if err != nil {
			t.Fatalf("PersistTokenUsage(%s): %v", event.executionID, err)
		}
	}

	path := filepath.Join("costs", "execution", "default", orchestrator.CostDateKey(time.Now())+".json")
	content, err := hcpo.ReadWorkspaceFile(ctx, path)
	if err != nil {
		t.Fatalf("read execution ledger: %v", err)
	}
	var daily orchestrator.DailyGroupTokenUsageFile
	if err := json.Unmarshal([]byte(content), &daily); err != nil {
		t.Fatalf("decode execution ledger: %v", err)
	}
	if len(daily.Executions) != 2 || daily.Executions["execution-A"] == nil || daily.Executions["execution-B"] == nil {
		t.Fatalf("executions = %#v, want separate execution-A and execution-B records", daily.Executions)
	}
	if daily.Executions["execution-A"].TokenUsage.ByModel["gpt-5.6-terra"].InputTokens != 100 {
		t.Fatal("execution-A was merged with a later reuse of iteration-0/default")
	}
	if daily.Executions["execution-B"].TokenUsage.ByModel["gpt-5.6-terra"].InputTokens != 250 {
		t.Fatal("execution-B token total was not retained independently")
	}

}

func TestEvaluationLedgerSeparatesRepeatedIterationZeroEvaluations(t *testing.T) {
	hcpo, _ := newEvalReportPhaseWiringTestOrchestrator(t)
	ctx := context.Background()
	const runFolder = "iteration-0/default"
	const generatedAt = "2026-08-06T06:00:00Z"

	for _, evaluationID := range []string{"evaluation-A", "evaluation-B"} {
		report := &EvaluationReport{EvaluationID: evaluationID, TargetRunFolder: runFolder, GeneratedAt: generatedAt}
		if err := hcpo.persistEvaluationScoreLedger(ctx, report, runFolder); err != nil {
			t.Fatalf("persistEvaluationScoreLedger(%s): %v", evaluationID, err)
		}
	}

	path := filepath.Join("scores", "evaluation", "default", "2026-08-06.json")
	content, err := hcpo.ReadWorkspaceFile(ctx, path)
	if err != nil {
		t.Fatalf("read evaluation ledger: %v", err)
	}
	var daily EvaluationScoreDailyFile
	if err := json.Unmarshal([]byte(content), &daily); err != nil {
		t.Fatalf("decode evaluation ledger: %v", err)
	}
	if len(daily.Evaluations) != 2 || daily.Evaluations["evaluation-A"] == nil || daily.Evaluations["evaluation-B"] == nil {
		t.Fatalf("evaluations = %#v, want two immutable evaluation records", daily.Evaluations)
	}
	if len(daily.RunFolders) != 0 {
		t.Fatalf("new evaluation writes must not fall back to reusable run_folders: %#v", daily.RunFolders)
	}
}
