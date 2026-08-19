package server

import (
	"strings"
	"testing"
)

func TestWorkflowVersionUpgradePlanFrom122MigratesReportsThenScheduledRoutes(t *testing.T) {
	plan := workflowVersionUpgradePlan(&WorkflowManifest{Version: "1.0.22"})
	if len(plan) != 3 {
		t.Fatalf("plan from 1.0.22 = %d steps, want direct-report, scheduled-route, then periodic-pulse-review migrations: %+v", len(plan), plan)
	}
	if plan[0].label != "upgrade-direct-html-reports" || plan[0].to != "1.0.23" {
		t.Fatalf("plan[0] = %+v, want direct-report migration to 1.0.23", plan[0])
	}
	if plan[1].label != "upgrade-schedule-execution-model" || plan[1].to != workflowContractScheduleExecutionModelVersion {
		t.Fatalf("plan[1] = %+v, want schedule execution-model migration to %s", plan[1], workflowContractScheduleExecutionModelVersion)
	}
	if plan[2].label != "upgrade-dedicated-pulse-schedule" || plan[2].to != WorkflowContractCurrentVersion {
		t.Fatalf("plan[2] = %+v, want periodic-pulse-review migration to current", plan[2])
	}
}

func TestUpgradeDirectHTMLReportsPreservesPrimaryDocuments(t *testing.T) {
	normalizedPrompt := strings.ToLower(strings.Join(strings.Fields(upgradeDirectHTMLReports), " "))
	for _, want := range []string{
		"reports/report_plan.json",
		"inventory every enabled HTML file widget",
		"db/report.html",
		"db/reports/index.html",
		"platform does not create report tabs or navigation",
		"Preserve the old primary dashboard",
		"including the primary dashboard",
		"validate_report_html",
		`set_workflow_contract_version(version="1.0.23")`,
	} {
		if !strings.Contains(normalizedPrompt, strings.ToLower(want)) {
			t.Errorf("direct-report upgrade prompt missing %q", want)
		}
	}
}
