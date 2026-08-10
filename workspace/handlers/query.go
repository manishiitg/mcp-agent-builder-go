package handlers

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/manishiitg/coding-agent-loop/workspace/models"
	"github.com/manishiitg/coding-agent-loop/workspace/utils"

	"github.com/gin-gonic/gin"
	"github.com/spf13/viper"

	_ "modernc.org/sqlite"
)

// queryTimeout bounds a single read-only query / inspection request.
const queryTimeout = 30 * time.Second

const (
	defaultQueryMaxRows = 10_000
	maximumQueryMaxRows = 50_000
	maximumMutationSQL  = 100_000
	maximumStatements   = 20
	maximumPragmaErrors = 1_000
)

// dbTablesSampleRows is how many sample rows the inspector returns per table.
const dbTablesSampleRows = 50

// resolveReadonlyDBPath resolves a workspace-relative SQLite path with the same
// user-isolation rules as document access, and rejects cross-user (_users/)
// access and traversal. Returns the absolute filesystem path.
func resolveReadonlyDBPath(c *gin.Context, requestedPath string) (string, error) {
	docsDir := viper.GetString("docs-dir")
	clean := utils.SanitizeInputPath(requestedPath, docsDir)
	// Never allow a query endpoint to reach into another user's private tree.
	if clean == utils.UsersDirectory || strings.HasPrefix(clean, utils.UsersDirectory+"/") {
		return "", fmt.Errorf("access to %s/ is not allowed", utils.UsersDirectory)
	}
	fullPath, err := resolveUserPath(c, requestedPath)
	if err != nil {
		return "", err
	}
	if !utils.IsValidFilePath(fullPath, docsDir) {
		return "", fmt.Errorf("path escapes the workspace boundary")
	}
	if info, err := os.Stat(fullPath); err != nil || info.IsDir() {
		return "", fmt.Errorf("database file not found: %s", requestedPath)
	}
	return fullPath, nil
}

// openQueryOnlyDB opens an existing SQLite file read-write-capable so SQLite can
// materialize WAL/SHM sidecars, then makes every pooled connection query-only.
// SQL validation is still required: query_only is defense in depth, not the
// authorization boundary for caller-supplied SQL.
func openQueryOnlyDB(fullPath string) (*sql.DB, error) {
	dsn := (&url.URL{Scheme: "file", Path: fullPath}).String() + "?mode=rw&_pragma=query_only(true)&_pragma=busy_timeout(5000)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	return db, nil
}

func openMutationDB(fullPath string) (*sql.DB, error) {
	dsn := (&url.URL{Scheme: "file", Path: fullPath}).String() + "?mode=rw&_pragma=busy_timeout(5000)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	return db, nil
}

// scanRows reads all rows of a *sql.Rows into []map[string]interface{}, keyed by
// column name. []byte values are coerced to string so JSON output is readable.
func scanRows(rows *sql.Rows, maxRows int) ([]string, []map[string]interface{}, bool, error) {
	cols, err := rows.Columns()
	if err != nil {
		return nil, nil, false, err
	}
	out := make([]map[string]interface{}, 0)
	truncated := false
	for rows.Next() {
		if len(out) >= maxRows {
			truncated = true
			break
		}
		vals := make([]interface{}, len(cols))
		ptrs := make([]interface{}, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, nil, false, err
		}
		row := make(map[string]interface{}, len(cols))
		for i, c := range cols {
			if b, ok := vals[i].([]byte); ok {
				row[c] = string(b)
			} else {
				row[c] = vals[i]
			}
		}
		out = append(out, row)
	}
	return cols, out, truncated, rows.Err()
}

var (
	readPragmaPattern      = regexp.MustCompile(`(?is)^pragma\s+(?:main\.)?([a-z_]+)\s*(?:\(([^;]*)\))?\s*;?$`)
	positiveIntegerPattern = regexp.MustCompile(`^[1-9][0-9]*$`)
	sqlIdentifierPattern   = regexp.MustCompile(`(?s)^(?:[A-Za-z_][A-Za-z0-9_]*|"(?:[^"]|"")+"|'(?:[^']|'')+'|\[[^\]]+\]|` + "`(?:[^`]|``)+`" + `)$`)
)

