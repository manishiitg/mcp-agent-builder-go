package step_based_workflow

import (
	"encoding/json"
	"strings"
)

const defaultEvaluationContextOutput = "context_output.json"

// EvaluationStep represents a single step in an evaluation plan.
// It implements PlanStepInterface to reuse existing execution infrastructure.
//
// Note: success_criteria has been removed. The eval step's description should
// fully encode what passing/failing looks like (deterministic checks via
// scripted, or LLM judgment grounded in the description).
type EvaluationStep struct {
	ID            string            `json:"id"`
	Title         string            `json:"title"`
	Description   string            `json:"description"`
	PreValidation *ValidationSchema `json:"validation_schema,omitempty"`
	AgentConfigs  *AgentConfigs     `json:"-"`                        // runtime config
	ContextOutput string            `json:"context_output,omitempty"` // Filename of output produced by the step
	// ExecutionMode is "scripted" (a persistent learnings/<id>/main.py replayed
	// each run) or "agentic" (the LLM judges each run); empty means agentic.
	// It lives on the eval step itself because an evaluation step has no
	// regular/message_sequence type to decide it (PLAT-287); set it with
	// update_evaluation_plan(execution_mode=...).
	ExecutionMode   string                         `json:"execution_mode,omitempty"`
	AppliesToRoutes []EvaluationRouteApplicability `json:"applies_to_routes,omitempty"`
	// DBWrite grants this evaluation step write access to db/. Read is always allowed.
	// Off by default: evaluation typically reads db/ to score against real state, and its
	// own writes stay in the sandbox run folder. Set true only for workflows where the eval
	// step is the canonical data producer (the builder prompt warns about this).
	// See docs/workflow/persistent_stores_design.md section 1.
	DBWrite bool `json:"db_write,omitempty"`
}

// EvaluationRouteApplicability gates an evaluation step to one or more selected
// routes from a routing step in the target workflow run. When set, the eval step
// only runs if the target run's routing-evaluation.json selected one of RouteIDs.
type EvaluationRouteApplicability struct {
	RoutingStepID string   `json:"routing_step_id"`
	RouteIDs      []string `json:"route_ids"`
}

// Implement PlanStepInterface for EvaluationStep

func (e *EvaluationStep) GetID() string                    { return e.ID }
func (e *EvaluationStep) GetTitle() string                 { return e.Title }
func (e *EvaluationStep) GetDescription() string           { return e.Description }
func (e *EvaluationStep) GetSuccessCriteria() string       { return "" } // dropped — see struct doc
func (e *EvaluationStep) GetContextDependencies() []string { return nil }
func (e *EvaluationStep) GetContextOutput() FlexibleContextOutput {
	if strings.TrimSpace(e.ContextOutput) == "" {
		return FlexibleContextOutput(defaultEvaluationContextOutput)
	}
	return FlexibleContextOutput(e.ContextOutput)
}
func (e *EvaluationStep) GetValidationSchema() *ValidationSchema { return e.PreValidation }
func (e *EvaluationStep) StepType() StepType                     { return StepTypeRegular }

func (e *EvaluationStep) GetCommonFields() CommonStepFields {
	return CommonStepFields{
		ID:               e.ID,
		Title:            e.Title,
		Description:      e.Description,
		ValidationSchema: e.PreValidation,
		ContextOutput:    e.GetContextOutput(),
	}
}

// UnmarshalJSON accepts the canonical validation_schema field while preserving
// compatibility with evaluation plans that still use pre_validation.
func (e *EvaluationStep) UnmarshalJSON(data []byte) error {
	type Alias EvaluationStep
	decoded := struct {
		*Alias
		LegacyPreValidation *ValidationSchema `json:"pre_validation,omitempty"`
	}{
		Alias: (*Alias)(e),
	}
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	if e.PreValidation == nil {
		e.PreValidation = decoded.LegacyPreValidation
	}
	return nil
}

// MarshalJSON ensures the type field is always set when marshaling (if needed by frontend)
func (e *EvaluationStep) MarshalJSON() ([]byte, error) {
	type Alias EvaluationStep
	return json.Marshal(&struct {
		Type StepType `json:"type"`
		*Alias
	}{
		Type:  StepTypeRegular,
		Alias: (*Alias)(e),
	})
}

// EvaluationPlan represents the structured evaluation plan
type EvaluationPlan struct {
	Steps []*EvaluationStep `json:"steps"`
}

// UnmarshalJSON implements custom unmarshaling for EvaluationPlan
// Handles multiple formats:
// 1. {"steps": [...]} - expected format
// 2. {"eval_steps": [...]} - alternate key format
// 3. [...] - legacy format (array at top level)
func (ep *EvaluationPlan) UnmarshalJSON(data []byte) error {
	// First, try to unmarshal as object with "steps" or "eval_steps" field
	var temp struct {
		Steps     []*EvaluationStep `json:"steps"`
		EvalSteps []*EvaluationStep `json:"eval_steps"`
	}
	if err := json.Unmarshal(data, &temp); err == nil {
		if temp.Steps != nil {
			ep.Steps = temp.Steps
			return nil
		}
		if temp.EvalSteps != nil {
			ep.Steps = temp.EvalSteps
			return nil
		}
	}

	// If that fails, try to unmarshal as a top-level array (legacy format)
	var stepsArray []*EvaluationStep
	if err := json.Unmarshal(data, &stepsArray); err != nil {
		return err
	}
	ep.Steps = stepsArray
	return nil
}

