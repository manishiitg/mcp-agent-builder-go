package step_based_workflow

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

func TestValidateHTMLReportRequiresTheSingleWorkflowDocument(t *testing.T) {
	t.Parallel()
	agent := newWorkshopDefinitionDraft()
	const workspace = "Workflow/demo"
	files := map[string]string{
		"Workflow/demo/db/reports/index.html": "<!doctype html><html><head><title>Daily briefing</title></head><body>OK</body></html>",
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
	result, err := tool.Execute(context.Background(), map[string]interface{}{})
	if err != nil {
		t.Fatalf("validate page: %v", err)
	}
	if !strings.Contains(result, `"valid": true`) || !strings.Contains(result, "Daily briefing") {
		t.Fatalf("unexpected validation result: %s", result)
	}
}

func TestValidateHTMLReportRejectsImmediateWritesToMissingElements(t *testing.T) {
	t.Parallel()
	agent := newWorkshopDefinitionDraft()
	const workspace = "Workflow/demo"
	files := map[string]string{
		"Workflow/demo/db/reports/index.html": `<!doctype html><html><head><title>Combined report</title></head><body><div id="lat-asof"></div><script>document.getElementById('asof').textContent = 'ready'</script></body></html>`,
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
	result, err := agent.tools["validate_report_html"].Execute(context.Background(), map[string]interface{}{})
	if err != nil {
		t.Fatalf("validate page: %v", err)
	}
	if !strings.Contains(result, `"valid": false`) || !strings.Contains(result, `missing element id \"asof\"`) {
		t.Fatalf("unexpected validation result: %s", result)
	}
}