// isSafeReadPragma deliberately recognizes PRAGMA names and their argument
// shapes instead of growing one permissive regular expression. Every entry in
// this allowlist is observational; assignments and unknown pragmas are denied.
func isSafeReadPragma(input string) bool {
	matches := readPragmaPattern.FindStringSubmatch(strings.TrimSpace(input))
	if matches == nil {
		return false
	}
	name := strings.ToLower(matches[1])
	argument := strings.TrimSpace(matches[2])
	hasArgument := strings.Contains(matches[0], "(")

	switch name {
	case "table_info", "table_xinfo", "index_list", "index_info", "index_xinfo", "foreign_key_list":
		return hasArgument && argument != "" && sqlIdentifierPattern.MatchString(argument)
	case "database_list", "journal_mode", "user_version", "schema_version":
		return !hasArgument
	case "integrity_check", "quick_check":
		if !hasArgument {
			return true
		}
		if !positiveIntegerPattern.MatchString(argument) {
			return false
		}
		limit, err := strconv.Atoi(argument)
		return err == nil && limit <= maximumPragmaErrors
	case "foreign_key_check":
		return !hasArgument || sqlIdentifierPattern.MatchString(argument)
	default:
		return false
	}
}

// stripSQLCommentsAndSpace removes leading whitespace and comments so policy is
// applied to the actual first token rather than a model-controlled prefix.
func stripSQLCommentsAndSpace(input string) string {
	remaining := input
	for {
		remaining = strings.TrimSpace(remaining)
		switch {
		case strings.HasPrefix(remaining, "--"):
			if newline := strings.IndexByte(remaining, '\n'); newline >= 0 {
				remaining = remaining[newline+1:]
				continue
			}
			return ""
		case strings.HasPrefix(remaining, "/*"):
			if end := strings.Index(remaining[2:], "*/"); end >= 0 {
				remaining = remaining[end+4:]
				continue
			}
			return remaining
		default:
			return remaining
		}
	}
}

// hasAdditionalSQLStatement rejects stacked SQL while allowing one optional
// trailing semicolon. It understands SQLite strings, identifiers and comments.
func hasAdditionalSQLStatement(input string) bool {
	const (
		plain = iota
		singleQuote
		doubleQuote
		backtickQuote
		bracketQuote
		lineComment
		blockComment
	)
	state := plain
	for i := 0; i < len(input); i++ {
		ch := input[i]
		next := byte(0)
		if i+1 < len(input) {
			next = input[i+1]
		}
		switch state {
		case plain:
			switch {
			case ch == '\'':
				state = singleQuote
			case ch == '"':
				state = doubleQuote
			case ch == '`':
				state = backtickQuote
			case ch == '[':
				state = bracketQuote
			case ch == '-' && next == '-':
				state = lineComment
				i++
			case ch == '/' && next == '*':
				state = blockComment
				i++
			case ch == ';':
				return stripSQLCommentsAndSpace(input[i+1:]) != ""
			}
		case singleQuote:
			if ch == '\'' {
				if next == '\'' {
					i++
				} else {
					state = plain
				}
			}
		case doubleQuote:
			if ch == '"' {
				if next == '"' {
					i++
				} else {
					state = plain
				}
			}
		case backtickQuote:
			if ch == '`' {
				state = plain
			}
		case bracketQuote:
			if ch == ']' {
				state = plain
			}
		case lineComment:
			if ch == '\n' {
				state = plain
			}
		case blockComment:
			if ch == '*' && next == '/' {
				state = plain
				i++
			}
		}
	}
	return false
}

