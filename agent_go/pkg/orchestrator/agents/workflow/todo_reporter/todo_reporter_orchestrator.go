package todo_reporter

import (
	"context"
	"fmt"
	"time"

	"mcp-agent/agent_go/internal/observability"
	"mcp-agent/agent_go/internal/utils"
	"mcp-agent/agent_go/pkg/orchestrator"
	"mcp-agent/agent_go/pkg/orchestrator/agents"
	"mcp-agent/agent_go/pkg/orchestrator/agents/workflow/todo_optimization"
)

// TodoReporterOrchestrator manages the multi-agent report generation process
type TodoReporterOrchestrator struct {
	// Base orchestrator for common functionality
	*orchestrator.BaseOrchestrator

	// Sub-agents (created on-demand)
	reportAgent   agents.OrchestratorAgent
	critiqueAgent agents.OrchestratorAgent
}

// NewTodoReporterOrchestrator creates a new multi-agent report generation orchestrator
func NewTodoReporterOrchestrator(
	config *agents.OrchestratorAgentConfig,
	logger utils.ExtendedLogger,
	tracer observability.Tracer,
	eventBridge interface{},
) (*TodoReporterOrchestrator, error) {
	baseOrchestrator, err := orchestrator.NewBaseOrchestrator(
		config,
		logger,
		tracer,
		eventBridge,
		agents.TodoReporterAgentType,
		orchestrator.OrchestratorTypeWorkflow,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create base orchestrator: %w", err)
	}

	return &TodoReporterOrchestrator{
		BaseOrchestrator: baseOrchestrator,
	}, nil
}

// ExecuteReportGeneration orchestrates the iterative report generation process
func (tro *TodoReporterOrchestrator) ExecuteReportGeneration(ctx context.Context, objective, workspacePath string) (string, error) {
	tro.AgentTemplate.GetLogger().Infof("📊 Starting iterative report generation for objective: %s", objective)
	tro.AgentTemplate.GetLogger().Infof("📁 Using workspace path: %s", workspacePath)

	// Set objective and workspace path directly
	tro.SetObjective(objective)
	tro.SetWorkspacePath(workspacePath)

	maxIterations := 3 // Configurable max report refinement iterations
	var finalReportResult string

	// Iterative report refinement loop with critique feedback
	var previousCritiqueResult string
	for iteration := 1; iteration <= maxIterations; iteration++ {
		tro.AgentTemplate.GetLogger().Infof("📊 Report generation iteration %d/%d", iteration, maxIterations)

		// Step 1: Generate report (with previous critique feedback for iterations 2+)
		var reportResult string
		var err error

		if iteration == 1 {
			// First iteration - no previous critique available
			reportResult, err = tro.runReportGenerationPhase(ctx, objective, "")
		} else {
			// Subsequent iterations - use previous critique result for improvement
			reportResult, err = tro.runReportGenerationPhase(ctx, objective, previousCritiqueResult)
		}

		if err != nil {
			return "", fmt.Errorf("report generation iteration %d failed: %w", iteration, err)
		}

		// Step 2: Critique the generated report
		reportPrompt := fmt.Sprintf("Critique the generated report for iteration %d. Focus on accuracy, completeness, and alignment with the objective.", iteration)
		critiqueResult, err := tro.runCritiquePhase(ctx, objective, reportResult, reportPrompt, iteration)

		if err != nil {
			return "", fmt.Errorf("report critique iteration %d failed: %w", iteration, err)
		}

		// Step 3: Check if more improvement is needed
		needsMoreWork, improvementReason, err := tro.checkImprovementNeeded(ctx, critiqueResult)
		if err != nil {
			tro.AgentTemplate.GetLogger().Warnf("⚠️ Report improvement check failed: %v", err)
			needsMoreWork = true
			improvementReason = "Improvement check failed: " + err.Error()
		}

		tro.AgentTemplate.GetLogger().Infof("📊 Report iteration %d improvement check: needs_more=%t, reason=%s", iteration, needsMoreWork, improvementReason)

		// Store the current report result
		finalReportResult = reportResult

		// If critique is satisfied, exit the loop
		if !needsMoreWork {
			tro.AgentTemplate.GetLogger().Infof("✅ Report generation iteration %d: Critique satisfied, report complete", iteration)
			break
		}

		// Store critique result for next iteration
		previousCritiqueResult = critiqueResult

		// If this is the last iteration, log warning
		if iteration == maxIterations {
			tro.AgentTemplate.GetLogger().Warnf("⚠️ Max report refinement iterations (%d) reached, returning last result", maxIterations)
		}
	}

	duration := time.Since(tro.GetStartTime())
	tro.AgentTemplate.GetLogger().Infof("✅ Iterative report generation completed in %v", duration)

	return finalReportResult, nil
}

// runReportGenerationPhase runs a single report generation iteration using the proper agent pattern
func (tro *TodoReporterOrchestrator) runReportGenerationPhase(ctx context.Context, objective, previousCritiqueResult string) (string, error) {
	tro.AgentTemplate.GetLogger().Infof("📊 Running report generation phase")

	// Create ReportGenerationAgent for report generation
	reportAgent, err := tro.createReportAgent()
	if err != nil {
		return "", fmt.Errorf("failed to create report generation agent: %w", err)
	}

	// Prepare template variables with critique feedback
	templateVars := map[string]string{
		"Objective":        objective,
		"WorkspacePath":    tro.GetWorkspacePath(),
		"CritiqueFeedback": previousCritiqueResult,
	}

	// Execute report generation using the ReportGenerationAgent
	reportResult, err := reportAgent.Execute(ctx, templateVars, nil)
	if err != nil {
		return "", fmt.Errorf("report generation execution failed: %v", err)
	}

	tro.AgentTemplate.GetLogger().Infof("✅ Report generation phase completed: %d characters", len(reportResult))
	return reportResult, nil
}

