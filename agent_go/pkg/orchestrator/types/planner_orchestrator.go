package types

import (
	"context"
	"fmt"
	"strings"
	"time"

	"mcp-agent/agent_go/internal/observability"
	"mcp-agent/agent_go/internal/utils"
	"mcp-agent/agent_go/pkg/events"
	"mcp-agent/agent_go/pkg/mcpagent"
	"mcp-agent/agent_go/pkg/orchestrator"
	"mcp-agent/agent_go/pkg/orchestrator/agents"

	"github.com/tmc/langchaingo/llms"
)

// OrchestratorState represents the complete state of the orchestrator
type OrchestratorState struct {
	// Execution state
	CurrentIteration int `json:"current_iteration"`
	CurrentStepIndex int `json:"current_step_index"`
	MaxIterations    int `json:"max_iterations"`

	// Results from each phase
	PlanningResults     []string `json:"planning_results"`
	ExecutionResults    []string `json:"execution_results"`
	ValidationResults   []string `json:"validation_results"`
	OrganizationResults []string `json:"organization_results"`
	ReportResults       []string `json:"report_results"`

	// Current phase context
	CurrentPhase       string `json:"current_phase"` // "planning", "execution", "validation", "organizer"
	ShouldContinue     bool   `json:"should_continue"`
	LastPlanningResult string `json:"last_planning_result"`

	// Configuration state
	Objective       string   `json:"objective"`
	SelectedServers []string `json:"selected_servers"`

	// Timing
	StartTime      time.Time `json:"start_time"`
	LastUpdateTime time.Time `json:"last_update_time"`

	// Conversation history
	ConversationHistory []llms.MessageContent `json:"conversation_history"`
}

// LLMConfig represents the LLM configuration from frontend
type LLMConfig struct {
	Provider              string                        `json:"provider"`
	ModelID               string                        `json:"model_id"`
	FallbackModels        []string                      `json:"fallback_models"`
	CrossProviderFallback *agents.CrossProviderFallback `json:"cross_provider_fallback,omitempty"`
}

// PlannerOrchestrator handles the flow from planning agent to execution agent
type PlannerOrchestrator struct {
	// Base orchestrator for common functionality
	*orchestrator.BaseOrchestrator

	// Planner-specific agents
	planningAgent   agents.OrchestratorAgent
	executionAgent  agents.OrchestratorAgent
	validationAgent agents.OrchestratorAgent
	organizerAgent  agents.OrchestratorAgent
	reportAgent     agents.OrchestratorAgent

	// Configuration
	provider      string
	model         string
	mcpConfigPath string
	temperature   float64
	agentMode     string

	// Structured Output LLM Configuration
	structuredOutputProvider string
	structuredOutputModel    string
	structuredOutputTemp     float64

	// Selected servers for execution
	selectedServers []string

	// Detailed LLM configuration from frontend
	llmConfig *LLMConfig

	// Tracer for observability
	tracer observability.Tracer

	// Conversation history for context
	conversationHistory []llms.MessageContent

	// Orchestrator utils for agent management
	orchestratorUtils *OrchestratorUtils

	// Workspace path extracted from objective
	workspacePath string

	// Planner-specific state management
	currentIteration int
	currentStepIndex int
	maxIterations    int

	// Planner-specific results storage
	planningResults     []string
	executionResults    []string
	validationResults   []string
	organizationResults []string
	reportResults       []string
}

// NewPlannerOrchestrator creates a new planner orchestrator with default configurations
func NewPlannerOrchestrator(logger utils.ExtendedLogger, agentMode string, structuredOutputProvider, structuredOutputModel string, structuredOutputTemp float64) *PlannerOrchestrator {
	return &PlannerOrchestrator{
		agentMode:                agentMode,
		structuredOutputProvider: structuredOutputProvider,
		structuredOutputModel:    structuredOutputModel,
		structuredOutputTemp:     structuredOutputTemp,
		// Initialize planner-specific state
		currentIteration:    0,
		currentStepIndex:    0,
		maxIterations:       10,
		planningResults:     make([]string, 0),
		executionResults:    make([]string, 0),
		validationResults:   make([]string, 0),
		organizationResults: make([]string, 0),
		reportResults:       make([]string, 0),
	}
}