func firstSQLKeyword(input string) string {
	input = stripSQLCommentsAndSpace(input)
	end := 0
	for end < len(input) {
		ch := input[end]
		if ch < 'A' || ch > 'Z' && ch < 'a' || ch > 'z' {
			break
		}
		end++
	}
	return strings.ToUpper(input[:end])
}

type sqlPolicyToken struct {
	word       string
	symbol     byte
	identifier bool
}

// tokenizeSQLForPolicy is a deliberately small lexer for the mutation policy.
// It does not try to validate SQLite syntax; SQLite still does that. Its job is
// only to distinguish CTE structure from quoted text/comments so the policy can
// identify the top-level statement that follows a WITH clause without guessing
// from a substring.
func tokenizeSQLForPolicy(input string) ([]sqlPolicyToken, error) {
	var tokens []sqlPolicyToken
	for i := 0; i < len(input); {
		ch := input[i]
		switch {
		case ch == ' ' || ch == '\t' || ch == '\r' || ch == '\n' || ch == '\f':
			i++
		case ch == '-' && i+1 < len(input) && input[i+1] == '-':
			i += 2
			for i < len(input) && input[i] != '\n' {
				i++
			}
		case ch == '/' && i+1 < len(input) && input[i+1] == '*':
			end := strings.Index(input[i+2:], "*/")
			if end < 0 {
				return nil, fmt.Errorf("unterminated block comment")
			}
			i += end + 4
		case ch == '\'' || ch == '"' || ch == '`':
			quote := ch
			quotedIdentifier := quote != '\''
			i++
			closed := false
			for i < len(input) {
				if input[i] != quote {
					i++
					continue
				}
				if i+1 < len(input) && input[i+1] == quote {
					i += 2
					continue
				}
				i++
				closed = true
				break
			}
			if !closed {
				return nil, fmt.Errorf("unterminated quoted value")
			}
			tokens = append(tokens, sqlPolicyToken{identifier: quotedIdentifier})
		case ch == '[':
			i++
			closed := false
			for i < len(input) {
				if input[i] == ']' {
					i++
					closed = true
					break
				}
				i++
			}
			if !closed {
				return nil, fmt.Errorf("unterminated quoted identifier")
			}
			tokens = append(tokens, sqlPolicyToken{identifier: true})
		case ch == '(' || ch == ')' || ch == ',' || ch == ';':
			tokens = append(tokens, sqlPolicyToken{symbol: ch})
			i++
		case ch == '_' || ch >= 'A' && ch <= 'Z' || ch >= 'a' && ch <= 'z':
			start := i
			i++
			for i < len(input) {
				current := input[i]
				if current == '_' || current == '$' || current >= 'A' && current <= 'Z' || current >= 'a' && current <= 'z' || current >= '0' && current <= '9' {
					i++
					continue
				}
				break
			}
			tokens = append(tokens, sqlPolicyToken{word: strings.ToUpper(input[start:i]), identifier: true})
		default:
			tokens = append(tokens, sqlPolicyToken{symbol: ch})
			i++
		}
	}
	return tokens, nil
}

func skipSQLPolicyParentheses(tokens []sqlPolicyToken, start int) (int, error) {
	if start >= len(tokens) || tokens[start].symbol != '(' {
		return start, fmt.Errorf("expected parenthesized CTE query")
	}
	depth := 0
	for i := start; i < len(tokens); i++ {
		switch tokens[i].symbol {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return i + 1, nil
			}
			if depth < 0 {
				return i, fmt.Errorf("unbalanced parentheses")
			}
		}
	}
	return len(tokens), fmt.Errorf("unterminated parenthesized CTE query")
}

