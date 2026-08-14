package main

import (
	"context"
	"testing"
	"time"

	"github.com/manishiitg/mcpagent/events"
)

func TestToolCallCollectorKeepsBridgeArgumentsAndActualResult(t *testing.T) {
	collector := newToolCallCollector("parent:test", nil)
	start := events.NewToolCallStartEvent(0, "open_file", events.ToolParams{Arguments: `{"path":"math.md"}`}, "direct_execution", "")
	start.ToolCallID = "direct-1"
	if err := collector.HandleEvent(context.Background(), events.NewAgentEvent(start)); err != nil {
		t.Fatal(err)
	}
	end := events.NewToolCallEndEvent(0, "open_file", `{"opened":true}`, "direct_execution", 15*time.Millisecond, "")
	end.ToolCallID = "direct-1"
	if err := collector.HandleEvent(context.Background(), events.NewAgentEvent(end)); err != nil {
		t.Fatal(err)
	}
	calls := collector.Snapshot()
	if len(calls) != 1 {
		t.Fatalf("calls = %#v, want one merged record", calls)
	}
	if got := calls[0]; got.Status != "completed" || got.Arguments != `{"path":"math.md"}` || got.Result != `{"opened":true}` {
		t.Fatalf("merged record = %#v", got)
	}
}
