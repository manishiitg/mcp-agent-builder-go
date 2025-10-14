package todo_optimization

import (
	"context"
	"fmt"
	"time"

	"mcp-agent/agent_go/internal/observability"
	"mcp-agent/agent_go/internal/utils"
	"mcp-agent/agent_go/pkg/orchestrator"
	"mcp-agent/agent_go/pkg/orchestrator/agents"
)

// TodoOptimizationOrchestrator manages the multi-agent optimization process
type TodoOptimizationOrchestrator struct {
	// Base orchestrator for common functionality
	*orchestrator.BaseOrchestrator

	// Sub-agents (created on-demand)
	refineAgent   agents.OrchestratorAgent
	critiqueAgent agents.OrchestratorAgent
}

// NewTodoOptimizationOrchestrator creates a new multi-agent optimization orchestrator
func NewTodoOptimizationOrchestrator(
	config *agents.OrchestratorAgentConfig,
	logger utils.ExtendedLogger,
	tracer observability.Tracer,
	eventBridge interface{},
) (*TodoOptimizationOrchestrator, error) {
	baseOrchestrator, err := orchestrator.NewBaseOrchestrator(
		config,
		logger,
		tracer,
		eventBridge,
		agents.TodoOptimizationAgentType,
		orchestrator.OrchestratorTypeWorkflow,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create base orchestrator: %w", err)
	}

	return &TodoOptimizationOrchestrator{
		BaseOrchestrator: baseOrchestrator,
	}, nil
}

// ExecuteRefinement orchestrates the iterative refinement process
func (too *TodoOptimizationOrchestrator) ExecuteRefinement(ctx context.Context, objective, workspacePath string) (string, error) {
	too.AgentTemplate.GetLogger().Infof("🔄 Starting iterative refinement for objective: %s", objective)
	too.AgentTemplate.GetLogger().Infof("📁 Using workspace path: %s", workspacePath)

	// Set objective and workspace path directly
	too.SetObjective(objective)
	too.SetWorkspacePath(workspacePath)

	maxIterations := 3 // Configurable max refinement iterations
	var finalRefinementResult string

	// Iterative refinement loop
	var previousCritiqueResult string
	for iteration := 1; iteration <= maxIterations; iteration++ {
		too.AgentTemplate.GetLogger().Infof("🔄 Refinement iteration %d/%d", iteration, maxIterations)

		// Step 1: Refine the todo list (use previous critique result for improvement)
		refinementResult, err := too.runRefinementPhase(ctx, objective, previousCritiqueResult, iteration)

		if err != nil {
			return "", fmt.Errorf("refinement iteration %d failed: %w", iteration, err)
		}

		// Step 2: Critique the refined output
		refinementPrompt := fmt.Sprintf("Critique the refined todo list output for iteration %d. Focus on factual accuracy, completeness, and alignment with the objective.", iteration)
		critiqueResult, err := too.runCritiquePhase(ctx, objective, refinementResult, refinementPrompt, iteration)

		if err != nil {
			return "", fmt.Errorf("critique iteration %d failed: %w", iteration, err)
		}

		// Step 3: Check if there's room for more improvement
		needsMoreImprovement, improvementReason, err := too.checkImprovementNeeded(ctx, critiqueResult)
		if err != nil {
			too.AgentTemplate.GetLogger().Warnf("⚠️ Improvement check failed: %v", err)
			needsMoreImprovement = true
			improvementReason = "Improvement check failed: " + err.Error()
		}

		too.AgentTemplate.GetLogger().Infof("🎯 Iteration %d improvement check: needs_more=%t, reason=%s", iteration, needsMoreImprovement, improvementReason)

		// Store the current refinement result
		finalRefinementResult = refinementResult

		// If critique is satisfied, exit the loop
		if !needsMoreImprovement {
			too.AgentTemplate.GetLogger().Infof("✅ Refinement iteration %d: Critique satisfied, refinement complete", iteration)
			break
		}

		// Store critique result for next iteration
		previousCritiqueResult = critiqueResult

		// If this is the last iteration, log warning
		if iteration == maxIterations {
			too.AgentTemplate.GetLogger().Warnf("⚠️ Max refinement iterations (%d) reached, returning last result", maxIterations)
		}
	}

	duration := time.Since(too.GetStartTime())
	too.AgentTemplate.GetLogger().Infof("✅ Iterative refinement completed in %v", duration)

	return finalRefinementResult, nil
}

// runRefinementPhase runs a single refinement iteration using the proper agent pattern
func (too *TodoOptimizationOrchestrator) runRefinementPhase(ctx context.Context, objective, previousCritiqueResult string, iteration int) (string, error) {
	too.AgentTemplate.GetLogger().Infof("🔧 Running refinement phase iteration %d", iteration)

	// Create TodoRefinePlannerAgent for refinement
	refineAgent, err := too.createRefineAgent()
	if err != nil {
		return "", fmt.Errorf("failed to create refine agent: %w", err)
	}

	// Prepare template variables with iteration context and previous critique
	templateVars := map[string]string{
		"Objective":        objective,
		"WorkspacePath":    too.GetWorkspacePath(),
		"iteration":        fmt.Sprintf("%d", iteration),
		"CritiqueFeedback": previousCritiqueResult,
	}

	// Execute refinement using the TodoRefinePlannerAgent
	refinementResult, err := refineAgent.Execute(ctx, templateVars, nil)
	if err != nil {
		return "", fmt.Errorf("refinement execution failed: %v", err)
	}

	too.AgentTemplate.GetLogger().Infof("✅ Refinement phase iteration %d completed: %d characters", iteration, len(refinementResult))
	return refinementResult, nil
}

