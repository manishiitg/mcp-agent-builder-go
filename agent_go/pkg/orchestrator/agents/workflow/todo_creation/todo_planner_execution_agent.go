package todo_creation

import (
	"context"
	"fmt"
	"strings"
	"text/template"

	"mcp-agent/agent_go/internal/observability"
	"mcp-agent/agent_go/internal/utils"
	"mcp-agent/agent_go/pkg/orchestrator/agents"
	"mcp-agent/agent_go/pkg/orchestrator/agents/workflow/memory"

	"github.com/tmc/langchaingo/llms"
)

// TodoPlannerExecutionTemplate holds template variables for execution prompts
type TodoPlannerExecutionTemplate struct {
	Objective     string
	Plan          string
	WorkspacePath string
	Strategy      string
	Focus         string
}

// TodoPlannerExecutionAgent executes the objective using MCP servers to understand requirements
type TodoPlannerExecutionAgent struct {
	*agents.BaseOrchestratorAgent
}

// NewTodoPlannerExecutionAgent creates a new todo planner execution agent
func NewTodoPlannerExecutionAgent(config *agents.OrchestratorAgentConfig, logger utils.ExtendedLogger, tracer observability.Tracer, eventBridge interface{}) *TodoPlannerExecutionAgent {
	baseAgent := agents.NewBaseOrchestratorAgentWithEventBridge(
		config,
		logger,
		tracer,
		agents.TodoPlannerExecutionAgentType,
		eventBridge,
	)

	return &TodoPlannerExecutionAgent{
		BaseOrchestratorAgent: baseAgent,
	}
}

// GetBaseAgent implements the OrchestratorAgent interface
func (tpea *TodoPlannerExecutionAgent) GetBaseAgent() *agents.BaseAgent {
	return tpea.BaseOrchestratorAgent.BaseAgent()
}

// Execute implements the OrchestratorAgent interface
func (tpea *TodoPlannerExecutionAgent) Execute(ctx context.Context, templateVars map[string]string, conversationHistory []llms.MessageContent) (string, error) {
	// Extract objective, plan, and workspace path from template variables
	objective := templateVars["Objective"]
	plan := templateVars["Plan"]
	workspacePath := templateVars["WorkspacePath"]

	// Create default strategy for backward compatibility
	defaultStrategy := IterationStrategy{
		Name:  "Default Strategy",
		Focus: "Execute plan comprehensively",
	}

	// Prepare template variables with strategy
	executionTemplateVars := map[string]string{
		"Objective":     objective,
		"Plan":          plan,
		"WorkspacePath": workspacePath,
		"Strategy":      defaultStrategy.Name,
		"Focus":         defaultStrategy.Focus,
	}

	// Execute using input processor
	return tpea.ExecuteWithInputProcessor(ctx, executionTemplateVars, tpea.executionInputProcessor, conversationHistory)
}

