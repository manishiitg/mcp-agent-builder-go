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