// runCritiquePhase runs a single critique iteration using the proper agent pattern
func (too *TodoOptimizationOrchestrator) runCritiquePhase(ctx context.Context, objective, inputData, inputPrompt string, iteration int) (string, error) {
	too.AgentTemplate.GetLogger().Infof("🔍 Running critique phase iteration %d", iteration)

	// Create DataCritiqueAgent for critique
	critiqueAgent, err := too.createCritiqueAgent()
	if err != nil {
		return "", fmt.Errorf("failed to create critique agent: %w", err)
	}

	// Prepare template variables
	templateVars := map[string]string{
		"objective":          objective,
		"input_data":         inputData,
		"input_prompt":       inputPrompt,
		"refinement_history": "No refinement history available for first iteration",
		"iteration":          fmt.Sprintf("%d", iteration),
	}

	// Execute critique using the DataCritiqueAgent
	critiqueResult, err := critiqueAgent.Execute(ctx, templateVars, nil)
	if err != nil {
		return "", fmt.Errorf("critique execution failed: %v", err)
	}

	too.AgentTemplate.GetLogger().Infof("✅ Critique phase iteration %d completed: %d characters", iteration, len(critiqueResult))
	return critiqueResult, nil
}

// checkImprovementNeeded uses conditional logic to determine if there's room for more improvement
func (too *TodoOptimizationOrchestrator) checkImprovementNeeded(ctx context.Context, critiqueResult string) (bool, string, error) {
	too.AgentTemplate.GetLogger().Infof("🎯 Checking if more improvement is needed based on critique")

	if too.GetConditionalLLM() == nil {
		too.AgentTemplate.GetLogger().Errorf("❌ Conditional LLM not initialized")
		return false, "Conditional LLM not initialized", fmt.Errorf("conditional LLM not initialized")
	}

	// Use the improvement check prompt - focus on specific decision criteria
	context := critiqueResult
	question := `Analyze the critique report and determine if another refinement iteration is needed.

Focus on these critical criteria:
- FACTUAL ERRORS: Major factual inaccuracies that need correction
- ANALYTICAL GAPS: Significant missing analysis or weak reasoning
- PROMPT MISALIGNMENT: Major deviations from the required task
- QUALITY ISSUES: Issues that would significantly impact the objective achievement

If the critique identifies ANY of these critical issues that would benefit from another refinement iteration, return true. Otherwise, return false.
`

	// Use conditional LLM to make the decision
	result, err := too.GetConditionalLLM().Decide(ctx, context, question, 0, 0)
	if err != nil {
		too.AgentTemplate.GetLogger().Errorf("❌ Improvement check decision failed: %v", err)
		return false, "Improvement check failed: " + err.Error(), err
	}

	too.AgentTemplate.GetLogger().Infof("🎯 Improvement check result: %t - %s", result.GetResult(), result.Reason)
	return result.GetResult(), result.Reason, nil
}

// Agent creation methods
func (too *TodoOptimizationOrchestrator) createRefineAgent() (agents.OrchestratorAgent, error) {
	if too.refineAgent != nil {
		return too.refineAgent, nil
	}

	agent := NewTodoRefinePlannerAgent(too.AgentTemplate.GetConfig(), too.AgentTemplate.GetLogger(), too.GetTracer(), too.GetEventBridge())

	// Initialize the agent
	if err := agent.Initialize(context.Background()); err != nil {
		return nil, fmt.Errorf("failed to initialize refine agent: %w", err)
	}

	// Register workspace tools if available
	if too.WorkspaceTools != nil && too.WorkspaceToolExecutors != nil {
		if err := too.RegisterWorkspaceTools(agent); err != nil {
			too.AgentTemplate.GetLogger().Warnf("⚠️ Failed to register workspace tools for refine agent: %v", err)
		}
	}

	// Connect to event bridge if available
	if err := too.ConnectAgentToEventBridge(agent, "todo_optimization_refine"); err != nil {
		too.AgentTemplate.GetLogger().Warnf("⚠️ Failed to connect refine agent to event bridge: %v", err)
	}

	too.refineAgent = agent
	return agent, nil
}

func (too *TodoOptimizationOrchestrator) createCritiqueAgent() (agents.OrchestratorAgent, error) {
	if too.critiqueAgent != nil {
		return too.critiqueAgent, nil
	}

	agent := NewDataCritiqueAgent(too.AgentTemplate.GetConfig(), too.AgentTemplate.GetLogger(), too.GetTracer(), too.GetEventBridge())

	// Initialize the agent
	if err := agent.Initialize(context.Background()); err != nil {
		return nil, fmt.Errorf("failed to initialize critique agent: %w", err)
	}

	// Register workspace tools if available
	if too.WorkspaceTools != nil && too.WorkspaceToolExecutors != nil {
		if err := too.RegisterWorkspaceTools(agent); err != nil {
			too.AgentTemplate.GetLogger().Warnf("⚠️ Failed to register workspace tools for critique agent: %v", err)
		}
	}

	// Connect to event bridge if available
	if err := too.ConnectAgentToEventBridge(agent, "todo_optimization_critique"); err != nil {
		too.AgentTemplate.GetLogger().Warnf("⚠️ Failed to connect critique agent to event bridge: %v", err)
	}

	too.critiqueAgent = agent
	return agent, nil
}
