package virtualtools

import (
	"context"
	"database/sql"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/common"
	workspacehandlers "github.com/manishiitg/coding-agent-loop/workspace/handlers"
	"github.com/spf13/viper"
)

// PLAT-126. SQLite reserves $, @, :, and ? to introduce a bind parameter
// directly in SQL text. A string literal written unquoted that happens to
// start with $ or @ is not a parameter reference, but SQLite cannot tell
// that — it reports the sigil itself as an unrecognized token, which reads
// as a location, not an explanation. The dominant real instance is
// json_extract(col, $.field) instead of json_extract(col, '$.field'): 19 of
// 22 identical failures on one workflow (social-media) in one day, none of
// which told the caller that the fix was quoting the path.
//
// These reuse startWorkflowDBSchemaHintServer (workflow_db_schema_hint_test.go)
// — the real workspace POST /api/query handler over a real SQLite file — so
// the assertions are against SQLite's own error text, not a hand-written
// stand-in for it.

// startWorkflowDBMutationServer stands up the production write path — the
// workspace POST /api/mutate handler over a real SQLite file, with a session
// granted db_access=read-write — the same shape as
// startWorkflowDBSchemaHintServer (workflow_db_schema_hint_test.go), applied
// to the mutation route instead of the query route.
func startWorkflowDBMutationServer(t *testing.T) WorkflowDBToolRegistry {
	t.Helper()
	root := t.TempDir()
	const workspacePath = "Workflow/sigil-hint"
	dbDir := filepath.Join(root, filepath.FromSlash(workspacePath), "db")
	if err := os.MkdirAll(dbDir, 0o755); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", filepath.Join(dbDir, "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		"CREATE TABLE human_inputs (id INTEGER PRIMARY KEY, question TEXT, status TEXT)",
		"INSERT INTO human_inputs(id, question, status) VALUES (1, 'ready?', 'open')",
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

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/api/mutate", workspacehandlers.MutateWorkflowDB)
	server := httptest.NewServer(router)
	t.Cleanup(server.Close)

	sessionID := "workflow-db-sigil-hint-" + t.Name()
	common.SetSessionFolderGuard(sessionID, []string{workspacePath + "/db"}, []string{workspacePath + "/db"})
	common.SetSessionShellEnv(sessionID, map[string]string{workflowDBAccessEnv: "read-write"})
	t.Cleanup(func() { common.ClearSessionShellConfig(sessionID) })

	return CreateWorkflowDBToolRegistry(server.URL, "", sessionID)
}

func TestQueryWorkflowDBUnquotedJSONPathExplainsTheFix(t *testing.T) {
	registry, _ := startWorkflowDBSchemaHintServer(t, 0)

	_, err := registry.Executors["query_workflow_db"](context.Background(), map[string]any{
		"sql": "SELECT json_extract(question, $.field) FROM human_inputs",
	})
	if err == nil {
		t.Fatal("expected the unquoted JSON path to fail")
	}
	message := err.Error()
	if !strings.Contains(message, `unrecognized token: \"$\"`) {
		t.Fatalf("original SQLite error was dropped: %s", message)
	}
	if !strings.Contains(message, "bind parameter") {
		t.Fatalf("hint does not explain why SQLite rejected this: %s", message)
	}
	if !strings.Contains(message, "json_extract(col, '$.field')") {
		t.Fatalf("hint does not show the corrected form: %s", message)
	}
}

func TestQueryWorkflowDBUnquotedAtSigilExplainsTheFix(t *testing.T) {
	registry, _ := startWorkflowDBSchemaHintServer(t, 0)

	_, err := registry.Executors["query_workflow_db"](context.Background(), map[string]any{
		"sql": "SELECT ltrim(question, @) FROM human_inputs",
	})
	if err == nil {
		t.Fatal("expected the unquoted @ literal to fail")
	}
	message := err.Error()
	if !strings.Contains(message, `unrecognized token: \"@\"`) {
		t.Fatalf("original SQLite error was dropped: %s", message)
	}
	if !strings.Contains(message, "bind parameter") {
		t.Fatalf("hint does not explain why SQLite rejected this: %s", message)
	}
}

func TestMutateWorkflowDBUnquotedJSONPathExplainsTheFix(t *testing.T) {
	registry := startWorkflowDBMutationServer(t)

	_, err := registry.Executors["mutate_workflow_db"](context.Background(), map[string]any{
		"sql": "UPDATE human_inputs SET question = json_extract(question, $.field) WHERE id = 1",
	})
	if err == nil {
		t.Fatal("expected the unquoted JSON path to fail")
	}
	message := err.Error()
	if !strings.Contains(message, `unrecognized token: \"$\"`) {
		t.Fatalf("original SQLite error was dropped: %s", message)
	}
	if !strings.Contains(message, "json_extract(col, '$.field')") {
		t.Fatalf("hint does not show the corrected form: %s", message)
	}
}

// A quoted JSON path is the overwhelmingly common case and must be completely
// unaffected: no hint text, no behavior change, same result as before this
// existed.
func TestQueryWorkflowDBQuotedJSONPathIsUnaffected(t *testing.T) {
	registry, _ := startWorkflowDBSchemaHintServer(t, 0)

	result, err := registry.Executors["query_workflow_db"](context.Background(), map[string]any{
		"sql": `SELECT json_extract('{"field":"ready?"}', '$.field') AS field`,
	})
	if err != nil {
		t.Fatalf("a correctly quoted JSON path must succeed: %v", err)
	}
	if !strings.Contains(result, "ready?") {
		t.Fatalf("unexpected result for a quoted JSON path: %s", result)
	}
}

// A genuine syntax error unrelated to $ or @ must reach the caller exactly as
// SQLite reported it — the sigil hint must not fire on unrelated mistakes.
func TestQueryWorkflowDBUnrelatedSyntaxErrorIsUnaffected(t *testing.T) {
	registry, _ := startWorkflowDBSchemaHintServer(t, 0)

	_, err := registry.Executors["query_workflow_db"](context.Background(), map[string]any{
		"sql": "SELECT FROM human_inputs",
	})
	if err == nil {
		t.Fatal("expected a syntax error")
	}
	message := err.Error()
	if strings.Contains(message, "bind parameter") {
		t.Fatalf("sigil hint fired on an unrelated syntax error: %s", message)
	}
}
