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

func TestCheckValidationSchemaDBRulesPassesWhenAssertionsHold(t *testing.T) {
	ctx := context.Background()
	dbPath := setupPlanDriftDBTest(t, "Workflow/drift-test")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE emails(id INTEGER PRIMARY KEY, status TEXT); INSERT INTO emails VALUES (1, 'sent')`); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()

	rules := []DBValidationRule{{
		Name: "at least one sent email",
		SQL:  "SELECT status FROM emails WHERE status='sent'",
		Checks: []JSONValidationCheck{
			{Path: "$.status", MustExist: true, ValueType: "string"},
		},
	}}

	check, err := CheckValidationSchemaDBRules(ctx, "Workflow/drift-test", rules)
	if err != nil {
		t.Fatalf("CheckValidationSchemaDBRules returned error: %v", err)
	}
	if check.Status != "pass" {
		t.Fatalf("status = %q, want pass; evidence=%s", check.Status, check.Evidence)
	}
	if check.CheckID != validationSchemaDBDriftCheckID {
		t.Fatalf("check_id = %q, want %q", check.CheckID, validationSchemaDBDriftCheckID)
	}
}

func TestCheckValidationSchemaDBRulesFailsWhenColumnRenamed(t *testing.T) {
	ctx := context.Background()
	dbPath := setupPlanDriftDBTest(t, "Workflow/drift-test")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	// Simulates a step that renamed "status" to "delivery_status".
	if _, err := db.Exec(`CREATE TABLE emails(id INTEGER PRIMARY KEY, delivery_status TEXT); INSERT INTO emails VALUES (1, 'sent')`); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()

	rules := []DBValidationRule{{
		Name: "at least one sent email",
		SQL:  "SELECT status FROM emails WHERE status='sent'",
	}}

	check, err := CheckValidationSchemaDBRules(ctx, "Workflow/drift-test", rules)
	if err != nil {
		t.Fatalf("CheckValidationSchemaDBRules returned error: %v", err)
	}
	if check.Status != "fail" {
		t.Fatalf("status = %q, want fail (rule references a renamed column)", check.Status)
	}
}

func TestCheckValidationSchemaDBRulesFailsWhenRowCountAssertionBreaks(t *testing.T) {
	ctx := context.Background()
	dbPath := setupPlanDriftDBTest(t, "Workflow/drift-test")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	// Table exists and query runs fine, but now returns zero matching rows —
	// the step stopped writing status='sent' rows even though the schema
	// itself is intact.
	if _, err := db.Exec(`CREATE TABLE emails(id INTEGER PRIMARY KEY, status TEXT); INSERT INTO emails VALUES (1, 'draft')`); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()

	minRows := 1
	rules := []DBValidationRule{{
		Name:    "at least one sent email",
		SQL:     "SELECT status FROM emails WHERE status='sent'",
		MinRows: &minRows,
	}}

	check, err := CheckValidationSchemaDBRules(ctx, "Workflow/drift-test", rules)
	if err != nil {
		t.Fatalf("CheckValidationSchemaDBRules returned error: %v", err)
	}
	if check.Status != "fail" {
		t.Fatalf("status = %q, want fail (min_rows assertion no longer holds)", check.Status)
	}
}

func TestCheckValidationSchemaDBRulesPassesWhenNoRulesDeclared(t *testing.T) {
	ctx := context.Background()
	setupPlanDriftDBTest(t, "Workflow/drift-test")

	check, err := CheckValidationSchemaDBRules(ctx, "Workflow/drift-test", nil)
	if err != nil {
		t.Fatalf("CheckValidationSchemaDBRules returned error: %v", err)
	}
	if check.Status != "pass" {
		t.Fatalf("status = %q, want pass (no rules to check)", check.Status)
	}
	if check.Evidence == "" {
		t.Fatal("evidence must explain why there was nothing to check")
	}
}

func TestCheckValidationSchemaFileRulesPassesWhenFieldsResolve(t *testing.T) {
	ctx := context.Background()
	rules := []FileValidationRule{{
		FileName:  "extracted_data.json",
		MustExist: true,
		JSONChecks: []JSONValidationCheck{
			{Path: "$.status", MustExist: true, ValueType: "string"},
		},
	}}
	loadJSON := func(_ context.Context, fileName string) (interface{}, bool, error) {
		if fileName != "extracted_data.json" {
			return nil, false, nil
		}
		return map[string]interface{}{"status": "done"}, true, nil
	}

	check, err := CheckValidationSchemaFileRules(ctx, rules, loadJSON)
	if err != nil {
		t.Fatalf("CheckValidationSchemaFileRules returned error: %v", err)
	}
	if check.Status != "pass" {
		t.Fatalf("status = %q, want pass; evidence=%s", check.Status, check.Evidence)
	}
	if check.CheckID != validationSchemaFileDriftCheckID {
		t.Fatalf("check_id = %q, want %q", check.CheckID, validationSchemaFileDriftCheckID)
	}
}

func TestCheckValidationSchemaFileRulesFailsWhenFieldRemoved(t *testing.T) {
	ctx := context.Background()
	// The step's own output used to have "status"; a later edit renamed the
	// field to "outcome" without updating validation_schema.
	rules := []FileValidationRule{{
		FileName:  "extracted_data.json",
		MustExist: true,
		JSONChecks: []JSONValidationCheck{
			{Path: "$.status", MustExist: true, ValueType: "string"},
		},
	}}
	loadJSON := func(_ context.Context, _ string) (interface{}, bool, error) {
		return map[string]interface{}{"outcome": "done"}, true, nil
	}

	check, err := CheckValidationSchemaFileRules(ctx, rules, loadJSON)
	if err != nil {
		t.Fatalf("CheckValidationSchemaFileRules returned error: %v", err)
	}
	if check.Status != "fail" {
		t.Fatalf("status = %q, want fail (declared field no longer exists in real output)", check.Status)
	}
}

func TestCheckValidationSchemaFileRulesFailsWhenMustExistFileMissing(t *testing.T) {
	ctx := context.Background()
	rules := []FileValidationRule{{FileName: "results.json", MustExist: true}}
	loadJSON := func(_ context.Context, _ string) (interface{}, bool, error) {
		return nil, false, nil
	}

	check, err := CheckValidationSchemaFileRules(ctx, rules, loadJSON)
	if err != nil {
		t.Fatalf("CheckValidationSchemaFileRules returned error: %v", err)
	}
	if check.Status != "fail" {
		t.Fatalf("status = %q, want fail (must_exist file missing from recent output)", check.Status)
	}
}

func TestCheckValidationSchemaFileRulesPassesWhenNoRulesDeclared(t *testing.T) {
	ctx := context.Background()
	check, err := CheckValidationSchemaFileRules(ctx, nil, func(context.Context, string) (interface{}, bool, error) {
		t.Fatal("loadJSON should not be called when there are no file rules")
		return nil, false, nil
	})
	if err != nil {
		t.Fatalf("CheckValidationSchemaFileRules returned error: %v", err)
	}
	if check.Status != "pass" {
		t.Fatalf("status = %q, want pass (no rules to check)", check.Status)
	}
}
