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

// TodoPlannerPlanningTemplate holds template variables for planning prompts
type TodoPlannerPlanningTemplate struct {
	Objective     string
	WorkspacePath string
	Strategy      string
	Focus         string
}

// TodoPlannerPlanningAgent creates a step-wise plan from the objective
type TodoPlannerPlanningAgent struct {
	*agents.BaseOrchestratorAgent
}

// NewTodoPlannerPlanningAgent creates a new todo planner planning agent
func NewTodoPlannerPlanningAgent(config *agents.OrchestratorAgentConfig, logger utils.ExtendedLogger, tracer observability.Tracer, eventBridge interface{}) *TodoPlannerPlanningAgent {
	baseAgent := agents.NewBaseOrchestratorAgentWithEventBridge(
		config,
		logger,
		tracer,
		agents.TodoPlannerPlanningAgentType,
		eventBridge,
	)

	return &TodoPlannerPlanningAgent{
		BaseOrchestratorAgent: baseAgent,
	}
}

// GetBaseAgent implements the OrchestratorAgent interface
func (tppa *TodoPlannerPlanningAgent) GetBaseAgent() *agents.BaseAgent {
	return tppa.BaseOrchestratorAgent.BaseAgent()
}

// Execute implements the OrchestratorAgent interface
func (tppa *TodoPlannerPlanningAgent) Execute(ctx context.Context, templateVars map[string]string, conversationHistory []llms.MessageContent) (string, error) {
	// Extract objective and workspace path from template variables
	objective := templateVars["Objective"]
	workspacePath := templateVars["WorkspacePath"]

	// Create default strategy for backward compatibility
	defaultStrategy := IterationStrategy{
		Name:  "Default Strategy",
		Focus: "Create comprehensive plan",
	}

	// Prepare template variables with strategy
	planningTemplateVars := map[string]string{
		"Objective":     objective,
		"WorkspacePath": workspacePath,
		"Strategy":      defaultStrategy.Name,
		"Focus":         defaultStrategy.Focus,
	}

	// Execute using input processor
	return tppa.ExecuteWithInputProcessor(ctx, planningTemplateVars, tppa.planningInputProcessor, conversationHistory)
}

