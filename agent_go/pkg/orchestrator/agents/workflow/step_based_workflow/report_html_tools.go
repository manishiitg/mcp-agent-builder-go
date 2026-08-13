package step_based_workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	loggerv2 "github.com/manishiitg/mcpagent/logger/v2"
)

var (
	reportHTMLIDPattern                = regexp.MustCompile(`(?i)\bid\s*=\s*["']([^"']+)["']`)
	reportHTMLImmediateDOMWritePattern = regexp.MustCompile(`(?s)document\.getElementById\(\s*["']([^"']+)["']\s*\)\s*\.\s*(?:innerHTML|textContent|style|className|value)\b`)
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

// registerHTMLReportTools exposes the deliberately small report contract. A
// workflow owns one complete HTML reporting experience at db/reports/index.html;
// there is no platform navigation or JSON layout/registration file.
func registerHTMLReportTools(
	mcpAgent DefinitionToolRegistrar,
	workspacePath string,
	logger loggerv2.Logger,
	readFile func(context.Context, string) (string, error),
) error {
	schema := `{"type":"object","properties":{},"additionalProperties":false}`
	params, err := parseSchemaForToolParameters(schema)
	if err != nil {
		return fmt.Errorf("parse validate_report_html schema: %w", err)
	}

	mcpAgent.RegisterCustomTool(
		"validate_report_html",
		"Validate the workflow's complete db/reports/index.html reporting experience. The HTML owns its tabs, sections, sidebar, or scrolling layout; the platform adds no report navigation and there is no report_plan.json.",
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
			result := map[string]interface{}{
				"valid":         len(errors) == 0,
				"path":          relativePath,
				"title":         title,
				"bytes":         len(content),
				"errors":        errors,
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