// statementKeywordAfterWith parses only the SQLite WITH-clause envelope and
// returns the top-level statement keyword after its CTE definitions. This keeps
// the mutation endpoint fail-closed: WITH ... SELECT/CREATE/PRAGMA are not
// authorized merely because a nested or quoted INSERT token exists.
func statementKeywordAfterWith(input string) (string, error) {
	tokens, err := tokenizeSQLForPolicy(input)
	if err != nil {
		return "", err
	}
	if len(tokens) == 0 || tokens[0].word != "WITH" {
		return "", fmt.Errorf("expected WITH")
	}
	index := 1
	if index < len(tokens) && tokens[index].word == "RECURSIVE" {
		index++
	}
	for {
		if index >= len(tokens) || !tokens[index].identifier {
			return "", fmt.Errorf("expected CTE name")
		}
		index++
		if index < len(tokens) && tokens[index].symbol == '(' {
			index, err = skipSQLPolicyParentheses(tokens, index)
			if err != nil {
				return "", err
			}
		}
		if index >= len(tokens) || tokens[index].word != "AS" {
			return "", fmt.Errorf("expected AS after CTE name")
		}
		index++
		if index < len(tokens) && tokens[index].word == "NOT" {
			index++
			if index >= len(tokens) || tokens[index].word != "MATERIALIZED" {
				return "", fmt.Errorf("expected MATERIALIZED after NOT")
			}
			index++
		} else if index < len(tokens) && tokens[index].word == "MATERIALIZED" {
			index++
		}
		index, err = skipSQLPolicyParentheses(tokens, index)
		if err != nil {
			return "", err
		}
		if index < len(tokens) && tokens[index].symbol == ',' {
			index++
			continue
		}
		if index >= len(tokens) || tokens[index].word == "" {
			return "", fmt.Errorf("expected statement after WITH clause")
		}
		return tokens[index].word, nil
	}
}

func validateReadSQL(input string) error {
	trimmed := stripSQLCommentsAndSpace(input)
	if trimmed == "" {
		return fmt.Errorf("sql cannot be empty")
	}
	if len(trimmed) > maximumMutationSQL {
		return fmt.Errorf("sql exceeds %d bytes", maximumMutationSQL)
	}
	if hasAdditionalSQLStatement(trimmed) {
		return fmt.Errorf("exactly one SQL statement is allowed")
	}
	if strings.Contains(strings.ToLower(trimmed), "load_extension") {
		return fmt.Errorf("extension loading is not allowed")
	}
	switch firstSQLKeyword(trimmed) {
	case "SELECT", "WITH", "EXPLAIN":
		return nil
	case "PRAGMA":
		if isSafeReadPragma(trimmed) {
			return nil
		}
		return fmt.Errorf("pragma is not in the read-only allowlist")
	default:
		return fmt.Errorf("only SELECT, read-only WITH/EXPLAIN, and safe schema PRAGMA statements are allowed")
	}
}

func validateMutationSQL(input string) error {
	trimmed := stripSQLCommentsAndSpace(input)
	if trimmed == "" {
		return fmt.Errorf("sql cannot be empty")
	}
	if len(trimmed) > maximumMutationSQL {
		return fmt.Errorf("sql exceeds %d bytes", maximumMutationSQL)
	}
	if hasAdditionalSQLStatement(trimmed) {
		return fmt.Errorf("exactly one SQL statement is allowed")
	}
	keyword := firstSQLKeyword(trimmed)
	if keyword == "WITH" {
		var err error
		keyword, err = statementKeywordAfterWith(trimmed)
		if err != nil {
			return fmt.Errorf("invalid WITH mutation: %w", err)
		}
	}
	switch keyword {
	case "INSERT", "UPDATE", "DELETE":
		return nil
	default:
		return fmt.Errorf("only INSERT, UPDATE, and DELETE, optionally prefixed by WITH, are allowed; schema changes use workflow migrations")
	}
}

