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
	wantIDs := []string{"correctness", "artifact-drift", "report-eval", "stores", "llm-ops", "consolidate"}
	gotIDs := make([]string, 0, len(items))
	for _, item := range items {
		gotIDs = append(gotIDs, item.ID)
	}
	if !reflect.DeepEqual(gotIDs, wantIDs) {
		t.Fatalf("default workflow review IDs = %#v, want %#v", gotIDs, wantIDs)
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

func TestExecuteDefaultWorkflowReviewerSequenceUsesSevenOrderedTurns(t *testing.T) {
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
	if len(agent.instructions) != 7 {
		t.Fatalf("reviewer turns = %d, want opening + 6 follow-ups", len(agent.instructions))
	}
	if agent.instructions[0] != "opening evidence map" || !strings.HasPrefix(agent.instructions[6], "Now reconcile every lens checkpoint") {
		t.Fatalf("reviewer turn order was not preserved: %#v", agent.instructions)
	}
	if want := []int{0, 1, 2, 3, 4, 5, 6}; !reflect.DeepEqual(agent.historyLens, want) {
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