// runCritiquePhase runs a single critique iteration using the proper agent pattern
func (tro *TodoReporterOrchestrator) runCritiquePhase(ctx context.Context, objective, inputData, inputPrompt string, iteration int) (string, error) {
	tro.AgentTemplate.GetLogger().Infof("🔍 Running critique phase iteration %d", iteration)

	// Create DataCritiqueAgent for critique
	critiqueAgent, err := tro.createCritiqueAgent()
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

	tro.AgentTemplate.GetLogger().Infof("✅ Critique phase iteration %d completed: %d characters", iteration, len(critiqueResult))
	return critiqueResult, nil
}

// checkImprovementNeeded uses conditional logic to determine if there's room for more improvement
func (tro *TodoReporterOrchestrator) checkImprovementNeeded(ctx context.Context, critiqueResult string) (bool, string, error) {
	tro.AgentTemplate.GetLogger().Infof("🎯 Checking if more improvement is needed based on critique")

	if tro.GetConditionalLLM() == nil {
		tro.AgentTemplate.GetLogger().Errorf("❌ Conditional LLM not initialized")
		return false, "Conditional LLM not initialized", fmt.Errorf("conditional LLM not initialized")
	}

	// Use the improvement check prompt - focus on specific decision criteria
	context := critiqueResult
	question := `Analyze the critique report and determine if another report refinement iteration is needed.

Focus on these critical criteria:
- FACTUAL ERRORS: Major factual inaccuracies that need correction
- ANALYTICAL GAPS: Significant missing analysis or weak reasoning
- PROMPT MISALIGNMENT: Major deviations from the required task
- QUALITY ISSUES: Issues that would significantly impact the objective achievement

If the critique identifies ANY of these critical issues that would benefit from another refinement iteration, return true. Otherwise, return false.
`

	// Use conditional LLM to make the decision
	result, err := tro.GetConditionalLLM().Decide(ctx, context, question, 0, 0)
	if err != nil {
		tro.AgentTemplate.GetLogger().Errorf("❌ Improvement check decision failed: %v", err)
		return false, "Improvement check failed: " + err.Error(), err
	}

	tro.AgentTemplate.GetLogger().Infof("🎯 Improvement check result: %t - %s", result.GetResult(), result.Reason)
	return result.GetResult(), result.Reason, nil
}

// Agent creation methods
func (tro *TodoReporterOrchestrator) createReportAgent() (agents.OrchestratorAgent, error) {
	if tro.reportAgent != nil {
		return tro.reportAgent, nil
	}

	agent := todo_optimization.NewReportGenerationAgent(tro.AgentTemplate.GetConfig(), tro.AgentTemplate.GetLogger(), tro.GetTracer(), tro.GetEventBridge())

	// Initialize the agent
	if err := agent.Initialize(context.Background()); err != nil {
		return nil, fmt.Errorf("failed to initialize report agent: %w", err)
	}

	// Register workspace tools if available
	if tro.WorkspaceTools != nil && tro.WorkspaceToolExecutors != nil {
		if err := tro.RegisterWorkspaceTools(agent); err != nil {
			tro.AgentTemplate.GetLogger().Warnf("⚠️ Failed to register workspace tools for report agent: %v", err)
		}
	}

	// Connect to event bridge if available
	if err := tro.ConnectAgentToEventBridge(agent, "todo_reporter_report"); err != nil {
		tro.AgentTemplate.GetLogger().Warnf("⚠️ Failed to connect report agent to event bridge: %v", err)
	}

	tro.reportAgent = agent
	return agent, nil
}

func (tro *TodoReporterOrchestrator) createCritiqueAgent() (agents.OrchestratorAgent, error) {
	if tro.critiqueAgent != nil {
		return tro.critiqueAgent, nil
	}

	agent := todo_optimization.NewDataCritiqueAgent(tro.AgentTemplate.GetConfig(), tro.AgentTemplate.GetLogger(), tro.GetTracer(), tro.GetEventBridge())

	// Initialize the agent
	if err := agent.Initialize(context.Background()); err != nil {
		return nil, fmt.Errorf("failed to initialize critique agent: %w", err)
	}

	// Register workspace tools if available
	if tro.WorkspaceTools != nil && tro.WorkspaceToolExecutors != nil {
		if err := tro.RegisterWorkspaceTools(agent); err != nil {
			tro.AgentTemplate.GetLogger().Warnf("⚠️ Failed to register workspace tools for critique agent: %v", err)
		}
	}

	// Connect to event bridge if available
	if err := tro.ConnectAgentToEventBridge(agent, "todo_reporter_critique"); err != nil {
		tro.AgentTemplate.GetLogger().Warnf("⚠️ Failed to connect critique agent to event bridge: %v", err)
	}

	tro.critiqueAgent = agent
	return agent, nil
}
