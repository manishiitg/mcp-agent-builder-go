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
		workflowContractScheduledRouteVersion,
		workflowContractScheduleExecutionModelVersion,
		workflowContractPeriodicPulseReviewVersion,
		workflowContractDedicatedPulseScheduleVersion,
		workflowContractSchedulePromptContractVersion,
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

const upgradeLearningsLockAudit = `WORKFLOW CONTRACT UPGRADE: LEARNINGS LOCK AUDIT.

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

Pulse state is SQLite-backed and shown only in the Pulse popup. Do not create, read, update, publish, validate, or archive a separate Pulse HTML journal. builder/improve.html and builder/improve-archive/ are retired presentation surfaces, not a store of record. Typed Pulse state, soul, plan/config, reports, knowledgebase, learnings, runs, and database remain authoritative.

NOTHING IS DELETED IN THIS MIGRATION. Move builder/improve.html and builder/improve-archive/ into migration-backups/artifact-purity-<UTC timestamp>/, keeping their relative paths. Both files stay on disk, and in git history, at the new location. Read them back at the new path and confirm builder/ no longer holds them before you stamp; if the move cannot preserve a file, that IS a blocker — report it and do not stamp.

A previous turn on this workflow may have declined to advance this version. Read its recorded reason before deciding whether it still applies. The objection on record is to stamping the version while old finding history would be DESTROYED. A relocation does not destroy it: history that exists only in these files and not in the pulse_* tables survives this migration unchanged, at the new path. So do not migrate that history into SQLite, and do not treat its absence from SQLite as a reason to stop — verify the files are intact where you moved them, which is the thing that objection was actually protecting.

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

const upgradeDedicatedPulseSchedule = `WORKFLOW CONTRACT UPGRADE: DEDICATED PULSE REVIEW SCHEDULE.

Recurring Pulse is represented only by an enabled pulse_review_only schedule;
that schedule is the single source of truth for enablement and cadence.
Normal workflow schedules never run Gate/Review+Fix inline. Legacy
post_run_monitor and post_run_monitor_mode fields are obsolete and ignored;
remove them if present. If recurring Pulse was enabled previously and no
pulse_review_only schedule exists, create one at a cadence justified by the
enabled run schedules and retained run history. Do not add group_names,
route_selections, or messages to that review schedule.

Read the raw workflow.json before changing anything so a legacy
post_run_monitor=true can be distinguished from an absent/false field. If it
was true and no dedicated schedule exists, create the schedule. If it was
false/absent and no dedicated schedule exists, do not enable Pulse. Calling
set_workflow_contract_version rewrites the manifest through the current schema
and removes the retired fields.

Do not run the workflow. Call set_workflow_contract_version(version="1.0.27") and stop.`

const upgradeSchedulePromptContract = `WORKFLOW CONTRACT UPGRADE: CURRENT SCHEDULE INSTRUCTIONS.

Do only this one-time schedule migration. It cleans recurring schedule messages;
it does not run, redesign, pause, delete, or create any schedule.

Read workflow.json and inspect every schedule before changing anything. Preserve
each schedule's ID, name, cron/calendar timing, timezone, enabled state, groups,
route selections, execution mode, and every domain safety boundary. Do not change
a route, step, public-action policy, or backup destination in this migration.

Remove historical implementation debris from normal workflow schedule messages
and direct_messages_reason: dated incidents, previous failure counts, old operator
decisions, ticket references, watchdog/idle-wait stories, implementation folder
or database probes, and old workaround narratives. A recurring prompt must state
the current contract, not narrate why an older platform version needed it.

Evaluation ownership must remain correct:
1. If a schedule currently owns a routine evaluation, keep that behavior as one
   concise instruction immediately after the selected work completes. Do not
   turn it into a conditional probe/retry of an undocumented automatic pass.
2. If a higher-frequency schedule deliberately skips evaluation because one
   designated daily/closing schedule owns it, preserve that division plainly.
3. Do not add evaluation to a schedule that did not already own it, and do not
   remove a routine evaluation merely to shorten the message.

Keep a schedule message only when it carries genuinely schedule-specific work
that cannot be represented by its existing groups and route selections. Make it
one concise current-state instruction. A route-backed schedule whose message only
restates running the selected route should have no message. Do not turn ordinary
workflow behavior into a schedule-local procedure.

Do not weaken concrete backup behavior in this migration. Preserve the existing
configured backup contract; only remove historical dates or rationale that do
not change what must be backed up or how success is reported. Do not replace a
working concrete backup procedure with a promise that some other component will
handle it.

Use the schedule-management tools to make any required schedule updates. Re-read
workflow.json afterwards. Confirm every retained message is concise and current,
no retained schedule prose contains a historical incident date or workaround,
and normal workflow schedules do not perform Pulse Gate/Review/Fix inline. Do
not run the workflow. If any change would make evaluation, backup, safety, or a
public action ambiguous, preserve that schedule unchanged, report the blocker,
and do not stamp. Otherwise call
set_workflow_contract_version(version="1.0.28") and stop.`

const upgradeScheduledRoutes = `WORKFLOW CONTRACT UPGRADE: SCHEDULE EXECUTION MODEL (PLAT-086).

