package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	step_based_workflow "github.com/manishiitg/coding-agent-loop/agent_go/pkg/orchestrator/agents/workflow/step_based_workflow"
	mcpexecutor "github.com/manishiitg/mcpagent/executor"
	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
)

// TestPulseReviewerVerificationClosesThroughMCPBridge is the transport-level
// regression for the critical lifecycle seam. It uses the production mcpbridge
// stdio binary, the production mark_pulse_module_result executor, and SQLite.
// No direct executor call is used for the operation under test.
func TestPulseReviewerVerificationClosesThroughMCPBridge(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	t.Setenv("WORKSPACE_DOCS_PATH", root)
	workspacePath := "Workflow/pulse-bridge-verification"
	priorRunID := "pulse-prior-fix"
	currentRunID := "pulse-review-verification"

	if _, err := recordPulseWorklist(ctx, workspacePath, priorRunID, completePulseWorklistDecisions(map[string]PulseWorklistDecision{
		pulseModuleWorkflowReview: {Module: pulseModuleWorkflowReview, Due: true, Reason: "Repair the filed defect."},
	})); err != nil {
		t.Fatalf("record prior worklist: %v", err)
	}
	if _, err := step_based_workflow.RecordRunConcerns(ctx, workspacePath, priorRunID, "", pulseModuleBugReview, step_based_workflow.ConcernPhaseReview,
		"CONCERNS: collector omits the populated latency value after a producing run"); err != nil {
		t.Fatalf("file concern: %v", err)
	}
	backlog, err := step_based_workflow.LoadPulseFindingLifecycles(ctx, workspacePath, pulseModuleBugReview, -1)
	if err != nil || len(backlog) != 1 {
		t.Fatalf("load filed concern: count=%d err=%v", len(backlog), err)
	}
	finding := backlog[0]
	attempt, err := step_based_workflow.StartPulseFixAttempt(ctx, workspacePath, priorRunID, pulseModuleBugReview,
		"Populate latency through the canonical collector path",
		[]step_based_workflow.PulseFixFindingRef{{Fingerprint: finding.Fingerprint, FindingID: finding.Issue.ID}},
		[]string{"planning/step_config.json"}, []string{"sha256:before"})
	if err != nil {
		t.Fatalf("start prior attempt: %v", err)
	}
	_, err = markPulseModuleResultFromAgentWithAuditAndFindings(ctx, workspacePath, pulseModuleBugReview, priorRunID,
		"changed", "Change applied; next producing run is required.", []string{"bounded repair"},
		PulseModuleAuditInput{ChangedFiles: []string{"planning/step_config.json"}, Verification: []string{"awaiting next run"}},
		[]step_based_workflow.PulseFindingDisposition{{
			Fingerprint: finding.Fingerprint, FindingID: finding.Issue.ID, AttemptID: attempt.AttemptID,
			Disposition: step_based_workflow.FindingDispositionChangedUnverified,
			Summary:     "Awaiting run-12", ChangedFiles: []string{"planning/step_config.json"},
			BeforeRefs: []string{"sha256:before"}, AfterRefs: []string{"sha256:after"}, NextCheck: "run-12 collector artifact",
			Verification: []step_based_workflow.PulseFindingVerification{{Check: "producing run arrived", Verdict: step_based_workflow.VerificationInconclusive, Expected: "run-12 has latency", Observed: "run-12 had not run"}},
		}},
	)
	if err != nil {
		t.Fatalf("record prior changed_unverified result: %v", err)
	}

	if _, err := recordPulseWorklist(ctx, workspacePath, currentRunID, completePulseWorklistDecisions(map[string]PulseWorklistDecision{
		pulseModuleWorkflowReview: {Module: pulseModuleWorkflowReview, Due: true, Reason: "Verify the prior repair."},
	})); err != nil {
		t.Fatalf("record current worklist: %v", err)
	}
	expected := "run-12 has latency"
	observed := "run-12 contains latency_ms=42"
	reviewRunID := "2026-08-01T00-00-00.000Z_bridge-verification"
	marker := fmt.Sprintf(`PULSE_VERIFICATION_JSON: {"finding_id":%q,"fingerprint":%q,"attempt_id":%q,"verdict":"passed","expected":%q,"observed":%q,"evidence":["runs/run-12/result.json"]}`,
		finding.Issue.ID, finding.Fingerprint, attempt.AttemptID, expected, observed)
	if err := step_based_workflow.RecordPulseReview(ctx, workspacePath, pulseModuleBugReview, reviewRunID, currentRunID, "", "## Verification\n"+marker); err != nil {
		t.Fatalf("record structured review: %v", err)
	}

	_, executors, _ := createPulseWorklistTools()
	executor, ok := executors["mark_pulse_module_result"].(func(context.Context, map[string]interface{}) (string, error))
	if !ok {
		t.Fatal("mark_pulse_module_result executor has unexpected type")
	}
	const sessionID = "pulse-fixer-bridge-session"
	release := registerTrustedPulseSession(sessionID, currentRunID)
	defer release()

	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/tools/custom/mark_pulse_module_result" {
			http.Error(w, "unknown tool", http.StatusNotFound)
			return
		}
		var args map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&args); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		requestCtx := mcpexecutor.WithSessionID(r.Context(), r.Header.Get("X-Session-ID"))
		result, execErr := executor(requestCtx, args)
		w.Header().Set("Content-Type", "application/json")
		response := map[string]interface{}{"success": execErr == nil, "result": result}
		if execErr != nil {
			response["error"] = execErr.Error()
		}
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer api.Close()

	bridgeBinary := buildPulseTestMCPBridge(t)
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"workspace_path":       map[string]interface{}{"type": "string"},
			"pulse_run_id":         map[string]interface{}{"type": "string"},
			"module":               map[string]interface{}{"type": "string"},
			"result":               map[string]interface{}{"type": "string"},
			"reason":               map[string]interface{}{"type": "string"},
			"evidence":             map[string]interface{}{"type": "array"},
			"finding_dispositions": map[string]interface{}{"type": "array"},
		},
		"required": []string{"workspace_path", "pulse_run_id", "module", "result", "reason"},
	}
	schemaJSON, _ := json.Marshal(schema)
	toolsJSON, _ := json.Marshal([]map[string]interface{}{{
		"name": "mark_pulse_module_result", "description": "Apply a verified Pulse lifecycle result.",
		"input_schema": json.RawMessage(schemaJSON), "type": "custom",
	}})
	bridgeClient, err := client.NewStdioMCPClient(bridgeBinary, append(os.Environ(),
		"MCP_API_URL="+api.URL,
		"MCP_API_TOKEN=bridge-test-token",
		"MCP_SESSION_ID="+sessionID,
		"MCP_TOOLS="+string(toolsJSON),
	))
	if err != nil {
		t.Fatalf("start mcpbridge: %v", err)
	}
	defer bridgeClient.Close()
	bridgeCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	if _, err := bridgeClient.Initialize(bridgeCtx, mcp.InitializeRequest{Params: mcp.InitializeParams{
		ProtocolVersion: mcp.LATEST_PROTOCOL_VERSION,
		ClientInfo:      mcp.Implementation{Name: "pulse-e2e", Version: "1"},
	}}); err != nil {
		t.Fatalf("initialize mcpbridge: %v", err)
	}
	request := mcp.CallToolRequest{}
	request.Params.Name = "mark_pulse_module_result"
	request.Params.Arguments = map[string]interface{}{
		"workspace_path": workspacePath, "pulse_run_id": currentRunID, "module": pulseModuleBugReview,
		"result": "done", "reason": "Independent reviewer proved the prior repair on run-12.",
		"evidence": []interface{}{"runs/run-12/result.json"},
		"finding_dispositions": []interface{}{map[string]interface{}{
			"fingerprint": finding.Fingerprint, "finding_id": finding.Issue.ID, "attempt_id": attempt.AttemptID,
			"disposition": step_based_workflow.FindingDispositionFixedVerified, "summary": "Verified on run-12",
			"changed_files": []interface{}{"planning/step_config.json"},
			"before_refs":   []interface{}{"sha256:before"}, "after_refs": []interface{}{"sha256:after"},
			"verification": []interface{}{map[string]interface{}{
				"check": "run-12 collector evidence", "verdict": step_based_workflow.VerificationPassed,
				"expected": expected, "observed": observed, "evidence": []interface{}{"runs/run-12/result.json"},
			}},
		}},
	}
	result, err := bridgeClient.CallTool(bridgeCtx, request)
	if err != nil {
		t.Fatalf("bridge tool call: %v", err)
	}
	if len(result.Content) != 1 || !strings.Contains(fmt.Sprint(result.Content[0]), `"status":"updated"`) {
		t.Fatalf("unexpected bridge result: %#v", result.Content)
	}

	closed, err := step_based_workflow.LoadPulseFindingLifecycles(ctx, workspacePath, pulseModuleBugReview, -1)
	if err != nil || len(closed) != 1 || closed[0].Status != step_based_workflow.ConcernStatusResolved {
		t.Fatalf("review verification did not close through bridge: %#v err=%v", closed, err)
	}
}

func buildPulseTestMCPBridge(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test source path")
	}
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(filename), "..", "..", ".."))
	source := filepath.Join(repoRoot, "..", "mcpagent", "cmd", "mcpbridge")
	binary := filepath.Join(t.TempDir(), "mcpbridge")
	cmd := exec.Command("go", "build", "-o", binary, ".")
	cmd.Dir = source
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build mcpbridge: %v\n%s", err, output)
	}
	return binary
}
