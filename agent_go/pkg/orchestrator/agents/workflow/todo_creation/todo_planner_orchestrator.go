package todo_creation

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

// TodoPlannerOrchestrator manages the multi-agent todo planning process
type TodoPlannerOrchestrator struct {
	// Base orchestrator for common functionality
	*orchestrator.BaseOrchestrator

	// Sub-agents (created on-demand)
	planningAgent   agents.OrchestratorAgent
	executionAgent  agents.OrchestratorAgent
	validationAgent agents.OrchestratorAgent
	writerAgent     agents.OrchestratorAgent
	cleanupAgent    agents.OrchestratorAgent

	// Enhanced critique system
	critiqueAgent agents.OrchestratorAgent
}

// NewTodoPlannerOrchestrator creates a new multi-agent todo planner orchestrator
func NewTodoPlannerOrchestrator(
	config *agents.OrchestratorAgentConfig,
	logger utils.ExtendedLogger,
	tracer observability.Tracer,
	eventBridge interface{},
) (*TodoPlannerOrchestrator, error) {

	// Create base workflow orchestrator
	baseOrchestrator, err := orchestrator.NewBaseOrchestrator(
		config,
		logger,
		tracer,
		eventBridge,
		agents.TodoPlannerAgentType,
		orchestrator.OrchestratorTypeWorkflow,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create base orchestrator: %w", err)
	}

	return &TodoPlannerOrchestrator{
		BaseOrchestrator: baseOrchestrator,
	}, nil
}