// planningInputProcessor processes inputs specifically for step-wise planning
func (tppa *TodoPlannerPlanningAgent) planningInputProcessor(templateVars map[string]string) string {
	// Create template data
	templateData := TodoPlannerPlanningTemplate{
		Objective:     templateVars["Objective"],
		WorkspacePath: templateVars["WorkspacePath"],
		Strategy:      templateVars["Strategy"],
		Focus:         templateVars["Focus"],
	}

	// Define the template
	templateStr := `## PRIMARY TASK - CREATE COMPREHENSIVE STEP-WISE PLAN

**OBJECTIVE**: {{.Objective}}
**WORKSPACE**: {{.WorkspacePath}}
**STRATEGY**: {{.Strategy}}
**FOCUS**: {{.Focus}}

**CORE TASK**: Break down the objective into ALL necessary steps and create a comprehensive plan that covers everything needed to achieve the objective. Focus on {{.Focus}}.

## Planning Strategy
- **Analyze Objective**: Understand what needs to be accomplished
- **Break Down ALL Steps**: Divide into small, actionable steps - don't miss any steps
- **Create Comprehensive Plan**: Define ALL steps needed to achieve the objective
- **Ensure Completeness**: Make sure no steps are missing
- **Strategy Focus**: {{.Focus}}
- **Ask Human Feedback**: If there are important questions that could help make a better plan, ask for clarification
- **Update Workspace**: Save the complete plan for execution agent

## Workspace Updates
Create/update files in {{.WorkspacePath}}/todo_creation/planning/:
- **plan.md**: Complete comprehensive step-wise plan

**⚠️ IMPORTANT**: Only create, update, or modify files within {{.WorkspacePath}}/todo_creation/ folder structure. Do not modify files outside this directory.

` + memory.GetWorkflowMemoryRequirements() + `

## Output Format
# Comprehensive Step-Wise Plan

## Objective Analysis
- **Objective**: {{.Objective}}
- **Created**: [Current date]
- **Overall Complexity**: [Simple/Medium/Complex]
- **Total Steps Estimated**: [Number of steps needed]

## Human Feedback Questions
**IMPORTANT**: If there are any important questions that could help create a better plan, ask them here:
- **Question 1**: [If you need clarification on requirements, preferences, constraints, or specific details]
- **Question 2**: [If you need to know about existing resources, tools, or systems]
- **Question 3**: [If you need clarification on success criteria or expected outcomes]
- **Question 4**: [If you need to understand constraints, limitations, or preferences]

**Note**: Only ask questions that would significantly improve the plan quality. If the objective is clear and complete, you may not need to ask any questions.

## Comprehensive Execution Plan
### Step 1: [First Step Name]
- **Description**: [What needs to be done first]
- **Task Type**: [Web scraping, File operations, API calls, Data processing, System admin, Database, Content creation, etc.]
- **MCP Server**: [Which MCP server to use - e.g., aws, gitlab, github, filesystem]
- **MCP Tool**: [Specific MCP tool name - e.g., aws_cli_query, gitlab_get_project]
- **Tool Arguments**: [Exact arguments/parameters to pass to the tool]
- **Success Criteria**: [How to know it's done]
- **Estimated Effort**: [How long it should take]
- **Dependencies**: [What must be prepared first]
- **Status**: [PENDING - Ready for execution]
- **Tool Call Record**: [Will be filled during execution with exact tool call details]

### Step 2: [Second Step Name]
- **Description**: [What needs to be done second]
- **Task Type**: [Web scraping, File operations, API calls, Data processing, System admin, Database, Content creation, etc.]
- **MCP Server**: [Which MCP server to use - e.g., aws, gitlab, github, filesystem]
- **MCP Tool**: [Specific MCP tool name - e.g., aws_cli_query, gitlab_get_project]
- **Tool Arguments**: [Exact arguments/parameters to pass to the tool]
- **Success Criteria**: [How to know it's done]
- **Estimated Effort**: [How long it should take]
- **Dependencies**: [What must be completed first]
- **Status**: [PENDING - Waiting for Step 1]
- **Tool Call Record**: [Will be filled during execution with exact tool call details]

[Continue for ALL steps...]

## Execution Strategy
**EXECUTION APPROACH**: Comprehensive plan execution
- **Step Status Tracking**: 
  - **PENDING**: Not yet executed, waiting for execution
  - **IN_PROGRESS**: Currently being executed
  - **COMPLETED**: Successfully executed with evidence
- **Tool Usage**: Use appropriate MCP tools for each step type
- **Tool Call Recording**: Record exact MCP server, tool name, and arguments that worked
- **Documentation**: Record methods, commands, and patterns for each step
- **Evidence Collection**: Save evidence for both successful and failed attempts
- **Completeness Focus**: Ensure ALL steps are covered and executed
- **Tool Call Details**: For each completed step, record:
  - **MCP Server**: Which server was used (e.g., aws, gitlab, github)
  - **Tool Name**: Exact tool name that worked (e.g., aws_cli_query)
  - **Arguments**: Exact parameters that were successful
  - **Result**: What the tool call produced
  - **Duration**: How long the tool call took

## Step Summary
- **Total Steps**: [Number of steps in plan]
- **Pending Steps**: [Steps not yet executed]
- **In Progress**: [Current step being executed]
- **Completed Steps**: [Steps successfully executed]

Focus on creating a COMPLETE plan that covers ALL necessary steps to achieve the objective.`

	// Parse and execute the template
	tmpl, err := template.New("planning").Parse(templateStr)
	if err != nil {
		return fmt.Sprintf("Error parsing planning template: %v", err)
	}

	var result strings.Builder
	if err := tmpl.Execute(&result, templateData); err != nil {
		return fmt.Sprintf("Error executing planning template: %v", err)
	}

	return result.String()
}

