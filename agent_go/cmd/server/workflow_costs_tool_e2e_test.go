package server

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	virtualtools "github.com/manishiitg/coding-agent-loop/agent_go/cmd/server/virtual-tools"
	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/common"
	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/costledger"
	workspacehandlers "github.com/manishiitg/coding-agent-loop/workspace/handlers"
	"github.com/spf13/viper"
	"net/http/httptest"
)

// TestQueryWorkflowCostsToolReadsOwnPhaseBreakdownAndIsIsolatedFromAnotherWorkflow
// is PLAT-184's migration acceptance test, run through the real tool-call path
// (MCP-bridge custom tool executor -> workspace DB HTTP handler -> SQLite),
// exactly as production calls it -- not just the storage layer.
//
// It proves both halves of the explicit acceptance requirement: a real
// model_cost_fitness-style Pulse turn scoped to one workflow can read its own
// phase/item cost breakdown, while a session scoped to a different workflow
// cannot see it. The tool never accepts a path argument at all -- the backend
// resolves it from the calling session's own folder-guard scope -- so a
// Social Media session has no way to even name Upwork's ledger, let alone
// read it.
func TestQueryWorkflowCostsToolReadsOwnPhaseBreakdownAndIsIsolatedFromAnotherWorkflow(t *testing.T) {
	root := t.TempDir()
	t.Setenv("WORKSPACE_DOCS_PATH", root)

	previousDocsDir := viper.Get("docs-dir")
	viper.Set("docs-dir", root)
	t.Cleanup(func() { viper.Set("docs-dir", previousDocsDir) })

	// Seed two real, independently-opened per-workspace ledgers -- the exact
	// write path costobserver.Observer.append uses in production -- each with
	// its own phase/item cost events.
	socialMedia, err := costledger.WorkspaceLedger("Workflow/social-media")
	if err != nil || socialMedia == nil {
		t.Fatalf("WorkspaceLedger(social-media): ledger=%v err=%v", socialMedia, err)
	}
	if err := socialMedia.Append(costledger.Entry{
		WorkflowID:   "Workflow/social-media",
		ExecutionID:  "workflow-full-social-1",
		Scope:        "workflow_execution",
		Phase:        "item:draft-message",
		LLMCallCount: 1,
		PromptTokens: 1000, CompletionTokens: 200,
		TotalCostUSD: 1.50,
	}); err != nil {
		t.Fatalf("append social-media entry: %v", err)
	}

	upwork, err := costledger.WorkspaceLedger("Workflow/upwork")
	if err != nil || upwork == nil {
		t.Fatalf("WorkspaceLedger(upwork): ledger=%v err=%v", upwork, err)
	}
	if err := upwork.Append(costledger.Entry{
		WorkflowID:   "Workflow/upwork",
		ExecutionID:  "workflow-full-upwork-1",
		Scope:        "workflow_execution",
		Phase:        "item:apply-to-job",
		LLMCallCount: 1,
		PromptTokens: 500, CompletionTokens: 100,
		TotalCostUSD: 9.99,
	}); err != nil {
		t.Fatalf("append upwork entry: %v", err)
	}

	if _, err := os.Stat(filepath.Join(root, "Workflow", "social-media", "costs", "costs.sqlite")); err != nil {
		t.Fatalf("expected social-media's own costs.sqlite on disk: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "Workflow", "upwork", "costs", "costs.sqlite")); err != nil {
		t.Fatalf("expected upwork's own costs.sqlite on disk: %v", err)
	}

	gin.SetMode(gin.TestMode)
	workspaceRouter := gin.New()
	workspaceRouter.POST("/api/query", workspacehandlers.QueryWorkflowDB)
	workspaceAPI := httptest.NewServer(workspaceRouter)
	defer workspaceAPI.Close()

	const socialMediaSession = "cost-tool-e2e-social-media"
	common.SetSessionFolderGuard(socialMediaSession, []string{"Workflow/social-media"}, nil)
	defer common.ClearSessionShellConfig(socialMediaSession)

	const upworkSession = "cost-tool-e2e-upwork"
	common.SetSessionFolderGuard(upworkSession, []string{"Workflow/upwork"}, nil)
	defer common.ClearSessionShellConfig(upworkSession)

	phaseBreakdownSQL := "SELECT phase, SUM(total_cost_usd) AS cost, SUM(prompt_tokens + completion_tokens) AS tokens FROM cost_events GROUP BY phase"

	socialMediaRegistry := virtualtools.CreateWorkflowCostsToolRegistry(workspaceAPI.URL, "", socialMediaSession)
	socialMediaResult, err := socialMediaRegistry.Executors["query_workflow_costs"](context.Background(), map[string]any{"sql": phaseBreakdownSQL})
	if err != nil {
		t.Fatalf("social-media query_workflow_costs: %v", err)
	}
	if !strings.Contains(socialMediaResult, "item:draft-message") {
		t.Fatalf("social-media result missing its own phase breakdown: %s", socialMediaResult)
	}
	if strings.Contains(socialMediaResult, "item:apply-to-job") || strings.Contains(socialMediaResult, "9.99") {
		t.Fatalf("ISOLATION BROKEN: social-media's query_workflow_costs result leaked Upwork's data: %s", socialMediaResult)
	}
	var socialMediaParsed struct {
		Rows []map[string]any `json:"rows"`
	}
	if err := json.Unmarshal([]byte(socialMediaResult), &socialMediaParsed); err != nil {
		t.Fatalf("unmarshal social-media result: %v", err)
	}
	if len(socialMediaParsed.Rows) != 1 || socialMediaParsed.Rows[0]["phase"] != "item:draft-message" {
		t.Fatalf("social-media rows = %+v, want exactly its own item:draft-message row", socialMediaParsed.Rows)
	}

	upworkRegistry := virtualtools.CreateWorkflowCostsToolRegistry(workspaceAPI.URL, "", upworkSession)
	upworkResult, err := upworkRegistry.Executors["query_workflow_costs"](context.Background(), map[string]any{"sql": phaseBreakdownSQL})
	if err != nil {
		t.Fatalf("upwork query_workflow_costs: %v", err)
	}
	if !strings.Contains(upworkResult, "item:apply-to-job") {
		t.Fatalf("upwork result missing its own phase breakdown: %s", upworkResult)
	}
	if strings.Contains(upworkResult, "item:draft-message") || strings.Contains(upworkResult, "1.5") {
		t.Fatalf("ISOLATION BROKEN: upwork's query_workflow_costs result leaked Social Media's data: %s", upworkResult)
	}

	// The tool takes no db_path argument at all -- there is nothing for a
	// session to pass that would even name another workflow's ledger. Confirm
	// that directly: any attempt to smuggle a path is simply ignored, the
	// backend resolves strictly from the session's own folder-guard scope.
	smuggledPathResult, err := socialMediaRegistry.Executors["query_workflow_costs"](context.Background(), map[string]any{
		"sql":     phaseBreakdownSQL,
		"db_path": "Workflow/upwork/costs/costs.sqlite",
	})
	if err != nil {
		t.Fatalf("social-media query with smuggled db_path errored instead of ignoring it: %v", err)
	}
	if strings.Contains(smuggledPathResult, "item:apply-to-job") {
		t.Fatalf("ISOLATION BROKEN: a smuggled db_path argument reached Upwork's ledger: %s", smuggledPathResult)
	}
}
