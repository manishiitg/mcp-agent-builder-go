package todo_execution

import (
	"context"
	"fmt"
	"time"

	"mcp-agent/agent_go/internal/observability"
	"mcp-agent/agent_go/internal/utils"
	"mcp-agent/agent_go/pkg/orchestrator"
	"mcp-agent/agent_go/pkg/orchestrator/agents"

	"github.com/tmc/langchaingo/llms"
)

// TodoExecutionOrchestrator manages the multi-agent todo execution process
type TodoExecutionOrchestrator struct {
	// Base orchestrator for common functionality
	*orchestrator.BaseOrchestrator

	// Sub-agents (created on-demand)
	executionAgent  agents.OrchestratorAgent
	validationAgent agents.OrchestratorAgent
	workspaceAgent  agents.OrchestratorAgent
}

// NewTodoExecutionOrchestrator creates a new multi-agent todo execution orchestrator
func NewTodoExecutionOrchestrator(
	config *agents.OrchestratorAgentConfig,
	logger utils.ExtendedLogger,
	tracer observability.Tracer,
	eventBridge interface{},
) (*TodoExecutionOrchestrator, error) {

	// Create base workflow orchestrator
	baseOrchestrator, err := orchestrator.NewBaseOrchestrator(
		config,
		logger,
		tracer,
		eventBridge,
		agents.TodoExecutionAgentType,
		orchestrator.OrchestratorTypeWorkflow,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create base orchestrator: %w", err)
	}

	return &TodoExecutionOrchestrator{
		BaseOrchestrator: baseOrchestrator,
	}, nil
}

// ExecuteTodos orchestrates the multi-agent todo execution process
func (teo *TodoExecutionOrchestrator) ExecuteTodos(ctx context.Context, objective, workspacePath, runOption string) (string, error) {
	teo.AgentTemplate.GetLogger().Infof("🚀 Starting multi-agent todo execution for objective: %s", objective)
	teo.AgentTemplate.GetLogger().Infof("📁 Using workspace path: %s", workspacePath)

	// Set objective and workspace path directly
	teo.SetObjective(objective)
	teo.SetWorkspacePath(workspacePath)

	// STEP 1: EXECUTION PHASE
	teo.AgentTemplate.GetLogger().Infof("🔄 Step 1/3: Todo Execution")

	executionResult, err := teo.runExecutionPhase(ctx, runOption)

	if err != nil {
		return "", fmt.Errorf("execution phase failed: %w", err)
	}

	// STEP 2: VALIDATION PHASE
	teo.AgentTemplate.GetLogger().Infof("🔄 Step 2/3: Todo Validation")

	validationResult, err := teo.runValidationPhase(ctx, executionResult)
	if err != nil {
		teo.AgentTemplate.GetLogger().Warnf("⚠️ Validation phase failed: %v", err)
		validationResult = "Validation failed: " + err.Error()
	}

	// STEP 3: WORKSPACE UPDATE PHASE
	teo.AgentTemplate.GetLogger().Infof("🔄 Step 3/3: Workspace Update")

	workspaceResult, err := teo.runWorkspaceUpdatePhase(ctx, executionResult, validationResult)
	if err != nil {
		teo.AgentTemplate.GetLogger().Warnf("⚠️ Workspace update phase failed: %v", err)
		workspaceResult = "Workspace update failed: " + err.Error()
	}

	// Check completion status
	allTodosCompleted, completionReason, err := teo.checkCompletion(ctx, workspaceResult)
	if err != nil {
		teo.AgentTemplate.GetLogger().Warnf("⚠️ Completion check failed: %v", err)
		allTodosCompleted = false
		completionReason = "Completion check failed: " + err.Error()
	}

	duration := time.Since(teo.GetStartTime())
	teo.AgentTemplate.GetLogger().Infof("✅ Multi-agent todo execution completed in %v", duration)

	return teo.formatResults(executionResult, validationResult, workspaceResult, allTodosCompleted, completionReason), nil
}

// Execute implements the OrchestratorAgent interface
func (teo *TodoExecutionOrchestrator) Execute(ctx context.Context, templateVars map[string]string, conversationHistory []llms.MessageContent) (string, error) {
	// Extract objective and run option from template variables
	objective := templateVars["Objective"]
	if objective == "" {
		objective = templateVars["objective"] // Try lowercase as fallback
	}
	if objective == "" {
		return "", fmt.Errorf("objective not found in template variables")
	}

	// Extract workspace path from template variables
	workspacePath := templateVars["WorkspacePath"]
	if workspacePath == "" {
		return "", fmt.Errorf("workspace path not found in template variables")
	}

	runOption := templateVars["RunOption"]
	if runOption == "" {
		runOption = "create_new_runs_always" // Default
	}

	// Delegate to ExecuteTodos
	return teo.ExecuteTodos(ctx, objective, workspacePath, runOption)
}

