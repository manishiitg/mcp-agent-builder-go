package step_based_workflow

import (
	"encoding/json"
	"testing"
)

func TestBuildStepOutputContentWrapsFlatJSON(t *testing.T) {
	out := buildStepOutputContent("evaluation/runs/iteration-0/default/execution/eval-step/output_content.json", `{"score":7,"follows_today_count":17}`)
	if out == nil {
		t.Fatal("expected output content")
	}
	if out.FilePath != "evaluation/runs/iteration-0/default/execution/eval-step/output_content.json" {
		t.Fatalf("unexpected file path: %q", out.FilePath)
	}
	if !out.IsJSON {
		t.Fatal("expected JSON output")
	}
	content, ok := out.Content.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map content, got %T", out.Content)
	}
	if content["score"] != float64(7) {
		t.Fatalf("expected score 7, got %#v", content["score"])
	}
	if content["follows_today_count"] != float64(17) {
		t.Fatalf("expected follows_today_count 17, got %#v", content["follows_today_count"])
	}
}

func TestBuildStepOutputContentUnwrapsLegacyEnvelope(t *testing.T) {
	out := buildStepOutputContent("output_content.json", `{"file_path":"old","is_json":true,"content":{"score":3,"verified_count":2}}`)
	if out == nil {
		t.Fatal("expected output content")
	}
	if !out.IsJSON {
		t.Fatal("expected JSON output")
	}
	content, ok := out.Content.(map[string]interface{})
	if !ok {
		t.Fatalf("expected unwrapped map content, got %T", out.Content)
	}
	if _, nested := content["content"]; nested {
		t.Fatalf("expected legacy envelope to be unwrapped, got %#v", content)
	}
	if content["verified_count"] != float64(2) {
		t.Fatalf("expected verified_count 2, got %#v", content["verified_count"])
	}
}

func TestBuildStepOutputContentKeepsTextOutput(t *testing.T) {
	out := buildStepOutputContent("output.txt", "plain result")
	if out == nil {
		t.Fatal("expected output content")
	}
	if out.IsJSON {
		t.Fatal("expected non-JSON output")
	}
	if out.Content != "plain result" {
		t.Fatalf("unexpected content: %#v", out.Content)
	}
}

func TestEvaluationOutputContentCandidatesPreferDeclaredEvalArtifacts(t *testing.T) {
	step := &EvaluationStep{
		ID: "eval-variety-coverage",
		PreValidation: &ValidationSchema{Files: []FileValidationRule{{
			FileName:  "eval_result.json",
			MustExist: true,
		}}},
	}
	candidates := evaluationOutputContentCandidates("evaluation/runs/iteration-0/test-run/execution", "eval-variety-coverage", step)

	expected := []string{
		"evaluation/runs/iteration-0/test-run/execution/eval-variety-coverage/output_content.json",
		"evaluation/runs/iteration-0/test-run/execution/eval-variety-coverage/context_output.json",
		"evaluation/runs/iteration-0/test-run/execution/eval-variety-coverage/eval_result.json",
	}
	if len(candidates) != len(expected) {
		t.Fatalf("expected %d candidates, got %d: %#v", len(expected), len(candidates), candidates)
	}
	for i := range expected {
		if candidates[i] != expected[i] {
			t.Fatalf("candidate[%d]: expected %q, got %q", i, expected[i], candidates[i])
		}
	}
}

func TestEvaluationStepDefaultsContextOutput(t *testing.T) {
	step := &EvaluationStep{ID: "eval-empty-output"}
	if got := step.GetContextOutput().String(); got != defaultEvaluationContextOutput {
		t.Fatalf("expected default evaluation output %q, got %q", defaultEvaluationContextOutput, got)
	}
	if got := step.GetCommonFields().ContextOutput.String(); got != defaultEvaluationContextOutput {
		t.Fatalf("expected common fields to use default evaluation output %q, got %q", defaultEvaluationContextOutput, got)
	}
}

func TestIsValidationSchemaLikeJSON(t *testing.T) {
	schema := `{"files":[{"file_name":"eval_result.json","must_exist":true,"json_checks":[{"path":"$.score","must_exist":true}]}]}`
	if !isValidationSchemaLikeJSON(schema) {
		t.Fatal("expected validation schema stub to be detected")
	}
	result := `{"score":1,"category_distinct_30d":6}`
	if isValidationSchemaLikeJSON(result) {
		t.Fatal("did not expect normal result JSON to be treated as validation schema")
	}
}