// ToPlanSteps converts EvaluationPlan steps to PlanStepInterface slice
func (ep *EvaluationPlan) ToPlanSteps() []PlanStepInterface {
	steps := make([]PlanStepInterface, len(ep.Steps))
	for i, step := range ep.Steps {
		steps[i] = step
	}
	return steps
}

// StepOutputContent represents the content of a step's output file
type StepOutputContent struct {
	FilePath string      `json:"file_path"`
	Content  interface{} `json:"content"`
	IsJSON   bool        `json:"is_json"`
}

// EvaluationStepScore is the per-step entry in evaluation_report.json. There
// is no separate scoring agent: each eval step already writes its own real
// score/max_score/reasoning (or, in real practice, "pass_fail_reason") into
// its own output_content per the step's validation_schema, and this struct's
// Score/MaxScore/Reasoning/Evidence fields are extracted directly from that
// output_content — see extractEvalVerdictFromOutputContent. They are the
// authoritative per-criterion verdict, not a legacy/placeholder value.
// step_title and success_criteria are intentionally absent — UI consumers can
// look them up by step_id from evaluation_plan.json (the plan is loaded next
// to the report by the same API endpoint).
type EvaluationStepScore struct {
	StepID string `json:"step_id"`
	// Score has no omitempty: a genuine score of exactly 0 (a confirmed total
	// failure — a real, legitimate value) must serialize as "score": 0. The
	// explicit ScoreCaptured bit distinguishes that value from a report stub
	// whose source output contained no score. Scores are float64 because JSON
	// evaluations may legitimately use fractional values.
	Score         float64            `json:"score"`
	MaxScore      float64            `json:"max_score,omitempty"`
	ScoreCaptured bool               `json:"score_captured"`
	Reasoning     string             `json:"reasoning"`
	Evidence      string             `json:"evidence"`
	Skipped       bool               `json:"skipped,omitempty"`
	ContextOutput string             `json:"context_output,omitempty"`
	OutputContent *StepOutputContent `json:"output_content,omitempty"`
}

// EvaluationReport captures eval step outputs for a target run. There is
// intentionally no combined/blended score across steps here (no
// TotalScore/MaxPossibleScore/ScorePercentage) — per soul.md, each step
// measures its own success criterion, and blending N independent criteria
// into one number is lossy: a workflow could look fine on average while its
// single most important criterion is failing, hidden inside the blend. Read
// each StepScores entry on its own; a UI wanting an overall state should
// apply a worst-case rule (any material criterion failing => overall short),
// not a weighted average.
type EvaluationReport struct {
	// EvaluationID is the immutable identity of this evaluation attempt. The
	// target run folder is display metadata only because iteration-0 rotates.
	EvaluationID    string                 `json:"evaluation_id,omitempty"`
	TargetRunFolder string                 `json:"target_run_folder"`
	GeneratedAt     string                 `json:"generated_at"`
	StepScores      []*EvaluationStepScore `json:"step_scores"`
}

// EvaluationReportFileName is the filename the Go report phase writes the assembled
// report to inside the eval run folder. Kept as a constant so the validation schema
// and the report writer use the same path.
const EvaluationReportFileName = "evaluation_report.json"

// BuildEvaluationReportValidationSchema returns a fixed pre-validation schema for the
// assembled evaluation report JSON. Same shape as any step's validation_schema, so it flows
// through the existing RunPreValidation engine. Validates per-step structure (score
// range 0-10, min text lengths for reasoning/evidence) and pins the step_scores array
// length to numSteps.
func BuildEvaluationReportValidationSchema(numSteps int) *ValidationSchema {
	intPtr := func(v int) *int { return &v }
	floatPtr := func(v float64) *float64 { return &v }

	checks := []JSONValidationCheck{
		{Path: "$.step_scores", MustExist: true, ValueType: "array",
			MinLength: intPtr(numSteps), MaxLength: intPtr(numSteps)},
		{Path: "$.step_scores[*].step_id", MustExist: true, ValueType: "string"},
		{Path: "$.step_scores[*].score", MustExist: true, ValueType: "number",
			MinValue: floatPtr(0), MaxValue: floatPtr(10)},
		{Path: "$.step_scores[*].reasoning", MustExist: true, ValueType: "string", MinLength: intPtr(20)},
		{Path: "$.step_scores[*].evidence", MustExist: true, ValueType: "string", MinLength: intPtr(10)},
	}

	return &ValidationSchema{
		Files: []FileValidationRule{{
			FileName:   EvaluationReportFileName,
			MustExist:  true,
			JSONChecks: checks,
		}},
	}
}
