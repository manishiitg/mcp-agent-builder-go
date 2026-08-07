package step_based_workflow

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func assertGoalAdvisorPromptContains(t *testing.T, prompt string, snippets ...string) {
	t.Helper()
	for _, snippet := range snippets {
		if !strings.Contains(prompt, snippet) {
			t.Fatalf("goal advisor prompt missing %q\n\nPrompt:\n%s", snippet, prompt)
		}
	}
}

func assertToolListContains(t *testing.T, tools []string, tool string) {
	t.Helper()
	for _, candidate := range tools {
		if candidate == tool {
			return
		}
	}
	t.Fatalf("tool list missing %q in %v", tool, tools)
}

func assertToolListDoesNotContain(t *testing.T, tools []string, tool string) {
	t.Helper()
	for _, candidate := range tools {
		if candidate == tool {
			t.Fatalf("tool list should not contain %q in %v", tool, tools)
		}
	}
}

func TestParseAdvisorSpecializationDecisionContext(t *testing.T) {
	strategy, goal, err := parseAdvisorSpecializationDecisionContext(`Proposal:
Specialize the reviewers.
Strategy Auditor specialization:
Inspect current-channel concentration and funnel leakage.
Goal Advisor specialization:
Explore partnerships and product-led referral loops.`)
	if err != nil {
		t.Fatalf("parse specialization context: %v", err)
	}
	if strategy != "Inspect current-channel concentration and funnel leakage." {
		t.Fatalf("strategy = %q", strategy)
	}
	if goal != "Explore partnerships and product-led referral loops." {
		t.Fatalf("goal = %q", goal)
	}

	if _, _, err := parseAdvisorSpecializationDecisionContext("Strategy Auditor specialization: only one"); err == nil {
		t.Fatal("missing Goal Advisor section should fail")
	}
}

func TestApplyAdvisorSpecializationToManifestIsVersionedAndIdempotent(t *testing.T) {
	base := `{"schema_version":1,"id":"demo","label":"Demo","capabilities":{},"schedules":[]}`
	updated, specialization, already, err := applyAdvisorSpecializationToManifest(
		base,
		"advisor-specialization-20260806T120000Z",
		"Inspect the current funnel.",
		"Explore a new distribution channel.",
		"2026-08-06T12:00:00Z",
	)
	if err != nil || already {
		t.Fatalf("first activation err=%v already=%v", err, already)
	}
	if specialization.Version != 1 || specialization.ApprovedInputID == "" {
		t.Fatalf("unexpected specialization: %#v", specialization)
	}

	unchanged, same, already, err := applyAdvisorSpecializationToManifest(
		updated,
		specialization.ApprovedInputID,
		specialization.StrategyAuditor,
		specialization.GoalAdvisor,
		"2026-08-06T12:01:00Z",
	)
	if err != nil || !already {
		t.Fatalf("idempotent activation err=%v already=%v", err, already)
	}
	if unchanged != updated || same.Version != 1 {
		t.Fatalf("idempotent activation changed manifest or version")
	}

	_, next, already, err := applyAdvisorSpecializationToManifest(
		updated,
		"advisor-specialization-20260806T130000Z",
		"Inspect cohorts inside the current strategy.",
		"Explore an ecosystem partnership.",
		"2026-08-06T13:00:00Z",
	)
	if err != nil || already || next.Version != 2 {
		t.Fatalf("replacement activation err=%v already=%v version=%d", err, already, next.Version)
	}
}

func TestAdvisorSpecializationPromptIsModuleScoped(t *testing.T) {
	specialization := &workflowAdvisorSpecialization{
		Version:         2,
		StrategyAuditor: "STRATEGY-LENS-UNIQUE",
		GoalAdvisor:     "GOAL-LENS-UNIQUE",
	}
	strategy := advisorSpecializationPrompt(specialization, "strategy_auditor")
	goal := advisorSpecializationPrompt(specialization, "goal_advisor")
	if !strings.Contains(strategy, "STRATEGY-LENS-UNIQUE") || strings.Contains(strategy, "GOAL-LENS-UNIQUE") {
		t.Fatalf("strategy prompt was not isolated: %s", strategy)
	}
	if !strings.Contains(goal, "GOAL-LENS-UNIQUE") || strings.Contains(goal, "STRATEGY-LENS-UNIQUE") {
		t.Fatalf("goal prompt was not isolated: %s", goal)
	}
	if got := advisorSpecializationPrompt(specialization, "workflow_review"); got != "" {
		t.Fatalf("engineering review received advisor specialization: %q", got)
	}
}

