package step_based_workflow

import (
	"context"
	"fmt"
	"strings"
)

// Learnings access levels (mirror of knowledgebase_access).
const (
	LearningsAccessRead      = "read"
	LearningsAccessReadWrite = "read-write"
	LearningsAccessNone      = "none"
)

// resolveLearningsAccess returns the effective learnings_access for a step.
// Explicit value wins; empty falls back to auto-migration:
//   - learning_objective non-empty → "read-write" (preserves legacy behavior)
//   - learning_objective empty     → "read"       (new default — all steps see _global/)
//
// An explicit bad value is normalized to "read" with a warning at validation time.
func resolveLearningsAccess(agentConfigs *AgentConfigs) string {
	if agentConfigs == nil {
		return LearningsAccessRead
	}
	v := strings.TrimSpace(agentConfigs.LearningsAccess)
	switch v {
	case LearningsAccessRead, LearningsAccessReadWrite, LearningsAccessNone:
		return v
	case "":
		if strings.TrimSpace(agentConfigs.LearningObjective) != "" {
			return LearningsAccessReadWrite
		}
		return LearningsAccessRead
	default:
		return LearningsAccessRead
	}
}

// canReadLearnings reports whether this step's execution prompt should include
// the global SKILL.md content. Read is the default unless explicitly set to
// "none"; routing steps and evaluation-mode runs always skip to keep their
// prompts lean.
func canReadLearnings(agentConfigs *AgentConfigs, step PlanStepInterface, isEvalMode bool) bool {
	if isEvalMode || (step != nil && isRoutingStep(step)) {
		return false
	}
	return resolveLearningsAccess(agentConfigs) != LearningsAccessNone
}

// resolveExecutionLearningsAccess is the single capability decision used by
// both prompt injection and filesystem guards. Evaluation and deterministic
// routing steps intentionally consume no workflow learnings; returning none
// here prevents their shell access from being broader than their prompt.
func resolveExecutionLearningsAccess(agentConfigs *AgentConfigs, step PlanStepInterface, isEvalMode bool) string {
	if !canReadLearnings(agentConfigs, step, isEvalMode) {
		return LearningsAccessNone
	}
	return resolveLearningsAccess(agentConfigs)
}

// canWriteLearnings reports whether the step agent should run its direct
// post-completion learnings turn. Requires learnings_access == "read-write"
// AND a non-empty learning_objective (the extraction target for the writer).
// Routing and eval mode always skip.
func canWriteLearnings(agentConfigs *AgentConfigs, step PlanStepInterface, isEvalMode bool) bool {
	if isEvalMode || (step != nil && isRoutingStep(step)) {
		return false
	}
	if agentConfigs == nil {
		return false
	}
	if resolveLearningsAccess(agentConfigs) != LearningsAccessReadWrite {
		return false
	}
	return strings.TrimSpace(agentConfigs.LearningObjective) != ""
}

// shouldDirectWriteLearnings reports whether the step is configured for
// learnings writes. Since direct is now the only mode, this collapses to
// "is the step access+objective gate satisfied?". Kept as a named helper so
// the call sites still read intuitively.
func shouldDirectWriteLearnings(agentConfigs *AgentConfigs, step PlanStepInterface, isEvalMode bool) bool {
	return canWriteLearnings(agentConfigs, step, isEvalMode)
}

// PLAT-060. Ops-owned config decisions must carry the reason that justified
// them into step_config.json, which is what the *next* reviewer actually reads.
//
// llm_ops_review already owns tier, mode, and model selection, is read-only, and
// already produces "current state, exact suggestion, expected benefit, risk, and
// evidence" for every recommendation. That rationale lived only in the Pulse
// finding: the Fixer applied it through a tool call with no reason parameter, so
// the config recorded the change with no trace of why.
//
// The escape hatch matters as much as the requirement. A required field invites
// a confabulated answer from an agent that has already decided to act, and an
// invented justification is harder to challenge later than a missing one. So
// every message names the sanctioned alternative: raise a decision with
// create_human_input_request and park the finding awaiting_user. Uncertainty is
// a legitimate terminal state.
const reasonEscapeHatch = " If the evidence does not settle it, do not make the change: raise a decision with create_human_input_request and park the finding awaiting_user. An invented reason is worse than no change."

// validateExecutionTierChange rejects a tier override that states no reason.
// Naming the adaptive-tiering opt-out here is the point — it is the consequence
// the caller is least likely to know about, and this is the last moment they can
// reconsider.
func validateExecutionTierChange(tier string, reason string) error {
	if strings.TrimSpace(tier) == "" || strings.TrimSpace(reason) != "" {
		return nil
	}
	return fmt.Errorf("execution_tier=%q requires execution_tier_reason: pinning the tier also DISABLES adaptive tiering for this step, so it no longer promotes high→medium automatically after 3 stable runs — that cost decision needs a justification a later reviewer can judge. Cite the owning llm_ops_review finding (and the human_input_id if it was approved), the current state, and the evidence.%s",
		strings.TrimSpace(tier), reasonEscapeHatch)
}

// validateExecutionLLMChange rejects a model pin that states no reason. A pin
// outranks tier entirely, so it silently overrides every tier decision above it.
func validateExecutionLLMChange(pinned bool, reason string) error {
	if !pinned || strings.TrimSpace(reason) != "" {
		return nil
	}
	return fmt.Errorf("execution_llm requires execution_llm_reason: a model pin outranks execution_tier entirely, so it silently overrides every tier decision above it and will not follow provider-profile updates. Cite the owning llm_ops_review finding (and the human_input_id if it was approved), the current model, and the capability/cost comparison that justified the pin.%s",
		reasonEscapeHatch)
}

// validateDeclaredExecutionModeChange rejects a scripted/agentic flip that
// states no reason. The field already existed as an optional audit trail — "not
// consumed by Go runtime, but preserved so future Pulse and plan-change
// reviewers reading step_config.json see the original rationale" — which is
// exactly the contract; it was simply never enforced.
func validateDeclaredExecutionModeChange(mode string, reason string) error {
	if strings.TrimSpace(mode) == "" || strings.TrimSpace(reason) != "" {
		return nil
	}
	return fmt.Errorf("declared_execution_mode=%q requires declared_execution_mode_reason: moving a step between scripted and agentic changes how it executes for every future run — scripted freezes the behaviour into main.py, agentic pays for judgment on every run. State what makes this step deterministic (or not), citing the owning finding and the evidence.%s",
		strings.TrimSpace(mode), reasonEscapeHatch)
}

// findStepInPlan recursively finds a step by ID in the plan structure
// LoadGlobalLearningHistory loads and formats the global workflow-level learning history.
// Returns empty string if no global learnings found or on error.
func (hcpo *StepBasedWorkflowOrchestrator) LoadGlobalLearningHistory(
	ctx context.Context,
) (string, error) {
	globalLearningsPath := hcpo.getLearningsBasePath() + "/" + GlobalLearningID

	// Read learning files from global folder
	learningFiles, err := hcpo.readStepLearningFiles(ctx, globalLearningsPath)
	if err != nil {
		hcpo.GetLogger().Warn(fmt.Sprintf("⚠️ Failed to read global learning files from %s: %v - proceeding without global learnings", globalLearningsPath, err))
		return "", nil
	}

	if len(learningFiles) == 0 {
		return "", nil
	}

	// Format learnings for system prompt
	formattedLearnings, _ := hcpo.formatStepLearningFilesAsHistory(learningFiles)
	hcpo.GetLogger().Info(fmt.Sprintf("✅ Loaded %d global learning file(s) for execution agent system prompt", len(learningFiles)))

	return formattedLearnings, nil
}
