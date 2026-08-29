package step_based_workflow

import (
	"strings"
	"testing"
)

// TestValidateValueTypePatternCompatibilityRejectsUnsatisfiableCombination
// pins PLAT-236: Twitter/social-media PUL-08AC60BB and PUL-ED75E920 both
// traced to a validation_schema check that paired value_type=boolean with a
// string-only pattern on the same path, which no JSON value can ever
// satisfy (validatePattern always fails a non-string value). The workflow
// had already fixed its own live schema before these findings were
// dispositioned; this write-time guard stops the shape from ever landing
// again through any of the tools that accept a validation_schema.
func TestValidateValueTypePatternCompatibilityRejectsUnsatisfiableCombination(t *testing.T) {
	schema := &ValidationSchema{
		Files: []FileValidationRule{
			{
				FileName: "metrics_today.json",
				JSONChecks: []JSONValidationCheck{
					{Path: "$.reach_snapshot_table_updated", ValueType: "boolean", Pattern: "^true$"},
				},
			},
		},
	}
	err := validateValueTypePatternCompatibility(schema)
	if err == nil {
		t.Fatal("expected an error for value_type=boolean combined with a pattern")
	}
	if !strings.Contains(err.Error(), "reach_snapshot_table_updated") || !strings.Contains(err.Error(), "boolean") {
		t.Fatalf("error does not name the offending path/value_type: %v", err)
	}
}

func TestValidateValueTypePatternCompatibilityAllowsStringWithPattern(t *testing.T) {
	schema := &ValidationSchema{
		Files: []FileValidationRule{
			{
				FileName: "profile.json",
				JSONChecks: []JSONValidationCheck{
					{Path: "$.handle", ValueType: "string", Pattern: "^@"},
					{Path: "$.notes", Pattern: "^ok$"}, // no value_type set at all
				},
			},
		},
	}
	if err := validateValueTypePatternCompatibility(schema); err != nil {
		t.Fatalf("string value_type + pattern should be allowed: %v", err)
	}
}

func TestValidateValueTypePatternCompatibilityAllowsNonStringValueTypeWithoutPattern(t *testing.T) {
	schema := &ValidationSchema{
		Files: []FileValidationRule{
			{
				FileName: "metrics_today.json",
				JSONChecks: []JSONValidationCheck{
					{Path: "$.reach_snapshot_table_updated", ValueType: "boolean"},
				},
			},
		},
	}
	if err := validateValueTypePatternCompatibility(schema); err != nil {
		t.Fatalf("value_type without a pattern should be allowed: %v", err)
	}
}

func TestValidateValueTypePatternCompatibilityNilSchema(t *testing.T) {
	if err := validateValueTypePatternCompatibility(nil); err != nil {
		t.Fatalf("nil schema should not error: %v", err)
	}
}