func TestWorkshopStageAgentIdentityIsDistinctAndSafe(t *testing.T) {
	first := newWorkshopStageAgentIdentity("Pulse reviewer - artifact review")
	second := newWorkshopStageAgentIdentity("Pulse reviewer - artifact review")
	if first == second {
		t.Fatalf("stage identities must be unique, got %q", first)
	}
	for _, identity := range []string{first, second} {
		if !strings.HasPrefix(identity, "pulse-reviewer-artifact-review-") {
			t.Fatalf("unexpected sanitized stage identity %q", identity)
		}
		if strings.ContainsAny(identity, " _/") {
			t.Fatalf("stage identity contains unsafe separators: %q", identity)
		}
	}
}

func TestCompletedPulseReviewerResultRequiresFinalMarker(t *testing.T) {
	marker := pulseReviewerCompletionMarker("artifact-review-2026-07-14")
	completed, err := completedPulseReviewerResult("Verdict: clean\n"+marker, marker)
	if err != nil {
		t.Fatalf("completed reviewer result rejected: %v", err)
	}
	if completed != "Verdict: clean" {
		t.Fatalf("completed result = %q, want stripped findings", completed)
	}

	for name, result := range map[string]string{
		"still thinking":   "I am still inspecting the evidence...",
		"marker not final": marker + "\nmore progress text",
		"empty findings":   marker,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := completedPulseReviewerResult(result, marker); err == nil {
				t.Fatalf("expected incomplete reviewer output to be rejected: %q", result)
			}
		})
	}
}

func TestBuildPulseReviewerInstructionMakesTypedToolsAuthoritative(t *testing.T) {
	marker := pulseReviewerCompletionMarker("eval-health")
	brief := "Return findings.\nEnd with exactly: REVIEW_COMPLETE eval_health"
	instruction := buildPulseReviewerInstruction("Workflow/example", "pulse_review_log:run:eval_health", brief, marker)

	if !strings.HasSuffix(instruction, marker) {
		t.Fatalf("tool marker must be the final instruction, got:\n%s", instruction)
	}
	if strings.LastIndex(instruction, marker) <= strings.LastIndex(instruction, "REVIEW_COMPLETE eval_health") {
		t.Fatalf("tool marker must override a conflicting marker from the caller, got:\n%s", instruction)
	}
	if !strings.Contains(instruction, "overrides any earlier response-ending instruction") {
		t.Fatalf("instruction must explain marker precedence, got:\n%s", instruction)
	}
	for _, want := range []string{
		"TYPED REVIEW CONTRACT",
		"record_pulse_finding",
		"record_pulse_verification",
		"complete_pulse_review",
		"final response is informational only",
		"BACKLOG RECONCILIATION",
		`get_pulse_state(view="backlog")`,
		"expected outcome",
		"observed failure",
		"reuse the stable concern/finding identity",
		"Do not write or return a Markdown report",
	} {
		if !strings.Contains(instruction, want) {
			t.Fatalf("artifact-first instruction missing %q:\n%s", want, instruction)
		}
	}
}

func TestBuildPulseFixerInstructionDoesNotInheritReadOnlyReviewerContract(t *testing.T) {
	marker := pulseReviewerCompletionMarker("all-modules-fixer")
	instruction := buildPulseFixerInstruction("Workflow/example", "Drain bug_review and eval_health.", marker)
	for _, want := range []string{"PULSE FIXER WRITE SCOPE", "single writer", "CONSOLIDATED FIX QUEUE", "no attempt_id to carry", "repair bundles"} {
		if !strings.Contains(instruction, want) {
			t.Fatalf("Fixer instruction missing %q:\n%s", want, instruction)
		}
	}
	for _, forbidden := range []string{"READ-ONLY REVIEW SCOPE", "do not write a file", "ARTIFACT-FIRST RESULT CONTRACT"} {
		if strings.Contains(instruction, forbidden) {
			t.Fatalf("Fixer instruction inherited reviewer restriction %q:\n%s", forbidden, instruction)
		}
	}
	if !strings.HasSuffix(instruction, marker) {
		t.Fatalf("Fixer marker must be final: %s", instruction)
	}
}

