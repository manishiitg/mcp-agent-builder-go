package step_based_workflow

import (
	"encoding/json"
	"testing"
)

func TestSubAgentConversationPageUsesExecutionIDNotTodoID(t *testing.T) {
	records := []SubAgentCallRecord{
		{
			ExecutionID:  "exec-first",
			TodoID:       "shared-todo",
			Conversation: []ConversationEntry{{Role: "assistant", Content: "first attempt"}},
		},
		{
			ExecutionID:  "exec-retry",
			TodoID:       "shared-todo",
			Conversation: []ConversationEntry{{Role: "assistant", Content: "retry attempt"}},
		},
	}

	encoded, err := subAgentConversationPage(records, "exec-first", 20, 0)
	if err != nil {
		t.Fatal(err)
	}
	var page struct {
		Record SubAgentCallRecord `json:"record"`
	}
	if err := json.Unmarshal([]byte(encoded), &page); err != nil {
		t.Fatal(err)
	}
	if page.Record.ExecutionID != "exec-first" || len(page.Record.Conversation) != 1 || page.Record.Conversation[0].Content != "first attempt" {
		t.Fatalf("lookup crossed executions for one todo: %+v", page.Record)
	}

	if _, err := subAgentConversationPage(records, "shared-todo", 20, 0); err == nil {
		t.Fatal("task_id was incorrectly accepted as an execution identity")
	}
}
