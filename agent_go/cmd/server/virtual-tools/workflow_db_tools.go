package virtualtools

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/common"
	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/workspace"
	"github.com/manishiitg/multi-llm-provider-go/llmtypes"
)

const WorkflowDBToolCategory = "workflow_db"

const workflowDBAccessEnv = "WORKFLOW_DB_ACCESS"

var safeWorkflowDBTableName = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,128}$`)

type WorkflowDBToolRegistry struct {
	Tools      []llmtypes.Tool
	Executors  map[string]func(context.Context, map[string]any) (string, error)
	Categories map[string]string
}

func workflowDBQueryToolDefinition() llmtypes.Tool {
	return llmtypes.Tool{Type: "function", Function: &llmtypes.FunctionDefinition{
		Name:        "query_workflow_db",
		Description: "Read the current workflow SQLite database through the guarded backend. Use action=describe before querying an unfamiliar table. The backend resolves the database; never pass a path. Queries are single-statement, row-bounded, WAL-aware, and cannot mutate data.",
		Parameters: llmtypes.NewParameters(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"action":   map[string]any{"type": "string", "enum": []string{"describe", "query"}, "description": "describe lists schemas or columns; query executes one bounded read-only SQL statement."},
				"table":    map[string]any{"type": "string", "description": "Optional table name for action=describe. Omit to list all table/view definitions."},
				"sql":      map[string]any{"type": "string", "description": "Required for action=query. One SELECT, read-only WITH/EXPLAIN, or safe schema PRAGMA statement."},
				"max_rows": map[string]any{"type": "integer", "minimum": 1, "maximum": 1000, "description": "Maximum rows to return for action=query. Default 500."},
			},
			"required": []string{"action"},
		}),
	}}
}

func workflowDBMutateToolDefinition() llmtypes.Tool {
	return llmtypes.Tool{Type: "function", Function: &llmtypes.FunctionDefinition{
		Name:        "mutate_workflow_db",
		Description: "Atomically mutate rows in the current workflow SQLite database. Available only with trusted DB write authority. Accepts 1-20 INSERT, UPDATE, or DELETE statements, commits all or rolls all back, and returns affected-row receipts. Schema changes use workflow migrations, not this tool.",
		Parameters: llmtypes.NewParameters(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"statements": map[string]any{
					"type": "array", "minItems": 1, "maxItems": 20,
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"sql":    map[string]any{"type": "string", "description": "One INSERT, UPDATE, or DELETE statement. Use ? placeholders for values."},
							"params": map[string]any{"type": "array", "description": "Optional positional values for ? placeholders."},
						},
						"required": []string{"sql"},
					},
				},
			},
			"required": []string{"statements"},
		}),
	}}
}

// CreateWorkflowDBToolRegistry creates one implementation shared by Builder,
// managed background agents and workflow execution steps. Availability is
// narrowed later from trusted role/db_access configuration.
func CreateWorkflowDBToolRegistry(workspaceURL, userID, fallbackSessionID string) WorkflowDBToolRegistry {
	if strings.TrimSpace(workspaceURL) == "" {
		workspaceURL = getWorkspaceAPIURL()
	}
	client := workspace.NewClient(
		workspaceURL,
		workspace.WithUserID(userID),
		workspace.WithExtraEnv(getMCPExtraEnv(fallbackSessionID)),
	)
	queryExecutor := func(ctx context.Context, args map[string]any) (string, error) {
		dbPath, err := resolveCurrentWorkflowDBPath(ctx, fallbackSessionID)
		if err != nil {
			return "", err
		}
		action, _ := args["action"].(string)
		action = strings.TrimSpace(strings.ToLower(action))
		var sqlText string
		maxRows := 500
		if raw, ok := args["max_rows"].(float64); ok && raw > 0 {
			maxRows = int(raw)
		} else if raw, ok := args["max_rows"].(int); ok && raw > 0 {
			maxRows = raw
		}
		if maxRows > 1000 {
			maxRows = 1000
		}
		switch action {
		case "describe":
			table, _ := args["table"].(string)
			table = strings.TrimSpace(table)
			if table == "" {
				sqlText = "SELECT name, type, sql FROM sqlite_master WHERE type IN ('table','view') AND name NOT LIKE 'sqlite_%' ORDER BY type, name"
			} else {
				if !safeWorkflowDBTableName.MatchString(table) {
					return "", fmt.Errorf("table must contain only letters, digits, underscore, dot, or hyphen")
				}
				sqlText = `PRAGMA table_info("` + strings.ReplaceAll(table, `"`, `""`) + `")`
			}
		case "query":
			sqlText, _ = args["sql"].(string)
			if strings.TrimSpace(sqlText) == "" {
				return "", fmt.Errorf("sql is required for action=query")
			}
		default:
			return "", fmt.Errorf("action must be describe or query")
		}
		result, err := client.QueryAuthorizedWorkflowDB(ctx, workspace.QueryWorkflowDBParams{DBPath: dbPath, SQL: sqlText, MaxRows: maxRows})
		if err != nil {
			return "", err
		}
		encoded, err := json.Marshal(result)
		if err != nil {
			return "", fmt.Errorf("encode workflow DB query result: %w", err)
		}
		return string(encoded), nil
	}

	mutationExecutor := func(ctx context.Context, args map[string]any) (string, error) {
		sessionID, cfg, err := resolveWorkflowDBSession(ctx, fallbackSessionID)
		if err != nil {
			return "", err
		}
		if access := strings.TrimSpace(cfg.Env[workflowDBAccessEnv]); access != "" && access != "read-write" {
			return "", fmt.Errorf("workflow database mutation denied for session %q: effective db_access is %q", sessionID, access)
		}
		dbPath, err := resolveWorkflowDBPathFromConfig(sessionID, cfg)
		if err != nil {
			return "", err
		}
		encodedArgs, err := json.Marshal(args)
		if err != nil {
			return "", fmt.Errorf("encode mutation arguments: %w", err)
		}
		var payload struct {
			Statements []workspace.WorkflowDBMutationStatement `json:"statements"`
		}
		if err := json.Unmarshal(encodedArgs, &payload); err != nil {
			return "", fmt.Errorf("invalid mutation arguments: %w", err)
		}
		if len(payload.Statements) == 0 {
			return "", fmt.Errorf("statements must contain at least one mutation")
		}
		result, err := client.MutateAuthorizedWorkflowDB(ctx, workspace.MutateWorkflowDBParams{DBPath: dbPath, Statements: payload.Statements})
		if err != nil {
			return "", err
		}
		encoded, err := json.Marshal(result)
		if err != nil {
			return "", fmt.Errorf("encode workflow DB mutation result: %w", err)
		}
		return string(encoded), nil
	}

	tools := []llmtypes.Tool{workflowDBQueryToolDefinition(), workflowDBMutateToolDefinition()}
	return WorkflowDBToolRegistry{
		Tools: tools,
		Executors: map[string]func(context.Context, map[string]any) (string, error){
			"query_workflow_db":  queryExecutor,
			"mutate_workflow_db": mutationExecutor,
		},
		Categories: map[string]string{
			"query_workflow_db":  WorkflowDBToolCategory,
			"mutate_workflow_db": WorkflowDBToolCategory,
		},
	}
}

