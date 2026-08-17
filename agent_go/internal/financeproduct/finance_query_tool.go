package financeproduct

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/agentprofiles"
	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/workspace"
)

// financeSourceDBPaths maps the whitelisted `source` argument to the
// workspace-relative db/db.sqlite path each real finance workflow owns.
// These are the same five paths the frontend dashboard's own adapters
// query directly (frontend/src/products/finance/adapters/*.ts) -- kept in
// sync by hand since they're plain string constants on both sides, not
// logic to share.
var financeSourceDBPaths = map[string]string{
	"hdfc":        "Workflow/HDFC-Personal-Accounts/db/db.sqlite",
	"icici":       "Workflow/ICICI-BANK-PARSING/db/db.sqlite",
	"mutual_fund": "Workflow/Mututal-Fund/db/db.sqlite",
	"tax":         "Workflow/check-form-26as-xspaces/db/db.sqlite",
	"gst":         "Workflow/gstdatacollection/db/db.sqlite",
}

const financeQueryMaxRows = 500

// financeQuerySourceFactory returns the one tool this profile exposes: a
// read-only SQL query against one of the five whitelisted finance sources.
// Read-only is enforced server-side (workspace/handlers/query.go rejects
// anything but SELECT/read-only WITH/EXPLAIN/safe PRAGMA), not by trusting
// the model -- so this tool is safe by construction regardless of what SQL
// it's asked to run, the same guarantee the platform's own
// query_workflow_db tool already has.
func financeQuerySourceFactory(workspaceAPIURL string) agentprofiles.ToolFactory {
	return func(runtime agentprofiles.ToolRuntimeContext, _ json.RawMessage) (agentprofiles.ToolSpec, error) {
		client := workspace.NewClient(
			workspaceAPIURL,
			workspace.WithUserID(runtime.UserID),
			workspace.WithExtraEnv(map[string]string{"MCP_SESSION_ID": runtime.SessionID}),
		)
		sourceNames := make([]string, 0, len(financeSourceDBPaths))
		for name := range financeSourceDBPaths {
			sourceNames = append(sourceNames, name)
		}
		return agentprofiles.ToolSpec{
			Name:        "query_finance_source",
			Category:    "finance_query",
			Description: "Run a read-only SQL SELECT against one of the finance sources (hdfc, icici, mutual_fund, tax, gst). See the system prompt for each source's real tables and columns, and its known data-quality caveats.",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"source": map[string]interface{}{
						"type":        "string",
						"enum":        sourceNames,
						"description": "Which finance source to query.",
					},
					"sql": map[string]interface{}{
						"type":        "string",
						"description": "A read-only SELECT statement. Non-SELECT statements are rejected.",
					},
				},
				"required": []string{"source", "sql"},
			},
			Execute: func(ctx context.Context, args map[string]interface{}) (string, error) {
				source := strings.TrimSpace(stringArg(args, "source"))
				dbPath, ok := financeSourceDBPaths[source]
				if !ok {
					return fmt.Sprintf("Unknown source %q. Valid sources: hdfc, icici, mutual_fund, tax, gst.", source), nil
				}
				sql := strings.TrimSpace(stringArg(args, "sql"))
				if sql == "" {
					return "sql is required.", nil
				}
				result, err := client.QueryAuthorizedWorkflowDB(ctx, workspace.QueryWorkflowDBParams{
					DBPath:  dbPath,
					SQL:     sql,
					MaxRows: financeQueryMaxRows,
				})
				if err != nil {
					return "", err
				}
				encoded, err := json.Marshal(result)
				if err != nil {
					return "", fmt.Errorf("encode finance query result: %w", err)
				}
				return string(encoded), nil
			},
		}, nil
	}
}

func stringArg(args map[string]interface{}, key string) string {
	value, _ := args[key].(string)
	return value
}

// RegisterAgentProfileRuntime connects Finance's one query tool to the
// generic Agent Profile registry -- same shape as Video Studio's own
// RegisterAgentProfileRuntime. No initializer: unlike Video Studio, there
// is no per-project workspace to provision.
func RegisterAgentProfileRuntime(registry *agentprofiles.Registry, workspaceAPIURL string) error {
	return registry.RegisterToolFactory("finance.query-source", financeQuerySourceFactory(workspaceAPIURL))
}
