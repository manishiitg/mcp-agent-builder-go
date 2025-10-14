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

// TodoPlannerWriterTemplate holds template variables for todo writing prompts
type TodoPlannerWriterTemplate struct {
	Objective        string
	PlanResult       string
	ExecutionResult  string
	ValidationResult string
	CritiqueResult   string
	WorkspacePath    string
	Strategy         string
	Focus            string
}

// TodoPlannerWriterAgent creates optimal todo list based on execution experience
type TodoPlannerWriterAgent struct {
	*agents.BaseOrchestratorAgent
}

// NewTodoPlannerWriterAgent creates a new todo planner writer agent
func NewTodoPlannerWriterAgent(config *agents.OrchestratorAgentConfig, logger utils.ExtendedLogger, tracer observability.Tracer, eventBridge interface{}) *TodoPlannerWriterAgent {
	baseAgent := agents.NewBaseOrchestratorAgentWithEventBridge(
		config,
		logger,
		tracer,
		agents.TodoPlannerWriterAgentType,
		eventBridge,
	)

	return &TodoPlannerWriterAgent{
		BaseOrchestratorAgent: baseAgent,
	}
}

// GetBaseAgent implements the OrchestratorAgent interface
func (tpwa *TodoPlannerWriterAgent) GetBaseAgent() *agents.BaseAgent {
	return tpwa.BaseOrchestratorAgent.BaseAgent()
}

// Execute implements the OrchestratorAgent interface
func (tpwa *TodoPlannerWriterAgent) Execute(ctx context.Context, templateVars map[string]string, conversationHistory []llms.MessageContent) (string, error) {
	// Extract objective, plan result, execution result, validation result, critique result, and workspace path from template variables
	objective := templateVars["Objective"]
	planResult := templateVars["PlanResult"]
	executionResult := templateVars["ExecutionResult"]
	validationResult := templateVars["ValidationResult"]
	critiqueResult := templateVars["CritiqueResult"]
	workspacePath := templateVars["WorkspacePath"]

	// Create default strategy for backward compatibility
	defaultStrategy := IterationStrategy{
		Name:  "Default Strategy",
		Focus: "Create optimal todo list",
	}

	// Prepare template variables
	writerTemplateVars := map[string]string{
		"Objective":        objective,
		"PlanResult":       planResult,
		"ExecutionResult":  executionResult,
		"ValidationResult": validationResult,
		"CritiqueResult":   critiqueResult,
		"WorkspacePath":    workspacePath,
		"Strategy":         defaultStrategy.Name,
		"Focus":            defaultStrategy.Focus,
	}

	// Execute using input processor
	return tpwa.ExecuteWithInputProcessor(ctx, writerTemplateVars, tpwa.writerInputProcessor, conversationHistory)
}

