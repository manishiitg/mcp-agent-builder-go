package step_based_workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	loggerv2 "github.com/manishiitg/mcpagent/logger/v2"
)

const evaluationPlanRelPath = "evaluation/evaluation_plan.json"

// evaluationPlanEditableFields are the step fields update_evaluation_plan may
// set. Everything else on a step is preserved untouched.
var evaluationPlanEditableFields = []string{
	"title",
	"description",
	"context_output",
	"context_dependencies",
	"max_score",
	"applies_to_routes",
	"validation_schema",
	"pre_validation",
	"db_write",
}

// UpdateEvaluationPlanStep edits one step in evaluation/evaluation_plan.json and
// records the change in planning/changelog.
//
// It edits the decoded JSON rather than the EvaluationStep struct, because the
// struct is not a faithful mirror of the file: social-media's plan carries
// max_score and context_dependencies on every step and EvaluationStep has
// neither, so a read-modify-write through it would delete both from any step it
// touched — silently, producing valid JSON. max_score is the subject of four of
// that workflow's open findings, so a struct-based tool would have deepened the
// exact problem it was built to fix. Editing the map preserves every key it does
// not recognize, including fields added after this code was written.
//
// This is also why the eval plan had no tool at all until now, and therefore no
// changelog entries: Artifact Review reads planning/changelog to detect drift,
// and social-media's newest eval-step entry was 2026-05-29 while git showed four
// later evaluation-plan commits (AR-20260729-2).
func UpdateEvaluationPlanStep(
	ctx context.Context,
	workspacePath string,
	stepID string,
	updates map[string]interface{},
	reason string,
	readFile func(context.Context, string) (string, error),
	writeFile func(context.Context, string, string) error,
	logger loggerv2.Logger,
) (string, error) {
	stepID = strings.TrimSpace(stepID)
	reason = strings.TrimSpace(reason)
	if stepID == "" {
		return "", fmt.Errorf("step_id is required")
	}
	if reason == "" {
		return "", fmt.Errorf("reason is required: the changelog records why an evaluation step changed, and drift review cannot judge a change without it")
	}
	if len(updates) == 0 {
		return "", fmt.Errorf("no fields to update; provide at least one of %s", strings.Join(evaluationPlanEditableFields, ", "))
	}
	allowed := map[string]bool{}
	for _, field := range evaluationPlanEditableFields {
		allowed[field] = true
	}
	unknown := make([]string, 0, len(updates))
	for field := range updates {
		if !allowed[field] {
			unknown = append(unknown, field)
		}
	}
	if len(unknown) > 0 {
		sort.Strings(unknown)
		return "", fmt.Errorf("cannot set %s; editable fields are %s", strings.Join(unknown, ", "), strings.Join(evaluationPlanEditableFields, ", "))
	}

	// validation_schema/pre_validation are accepted here as raw decoded JSON
	// (see the function doc for why this tool edits the map, not a struct),
	// which meant they never passed through the same schema validators every
	// other schema-writing tool in this file does -- PLAT-236 found this only
	// after the exact unsatisfiable value_type/pattern shape it fixed
	// elsewhere could still be authored through this specific tool
	// untouched. Validate both by round-tripping through ValidationSchema.
	for _, field := range []string{"validation_schema", "pre_validation"} {
		raw, ok := updates[field]
		if !ok {
			continue
		}
		if err := validateSchemaLikeUpdateField(field, raw); err != nil {
			return "", err
		}
	}

	path := strings.Trim(strings.TrimSpace(workspacePath), "/") + "/" + evaluationPlanRelPath
	raw, err := readFile(ctx, path)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", evaluationPlanRelPath, err)
	}
	if strings.TrimSpace(raw) == "" {
		return "", fmt.Errorf("%s is empty or missing", evaluationPlanRelPath)
	}

	// Decode into generic containers so unrecognized keys survive the write.
	var document map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &document); err != nil {
		return "", fmt.Errorf("parse %s: %w", evaluationPlanRelPath, err)
	}
	stepsKey := "steps"
	rawSteps, ok := document[stepsKey].([]interface{})
	if !ok {
		// The plan also loads from an "eval_steps" key; honor whichever this
		// file actually uses rather than rewriting it into the other shape.
		if alt, altOK := document["eval_steps"].([]interface{}); altOK {
			stepsKey, rawSteps, ok = "eval_steps", alt, true
		}
	}
	if !ok {
		return "", fmt.Errorf("%s has no steps array", evaluationPlanRelPath)
	}

	changes := make([]PlanFieldChange, 0, len(updates))
	found := false
	for _, entry := range rawSteps {
		step, isObject := entry.(map[string]interface{})
		if !isObject {
			continue
		}
		if id, _ := step["id"].(string); strings.TrimSpace(id) != stepID {
			continue
		}
		found = true
		fields := make([]string, 0, len(updates))
		for field := range updates {
			fields = append(fields, field)
		}
		sort.Strings(fields)
		for _, field := range fields {
			previous, existed := step[field]
			next := updates[field]
			if existed && jsonEqual(previous, next) {
				continue
			}
			step[field] = next
			if !existed {
				previous = nil
			}
			changes = append(changes, PlanFieldChange{
				StepID:   stepID,
				Field:    field,
				OldValue: previous,
				NewValue: next,
			})
		}
		break
	}
	if !found {
		return "", fmt.Errorf("no evaluation step with id %q", stepID)
	}
	if len(changes) == 0 {
		return fmt.Sprintf("No change: evaluation step %q already holds these values.", stepID), nil
	}

	document[stepsKey] = rawSteps
	encoded, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		return "", fmt.Errorf("encode %s: %w", evaluationPlanRelPath, err)
	}
	// Prove the result still loads as an evaluation plan before writing it. A
	// tool that writes a file the runtime cannot read is worse than one that
	// refuses.
	var check EvaluationPlan
	if err := json.Unmarshal(encoded, &check); err != nil {
		return "", fmt.Errorf("refusing to write: the edited plan no longer parses as an evaluation plan: %w", err)
	}
	if len(check.Steps) != len(rawSteps) {
		return "", fmt.Errorf("refusing to write: the edited plan parses to %d steps, expected %d", len(check.Steps), len(rawSteps))
	}
	if err := writeFile(ctx, path, string(encoded)); err != nil {
		return "", fmt.Errorf("write %s: %w", evaluationPlanRelPath, err)
	}

	logPlanChange(ctx, workspacePath, PlanChangelogEntry{
		Tool:    "update_evaluation_plan",
		Reason:  reason,
		StepIDs: []string{stepID},
		Changes: changes,
	}, readFile, writeFile, logger)

	changed := make([]string, 0, len(changes))
	for _, change := range changes {
		changed = append(changed, change.Field)
	}
	return fmt.Sprintf("Updated evaluation step %q (%s) and recorded it in planning/changelog.", stepID, strings.Join(changed, ", ")), nil
}