// CreateTodoList orchestrates the multi-agent todo planning process with objective achievement checking
func (tpo *TodoPlannerOrchestrator) CreateTodoList(ctx context.Context, objective, workspacePath string) (string, error) {
	tpo.AgentTemplate.GetLogger().Infof("🚀 Starting multi-agent todo planning for objective: %s", objective)
	tpo.AgentTemplate.GetLogger().Infof("📁 Using workspace path: %s", workspacePath)

	// Set objective and workspace path directly
	tpo.SetObjective(objective)
	tpo.SetWorkspacePath(workspacePath)

	// Balanced iterative approach: optimization-focused early iterations, completion-focused later iterations
	maxExecutionIterations := 10
	var finalExecutionResult string
	var finalValidationResult string
	var finalCritiqueResult string
	var planResult string

	for iteration := 1; iteration <= maxExecutionIterations; iteration++ {
		// Determine strategy based on iteration phase
		strategy := tpo.getIterationStrategy(iteration, maxExecutionIterations)
		tpo.AgentTemplate.GetLogger().Infof("🔄 %s iteration %d/%d", strategy.Name, iteration, maxExecutionIterations)

		// Phase 1: Create/Refine plan based on iteration strategy
		tpo.AgentTemplate.GetLogger().Infof("📋 Phase 1: %s (iteration %d)", strategy.PlanningPhase, iteration)

		// Pass structured execution results to planning phase
		structuredExecutionResult := tpo.structureExecutionResults(finalExecutionResult)
		planResult, err := tpo.runPlanningPhase(ctx, structuredExecutionResult, finalValidationResult, finalCritiqueResult, iteration, strategy)

		if err != nil {
			return "", fmt.Errorf("planning phase failed: %w", err)
		}

		// Phase 2: Execute based on iteration strategy
		tpo.AgentTemplate.GetLogger().Infof("🚀 Phase 2: %s (iteration %d)", strategy.ExecutionPhase, iteration)

		executionResult, err := tpo.runExecutionPhase(ctx, planResult, iteration, strategy)

		if err != nil {
			return "", fmt.Errorf("execution phase failed: %w", err)
		}

		// Phase 3: Validation based on iteration strategy
		tpo.AgentTemplate.GetLogger().Infof("🔍 Phase 3: %s (iteration %d)", strategy.ValidationPhase, iteration)

		validationResult, err := tpo.runValidationPhase(ctx, planResult, iteration, executionResult, strategy)
		if err != nil {
			tpo.AgentTemplate.GetLogger().Warnf("⚠️ Validation phase failed: %v", err)
			validationResult = "Validation failed: " + err.Error()
		}

		// Phase 4: Write/Update todo list based on iteration strategy
		tpo.AgentTemplate.GetLogger().Infof("📝 Phase 4: %s (iteration %d)", strategy.WriterPhase, iteration)

		_, err = tpo.runWriterPhase(ctx, planResult, executionResult, validationResult, finalCritiqueResult, strategy)
		if err != nil {
			tpo.AgentTemplate.GetLogger().Warnf("⚠️ Writer phase failed: %v", err)
		}

		// Phase 5: Critique todo list quality
		tpo.AgentTemplate.GetLogger().Infof("🔍 Phase 5: Critiquing todo list quality (iteration %d)", iteration)

		critiqueResult, err := tpo.runTodoListCritiquePhase(ctx, tpo.GetObjective(), iteration)
		if err != nil {
			tpo.AgentTemplate.GetLogger().Warnf("⚠️ Todo list critique phase failed: %v", err)
			critiqueResult = "Todo list critique failed: " + err.Error()
		}

		// Store the current results
		finalExecutionResult = executionResult
		finalValidationResult = validationResult
		finalCritiqueResult = critiqueResult

		// Check if we should continue to next iteration or stop using iteration-aware conditional LLM
		objectiveAchieved, reason, err := tpo.checkObjectiveAchievement(ctx, planResult, critiqueResult, iteration, strategy)
		if err != nil {
			tpo.AgentTemplate.GetLogger().Warnf("⚠️ Failed to check objective achievement: %v", err)
			// Continue execution on error
		} else if objectiveAchieved {
			tpo.AgentTemplate.GetLogger().Infof("🎯 Objective achieved at iteration %d (%s): %s", iteration, strategy.Name, reason)
			break
		}

		// If this is the last iteration, log warning
		if iteration == maxExecutionIterations {
			tpo.AgentTemplate.GetLogger().Warnf("⚠️ Max execution iterations (%d) reached, proceeding with current results", maxExecutionIterations)
		}
	}

	// Note: todoListResult is handled by the writer agent and saved to workspace

	// Phase 6: Cleanup planning workspace
	tpo.AgentTemplate.GetLogger().Infof("🧹 Phase 6: Cleaning up planning workspace")

	cleanupResult, err := tpo.runCleanupPhase(ctx)
	if err != nil {
		tpo.AgentTemplate.GetLogger().Warnf("⚠️ Cleanup phase failed: %v", err)
	}

	// Note: The writer agent handles saving the todo.md file directly

	duration := time.Since(tpo.GetStartTime())
	tpo.AgentTemplate.GetLogger().Infof("✅ Multi-agent todo planning completed in %v", duration)

	// Emit orchestrator end event

	return fmt.Sprintf(`# Todo Planning Complete

## Planning Summary
- **Objective**: %s
- **Duration**: %v
- **Workspace**: %s
- **Phases**: Comprehensive Planning → Complete Execution → Validation → Writing → Critique → Cleanup

## Plan Created
%s

## Final Execution Results
%s

## Final Validation Results
%s

## Final Critique Results
%s

## Final Todo List
Todo list has been created and saved to the workspace by the writer agent.

## Cleanup Results
%s

## Next Steps
The todo list has been saved to %s/todo_final.md (moved from todo_creation folder) and is ready for the execution phase.`,
		tpo.GetObjective(), duration, tpo.GetWorkspacePath(),
		planResult, finalExecutionResult, finalValidationResult, finalCritiqueResult, cleanupResult, tpo.GetWorkspacePath()), nil
}

// Execute implements the OrchestratorAgent interface
func (tpo *TodoPlannerOrchestrator) Execute(ctx context.Context, templateVars map[string]string, conversationHistory []llms.MessageContent) (string, error) {
	// Extract objective from template variables
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

	// Delegate to CreateTodoList
	return tpo.CreateTodoList(ctx, objective, workspacePath)
}

// GetType implements the OrchestratorAgent interface
func (tpo *TodoPlannerOrchestrator) GetType() string {
	return string(agents.TodoPlannerAgentType)
}

