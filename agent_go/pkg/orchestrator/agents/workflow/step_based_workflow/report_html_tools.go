package step_based_workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	loggerv2 "github.com/manishiitg/mcpagent/logger/v2"
)

// registerHTMLReportTools exposes the deliberately small report contract. A
// report is one or more standalone HTML pages beneath db/reports/, discovered
// by the frontend directly; there is no JSON layout/registration file.
func registerHTMLReportTools(
	mcpAgent DefinitionToolRegistrar,
	workspacePath string,
	logger loggerv2.Logger,
	readFile func(context.Context, string) (string, error),
) error {
	schema := `{
  "type":"object",
  "properties":{"path":{"type":"string","description":"HTML page path relative to the workflow, e.g. db/reports/daily.html."}},
  "required":["path"],
  "additionalProperties":false
}`
	params, err := parseSchemaForToolParameters(schema)
	if err != nil {
		return fmt.Errorf("parse validate_report_html schema: %w", err)
	}

	mcpAgent.RegisterCustomTool(
		"validate_report_html",
		"Validate one standalone HTML report page under db/reports/. Reports are discovered directly from that folder; there is no report_plan.json. Pass the relative path after writing it. The result reports its title, optional report-order metadata, byte count, and concrete authoring errors.",
		params,
		func(ctx context.Context, args map[string]interface{}) (string, error) {
			relativePath, _ := args["path"].(string)
			relativePath = strings.TrimSpace(strings.ReplaceAll(relativePath, "\\", "/"))
			pageName := strings.TrimPrefix(relativePath, "db/reports/")
			if strings.HasPrefix(relativePath, "/") || strings.Contains(relativePath, "../") || relativePath == ".." || !strings.HasPrefix(relativePath, "db/reports/") || strings.Contains(pageName, "/") || pageName == "" || !strings.HasSuffix(strings.ToLower(relativePath), ".html") {
				return "", fmt.Errorf("path must be a relative db/reports/*.html file, got %q", relativePath)
			}
			content, err := readFile(ctx, filepath.ToSlash(filepath.Join(workspacePath, relativePath)))
			if err != nil {
				return "", fmt.Errorf("read %s: %w", relativePath, err)
			}
			lower := strings.ToLower(content)
			errors := make([]string, 0)
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
				errors = append(errors, "missing non-empty <title>; it is the page label in the report navigation")
			}
			result := map[string]interface{}{
				"valid":         len(errors) == 0,
				"path":          relativePath,
				"title":         title,
				"bytes":         len(content),
				"errors":        errors,
				"next_step":     "Open the Report tab to verify layout and scrolling.",
				"page_contract": "One db/reports/*.html file is one top-level report page. Add <meta name=\"report-order\" content=\"10\"> only when alphabetical filename order is not the desired navigation order.",
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
