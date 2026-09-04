package step_based_workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	loggerv2 "github.com/manishiitg/mcpagent/logger/v2"
)

var (
	reportHTMLIDPattern                = regexp.MustCompile(`(?i)\bid\s*=\s*["']([^"']+)["']`)
	reportHTMLImmediateDOMWritePattern = regexp.MustCompile(`(?s)document\.getElementById\(\s*["']([^"']+)["']\s*\)\s*\.\s*(?:innerHTML|textContent|style|className|value)\b`)

	// window.report.query('...') / ("...") / (`...`). Only literal SQL is
	// checked; a query built from variables or a template with ${} is reported
	// as unchecked rather than guessed at.
	reportHTMLQueryCallPattern = regexp.MustCompile("(?s)window\\.report\\.query\\(\\s*(?:'((?:[^'\\\\]|\\\\.)*)'|\"((?:[^\"\\\\]|\\\\.)*)\"|`([^`]*)`)")
	// Any JS string literal that reads as a SQL statement. Reports commonly
	// wrap window.report.query in a local helper (`async function query(sql)`)
	// and pass literals to THAT, which the direct-call pattern never sees --
	// measured on a real report: 13 statements, 0 direct calls.
	reportHTMLSQLLiteralPattern = regexp.MustCompile("(?s)(?:'((?:[^'\\\\]|\\\\.)*)'|\"((?:[^\"\\\\]|\\\\.)*)\"|`([^`]*)`)")
	reportHTMLSQLStartPattern   = regexp.MustCompile(`(?i)^\s*(?:select|with|pragma)\b`)
	reportHTMLSQLBodyPattern    = regexp.MustCompile(`(?i)\b(?:from|pragma)\b|^\s*with\b.*\bselect\b`)
	// window.report.get/getText/getHtml/fileUrl/openFile('db/...') plus plain
	// src="db/..." / href="db/..." attributes -- every workspace path a report
	// resolves at runtime.
	reportHTMLPathCallPattern = regexp.MustCompile(`window\.report\.(?:get|getText|getHtml|fileUrl|openFile)\(\s*['"]([^'"]+)['"]`)
	reportHTMLPathAttrPattern = regexp.MustCompile(`(?i)\b(?:src|href)\s*=\s*['"]((?:db|knowledgebase|docs|planning|evaluation|costs|variables)/[^'"#?]+)['"]`)
	reportHTMLExternalAsset   = regexp.MustCompile(`(?i)<(?:link|script)\b[^>]*\b(?:href|src)\s*=\s*['"]https?://`)
)