// planRefinementInputProcessor processes inputs specifically for plan refinement
func (tppa *TodoPlannerPlanningAgent) planRefinementInputProcessor(templateVars map[string]string) string {
	// Create template data
	templateData := TodoPlannerPlanningTemplate{
		Objective:     templateVars["Objective"],
		WorkspacePath: templateVars["WorkspacePath"],
	}

	// Define the template
	templateStr := `## PRIMARY TASK - REFINE COMPREHENSIVE STEP-WISE PLAN

**OBJECTIVE**: {{.Objective}}
**WORKSPACE**: {{.WorkspacePath}}
**ITERATION**: ` + templateVars["Iteration"] + `

**PREVIOUS EXECUTION RESULT**: ` + templateVars["PreviousExecutionResult"] + `
**PREVIOUS VALIDATION RESULT**: ` + templateVars["PreviousValidationResult"] + `
**PREVIOUS CRITIQUE RESULT**: ` + templateVars["PreviousCritiqueResult"] + `

**CORE TASK**: Refine the comprehensive step-wise plan based on previous execution experience, validation feedback, and critique feedback.

## Plan Refinement Strategy
- **Analyze Previous Results**: Review what was accomplished in previous iterations
- **Incorporate Validation Feedback**: Use validation results to improve plan
- **Address Critique Feedback**: Use critique results to identify gaps and quality issues
- **Ask Human Feedback**: If there are important questions that could help improve the refined plan, ask for clarification
- **Refine Remaining Steps**: Update only steps that still need to be executed
- **Maintain Completeness**: Ensure ALL steps are still covered
- **Skip Completed Steps**: Don't re-plan steps that are already completed
- **Apply Proven Methods**: Use the exact MCP server/tool/arguments that worked in previous executions
- **Create Updated Plan**: Create a plan focused on remaining work using proven methods

## Workspace Updates
Create/update files in {{.WorkspacePath}}/todo_creation/planning/:
- **refined_plan.md**: The improved comprehensive step-wise plan
- **refinement_notes.md**: Notes on what was changed and why

## Execution Results Analysis
**CRITICAL**: Use both structured data and file reading for comprehensive context and evidence:

### Extract from PreviousExecutionResult Parameter:
- **Completed Steps**: Steps marked as COMPLETED with their execution details
- **MCP Server Details**: Exact MCP server names that were used successfully
- **Tool Information**: Specific MCP tools that worked (e.g., aws_cli_query, gitlab_api_call)
- **Command Arguments**: Exact arguments and parameters that were successful
- **Execution Methods**: The specific approaches that worked for each step type
- **Failed Attempts**: What didn't work and why (to avoid repeating mistakes)

### Key Sections to Parse:
- **"Steps Executed This Iteration"**: Contains detailed execution information
- **"Tools Used"**: Lists the MCP tools that were actually called
- **"Commands Used"**: Shows the exact commands/arguments that worked
- **"Execution Method"**: Describes the successful approach
- **"Previously Completed Steps"**: Steps that worked in prior iterations

### Read Execution Files for Detailed Evidence:
- **{{.WorkspacePath}}/todo_creation/execution/execution_results.md**: Comprehensive execution results
- **{{.WorkspacePath}}/todo_creation/execution/completed_steps.md**: Steps that were successfully executed
- **{{.WorkspacePath}}/todo_creation/execution/evidence/**: Evidence files for completed steps

### MCP Details to Extract:
For each completed step, identify:
- **Server**: Which MCP server (e.g., aws, gitlab, github, filesystem)
- **Tool**: Which specific tool (e.g., aws_cli_query, gitlab_get_project)
- **Arguments**: The exact parameters that worked
- **Success Pattern**: What made this execution successful

### Combine with Structured Data:
- **Use PreviousExecutionResult**: For immediate execution context and results
- **Use PreviousValidationResult**: For validation feedback and quality assessment
- **Use PreviousCritiqueResult**: For critique analysis and improvement suggestions
- **Read Execution Files**: For detailed evidence, tool call specifics, and comprehensive context

This dual approach ensures you have both immediate context from parameters and detailed evidence from files.

**⚠️ IMPORTANT**: Only create, update, or modify files within {{.WorkspacePath}}/todo_creation/ folder structure. Do not modify files outside this directory.

` + memory.GetWorkflowMemoryRequirements() + `

## Output Format
# Refined Comprehensive Step-Wise Plan

## Objective
{{.Objective}}

## Human Feedback Questions
**IMPORTANT**: If there are any important questions that could help improve the refined plan, ask them here:
- **Question 1**: [If you need clarification on requirements, preferences, constraints, or specific details]
- **Question 2**: [If you need to know about existing resources, tools, or systems]
- **Question 3**: [If you need clarification on success criteria or expected outcomes]
- **Question 4**: [If you need to understand constraints, limitations, or preferences]

**Note**: Only ask questions that would significantly improve the refined plan quality. If the objective and feedback are clear, you may not need to ask any questions.

## Refinement Summary
[What was changed based on completion status, execution experience, validation feedback, and critique feedback]

## Feedback Analysis
### Execution Feedback
[Key insights from parsing PreviousExecutionResult:]
- **What Worked**: [Specific MCP servers/tools/arguments that succeeded]
- **MCP Details Extracted**: [Exact server names, tool names, and arguments used]
- **Successful Patterns**: [Specific execution methods that worked]
- **What Failed**: [Failed execution attempts and why they failed]
- **Discovered Methods**: [New MCP tools and approaches found during execution]

### Validation Feedback
[Key insights from validation results:]
- **Successes**: [Steps that were successfully validated]
- **Failures**: [Steps that failed validation]
- **Gaps Identified**: [Missing elements or incomplete work]

### Critique Feedback
[Key insights from critique results:]
- **Quality Issues**: [Problems with todo list quality]
- **Missing Elements**: [Elements that were identified as missing]
- **Improvement Areas**: [Areas that need better planning]

## Completion Status Analysis
### Steps Already Completed (Skip These)
[Steps extracted from PreviousExecutionResult that were successfully executed:]
- **Step Name**: [Name of completed step from execution results]
- **Status**: [COMPLETED - Skip in planning]
- **MCP Server Used**: [Which MCP server was used]
- **Tools Used**: [Specific MCP tools that worked]
- **Arguments Used**: [Exact arguments that were successful]
- **Execution Method**: [How it was executed successfully]
- **Completion Evidence**: [What was produced]
- **Action**: [SKIP - Already completed, use this MCP approach for similar steps]

### Steps Still Pending (Plan These)
[Steps that still need to be executed:]
- **Step Name**: [Name of pending step]
- **Status**: [PENDING - Needs planning]
- **Current Issues**: [What problems were encountered]
- **Refinement Focus**: [What aspects need improvement]
- **Action**: [PLAN - Include in refined plan]

### New Steps Identified
[Steps that were discovered during execution:]
- **Step Name**: [Name of new step]
- **Status**: [PENDING - Newly identified]
- **Reason**: [Why this step is needed]
- **Action**: [ADD - Include in plan]

## Parsed Execution Results Data
[Data extracted from parsing PreviousExecutionResult parameter:]
- **Completed Steps Analysis**: [Steps that succeeded with MCP details]
- **MCP Server Usage**: [Which servers were used successfully]
- **Tool Effectiveness**: [Which MCP tools worked and their arguments]
- **Execution Patterns**: [Successful methods and approaches discovered]
- **Failure Analysis**: [What didn't work and lessons learned]
- **Validation Feedback**: [Validation results from PreviousValidationResult]
- **Critique Feedback**: [Quality assessment from PreviousCritiqueResult]

## Refined Comprehensive Plan
[Detailed, refined sequential steps to achieve the objective - focus on remaining work only]

### Step Status Updates
- **Completed Steps**: [List of steps that were successfully executed - SKIP these]
- **Pending Steps**: [Steps waiting to be executed - PLAN these]
- **Refined Steps**: [List of steps that were improved based on execution experience]

## Key Improvements
[Specific improvements made based on completion status, execution feedback, validation feedback, and critique feedback]

## MCP Method Integration
[How extracted MCP server/tool/arguments information is applied to remaining steps:]

### Proven Method Application:
- **Use Successful MCP Servers**: Apply the same MCP servers that worked for similar step types
- **Reuse Effective Tools**: Use the specific MCP tools that succeeded in previous executions
- **Apply Working Arguments**: Use the exact arguments and parameters that were successful
- **Follow Successful Patterns**: Apply the execution methods that worked consistently

### Learning from Experience:
- **Avoid Failed Approaches**: Don't repeat methods that failed in PreviousExecutionResult
- **Scale Successful Methods**: Apply working MCP approaches to similar remaining steps
- **Optimize Tool Selection**: Choose MCP tools based on what worked in prior iterations

Focus on creating a better plan that addresses remaining work using proven MCP methods from completed steps.`

	// Parse and execute the template
	tmpl, err := template.New("plan_refinement").Parse(templateStr)
	if err != nil {
		return fmt.Sprintf("Error parsing plan refinement template: %v", err)
	}

	var result strings.Builder
	if err := tmpl.Execute(&result, templateData); err != nil {
		return fmt.Sprintf("Error executing plan refinement template: %v", err)
	}

	return result.String()
}