// InitializeAgents initializes the planning and execution agents with provided configurations
func (po *PlannerOrchestrator) InitializeAgents(ctx context.Context, provider, model, mcpConfigPath string, tracerID observability.TraceID, temperature float64, agentEventBridge EventBridge, selectedServers []string, cacheOnly bool, llmConfig *LLMConfig, tracer observability.Tracer, logger utils.ExtendedLogger, customTools []llms.Tool, customToolExecutors map[string]interface{}) error {

	// Store configuration values
	po.provider = provider
	po.model = model
	po.mcpConfigPath = mcpConfigPath
	po.temperature = temperature
	po.selectedServers = selectedServers
	po.llmConfig = llmConfig
	po.tracer = tracer

	// Create base orchestrator
	config := po.createAgentConfig("planner", "planner-orchestrator", 100, logger)
	baseOrchestrator, err := orchestrator.NewBaseOrchestrator(
		config,
		logger,
		tracer,
		agentEventBridge,
		agents.PlannerOrchestratorAgentType,
		orchestrator.OrchestratorTypePlanner,
	)
	if err != nil {
		return fmt.Errorf("failed to create base orchestrator: %w", err)
	}

	po.BaseOrchestrator = baseOrchestrator

	// Store custom tools for use during agent setup
	po.SetWorkspaceTools(customTools, customToolExecutors)

	// Create Planning Agent
	po.AgentTemplate.GetLogger().Infof("📋 Creating Planning Agent...")

	planningConfig := po.createAgentConfig("planning", "test-planning-agent", 100, logger)
	planningAgent := agents.NewOrchestratorPlanningAgent(planningConfig, po.AgentTemplate.GetLogger(), po.tracer, agentEventBridge)

	// Use shared setup function
	if err := po.setupAgent(planningAgent, "planning", "planning agent", agentEventBridge); err != nil {
		return fmt.Errorf("failed to setup planning agent: %w", err)
	}

	po.planningAgent = planningAgent
	po.AgentTemplate.GetLogger().Infof("✅ Planning Agent created and initialized successfully")

	// Create Validation Agent
	po.AgentTemplate.GetLogger().Infof("🔍 Creating Validation Agent...")
	validationConfig := po.createAgentConfig("validation", "test-validation-agent", 100, logger)

	validationAgent := agents.NewOrchestratorValidationAgent(validationConfig, po.AgentTemplate.GetLogger(), po.tracer, agentEventBridge)

	// Use shared setup function
	if err := po.setupAgent(validationAgent, "validation", "validation agent", agentEventBridge); err != nil {
		return fmt.Errorf("failed to setup validation agent: %w", err)
	}

	po.validationAgent = validationAgent
	po.AgentTemplate.GetLogger().Infof("✅ Validation Agent created successfully")

	// Create Organizer Agent
	po.AgentTemplate.GetLogger().Infof("📊 Creating Organizer Agent...")
	organizerConfig := po.createAgentConfig("plan_organizer", "plan-organizer-step-1", 100, logger)

	organizerAgent := agents.NewPlanOrganizerAgent(organizerConfig, po.AgentTemplate.GetLogger(), po.tracer, agentEventBridge)

	// Use shared setup function
	if err := po.setupAgent(organizerAgent, "organizer", "organizer agent", agentEventBridge); err != nil {
		return fmt.Errorf("failed to setup organizer agent: %w", err)
	}

	po.organizerAgent = organizerAgent
	po.AgentTemplate.GetLogger().Infof("✅ Organizer Agent created successfully")

	// Create Report Agent
	po.AgentTemplate.GetLogger().Infof("📊 Creating Report Agent...")
	reportConfig := po.createAgentConfig("report_generation", "test-report-agent", 100, logger)

	reportAgent := agents.NewOrchestratorReportAgent(reportConfig, po.AgentTemplate.GetLogger(), po.tracer, agentEventBridge)

	// Use shared setup function
	if err := po.setupAgent(reportAgent, "report_generation", "report agent", agentEventBridge); err != nil {
		return fmt.Errorf("failed to setup report agent: %w", err)
	}

	po.reportAgent = reportAgent
	po.AgentTemplate.GetLogger().Infof("✅ Report Agent created successfully")

	// Create Execution Agent
	po.AgentTemplate.GetLogger().Infof("🚀 Creating Execution Agent...")
	executionConfig := po.createAgentConfig("execution", "test-execution-agent", 100, logger)

	executionAgent := agents.NewOrchestratorExecutionAgent(ctx, executionConfig, po.AgentTemplate.GetLogger(), po.tracer, agentEventBridge)

	// Use shared setup function
	if err := po.setupAgent(executionAgent, "execution", "execution agent", agentEventBridge); err != nil {
		return fmt.Errorf("failed to setup execution agent: %w", err)
	}

	po.executionAgent = executionAgent
	po.AgentTemplate.GetLogger().Infof("✅ Execution Agent created successfully")

	return nil
}

