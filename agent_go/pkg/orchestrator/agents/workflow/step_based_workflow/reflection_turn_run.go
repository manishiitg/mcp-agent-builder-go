package step_based_workflow

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/common"
	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/orchestrator/agents"
	"github.com/manishiitg/multi-llm-provider-go/llmtypes"
)

// PLAT-055. Runs the merged reflection turn that replaced the KB turn followed
// by the direct-learnings turn.
//
// Everything the two turns did is preserved: the shared-file mutex, the
// continuation-phase bookkeeping, both freshness ledgers, the canonical
// artifact-change log, and learning metadata. What changes is that they now
// happen once, with every store the agent must route to reachable at the same
// moment.

type stepReflectionTurnResult struct {
	Summary  string
	History  []llmtypes.MessageContent
	Executed bool
}

// runStepReflectionTurn fires the merged post-completion turn for a step.
//
// Best-effort by contract, like the turns it replaces: a step that did its work
// and passed validation is never failed because reflection failed. Errors are
// reported through the returned summary and the continuation ledger.
func (hcpo *StepBasedWorkflowOrchestrator) runStepReflectionTurn(
	ctx context.Context,
	step PlanStepInterface,
	stepIndex int,
	stepPath string,
	artifactStepID string,
	artifactStepPath string,
	stepConfig *AgentConfigs,
	executionAgent agents.OrchestratorAgent,
	history []llmtypes.MessageContent,
	isScriptedMode bool,
	turnCount int,
	executionLLM string,
) stepReflectionTurnResult {
	result := stepReflectionTurnResult{History: history}
	if executionAgent == nil {
		return result
	}

	writesLearnings := shouldDirectWriteLearnings(stepConfig, step, hcpo.isEvaluationMode)
	kbAccess := resolveKnowledgebaseAccess(stepConfig, hcpo.UseKnowledgebase())
	kbContribution := kbContributionForPrompt(stepConfig)
	writesKB := kbAccessAllowsWrite(kbAccess) && strings.TrimSpace(kbContribution) != ""

	// lock_learnings suppresses only the learnings half. A locked step can still
	// route to KB and still report a defect — silencing those too was never the
	// intent of a learnings freeze.
	skipLearningsDueToLock := false
	if writesLearnings {
		skipLearningsDueToLock = hcpo.shouldSkipDirectLearningsDueToLock(ctx, stepConfig, stepIndex)
		if skipLearningsDueToLock {
			hcpo.recordWorkflowContinuationPhase(ctx, artifactStepID, artifactStepPath,
				workflowContinuationOwnerStepExecution, workflowContinuationPhaseDirectLearning,
				workflowContinuationStatusSkipped, "lock_learnings=true with existing _global content", executionAgent)
		}
	}
	effectiveLearnings := writesLearnings && !skipLearningsDueToLock

	baseWorkspace := hcpo.GetWorkspacePath()
	globalLearningsPath := fmt.Sprintf("%s/learnings/%s", baseWorkspace, GlobalLearningID)
	kbNotesPath := fmt.Sprintf("%s/%s/%s", baseWorkspace, KnowledgebaseFolderName, KBNotesFolderName)

	learnObjective := ""
	if stepConfig != nil {
		learnObjective = stepConfig.LearningObjective
	}
	if !effectiveLearnings {
		learnObjective = ""
	}

	input := StepReflectionTurnInput{
		StepID:              step.GetID(),
		StepDescription:     step.GetDescription(),
		LearningObjective:   learnObjective,
		LearningsTargetPath: filepath.Join(GetPromptDocsRoot(), baseWorkspace, "learnings", GlobalLearningID),
		KBNotesTargetPath:   filepath.Join(GetPromptDocsRoot(), baseWorkspace, KnowledgebaseFolderName, KBNotesFolderName),
		HasBrowserAccess:    hcpo.HasBrowserCapability(),
		DBTableNames:        hcpo.reflectionDBTableNames(ctx),
	}
	if writesKB {
		input.KBAccess = kbAccess
		input.KBContribution = kbContribution
	}
	if effectiveLearnings {
		input.SkillIndexLines = hcpo.reflectionSkillIndexLines(ctx)
	}

	message := BuildStepReflectionTurn(input)
	if strings.TrimSpace(message) == "" {
		return result
	}

	// Widen only for this turn. Main execution had write access to none of
	// these paths, and the outer step defer restores session shell config.
	addedPaths := []string{globalLearningsPath}
	if writesKB {
		addedPaths = append(addedPaths, kbNotesPath)
	}

	hcpo.recordWorkflowContinuationPhase(ctx, artifactStepID, artifactStepPath,
		workflowContinuationOwnerStepExecution, workflowContinuationPhaseDirectLearning,
		workflowContinuationStatusWaitingForLock, "", executionAgent)

	func() {
		restore := hcpo.prepareDirectLearningTurn(executionAgent, addedPaths)
		defer restore()

		if cfg := executionAgent.GetConfig(); cfg != nil && strings.TrimSpace(cfg.MCPSessionID) != "" {
			// Concerns raised from here are attributed to the reflection phase,
			// which is what tells a reviewer the contradiction was found while
			// reconciling stores rather than during the task itself.
			common.SetRunConcernSessionPhase(strings.TrimSpace(cfg.MCPSessionID), ConcernPhaseLearnings)
			hcpo.GetLogger().Info(fmt.Sprintf("🔓 [REFLECT] Widened sub-agent session %s for reflection turn on step %s: +%v",
				strings.TrimSpace(cfg.MCPSessionID), step.GetID(), addedPaths))
		}

		// _global/SKILL.md is shared across steps, so parallel reflection turns
		// would race on diff_patch without this.
		learningsGlobalFileMutex.Lock()
		defer learningsGlobalFileMutex.Unlock()

		beforeRef := hcpo.snapshotCanonicalArtifactRef(ctx, globalLearningsPath)
		hcpo.recordWorkflowContinuationPhase(ctx, artifactStepID, artifactStepPath,
			workflowContinuationOwnerStepExecution, workflowContinuationPhaseDirectLearning,
			workflowContinuationStatusRunning, "", executionAgent)
		hcpo.GetLogger().Info(fmt.Sprintf("🧠 Reflection turn for step %d (learnings=%t kb=%t objective_len=%d)",
			stepIndex+1, effectiveLearnings, writesKB, len(learnObjective)))

		ba := executionAgent.GetBaseAgent()
		if ba == nil {
			return
		}
		turnResult, updatedHistory, turnErr := hcpo.withWorkshopMessageTarget(ctx, step.GetID(), "reflection", executionAgent,
			func() (string, []llmtypes.MessageContent, error) {
				return ba.Execute(ctx, message, history, "", false)
			})

		afterRef := hcpo.snapshotCanonicalArtifactRef(context.Background(), globalLearningsPath)
		if afterRef != beforeRef {
			LogCanonicalArtifactChange(context.Background(), hcpo.GetWorkspacePath(), "runtime_learning_update",
				"Step post-completion turn changed reusable runtime guidance.",
				[]PlanFieldChange{{StepID: step.GetID(), Field: "artifact_tree", OldValue: beforeRef, NewValue: afterRef}},
				hcpo.ReadWorkspaceFile, hcpo.WriteWorkspaceFile, hcpo.GetLogger(),
				"", nil, nil)
		}

		if turnErr != nil {
			hcpo.GetLogger().Warn(fmt.Sprintf("⚠️ Reflection turn failed for step %d: %v (accepting step anyway)", stepIndex+1, turnErr))
			result.Summary = fmt.Sprintf("CONCERNS: step reflection turn failed for this run: %v\nSTATUS: COMPLETED", turnErr)
			hcpo.recordWorkflowContinuationPhase(context.Background(), artifactStepID, artifactStepPath,
				workflowContinuationOwnerStepExecution, workflowContinuationPhaseDirectLearning,
				workflowContinuationStatusFailed, turnErr.Error(), executionAgent)
			return
		}

		result.Summary = summarizeExecutionResultForNotification(turnResult)
		result.History = updatedHistory
		result.Executed = true
		hcpo.recordWorkflowContinuationPhase(context.Background(), artifactStepID, artifactStepPath,
			workflowContinuationOwnerStepExecution, workflowContinuationPhaseDirectLearning,
			workflowContinuationStatusCompleted, "", executionAgent)

		// Freshness ledgers are code-owned and best-effort. Both stores were
		// genuinely reviewed in this turn when the step writes to them, so both
		// are confirmed here — previously each turn confirmed its own.
		hasNewLearning, reasoning, confidence := inferHasNewLearningFromResult(turnResult)
		if writesKB {
			if err := hcpo.recordKnowledgebaseConfirmation(context.Background(), hcpo.selectedRunFolder, step.GetID()); err != nil {
				hcpo.GetLogger().Warn(fmt.Sprintf("⚠️ Failed to record knowledgebase freshness for step %s: %v", step.GetID(), err))
			}
		}
		if effectiveLearnings {
			if err := hcpo.recordLearningsConfirmation(context.Background(), hcpo.selectedRunFolder, step.GetID(), hasNewLearning); err != nil {
				hcpo.GetLogger().Warn(fmt.Sprintf("⚠️ Failed to record learnings freshness for step %s: %v", step.GetID(), err))
			}
			pathIdentifier := getEffectiveLearningPathIdentifier(step.GetID(), stepPath, stepConfig)
			if err := hcpo.updateLearningMetadataWithTurnCount(
				ctx, stepIndex, stepPath, pathIdentifier,
				hasNewLearning, reasoning, confidence, turnCount, step,
				true, // pre-validation already passed
				executionLLM, executionLLM,
			); err != nil {
				hcpo.GetLogger().Warn(fmt.Sprintf("⚠️ Failed to update reflection learning metadata for step %s: %v", step.GetID(), err))
			}
		}
	}()

	return result
}

// reflectionDBTableNames lists the workflow's tables so the routing rule can
// name real destinations. Best-effort: an unavailable database simply omits the
// list rather than blocking reflection.
func (hcpo *StepBasedWorkflowOrchestrator) reflectionDBTableNames(ctx context.Context) []string {
	names, err := LoadWorkflowDBTableNames(ctx, hcpo.GetWorkspacePath())
	if err != nil {
		hcpo.GetLogger().Info(fmt.Sprintf("ℹ️ Reflection turn omitting DB table list: %v", err))
		return nil
	}
	return names
}

// reflectionLearningsSizes reports the current index size and the step's own
// file, so the prompt can state the gap instead of asserting a budget nobody
// measures.
func (hcpo *StepBasedWorkflowOrchestrator) reflectionSkillIndexLines(ctx context.Context) int {
	base := fmt.Sprintf("%s/learnings/%s", hcpo.GetWorkspacePath(), GlobalLearningID)
	content, err := hcpo.ReadWorkspaceFile(ctx, base+"/SKILL.md")
	if err != nil {
		return 0
	}
	return len(strings.Split(strings.TrimRight(content, "\n"), "\n"))
}
