package server

import (
	"fmt"
	"strings"
)

type workflowVersionUpgrade struct {
	from  string
	to    string
	label string
	query string
}

func workflowContractVersionForUpgrade(manifest *WorkflowManifest) string {
	if manifest == nil {
		return workflowContractInitialVersion
	}
	version := strings.TrimSpace(manifest.Version)
	if version == "" {
		return workflowContractInitialVersion
	}
	return version
}

// workflowContractVersionRank is intentionally a closed set. A workflow made
// by a newer server must never be silently "upgraded" backwards by an older
// server: scheduledWorkshopTurns treats an unknown version as having no path.
func workflowContractVersionRank(version string) (int, bool) {
	known := []string{
		"1.0.0", "1.0.1", "1.0.2", "1.0.3", "1.0.4", "1.0.5", "1.0.6", "1.0.7", "1.0.8", "1.0.9",
		workflowContractMessageSequenceCodeVersion,
		workflowContractPulseHistoryVersion,
		workflowContractNotificationConfigVersion,
		workflowContractHumanInputOwnershipVersion,
		workflowContractHumanReadablePulseStateVersion,
		workflowContractKBWriteMethodRetiredVersion,
		workflowContractEvalVerdictSchemaVersion,
		workflowContractCompactPulseReportVersion,
		workflowContractLightweightPulseReportVersion,
		workflowContractExecutivePulseJournalVersion,
		workflowContractArtifactPurityVersion,
		workflowContractLearningsLockAuditVersion,
		workflowContractDirectHTMLReportsVersion,
	}
	for rank, candidate := range known {
		if version == candidate {
			return rank, true
		}
	}
	return 0, false
}

const upgradeMessageSequenceCode = `WORKFLOW CONTRACT UPGRADE: MESSAGE-SEQUENCE CODE.

This workflow predates the current message-sequence contract. Do only this migration.
Call migrate_message_sequence_code_items. It is the trusted tool that either converts unambiguous legacy code items into standalone scripted steps or reports a precise blocker. Then inspect the resulting plan and confirm no legacy message-sequence code item remains. Do not run the workflow. If the migration is blocked or validation fails, do not stamp a version. Otherwise call set_workflow_contract_version(version="1.0.10") and stop.`

const upgradeNotificationConfig = `WORKFLOW CONTRACT UPGRADE: NOTIFICATION CONFIGURATION.

This workflow predates workflow-scoped notification configuration. Do only this migration. Read workflow.json and soul/soul.md. If soul contains an explicit user-approved Notifications preference, move only the mappable workflow-specific preference into workflow.json capabilities.notifications using update_workflow_config, then remove that duplicate preference from soul. Preserve account-wide delivery settings, unknown text, recipients, and constraints rather than guessing. If there is no legacy Notifications section, this is a no-op. Do not run the workflow. After re-reading the changed artifacts and confirming no notification meaning was lost, call set_workflow_contract_version(version="1.0.12") and stop.`

const upgradeKBWriteMethod = `WORKFLOW CONTRACT UPGRADE: RETIRED KNOWLEDGEBASE WRITE METHOD.

This workflow predates inline knowledgebase writes. Do only this migration. Inspect planning/step_config.json and remove the retired knowledgebase_write_method and learnings_write_method keys from every step configuration while preserving every other field. A KB-writing step still needs knowledgebase_access write/read-write plus a non-empty knowledgebase_contribution; do not invent contributions or change its access. Re-read and validate the complete config after the rewrite. Do not run the workflow. If any ambiguous legacy configuration cannot be safely preserved, report it and do not stamp. Otherwise call set_workflow_contract_version(version="1.0.15") and stop.`

const upgradeEvalVerdictSchema = `WORKFLOW CONTRACT UPGRADE: EVALUATION VERDICTS.

This workflow predates the current evaluation output contract. Do only this migration. If evaluation/evaluation_plan.json is absent, this is a no-op. Otherwise load the evaluation-plan reference, inspect every evaluation step, and make each step emit numeric output_content.score on the current 0-10 scale without changing what it measures. Update validation schemas to require the emitted score and use validate_evaluation_plan. Do not run a normal workflow execution. If a representative evaluation can safely be run, use it to verify changed steps produce scores; otherwise record the exact evidence boundary. Do not stamp on a validation failure. When the plan is compliant, call set_workflow_contract_version(version="1.0.16") and stop.`

