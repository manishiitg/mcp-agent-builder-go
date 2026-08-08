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

const upgradeCurrentArtifactContract = `WORKFLOW CONTRACT UPGRADE: CURRENT ARTIFACT CONTRACT.

Pulse state is SQLite-backed and shown only in the Pulse popup. Do not create, read, update, publish, validate, or archive a separate Pulse HTML journal. Delete a retired builder/improve.html and its retired improve archive only after preserving no workflow-owned information from them: typed Pulse state, soul, plan/config, reports, knowledgebase, learnings, runs, and database remain authoritative.

Then inspect workflow.json, planning/plan.json, planning/step_config.json, learnings/_global/SKILL.md and referenced learning Markdown, plus knowledgebase notes. Remove only shared AgentWorks transport, MCP bridge, Folder Guard, managed-tool, tool-discovery, or native-session mechanics from workflow-authored prose. Preserve domain-specific inputs, outputs, side effects, safety boundaries, acceptance criteria, selectors, external API behavior, and recovery knowledge. Use the normal typed plan/config tools for plan changes. Re-read every changed artifact and validate the plan/config. If a rewrite is ambiguous, do not stamp. Otherwise call set_workflow_contract_version(version="1.0.21") and stop.`

// workflowVersionUpgradePlan keeps the retired HTML presentation migrations
// retired, but preserves the independent behavioral/data migrations older
// workflows still need. They are deliberately grouped into five bounded,
// blocking preflight turns rather than replaying the old 21-turn HTML chain.
func workflowVersionUpgradePlan(manifest *WorkflowManifest) []workflowVersionUpgrade {
	version := workflowContractVersionForUpgrade(manifest)
	rank, known := workflowContractVersionRank(version)
	if !known || version == WorkflowContractCurrentVersion {
		return nil
	}

	steps := make([]workflowVersionUpgrade, 0, 5)
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
	steps = append(steps, workflowVersionUpgrade{from: version, to: WorkflowContractCurrentVersion, label: "upgrade-current-artifact-contract", query: upgradeCurrentArtifactContract})
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
