package step_based_workflow

import (
	"strings"
	"testing"
)

func TestFormatMessageSequenceValidationSchemaNilReturnsEmpty(t *testing.T) {
	if got := formatMessageSequenceValidationSchema(nil); got != "" {
		t.Fatalf("formatMessageSequenceValidationSchema(nil) = %q; want empty", got)
	}
}

func TestFormatMessageSequenceValidationSchemaRendersRequiredFiles(t *testing.T) {
	schema := &ValidationSchema{
		Files: []FileValidationRule{{
			FileName:  "results.json",
			MustExist: true,
			JSONChecks: []JSONValidationCheck{{
				Path:      "$.status",
				MustExist: true,
			}},
		}},
	}

	got := formatMessageSequenceValidationSchema(schema)

	if !strings.Contains(got, "## Required Output (Pre-Validation Schema)") {
		t.Fatalf("formatted schema missing header: %q", got)
	}
	if !strings.Contains(got, "results.json") || !strings.Contains(got, "$.status") {
		t.Fatalf("formatted schema missing schema content: %q", got)
	}
}

func TestAppendMessageSequenceValidationSchemaNoSchemaLeavesContextUnchanged(t *testing.T) {
	const existing = "Step description (opening instruction):\ndo the thing"

	if got := appendMessageSequenceValidationSchema(existing, nil); got != existing {
		t.Fatalf("appendMessageSequenceValidationSchema with nil schema = %q; want unchanged %q", got, existing)
	}
}

func TestAppendMessageSequenceValidationSchemaWithNoExistingContextReturnsSchemaOnly(t *testing.T) {
	schema := &ValidationSchema{Files: []FileValidationRule{{FileName: "out.json", MustExist: true}}}

	got := appendMessageSequenceValidationSchema("", schema)

	if !strings.HasPrefix(got, "## Required Output (Pre-Validation Schema)") {
		t.Fatalf("appendMessageSequenceValidationSchema with empty context = %q; want to start with the schema section", got)
	}
}

func TestAppendMessageSequenceValidationSchemaJoinsAfterOpeningInstruction(t *testing.T) {
	const opening = "Step description (opening instruction):\ndo the thing"
	schema := &ValidationSchema{Files: []FileValidationRule{{FileName: "out.json", MustExist: true}}}

	got := appendMessageSequenceValidationSchema(opening, schema)

	if !strings.HasPrefix(got, opening+"\n\n## Required Output (Pre-Validation Schema)") {
		t.Fatalf("appendMessageSequenceValidationSchema = %q; want opening instruction followed by the schema section", got)
	}
	if !strings.Contains(got, "out.json") {
		t.Fatalf("appendMessageSequenceValidationSchema result missing schema content: %q", got)
	}
}