func TestGoalAdvisorToolAllowlistsSeparateReadOnlyAndFinalizerActions(t *testing.T) {
	readOnly := goalAdvisorReadOnlyToolAgentAllowedToolNames()
	proposal := goalAdvisorFinalizerProposalToolAgentAllowedToolNames()
	approved := goalAdvisorFinalizerApprovedToolAgentAllowedToolNames()

	for _, tool := range []string{"get_workflow_command_guidance", "execute_shell_command"} {
		assertToolListContains(t, readOnly, tool)
		assertToolListContains(t, proposal, tool)
		assertToolListContains(t, approved, tool)
	}
	for name, tools := range map[string][]string{"read-only": readOnly, "proposal": proposal, "approved": approved} {
		if toolSet(tools)["read_skill"] {
			t.Fatalf("%s builder allowlist should not own mcpagent's intrinsic read_skill tool", name)
		}
	}
	for _, tool := range []string{"get_pulse_state"} {
		assertToolListContains(t, readOnly, tool)
	}

	for _, tool := range []string{"diff_patch_workspace_file", "create_human_input_request", "upsert_report_widget"} {
		assertToolListDoesNotContain(t, readOnly, tool)
		assertToolListContains(t, proposal, tool)
		assertToolListContains(t, approved, tool)
	}

	for _, tool := range []string{"mark_human_input_consumed", "update_scripted_step", "update_step_config", "update_validation_schema"} {
		assertToolListDoesNotContain(t, readOnly, tool)
		assertToolListDoesNotContain(t, proposal, tool)
		assertToolListContains(t, approved, tool)
	}

	for _, tool := range []string{"harden_workflow", "record_pulse_result", "notify_user"} {
		assertToolListDoesNotContain(t, readOnly, tool)
		assertToolListDoesNotContain(t, proposal, tool)
		assertToolListDoesNotContain(t, approved, tool)
	}
}

func TestGoalAdvisorAdvisorInstructionIsReadOnlyDraft(t *testing.T) {
	prompt := buildGoalAdvisorAdvisorInstruction("pulse-123", "goals are flat")

	assertGoalAdvisorPromptContains(t, prompt,
		"stage 1/3: ADVISOR DRAFT",
		"Pulse run id: pulse-123",
		"Focus from Pulse Gate: goals are flat",
		"this stage is read-only",
		"Do NOT launch nested maintenance reviewers",
		"Do NOT modify plan/config/eval/report/HTML files",
		"Evidence used",
		"Advisor hypothesis",
		"Review mode: recovery | headroom | active_experiment | approved_answer",
		"Exactly one experiment may be active",
		"10x counterfactual",
		"Current baseline and current strategy ceiling",
		"primary success metric, guardrails, review checkpoint, and rollback condition",
		"Routine-maintenance deferrals",
		"Spend this stage on goal reality, strategy ceiling",
		"Do not inspect CSS, visual design, unrelated timeline history, or page formatting",
		"formatting belongs to Report Health",
	)
}

func TestGoalAdvisorCriticInstructionChallengesAdvisorWithoutMutating(t *testing.T) {
	prompt := buildGoalAdvisorCriticInstruction("pulse-123", "conversion stalled", "advisor draft body")

	assertGoalAdvisorPromptContains(t, prompt,
		"stage 2/3: INDEPENDENT CRITIC",
		"Advisor draft to critique",
		"advisor draft body",
		"Do NOT modify plan/config/eval/report/HTML files",
		"Is every important claim backed by concrete run/eval/report/HTML/db evidence?",
		"Does it hallucinate unavailable data",
		"Is the 10x thesis materially different from incremental tuning",
		"preserve the current successful baseline",
		"reject any second active proposal",
		"Verdict: approve | revise | reject | needs_user | no_action",
		"What the Finalizer is allowed to do",
	)
}

func TestGoalAdvisorFinalizerInstructionOwnsDurableActions(t *testing.T) {
	prompt := buildGoalAdvisorFinalizerInstruction("pulse-123", "strategy gap", "advisor draft body", "critic verdict body", nil, false)

	assertGoalAdvisorPromptContains(t, prompt,
		"stage 3/3: FINALIZER",
		"Advisor draft",
		"advisor draft body",
		"Critic verdict",
		"critic verdict body",
		"only stage allowed to make durable changes",
		"plan/config/eval mutation tools: DISABLED",
		"create_human_input_request",
		"mark_human_input_consumed: DISABLED",
		"Do not launch nested maintenance reviewers",
		"Do not call record_pulse_result",
		"Advisor proposal/takeaway",
		"Critic verdict/objections",
		"Never leave more than one active .advisor-experiment",
		`data-advisor-experiment-id="advisor-exp-<stable-slug>"`,
		"Current baseline, Current strategy ceiling, 10x thesis",
		"Update the existing card in place",
		"Do not repeat their research or turn this stage into an HTML design task",
		"Make at most one targeted builder/improve.html patch",
		"Do not load review-improve-log-skeleton or html-output",
		"do not edit builder/improve.html merely to log activity",
	)
}

