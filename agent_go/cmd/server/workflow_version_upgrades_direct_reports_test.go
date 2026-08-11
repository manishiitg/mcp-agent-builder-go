package server

import (
	"strings"
	"testing"
)

func TestWorkflowVersionUpgradePlanFrom122MigratesReportsThenScheduledRoutes(t *testing.T) {
	plan := workflowVersionUpgradePlan(&WorkflowManifest{Version: "1.0.22"})
	if len(plan) != 2 {
		t.Fatalf("plan from 1.0.22 = %d steps, want direct-report then scheduled-route migrations: %+v", len(plan), plan)
	}
	if plan[0].label != "upgrade-direct-html-reports" || plan[0].to != "1.0.23" {
		t.Fatalf("plan[0] = %+v, want direct-report migration to 1.0.23", plan[0])
	}
	if plan[1].label != "upgrade-schedule-execution-model" || plan[1].to != WorkflowContractCurrentVersion {
		t.Fatalf("plan[1] = %+v, want schedule execution-model migration to current", plan[1])
	}
}

func TestUpgradeDirectHTMLReportsPreservesPrimaryDocuments(t *testing.T) {
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
		if !strings.Contains(upgradeDirectHTMLReports, want) {
			t.Errorf("direct-report upgrade prompt missing %q", want)
		}
	}
}