// executionInputProcessor processes inputs specifically for plan execution
func (tpea *TodoPlannerExecutionAgent) executionInputProcessor(templateVars map[string]string) string {
	// Create template data
	templateData := TodoPlannerExecutionTemplate{
		Objective:     templateVars["Objective"],
		Plan:          templateVars["Plan"],
		WorkspacePath: templateVars["WorkspacePath"],
		Strategy:      templateVars["Strategy"],
		Focus:         templateVars["Focus"],
	}

	// Define the template
	templateStr := `## PRIMARY TASK - EXECUTE PLAN BASED ON ITERATION STRATEGY

**OBJECTIVE**: {{.Objective}}
**PLAN**: {{.Plan}}
**WORKSPACE**: {{.WorkspacePath}}
**STRATEGY**: {{.Strategy}}
**FOCUS**: {{.Focus}}

**CORE TASK**: Execute the plan based on the current iteration strategy. Focus on {{.Focus}}.

## Execution Strategy
- **Strategy Focus**: {{.Focus}}
- **Use Appropriate Tools**: Select the best MCP tools for each step type
- **Document Key Results**: Record methods, commands, and outcomes for significant steps
- **Evidence for Critical Steps**: Save evidence only for steps involving magic numbers, assumptions, failures, or decision making
- **Complete Execution**: Execute as many steps as possible in this iteration
- **Method Documentation**: Record what works and what doesn't for future reference

## Workspace Updates
Create/update files in {{.WorkspacePath}}/todo_creation/execution/:
- **execution_results.md**: APPEND execution results (preserve previous iterations)
- **completed_steps.md**: Track of completed steps (evidence only for critical steps)
- **evidence/**: Evidence files only for steps involving magic numbers, assumptions, failures, or decision making

**⚠️ CRITICAL**: 
1. **PRESERVE PREVIOUS WORK**: Always read existing execution files first and preserve all previously completed steps
2. **APPEND ONLY**: Only add new execution results, never overwrite existing completed steps
3. **EXECUTE ALL STEPS**: Try to execute ALL steps in the plan, not just a subset
4. **FILE SCOPE**: Only create, update, or modify files within {{.WorkspacePath}}/todo_creation/ folder structure

` + memory.GetWorkflowMemoryRequirements() + `

## Output Format
# Plan Execution Results

## Execution Strategy Summary
- **Steps Executed**: [Number of steps executed in this iteration]
- **Steps Completed**: [Number of steps successfully completed]
- **Steps Failed**: [Number of steps that failed]
- **Total Steps Processed**: [Total steps handled in this execution]
- **Execution Approach**: Complete - execute ALL steps in the plan in one go

## Steps Executed This Iteration
[Steps that were executed in this iteration:]
- **Step Name**: [Name of step]
- **Status**: [COMPLETED/FAILED/IN_PROGRESS]
- **MCP Server**: [Which MCP server was used - e.g., aws, gitlab, github, filesystem]
- **MCP Tool**: [Exact MCP tool name that was called - e.g., aws_cli_query, gitlab_get_project]
- **Tool Arguments**: [Exact arguments/parameters passed to the tool]
- **Tool Call Result**: [What the tool call returned]
- **Tool Call Duration**: [How long the tool call took]
- **Execution Method**: [How it was executed]
- **Success Criteria Met**: [Whether success criteria were achieved]
- **Evidence Files**: [Files created as evidence - only for critical steps]
- **Verification**: [How to verify this step was completed]
- **Execution Time**: [How long it took]
- **Output**: [What was produced]
- **Evidence Required**: [YES/NO - Only YES for steps involving magic numbers, assumptions, failures, or decision making]

## Previously Completed Steps (Preserve These)
[Steps that were completed in previous iterations:]
- **Step Name**: [Name of completed step]
- **Status**: [COMPLETED - Keep results]
- **MCP Server**: [Which MCP server was used]
- **MCP Tool**: [Exact MCP tool name that worked]
- **Tool Arguments**: [Exact arguments that were successful]
- **Tool Call Result**: [What the tool call produced]
- **Execution Method**: [Method that worked]
- **Evidence Files**: [Reference to evidence files - only for critical steps]
- **Verification**: [How to verify this step]
- **Action**: [PRESERVE - Keep completed results]

## Steps Still Pending
[Steps that are still waiting to be executed:]
- **Step Name**: [Name of step]
- **Status**: [PENDING - Not yet executed]
- **Reason**: [Why it wasn't executed this iteration]
- **Dependencies**: [What must be completed first]
- **Next Iteration Priority**: [HIGH/MEDIUM/LOW]

## Execution Summary
- **Total Steps in Plan**: [Total number of steps]
- **Steps Completed Previously**: [Number of steps completed in previous iterations]
- **Steps Completed This Iteration**: [Number of steps completed in this iteration]
- **Total Steps Completed**: [Total completed steps across all iterations]
- **Steps Pending**: [Number of steps still needing execution]
- **Execution Progress**: [Percentage complete]

## Evidence Files
[List of files created or outputs generated during execution]

## Key Insights for Execution
[Important discoveries about execution strategies, tool selection, and what works best]

## Failed Steps (Learning Opportunities)
[Steps that didn't work and why:]
- **Step**: [Step name]
- **Why It Failed**: [Reasons for failure]
- **Lessons Learned**: [What we learned from the failure]
- **Better Approaches**: [What might work better]

## Successful Patterns
[Reusable patterns discovered:]
- **Pattern Name**: [Name of the pattern]
- **Description**: [What the pattern does]
- **When to Use**: [When this pattern is applicable]
- **Implementation**: [How to implement this pattern]
- **Success Rate**: [How often this pattern works]

## Evidence Requirements
**Evidence Required For:**
- **Magic Numbers**: Steps involving specific numbers, metrics, or quantitative claims
- **Assumptions**: Steps where assumptions are made about data, tools, or processes
- **Failures**: Steps that failed and need analysis of why they failed
- **Decision Making**: Steps where decisions are made between alternatives

**No Evidence Required For:**
- **Routine Steps**: Simple, straightforward steps that are clearly completed
- **Standard Operations**: Common operations like file creation, basic commands
- **Clear Success**: Steps where success is obvious from the output

Focus on executing ALL steps efficiently while collecting evidence only for critical decision points.`

	// Parse and execute the template
	tmpl, err := template.New("execution").Parse(templateStr)
	if err != nil {
		return fmt.Sprintf("Error parsing execution template: %v", err)
	}

	var result strings.Builder
	if err := tmpl.Execute(&result, templateData); err != nil {
		return fmt.Sprintf("Error executing execution template: %v", err)
	}

	return result.String()
}