func TestEvaluationStepLoadsCanonicalValidationSchema(t *testing.T) {
	var step EvaluationStep
	err := json.Unmarshal([]byte(`{
		"id":"eval-result",
		"title":"Evaluate result",
		"description":"Check the result",
		"validation_schema":{"files":[{"file_name":"eval_result.json","must_exist":true}]}
	}`), &step)
	if err != nil {
		t.Fatalf("unmarshal evaluation step: %v", err)
	}
	if step.PreValidation == nil || len(step.PreValidation.Files) != 1 {
		t.Fatalf("expected canonical validation_schema to load, got %#v", step.PreValidation)
	}
	if got := step.PreValidation.Files[0].FileName; got != "eval_result.json" {
		t.Fatalf("expected eval_result.json, got %q", got)
	}
	if step.GetValidationSchema() == nil {
		t.Fatal("expected validation schema to be available to execution")
	}
}

func TestEvaluationStepLoadsLegacyPreValidation(t *testing.T) {
	var step EvaluationStep
	err := json.Unmarshal([]byte(`{
		"id":"eval-result",
		"title":"Evaluate result",
		"description":"Check the result",
		"pre_validation":{"files":[{"file_name":"legacy_result.json","must_exist":true}]}
	}`), &step)
	if err != nil {
		t.Fatalf("unmarshal legacy evaluation step: %v", err)
	}
	if step.PreValidation == nil || len(step.PreValidation.Files) != 1 {
		t.Fatalf("expected legacy pre_validation to load, got %#v", step.PreValidation)
	}
	if got := step.PreValidation.Files[0].FileName; got != "legacy_result.json" {
		t.Fatalf("expected legacy_result.json, got %q", got)
	}
}

// TestExtractEvalVerdictFromOutputContentUsesRealProductionShape proves
// extraction against the actual shape found live in a real workflow run
// (social-media, iteration-202, eval-workflow-success) rather than an
// invented fixture: real eval steps use "pass_fail_reason", not the
// "reasoning" key the authoring guidance names, and carry no separate
// "evidence" key at all.
func TestExtractEvalVerdictFromOutputContentUsesRealProductionShape(t *testing.T) {
	raw := `{
		"cdp_connected": true,
		"follower_delta_7d": -2,
		"score": 0.0,
		"max_score": 10,
		"pass_fail_reason": "Score 0.0/10. Top deductions: -4: follower_delta_7d declining (-2); -3: bot_detection_risk signal ACTIVE (unresolved, within 3 days)."
	}`
	score := &EvaluationStepScore{
		StepID:        "eval-workflow-success",
		OutputContent: buildStepOutputContent("evaluation/runs/iteration-202/default/execution/eval-workflow-success/output_content.json", raw),
	}
	extractEvalVerdictFromOutputContent(score)

	if score.Score != 0 || !score.ScoreCaptured {
		t.Fatalf("Score = %v captured=%v, want captured zero", score.Score, score.ScoreCaptured)
	}
	if score.MaxScore != 10 {
		t.Fatalf("MaxScore = %v, want 10", score.MaxScore)
	}
	if score.Reasoning == "" || score.Reasoning == "No score captured — this eval step produced no output_content, or output_content had no score field." {
		t.Fatalf("Reasoning not extracted from pass_fail_reason, got %q", score.Reasoning)
	}
	if score.Evidence == "" {
		t.Fatal("Evidence should never be empty when a score was captured")
	}
	if score.Evidence != "See evaluation/runs/iteration-202/default/execution/eval-workflow-success/output_content.json for the extracted facts this score was computed from." {
		t.Fatalf("Evidence fallback wrong: %q", score.Evidence)
	}
}

// TestExtractEvalVerdictFromOutputContentPrefersExplicitReasoningAndEvidence
// proves a step that DOES follow the authoring guidance literally (using
// "reasoning"/"evidence" keys, per evaluation-plan.md) is read correctly too
// — pass_fail_reason is preferred only when present, never required.
func TestExtractEvalVerdictFromOutputContentPrefersExplicitReasoningAndEvidence(t *testing.T) {
	raw := `{"score":8,"max_score":10,"reasoning":"8 of 10 checks passed.","evidence":"db/db.sqlite rows 1-8 verified against source."}`
	score := &EvaluationStepScore{OutputContent: buildStepOutputContent("output_content.json", raw)}
	extractEvalVerdictFromOutputContent(score)

	if score.Score != 8 || score.MaxScore != 10 {
		t.Fatalf("Score/MaxScore = %v/%v, want 8/10", score.Score, score.MaxScore)
	}
	if score.Reasoning != "8 of 10 checks passed." {
		t.Fatalf("Reasoning = %q, want the explicit reasoning field", score.Reasoning)
	}
	if score.Evidence != "db/db.sqlite rows 1-8 verified against source." {
		t.Fatalf("Evidence = %q, want the explicit evidence field, not the fallback", score.Evidence)
	}
}