// runPlanningPhase creates or refines the step-wise plan based on iteration strategy
func (tpo *TodoPlannerOrchestrator) runPlanningPhase(ctx context.Context, previousExecutionResult, previousValidationResult, previousCritiqueResult string, iteration int, strategy IterationStrategy) (string, error) {
	tpo.AgentTemplate.GetLogger().Infof("📋 Creating planning agent for %s", strategy.Name)

	planningAgent, err := tpo.createPlanningAgent()
	if err != nil {
		return "", fmt.Errorf("failed to create planning agent: %w", err)
	}

	if iteration == 1 {
		tpo.AgentTemplate.GetLogger().Infof("📋 Creating initial step-wise plan")
		planningTemplateVars := map[string]string{
			"Objective":     tpo.GetObjective(),
			"WorkspacePath": tpo.GetWorkspacePath(),
			"Strategy":      strategy.Name,
			"Focus":         strategy.Focus,
		}
		planResult, err := planningAgent.Execute(ctx, planningTemplateVars, nil)
		if err != nil {
			return "", fmt.Errorf("planning failed: %w", err)
		}
		tpo.AgentTemplate.GetLogger().Infof("✅ Planning phase completed: %d characters", len(planResult))
		return planResult, nil
	} else {
		tpo.AgentTemplate.GetLogger().Infof("📋 Refining step-wise plan based on previous execution, validation, and critique feedback (iteration %d)", iteration)
		planningTemplateVars := map[string]string{
			"Objective":     tpo.GetObjective(),
			"WorkspacePath": tpo.GetWorkspacePath(),
			"Strategy":      strategy.Name,
			"Focus":         strategy.Focus,
		}
		planResult, err := planningAgent.Execute(ctx, planningTemplateVars, nil)
		if err != nil {
			return "", fmt.Errorf("plan refinement failed: %w", err)
		}
		tpo.AgentTemplate.GetLogger().Infof("✅ Planning refinement completed: %d characters", len(planResult))
		return planResult, nil
	}
}

// runExecutionPhase executes the plan for the current iteration based on strategy
func (tpo *TodoPlannerOrchestrator) runExecutionPhase(ctx context.Context, plan string, iteration int, strategy IterationStrategy) (string, error) {
	tpo.AgentTemplate.GetLogger().Infof("🚀 Creating execution agent for %s", strategy.Name)

	executionAgent, err := tpo.createExecutionAgent()
	if err != nil {
		return "", fmt.Errorf("failed to create execution agent: %w", err)
	}

	tpo.AgentTemplate.GetLogger().Infof("🚀 Executing plan for iteration %d", iteration)

	// Prepare template variables for Execute method
	templateVars := map[string]string{
		"Objective":     tpo.GetObjective(),
		"Plan":          plan,
		"WorkspacePath": tpo.GetWorkspacePath(),
	}

	executionResult, err := executionAgent.Execute(ctx, templateVars, nil)
	if err != nil {
		return "", fmt.Errorf("execution failed: %w", err)
	}

	tpo.AgentTemplate.GetLogger().Infof("✅ Execution phase completed: %d characters", len(executionResult))
	return executionResult, nil
}

// runValidationPhase validates the execution results for the current iteration based on strategy
func (tpo *TodoPlannerOrchestrator) runValidationPhase(ctx context.Context, plan string, iteration int, executionResult string, strategy IterationStrategy) (string, error) {
	tpo.AgentTemplate.GetLogger().Infof("🔍 Creating validation agent for %s", strategy.Name)

	validationAgent, err := tpo.createValidationAgent()
	if err != nil {
		return "", fmt.Errorf("failed to create validation agent: %w", err)
	}

	tpo.AgentTemplate.GetLogger().Infof("🔍 Validating execution results for iteration %d", iteration)
	validationTemplateVars := map[string]string{
		"Objective":       tpo.GetObjective(),
		"Plan":            plan,
		"ExecutionResult": executionResult,
		"WorkspacePath":   tpo.GetWorkspacePath(),
		"Strategy":        strategy.Name,
		"Focus":           strategy.Focus,
	}
	validationResult, err := validationAgent.Execute(ctx, validationTemplateVars, nil)
	if err != nil {
		return "", fmt.Errorf("validation failed: %w", err)
	}

	tpo.AgentTemplate.GetLogger().Infof("✅ Validation phase completed: %d characters", len(validationResult))
	return validationResult, nil
}