Workflow schedules support two valid execution models. A route-based schedule
selects canonical plan work and therefore receives normal step learnings,
validation/retry, repair, and Pulse attribution. A direct message sequence is a
workshop conversation owned by the schedule; it is more flexible, but those
step-level lifecycle guarantees are not automatic. Audit every workflow.json
schedule and classify it from its actual behavior, not merely its text length.

Choose exactly one classification per schedule:

1. EQUIVALENT ROUTE EXISTS. Verify the same inputs, outputs, external side
   effects, approval boundary, failure behavior, and group semantics. Move any
   deterministic routing choice into route_selections and clear messages.
2. DURABLE WORKFLOW BEHAVIOR, NO ROUTE EXISTS. Create the missing canonical
   route/steps and their validation before changing the schedule. Never map a
   draft-only schedule to a route that can publish. Then set route_selections
   and clear messages.
3. GENUINELY SCHEDULE-SPECIFIC CONVERSATION. Preserve its messages and set a
   concise direct_messages_reason explaining why a planned route is the wrong
   abstraction and acknowledging the weaker step-level lifecycle. Do not force
   a route merely because the queue is long.

Compact messages that only restate run_full_workflow/execute_step should normally
be converted to data-backed group_names/route_selections. Move reusable domain
procedure into owning steps or skills, but preserve conversation turns when their
schedule-specific context is the reason the direct model was chosen.

Validate every plan/config mutation, re-read workflow.json, and confirm each
enabled schedule is either safely route-backed or carries an explicit direct
message rationale. Do not run the workflow. If route equivalence is ambiguous,
keep the direct sequence with an honest rationale rather than guessing. Then call
set_workflow_contract_version(version="1.0.25") and stop.`

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

	steps := make([]workflowVersionUpgrade, 0, 8)
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
	if rank < 22 {
		steps = append(steps, workflowVersionUpgrade{from: version, to: workflowContractDirectHTMLReportsVersion, label: "upgrade-direct-html-reports", query: upgradeDirectHTMLReports})
	}
	// workflowContractScheduleExecutionModelVersion ("1.0.25") sits at rank 24
	// — a workflow already at or past it must not repeat this rung.
	if rank < 24 {
		steps = append(steps, workflowVersionUpgrade{from: version, to: workflowContractScheduleExecutionModelVersion, label: "upgrade-schedule-execution-model", query: upgradeScheduledRoutes})
	}
	if rank < 26 {
		steps = append(steps, workflowVersionUpgrade{from: version, to: workflowContractDedicatedPulseScheduleVersion, label: "upgrade-dedicated-pulse-schedule", query: upgradeDedicatedPulseSchedule})
	}
	if rank < 27 {
		steps = append(steps, workflowVersionUpgrade{from: version, to: workflowContractSchedulePromptContractVersion, label: "upgrade-schedule-prompt-contract", query: upgradeSchedulePromptContract})
	}
	// Attached here rather than at the call site so the turn text is identical
	// wherever it is built. The version pair used to be added only on the Pulse
	// delivery path, which meant the blocking preflight — the one that actually
	// gates on the stamp — never told the agent which version it was moving
	// from or to. That path is gone; the context it carried is not.
	for i := range steps {
		steps[i].query += fmt.Sprintf(
			"\n\nCurrent workflow.json version seen by scheduler: %q. Target workflow contract version: %q.",
			version, steps[i].to,
		) + upgradeTurnAutonomyNote
	}
	return steps
}

// upgradeTurnAutonomyNote is appended to every upgrade query.
//
// These turns are fired by the scheduler, usually outside working hours, and
// nobody reads their output. An upgrade turn that ends by asking the owner a
// question in its reply text is not waiting for an answer — it is stalling, and
// the stall repeats on every future trigger while the workflow never runs.
// confida-login sat at 1.0.20 that way: three separate turns each correctly
// identified the same blocker, each ended with "Which would you like?", and its
// QA had not executed since.
//
// The wording matters more than it looks. An earlier draft said "do not
// re-open it as a judgement call" and "a question reaches nobody", which reads
// as pressure to override a safety pause — the exact shape of an injection
// attempt. An agent that had correctly blocked this migration refused again,
// and cited the urgency framing as its reason. It was right to.
//
// So this states a fact about the execution context and leaves refusing fully
// available. What it removes is only the *ineffective* form of stopping:
// a question in reply text that no one receives. create_human_input_request is
// durable, surfaces to the owner, and scheduledDecisionDrainTurn applies the
// answer before the next run.
const upgradeTurnAutonomyNote = `

EXECUTION CONTEXT. This is an automated platform migration — the platform's own scheduled maintenance on this workflow, not a user request relayed through the scheduler. It runs unattended, and this turn is the one that owns the decision. Use your engineering judgment and take the best action to complete the migration properly, the same way you would if you had found this work yourself.

Completing it and stamping, or stopping without stamping, are both acceptable outcomes. What does not work is asking a question in your reply: the reply is not delivered to anyone, so the run simply ends and the same turn repeats unchanged on the next schedule. If you find something the instruction above does not cover, raise it with create_human_input_request — durable, reaches the workflow owner, and their answer is applied before the next run — then stop without stamping.`
