package step_based_workflow

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	_ "modernc.org/sqlite"
)

func TestExtractReportQueriesHandlesAllQuoteStyles(t *testing.T) {
	html := "" +
		"<script>\n" +
		"const rows1 = await window.report.query('SELECT id FROM emails');\n" +
		"const rows2 = await window.report.query(\"SELECT status FROM emails\");\n" +
		"const rows3 = await window.report.query(`SELECT note FROM emails`);\n" +
		"</script>"
	got := extractReportQueries(html)
	want := []string{"SELECT id FROM emails", "SELECT status FROM emails", "SELECT note FROM emails"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("extractReportQueries = %v, want %v", got, want)
	}
}

func TestExtractReportQueriesDedupsAndPreservesOrder(t *testing.T) {
	html := "" +
		"window.report.query('SELECT b FROM t');\n" +
		"window.report.query('SELECT a FROM t');\n" +
		"window.report.query('SELECT b FROM t');\n"
	got := extractReportQueries(html)
	want := []string{"SELECT b FROM t", "SELECT a FROM t"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("extractReportQueries = %v, want %v (dedup should keep first occurrence position)", got, want)
	}
}

func TestExtractReportQueriesHandlesEscapedQuotes(t *testing.T) {
	html := `window.report.query('SELECT * FROM t WHERE name = \'x\'')`
	got := extractReportQueries(html)
	want := []string{`SELECT * FROM t WHERE name = 'x'`}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("extractReportQueries = %v, want %v", got, want)
	}
}

func TestExtractReportQueriesReturnsEmptyForNoCalls(t *testing.T) {
	got := extractReportQueries("<html><body>no report calls here</body></html>")
	if len(got) != 0 {
		t.Fatalf("extractReportQueries = %v, want empty", got)
	}
}

func setupPlanDriftDBTest(t *testing.T, workspacePath string) string {
	t.Helper()
	docsDir := t.TempDir()
	t.Setenv("WORKSPACE_DOCS_PATH", docsDir)
	dbDir := filepath.Join(docsDir, workspacePath, "db")
	if err := os.MkdirAll(dbDir, 0o755); err != nil {
		t.Fatalf("mkdir db dir: %v", err)
	}
	return filepath.Join(dbDir, "db.sqlite")
}

func TestCheckReportQueryCompatibilityPassesWhenSchemaMatches(t *testing.T) {
	ctx := context.Background()
	dbPath := setupPlanDriftDBTest(t, "Workflow/drift-test")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE emails(id INTEGER PRIMARY KEY, status TEXT)`); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()

	readFile := func(_ context.Context, path string) (string, error) {
		if path == "Workflow/drift-test/db/reports/index.html" {
			return `<script>window.report.query('SELECT id, status FROM emails')</script>`, nil
		}
		return "", os.ErrNotExist
	}

	check, err := CheckReportQueryCompatibility(ctx, "Workflow/drift-test", readFile)
	if err != nil {
		t.Fatalf("CheckReportQueryCompatibility returned error: %v", err)
	}
	if check.Status != "pass" {
		t.Fatalf("status = %q, want pass; evidence=%s", check.Status, check.Evidence)
	}
	if check.CheckID != reportDriftCheckID {
		t.Fatalf("check_id = %q, want %q", check.CheckID, reportDriftCheckID)
	}
	if check.Evidence == "" {
		t.Fatal("evidence must not be empty even on pass")
	}
}

func TestCheckReportQueryCompatibilityFailsWhenColumnDropped(t *testing.T) {
	ctx := context.Background()
	dbPath := setupPlanDriftDBTest(t, "Workflow/drift-test")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	// Simulates a step that used to write a "status" column and no longer does.
	if _, err := db.Exec(`CREATE TABLE emails(id INTEGER PRIMARY KEY)`); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()

	readFile := func(_ context.Context, path string) (string, error) {
		if path == "Workflow/drift-test/db/reports/index.html" {
			return `<script>window.report.query('SELECT id, status FROM emails')</script>`, nil
		}
		return "", os.ErrNotExist
	}

	check, err := CheckReportQueryCompatibility(ctx, "Workflow/drift-test", readFile)
	if err != nil {
		t.Fatalf("CheckReportQueryCompatibility returned error: %v", err)
	}
	if check.Status != "fail" {
		t.Fatalf("status = %q, want fail (report query references a dropped column)", check.Status)
	}
	if check.Evidence == "" {
		t.Fatal("evidence must not be empty on fail")
	}
}

func TestCheckReportQueryCompatibilityPassesWhenNoReportExists(t *testing.T) {
	ctx := context.Background()
	setupPlanDriftDBTest(t, "Workflow/drift-test")

	readFile := func(_ context.Context, _ string) (string, error) {
		return "", os.ErrNotExist
	}

	check, err := CheckReportQueryCompatibility(ctx, "Workflow/drift-test", readFile)
	if err != nil {
		t.Fatalf("CheckReportQueryCompatibility returned error: %v", err)
	}
	if check.Status != "pass" {
		t.Fatalf("status = %q, want pass (no report to check)", check.Status)
	}
	if check.Evidence == "" {
		t.Fatal("evidence must explain why there was nothing to check")
	}
}

func TestCheckReportQueryCompatibilityDoesNotMutateDB(t *testing.T) {
	ctx := context.Background()
	dbPath := setupPlanDriftDBTest(t, "Workflow/drift-test")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE emails(id INTEGER PRIMARY KEY, status TEXT); INSERT INTO emails(id, status) VALUES (1, 'pending')`); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()

	// A malicious/buggy report embedding a mutation must never actually run —
	// the check opens the DB query_only, so this must fail to execute, not
	// silently mutate real data.
	readFile := func(_ context.Context, path string) (string, error) {
		if path == "Workflow/drift-test/db/reports/index.html" {
			return `<script>window.report.query("UPDATE emails SET status='hacked'")</script>`, nil
		}
		return "", os.ErrNotExist
	}

	if _, err := CheckReportQueryCompatibility(ctx, "Workflow/drift-test", readFile); err != nil {
		t.Fatalf("CheckReportQueryCompatibility returned error: %v", err)
	}

	check, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer check.Close()
	var status string
	if err := check.QueryRow(`SELECT status FROM emails WHERE id=1`).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "pending" {
		t.Fatalf("status = %q, want unchanged %q — query_only guard did not hold", status, "pending")
	}
}
