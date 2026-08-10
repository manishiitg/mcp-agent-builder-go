package main

import (
	"context"
	"testing"
)

func callSetChildSchedule(t *testing.T, entries []map[string]interface{}) (string, error) {
	t.Helper()
	raw := make([]interface{}, 0, len(entries))
	for _, e := range entries {
		raw = append(raw, e)
	}
	tool := setChildScheduleTool(parentToolSinks{})
	return tool.Handler(context.Background(), map[string]interface{}{"entries": raw})
}

func TestSetChildScheduleSkipsExactDuplicate(t *testing.T) {
	t.Setenv("FAMILY_DATA_DIR", t.TempDir())

	school := map[string]interface{}{"day": "Monday", "start": "08:00", "end": "14:00", "label": "School"}
	if _, err := callSetChildSchedule(t, []map[string]interface{}{school}); err != nil {
		t.Fatalf("first call: %v", err)
	}
	// A Pulse cycle (or the conversational path) trying to save the same
	// commitment again — e.g. because it wasn't sure it was already
	// captured — must not create a second, duplicate row.
	if _, err := callSetChildSchedule(t, []map[string]interface{}{school}); err != nil {
		t.Fatalf("second call: %v", err)
	}

	got := loadState().Schedule.Entries
	if len(got) != 1 {
		t.Fatalf("expected exactly 1 entry after saving the same commitment twice, got %d: %+v", len(got), got)
	}
}

func TestSetChildScheduleAddsGenuinelyNewEntry(t *testing.T) {
	t.Setenv("FAMILY_DATA_DIR", t.TempDir())

	school := map[string]interface{}{"day": "Monday", "start": "08:00", "end": "14:00", "label": "School"}
	swimming := map[string]interface{}{"day": "Tuesday", "start": "17:00", "end": "18:00", "label": "Swimming"}
	if _, err := callSetChildSchedule(t, []map[string]interface{}{school}); err != nil {
		t.Fatalf("first call: %v", err)
	}
	if _, err := callSetChildSchedule(t, []map[string]interface{}{swimming}); err != nil {
		t.Fatalf("second call: %v", err)
	}

	got := loadState().Schedule.Entries
	if len(got) != 2 {
		t.Fatalf("expected 2 distinct entries, got %d: %+v", len(got), got)
	}
}

// A same day/label but a DIFFERENT time (a practice time that genuinely
// changed) is not a duplicate and must still be saved.
func TestSetChildScheduleAllowsSameLabelDifferentTime(t *testing.T) {
	t.Setenv("FAMILY_DATA_DIR", t.TempDir())

	morning := map[string]interface{}{"day": "Monday", "start": "08:00", "end": "14:00", "label": "School"}
	afternoon := map[string]interface{}{"day": "Monday", "start": "09:00", "end": "15:00", "label": "School"}
	if _, err := callSetChildSchedule(t, []map[string]interface{}{morning}); err != nil {
		t.Fatalf("first call: %v", err)
	}
	if _, err := callSetChildSchedule(t, []map[string]interface{}{afternoon}); err != nil {
		t.Fatalf("second call: %v", err)
	}

	got := loadState().Schedule.Entries
	if len(got) != 2 {
		t.Fatalf("expected 2 entries (times differ, not a duplicate), got %d: %+v", len(got), got)
	}
}
