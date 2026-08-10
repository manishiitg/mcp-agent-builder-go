package main

import (
	"testing"
	"time"
)

func TestRecordActivityLogEntryAccumulatesSameDaySameActivity(t *testing.T) {
	t.Setenv("FAMILY_DATA_DIR", t.TempDir())

	recordActivityLogEntry("Mathematics/Fractions/quick-check", 5*time.Minute)
	recordActivityLogEntry("Mathematics/Fractions/quick-check", 3*time.Minute)

	entries := loadActivityLog()
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d: %+v", len(entries), entries)
	}
	if got, want := entries[0].DurationSeconds, int(8*time.Minute.Seconds()); got != want {
		t.Fatalf("expected accumulated duration %d, got %d", want, got)
	}
}

// The bug the design review caught: a child bouncing between two activities
// in one sitting (Math -> English -> Math, completely normal) must not have
// the second Math turn miss the last entry (English) and append a second,
// fragmented Math entry instead of accumulating onto the first.
func TestRecordActivityLogEntryHandlesInterleavedActivities(t *testing.T) {
	t.Setenv("FAMILY_DATA_DIR", t.TempDir())

	recordActivityLogEntry("Mathematics/Fractions/quick-check", 5*time.Minute)
	recordActivityLogEntry("English/Grammar/practice", 4*time.Minute)
	recordActivityLogEntry("Mathematics/Fractions/quick-check", 2*time.Minute)

	entries := loadActivityLog()
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries (Math, English), got %d: %+v", len(entries), entries)
	}
	var mathSeconds, englishSeconds int
	for _, e := range entries {
		switch e.ActivityDir {
		case "Mathematics/Fractions/quick-check":
			mathSeconds = e.DurationSeconds
		case "English/Grammar/practice":
			englishSeconds = e.DurationSeconds
		}
	}
	if want := int(7 * time.Minute.Seconds()); mathSeconds != want {
		t.Fatalf("expected Math's two turns to accumulate to %d, got %d", want, mathSeconds)
	}
	if want := int(4 * time.Minute.Seconds()); englishSeconds != want {
		t.Fatalf("expected English's single turn to be %d, got %d", want, englishSeconds)
	}
}

func TestRecordActivityLogEntryNewDayStartsFreshEntry(t *testing.T) {
	t.Setenv("FAMILY_DATA_DIR", t.TempDir())

	recordActivityLogEntry("Mathematics/Fractions/quick-check", 5*time.Minute)
	entries := loadActivityLog()
	entries[0].Date = "2020-01-01" // simulate yesterday without waiting a real day
	if err := saveActivityLog(entries); err != nil {
		t.Fatalf("saveActivityLog: %v", err)
	}

	recordActivityLogEntry("Mathematics/Fractions/quick-check", 2*time.Minute)

	entries = loadActivityLog()
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries (yesterday + today), got %d: %+v", len(entries), entries)
	}
	if entries[1].DurationSeconds != int(2*time.Minute.Seconds()) {
		t.Fatalf("expected today's entry to start fresh at %d, got %d", int(2*time.Minute.Seconds()), entries[1].DurationSeconds)
	}
}