// setupAgent performs common agent setup tasks using shared utilities
func (po *PlannerOrchestrator) setupAgent(agent agents.OrchestratorAgent, agentType, agentName string, agentEventBridge EventBridge) error {
	// Create orchestrator config
	config := &OrchestratorConfig{
		Provider:        po.provider,
		Model:           po.model,
		MCPConfigPath:   po.mcpConfigPath,
		Temperature:     po.temperature,
		SelectedServers: po.selectedServers,
		AgentMode:       po.agentMode,
		Logger:          po.AgentTemplate.GetLogger(),
	}

	utils := newOrchestratorUtils(config)
	po.orchestratorUtils = utils // Store reference for later context updates

	// Use shared setup function with custom tools passed during initialization
	return utils.setupAgent(
		agent,
		agentType,
		agentName,
		po.WorkspaceTools, // Use custom tools passed during InitializeAgents
		po.WorkspaceToolExecutors,
		agentEventBridge,
		nil, // Context setting function - planner doesn't have specific context setting
	)
}

// createAgentConfig creates a generic agent configuration using shared utilities
func (po *PlannerOrchestrator) createAgentConfig(agentType, agentName string, maxTurns int, logger utils.ExtendedLogger) *agents.OrchestratorAgentConfig {
	config := &OrchestratorConfig{
		Provider:        po.provider,
		Model:           po.model,
		MCPConfigPath:   po.mcpConfigPath,
		Temperature:     po.temperature,
		SelectedServers: po.selectedServers,
		AgentMode:       po.agentMode,
		Logger:          logger,
	}

	utils := newOrchestratorUtils(config)

	setupConfig := &AgentSetupConfig{
		AgentType:    agentType,
		AgentName:    agentName,
		MaxTurns:     maxTurns,
		AgentMode:    po.agentMode,
		OutputFormat: agents.OutputFormatStructured,
	}

	// Convert LLMConfig to shared format
	var llmConfig *SharedLLMConfig
	if po.llmConfig != nil {
		llmConfig = &SharedLLMConfig{
			Provider:              po.llmConfig.Provider,
			ModelID:               po.llmConfig.ModelID,
			FallbackModels:        po.llmConfig.FallbackModels,
			CrossProviderFallback: po.llmConfig.CrossProviderFallback,
		}
	}

	return utils.createAgentConfigWithLLM(setupConfig, llmConfig)
}

