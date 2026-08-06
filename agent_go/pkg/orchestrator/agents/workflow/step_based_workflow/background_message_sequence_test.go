package step_based_workflow

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	orchestratoragents "github.com/manishiitg/coding-agent-loop/agent_go/pkg/orchestrator/agents"
	"github.com/manishiitg/multi-llm-provider-go/llmtypes"
)

type recordingBackgroundSequenceAgent struct {
	instructions []string
	historyLens  []int
	failAt       int
}

func (a *recordingBackgroundSequenceAgent) Execute(_ context.Context, vars map[string]string, history []llmtypes.MessageContent) (string, []llmtypes.MessageContent, error) {
	a.instructions = append(a.instructions, vars["Instruction"])
	a.historyLens = append(a.historyLens, len(history))
	if a.failAt > 0 && len(a.instructions) == a.failAt {
		return "", history, errors.New("turn failed")
	}
	history = append(history, llmtypes.MessageContent{})
	return "result:" + vars["Instruction"], history, nil
}

func (a *recordingBackgroundSequenceAgent) GetType() string { return "test" }
func (a *recordingBackgroundSequenceAgent) GetConfig() *orchestratoragents.OrchestratorAgentConfig {
	return nil
}
func (a *recordingBackgroundSequenceAgent) Initialize(context.Context) error { return nil }
func (a *recordingBackgroundSequenceAgent) Close() error                     { return nil }
func (a *recordingBackgroundSequenceAgent) GetBaseAgent() *orchestratoragents.BaseAgent {
	return nil
}

func TestParseBackgroundMessageSequence(t *testing.T) {
	items, err := parseBackgroundMessageSequence(map[string]interface{}{
		"message_sequence": []interface{}{
			map[string]interface{}{"id": "first", "title": "First", "message": " inspect "},
			map[string]interface{}{"id": "final", "message": "consolidate"},
		},
	})
	if err != nil {
		t.Fatalf("parse sequence: %v", err)
	}
	want := []backgroundMessageSequenceItem{
		{ID: "first", Title: "First", Message: "inspect"},
		{ID: "final", Message: "consolidate"},
	}
	if !reflect.DeepEqual(items, want) {
		t.Fatalf("items = %#v, want %#v", items, want)
	}
}

func TestParseBackgroundMessageSequenceDefaultsWorkflowReviewerLenses(t *testing.T) {
	items, err := parseBackgroundMessageSequence(map[string]interface{}{
		"role":   "reviewer",
		"module": "workflow_review",
	})
	if err != nil {
		t.Fatalf("parse default workflow review sequence: %v", err)
	}
	wantIDs := []string{"engineering", "llm-ops", "consolidate"}
	gotIDs := make([]string, 0, len(items))
	for _, item := range items {
		gotIDs = append(gotIDs, item.ID)
	}
	if !reflect.DeepEqual(gotIDs, wantIDs) {
		t.Fatalf("default workflow review IDs = %#v, want %#v", gotIDs, wantIDs)
	}
	engineering := items[0].Message
	for _, want := range []string{
		"engineering QA reviewer",
		"bug-review",
		"artifact-drift",
		"improve-report",
		"improve-evaluation",
		"DB/knowledgebase/learnings integrity",
		"wrong business outcome",
		"Strategy Auditor",
		"LLM/Ops",
	} {
		if !strings.Contains(engineering, want) {
			t.Fatalf("default engineering lens missing contract %q:\n%s", want, engineering)
		}
	}
}

func TestParseBackgroundMessageSequenceRunsOnlySelectedWorkflowReviewLanes(t *testing.T) {
	items, err := parseBackgroundMessageSequence(map[string]interface{}{
		"role":         "reviewer",
		"module":       "workflow_review",
		"review_lanes": []interface{}{"llm_ops_review"},
	})
	if err != nil {
		t.Fatalf("parse selected workflow review sequence: %v", err)
	}
	gotIDs := make([]string, 0, len(items))
	for _, item := range items {
		gotIDs = append(gotIDs, item.ID)
	}
	if want := []string{"llm-ops", "consolidate"}; !reflect.DeepEqual(gotIDs, want) {
		t.Fatalf("selected workflow review IDs = %#v, want %#v", gotIDs, want)
	}
	for _, item := range items {
		if item.ID == "engineering" {
			t.Fatalf("skipped lane unexpectedly ran: %#v", item)
		}
	}
}

