package step_based_workflow

import (
	"context"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/orchestrator"
	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/workspace"
	workspacehandlers "github.com/manishiitg/coding-agent-loop/workspace/handlers"
	loggerv2 "github.com/manishiitg/mcpagent/logger/v2"
	"github.com/spf13/viper"
)

// newEvalReportEnrichmentTestOrchestrator wires a REAL workspace-docs HTTP
// server backed by a real temp directory, so enrichEvaluationReportWithStepOutputs
// exercises its real hcpo.ReadWorkspaceFile path instead of a mock.
func newEvalReportEnrichmentTestOrchestrator(t *testing.T) (*StepBasedWorkflowOrchestrator, string) {
	t.Helper()

	base, err := orchestrator.NewBaseOrchestrator(
		loggerv2.NewDefault(), nil, orchestrator.OrchestratorTypeWorkflow,
		"", 0, "", nil, nil, false, &orchestrator.LLMConfig{}, 1, nil, nil, nil,
	)
	if err != nil {
		t.Fatalf("NewBaseOrchestrator: %v", err)
	}
	workflowRelPath := "Workflow/eval-report-enrichment-test"
	hcpo := &StepBasedWorkflowOrchestrator{BaseOrchestrator: base}

	docsDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(docsDir, workflowRelPath), 0o755); err != nil {
		t.Fatalf("mkdir workflow dir: %v", err)
	}
	gin.SetMode(gin.TestMode)
	viper.Set("docs-dir", docsDir)
	router := gin.New()
	router.Any("/api/documents/*filepath", workspacehandlers.HandleDocumentRequest)
	wsServer := httptest.NewServer(router)
	t.Cleanup(wsServer.Close)

	hcpo.WorkspaceClient = workspace.NewClient(wsServer.URL)
	hcpo.SetWorkspacePath(workflowRelPath)

	return hcpo, filepath.Join(docsDir, workflowRelPath)
}

// TestEnrichEvaluationReportWithStepOutputsExtractsRealVerdict is the
// integration-level counterpart to the extractEvalVerdictFromOutputContent
// unit tests: it proves the extraction is actually WIRED IN to
// enrichEvaluationReportWithStepOutputs, not just correct as a standalone
// function. Writes a real output_content.json to a real workspace-docs
// server, at the exact path the real pipeline would produce, using the
// real shape found live in social-media's eval-workflow-success step.
func TestEnrichEvaluationReportWithStepOutputsExtractsRealVerdict(t *testing.T) {
	hcpo, docsRoot := newEvalReportEnrichmentTestOrchestrator(t)
	ctx := context.Background()

	evalExecutionPath := "evaluation/runs/iteration-0/default/execution"
	stepDir := filepath.Join(docsRoot, evalExecutionPath, "eval-workflow-success")
	if err := os.MkdirAll(stepDir, 0o755); err != nil {
		t.Fatalf("mkdir eval step dir: %v", err)
	}
	realOutput := `{
		"cdp_connected": true,
		"follower_delta_7d": -2,
		"score": 0.0,
		"max_score": 10,
		"pass_fail_reason": "Score 0.0/10. Top deductions: -4: follower_delta_7d declining (-2)."
	}`
	if err := os.WriteFile(filepath.Join(stepDir, "output_content.json"), []byte(realOutput), 0o644); err != nil {
		t.Fatalf("write output_content.json: %v", err)
	}

	plan := &EvaluationPlan{Steps: []*EvaluationStep{{ID: "eval-workflow-success"}}}
	report := &EvaluationReport{
		StepScores: []*EvaluationStepScore{{
			StepID:    "eval-workflow-success",
			Reasoning: "No score captured — this eval step produced no output_content, or output_content had no score field.",
			Evidence:  "No output_content found for this step.",
		}},
	}

	hcpo.enrichEvaluationReportWithStepOutputs(ctx, report, plan, evalExecutionPath)

	if len(report.StepScores) != 1 {
		t.Fatalf("expected 1 step score, got %d", len(report.StepScores))
	}
	score := report.StepScores[0]
	if score.OutputContent == nil {
		t.Fatal("expected OutputContent to be populated from the real file")
	}
	if score.MaxScore != 10 {
		t.Fatalf("MaxScore = %d, want 10 (real verdict must reach the report through the actual pipeline call, not just the leaf function)", score.MaxScore)
	}
	if score.Reasoning == "No score captured — this eval step produced no output_content, or output_content had no score field." {
		t.Fatal("Reasoning still shows the stub — extraction was not wired into enrichEvaluationReportWithStepOutputs")
	}
}