// runWriterPhase creates optimal todo list based on plan and execution experience using strategy
func (tpo *TodoPlannerOrchestrator) runWriterPhase(ctx context.Context, planResult, executionResult, validationResult, critiqueResult string, strategy IterationStrategy) (string, error) {
	tpo.AgentTemplate.GetLogger().Infof("📝 Creating writer agent for %s", strategy.Name)

	writerAgent, err := tpo.createWriterAgent()
	if err != nil {
		return "", fmt.Errorf("failed to create writer agent: %w", err)
	}

	tpo.AgentTemplate.GetLogger().Infof("📝 Creating optimal todo list based on plan and execution experience")
	// Prepare template variables for Execute method
	writerTemplateVars := map[string]string{
		"Objective":        tpo.GetObjective(),
		"PlanResult":       planResult,
		"ExecutionResult":  executionResult,
		"ValidationResult": validationResult,
		"CritiqueResult":   critiqueResult,
		"WorkspacePath":    tpo.GetWorkspacePath(),
	}

	todoListResult, err := writerAgent.Execute(ctx, writerTemplateVars, nil)
	if err != nil {
		return "", fmt.Errorf("todo list creation failed: %w", err)
	}

	tpo.AgentTemplate.GetLogger().Infof("✅ Writer phase completed: %d characters", len(todoListResult))
	return todoListResult, nil
}

// runCleanupPhase cleans up the planning workspace
func (tpo *TodoPlannerOrchestrator) runCleanupPhase(ctx context.Context) (string, error) {
	tpo.AgentTemplate.GetLogger().Infof("🧹 Creating cleanup agent")

	cleanupAgent, err := tpo.createCleanupAgent()
	if err != nil {
		return "", fmt.Errorf("failed to create cleanup agent: %w", err)
	}

	tpo.AgentTemplate.GetLogger().Infof("🧹 Cleaning up planning workspace")
	cleanupTemplateVars := map[string]string{
		"WorkspacePath": tpo.GetWorkspacePath(),
	}
	cleanupResult, err := cleanupAgent.Execute(ctx, cleanupTemplateVars, nil)
	if err != nil {
		return "", fmt.Errorf("cleanup failed: %w", err)
	}

	tpo.AgentTemplate.GetLogger().Infof("✅ Cleanup phase completed: %d characters", len(cleanupResult))
	return cleanupResult, nil
}

// runTodoListCritiquePhase critiques the todo list quality and reproducibility
func (tpo *TodoPlannerOrchestrator) runTodoListCritiquePhase(ctx context.Context, objective string, iteration int) (string, error) {
	tpo.AgentTemplate.GetLogger().Infof("🔍 Creating todo list critique agent")

	critiqueAgent, err := tpo.createCritiqueAgent()
	if err != nil {
		return "", fmt.Errorf("failed to create critique agent: %w", err)
	}

	// Prepare template variables for TodoPlannerCritiqueAgent
	templateVars := map[string]string{
		"objective":      objective,
		"iteration":      fmt.Sprintf("%d", iteration),
		"workspace_path": tpo.GetWorkspacePath(),
	}

	// Execute todo list critique
	critiqueResult, err := critiqueAgent.Execute(ctx, templateVars, nil)
	if err != nil {
		return "", fmt.Errorf("todo list critique failed: %w", err)
	}

	tpo.AgentTemplate.GetLogger().Infof("✅ Todo list critique phase completed: %d characters", len(critiqueResult))
	return critiqueResult, nil
}