// ExecuteFlow executes the complete orchestrator flow from planning to execution
func (po *PlannerOrchestrator) ExecuteFlow(ctx context.Context, objective string, conversationHistory []llms.MessageContent, agentEventBridge EventBridge) (string, error) {
	// Set objective and conversation history
	po.conversationHistory = conversationHistory

	if len(conversationHistory) > 0 {
		po.AgentTemplate.GetLogger().Infof("📚 Loaded %d messages from conversation history", len(conversationHistory))
	}

	// Extract workspace path from objective
	po.workspacePath = extractWorkspacePathFromObjective(objective)
	if po.workspacePath != "" {
		po.AgentTemplate.GetLogger().Infof("📁 Using workspace path: %s", po.workspacePath)
	} else {
		po.AgentTemplate.GetLogger().Warnf("⚠️ No workspace path found in objective")
	}

	// Orchestrator start event is now automatically emitted by BasePlannerOrchestrator.Execute()

	// Helper function to emit orchestrator error event
	emitOrchestratorError := func(err error, context string) {
		if po.GetEventBridge() != nil {
			duration := time.Since(po.GetStartTime())
			orchestratorErrorEvent := &events.OrchestratorErrorEvent{
				BaseEventData: events.BaseEventData{
					Timestamp: time.Now(),
				},
				Context:  context,
				Error:    err.Error(),
				Duration: duration,
			}

			// Create unified event wrapper
			unifiedEvent := &events.AgentEvent{
				Type:      events.OrchestratorError,
				Timestamp: time.Now(),
				Data:      orchestratorErrorEvent,
			}

			// Emit through the bridge
			if bridge, ok := po.GetEventBridge().(mcpagent.AgentEventListener); ok {
				bridge.HandleEvent(ctx, unifiedEvent)
				po.AgentTemplate.GetLogger().Infof("✅ Emitted orchestrator error event: %s", context)
			}
		}
	}

	// Initialize variables for the iterative loop
	maxIterations := po.GetMaxIterations()
	currentStepIndex := po.GetCurrentStepIndex()
	executionResults := po.GetExecutionResults()
	validationResults := po.GetValidationResults()
	startIteration := po.GetCurrentIteration()

	po.AgentTemplate.GetLogger().Infof("🔄 Starting iterative execution (max %d iterations) from iteration %d", maxIterations, startIteration)

	// Main iterative loop - continue from current state
	for iteration := startIteration; iteration < maxIterations; iteration++ {
		po.AgentTemplate.GetLogger().Infof("🔄 Iteration %d/%d", iteration+1, maxIterations)

		// ✅ PLANNING PHASE - Determine next step or workflow completion

		// Build context based on iteration
		var executionResultsStr, validationResultsStr, reportResultsStr string

		if iteration > 0 {
			// Step 2 onwards: Include actual content from previous iterations
			if len(executionResults) > 0 {
				executionResultsStr = strings.Join(executionResults, "\n\n")
			} else {
				executionResultsStr = "No previous execution results"
			}

			if len(validationResults) > 0 {
				validationResultsStr = strings.Join(validationResults, "\n\n")
			} else {
				validationResultsStr = "No previous validation results"
			}

			if len(po.GetReportResults()) > 0 {
				reportResultsStr = strings.Join(po.GetReportResults(), "\n\n")
			} else {
				reportResultsStr = "No previous report results"
			}

		} else {
			// First step: No previous execution results
			executionResultsStr = ""
			validationResultsStr = ""
			reportResultsStr = ""
		}

		// Prepare planning template variables with current context
		planningTemplateVars := map[string]string{
			"Objective":         objective,
			"ExecutionResults":  executionResultsStr,
			"ValidationResults": validationResultsStr,
			"ReportResults":     reportResultsStr,
			"WorkspacePath":     po.workspacePath,
		}

		// Get next step decision from planning agent

		// Set orchestrator context for planning agent
		po.planningAgent.SetOrchestratorContext(currentStepIndex, iteration, objective, "planning-agent")
		// Also update the context-aware event bridge
		if po.orchestratorUtils != nil {
			po.orchestratorUtils.UpdateAgentContext("planning agent", "planning", currentStepIndex, iteration)
		}

		// Use Execute method to get structured response from planning agent with guidance
		planningTemplateVars["Objective"] = objective
		planningResult, err := po.planningAgent.Execute(ctx, planningTemplateVars, po.conversationHistory)

		if err != nil {
			po.AgentTemplate.GetLogger().Errorf("❌ Planning failed: %v", err)
			emitOrchestratorError(err, "planning phase")
			return "", fmt.Errorf("planning failed: %w", err)
		}

		// Extract should_continue from the raw planning result using conditional LLM
		shouldContinue := po.extractShouldContinue(ctx, planningResult)

		// Store planning result for this iteration
		po.AddPlanningResult(planningResult)

		// Check if we should continue - BREAK if planning says no
		if !shouldContinue {
			po.AgentTemplate.GetLogger().Infof("✅ Workflow completion confirmed by planning agent")
			break
		}

		// Execute the current step
		po.AgentTemplate.GetLogger().Infof("🚀 Executing step %d", currentStepIndex+1)

		// Use the pre-created execution agent
		if po.executionAgent == nil {
			po.AgentTemplate.GetLogger().Errorf("❌ Execution agent not initialized")
			emitOrchestratorError(fmt.Errorf("execution agent not initialized"), "execution phase")
			return "", fmt.Errorf("execution agent not initialized")
		}

		// Set orchestrator context for execution agent
		executionObjective := fmt.Sprintf("Execute step %d: %s", currentStepIndex+1, planningResult)
		po.executionAgent.SetOrchestratorContext(currentStepIndex, iteration, executionObjective, "execution-agent")
		// Also update the context-aware event bridge
		if po.orchestratorUtils != nil {
			po.orchestratorUtils.UpdateAgentContext("execution agent", "execution", currentStepIndex, iteration)
		}

		// Execute the current step using the raw planning response with guidance
		executionTemplateVars := map[string]string{
			"Objective":     planningResult, // Pass the planning result directly
			"WorkspacePath": po.workspacePath,
		}

		executionResult, err := po.executionAgent.Execute(ctx, executionTemplateVars, po.conversationHistory)

		if err != nil {
			po.AgentTemplate.GetLogger().Errorf("❌ Execution failed for step %d: %v", currentStepIndex+1, err)
			emitOrchestratorError(err, fmt.Sprintf("execution phase - step %d", currentStepIndex+1))
			return "", fmt.Errorf("failed to execute step %d: %w", currentStepIndex+1, err)
		}

		po.AddExecutionResult(executionResult)

		// ✅ VALIDATION PHASE - Validate this step's execution result immediately

		// Set orchestrator context for validation agent
		validationObjective := fmt.Sprintf("Validate step %d execution result against original plan", currentStepIndex+1)
		po.validationAgent.SetOrchestratorContext(currentStepIndex, iteration, validationObjective, "validation-agent")
		// Also update the context-aware event bridge
		if po.orchestratorUtils != nil {
			po.orchestratorUtils.UpdateAgentContext("validation agent", "validation", currentStepIndex, iteration)
		}

		// Prepare validation template variables with guidance
		validationTemplateVars := map[string]string{
			"Objective":        objective,
			"StepDescription":  planningResult, // Pass the original planning result directly
			"ExecutionResults": fmt.Sprintf("Step %d: %s", currentStepIndex+1, executionResult),
			"WorkspacePath":    po.workspacePath,
		}

		stepValidationResult, err := po.validationAgent.Execute(ctx, validationTemplateVars, po.conversationHistory)

		if err != nil {
			po.AgentTemplate.GetLogger().Errorf("❌ Validation failed for step %d: %v", currentStepIndex+1, err)
			// Continue with execution result even if validation fails
			po.AgentTemplate.GetLogger().Warnf("⚠️ Continuing with execution result despite validation failure")
			// Set empty validation result when validation fails
			stepValidationResult = "Validation failed: " + err.Error()
		} else {
		}

		// Store validation results for this step
		po.AddValidationResult(stepValidationResult)

		// ✅ ORGANIZATION PHASE - Organize this step's results immediately

		// Use the stored organizer agent (created during initialization)
		if po.organizerAgent == nil {
			po.AgentTemplate.GetLogger().Errorf("❌ Organizer agent not initialized")
			emitOrchestratorError(fmt.Errorf("organizer agent not initialized"), "organization phase")
			return "", fmt.Errorf("organizer agent not initialized")
		}

		// Execute plan organization for this step with guidance
		organizationTemplateVars := map[string]string{
			"WorkflowContext":  fmt.Sprintf("Step %d of workflow for objective: %s", currentStepIndex+1, objective),
			"PlanningOutput":   planningResult, // Pass the original planning result directly
			"ExecutionOutput":  executionResult,
			"ValidationOutput": stepValidationResult,
			"WorkspacePath":    po.workspacePath,
		}

		organizerObjective := fmt.Sprintf("Organize and consolidate results from step %d for objective: %s", currentStepIndex+1, objective)

		// Set orchestrator context for organizer agent
		po.organizerAgent.SetOrchestratorContext(currentStepIndex, iteration, organizerObjective, "plan-organizer-agent")
		// Also update the context-aware event bridge
		if po.orchestratorUtils != nil {
			po.orchestratorUtils.UpdateAgentContext("organizer agent", "plan_organizer", currentStepIndex, iteration)
		}

		stepOrganizationResult, err := po.organizerAgent.Execute(ctx, organizationTemplateVars, po.conversationHistory)

		if err != nil {
			po.AgentTemplate.GetLogger().Errorf("❌ Step %d organization failed: %v", currentStepIndex+1, err)
		} else {
			// Store the organized results for this step
			po.AddOrganizationResult(stepOrganizationResult)
		}

		// ✅ REPORT GENERATION PHASE - Generate report for this iteration

		// Use the stored report agent (created during initialization)
		if po.reportAgent == nil {
			po.AgentTemplate.GetLogger().Errorf("❌ Report agent not initialized")
			emitOrchestratorError(fmt.Errorf("report agent not initialized"), "report generation phase")
			return "", fmt.Errorf("report agent not initialized")
		}

		// Execute report generation for this step with guidance
		reportTemplateVars := map[string]string{
			"Objective":           objective,
			"PlanningResults":     planningResult, // Current step planning result
			"ExecutionResults":    executionResult,
			"ValidationResults":   stepValidationResult,
			"OrganizationResults": stepOrganizationResult,
			"WorkspacePath":       po.workspacePath,
		}

		reportObjective := fmt.Sprintf("Generate report for step %d of objective: %s", currentStepIndex+1, objective)

		// Set orchestrator context for report agent
		po.reportAgent.SetOrchestratorContext(currentStepIndex, iteration, reportObjective, "report-agent")
		// Also update the context-aware event bridge
		if po.orchestratorUtils != nil {
			po.orchestratorUtils.UpdateAgentContext("report agent", "report_generation", currentStepIndex, iteration)
		}

		reportResult, err := po.reportAgent.Execute(ctx, reportTemplateVars, po.conversationHistory)

		if err != nil {
			po.AgentTemplate.GetLogger().Errorf("❌ Step %d report generation failed: %v", currentStepIndex+1, err)
			// Continue even if report generation fails
			po.AgentTemplate.GetLogger().Warnf("⚠️ Continuing despite report generation failure")
		} else {
			// Store the report result for this step
			po.AddReportResult(reportResult)
		}

		// Move to next step
		currentStepIndex++
		po.SetCurrentStepIndex(currentStepIndex)
		po.SetCurrentIteration(iteration + 1)
	}

	// Prepare final result with iteration-by-iteration breakdown
	finalResult := fmt.Sprintf("Orchestrator completed after %d iterations with %d steps executed.\n\n", startIteration+1, len(po.GetExecutionResults()))

	// Add iteration-by-iteration results
	finalResult += "ITERATION RESULTS:\n"
	finalResult += "=================\n"

	if len(po.GetExecutionResults()) > 0 {
		for i := 0; i < len(po.GetExecutionResults()); i++ {
			finalResult += fmt.Sprintf("ITERATION %d:\n", i+1)
			finalResult += "-----------\n"

			// Planning
			finalResult += "📋 PLANNING:\n"
			if i < len(po.GetPlanningResults()) && po.GetPlanningResults()[i] != "" {
				finalResult += fmt.Sprintf("Raw Response: %s\n", po.GetPlanningResults()[i])
			} else {
				finalResult += "No planning result available\n"
			}
			finalResult += "\n"

			// Execution
			finalResult += "🚀 EXECUTION:\n"
			if i < len(po.GetExecutionResults()) && po.GetExecutionResults()[i] != "" {
				finalResult += po.GetExecutionResults()[i] + "\n"
			} else {
				finalResult += "No execution result available\n"
			}
			finalResult += "\n"

			// Validation
			finalResult += "🔍 VALIDATION:\n"
			if i < len(po.GetValidationResults()) && po.GetValidationResults()[i] != "" {
				finalResult += po.GetValidationResults()[i] + "\n"
			} else {
				finalResult += "No validation result available\n"
			}
			finalResult += "\n"

			// Organization
			finalResult += "📊 ORGANIZATION:\n"
			if i < len(po.GetOrganizationResults()) && po.GetOrganizationResults()[i] != "" {
				finalResult += po.GetOrganizationResults()[i] + "\n"
			} else {
				finalResult += "No organization result available\n"
			}
			finalResult += "\n"

			finalResult += "---\n\n"
		}
	} else {
		finalResult += "No iterations completed\n\n"
	}

	// Add planning decision results if available
	if len(po.GetPlanningResults()) > 0 && po.GetPlanningResults()[len(po.GetPlanningResults())-1] != "" {
		lastPlanningResult := po.GetPlanningResults()[len(po.GetPlanningResults())-1]
		finalResult += "FINAL PLANNING DECISION:\n"
		finalResult += "========================\n"
		finalResult += fmt.Sprintf("Raw Response: %s\n", lastPlanningResult)
	}

	po.AgentTemplate.GetLogger().Infof("🎉 Planner Orchestrator Flow completed successfully after %d iterations!", startIteration+1)

	// Orchestrator end event is now automatically emitted by BasePlannerOrchestrator.Execute()

	return finalResult, nil
}

