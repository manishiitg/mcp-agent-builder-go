package server

import (
	"testing"
	"time"

	internalevents "github.com/manishiitg/coding-agent-loop/agent_go/internal/events"
)

func TestMergeChatHistoryUIEventsKeepsTheSavedTraceBeforeTheLiveOne(t *testing.T) {
	base := time.Date(2026, 9, 3, 13, 39, 0, 0, time.UTC)
	persisted := []internalevents.Event{
		{ID: "a", Type: "user_message", Timestamp: base},
		{ID: "b", Type: "tool_call_start", Timestamp: base.Add(time.Second)},
		{ID: "c", Type: "agent_end", Timestamp: base.Add(2 * time.Second)},
		{ID: "d", Type: "user_message", Timestamp: base.Add(11 * time.Minute)}, // covered by the live store
	}
	live := []internalevents.Event{
		{ID: "d", Type: "user_message", Timestamp: base.Add(11 * time.Minute)},
		{ID: "e", Type: "agent_end", Timestamp: base.Add(12 * time.Minute)},
	}
	merged := mergeChatHistoryUIEvents(persisted, live)
	got := ""
	for _, event := range merged {
		got += event.ID
	}
	if got != "abcde" {
		t.Fatalf("merged ids = %q, want abcde", got)
	}
	if len(mergeChatHistoryUIEvents(nil, live)) != 2 || len(mergeChatHistoryUIEvents(persisted, nil)) != 4 {
		t.Fatal("an empty side must yield the other side unchanged")
	}
}