// checkObjectiveAchievement uses conditional LLM to determine if the objective was achieved based on iteration strategy
func (tpo *TodoPlannerOrchestrator) checkObjectiveAchievement(ctx context.Context, planResult, critiqueResult string, iteration int, strategy IterationStrategy) (bool, string, error) {
	tpo.AgentTemplate.GetLogger().Infof("🎯 Checking objective achievement using %s approach", strategy.Name)

	// Emit conditional LLM start event

	if tpo.GetConditionalLLM() == nil {
		tpo.AgentTemplate.GetLogger().Errorf("❌ Conditional LLM not initialized")
		return false, "Conditional LLM not initialized", fmt.Errorf("conditional LLM not initialized")
	}

	// Prepare context for objective achievement assessment based on iteration strategy
	context := fmt.Sprintf(`Objective: %s

Plan:
%s

Critique Results:
%s

Iteration Strategy: %s
Focus: %s
Iteration: %d

Assessment Context:
- This is a balanced todo creation workflow that adapts strategy based on iteration phase
- Early iterations (1-3): Focus on discovering optimal methods and approaches
- Middle iterations (4-6): Focus on validating and refining optimal methods
- Later iterations (7-10): Focus on completing the objective using proven optimal methods
- Current strategy: %s
- Current focus: %s`,
		tpo.GetObjective(), planResult, critiqueResult, strategy.Name, strategy.Focus, iteration, strategy.Name, strategy.Focus)

	question := fmt.Sprintf(`Based on the plan, critique analysis, and current iteration strategy (%s), %s

Consider:
1. **Strategy Alignment**: Are we achieving the goals of the current iteration strategy?
2. **Method Discovery**: Have we discovered optimal methods for critical steps?
3. **Method Validation**: Have we validated that our methods work consistently?
4. **Objective Completion**: Have we completed the objective using optimal methods?
5. **Quality Assessment**: Is the todo list comprehensive, clear, and reproducible?

%s`,
		strategy.Name, strategy.StoppingCriteria, strategy.StoppingCriteria)

	// Use conditional LLM to make the decision
	result, err := tpo.GetConditionalLLM().Decide(ctx, context, question, 0, 0)

	if err != nil {
		tpo.AgentTemplate.GetLogger().Errorf("❌ Conditional LLM decision failed: %v", err)
		return false, "Conditional decision failed: " + err.Error(), err
	}

	tpo.AgentTemplate.GetLogger().Infof("🎯 Balanced objective achievement check result: %t - %s", result.GetResult(), result.Reason)

	return result.GetResult(), result.Reason, nil
}

// Agent creation methods
func (tpo *TodoPlannerOrchestrator) createPlanningAgent() (agents.OrchestratorAgent, error) {
	if tpo.planningAgent != nil {
		return tpo.planningAgent, nil
	}

	agent := NewTodoPlannerPlanningAgent(tpo.AgentTemplate.GetConfig(), tpo.AgentTemplate.GetLogger(), tpo.GetTracer(), tpo.GetEventBridge())

	// Initialize the agent
	if err := agent.Initialize(context.Background()); err != nil {
		return nil, fmt.Errorf("failed to initialize planning agent: %w", err)
	}

	// Register workspace tools if available
	if tpo.WorkspaceTools != nil && tpo.WorkspaceToolExecutors != nil {
		if err := tpo.RegisterWorkspaceTools(agent); err != nil {
			tpo.AgentTemplate.GetLogger().Warnf("⚠️ Failed to register workspace tools for planning agent: %v", err)
		}
	}

	// Connect to event bridge if available
	if err := tpo.ConnectAgentToEventBridge(agent, "todo_planner_planning"); err != nil {
		tpo.AgentTemplate.GetLogger().Warnf("⚠️ Failed to connect planning agent to event bridge: %v", err)
	}

	tpo.planningAgent = agent
	return agent, nil
}

func (tpo *TodoPlannerOrchestrator) createExecutionAgent() (agents.OrchestratorAgent, error) {
	if tpo.executionAgent != nil {
		return tpo.executionAgent, nil
	}

	agent := NewTodoPlannerExecutionAgent(tpo.AgentTemplate.GetConfig(), tpo.AgentTemplate.GetLogger(), tpo.GetTracer(), tpo.GetEventBridge())

	// Initialize the agent
	if err := agent.Initialize(context.Background()); err != nil {
		return nil, fmt.Errorf("failed to initialize execution agent: %w", err)
	}

	// Register workspace tools if available
	if tpo.WorkspaceTools != nil && tpo.WorkspaceToolExecutors != nil {
		if err := tpo.RegisterWorkspaceTools(agent); err != nil {
			tpo.AgentTemplate.GetLogger().Warnf("⚠️ Failed to register workspace tools for execution agent: %v", err)
		}
	}

	// Connect to event bridge if available
	if err := tpo.ConnectAgentToEventBridge(agent, "todo_planner_execution"); err != nil {
		tpo.AgentTemplate.GetLogger().Warnf("⚠️ Failed to connect execution agent to event bridge: %v", err)
	}

	tpo.executionAgent = agent
	return agent, nil
}