// GetState returns the current orchestrator state
func (po *PlannerOrchestrator) GetState(objective string) (*OrchestratorState, error) {
	state := &OrchestratorState{
		CurrentIteration:    po.GetCurrentIteration(),
		CurrentStepIndex:    po.GetCurrentStepIndex(),
		MaxIterations:       po.GetMaxIterations(),
		PlanningResults:     po.GetPlanningResults(),
		ExecutionResults:    po.GetExecutionResults(),
		ValidationResults:   po.GetValidationResults(),
		OrganizationResults: po.GetOrganizationResults(),
		ReportResults:       po.GetReportResults(),
		CurrentPhase:        "planning", // Default phase
		ShouldContinue:      true,       // Default to continue
		LastPlanningResult:  "",         // Will be set during execution
		Objective:           objective,
		SelectedServers:     po.selectedServers,
		StartTime:           po.GetStartTime(),
		LastUpdateTime:      time.Now(),
		ConversationHistory: po.conversationHistory,
	}
	return state, nil
}

// RestoreState restores the orchestrator state
func (po *PlannerOrchestrator) RestoreState(state *OrchestratorState) error {
	if state == nil {
		return fmt.Errorf("state cannot be nil")
	}

	po.SetCurrentIteration(state.CurrentIteration)
	po.SetCurrentStepIndex(state.CurrentStepIndex)
	po.SetMaxIterations(state.MaxIterations)
	po.selectedServers = state.SelectedServers
	po.conversationHistory = state.ConversationHistory

	// Restore results arrays using local methods
	po.SetPlanningResults(state.PlanningResults)
	po.SetExecutionResults(state.ExecutionResults)
	po.SetValidationResults(state.ValidationResults)
	po.SetOrganizationResults(state.OrganizationResults)
	po.SetReportResults(state.ReportResults)

	po.AgentTemplate.GetLogger().Infof("✅ Orchestrator state restored - iteration %d, step %d", state.CurrentIteration, state.CurrentStepIndex)
	return nil
}

