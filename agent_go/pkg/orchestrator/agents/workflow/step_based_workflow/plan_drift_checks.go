package step_based_workflow

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/fsutil"

	_ "modernc.org/sqlite"
)

// PLAT-258 phase 2. Deterministic plan-drift checks: pure Go, no LLM turn, no
// agent tool call to skip — the same design constraint run_concerns.go already
// documents ("there is no call for an agent to skip"). Each check reads the
// workflow's own current db/reports/index.html and db/db.sqlite directly and
// returns one evidence-required StepDriftCheck. Judgment checks (description
// accuracy, learnings/KB content staleness, DB normalization) are NOT here —
// those need an actual Pulse reviewer turn and are phase 4.

const reportDriftCheckID = "report_query_compatibility"

func planDriftWorkflowDBPath(workspacePath string) string {
	return filepath.Join(fsutil.WorkspaceDocsRoot(), filepath.FromSlash(strings.Trim(strings.TrimSpace(workspacePath), "/")), "db", "db.sqlite")
}

// openPlanDriftQueryOnlyDB opens the workflow's db.sqlite read-only (WAL-capable,
// query_only pragma) — a drift check must never be able to mutate real workflow
// data, no matter what SQL a report or a step's own code embeds.
func openPlanDriftQueryOnlyDB(dbPath string) (*sql.DB, error) {
	if _, err := os.Stat(dbPath); err != nil {
		return nil, err
	}
	dsn := (&url.URL{Scheme: "file", Path: dbPath}).String() + "?mode=rw&_pragma=query_only(true)&_pragma=busy_timeout(5000)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	return db, nil
}

// reportQueryPatterns match window.report.query("..."), ('...'), and (`...`)
// separately — one pattern per quote character rather than a single pattern
// with a backreference, since Go's RE2 engine (regexp) does not support
// backreferences at all. Each captures the SQL text between matching
// unescaped quotes of its own kind. Deliberately regexes, not a JS parser —
// report HTML is simple, agent-authored, and this only needs to find literal
// query() call arguments, not evaluate arbitrary expressions.
var reportQueryPatterns = []*regexp.Regexp{
	regexp.MustCompile(`window\.report\.query\(\s*'((?:\\.|[^'\\])*)'`),
	regexp.MustCompile(`window\.report\.query\(\s*"((?:\\.|[^"\\])*)"`),
	regexp.MustCompile("window\\.report\\.query\\(\\s*`((?:\\\\.|[^`\\\\])*)`"),
}

// extractReportQueries pulls every window.report.query("...") SQL string out of
// a report's raw HTML/JS text, deduplicated and in first-seen (source-order)
// position. Queries built by string concatenation or built at runtime (not a
// literal argument) are not extractable this way — a known limitation, not a
// false negative on what IS extractable.
func extractReportQueries(html string) []string {
	type found struct {
		pos int
		sql string
	}
	var all []found
	for _, pattern := range reportQueryPatterns {
		for _, m := range pattern.FindAllStringSubmatchIndex(html, -1) {
			if len(m) < 4 {
				continue
			}
			sqlText := strings.TrimSpace(unescapeJSStringLiteral(html[m[2]:m[3]]))
			if sqlText == "" {
				continue
			}
			all = append(all, found{pos: m[0], sql: sqlText})
		}
	}
	sort.Slice(all, func(i, j int) bool { return all[i].pos < all[j].pos })
	seen := make(map[string]bool, len(all))
	queries := make([]string, 0, len(all))
	for _, f := range all {
		if seen[f.sql] {
			continue
		}
		seen[f.sql] = true
		queries = append(queries, f.sql)
	}
	return queries
}

