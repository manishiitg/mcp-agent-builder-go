package virtualtools

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/common"
	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/workspace"
	"github.com/manishiitg/multi-llm-provider-go/llmtypes"
)

const WorkflowDBToolCategory = "workflow_db"

const workflowDBAccessEnv = "WORKFLOW_DB_ACCESS"

var safeWorkflowDBTableName = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,128}$`)

// workflowDBDescribeAllSQL lists every user table and view. It is the schema
// source for action=describe and for the schema hint attached to a failed query,
// so both answer from the same statement.
const workflowDBDescribeAllSQL = "SELECT name, type, sql FROM sqlite_master WHERE type IN ('table','view') AND name NOT LIKE 'sqlite_%' ORDER BY type, name"

// workflowDBDescribeRows bounds the follow-up describe. Schema rows are one per
// column or one per table, so this is far above any real workflow database.
const workflowDBDescribeRows = 500

// workflowDBSchemaHintBudget caps the schema text appended to a failed query, in
// bytes, so a wide table cannot bury the SQLite error that the caller has to
// read. Names past the budget are replaced by a "(+N more)" count, and the
// caller can still get the full list with action=describe.
const workflowDBSchemaHintBudget = 1000

// workflowDBDescribeTableSQL renders the column-listing PRAGMA. Callers must
// have checked the name against safeWorkflowDBTableName first.
func workflowDBDescribeTableSQL(table string) string {
	return `PRAGMA table_info("` + strings.ReplaceAll(table, `"`, `""`) + `")`
}

type WorkflowDBToolRegistry struct {
	Tools      []llmtypes.Tool
	Executors  map[string]func(context.Context, map[string]any) (string, error)
	Categories map[string]string
}

func workflowDBQueryToolDefinition() llmtypes.Tool {
	return llmtypes.Tool{Type: "function", Function: &llmtypes.FunctionDefinition{
		Name:        "query_workflow_db",
		Description: "Read the current workflow SQLite database. Pass sql to run one statement; it opens read-only and cannot mutate. Use action=describe to inspect an unfamiliar table first. The backend resolves the database; never pass a path. Single-statement, row-bounded, WAL-aware.",
		Parameters: llmtypes.NewParameters(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"action":   map[string]any{"type": "string", "enum": []string{"describe", "query"}, "description": "Optional. Omit it and pass sql to run a statement. Use describe to list schemas or columns."},
				"table":    map[string]any{"type": "string", "description": "Optional table name for action=describe. Omit to list all table/view definitions."},
				"sql":      map[string]any{"type": "string", "description": "One SELECT, read-only WITH/EXPLAIN, or safe schema PRAGMA statement. This is the normal way to use the tool."},
				"max_rows": map[string]any{"type": "integer", "minimum": 1, "maximum": 1000, "description": "Maximum rows to return for action=query. Default 500."},
			},
		}),
	}}
}

