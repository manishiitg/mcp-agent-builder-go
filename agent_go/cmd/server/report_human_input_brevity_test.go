package server

import (
	"strings"
	"testing"
)

// A real decision card produced under the old tool description ran to several
// paragraphs of analyst-style prose per section (e.g. a Goal Advisor proposal
// walking through score distributions and research-bias statistics sentence
// by sentence) before stating its actual recommendation. The question and
// context fields must instruct brevity and plain language explicitly, or the
// model defaults to the same exhaustive, technical register it uses for the
// reviewer findings file the card is summarizing.
func TestCreateHumanInputRequestDescriptionsRequireBrevity(t *testing.T) {
	tools, _, _ := createReportHumanInputTools()
	var create *struct {
		name, question, context string
	}
	for _, tool := range tools {
		if tool.Function == nil || tool.Function.Name != "create_human_input_request" {
			continue
		}
		props := tool.Function.Parameters.Properties
		question, ok := props["question"].(map[string]interface{})
		if !ok {
			t.Fatal("question property missing or malformed")
		}
		context, ok := props["context"].(map[string]interface{})
		if !ok {
			t.Fatal("context property missing or malformed")
		}
		create = &struct{ name, question, context string }{
			name:     tool.Function.Name,
			question: question["description"].(string),
			context:  context["description"].(string),
		}
	}
	if create == nil {
		t.Fatal("create_human_input_request tool not found")
	}

	for _, want := range []string{"short", "plain"} {
		if !strings.Contains(strings.ToLower(create.question), want) {
			t.Errorf("question description missing %q: %s", want, create.question)
		}
	}
	for _, want := range []string{
		"non-technical operator",
		"one to three short sentences",
		"not how you got there",
	} {
		if !strings.Contains(create.context, want) {
			t.Errorf("context description missing %q: %s", want, create.context)
		}
	}
}
