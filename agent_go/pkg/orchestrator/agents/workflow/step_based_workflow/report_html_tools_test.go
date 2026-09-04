package step_based_workflow

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func reportToolReadFile(files map[string]string) func(context.Context, string) (string, error) {
	return func(_ context.Context, path string) (string, error) {
		content, ok := files[path]
		if !ok {
			return "", fmt.Errorf("file not found: %s", path)
		}
		return content, nil
	}
}

func validateReport(t *testing.T, html string, hooks ReportHTMLValidationHooks) string {
	t.Helper()
	agent := newWorkshopDefinitionDraft()
	const workspace = "Workflow/demo"
	files := map[string]string{"Workflow/demo/db/reports/index.html": html}
	if err := registerHTMLReportTools(agent, workspace, workshopToolTestLogger{}, reportToolReadFile(files), hooks); err != nil {
		t.Fatalf("registerHTMLReportTools: %v", err)
	}
	tool := agent.tools["validate_report_html"]
	if tool.Execute == nil {
		t.Fatal("validate_report_html was not registered")
	}
	result, err := tool.Execute(context.Background(), map[string]interface{}{})
	if err != nil {
		t.Fatalf("validate page: %v", err)
	}
	return result
}

func TestValidateHTMLReportRequiresTheSingleWorkflowDocument(t *testing.T) {
	t.Parallel()
	result := validateReport(t, "<!doctype html><html><head><title>Daily briefing</title></head><body>OK</body></html>", ReportHTMLValidationHooks{})
	if !strings.Contains(result, `"valid": true`) || !strings.Contains(result, "Daily briefing") {
		t.Fatalf("unexpected validation result: %s", result)
	}
	if !strings.Contains(result, `"sql_check_enabled": false`) || !strings.Contains(result, `"path_check_enabled": false`) {
		t.Fatalf("expected the result to say the runtime checks were skipped without hooks: %s", result)
	}
}

func TestValidateHTMLReportRejectsImmediateWritesToMissingElements(t *testing.T) {
	t.Parallel()
	result := validateReport(t, `<!doctype html><html><head><title>Combined report</title></head><body><div id="lat-asof"></div><script>document.getElementById('asof').textContent = 'ready'</script></body></html>`, ReportHTMLValidationHooks{})
	if !strings.Contains(result, `"valid": false`) || !strings.Contains(result, `missing element id \"asof\"`) {
		t.Fatalf("unexpected validation result: %s", result)
	}
}

func TestValidateHTMLReportRunsLiteralQueriesAgainstTheDatabase(t *testing.T) {
	t.Parallel()
	html := "<!doctype html><html><head><title>Runs</title></head><body><script>" +
		"window.report.ready(async function(){" +
		"  var a = await window.report.query('SELECT id FROM runs ORDER BY id DESC LIMIT 5');" +
		"  var b = await window.report.query(\"SELECT count(*) AS n FROM run_summaries\");" +
		"  var c = await window.report.query(`SELECT * FROM leads WHERE status = 'new'`);" +
		"  var d = await window.report.query(`SELECT * FROM ${table}`);" +
		"  var e = await window.report.query(builtSql);" +
		"});</script></body></html>"
	seen := []string{}
	hooks := ReportHTMLValidationHooks{
		ExplainSQL: func(_ context.Context, sql string) error {
			seen = append(seen, sql)
			if strings.Contains(sql, "run_summaries") {
				return errors.New("no such table: run_summaries")
			}
			return nil
		},
	}
	result := validateReport(t, html, hooks)
	if len(seen) != 3 {
		t.Fatalf("expected the three literal queries to be explained, got %d: %v", len(seen), seen)
	}
	if !strings.Contains(result, `"valid": false`) || !strings.Contains(result, "no such table: run_summaries") {
		t.Fatalf("expected the failing query to be reported: %s", result)
	}
	if !strings.Contains(result, `"sql_literals": 3`) || !strings.Contains(result, `"sql_unchecked": 2`) {
		t.Fatalf("expected the result to count checked and unchecked queries: %s", result)
	}
}

func TestValidateHTMLReportChecksReferencedFilesAndExternalAssets(t *testing.T) {
	t.Parallel()
	html := `<!doctype html><html><head><title>Assets</title>
<link rel="stylesheet" href="https://cdn.example.com/x.css">
</head><body>
<img src="db/assets/logo.png"><a href="db/reports/proof.pdf">proof</a>
<script>window.report.ready(async()=>{ el.innerHTML = await window.report.getHtml('db/notes/summary.md'); window.report.openFile("db/assets/logo.png") })</script>
</body></html>`
	hooks := ReportHTMLValidationHooks{
		FileExists: func(_ context.Context, path string) (bool, error) {
			return path == "db/assets/logo.png", nil
		},
	}
	result := validateReport(t, html, hooks)
	for _, want := range []string{
		`"valid": false`,
		`referenced file \"db/notes/summary.md\" does not exist`,
		`referenced file \"db/reports/proof.pdf\" does not exist`,
		"external stylesheet/script URL found",
		`"referenced_paths": 3`,
	} {
		if !strings.Contains(result, want) {
			t.Fatalf("expected %q in result: %s", want, result)
		}
	}
	if strings.Contains(result, `\"db/assets/logo.png\" does not exist`) {
		t.Fatalf("existing asset must not be reported: %s", result)
	}
}

func TestValidateHTMLReportWarnsWhenDarkModeOnlyFollowsTheOS(t *testing.T) {
	t.Parallel()
	osOnly := `<!doctype html><html><head><title>Theme</title><style>@media (prefers-color-scheme: dark){body{background:#000}}</style></head><body>x</body></html>`
	result := validateReport(t, osOnly, ReportHTMLValidationHooks{})
	if !strings.Contains(result, `"valid": true`) || !strings.Contains(result, "ignores the app's light/dark toggle") {
		t.Fatalf("expected a theme warning without failing validation: %s", result)
	}
	appTheme := `<!doctype html><html><head><title>Theme</title><style>:root.dark body{background:#000}</style></head><body>x</body></html>`
	result = validateReport(t, appTheme, ReportHTMLValidationHooks{})
	if strings.Contains(result, "ignores the app's light/dark toggle") {
		t.Fatalf("a report keyed off .dark must not be warned: %s", result)
	}
}