func TestParseBackgroundMessageSequenceContinuesOperationalReviewIntoFixer(t *testing.T) {
	items, err := parseBackgroundMessageSequence(map[string]interface{}{
		"role":         "fixer",
		"module":       "workflow_review",
		"review_lanes": []interface{}{"workflow_review"},
	})
	if err != nil {
		t.Fatalf("parse combined review/fixer sequence: %v", err)
	}
	gotIDs := make([]string, 0, len(items))
	for _, item := range items {
		gotIDs = append(gotIDs, item.ID)
	}
	if want := []string{"engineering", "consolidate", "fix"}; !reflect.DeepEqual(gotIDs, want) {
		t.Fatalf("combined sequence IDs = %#v, want %#v", gotIDs, want)
	}
	if !strings.Contains(items[len(items)-1].Message, "same conversation") || !strings.Contains(items[len(items)-1].Message, "record_pulse_result") {
		t.Fatalf("combined Fixer turn lacks continuation/lifecycle contract: %s", items[len(items)-1].Message)
	}
}

func TestParseBackgroundMessageSequenceRejectsUnknownWorkflowReviewLane(t *testing.T) {
	_, err := parseBackgroundMessageSequence(map[string]interface{}{
		"role":         "reviewer",
		"module":       "workflow_review",
		"review_lanes": []interface{}{"goal_advisor"},
	})
	if err == nil || !strings.Contains(err.Error(), "unsupported operational lane") {
		t.Fatalf("expected unsupported-lane error, got %v", err)
	}
}

func TestPartitionPulseReviewConcernsPreservesSelectedLaneOwnership(t *testing.T) {
	summary := strings.Join([]string{
		`PULSE_FINDING_JSON: {"module":"workflow_review","concern":"plan changelog is stale","issue_kind":"workflow_issue","summary":"stale plan"}`,
		`CONCERNS: plan changelog is stale`,
		`PULSE_FINDING_JSON: {"module":"llm_ops_review","concern":"tool retries duplicate cost","issue_kind":"workflow_issue","summary":"duplicate retries"}`,
		`CONCERNS: tool retries duplicate cost`,
	}, "\n")

	partitioned, err := partitionPulseReviewConcerns(summary, []string{"workflow_review", "llm_ops_review"})
	if err != nil {
		t.Fatalf("partition concerns: %v", err)
	}
	if !strings.Contains(partitioned["workflow_review"], "plan changelog is stale") || strings.Contains(partitioned["workflow_review"], "tool retries") {
		t.Fatalf("engineering partition = %q", partitioned["workflow_review"])
	}
	if !strings.Contains(partitioned["llm_ops_review"], "tool retries duplicate cost") || strings.Contains(partitioned["llm_ops_review"], "plan changelog") {
		t.Fatalf("ops partition = %q", partitioned["llm_ops_review"])
	}
}

func TestPartitionPulseReviewConcernsRejectsMissingOrSkippedLane(t *testing.T) {
	for name, summary := range map[string]string{
		"missing": `CONCERNS: unattributed problem`,
		"skipped": strings.Join([]string{
			`PULSE_FINDING_JSON: {"module":"strategy_auditor","concern":"wrong lane","issue_kind":"workflow_issue"}`,
			`CONCERNS: wrong lane`,
		}, "\n"),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := partitionPulseReviewConcerns(summary, []string{"workflow_review", "llm_ops_review"}); err == nil {
				t.Fatalf("expected %s attribution to fail", name)
			}
		})
	}
}