const upgradeLearningsLockAudit = `WORKFLOW CONTRACT UPGRADE: LEARNINGS LOCK AUDIT (PLAT-055 / J).

This is a read-mostly audit, not a rewrite. Note first: the routing rule that used
to have to live inside each step's learning_objective (which store owns what,
which file to write, the size budget) is now injected by the platform itself into
every reflection turn — see reflection_turn.go. Do not add or judge routing
language in objectives; that check is obsolete as of this contract version.

What remains genuinely worth auditing: inspect planning/step_config.json for every
step with lock_learnings=true. A lock freezes that step's contribution to
learnings/_global/ — existing content still flows into every step's prompt, but
this step never updates it. That is sometimes exactly right (a step whose
learnings are stable and reviewed) and sometimes just inertia nobody explained.

For each locked step, read its review_notes. If review_notes gives a concrete,
learnings-specific reason the freeze is intentional, leave it alone — a
documented lock must survive this audit untouched. If review_notes is empty, or
present but does not mention learnings/lock/why the freeze is there (e.g. it only
explains a KB or tool decision), call record_pulse_finding with issue_kind=
"workflow_issue", classification="maintainability_bug", severity="low",
target_key naming the step id, a summary stating the step is locked without a
learnings-specific rationale, evidence citing the step's own review_notes text (or
its absence), and recommended_route="decision_required" — unlocking or
documenting the lock is the workflow owner's call, not something this turn makes
for them. Do not call update_step_config to clear lock_learnings on any step. Do
not invent a rationale on the owner's behalf.

Do not run the workflow. When every locked step has been checked, call
set_workflow_contract_version(version="1.0.22") and stop.`

const upgradeCurrentArtifactContract = `WORKFLOW CONTRACT UPGRADE: CURRENT ARTIFACT CONTRACT.

Pulse state is SQLite-backed and shown only in the Pulse popup. Do not create, read, update, publish, validate, or archive a separate Pulse HTML journal. Delete a retired builder/improve.html and its retired improve archive only after preserving no workflow-owned information from them: typed Pulse state, soul, plan/config, reports, knowledgebase, learnings, runs, and database remain authoritative.

Then inspect workflow.json, planning/plan.json, planning/step_config.json, learnings/_global/SKILL.md and referenced learning Markdown, plus knowledgebase notes. Remove only shared AgentWorks transport, MCP bridge, Folder Guard, managed-tool, tool-discovery, or native-session mechanics from workflow-authored prose. Preserve domain-specific inputs, outputs, side effects, safety boundaries, acceptance criteria, selectors, external API behavior, and recovery knowledge. Use the normal typed plan/config tools for plan changes. Re-read every changed artifact and validate the plan/config. If a rewrite is ambiguous, do not stamp. Otherwise call set_workflow_contract_version(version="1.0.21") and stop.`

const upgradeDirectHTMLReports = `WORKFLOW CONTRACT UPGRADE: DIRECT HTML REPORT PAGES.

This workflow predates the direct HTML report-page contract. Migrate its report
documents agentically; do not replace them with generic placeholders, wrappers,
iframes, or an empty navigation shell.

First inspect reports/report_plan.json when it exists. In its existing section and
entry order, inventory every enabled HTML file widget and read every complete source
document, including old sources outside the report folder such as db/report.html.
Also inventory immediate db/reports/*.html files so useful report content
that was not registered in the old plan is preserved.

Compose one complete standalone report at db/reports/index.html. The platform does
not create report tabs or navigation: this HTML owns the entire user experience and
may choose tabs, a sidebar, anchored sections, expandable panels, or one scrolling
briefing according to the workflow's reporting needs. Preserve the old primary
dashboard, SQL, window.report calls, scripts, styles, media references, and all
useful user-visible content. Consolidate secondary documents as coherent sections
or views inside index.html rather than exposing their filenames as platform pages.
Historical setup/choice artifacts should be linked or placed in a clearly secondary
view, not presented as peers of the operational dashboard. Ensure index.html has a
meaningful <title>. Fix relative asset references only when consolidation changes
their resolution. Do not change what the report measures during this migration.

Validate db/reports/index.html with validate_report_html. Re-read it and confirm
every enabled old-plan document is represented, including the primary dashboard,
and that its internal navigation exposes every intended reporting section. Only
after that verification, delete the retired reports/report_plan.json and superseded
standalone report HTML files; retain their media assets. Do not delete any old source
until index.html has passed validation. Do not run the workflow. If a source is
missing or the consolidation is ambiguous, report the blocker and do not stamp.
Otherwise call set_workflow_contract_version(version="1.0.23") and stop.`