// unescapeJSStringLiteral handles the handful of escapes that actually show up
// in a hand-authored SQL string embedded in JS source — not a full JS string
// grammar, just enough that a query containing an escaped quote or a newline
// still dry-runs as the same SQL the browser would actually send.
func unescapeJSStringLiteral(s string) string {
	replacer := strings.NewReplacer(
		`\'`, `'`, `\"`, `"`, "\\`", "`",
		`\n`, "\n", `\t`, "\t", `\\`, `\`,
	)
	return replacer.Replace(s)
}

// CheckReportQueryCompatibility dry-runs every window.report.query(...) call
// found in db/reports/index.html against the workflow's live db.sqlite. A step
// changing what it writes (renaming/dropping a column or table) breaks the
// report silently in the UI; this catches it mechanically, no LLM needed.
func CheckReportQueryCompatibility(ctx context.Context, workspacePath string, readFile func(context.Context, string) (string, error)) (StepDriftCheck, error) {
	check := StepDriftCheck{CheckID: reportDriftCheckID}

	reportPath := normalizePathForWorkspaceAPI(filepath.Join("db", "reports", "index.html"), workspacePath)
	html, err := readFile(ctx, reportPath)
	if err != nil || strings.TrimSpace(html) == "" {
		check.Status = "pass"
		check.Evidence = "no db/reports/index.html found for this workflow; nothing to check"
		return check, nil
	}

	queries := extractReportQueries(html)
	if len(queries) == 0 {
		check.Status = "pass"
		check.Evidence = "db/reports/index.html has no window.report.query(...) calls; nothing to check"
		return check, nil
	}

	dbPath := planDriftWorkflowDBPath(workspacePath)
	db, err := openPlanDriftQueryOnlyDB(dbPath)
	if err != nil {
		check.Status = "fail"
		check.Evidence = fmt.Sprintf("report declares %d query(...) call(s) but db/db.sqlite could not be opened: %v", len(queries), err)
		return check, nil
	}
	defer db.Close()

	var failures []string
	for _, q := range queries {
		if _, err := db.QueryContext(ctx, q); err != nil {
			preview := q
			if len(preview) > 120 {
				preview = preview[:120] + "..."
			}
			failures = append(failures, fmt.Sprintf("%q -> %v", preview, err))
		}
	}

	if len(failures) == 0 {
		check.Status = "pass"
		check.Evidence = fmt.Sprintf("all %d report query(...) call(s) ran cleanly against the current db.sqlite schema", len(queries))
		return check, nil
	}
	check.Status = "fail"
	check.Evidence = fmt.Sprintf("%d of %d report query(...) call(s) failed against the current schema: %s", len(failures), len(queries), strings.Join(failures, "; "))
	return check, nil
}

const validationSchemaDBDriftCheckID = "validation_schema_db_rules"

// scanPlanDriftRows converts *sql.Rows into the same []map[string]interface{}
// shape evaluateDBRule (pre_validation_db.go) expects, so drift checks reuse
// the real, already-tested validation semantics instead of a second
// implementation of "what does a row check mean."
func scanPlanDriftRows(rows *sql.Rows) ([]map[string]interface{}, error) {
	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	var out []map[string]interface{}
	for rows.Next() {
		vals := make([]interface{}, len(cols))
		ptrs := make([]interface{}, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
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
	return out, rows.Err()
}

// CheckValidationSchemaDBRules dry-runs every ValidationSchema.DB[] rule's SQL
// against the workflow's live db.sqlite (query_only) and applies its MinRows/
// MaxRows/Checks assertions via the real evaluateDBRule — the exact function
// normal pre-validation uses — so a step's own declared DB validation is
// re-checked against CURRENT data, not just at the moment it last ran. A rule
// that used to pass and no longer does means the step's actual output shape
// (or the DB itself) drifted out from under its own validation contract.
func CheckValidationSchemaDBRules(ctx context.Context, workspacePath string, dbRules []DBValidationRule) (StepDriftCheck, error) {
	check := StepDriftCheck{CheckID: validationSchemaDBDriftCheckID}
	if len(dbRules) == 0 {
		check.Status = "pass"
		check.Evidence = "this step's validation_schema declares no db[] rules; nothing to check"
		return check, nil
	}

	dbPath := planDriftWorkflowDBPath(workspacePath)
	db, err := openPlanDriftQueryOnlyDB(dbPath)
	if err != nil {
		check.Status = "fail"
		check.Evidence = fmt.Sprintf("validation_schema declares %d db[] rule(s) but db/db.sqlite could not be opened: %v", len(dbRules), err)
		return check, nil
	}
	defer db.Close()

	var failures []string
	for _, rule := range dbRules {
		label := strings.TrimSpace(rule.Name)
		if label == "" {
			label = truncateForLabel(rule.SQL, 60)
		}
		if strings.TrimSpace(rule.SQL) == "" {
			failures = append(failures, fmt.Sprintf("rule %q has no sql", label))
			continue
		}
		sqlRows, err := db.QueryContext(ctx, rule.SQL)
		if err != nil {
			failures = append(failures, fmt.Sprintf("rule %q: query failed: %v", label, err))
			continue
		}
		rows, err := scanPlanDriftRows(sqlRows)
		sqlRows.Close()
		if err != nil {
			failures = append(failures, fmt.Sprintf("rule %q: failed to read rows: %v", label, err))
			continue
		}
		for _, result := range evaluateDBRule(ctx, rule, rows) {
			if !result.Passed && !result.SchemaError {
				failures = append(failures, fmt.Sprintf("rule %q, check %s: expected %v, got %v (%s)", label, result.Path, result.Expected, result.Actual, result.ErrorMsg))
			}
		}
	}

	if len(failures) == 0 {
		check.Status = "pass"
		check.Evidence = fmt.Sprintf("all %d validation_schema db[] rule(s) still pass against the current db.sqlite", len(dbRules))
		return check, nil
	}
	check.Status = "fail"
	check.Evidence = fmt.Sprintf("%d db[] rule assertion(s) failed against current data: %s", len(failures), strings.Join(failures, "; "))
	return check, nil
}

const validationSchemaFileDriftCheckID = "validation_schema_file_rules"

// CheckValidationSchemaFileRules re-checks a step's ValidationSchema.Files[]
// json_checks against the step's own MOST RECENT real output, using the real
// validateJSONCheck evaluator (pre_validation.go) — the same function normal
// pre-validation calls — so semantics never drift between what a live run
// checks and what this drift check checks. loadJSON resolves a declared
// FileName to its current parsed content; callers (the phase-3 orchestration
// layer) own finding the right run folder, since that resolution has nothing
// to do with what a drift check itself asserts. A field that used to resolve
// and no longer does, or now fails value_type/pattern/etc., means the step's
// actual output shape changed without its validation_schema catching up.
func CheckValidationSchemaFileRules(ctx context.Context, fileRules []FileValidationRule, loadJSON func(ctx context.Context, fileName string) (data interface{}, exists bool, err error)) (StepDriftCheck, error) {
	check := StepDriftCheck{CheckID: validationSchemaFileDriftCheckID}
	if len(fileRules) == 0 {
		check.Status = "pass"
		check.Evidence = "this step's validation_schema declares no files[] rules; nothing to check"
		return check, nil
	}

	var failures []string
	checked := 0
	for _, rule := range fileRules {
		data, exists, err := loadJSON(ctx, rule.FileName)
		if err != nil {
			failures = append(failures, fmt.Sprintf("file %q: failed to load current output: %v", rule.FileName, err))
			continue
		}
		if !exists {
			if rule.MustExist {
				failures = append(failures, fmt.Sprintf("file %q must exist but no recent run produced it", rule.FileName))
			}
			continue
		}
		for _, jsonCheck := range rule.JSONChecks {
			checked++
			result := validateJSONCheck(ctx, jsonCheck, data)
			if !result.Passed && !result.SchemaError {
				failures = append(failures, fmt.Sprintf("file %q, path %s: expected %v, got %v (%s)", rule.FileName, result.Path, result.Expected, result.Actual, result.ErrorMsg))
			}
		}
	}

	if len(failures) == 0 {
		check.Status = "pass"
		check.Evidence = fmt.Sprintf("all %d validation_schema files[] json_checks assertion(s) still pass against the most recent output", checked)
		return check, nil
	}
	check.Status = "fail"
	check.Evidence = fmt.Sprintf("%d files[] assertion(s) failed against the most recent output: %s", len(failures), strings.Join(failures, "; "))
	return check, nil
}