// Close cleans up the orchestrator resources
func (po *PlannerOrchestrator) Close() {
	if po.planningAgent != nil {
		po.planningAgent.Close()
	}
	if po.executionAgent != nil {
		po.executionAgent.Close()
	}
	if po.validationAgent != nil {
		po.validationAgent.Close()
	}
	if po.organizerAgent != nil {
		po.organizerAgent.Close()
	}
	if po.reportAgent != nil {
		po.reportAgent.Close()
	}
}

// extractShouldContinue uses the conditional LLM to determine if the plan is executable and will achieve the objective
func (po *PlannerOrchestrator) extractShouldContinue(ctx context.Context, rawResponse string) bool {
	return po.extractShouldContinueWithContext(ctx, rawResponse, 0, 0)
}

// extractShouldContinueWithContext uses the conditional LLM to determine if the plan is executable and will achieve the objective
// with step index and iteration context for better event tracking
func (po *PlannerOrchestrator) extractShouldContinueWithContext(ctx context.Context, rawResponse string, stepIndex, iteration int) bool {
	if po.GetConditionalLLM() == nil {
		po.AgentTemplate.GetLogger().Errorf("❌ Conditional LLM not initialized")
		panic("Conditional LLM not initialized - cannot assess objective achievement")
	}

	// Use conditional LLM to make the objective achievement decision
	result, err := po.GetConditionalLLM().Decide(ctx, rawResponse, "Are there any incomplete steps in the plan. Yes or no", stepIndex, iteration)
	if err != nil {
		po.AgentTemplate.GetLogger().Errorf("❌ Conditional LLM objective achievement check failed: %v", err)
		return true // Default to continue if conditional LLM fails
	}

	po.AgentTemplate.GetLogger().Infof("🤔 Conditional LLM objective achievement check: %t", result.GetResult())
	return result.GetResult()
}

