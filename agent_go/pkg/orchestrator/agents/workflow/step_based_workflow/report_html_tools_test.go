package step_based_workflow

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

func TestValidateHTMLReportRequiresAStandaloneTopLevelPage(t *testing.T) {
	t.Parallel()
	agent := newWorkshopDefinitionDraft()
	const workspace = "Workflow/demo"
	files := map[string]string{
		"Workflow/demo/db/reports/daily.html": "<!doctype html><html><head><title>Daily briefing</title></head><body>OK</body></html>",
	}
	readFile := func(_ context.Context, path string) (string, error) {
		content, ok := files[path]
		if !ok {
			return "", fmt.Errorf("file not found: %s", path)
		}
		return content, nil
	}
	if err := registerHTMLReportTools(agent, workspace, workshopToolTestLogger{}, readFile); err != nil {
		t.Fatalf("registerHTMLReportTools: %v", err)
	}
	tool := agent.tools["validate_report_html"]
	if tool.Execute == nil {
		t.Fatal("validate_report_html was not registered")
	}
	result, err := tool.Execute(context.Background(), map[string]interface{}{"path": "db/reports/daily.html"})
	if err != nil {
		t.Fatalf("validate page: %v", err)
	}
	if !strings.Contains(result, `"valid": true`) || !strings.Contains(result, "Daily briefing") {
		t.Fatalf("unexpected validation result: %s", result)
	}
	if _, err := tool.Execute(context.Background(), map[string]interface{}{"path": "db/reports/archive/daily.html"}); err == nil {
		t.Fatal("nested report page path was accepted")
	}
}