func WorkflowDBToolNames() map[string]bool {
	return map[string]bool{"query_workflow_db": true, "mutate_workflow_db": true}
}

func resolveCurrentWorkflowDBPath(ctx context.Context, fallbackSessionID string) (string, error) {
	sessionID, cfg, err := resolveWorkflowDBSession(ctx, fallbackSessionID)
	if err != nil {
		return "", err
	}
	return resolveWorkflowDBPathFromConfig(sessionID, cfg)
}

func resolveWorkflowDBSession(ctx context.Context, fallbackSessionID string) (string, *common.SessionShellConfig, error) {
	sessionID := strings.TrimSpace(fallbackSessionID)
	if ctx != nil {
		if current, ok := ctx.Value(common.ChatSessionIDKey).(string); ok && strings.TrimSpace(current) != "" {
			sessionID = strings.TrimSpace(current)
		}
	}
	if sessionID == "" {
		return "", nil, fmt.Errorf("workflow database context is unavailable: missing trusted session")
	}
	cfg := common.GetSessionShellConfig(sessionID)
	if cfg == nil {
		return "", nil, fmt.Errorf("workflow database context is unavailable for session %q", sessionID)
	}
	return sessionID, cfg, nil
}

func resolveWorkflowDBPathFromConfig(sessionID string, cfg *common.SessionShellConfig) (string, error) {
	if dbPath := workflowDBPathFromCandidate(cfg.Env["DB_PATH"]); dbPath != "" {
		return dbPath, nil
	}
	candidates := append([]string{}, cfg.ReadPaths...)
	candidates = append(candidates, cfg.WritePaths...)
	candidates = append(candidates, cfg.WorkingDir)
	seen := map[string]bool{}
	var matches []string
	for _, candidate := range candidates {
		if dbPath := workflowDBPathFromCandidate(candidate); dbPath != "" && !seen[dbPath] {
			seen[dbPath] = true
			matches = append(matches, dbPath)
		}
	}
	if len(matches) == 1 {
		return matches[0], nil
	}
	if len(matches) > 1 {
		return "", fmt.Errorf("workflow database context is ambiguous for session %q", sessionID)
	}
	return "", fmt.Errorf("workflow database context is unavailable for session %q", sessionID)
}

func workflowDBPathFromCandidate(candidate string) string {
	clean := filepath.ToSlash(filepath.Clean(strings.TrimSpace(candidate)))
	if clean == "." || clean == "" {
		return ""
	}
	parts := strings.Split(strings.Trim(clean, "/"), "/")
	for i := 0; i+1 < len(parts); i++ {
		if parts[i] == "Workflow" && strings.TrimSpace(parts[i+1]) != "" {
			return filepath.ToSlash(filepath.Join("Workflow", parts[i+1], "db", "db.sqlite"))
		}
	}
	return ""
}
