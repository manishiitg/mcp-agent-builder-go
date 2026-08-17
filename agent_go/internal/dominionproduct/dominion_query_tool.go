package dominionproduct

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/agentprofiles"
	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/workspace"
)

// dominionSourceDBPaths maps the whitelisted `source` argument to the
// workspace-relative db/db.sqlite path. Only one source exists today, but
// this keeps the same source-parameterized shape as Finance's tool so a
// second trading workflow can be added later without changing the tool's
// signature. Kept in sync by hand with the frontend adapters' own DB_PATH
// constants (frontend/src/products/dominion/adapters/*.ts).
var dominionSourceDBPaths = map[string]string{
	"trading": "Workflow/tectonicusadaytrading/db/db.sqlite",
}

const dominionQueryMaxRows = 500

// dominionQuerySourceFactory returns the one tool this profile exposes: a
// read-only SQL query against the trading workflow's database. Read-only is
// enforced server-side (workspace/handlers/query.go rejects anything but
// SELECT/read-only WITH/EXPLAIN/safe PRAGMA), not by trusting the model.
func dominionQuerySourceFactory(workspaceAPIURL string) agentprofiles.ToolFactory {
	return func(runtime agentprofiles.ToolRuntimeContext, _ json.RawMessage) (agentprofiles.ToolSpec, error) {
		client := workspace.NewClient(
			workspaceAPIURL,
			workspace.WithUserID(runtime.UserID),
			workspace.WithExtraEnv(map[string]string{"MCP_SESSION_ID": runtime.SessionID}),
		)
		sourceNames := make([]string, 0, len(dominionSourceDBPaths))
		for name := range dominionSourceDBPaths {
			sourceNames = append(sourceNames, name)
		}
		return agentprofiles.ToolSpec{
			Name:        "query_dominion_source",
			Category:    "dominion_query",
			Description: "Run a read-only SQL SELECT against the paper-trading workflow's database (source: trading). See the system prompt for the real tables/columns and the mandatory horizon filter.",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"source": map[string]interface{}{
						"type":        "string",
						"enum":        sourceNames,
						"description": "Which Dominion source to query.",
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
				dbPath, ok := dominionSourceDBPaths[source]
				if !ok {
					return fmt.Sprintf("Unknown source %q. Valid sources: trading.", source), nil
				}
				sql := strings.TrimSpace(stringArg(args, "sql"))
				if sql == "" {
					return "sql is required.", nil
				}
				result, err := client.QueryAuthorizedWorkflowDB(ctx, workspace.QueryWorkflowDBParams{
					DBPath:  dbPath,
					SQL:     sql,
					MaxRows: dominionQueryMaxRows,
				})
				if err != nil {
					return "", err
				}
				encoded, err := json.Marshal(result)
				if err != nil {
					return "", fmt.Errorf("encode dominion query result: %w", err)
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

// RegisterAgentProfileRuntime connects Dominion's tools to the generic Agent
// Profile registry -- same shape as Finance's own RegisterAgentProfileRuntime,
// plus the add-only watchlist tool Dominion has that Finance doesn't.
func RegisterAgentProfileRuntime(registry *agentprofiles.Registry, workspaceAPIURL string) error {
	if err := registry.RegisterToolFactory("dominion.query-source", dominionQuerySourceFactory(workspaceAPIURL)); err != nil {
		return err
	}
	return registry.RegisterToolFactory("dominion.add-watchlist-symbol", addWatchlistSymbolFactory(workspaceAPIURL))
}