// GetType implements the OrchestratorAgent interface
func (teo *TodoExecutionOrchestrator) GetType() string {
	return string(agents.TodoExecutionAgentType)
}

// runExecutionPhase executes todos using the execution agent
func (teo *TodoExecutionOrchestrator) runExecutionPhase(ctx context.Context, runOption string) (string, error) {
	teo.AgentTemplate.GetLogger().Infof("🚀 Creating execution agent")

	executionAgent, err := teo.createExecutionAgent()
	if err != nil {
		return "", fmt.Errorf("failed to create execution agent: %w", err)
	}

	teo.AgentTemplate.GetLogger().Infof("🚀 Executing todos with run option: %s", runOption)

	// Prepare template variables for Execute method
	templateVars := map[string]string{
		"Objective":     teo.GetObjective(),
		"WorkspacePath": teo.GetWorkspacePath(),
		"RunOption":     runOption,
	}

	executionResult, err := executionAgent.Execute(ctx, templateVars, nil)
	if err != nil {
		return "", fmt.Errorf("execution failed: %w", err)
	}

	teo.AgentTemplate.GetLogger().Infof("✅ Execution phase completed: %d characters", len(executionResult))
	return executionResult, nil
}

// runValidationPhase validates execution results using the validation agent
func (teo *TodoExecutionOrchestrator) runValidationPhase(ctx context.Context, executionResult string) (string, error) {
	teo.AgentTemplate.GetLogger().Infof("🔍 Creating validation agent")

	validationAgent, err := teo.createValidationAgent()
	if err != nil {
		return "", fmt.Errorf("failed to create validation agent: %w", err)
	}

	teo.AgentTemplate.GetLogger().Infof("🔍 Validating execution results")
	validationTemplateVars := map[string]string{
		"Objective":       teo.GetObjective(),
		"WorkspacePath":   teo.GetWorkspacePath(),
		"ExecutionOutput": executionResult,
	}
	validationResult, err := validationAgent.Execute(ctx, validationTemplateVars, nil)
	if err != nil {
		return "", fmt.Errorf("validation failed: %w", err)
	}

	teo.AgentTemplate.GetLogger().Infof("✅ Validation phase completed: %d characters", len(validationResult))
	return validationResult, nil
}

// runWorkspaceUpdatePhase updates workspace using the workspace agent
func (teo *TodoExecutionOrchestrator) runWorkspaceUpdatePhase(ctx context.Context, executionResult, validationResult string) (string, error) {
	teo.AgentTemplate.GetLogger().Infof("📁 Creating workspace update agent")

	workspaceAgent, err := teo.createWorkspaceAgent()
	if err != nil {
		return "", fmt.Errorf("failed to create workspace agent: %w", err)
	}

	teo.AgentTemplate.GetLogger().Infof("📁 Updating workspace with execution and validation results")
	workspaceTemplateVars := map[string]string{
		"Objective":        teo.GetObjective(),
		"WorkspacePath":    teo.GetWorkspacePath(),
		"ExecutionOutput":  executionResult,
		"ValidationOutput": validationResult,
	}
	workspaceResult, err := workspaceAgent.Execute(ctx, workspaceTemplateVars, nil)
	if err != nil {
		return "", fmt.Errorf("workspace update failed: %w", err)
	}

	teo.AgentTemplate.GetLogger().Infof("✅ Workspace update phase completed: %d characters", len(workspaceResult))
	return workspaceResult, nil
}

// checkCompletion uses conditional logic to determine if all todos are completed
func (teo *TodoExecutionOrchestrator) checkCompletion(ctx context.Context, workspaceResult string) (bool, string, error) {
	teo.AgentTemplate.GetLogger().Infof("🎯 Checking todo completion status using conditional logic")

	if teo.GetConditionalLLM() == nil {
		teo.AgentTemplate.GetLogger().Errorf("❌ Conditional LLM not initialized")
		return false, "Conditional LLM not initialized", fmt.Errorf("conditional LLM not initialized")
	}

	// Prepare context and question for true/false decision
	context := fmt.Sprintf("Workspace Update Agent Report:\n%s", workspaceResult)
	question := "Are all todos completed? Look for completion status, remaining todos, and overall project status in the workspace update report."

	// Use conditional LLM to make the decision
	result, err := teo.GetConditionalLLM().Decide(ctx, context, question, 0, 0)
	if err != nil {
		teo.AgentTemplate.GetLogger().Errorf("❌ Conditional LLM decision failed: %v", err)
		return false, "Conditional decision failed: " + err.Error(), err
	}

	teo.AgentTemplate.GetLogger().Infof("🎯 Conditional logic result: %t - %s", result.GetResult(), result.Reason)
	return result.GetResult(), result.Reason, nil
}