// GetWorkspacePath returns the current workspace path
func (po *PlannerOrchestrator) GetWorkspacePath() string {
	return po.workspacePath
}

// Planner-specific state management methods

// GetCurrentIteration returns the current iteration
func (po *PlannerOrchestrator) GetCurrentIteration() int {
	return po.currentIteration
}

// SetCurrentIteration sets the current iteration
func (po *PlannerOrchestrator) SetCurrentIteration(iteration int) {
	po.currentIteration = iteration
}

// GetCurrentStepIndex returns the current step index
func (po *PlannerOrchestrator) GetCurrentStepIndex() int {
	return po.currentStepIndex
}

// SetCurrentStepIndex sets the current step index
func (po *PlannerOrchestrator) SetCurrentStepIndex(stepIndex int) {
	po.currentStepIndex = stepIndex
}

// GetMaxIterations returns the max iterations
func (po *PlannerOrchestrator) GetMaxIterations() int {
	return po.maxIterations
}

// SetMaxIterations sets the max iterations
func (po *PlannerOrchestrator) SetMaxIterations(maxIterations int) {
	po.maxIterations = maxIterations
}

// Planner-specific results management methods

// GetPlanningResults returns the planning results
func (po *PlannerOrchestrator) GetPlanningResults() []string {
	return po.planningResults
}