// TestExtractEvalVerdictFromOutputContentLeavesStubWhenNoScoreField proves a
// step whose output genuinely carries no score field leaves the "not
// captured" stub alone — 0 is never fabricated as a real score.
func TestExtractEvalVerdictFromOutputContentLeavesStubWhenNoScoreField(t *testing.T) {
	score := &EvaluationStepScore{
		Reasoning:     "No score captured — this eval step produced no output_content, or output_content had no score field.",
		Evidence:      "No output_content found for this step.",
		OutputContent: buildStepOutputContent("output_content.json", `{"some_fact": 42, "another_fact": "value"}`),
	}
	extractEvalVerdictFromOutputContent(score)

	if score.Score != 0 || score.MaxScore != 0 || score.ScoreCaptured {
		t.Fatalf("Score/MaxScore = %v/%v captured=%v, want uncaptured 0/0", score.Score, score.MaxScore, score.ScoreCaptured)
	}
	if score.Reasoning != "No score captured — this eval step produced no output_content, or output_content had no score field." {
		t.Fatalf("Reasoning stub was overwritten despite no score field being present: %q", score.Reasoning)
	}
	if score.Evidence != "No output_content found for this step." {
		t.Fatalf("Evidence stub was overwritten despite no score field being present: %q", score.Evidence)
	}
}

// TestExtractEvalVerdictFromOutputContentIgnoresNonJSONOutput proves a
// non-JSON eval output (e.g. plain text) is safely skipped, not partially
// parsed.
func TestExtractEvalVerdictFromOutputContentIgnoresNonJSONOutput(t *testing.T) {
	score := &EvaluationStepScore{OutputContent: buildStepOutputContent("output.txt", "plain result, not json")}
	extractEvalVerdictFromOutputContent(score) // must not panic
	if score.Score != 0 || score.Evidence != "" {
		t.Fatalf("non-JSON output should leave score fields untouched, got Score=%v Evidence=%q", score.Score, score.Evidence)
	}
}

// TestEvaluationStepScoreSerializesGenuineZeroScore reproduces a real
// production report bug (EVAL-20260729-1, social-media): a step whose real,
// legitimate verdict is a confirmed total failure — score 0 — had its "score"
// key vanish entirely from evaluation_report.json because the field carried
// `omitempty`, and Go's omitempty drops an int at its zero value. Once the
// key is gone, a genuine "confirmed failing" is indistinguishable from "no
// score captured", which is exactly the ambiguity extraction is meant to
// remove. A real score of 0 must always serialize.
func TestEvaluationStepScoreSerializesGenuineZeroScore(t *testing.T) {
	score := &EvaluationStepScore{
		StepID:        "eval-workflow-success",
		Score:         0,
		MaxScore:      10,
		ScoreCaptured: true,
		Reasoning:     "Score 0.0/10. Confirmed failing on real evidence.",
		Evidence:      "see output_content.json",
	}
	raw, err := json.Marshal(score)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded map[string]interface{}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("decode marshaled score: %v", err)
	}
	got, ok := decoded["score"]
	if !ok {
		t.Fatalf("expected \"score\" key to survive marshaling for a genuine 0, got %s", raw)
	}
	if got != float64(0) {
		t.Fatalf("expected score 0, got %v", got)
	}
	if decoded["score_captured"] != true {
		t.Fatalf("expected score_captured=true for genuine zero, got %s", raw)
	}
}

func TestExtractEvalVerdictPreservesFractionalScore(t *testing.T) {
	score := &EvaluationStepScore{OutputContent: buildStepOutputContent("output_content.json", `{"score":7.5,"max_score":10}`)}
	extractEvalVerdictFromOutputContent(score)
	if score.Score != 7.5 || score.MaxScore != 10 || !score.ScoreCaptured {
		t.Fatalf("fractional score = %v/%v captured=%v, want 7.5/10 captured", score.Score, score.MaxScore, score.ScoreCaptured)
	}
}

func TestEvaluationStepMarshalsCanonicalValidationSchema(t *testing.T) {
	step := &EvaluationStep{
		ID:            "eval-result",
		Title:         "Evaluate result",
		Description:   "Check the result",
		PreValidation: &ValidationSchema{Files: []FileValidationRule{{FileName: "eval_result.json", MustExist: true}}},
	}
	raw, err := json.Marshal(step)
	if err != nil {
		t.Fatalf("marshal evaluation step: %v", err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatalf("decode marshaled evaluation step: %v", err)
	}
	if _, ok := fields["validation_schema"]; !ok {
		t.Fatalf("expected canonical validation_schema in %s", raw)
	}
	if _, ok := fields["pre_validation"]; ok {
		t.Fatalf("did not expect legacy pre_validation in %s", raw)
	}
}