func (tpo *TodoPlannerOrchestrator) createValidationAgent() (agents.OrchestratorAgent, error) {
	if tpo.validationAgent != nil {
		return tpo.validationAgent, nil
	}

	agent := NewTodoPlannerValidationAgent(tpo.AgentTemplate.GetConfig(), tpo.AgentTemplate.GetLogger(), tpo.GetTracer(), tpo.GetEventBridge())

	// Initialize the agent
	if err := agent.Initialize(context.Background()); err != nil {
		return nil, fmt.Errorf("failed to initialize validation agent: %w", err)
	}

	// Register workspace tools if available
	if tpo.WorkspaceTools != nil && tpo.WorkspaceToolExecutors != nil {
		if err := tpo.RegisterWorkspaceTools(agent); err != nil {
			tpo.AgentTemplate.GetLogger().Warnf("⚠️ Failed to register workspace tools for validation agent: %v", err)
		}
	}

	// Connect to event bridge if available
	if err := tpo.ConnectAgentToEventBridge(agent, "todo_planner_validation"); err != nil {
		tpo.AgentTemplate.GetLogger().Warnf("⚠️ Failed to connect validation agent to event bridge: %v", err)
	}

	tpo.validationAgent = agent
	return agent, nil
}

func (tpo *TodoPlannerOrchestrator) createWriterAgent() (agents.OrchestratorAgent, error) {
	if tpo.writerAgent != nil {
		return tpo.writerAgent, nil
	}

	agent := NewTodoPlannerWriterAgent(tpo.AgentTemplate.GetConfig(), tpo.AgentTemplate.GetLogger(), tpo.GetTracer(), tpo.GetEventBridge())

	// Initialize the agent
	if err := agent.Initialize(context.Background()); err != nil {
		return nil, fmt.Errorf("failed to initialize writer agent: %w", err)
	}

	// Register workspace tools if available
	if tpo.WorkspaceTools != nil && tpo.WorkspaceToolExecutors != nil {
		if err := tpo.RegisterWorkspaceTools(agent); err != nil {
			tpo.AgentTemplate.GetLogger().Warnf("⚠️ Failed to register workspace tools for writer agent: %v", err)
		}
	}

	// Connect to event bridge if available
	if err := tpo.ConnectAgentToEventBridge(agent, "todo_planner_writer"); err != nil {
		tpo.AgentTemplate.GetLogger().Warnf("⚠️ Failed to connect writer agent to event bridge: %v", err)
	}

	tpo.writerAgent = agent
	return agent, nil
}

func (tpo *TodoPlannerOrchestrator) createCleanupAgent() (agents.OrchestratorAgent, error) {
	if tpo.cleanupAgent != nil {
		return tpo.cleanupAgent, nil
	}

	agent := NewTodoPlannerCleanupAgent(tpo.AgentTemplate.GetConfig(), tpo.AgentTemplate.GetLogger(), tpo.GetTracer(), tpo.GetEventBridge())

	// Initialize the agent
	if err := agent.Initialize(context.Background()); err != nil {
		return nil, fmt.Errorf("failed to initialize cleanup agent: %w", err)
	}

	// Register workspace tools if available
	if tpo.WorkspaceTools != nil && tpo.WorkspaceToolExecutors != nil {
		if err := tpo.RegisterWorkspaceTools(agent); err != nil {
			tpo.AgentTemplate.GetLogger().Warnf("⚠️ Failed to register workspace tools for cleanup agent: %v", err)
		}
	}

	// Connect to event bridge if available
	if err := tpo.ConnectAgentToEventBridge(agent, "todo_planner_cleanup"); err != nil {
		tpo.AgentTemplate.GetLogger().Warnf("⚠️ Failed to connect cleanup agent to event bridge: %v", err)
	}

	tpo.cleanupAgent = agent
	return agent, nil
}