// writerInputProcessor processes inputs specifically for todo list creation
func (tpwa *TodoPlannerWriterAgent) writerInputProcessor(templateVars map[string]string) string {
	// Create template data
	templateData := TodoPlannerWriterTemplate{
		Objective:        templateVars["Objective"],
		PlanResult:       templateVars["PlanResult"],
		ExecutionResult:  templateVars["ExecutionResult"],
		ValidationResult: templateVars["ValidationResult"],
		CritiqueResult:   templateVars["CritiqueResult"],
		WorkspacePath:    templateVars["WorkspacePath"],
		Strategy:         templateVars["Strategy"],
		Focus:            templateVars["Focus"],
	}

	// Define the template
	templateStr := `## PRIMARY TASK - CREATE COMPREHENSIVE TODO LIST BASED ON ITERATION STRATEGY

**OBJECTIVE**: {{.Objective}}
**PLAN RESULT**: {{.PlanResult}}
**EXECUTION RESULT**: {{.ExecutionResult}}
**VALIDATION RESULT**: {{.ValidationResult}}
**CRITIQUE RESULT**: {{.CritiqueResult}}
**WORKSPACE**: {{.WorkspacePath}}
**STRATEGY**: {{.Strategy}}
**FOCUS**: {{.Focus}}

**CORE TASK**: Create a comprehensive todo list based on the plan and execution experience, ensuring ALL necessary steps are covered for complete objective achievement. Focus on {{.Focus}}.

## Todo Creation Strategy
- **Strategy Focus**: {{.Focus}}
- **Review Comprehensive Plan**: Understand the complete plan created by planning agent
- **Review Execution Results**: Understand which steps were executed and what methods worked
- **Review Validation Feedback**: Address gaps and improvements identified
- **Review Critique Analysis**: Incorporate quality and reproducibility insights
- **Ensure Completeness**: Verify ALL steps needed for objective are included
- **Create Reproducible Steps**: Write steps that can be exactly replicated using proven MCP methods
- **Add Verification Methods**: Include specific ways to verify each step
- **Document Commands**: Include exact commands and procedures that worked
- **Specify Outputs**: Define what each step should produce
- **Create Comprehensive Todo List**: Write comprehensive todo.md file with tool call details
- **Update Workspace**: Save todo list in workspace

## Workspace Updates
Create/update files in {{.WorkspacePath}}/todo_creation/:
- **todo.md**: Production-ready comprehensive todo list

**⚠️ IMPORTANT**: Only create, update, or modify files within {{.WorkspacePath}}/todo_creation/ folder structure. Do not modify files outside this directory.

` + memory.GetWorkflowMemoryRequirements() + `

## Tool Call Details Extraction
**CRITICAL**: Extract exact MCP tool call details using both structured data and file reading:

### Extract from Execution Results Parameter:
- **MCP Server Names**: Which servers were used (e.g., aws, gitlab, github, filesystem)
- **Tool Names**: Exact MCP tool names that worked (e.g., aws_cli_query, gitlab_get_project)
- **Tool Arguments**: Exact parameters that were successful
- **Tool Results**: What each tool call produced
- **Tool Duration**: How long each tool call took
- **Success Patterns**: Which approaches worked consistently

### Read Execution Files for Detailed Evidence:
- **{{.WorkspacePath}}/todo_creation/execution/execution_results.md**: Comprehensive execution results
- **{{.WorkspacePath}}/todo_creation/execution/completed_steps.md**: Steps that were successfully executed
- **{{.WorkspacePath}}/todo_creation/execution/evidence/**: Evidence files for completed steps

### Key Sections to Parse:
- **"Steps Executed This Iteration"**: Contains detailed MCP tool call information
- **"MCP Server"**: Shows which MCP server was used
- **"MCP Tool"**: Shows the exact tool name that was called
- **"Tool Arguments"**: Shows the exact parameters passed
- **"Tool Call Result"**: Shows what the tool returned
- **"Tool Call Duration"**: Shows execution time

### Apply to Todo Steps:
For each step in the todo list, use the proven MCP details:
- **MCP Server**: Use the exact server that worked for similar steps
- **MCP Tool**: Use the exact tool name that succeeded
- **Tool Arguments**: Use the exact arguments that were successful
- **Tool Call Record**: Document the complete tool call details for reproducibility

## Output Format
# Comprehensive Todo List

## Project Overview
- **Objective**: {{.Objective}}
- **Created**: [Current date]
- **Based On**: Comprehensive plan and execution experience
- **Approach**: Systematic execution with documented methods

## Prerequisites
[What needs to be prepared before starting]

## Execution Plan
### Phase 1: [Phase Name]
#### Step 1.1: [Step Name]
- **Description**: [What needs to be done]
- **MCP Server**: [Which MCP server to use - e.g., aws, gitlab, github, filesystem]
- **MCP Tool**: [Specific MCP tool name - e.g., aws_cli_query, gitlab_get_project]
- **Tool Arguments**: [Exact arguments/parameters to pass to the tool]
- **Commands**: [Exact commands to run]
- **Success Criteria**: [Specific, measurable outcome]
- **Verification**: [How to verify the step was completed successfully]
- **Expected Output**: [What should be produced/created]
- **Estimated Effort**: [How long it should take]
- **Dependencies**: [What must be done first]
- **Tool Call Record**: [Will be filled during execution with exact tool call details]

#### Step 1.2: [Step Name]
- **Description**: [What needs to be done]
- **MCP Server**: [Which MCP server to use - e.g., aws, gitlab, github, filesystem]
- **MCP Tool**: [Specific MCP tool name - e.g., aws_cli_query, gitlab_get_project]
- **Tool Arguments**: [Exact arguments/parameters to pass to the tool]
- **Commands**: [Exact commands to run]
- **Success Criteria**: [Specific, measurable outcome]
- **Verification**: [How to verify the step was completed successfully]
- **Expected Output**: [What should be produced/created]
- **Estimated Effort**: [How long it should take]
- **Dependencies**: [What must be done first]
- **Tool Call Record**: [Will be filled during execution with exact tool call details]

### Task-Specific Fields (Add ONLY when relevant)
**Core Fields** (always include): Description, MCP Server, MCP Tool, Tool Arguments, Commands, Success Criteria, Verification, Expected Output, Effort, Dependencies, Tool Call Record

**Additional Fields** (add only when relevant):
- **Web scraping**: URL, Selectors, Data Extraction, Wait Conditions, Screenshots
- **File operations**: File Paths, File Formats, Backup Requirements, Permissions
- **API calls**: Endpoints, Authentication, Rate Limits, Error Handling, Request/Response Format
- **Data processing**: Input/Output Format, Processing Rules, Data Validation, Performance Requirements
- **System admin**: System Commands, User Permissions, Rollback Plan, System Dependencies
- **Database operations**: Database Type, Connection Details, Query/Schema, Backup Strategy, Migration Plan
- **Content creation**: Content Type, Format Requirements, Quality Standards, Review Process
- **Network operations**: Network Configuration, Security Settings, Monitoring Requirements

**Smart Detection**: Analyze each step to determine which additional fields are needed based on the task type.

### Examples by Task Type

**Web Scraping Example:**
- **URL**: https://example-store.com/products
- **Selectors**: ".product-item", ".product-price", ".product-title"
- **Data Extraction**: Product names, prices, and availability status
- **Wait Conditions**: Wait for ".product-item" elements to load completely
- **Screenshots**: Take screenshot after data extraction for verification

**File Operations Example:**
- **File Paths**: Input: "data/raw.csv", Output: "data/processed.json"
- **File Formats**: CSV to JSON conversion with data validation
- **Backup Requirements**: Create backup of original file before processing
- **Permissions**: Ensure read access to input file and write access to output directory

**API Integration Example:**
- **Endpoints**: https://api.example.com/users
- **Authentication**: Bearer token in Authorization header
- **Rate Limits**: Maximum 100 requests per minute
- **Error Handling**: Retry logic for 5xx errors, exponential backoff
- **Request Format**: GET request with query parameters
- **Response Format**: JSON array of user objects

**Data Processing Example:**
- **Input Format**: CSV files with product data
- **Output Format**: JSON files with processed and validated data
- **Processing Rules**: Remove duplicates, validate price formats, standardize categories
- **Data Validation**: Check for required fields, validate data types, range checks
- **Performance Requirements**: Process files under 1MB in under 30 seconds

[Continue for all steps...]

## Step Summary
- **Total Steps**: [Total number of steps]
- **Steps Completed**: [Number of steps that were successfully executed]
- **Steps Pending**: [Number of steps still needing execution]
- **Estimated Total Effort**: [Overall time estimate]

## Quality Assurance
[How to verify each phase and overall success]

## Verification Checklist
- [ ] Each step has specific, measurable success criteria
- [ ] Each step has clear verification methods
- [ ] Each step produces verifiable outputs
- [ ] Commands are exact and reproducible
- [ ] Dependencies are clearly defined
- [ ] Expected outputs are specified

## Risk Management
[High-risk areas and mitigation strategies]

## Execution Summary
[Summary of execution results:]
- **Steps Executed**: [Number of steps that were executed]
- **Key Methods Discovered**: [Main execution approaches that worked]
- **Success Rates**: [Average success rates for executed steps]
- **Common Patterns**: [Reusable patterns discovered during execution]

Focus on creating a comprehensive todo list that covers ALL necessary steps based on the plan and execution experience.`

	// Parse and execute the template
	tmpl, err := template.New("writer").Parse(templateStr)
	if err != nil {
		return fmt.Sprintf("Error parsing writer template: %v", err)
	}

	var result strings.Builder
	if err := tmpl.Execute(&result, templateData); err != nil {
		return fmt.Sprintf("Error executing writer template: %v", err)
	}

	return result.String()
}
