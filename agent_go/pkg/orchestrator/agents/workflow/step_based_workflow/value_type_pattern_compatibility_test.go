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

// TestSchemaValidatorsAlsoCoverDBChecks is an independent-review follow-up
// on PLAT-236: all four write-time schema validators
// (validateRegexPatternsInSchema, validateJSONPathSyntax,
// validateArrayLengthConsistencyChecks, validateValueTypePatternCompatibility)
// only ever walked schema.Files, never schema.DB -- even though
// DBValidationRule.Checks is the identical JSONValidationCheck type, fed
// through the identical validateJSONCheck/validatePattern logic at runtime
// (pre_validation_db.go). A DB check could carry the exact same
// unsatisfiable value_type/pattern combination, an invalid regex, a bad
// JSONPath, or a malformed array_length consistency check, completely
// unguarded. All four now route through forEachSchemaCheck, which walks
// both rule kinds.
func TestSchemaValidatorsAlsoCoverDBChecks(t *testing.T) {
	t.Run("value_type_pattern", func(t *testing.T) {
		schema := &ValidationSchema{
			DB: []DBValidationRule{
				{
					Name: "landed measurements",
					SQL:  "SELECT reach_snapshot_table_updated FROM metrics",
					Checks: []JSONValidationCheck{
						{Path: "$.reach_snapshot_table_updated", ValueType: "boolean", Pattern: "^true$"},
					},
				},
			},
		}
		err := validateValueTypePatternCompatibility(schema)
		if err == nil {
			t.Fatal("expected the unsatisfiable value_type/pattern combination in a DB check to be rejected")
		}
		if !strings.Contains(err.Error(), "landed measurements") || !strings.Contains(err.Error(), "reach_snapshot_table_updated") {
			t.Fatalf("error does not name the offending DB rule/path: %v", err)
		}
	})

	t.Run("regex_pattern", func(t *testing.T) {
		schema := &ValidationSchema{
			DB: []DBValidationRule{
				{SQL: "SELECT handle FROM engagement_attribution", Checks: []JSONValidationCheck{
					{Path: "$.handle", Pattern: "(unterminated"},
				}},
			},
		}
		if err := validateRegexPatternsInSchema(schema); err == nil {
			t.Fatal("expected an invalid regex in a DB check to be rejected")
		}
	})

	t.Run("jsonpath_syntax", func(t *testing.T) {
		schema := &ValidationSchema{
			DB: []DBValidationRule{
				{SQL: "SELECT handle FROM engagement_attribution", Checks: []JSONValidationCheck{
					{Path: "not-a-jsonpath"},
				}},
			},
		}
		if err := validateJSONPathSyntax(schema); err == nil {
			t.Fatal("expected a malformed JSONPath in a DB check to be rejected")
		}
	})

	t.Run("array_length_consistency", func(t *testing.T) {
		schema := &ValidationSchema{
			DB: []DBValidationRule{
				{SQL: "SELECT * FROM action_queue", Checks: []JSONValidationCheck{
					{
						Path: "$.actions",
						ConsistencyCheck: &ConsistencyRule{
							Type:            "array_length",
							CompareWithPath: "$.actions", // identical to Path -- malformed
						},
					},
				}},
			},
		}
		if err := validateArrayLengthConsistencyChecks(schema); err == nil {
			t.Fatal("expected a malformed array_length consistency check in a DB check to be rejected")
		}
	})
}