func TestGoalAdvisorFinalizerInstructionListsApprovedProposalGate(t *testing.T) {
	prompt := buildGoalAdvisorFinalizerInstruction(
		"pulse-123",
		"strategy gap",
		"advisor draft body",
		"Verdict: approve\nSafe to apply.",
		[]goalAdvisorApprovedPlanProposal{{
			ID:               "plan-proposal-new-segment",
			Context:          "Add a discovery segment.",
			Evidence:         "builder/improve.html",
			SelectedOptionID: "approve",
			Note:             "Do it.",
		}},
		true,
	)

	assertGoalAdvisorPromptContains(t, prompt,
		"plan/config/eval mutation tools: ENABLED",
		"mark_human_input_consumed: ENABLED",
		"input_id: plan-proposal-new-segment",
		"selected_option_id: approve",
		"apply only the verified approved plan-proposal ids listed above",
	)
}

func TestGoalAdvisorCriticApprovesPlanMutationRequiresApproveVerdict(t *testing.T) {
	tests := []struct {
		name string
		body string
		want bool
	}{
		{name: "approve", body: "- Verdict: approve\nLooks safe.", want: true},
		{name: "approve with detail", body: "Verdict: approve_with_limits", want: true},
		{name: "reject", body: "Verdict: reject", want: false},
		{name: "needs user", body: "Verdict: needs_user", want: false},
		{name: "missing", body: "Looks ok, but no structured verdict.", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := goalAdvisorCriticApprovesPlanMutation(tt.body); got != tt.want {
				t.Fatalf("goalAdvisorCriticApprovesPlanMutation() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestApprovedGoalAdvisorPlanProposalsReadsOnlyApprovedAnsweredRows(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	t.Setenv("WORKSPACE_DOCS_PATH", root)
	workspacePath := "Workflow/demo"
	dbDir := filepath.Join(root, "Workflow", "demo", "db")
	if err := os.MkdirAll(dbDir, 0o755); err != nil {
		t.Fatalf("mkdir db dir: %v", err)
	}
	db, err := sql.Open("sqlite", filepath.Join(dbDir, "db.sqlite"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()
	_, err = db.ExecContext(ctx, `CREATE TABLE report_human_inputs (
		id TEXT PRIMARY KEY,
		workspace_path TEXT NOT NULL,
		source TEXT NOT NULL,
		status TEXT NOT NULL,
		context TEXT NOT NULL DEFAULT '',
		evidence TEXT NOT NULL DEFAULT '',
		selected_option_id TEXT NOT NULL DEFAULT '',
		note TEXT NOT NULL DEFAULT '',
		answered_at TEXT NOT NULL DEFAULT '',
		updated_at TEXT NOT NULL DEFAULT ''
	)`)
	if err != nil {
		t.Fatalf("create table: %v", err)
	}
	rows := []struct {
		id, source, status, selected string
	}{
		{"plan-proposal-approved", "goal_advisor", "answered", "approve"},
		{"plan-proposal-deferred", "goal_advisor", "answered", "defer"},
		{"plan-proposal-pending", "goal_advisor", "pending", "approve"},
		{"input-other", "goal_advisor", "answered", "approve"},
		{"plan-proposal-pulse", "pulse", "answered", "approve"},
	}
	for _, row := range rows {
		if _, err := db.ExecContext(ctx, `INSERT INTO report_human_inputs
			(id, workspace_path, source, status, context, evidence, selected_option_id, note, answered_at, updated_at)
			VALUES (?, ?, ?, ?, 'context', 'evidence', ?, 'note', '2026-07-09T00:00:00Z', '2026-07-09T00:00:00Z')`,
			row.id, workspacePath, row.source, row.status, row.selected); err != nil {
			t.Fatalf("insert %s: %v", row.id, err)
		}
	}

	iwm := &InteractiveWorkshopManager{}
	got, err := iwm.approvedGoalAdvisorPlanProposals(ctx, workspacePath)
	if err != nil {
		t.Fatalf("approvedGoalAdvisorPlanProposals() error = %v", err)
	}
	if len(got) != 1 || got[0].ID != "plan-proposal-approved" {
		t.Fatalf("approved proposals = %#v, want only plan-proposal-approved", got)
	}
}

func TestTruncateGoalAdvisorStageOutputKeepsHeadAndTail(t *testing.T) {
	short := "short output"
	if got := truncateGoalAdvisorStageOutput(short); got != short {
		t.Fatalf("short output should not be changed: %q", got)
	}

	long := strings.Repeat("A", 11_000) + "MIDDLE" + strings.Repeat("Z", 11_000)
	got := truncateGoalAdvisorStageOutput(long)
	if len(got) >= len(long) {
		t.Fatalf("expected long output to be truncated; got len=%d want < %d", len(got), len(long))
	}
	assertGoalAdvisorPromptContains(t, got,
		strings.Repeat("A", 100),
		"[Goal Advisor stage output truncated for next-stage review]",
		strings.Repeat("Z", 100),
	)
	if strings.Contains(got, "MIDDLE") {
		t.Fatalf("expected middle of long output to be truncated")
	}
}