func (tpo *TodoPlannerOrchestrator) createCritiqueAgent() (agents.OrchestratorAgent, error) {
	if tpo.critiqueAgent != nil {
		return tpo.critiqueAgent, nil
	}

	agent := NewTodoPlannerCritiqueAgent(tpo.AgentTemplate.GetConfig(), tpo.AgentTemplate.GetLogger(), tpo.GetTracer(), tpo.GetEventBridge())

	// Initialize the agent
	if err := agent.Initialize(context.Background()); err != nil {
		return nil, fmt.Errorf("failed to initialize critique agent: %w", err)
	}

	// Register workspace tools if available
	if tpo.WorkspaceTools != nil && tpo.WorkspaceToolExecutors != nil {
		if err := tpo.RegisterWorkspaceTools(agent); err != nil {
			tpo.AgentTemplate.GetLogger().Warnf("⚠️ Failed to register workspace tools for critique agent: %v", err)
		}
	}

	// Connect to event bridge if available
	if err := tpo.ConnectAgentToEventBridge(agent, "todo_planner_critique"); err != nil {
		tpo.AgentTemplate.GetLogger().Warnf("⚠️ Failed to connect critique agent to event bridge: %v", err)
	}

	tpo.critiqueAgent = agent
	return agent, nil
}

// structureExecutionResults processes execution results to extract optimized steps
func (tpo *TodoPlannerOrchestrator) structureExecutionResults(executionResult string) string {
	if executionResult == "" {
		return ""
	}

	// Add structured header to help planning agent parse optimized steps
	structuredResult := fmt.Sprintf(`# STRUCTURED EXECUTION RESULTS FOR PLANNING REFINEMENT

## RAW EXECUTION RESULTS
%s

## OPTIMIZATION PARSING INSTRUCTIONS
The planning agent should extract the following from the execution results above:
1. Steps marked as OPTIMIZED - these have optimal methods and should NOT be re-planned
2. Steps marked as COMPLETED - these are done and should be skipped
3. Steps marked as IN_PROGRESS - these are being optimized and should continue
4. Steps marked as PENDING - these need optimization in the next iteration

## KEY SECTIONS TO PARSE
- "OPTIMIZED STEPS SUMMARY" - Contains steps with optimal methods
- "Step Optimization Status" - Shows which steps are optimized
- "Step Optimization Details" - Contains optimal methods and commands
- "Accomplished Steps" - Shows completed steps with evidence

Focus on preserving optimized methods and only refining steps that need improvement.`, executionResult)

	return structuredResult
}

// IterationStrategy defines the approach for each iteration phase
type IterationStrategy struct {
	Name             string
	Focus            string
	PlanningPhase    string
	ExecutionPhase   string
	ValidationPhase  string
	WriterPhase      string
	StoppingCriteria string
}

// getIterationStrategy determines the strategy based on current iteration
func (tpo *TodoPlannerOrchestrator) getIterationStrategy(iteration, maxIterations int) IterationStrategy {
	// Early iterations (1-3): Optimization & Method Discovery
	if iteration <= 3 {
		return IterationStrategy{
			Name:             "Optimization & Method Discovery",
			Focus:            "Find the best possible methods and approaches",
			PlanningPhase:    "Creating exploration plan to discover optimal methods",
			ExecutionPhase:   "Exploring and discovering optimal methods for each step",
			ValidationPhase:  "Validating discovered methods and approaches",
			WriterPhase:      "Creating todo list based on discovered optimal methods",
			StoppingCriteria: "Have we discovered the best possible methods for all critical steps?",
		}
	}

	// Middle iterations (4-6): Refinement & Validation
	if iteration <= 6 {
		return IterationStrategy{
			Name:             "Refinement & Validation",
			Focus:            "Refine the best methods and validate they work consistently",
			PlanningPhase:    "Refining plan based on proven optimal methods",
			ExecutionPhase:   "Testing and validating optimal methods",
			ValidationPhase:  "Validating method reliability and reproducibility",
			WriterPhase:      "Updating todo list with validated optimal methods",
			StoppingCriteria: "Have we validated that our optimal methods work consistently?",
		}
	}

	// Later iterations (7-10): Completion & Execution
	return IterationStrategy{
		Name:             "Completion & Execution",
		Focus:            "Complete the remaining steps using proven optimal methods",
		PlanningPhase:    "Finalizing plan with proven optimal methods",
		ExecutionPhase:   "Executing remaining steps using proven optimal methods",
		ValidationPhase:  "Validating completion of remaining steps",
		WriterPhase:      "Finalizing todo list with proven optimal methods",
		StoppingCriteria: "Have we completed the objective using optimal methods?",
	}
}
