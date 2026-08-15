package server

import (
	"encoding/json"
	"testing"
)

func TestTrimChatHistoryConversationForPreviewKeepsTailAndDropsBulk(t *testing.T) {
	var history []map[string]string
	for i := 0; i < 50; i++ {
		history = append(history, map[string]string{"Role": "ai", "id": string(rune('a' + i%26))})
	}
	doc := map[string]interface{}{
		"session_id":           "s1",
		"conversation_history": history,
		"ui_events":            make([]map[string]string, 200),
		"terminal_snapshots":   []map[string]string{{"content": "pane"}},
	}
	raw, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}

	out := trimChatHistoryConversationForPreview(raw, 10)
	var got map[string]json.RawMessage
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("trimmed document must stay parseable: %v", err)
	}
	if _, ok := got["ui_events"]; ok {
		t.Error("ui_events must be dropped -- it is the bulk of the file and a preview never reads it")
	}
	if _, ok := got["terminal_snapshots"]; ok {
		t.Error("terminal_snapshots must be dropped")
	}
	if _, ok := got["session_id"]; !ok {
		t.Error("unrelated fields must be preserved")
	}

	var trimmed []map[string]string
	if err := json.Unmarshal(got["conversation_history"], &trimmed); err != nil {
		t.Fatal(err)
	}
	if len(trimmed) != 10 {
		t.Fatalf("kept %d messages, want the last 10", len(trimmed))
	}
	// Must be the TAIL: a preview shows how the conversation ended.
	if trimmed[len(trimmed)-1]["id"] != history[len(history)-1]["id"] {
		t.Error("kept the wrong end of the conversation")
	}
}

// A preview that is too large is a performance problem; a preview that fails to
// load is a broken panel. Malformed input must pass through untouched.
func TestTrimChatHistoryConversationForPreviewFallsBackOnBadJSON(t *testing.T) {
	raw := []byte(`{"conversation_history": [ this is not json`)
	if got := string(trimChatHistoryConversationForPreview(raw, 5)); got != string(raw) {
		t.Errorf("malformed input must be returned unchanged, got %q", got)
	}
}

func TestTrimChatHistoryConversationForPreviewKeepsShortConversationsWhole(t *testing.T) {
	raw, _ := json.Marshal(map[string]interface{}{
		"conversation_history": []map[string]string{{"Role": "human"}, {"Role": "ai"}},
	})
	var got map[string][]map[string]string
	if err := json.Unmarshal(trimChatHistoryConversationForPreview(raw, 10), &got); err != nil {
		t.Fatal(err)
	}
	if len(got["conversation_history"]) != 2 {
		t.Errorf("kept %d, want both messages", len(got["conversation_history"]))
	}
}

func TestProjectChatHistoryConversationForResumeKeepsRealTurnsOnly(t *testing.T) {
	message := func(role, text string) map[string]interface{} {
		return map[string]interface{}{
			"Role":  role,
			"Parts": []map[string]string{{"Text": text}},
		}
	}
	raw, _ := json.Marshal(map[string]interface{}{
		"session_id": "resume-1",
		"conversation_history": []map[string]interface{}{
			message("system", "large system prompt"),
			message("human", "first question"),
			message("ai", "I will inspect it."),
			message("ai", `[Previous tool call: exec({"input":"secret trace"})]`),
			message("tool", "tool output"),
			message("ai", "First final answer."),
			message("human", "second question"),
			message("ai", "Second final answer."),
		},
		"ui_events":          []map[string]string{{"type": "tool_call_start"}},
		"terminal_snapshots": []map[string]string{{"content": "raw pane"}},
	})

	out := projectChatHistoryConversationForResume(raw, 20)
	var got map[string]json.RawMessage
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatal(err)
	}
	if _, ok := got["ui_events"]; ok {
		t.Error("resume projection must not return trace events")
	}
	if _, ok := got["terminal_snapshots"]; ok {
		t.Error("resume projection must not return terminal snapshots")
	}
	var history []struct {
		Role  string `json:"Role"`
		Parts []struct {
			Text string `json:"Text"`
		} `json:"Parts"`
	}
	if err := json.Unmarshal(got["conversation_history"], &history); err != nil {
		t.Fatal(err)
	}
	if len(history) != 4 {
		t.Fatalf("got %d projected messages, want two user/assistant pairs", len(history))
	}
	texts := make([]string, 0, len(history))
	for _, item := range history {
		texts = append(texts, item.Parts[0].Text)
	}
	want := []string{"first question", "First final answer.", "second question", "Second final answer."}
	for index := range want {
		if texts[index] != want[index] {
			t.Fatalf("message %d = %q, want %q", index, texts[index], want[index])
		}
	}
}

func TestProjectChatHistoryConversationForResumeLimitsByUserTurn(t *testing.T) {
	message := func(role, text string) map[string]interface{} {
		return map[string]interface{}{"role": role, "parts": []map[string]string{{"text": text}}}
	}
	raw, _ := json.Marshal(map[string]interface{}{
		"conversation_history": []map[string]interface{}{
			message("user", "one"), message("assistant", "answer one"),
			message("user", "two"), message("assistant", "answer two"),
			message("user", "three"), message("assistant", "answer three"),
		},
	})

	var got struct {
		History []json.RawMessage `json:"conversation_history"`
	}
	if err := json.Unmarshal(projectChatHistoryConversationForResume(raw, 2), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.History) != 4 {
		t.Fatalf("got %d messages, want the final two turns", len(got.History))
	}
	_, firstText := chatHistoryMessageRoleAndText(got.History[0])
	if firstText != "two" {
		t.Fatalf("first retained turn = %q, want two", firstText)
	}
}
