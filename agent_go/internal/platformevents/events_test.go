package platformevents

import (
	"reflect"
	"testing"
)

func TestCoreEventContract(t *testing.T) {
	want := []Type{
		"message_started", "message_delta", "message_completed",
		"tool_started", "tool_completed", "tool_failed",
		"status_changed", "human_input_required",
		"run_started", "run_completed", "run_failed", "run_cancelled", //nolint:misspell // wire value
	}
	if !reflect.DeepEqual(CoreTypes, want) {
		t.Fatalf("core event contract changed\n got: %v\nwant: %v", CoreTypes, want)
	}
}