// QueryWorkflowDB handles POST /api/query — runs a read-only SQL query against a
// workflow's db/db.sqlite and returns rows as an array of objects.
func QueryWorkflowDB(c *gin.Context) {
	var req models.QueryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.APIResponse[any]{
			Success: false, Message: "Invalid request body", Error: err.Error(),
		})
		return
	}
	if strings.TrimSpace(req.SQL) == "" {
		c.JSON(http.StatusBadRequest, models.APIResponse[any]{
			Success: false, Message: "sql is required", Error: "sql cannot be empty",
		})
		return
	}
	if err := validateReadSQL(req.SQL); err != nil {
		c.JSON(http.StatusBadRequest, models.APIResponse[any]{
			Success: false, Message: "Query rejected", Error: err.Error(),
		})
		return
	}

	fullPath, err := resolveReadonlyDBPath(c, req.DBPath)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.APIResponse[any]{
			Success: false, Message: "Invalid db_path", Error: err.Error(),
		})
		return
	}

	db, err := openQueryOnlyDB(fullPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.APIResponse[any]{
			Success: false, Message: "Failed to open database", Error: err.Error(),
		})
		return
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(c.Request.Context(), queryTimeout)
	defer cancel()

	rows, err := db.QueryContext(ctx, req.SQL, req.Params...)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.APIResponse[any]{
			Success: false, Message: "Query failed", Error: err.Error(),
		})
		return
	}
	defer rows.Close()

	maxRows := req.MaxRows
	if maxRows <= 0 {
		maxRows = defaultQueryMaxRows
	}
	if maxRows > maximumQueryMaxRows {
		maxRows = maximumQueryMaxRows
	}
	cols, data, truncated, err := scanRows(rows, maxRows)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.APIResponse[any]{
			Success: false, Message: "Failed to read rows", Error: err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, models.APIResponse[models.QueryResponse]{
		Success: true,
		Data:    models.QueryResponse{Columns: cols, Rows: data, Truncated: truncated},
	})
}

// MutateWorkflowDB handles POST /api/mutate. The caller must already hold the
// workspace service token; agent-side tool registration and FolderGuard decide
// whether that trusted session has workflow DB write authority.
func MutateWorkflowDB(c *gin.Context) {
	var req models.MutationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.APIResponse[any]{Success: false, Message: "Invalid request body", Error: err.Error()})
		return
	}
	if len(req.Statements) == 0 || len(req.Statements) > maximumStatements {
		c.JSON(http.StatusBadRequest, models.APIResponse[any]{Success: false, Message: "Invalid statements", Error: fmt.Sprintf("statements must contain 1-%d operations", maximumStatements)})
		return
	}
	for i, statement := range req.Statements {
		if err := validateMutationSQL(statement.SQL); err != nil {
			c.JSON(http.StatusBadRequest, models.APIResponse[any]{Success: false, Message: "Mutation rejected", Error: fmt.Sprintf("statement %d: %v", i+1, err)})
			return
		}
	}

	fullPath, err := resolveReadonlyDBPath(c, req.DBPath)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.APIResponse[any]{Success: false, Message: "Invalid db_path", Error: err.Error()})
		return
	}
	db, err := openMutationDB(fullPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.APIResponse[any]{Success: false, Message: "Failed to open database", Error: err.Error()})
		return
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(c.Request.Context(), queryTimeout)
	defer cancel()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.APIResponse[any]{Success: false, Message: "Failed to start transaction", Error: err.Error()})
		return
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	response := models.MutationResponse{Results: make([]models.MutationStatementResult, 0, len(req.Statements))}
	for i, statement := range req.Statements {
		result, execErr := tx.ExecContext(ctx, statement.SQL, statement.Params...)
		if execErr != nil {
			c.JSON(http.StatusBadRequest, models.APIResponse[any]{Success: false, Message: "Mutation failed and was rolled back", Error: fmt.Sprintf("statement %d: %v", i+1, execErr)})
			return
		}
		rowsAffected, rowsErr := result.RowsAffected()
		if rowsErr != nil {
			c.JSON(http.StatusInternalServerError, models.APIResponse[any]{Success: false, Message: "Failed to build mutation receipt", Error: rowsErr.Error()})
			return
		}
		lastInsertID, _ := result.LastInsertId()
		response.Results = append(response.Results, models.MutationStatementResult{RowsAffected: rowsAffected, LastInsertID: lastInsertID})
		response.TotalRowsAffected += rowsAffected
	}
	if err := tx.Commit(); err != nil {
		c.JSON(http.StatusInternalServerError, models.APIResponse[any]{Success: false, Message: "Failed to commit mutation", Error: err.Error()})
		return
	}
	committed = true
	log.Printf("[WORKFLOW_DB_MUTATION] user=%q db=%q statements=%d rows_affected=%d", getUserID(c), req.DBPath, len(req.Statements), response.TotalRowsAffected)
	c.JSON(http.StatusOK, models.APIResponse[models.MutationResponse]{Success: true, Data: response})
}