// validateSchemaLikeUpdateField round-trips a raw decoded validation_schema
// or pre_validation value through the shared ValidationSchema struct so it
// gets the same write-time checks every other schema-writing tool applies:
// regex pattern validity, JSONPath syntax, array_length consistency, and
// value_type/pattern compatibility (PLAT-236).
func validateSchemaLikeUpdateField(field string, raw interface{}) error {
	encoded, err := json.Marshal(raw)
	if err != nil {
		return fmt.Errorf("%s: %w", field, err)
	}
	var schema ValidationSchema
	if err := json.Unmarshal(encoded, &schema); err != nil {
		return fmt.Errorf("%s does not match the expected schema shape: %w", field, err)
	}
	if err := validateRegexPatternsInSchema(&schema); err != nil {
		return fmt.Errorf("%s has invalid regex patterns: %w", field, err)
	}
	if err := validateJSONPathSyntax(&schema); err != nil {
		return fmt.Errorf("%s has invalid JSONPath syntax: %w", field, err)
	}
	if err := validateArrayLengthConsistencyChecks(&schema); err != nil {
		return fmt.Errorf("%s has invalid array_length consistency checks: %w", field, err)
	}
	if err := validateValueTypePatternCompatibility(&schema); err != nil {
		return fmt.Errorf("%s has an invalid schema: %w", field, err)
	}
	return nil
}

// jsonEqual compares two decoded JSON values so an update that sets a field to
// what it already holds is not recorded as a change.
func jsonEqual(a, b interface{}) bool {
	left, leftErr := json.Marshal(a)
	right, rightErr := json.Marshal(b)
	if leftErr != nil || rightErr != nil {
		return false
	}
	return string(left) == string(right)
}

func getUpdateEvaluationPlanSchema() string {
	return `{
  "type": "object",
  "additionalProperties": false,
  "properties": {
    "step_id": {"type": "string", "description": "ID of the evaluation step to update."},
    "reason": {"type": "string", "description": "Why this change is being made. Recorded in planning/changelog; drift review cannot judge a change without it."},
    "title": {"type": "string"},
    "description": {"type": "string"},
    "context_output": {"type": "string", "description": "Filename this step writes."},
    "context_dependencies": {"type": "array", "items": {"type": "string"}},
    "max_score": {"type": "integer", "description": "Score scale for this step. Steps without one cannot be compared run to run."},
    "db_write": {"type": "boolean"},
    "validation_schema": {"type": "object", "description": "Validation schema for this step's OWN OUTPUT, checked after it runs."},
    "pre_validation": {"type": "object", "description": "Files required to exist BEFORE this step runs, e.g. {\"files\":[{\"file_name\":\"...\",\"must_exist\":true}]}. Distinct from validation_schema, which checks this step's own output afterward. Gate route/producer alignment against this together with applies_to_routes so a step's evidence requirement never outlives the route that actually produces it."},
    "applies_to_routes": {
      "type": "array",
      "description": "Gate this step to specific routes chosen by a routing step. Omit to run it on every route.",
      "items": {
        "type": "object",
        "additionalProperties": false,
        "properties": {
          "routing_step_id": {"type": "string"},
          "route_ids": {"type": "array", "items": {"type": "string"}}
        },
        "required": ["routing_step_id", "route_ids"]
      }
    }
  },
  "required": ["step_id", "reason"]
}`
}

func parseSchemaForToolParametersMust(schema string) map[string]interface{} {
	parsed, err := parseSchemaForToolParameters(schema)
	if err != nil {
		// The schema is a compile-time constant in this file; a parse failure is
		// a code defect, and an empty surface is clearer than a silent partial.
		return map[string]interface{}{"type": "object"}
	}
	return parsed
}

func createUpdateEvaluationPlanExecutor(
	workspacePath string,
	logger loggerv2.Logger,
	readFile func(context.Context, string) (string, error),
	writeFile func(context.Context, string, string) error,
) func(context.Context, map[string]interface{}) (string, error) {
	return func(ctx context.Context, args map[string]interface{}) (string, error) {
		stepID, _ := args["step_id"].(string)
		reason, _ := args["reason"].(string)
		updates := map[string]interface{}{}
		for _, field := range evaluationPlanEditableFields {
			if value, ok := args[field]; ok && value != nil {
				updates[field] = value
			}
		}
		return UpdateEvaluationPlanStep(ctx, workspacePath, stepID, updates, reason, readFile, writeFile, logger)
	}
}