func missingImmediateReportDOMTargets(content string) []string {
	ids := make(map[string]struct{})
	for _, match := range reportHTMLIDPattern.FindAllStringSubmatch(content, -1) {
		ids[match[1]] = struct{}{}
	}
	missing := make([]string, 0)
	seen := make(map[string]struct{})
	for _, match := range reportHTMLImmediateDOMWritePattern.FindAllStringSubmatch(content, -1) {
		id := match[1]
		if _, ok := ids[id]; ok {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		missing = append(missing, id)
	}
	return missing
}

func reportHTMLDecodeLiteral(match []string) (sqlText string, dynamic bool) {
	switch {
	case match[1] != "":
		sqlText = strings.ReplaceAll(match[1], `\'`, `'`)
	case match[2] != "":
		sqlText = strings.ReplaceAll(match[2], `\"`, `"`)
	case match[3] != "":
		if strings.Contains(match[3], "${") {
			return "", true
		}
		sqlText = match[3]
	}
	return strings.TrimSpace(sqlText), false
}

// reportHTMLLiteralQueries returns every SQL statement the report holds as a
// string literal -- passed straight to window.report.query or to a local
// wrapper around it -- deduplicated, plus how many direct calls used a
// non-literal (dynamic) query and so cannot be checked.
func reportHTMLLiteralQueries(content string) (literals []string, dynamic int) {
	seen := make(map[string]struct{})
	add := func(sqlText string) {
		if sqlText == "" {
			return
		}
		if _, ok := seen[sqlText]; ok {
			return
		}
		seen[sqlText] = struct{}{}
		literals = append(literals, sqlText)
	}
	direct := 0
	for _, match := range reportHTMLQueryCallPattern.FindAllStringSubmatch(content, -1) {
		sqlText, isDynamic := reportHTMLDecodeLiteral(match)
		if isDynamic {
			dynamic++
			continue
		}
		direct++
		add(sqlText)
	}
	for _, match := range reportHTMLSQLLiteralPattern.FindAllStringSubmatch(content, -1) {
		sqlText, isDynamic := reportHTMLDecodeLiteral(match)
		// "Select a market" is prose; a statement has a FROM (or is a PRAGMA /
		// a WITH that reaches a SELECT). Prose sent to EXPLAIN would fail and
		// wrongly mark the report invalid.
		if isDynamic || !reportHTMLSQLStartPattern.MatchString(sqlText) || !reportHTMLSQLBodyPattern.MatchString(sqlText) {
			continue
		}
		add(sqlText)
	}
	// A direct call whose argument isn't a string literal at all matches
	// neither pattern; count those so the result says how much was NOT checked.
	// (A wrapper call with a literal IS covered by the literal scan above.)
	total := strings.Count(content, "window.report.query(")
	if total > direct+dynamic {
		dynamic += total - direct - dynamic
	}
	return literals, dynamic
}

// reportHTMLReferencedPaths returns every workspace-relative path the report
// resolves at runtime, deduplicated and sorted.
func reportHTMLReferencedPaths(content string) []string {
	seen := make(map[string]struct{})
	add := func(path string) {
		path = strings.TrimSpace(path)
		if path == "" || strings.Contains(path, "${") {
			return
		}
		seen[path] = struct{}{}
	}
	for _, match := range reportHTMLPathCallPattern.FindAllStringSubmatch(content, -1) {
		add(match[1])
	}
	for _, match := range reportHTMLPathAttrPattern.FindAllStringSubmatch(content, -1) {
		add(match[1])
	}
	paths := make([]string, 0, len(seen))
	for path := range seen {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return paths
}

// ReportHTMLValidationHooks are the runtime-backed checks validate_report_html
// runs on top of its static parse. Either may be nil, in which case that check
// is skipped and the result says so -- the static checks never depend on them.
type ReportHTMLValidationHooks struct {
	// ExplainSQL runs `EXPLAIN <sql>` (or any equivalent read-only prepare)
	// against the workflow's db/db.sqlite and returns the database's own error
	// for a query that would fail at runtime.
	ExplainSQL func(ctx context.Context, sql string) error
	// FileExists reports whether a workspace-relative path (relative to the
	// workflow folder, e.g. "db/assets/logo.png") exists.
	FileExists func(ctx context.Context, relativePath string) (bool, error)
}

// registerHTMLReportTools exposes the deliberately small report contract. A
// workflow owns one complete HTML reporting experience at db/reports/index.html;
// there is no platform navigation or JSON layout/registration file.
func registerHTMLReportTools(
	mcpAgent DefinitionToolRegistrar,
	workspacePath string,
	logger loggerv2.Logger,
	readFile func(context.Context, string) (string, error),
	hooks ReportHTMLValidationHooks,
) error {
	schema := `{"type":"object","properties":{},"additionalProperties":false}`
	params, err := parseSchemaForToolParameters(schema)
	if err != nil {
		return fmt.Errorf("parse validate_report_html schema: %w", err)
	}

	mcpAgent.RegisterCustomTool(
		"validate_report_html",
		"Validate the workflow's complete db/reports/index.html reporting experience: document shape, scripted element ids, every literal window.report.query SQL against the live db/db.sqlite, every referenced db/ path, external asset URLs, and the in-app theme hooks. The HTML owns its tabs, sections, sidebar, or scrolling layout; the platform adds no report navigation and there is no report_plan.json.",
		params,
		func(ctx context.Context, args map[string]interface{}) (string, error) {
			_ = args
			const relativePath = "db/reports/index.html"
			content, err := readFile(ctx, filepath.ToSlash(filepath.Join(workspacePath, relativePath)))
			if err != nil {
				return "", fmt.Errorf("read %s: %w", relativePath, err)
			}
			lower := strings.ToLower(content)
			errors := make([]string, 0)
			warnings := make([]string, 0)
			if !strings.Contains(lower, "<html") {
				errors = append(errors, "missing <html> root")
			}
			if !strings.Contains(lower, "<body") {
				errors = append(errors, "missing <body>")
			}
			title := ""
			if start := strings.Index(lower, "<title"); start >= 0 {
				if openEnd := strings.Index(lower[start:], ">"); openEnd >= 0 {
					body := content[start+openEnd+1:]
					if end := strings.Index(strings.ToLower(body), "</title>"); end >= 0 {
						title = strings.TrimSpace(body[:end])
					}
				}
			}
			if title == "" {
				errors = append(errors, "missing non-empty <title> for the workflow report")
			}
			for _, id := range missingImmediateReportDOMTargets(content) {
				errors = append(errors, fmt.Sprintf("script writes to missing element id %q; update the script or restore that element", id))
			}

			// External assets never load: the Report tab renders the page from
			// srcdoc in a sandbox, and published copies must work offline.
			if reportHTMLExternalAsset.MatchString(content) {
				errors = append(errors, "external stylesheet/script URL found (link href / script src to https://...); the report must be self-contained -- inline the CSS/JS, no CDN")
			}

			// Theme: the Report tab mirrors the APP theme onto the document as
			// `.dark` + `data-theme`, not the OS scheme. A report that only keys
			// off prefers-color-scheme ignores the in-app toggle.
			usesOSScheme := strings.Contains(lower, "prefers-color-scheme")
			usesAppTheme := strings.Contains(lower, ".dark") || strings.Contains(lower, "data-theme") || strings.Contains(lower, "report:theme") || strings.Contains(lower, "var(--")
			if usesOSScheme && !usesAppTheme {
				warnings = append(warnings, "dark mode keys only off prefers-color-scheme (the OS), so it ignores the app's light/dark toggle; key off `:root.dark` / `[data-theme=\"dark\"]` or the injected hsl(var(--token)) palette instead")
			}

			// Live SQL: run each literal query through the database so a typo'd
			// table or column fails here, not silently in the Report tab.
			queries, dynamicQueries := reportHTMLLiteralQueries(content)
			sqlChecked := 0
			if hooks.ExplainSQL != nil {
				for _, sqlText := range queries {
					sqlChecked++
					if err := hooks.ExplainSQL(ctx, sqlText); err != nil {
						preview := sqlText
						if len(preview) > 160 {
							preview = preview[:160] + "…"
						}
						errors = append(errors, fmt.Sprintf("window.report.query SQL fails against db/db.sqlite: %v -- %s", err, preview))
					}
				}
			}

			// Referenced files: a path that is not under the workflow shows a
			// broken image and nothing else at runtime.
			referenced := reportHTMLReferencedPaths(content)
			pathsChecked := 0
			if hooks.FileExists != nil {
				for _, path := range referenced {
					pathsChecked++
					exists, err := hooks.FileExists(ctx, path)
					if err != nil {
						warnings = append(warnings, fmt.Sprintf("could not check referenced path %q: %v", path, err))
						continue
					}
					if !exists {
						errors = append(errors, fmt.Sprintf("referenced file %q does not exist in the workflow folder; publish it under db/ or fix the path", path))
					}
				}
			}

			result := map[string]interface{}{
				"valid":    len(errors) == 0,
				"path":     relativePath,
				"title":    title,
				"bytes":    len(content),
				"errors":   errors,
				"warnings": warnings,
				"checked": map[string]interface{}{
					"sql_literals":       sqlChecked,
					"sql_unchecked":      dynamicQueries + (len(queries) - sqlChecked),
					"referenced_paths":   pathsChecked,
					"paths_unchecked":    len(referenced) - pathsChecked,
					"sql_check_enabled":  hooks.ExplainSQL != nil,
					"path_check_enabled": hooks.FileExists != nil,
				},
				"next_step":     "Open the Report tab to verify layout and scrolling.",
				"page_contract": "db/reports/index.html owns the complete reporting experience and its internal navigation.",
			}
			out, marshalErr := json.MarshalIndent(result, "", "  ")
			if marshalErr != nil {
				return "", fmt.Errorf("marshal report validation: %w", marshalErr)
			}
			return string(out), nil
		},
		"workflow",
	)
	logger.Info("✅ Registered HTML report validation tool")
	return nil
}