// workflowVersionUpgradePlan keeps the retired HTML presentation migrations
// retired, but preserves the independent behavioral/data migrations older
// workflows still need. They are deliberately grouped into bounded,
// blocking preflight turns rather than replaying the old 21-turn HTML chain.
func workflowVersionUpgradePlan(manifest *WorkflowManifest) []workflowVersionUpgrade {
	version := workflowContractVersionForUpgrade(manifest)
	rank, known := workflowContractVersionRank(version)
	if !known || version == WorkflowContractCurrentVersion {
		return nil
	}

	steps := make([]workflowVersionUpgrade, 0, 7)
	if rank < 10 {
		steps = append(steps, workflowVersionUpgrade{from: version, to: workflowContractMessageSequenceCodeVersion, label: "upgrade-message-sequence-code", query: upgradeMessageSequenceCode})
	}
	if rank < 12 {
		steps = append(steps, workflowVersionUpgrade{from: version, to: workflowContractNotificationConfigVersion, label: "upgrade-notification-config", query: upgradeNotificationConfig})
	}
	if rank < 15 {
		steps = append(steps, workflowVersionUpgrade{from: version, to: workflowContractKBWriteMethodRetiredVersion, label: "upgrade-kb-write-method", query: upgradeKBWriteMethod})
	}
	if rank < 16 {
		steps = append(steps, workflowVersionUpgrade{from: version, to: workflowContractEvalVerdictSchemaVersion, label: "upgrade-eval-verdict-schema", query: upgradeEvalVerdictSchema})
	}
	// workflowContractArtifactPurityVersion ("1.0.21") sits at rank 20 in the
	// known-version list above — a workflow whose rank is already >= 20 passed
	// this step in an earlier preflight and must not repeat it.
	if rank < 20 {
		steps = append(steps, workflowVersionUpgrade{from: version, to: workflowContractArtifactPurityVersion, label: "upgrade-current-artifact-contract", query: upgradeCurrentArtifactContract})
	}
	if rank < 21 {
		steps = append(steps, workflowVersionUpgrade{from: version, to: workflowContractLearningsLockAuditVersion, label: "upgrade-learnings-lock-audit", query: upgradeLearningsLockAudit})
	}
	steps = append(steps, workflowVersionUpgrade{from: version, to: WorkflowContractCurrentVersion, label: "upgrade-direct-html-reports", query: upgradeDirectHTMLReports})
	return steps
}

func postRunMonitorStepsForManifest(manifest *WorkflowManifest) []postRunMonitorStep {
	steps := postRunMonitorUpgradeStepsForManifest(manifest)
	return append(steps, postRunMonitorSteps()...)
}

func postRunMonitorUpgradeStepsForManifest(manifest *WorkflowManifest) []postRunMonitorStep {
	upgrades := workflowVersionUpgradePlan(manifest)
	if len(upgrades) == 0 {
		return nil
	}

	steps := make([]postRunMonitorStep, 0, len(upgrades))
	for _, upgrade := range upgrades {
		steps = append(steps, postRunMonitorStep{
			label: upgrade.label,
			query: fmt.Sprintf("%s\n\nCurrent workflow.json version seen by scheduler: %q. Target workflow contract version: %q.", upgrade.query, workflowContractVersionForUpgrade(manifest), upgrade.to),
		})
	}
	return steps
}
