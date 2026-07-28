package main

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
)

func TestEventRegistryMatchesPayloadUnion(t *testing.T) {
	if err := validateEventRegistry(); err != nil {
		t.Fatalf("invalid event registry: %v", err)
	}
}

func TestRegisteredEventTypesAreStableAndUnique(t *testing.T) {
	eventTypes := registeredEventTypes()
	if !sort.StringsAreSorted(eventTypes) {
		t.Fatal("registered event types must be sorted for deterministic schemas")
	}
	for i := 1; i < len(eventTypes); i++ {
		if eventTypes[i] == eventTypes[i-1] {
			t.Fatalf("duplicate registered event type %q", eventTypes[i])
		}
	}
}

func TestPollingEventSchemaContainsEveryRegisteredEvent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "polling-event.schema.json")
	if err := generateDiscriminatedUnionSchema(path); err != nil {
		t.Fatalf("generate polling event schema: %v", err)
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read polling event schema: %v", err)
	}
	for _, eventType := range registeredEventTypes() {
		if !containsJSONValue(contents, eventType) {
			t.Errorf("schema is missing registered event type %q", eventType)
		}
	}
}

func containsJSONValue(contents []byte, value string) bool {
	needle := []byte(`"` + value + `"`)
	for i := 0; i+len(needle) <= len(contents); i++ {
		if string(contents[i:i+len(needle)]) == string(needle) {
			return true
		}
	}
	return false
}