// GetWorkflowDBTables handles GET /api/db/tables?db_path=... — lists tables,
// per-table schema, row count and a small sample, for the DatabasePopup viewer.
func GetWorkflowDBTables(c *gin.Context) {
	dbPath := c.Query("db_path")
	if strings.TrimSpace(dbPath) == "" {
		c.JSON(http.StatusBadRequest, models.APIResponse[any]{
			Success: false, Message: "db_path is required", Error: "db_path query parameter cannot be empty",
		})
		return
	}

	fullPath, err := resolveReadonlyDBPath(c, dbPath)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.APIResponse[any]{
			Success: false, Message: "Invalid db_path", Error: err.Error(),
		})
		return
	}

	db, err := openQueryOnlyDB(fullPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.APIResponse[any]{
			Success: false, Message: "Failed to open database", Error: err.Error(),
		})
		return
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(c.Request.Context(), queryTimeout)
	defer cancel()

	tableNames, err := listTableNames(ctx, db)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.APIResponse[any]{
			Success: false, Message: "Failed to list tables", Error: err.Error(),
		})
		return
	}

	tables := make([]models.DBTableInfo, 0, len(tableNames))
	for _, name := range tableNames {
		info := models.DBTableInfo{Name: name}

		if cols, err := tableColumns(ctx, db, name); err == nil {
			info.Columns = cols
		}

		quoted := quoteIdent(name)
		_ = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+quoted).Scan(&info.RowCount)

		if sampleRows, err := db.QueryContext(ctx, "SELECT * FROM "+quoted+" LIMIT ?", dbTablesSampleRows); err == nil {
			if _, data, _, err := scanRows(sampleRows, dbTablesSampleRows); err == nil {
				info.Sample = data
			}
			sampleRows.Close()
		}

		tables = append(tables, info)
	}

	c.JSON(http.StatusOK, models.APIResponse[models.DBTablesResponse]{
		Success: true,
		Data:    models.DBTablesResponse{Tables: tables},
	})
}

// listTableNames returns user table names (excluding sqlite_* internal tables).
func listTableNames(ctx context.Context, db *sql.DB) ([]string, error) {
	rows, err := db.QueryContext(ctx,
		"SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%' ORDER BY name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var names []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return nil, err
		}
		names = append(names, n)
	}
	return names, rows.Err()
}

// tableColumns returns column metadata via PRAGMA table_info.
func tableColumns(ctx context.Context, db *sql.DB, table string) ([]models.DBColumnInfo, error) {
	rows, err := db.QueryContext(ctx, "PRAGMA table_info("+quoteIdent(table)+")")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var cols []models.DBColumnInfo
	for rows.Next() {
		var (
			cid       int
			name      string
			ctype     string
			notNull   int
			dfltValue interface{}
			pk        int
		)
		if err := rows.Scan(&cid, &name, &ctype, &notNull, &dfltValue, &pk); err != nil {
			return nil, err
		}
		cols = append(cols, models.DBColumnInfo{Name: name, Type: ctype, PrimaryKey: pk > 0})
	}
	return cols, rows.Err()
}

// quoteIdent quotes a SQLite identifier (table/column) by doubling embedded
// double-quotes. Names come from sqlite_master (already-created objects), so
// this is defense-in-depth, not untrusted input.
func quoteIdent(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}