func workflowDBMutateToolDefinition() llmtypes.Tool {
	return llmtypes.Tool{Type: "function", Function: &llmtypes.FunctionDefinition{
		Name:        "mutate_workflow_db",
		Description: "Mutate rows in the current workflow SQLite database. Needs trusted DB write authority. Pass sql (and optional params) for one statement, same shape as query_workflow_db; pass statements with 1-20 entries for an all-or-nothing batch. INSERT, UPDATE, DELETE, optionally WITH CTEs; returns affected-row receipts. Schema changes use migrations.",
		Parameters: llmtypes.NewParameters(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"statements": map[string]any{
					"type": "array", "minItems": 1, "maxItems": 20,
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"sql":    map[string]any{"type": "string", "description": "One INSERT, UPDATE, or DELETE statement, optionally prefixed by WITH CTEs. Use ? placeholders for values."},
							"params": map[string]any{"type": "array", "description": "Optional positional values for ? placeholders."},
						},
						"required": []string{"sql"},
					},
				},
				"sql":    map[string]any{"type": "string", "description": "One INSERT, UPDATE, or DELETE statement for the single-statement form. Same shape as query_workflow_db. Use ? placeholders for values."},
				"params": map[string]any{"type": "array", "description": "Optional positional values for the ? placeholders in sql."},
			},
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
		// Raw SQL is the contract, exactly as on mutate_workflow_db; the only
		// difference between the two tools is that this one opens the database with
		// query_only(true). `action` is an optional convenience, not a required
		// preamble — requiring it produced "action must be describe or query" and
		// "sql is required for action=query" failures from callers that had already
		// supplied valid SQL.
		if action == "" {
			if raw, _ := args["sql"].(string); strings.TrimSpace(raw) != "" {
				action = "query"
			} else if _, hasTable := args["table"]; hasTable {
				action = "describe"
			}
		}
		switch action {
		case "describe":
			table, _ := args["table"].(string)
			table = strings.TrimSpace(table)
			if table == "" {
				sqlText = workflowDBDescribeAllSQL
			} else {
				if !safeWorkflowDBTableName.MatchString(table) {
					return "", fmt.Errorf("table must contain only letters, digits, underscore, dot, or hyphen")
				}
				sqlText = workflowDBDescribeTableSQL(table)
			}
		case "query":
			sqlText, _ = args["sql"].(string)
			if strings.TrimSpace(sqlText) == "" {
				return "", fmt.Errorf("sql is required for action=query")
			}
		default:
			return "", fmt.Errorf(
				"pass sql to run a read-only statement, or action=\"describe\" (with optional table) to list schemas. Received top-level keys %v",
				sortedArgumentKeys(args),
			)
		}
		result, err := client.QueryAuthorizedWorkflowDB(ctx, workspace.QueryWorkflowDBParams{DBPath: dbPath, SQL: sqlText, MaxRows: maxRows})
		if err != nil {
			// A bare "no such column: input_id" tells the caller only that its guess
			// was wrong, so it guesses again — one overnight run spent 18 tool calls
			// inventing column names and finished on `x`. Answer with the real
			// schema, read back over this same query_only(true) path.
			return "", workflowDBSchemaHintError(ctx, err, sqlText, func(hintCtx context.Context, hintSQL string) (workspace.QueryWorkflowDBResult, error) {
				return client.QueryAuthorizedWorkflowDB(hintCtx, workspace.QueryWorkflowDBParams{DBPath: dbPath, SQL: hintSQL, MaxRows: workflowDBDescribeRows})
			})
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
		if access := strings.TrimSpace(cfg.Env[workflowDBAccessEnv]); access != "read-write" {
			return "", fmt.Errorf("workflow database mutation denied for session %q: explicit db_access=read-write is required (effective value %q)", sessionID, access)
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
		// Single-statement form: sql at the top level, identical in shape to
		// query_workflow_db. The two tools now differ only in how the database is
		// opened — query_only(true) for reads, read-write here. Requiring a
		// `statements` wrapper for one UPDATE made callers that had just used
		// query_workflow_db reach for sql=, set=, and upsert= instead.
		if len(payload.Statements) == 0 {
			if raw, _ := args["sql"].(string); strings.TrimSpace(raw) != "" {
				statement := workspace.WorkflowDBMutationStatement{SQL: raw}
				if rawParams, ok := args["params"].([]interface{}); ok {
					statement.Params = rawParams
				}
				payload.Statements = []workspace.WorkflowDBMutationStatement{statement}
			}
		}
		if len(payload.Statements) == 0 {
			// A bare "statements must contain at least one mutation" told the caller
			// nothing about the shape it wanted, so agents guessed: sql=, set=, and
			// upsert= at top level produced 10 failures in a single run. Show the
			// contract and what actually arrived.
			return "", fmt.Errorf(
				"no mutation supplied. Received top-level keys %v. "+
					"Pass sql for one statement, exactly like query_workflow_db: "+
					`{"sql":"UPDATE t SET c = ? WHERE id = ?","params":["value",1]}. `+
					"For an all-or-nothing batch pass statements with 1-20 entries of the same shape. There is no `set` or `upsert` argument",
				sortedArgumentKeys(args),
			)
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

// workflowDBSchemaDescriber runs one read-only statement against the database
// the failed query already resolved to.
type workflowDBSchemaDescriber func(ctx context.Context, sqlText string) (workspace.QueryWorkflowDBResult, error)

// workflowDBSchemaHintError appends the real schema to a "no such column" or
// "no such table" failure so the caller can correct itself instead of guessing.
// The original error is always preserved and wrapped; if the follow-up describe
// itself fails, or the database has nothing to report, the original error is
// returned unchanged. Diagnostics must never turn one failure into two.
func workflowDBSchemaHintError(ctx context.Context, queryErr error, sqlText string, describe workflowDBSchemaDescriber) error {
	kind := workflowDBMissingSchemaKind(queryErr)
	if kind == "" || describe == nil {
		return queryErr
	}
	if kind == "column" {
		// Only name a table the SQL actually names. A guessed table presented as
		// fact would be a second wrong answer on top of the first.
		if table := workflowDBTableFromSQL(sqlText); table != "" {
			described, err := describe(ctx, workflowDBDescribeTableSQL(table))
			if err == nil {
				if columns := workflowDBNamedValues(described.Rows); len(columns) > 0 {
					return fmt.Errorf("%w. Table `%s` has columns: %s. Use these exact names", queryErr, table, workflowDBJoinWithinBudget(columns))
				}
			}
		}
	}
	described, err := describe(ctx, workflowDBDescribeAllSQL)
	if err != nil {
		return queryErr
	}
	names := workflowDBNamedValues(described.Rows)
	if len(names) == 0 {
		return queryErr
	}
	const nextStep = `Call query_workflow_db with action="describe" and table="<name>" to list its columns`
	if kind == "column" {
		return fmt.Errorf("%w. The table that column belongs to could not be identified from this SQL. Tables and views in this database: %s. %s", queryErr, workflowDBJoinWithinBudget(names), nextStep)
	}
	return fmt.Errorf("%w. Tables and views in this database: %s. %s", queryErr, workflowDBJoinWithinBudget(names), nextStep)
}

// workflowDBMissingSchemaKind classifies a failed query as naming a column that
// does not exist, a table that does not exist, or neither. The workspace service
// returns SQLite's message inside its JSON envelope, so this matches on the
// SQLite text wherever it sits in the error.
func workflowDBMissingSchemaKind(err error) string {
	if err == nil {
		return ""
	}
	lowered := strings.ToLower(err.Error())
	switch {
	case strings.Contains(lowered, "no such column"):
		return "column"
	case strings.Contains(lowered, "no such table"):
		return "table"
	default:
		return ""
	}
}

var workflowDBTableReferencePattern = regexp.MustCompile("(?is)\\b(?:from|join)\\s+([A-Za-z_][A-Za-z0-9_$]*|\"[^\"]+\"|`[^`]+`|\\[[^\\]]+\\])")

// workflowDBTableFromSQL returns the single table a statement reads from, or ""
// when the statement names none, names several, or names one this code cannot
// read confidently. Confidence is the point: the caller is told the columns of a
// named table, so naming the wrong table would be worse than naming none.
func workflowDBTableFromSQL(sqlText string) string {
	stripped := workflowDBStripSQLNoise(sqlText)
	matches := workflowDBTableReferencePattern.FindAllStringSubmatchIndex(stripped, -1)
	seen := map[string]bool{}
	found := ""
	for _, match := range matches {
		name := workflowDBUnquoteIdentifier(stripped[match[2]:match[3]])
		// A name this code cannot quote safely is not a name it may report.
		if name == "" || !safeWorkflowDBTableName.MatchString(name) {
			return ""
		}
		// `FROM a, b` puts two tables in scope without a second FROM/JOIN keyword.
		if strings.EqualFold(stripped[match[0]:match[0]+4], "from") && workflowDBFromListContinues(stripped, match[3]) {
			return ""
		}
		key := strings.ToLower(name)
		if seen[key] {
			continue
		}
		seen[key] = true
		if found != "" {
			return ""
		}
		found = name
	}
	return found
}

// workflowDBFromClauseBoundary marks the words that end a FROM item list, so a
// comma found after one of them belongs to some later clause rather than to the
// table list.
var workflowDBFromClauseBoundary = map[string]bool{
	"WHERE": true, "GROUP": true, "ORDER": true, "HAVING": true, "LIMIT": true,
	"OFFSET": true, "WINDOW": true, "UNION": true, "INTERSECT": true, "EXCEPT": true,
	"JOIN": true, "INNER": true, "LEFT": true, "RIGHT": true, "FULL": true,
	"CROSS": true, "NATURAL": true, "ON": true, "USING": true, "RETURNING": true,
	"SELECT": true, "VALUES": true,
}

// workflowDBFromListContinues reports whether the FROM item that ends at offset
// is followed by another comma-separated table in the same clause.
func workflowDBFromListContinues(stripped string, offset int) bool {
	depth := 0
	for i := offset; i < len(stripped); {
		ch := stripped[i]
		switch {
		case ch == '(':
			depth++
			i++
		case ch == ')':
			if depth == 0 {
				return false
			}
			depth--
			i++
		case ch == ',':
			if depth == 0 {
				return true
			}
			i++
		case ch == '"' || ch == '`' || ch == '[':
			closing := ch
			if ch == '[' {
				closing = ']'
			}
			i++
			for i < len(stripped) && stripped[i] != closing {
				i++
			}
			i++
		case ch == '_' || ch >= 'A' && ch <= 'Z' || ch >= 'a' && ch <= 'z':
			start := i
			for i < len(stripped) {
				current := stripped[i]
				if current == '_' || current == '$' || current >= 'A' && current <= 'Z' || current >= 'a' && current <= 'z' || current >= '0' && current <= '9' {
					i++
					continue
				}
				break
			}
			if depth == 0 && workflowDBFromClauseBoundary[strings.ToUpper(stripped[start:i])] {
				return false
			}
		default:
			i++
		}
	}
	return false
}

// workflowDBStripSQLNoise blanks string literals and comments so a table scan
// cannot be fooled by the word "from" inside quoted text.
func workflowDBStripSQLNoise(input string) string {
	var out strings.Builder
	for i := 0; i < len(input); {
		switch {
		case input[i] == '\'':
			i++
			for i < len(input) {
				if input[i] != '\'' {
					i++
					continue
				}
				if i+1 < len(input) && input[i+1] == '\'' {
					i += 2
					continue
				}
				i++
				break
			}
			out.WriteByte(' ')
		case input[i] == '-' && i+1 < len(input) && input[i+1] == '-':
			for i < len(input) && input[i] != '\n' {
				i++
			}
			out.WriteByte(' ')
		case input[i] == '/' && i+1 < len(input) && input[i+1] == '*':
			i += 2
			for i+1 < len(input) && !(input[i] == '*' && input[i+1] == '/') {
				i++
			}
			if i+1 < len(input) {
				i += 2
			} else {
				i = len(input)
			}
			out.WriteByte(' ')
		default:
			out.WriteByte(input[i])
			i++
		}
	}
	return out.String()
}

// workflowDBUnquoteIdentifier removes SQLite identifier quoting.
func workflowDBUnquoteIdentifier(identifier string) string {
	identifier = strings.TrimSpace(identifier)
	if len(identifier) < 2 {
		return identifier
	}
	first, last := identifier[0], identifier[len(identifier)-1]
	switch {
	case first == '"' && last == '"':
		return strings.ReplaceAll(identifier[1:len(identifier)-1], `""`, `"`)
	case first == '`' && last == '`':
		return strings.ReplaceAll(identifier[1:len(identifier)-1], "``", "`")
	case first == '[' && last == ']':
		return identifier[1 : len(identifier)-1]
	}
	return identifier
}

// workflowDBNamedValues pulls the name column out of PRAGMA table_info or
// sqlite_master rows.
func workflowDBNamedValues(rows []map[string]interface{}) []string {
	values := make([]string, 0, len(rows))
	for _, row := range rows {
		raw, ok := row["name"]
		if !ok || raw == nil {
			continue
		}
		if name := strings.TrimSpace(fmt.Sprint(raw)); name != "" {
			values = append(values, name)
		}
	}
	return values
}

// workflowDBJoinWithinBudget joins names until workflowDBSchemaHintBudget bytes
// are used, then reports how many were left out rather than cutting a name in
// half.
func workflowDBJoinWithinBudget(values []string) string {
	var joined strings.Builder
	for index, value := range values {
		addition := len(value)
		if index > 0 {
			addition += len(", ")
		}
		if joined.Len()+addition > workflowDBSchemaHintBudget {
			fmt.Fprintf(&joined, " ... (+%d more)", len(values)-index)
			break
		}
		if index > 0 {
			joined.WriteString(", ")
		}
		joined.WriteString(value)
	}
	return joined.String()
}

// sortedArgumentKeys lists the top-level argument names a caller supplied, so a
// contract error can show what arrived next to what was required.
func sortedArgumentKeys(args map[string]any) []string {
	keys := make([]string, 0, len(args))
	for key := range args {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
