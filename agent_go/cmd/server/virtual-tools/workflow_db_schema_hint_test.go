package virtualtools

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/common"
	workspacehandlers "github.com/manishiitg/coding-agent-loop/workspace/handlers"
	"github.com/spf13/viper"
	_ "modernc.org/sqlite"
)

// startWorkflowDBSchemaHintServer stands up the production read path — the
// workspace POST /api/query handler over a real SQLite file — so the tests
// observe SQLite's own "no such column" / "no such table" text rather than a
// hand-written error string. failAfter, when positive, makes every request past
// that count fail, which is how the follow-up describe is broken without
// touching the detection logic.
func startWorkflowDBSchemaHintServer(t *testing.T, failAfter int) (WorkflowDBToolRegistry, *int32) {
	t.Helper()
	root := t.TempDir()
	const workspacePath = "Workflow/schema-hint"
	dbDir := filepath.Join(root, filepath.FromSlash(workspacePath), "db")
	if err := os.MkdirAll(dbDir, 0o755); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", filepath.Join(dbDir, "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		"CREATE TABLE human_inputs (id INTEGER PRIMARY KEY, question TEXT, status TEXT, answered_at TEXT, workflow_id TEXT)",
		"CREATE TABLE runs (run_id INTEGER PRIMARY KEY, outcome TEXT)",
		"CREATE VIEW open_inputs AS SELECT id, question FROM human_inputs WHERE status = 'open'",
		"INSERT INTO human_inputs(question, status, answered_at, workflow_id) VALUES ('ready?', 'open', NULL, 'wf-1')",
	} {
		if _, err := db.Exec(statement); err != nil {
			t.Fatalf("%s: %v", statement, err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	previousDocsDir := viper.Get("docs-dir")
	viper.Set("docs-dir", root)
	t.Cleanup(func() { viper.Set("docs-dir", previousDocsDir) })

	var requests int32
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/api/query", func(c *gin.Context) {
		if count := atomic.AddInt32(&requests, 1); failAfter > 0 && int(count) > failAfter {
			c.AbortWithStatus(http.StatusServiceUnavailable)
			return
		}
		workspacehandlers.QueryWorkflowDB(c)
	})
	server := httptest.NewServer(router)
	t.Cleanup(server.Close)

	sessionID := "workflow-db-schema-hint-" + t.Name()
	common.SetSessionFolderGuard(sessionID, []string{workspacePath + "/db"}, nil)
	t.Cleanup(func() { common.ClearSessionShellConfig(sessionID) })

	return CreateWorkflowDBToolRegistry(server.URL, "", sessionID), &requests
}

func TestQueryWorkflowDBUnknownColumnReportsThatTablesColumns(t *testing.T) {
	registry, _ := startWorkflowDBSchemaHintServer(t, 0)

	_, err := registry.Executors["query_workflow_db"](context.Background(), map[string]any{
		"sql": "SELECT input_id FROM human_inputs WHERE status = 'open'",
	})
	if err == nil {
		t.Fatal("expected the unknown column to fail")
	}
	message := err.Error()
	if !strings.Contains(message, "no such column: input_id") {
		t.Fatalf("original SQLite error was dropped: %s", message)
	}
	if !strings.Contains(message, "Table `human_inputs` has columns:") {
		t.Fatalf("schema hint missing: %s", message)
	}
	for _, column := range []string{"id", "question", "status", "answered_at", "workflow_id"} {
		if !strings.Contains(message, column) {
			t.Fatalf("column %q missing from hint: %s", column, message)
		}
	}
}

func TestQueryWorkflowDBUnknownTableReportsAvailableTables(t *testing.T) {
	registry, _ := startWorkflowDBSchemaHintServer(t, 0)

	_, err := registry.Executors["query_workflow_db"](context.Background(), map[string]any{
		"sql": "SELECT * FROM strategy_pool",
	})
	if err == nil {
		t.Fatal("expected the unknown table to fail")
	}
	message := err.Error()
	if !strings.Contains(message, "no such table: strategy_pool") {
		t.Fatalf("original SQLite error was dropped: %s", message)
	}
	if !strings.Contains(message, "Tables and views in this database:") {
		t.Fatalf("table listing missing: %s", message)
	}
	for _, name := range []string{"human_inputs", "runs", "open_inputs"} {
		if !strings.Contains(message, name) {
			t.Fatalf("%q missing from table listing: %s", name, message)
		}
	}
	if strings.Contains(message, "has columns:") {
		t.Fatalf("a missing table must not be described as if it existed: %s", message)
	}
}