// AddPlanningResult adds a planning result
func (po *PlannerOrchestrator) AddPlanningResult(result string) {
	po.planningResults = append(po.planningResults, result)
}

// GetExecutionResults returns the execution results
func (po *PlannerOrchestrator) GetExecutionResults() []string {
	return po.executionResults
}

// AddExecutionResult adds an execution result
func (po *PlannerOrchestrator) AddExecutionResult(result string) {
	po.executionResults = append(po.executionResults, result)
}

// GetValidationResults returns the validation results
func (po *PlannerOrchestrator) GetValidationResults() []string {
	return po.validationResults
}

// AddValidationResult adds a validation result
func (po *PlannerOrchestrator) AddValidationResult(result string) {
	po.validationResults = append(po.validationResults, result)
}

// GetOrganizationResults returns the organization results
func (po *PlannerOrchestrator) GetOrganizationResults() []string {
	return po.organizationResults
}

// AddOrganizationResult adds an organization result
func (po *PlannerOrchestrator) AddOrganizationResult(result string) {
	po.organizationResults = append(po.organizationResults, result)
}

// GetReportResults returns the report results
func (po *PlannerOrchestrator) GetReportResults() []string {
	return po.reportResults
}

// AddReportResult adds a report result
func (po *PlannerOrchestrator) AddReportResult(result string) {
	po.reportResults = append(po.reportResults, result)
}

// ClearPlanningResults clears all planning results
func (po *PlannerOrchestrator) ClearPlanningResults() {
	po.planningResults = make([]string, 0)
}

// ClearExecutionResults clears all execution results
func (po *PlannerOrchestrator) ClearExecutionResults() {
	po.executionResults = make([]string, 0)
}

// ClearValidationResults clears all validation results
func (po *PlannerOrchestrator) ClearValidationResults() {
	po.validationResults = make([]string, 0)
}

// ClearOrganizationResults clears all organization results
func (po *PlannerOrchestrator) ClearOrganizationResults() {
	po.organizationResults = make([]string, 0)
}

// ClearReportResults clears all report results
func (po *PlannerOrchestrator) ClearReportResults() {
	po.reportResults = make([]string, 0)
}

// SetPlanningResults sets all planning results
func (po *PlannerOrchestrator) SetPlanningResults(results []string) {
	po.planningResults = make([]string, len(results))
	copy(po.planningResults, results)
}

// SetExecutionResults sets all execution results
func (po *PlannerOrchestrator) SetExecutionResults(results []string) {
	po.executionResults = make([]string, len(results))
	copy(po.executionResults, results)
}

// SetValidationResults sets all validation results
func (po *PlannerOrchestrator) SetValidationResults(results []string) {
	po.validationResults = make([]string, len(results))
	copy(po.validationResults, results)
}

// SetOrganizationResults sets all organization results
func (po *PlannerOrchestrator) SetOrganizationResults(results []string) {
	po.organizationResults = make([]string, len(results))
	copy(po.organizationResults, results)
}

// SetReportResults sets all report results
func (po *PlannerOrchestrator) SetReportResults(results []string) {
	po.reportResults = make([]string, len(results))
	copy(po.reportResults, results)
}