// formatResults formats the execution results into a comprehensive report
func (teo *TodoExecutionOrchestrator) formatResults(executionResult, validationResult, workspaceResult string, allCompleted bool, completionReason string) string {
	status := "IN PROGRESS"
	if allCompleted {
		status = "COMPLETED"
	}

	return fmt.Sprintf(`# Todo Execution Report

## Execution Summary
- **Objective**: %s
- **Status**: %s
- **Duration**: %v
- **Workspace**: %s
- **Completion Reason**: %s

## Execution Results
%s

## Validation Results
%s

## Workspace Update Results
%s

## Next Steps
%s

## Evidence Files
- **Runs Folder**: %s/runs/
- **Execution Logs**: Available in runs folder
- **Results**: Available in runs folder
- **Evidence**: Available in runs folder

Focus on executing as many incomplete todos as possible effectively and providing comprehensive results.`,
		teo.GetObjective(), status, time.Since(teo.GetStartTime()), teo.GetWorkspacePath(), completionReason,
		executionResult, validationResult, workspaceResult,
		func() string {
			if allCompleted {
				return "All todos completed successfully!"
			}
			return "Continue with next execution cycle for remaining todos."
		}(), teo.GetWorkspacePath())
}

// Agent creation methods
func (teo *TodoExecutionOrchestrator) createExecutionAgent() (agents.OrchestratorAgent, error) {
	if teo.executionAgent != nil {
		return teo.executionAgent, nil
	}

	agent := NewTodoExecutionAgent(teo.AgentTemplate.GetConfig(), teo.AgentTemplate.GetLogger(), teo.GetTracer(), teo.GetEventBridge())

	// Initialize the agent
	if err := agent.Initialize(context.Background()); err != nil {
		return nil, fmt.Errorf("failed to initialize execution agent: %w", err)
	}

	// Register workspace tools if available
	if teo.WorkspaceTools != nil && teo.WorkspaceToolExecutors != nil {
		if err := teo.RegisterWorkspaceTools(agent); err != nil {
			teo.AgentTemplate.GetLogger().Warnf("⚠️ Failed to register workspace tools for execution agent: %v", err)
		}
	}

	// Connect to event bridge if available
	if err := teo.ConnectAgentToEventBridge(agent, "todo_execution_execution"); err != nil {
		teo.AgentTemplate.GetLogger().Warnf("⚠️ Failed to connect execution agent to event bridge: %v", err)
	}

	teo.executionAgent = agent
	return agent, nil
}

func (teo *TodoExecutionOrchestrator) createValidationAgent() (agents.OrchestratorAgent, error) {
	if teo.validationAgent != nil {
		return teo.validationAgent, nil
	}

	agent := NewTodoValidationAgent(teo.AgentTemplate.GetConfig(), teo.AgentTemplate.GetLogger(), teo.GetTracer(), teo.GetEventBridge())

	// Initialize the agent
	if err := agent.Initialize(context.Background()); err != nil {
		return nil, fmt.Errorf("failed to initialize validation agent: %w", err)
	}

	// Register workspace tools if available
	if teo.WorkspaceTools != nil && teo.WorkspaceToolExecutors != nil {
		if err := teo.RegisterWorkspaceTools(agent); err != nil {
			teo.AgentTemplate.GetLogger().Warnf("⚠️ Failed to register workspace tools for validation agent: %v", err)
		}
	}

	// Connect to event bridge if available
	if err := teo.ConnectAgentToEventBridge(agent, "todo_execution_validation"); err != nil {
		teo.AgentTemplate.GetLogger().Warnf("⚠️ Failed to connect validation agent to event bridge: %v", err)
	}

	teo.validationAgent = agent
	return agent, nil
}

func (teo *TodoExecutionOrchestrator) createWorkspaceAgent() (agents.OrchestratorAgent, error) {
	if teo.workspaceAgent != nil {
		return teo.workspaceAgent, nil
	}

	agent := NewWorkspaceUpdateAgent(teo.AgentTemplate.GetConfig(), teo.AgentTemplate.GetLogger(), teo.GetTracer(), teo.GetEventBridge())

	// Initialize the agent
	if err := agent.Initialize(context.Background()); err != nil {
		return nil, fmt.Errorf("failed to initialize workspace agent: %w", err)
	}

	// Register workspace tools if available
	if teo.WorkspaceTools != nil && teo.WorkspaceToolExecutors != nil {
		if err := teo.RegisterWorkspaceTools(agent); err != nil {
			teo.AgentTemplate.GetLogger().Warnf("⚠️ Failed to register workspace tools for workspace agent: %v", err)
		}
	}

	// Connect to event bridge if available
	if err := teo.ConnectAgentToEventBridge(agent, "todo_execution_workspace"); err != nil {
		teo.AgentTemplate.GetLogger().Warnf("⚠️ Failed to connect workspace agent to event bridge: %v", err)
	}

	teo.workspaceAgent = agent
	return agent, nil
}
