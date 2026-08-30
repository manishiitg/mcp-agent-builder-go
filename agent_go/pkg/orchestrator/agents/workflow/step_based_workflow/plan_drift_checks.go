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
