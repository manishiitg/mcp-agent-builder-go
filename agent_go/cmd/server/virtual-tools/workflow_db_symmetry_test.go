package virtualtools

import (
	"encoding/json"
	"strings"
	"testing"
)

// query_workflow_db and mutate_workflow_db differ only in how the database is
// opened: query_only(true) for reads, read-write for mutations. Their argument
// shapes must therefore match, because an agent that has just used one reaches
// for the same shape on the other. When they diverged, a run produced ten
// "statements must contain at least one mutation" failures from callers that
// had sent sql=, set=, and upsert= at the top level.
func TestBothWorkflowDBToolsAcceptTopLevelSQL(t *testing.T) {
	for _, def := range []struct {
		label string
		tool  interface{}
	}{
		{"query", workflowDBQueryToolDefinition()},
		{"mutate", workflowDBMutateToolDefinition()},
	} {
		encoded, err := json.Marshal(def.tool)
		if err != nil {
			t.Fatalf("%s: %v", def.label, err)
		}
		var shape struct {
			Function struct {
				Parameters struct {
					Properties map[string]json.RawMessage `json:"properties"`
					Required   []string                   `json:"required"`
				} `json:"parameters"`
			} `json:"function"`
		}
		if err := json.Unmarshal(encoded, &shape); err != nil {
			t.Fatalf("%s: %v", def.label, err)
		}

		if _, ok := shape.Function.Parameters.Properties["sql"]; !ok {
			t.Errorf("%s_workflow_db has no top-level sql argument; the two tools must take raw SQL the same way", def.label)
		}
		for _, required := range shape.Function.Parameters.Required {
			if required == "action" || required == "statements" {
				t.Errorf("%s_workflow_db still requires %q, so raw SQL alone is rejected", def.label, required)
			}
		}
	}
}

// describe stays available; it is a convenience, not a required preamble.
func TestQueryWorkflowDBStillOffersDescribe(t *testing.T) {
	encoded, err := json.Marshal(workflowDBQueryToolDefinition())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(encoded), "describe") {
		t.Fatal("action=describe was removed; schema inspection has no replacement")
	}
}

func TestQueryWorkflowDBAdvertisesQueryCompatibilityAlias(t *testing.T) {
	encoded, err := json.Marshal(workflowDBQueryToolDefinition())
	if err != nil {
		t.Fatal(err)
	}
	var shape struct {
		Function struct {
			Parameters struct {
				Properties map[string]json.RawMessage `json:"properties"`
			} `json:"parameters"`
		} `json:"function"`
	}
	if err := json.Unmarshal(encoded, &shape); err != nil {
		t.Fatal(err)
	}
	if _, ok := shape.Function.Parameters.Properties["query"]; !ok {
		t.Fatal("query_workflow_db does not advertise its query compatibility alias")
	}
}