func TestParseBackgroundMessageSequenceRejectsDuplicateIDs(t *testing.T) {
	_, err := parseBackgroundMessageSequence(map[string]interface{}{
		"message_sequence": []interface{}{
			map[string]interface{}{"id": "lens", "message": "one"},
			map[string]interface{}{"id": "lens", "message": "two"},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "duplicated") {
		t.Fatalf("expected duplicate-id error, got %v", err)
	}
}

func TestExecuteBackgroundMessageSequenceReusesConversationHistory(t *testing.T) {
	agent := &recordingBackgroundSequenceAgent{}
	template := map[string]string{"WorkspacePath": "/workflow", "Instruction": "stale"}
	result, err := executeBackgroundMessageSequence(context.Background(), agent, template, "open", []backgroundMessageSequenceItem{
		{ID: "lens", Message: "inspect"},
		{ID: "final", Message: "consolidate"},
	})
	if err != nil {
		t.Fatalf("execute sequence: %v", err)
	}
	if result != "result:consolidate" {
		t.Fatalf("result = %q", result)
	}
	if want := []string{"open", "inspect", "consolidate"}; !reflect.DeepEqual(agent.instructions, want) {
		t.Fatalf("instructions = %#v, want %#v", agent.instructions, want)
	}
	if want := []int{0, 1, 2}; !reflect.DeepEqual(agent.historyLens, want) {
		t.Fatalf("history lengths = %#v, want %#v", agent.historyLens, want)
	}
	if template["Instruction"] != "stale" {
		t.Fatalf("caller template was mutated: %#v", template)
	}
}

func TestExecuteBackgroundMessageSequenceObserverCheckpointsBeforeNextTurn(t *testing.T) {
	agent := &recordingBackgroundSequenceAgent{}
	var checkpoints []string
	_, err := executeBackgroundMessageSequenceObserved(context.Background(), agent, nil, "open", []backgroundMessageSequenceItem{
		{ID: "consolidate", Message: "review"},
		{ID: "fix", Message: "repair"},
	}, func(turn backgroundMessageSequenceItem, result string) error {
		checkpoints = append(checkpoints, turn.ID+":"+result)
		return nil
	})
	if err != nil {
		t.Fatalf("execute observed sequence: %v", err)
	}
	if want := []string{"opening:result:open", "consolidate:result:review", "fix:result:repair"}; !reflect.DeepEqual(checkpoints, want) {
		t.Fatalf("checkpoints = %#v, want %#v", checkpoints, want)
	}
}

func TestExecuteDefaultWorkflowReviewerSequenceUsesFourOrderedTurns(t *testing.T) {
	items, err := parseBackgroundMessageSequence(map[string]interface{}{
		"role":   "reviewer",
		"module": "workflow_review",
	})
	if err != nil {
		t.Fatalf("parse default workflow reviewer sequence: %v", err)
	}
	agent := &recordingBackgroundSequenceAgent{}
	if _, err := executeBackgroundMessageSequence(context.Background(), agent, nil, "opening evidence map", items); err != nil {
		t.Fatalf("execute default workflow reviewer sequence: %v", err)
	}
	if len(agent.instructions) != 4 {
		t.Fatalf("reviewer turns = %d, want opening + 3 follow-ups", len(agent.instructions))
	}
	if agent.instructions[0] != "opening evidence map" || !strings.HasPrefix(agent.instructions[3], "Now reconcile every selected-lane checkpoint") {
		t.Fatalf("reviewer turn order was not preserved: %#v", agent.instructions)
	}
	if want := []int{0, 1, 2, 3}; !reflect.DeepEqual(agent.historyLens, want) {
		t.Fatalf("reviewer history lengths = %#v, want %#v", agent.historyLens, want)
	}
}

func TestExecuteBackgroundMessageSequenceStopsAtFailedTurn(t *testing.T) {
	agent := &recordingBackgroundSequenceAgent{failAt: 2}
	_, err := executeBackgroundMessageSequence(context.Background(), agent, nil, "open", []backgroundMessageSequenceItem{
		{ID: "broken", Message: "inspect"},
		{ID: "unreached", Message: "consolidate"},
	})
	if err == nil || !strings.Contains(err.Error(), "turn 2 (broken)") {
		t.Fatalf("expected turn identity in error, got %v", err)
	}
	if len(agent.instructions) != 2 {
		t.Fatalf("executed %d turns after failure, want 2", len(agent.instructions))
	}
}
