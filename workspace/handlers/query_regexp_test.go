package handlers

import (
	"bytes"
	"database/sql"
	"net/http"
	"testing"

	"github.com/manishiitg/coding-agent-loop/workspace/models"
)

// TestQueryWorkflowDBSupportsRegexpOperator pins PLAT-238: SQLite's REGEXP
// operator has no built-in implementation -- it is a syntax hook requiring
// an application-registered regexp(pattern, value) scalar function. Without
// that registration, any schema-declared REGEXP-based query fails uniformly
// with "no such function: REGEXP", even when the query itself is otherwise
// valid and correctly written.
func TestQueryWorkflowDBSupportsRegexpOperator(t *testing.T) {
	rel, abs, router := setupWorkflowDBTest(t)
	db, err := sql.Open("sqlite", abs)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`CREATE TABLE action_attempts_flat(id INTEGER PRIMARY KEY, status TEXT); ` +
		`INSERT INTO action_attempts_flat(status) VALUES ('strategy_gate_pass'), ('completed'), ('strategy_gate_fail');`); err != nil {
		t.Fatal(err)
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}

	recorder := postWorkflowDBTest(t, router, "/api/query", models.QueryRequest{
		DBPath: rel,
		SQL:    "SELECT status FROM action_attempts_flat WHERE status REGEXP 'strategy_gate_.*'",
	})
	if recorder.Code != http.StatusOK {
		t.Fatalf("REGEXP query failed: status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	body := recorder.Body.String()
	if !bytes.Contains([]byte(body), []byte("strategy_gate_pass")) || !bytes.Contains([]byte(body), []byte("strategy_gate_fail")) {
		t.Fatalf("REGEXP query did not match expected rows: %s", body)
	}
	if bytes.Contains([]byte(body), []byte(`"completed"`)) {
		t.Fatalf("REGEXP query incorrectly matched a non-matching row: %s", body)
	}
}

// TestQueryWorkflowDBRegexpRejectsInvalidPattern proves an invalid regex
// pattern surfaces as a query error rather than a panic or a false match.
func TestQueryWorkflowDBRegexpRejectsInvalidPattern(t *testing.T) {
	rel, abs, router := setupWorkflowDBTest(t)
	db, err := sql.Open("sqlite", abs)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`CREATE TABLE t(id INTEGER PRIMARY KEY, value TEXT); INSERT INTO t(value) VALUES ('x');`); err != nil {
		t.Fatal(err)
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}

	recorder := postWorkflowDBTest(t, router, "/api/query", models.QueryRequest{
		DBPath: rel,
		SQL:    "SELECT value FROM t WHERE value REGEXP '('", // unterminated group
	})
	if recorder.Code == http.StatusOK {
		t.Fatalf("expected an invalid regex pattern to fail the query, got 200: %s", recorder.Body.String())
	}
}

// TestQueryWorkflowDBRegexpTreatsNullOperandAsNull confirms REGEXP follows
// SQLite's own NULL-comparison convention rather than erroring on NULL.
func TestQueryWorkflowDBRegexpTreatsNullOperandAsNull(t *testing.T) {
	rel, abs, router := setupWorkflowDBTest(t)
	db, err := sql.Open("sqlite", abs)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`CREATE TABLE t(id INTEGER PRIMARY KEY, value TEXT); INSERT INTO t(value) VALUES (NULL);`); err != nil {
		t.Fatal(err)
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}

	recorder := postWorkflowDBTest(t, router, "/api/query", models.QueryRequest{
		DBPath: rel,
		SQL:    "SELECT COUNT(*) as n FROM t WHERE value REGEXP 'anything'",
	})
	if recorder.Code != http.StatusOK {
		t.Fatalf("REGEXP query with a NULL operand should not error: status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if !bytes.Contains(recorder.Body.Bytes(), []byte(`"n":0`)) {
		t.Fatalf("a NULL value should never match REGEXP: %s", recorder.Body.String())
	}
}