func TestQueryWorkflowDBUnknownColumnAcrossJoinNamesNoTable(t *testing.T) {
	registry, _ := startWorkflowDBSchemaHintServer(t, 0)

	_, err := registry.Executors["query_workflow_db"](context.Background(), map[string]any{
		"sql": "SELECT x FROM human_inputs JOIN runs ON runs.run_id = human_inputs.id",
	})
	if err == nil {
		t.Fatal("expected the unknown column to fail")
	}
	message := err.Error()
	if !strings.Contains(message, "no such column: x") {
		t.Fatalf("original SQLite error was dropped: %s", message)
	}
	// Two tables are in scope, so neither may be claimed as the column's owner.
	if strings.Contains(message, "has columns:") {
		t.Fatalf("guessed a table for an ambiguous column: %s", message)
	}
	if !strings.Contains(message, "could not be identified") || !strings.Contains(message, "human_inputs") {
		t.Fatalf("expected a table listing instead: %s", message)
	}
}

func TestQueryWorkflowDBSchemaLookupFailureKeepsOriginalError(t *testing.T) {
	// The first request (the caller's query) succeeds in reaching SQLite; the
	// follow-up describe finds the workspace service unavailable.
	registry, requests := startWorkflowDBSchemaHintServer(t, 1)

	_, err := registry.Executors["query_workflow_db"](context.Background(), map[string]any{
		"sql": "SELECT input_id FROM human_inputs",
	})
	if err == nil {
		t.Fatal("expected the unknown column to fail")
	}
	if atomic.LoadInt32(requests) < 2 {
		t.Fatalf("describe follow-up never ran: requests=%d", atomic.LoadInt32(requests))
	}
	message := err.Error()
	if !strings.Contains(message, "no such column: input_id") {
		t.Fatalf("original SQLite error was dropped: %s", message)
	}
	if strings.Contains(message, "has columns:") || strings.Contains(message, "Tables and views") {
		t.Fatalf("a failed describe must add nothing: %s", message)
	}
	if strings.Contains(message, "503") || strings.Contains(message, "Service Unavailable") {
		t.Fatalf("the describe failure leaked into the caller's error: %s", message)
	}
}

func TestQueryWorkflowDBSuccessfulQueryIsUnchanged(t *testing.T) {
	registry, requests := startWorkflowDBSchemaHintServer(t, 0)

	result, err := registry.Executors["query_workflow_db"](context.Background(), map[string]any{
		"sql": "SELECT question, status FROM human_inputs ORDER BY id",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, `"ready?"`) || !strings.Contains(result, `"columns"`) {
		t.Fatalf("unexpected query payload: %s", result)
	}
	if got := atomic.LoadInt32(requests); got != 1 {
		t.Fatalf("a successful query must not trigger a describe: requests=%d", got)
	}
}

func TestWorkflowDBTableFromSQLOnlyAnswersWhenCertain(t *testing.T) {
	for _, testCase := range []struct{ sqlText, want string }{
		{"SELECT a FROM human_inputs", "human_inputs"},
		{"select a from \"quoted_inputs\" qi where qi.a = 1", "quoted_inputs"},
		// Rejected by safeWorkflowDBTableName, so it is never fed back into a PRAGMA.
		{"select a from \"human inputs\"", ""},
		{"SELECT a FROM human_inputs hi LEFT JOIN human_inputs h2 ON h2.id = hi.id", "human_inputs"},
		{"SELECT a FROM human_inputs, runs", ""},
		{"SELECT a FROM (SELECT id FROM human_inputs) t JOIN runs ON 1=1", ""},
		{"SELECT 'from runs' AS label FROM human_inputs", "human_inputs"},
		{"SELECT a FROM human_inputs -- from runs\n", "human_inputs"},
		{"SELECT a FROM /* from runs */ human_inputs", "human_inputs"},
		{"PRAGMA table_info(\"human_inputs\")", ""},
		{"SELECT 1", ""},
	} {
		if got := workflowDBTableFromSQL(testCase.sqlText); got != testCase.want {
			t.Errorf("workflowDBTableFromSQL(%q)=%q, want %q", testCase.sqlText, got, testCase.want)
		}
	}
}

func TestWorkflowDBJoinWithinBudgetCapsWideSchemas(t *testing.T) {
	wide := make([]string, 300)
	for i := range wide {
		wide[i] = strings.Repeat("c", 20)
	}
	joined := workflowDBJoinWithinBudget(wide)
	if len(joined) > workflowDBSchemaHintBudget+32 {
		t.Fatalf("hint length=%d exceeds the budget", len(joined))
	}
	if !strings.Contains(joined, "more)") {
		t.Fatalf("truncation was not reported: %s", joined)
	}
}
