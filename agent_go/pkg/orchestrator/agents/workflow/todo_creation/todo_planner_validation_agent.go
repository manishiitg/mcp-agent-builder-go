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

// TodoPlannerValidationTemplate holds template variables for validation prompts
type TodoPlannerValidationTemplate struct {
	Objective       string
	Plan            string
	ExecutionResult string
	WorkspacePath   string
	Strategy        string
	Focus           string
}

// TodoPlannerValidationAgent validates execution results and assesses quality
type TodoPlannerValidationAgent struct {
	*agents.BaseOrchestratorAgent
}

// NewTodoPlannerValidationAgent creates a new todo planner validation agent
func NewTodoPlannerValidationAgent(config *agents.OrchestratorAgentConfig, logger utils.ExtendedLogger, tracer observability.Tracer, eventBridge interface{}) *TodoPlannerValidationAgent {
	baseAgent := agents.NewBaseOrchestratorAgentWithEventBridge(
		config,
		logger,
		tracer,
		agents.TodoPlannerValidationAgentType,
		eventBridge,
	)

	return &TodoPlannerValidationAgent{
		BaseOrchestratorAgent: baseAgent,
	}
}

// GetBaseAgent implements the OrchestratorAgent interface
func (tpva *TodoPlannerValidationAgent) GetBaseAgent() *agents.BaseAgent {
	return tpva.BaseOrchestratorAgent.BaseAgent()
}

// Execute implements the OrchestratorAgent interface
func (tpva *TodoPlannerValidationAgent) Execute(ctx context.Context, templateVars map[string]string, conversationHistory []llms.MessageContent) (string, error) {
	// Extract objective, plan, execution result, and workspace path from template variables
	objective := templateVars["Objective"]
	plan := templateVars["Plan"]
	executionResult := templateVars["ExecutionResult"]
	workspacePath := templateVars["WorkspacePath"]

	// Create default strategy for backward compatibility
	defaultStrategy := IterationStrategy{
		Name:  "Default Strategy",
		Focus: "Validate execution results",
	}

	// Prepare template variables with strategy
	validationTemplateVars := map[string]string{
		"Objective":       objective,
		"Plan":            plan,
		"ExecutionResult": executionResult,
		"WorkspacePath":   workspacePath,
		"Strategy":        defaultStrategy.Name,
		"Focus":           defaultStrategy.Focus,
	}

	// Execute using input processor
	return tpva.ExecuteWithInputProcessor(ctx, validationTemplateVars, tpva.validationInputProcessor, conversationHistory)
}

// validationInputProcessor processes inputs specifically for execution validation
func (tpva *TodoPlannerValidationAgent) validationInputProcessor(templateVars map[string]string) string {
	// Create template data
	templateData := TodoPlannerValidationTemplate{
		Objective:       templateVars["Objective"],
		Plan:            templateVars["Plan"],
		ExecutionResult: templateVars["ExecutionResult"],
		WorkspacePath:   templateVars["WorkspacePath"],
		Strategy:        templateVars["Strategy"],
		Focus:           templateVars["Focus"],
	}

	// Define the template
	templateStr := `## PRIMARY TASK - VALIDATE EXECUTION RESULTS BASED ON ITERATION STRATEGY

**OBJECTIVE**: {{.Objective}}
**PLAN**: {{.Plan}}
**EXECUTION RESULT**: {{.ExecutionResult}}
**WORKSPACE**: {{.WorkspacePath}}
**STRATEGY**: {{.Strategy}}
**FOCUS**: {{.Focus}}

**CORE TASK**: Validate execution results against the plan based on the current iteration strategy. Focus on {{.Focus}}.

## Validation Strategy
- **Strategy Focus**: {{.Focus}}
- **Focus on Significant Steps**: Only validate steps that significantly impact the objective
- **Verify Major Claims**: Check important technical claims and commands that matter
- **Tool Failure Detection**: Identify execution claims that might be based on tool failures
- **Basic Quality Assessment**: Evaluate the overall quality and reliability of execution results
- **Evidence for Critical Steps**: Only require evidence for steps that are critical to objective success
- **Skip Minor Steps**: Don't burden small, routine steps with evidence requirements
- **Flag Major Issues**: Identify areas where execution agent made unsupported major claims

## Workspace Updates
Create/update files in {{.WorkspacePath}}/todo_creation/validation/:
- **execution_validation_report.md**: Comprehensive validation with evidence verification and execution checking

**⚠️ IMPORTANT**: Only create, update, or modify files within {{.WorkspacePath}}/todo_creation/ folder structure. Do not modify files outside this directory.

## Evidence Verification Instructions
**CRITICAL**: Use both structured data and file reading for comprehensive validation:

### Use Execution Results Parameter:
- **Immediate Context**: Analyze execution results for immediate context and claims
- **Tool Call Claims**: Review MCP server/tool/arguments that were claimed to work
- **Success Claims**: Review steps that were claimed to be completed

### Read Execution Files for Evidence:
- **{{.WorkspacePath}}/todo_creation/execution/execution_results.md**: Comprehensive execution results
- **{{.WorkspacePath}}/todo_creation/execution/completed_steps.md**: Steps that were successfully executed
- **{{.WorkspacePath}}/todo_creation/execution/evidence/**: Evidence files for completed steps

### Verification Process:
1. **Cross-reference claims** in execution results with actual evidence files
2. **Verify tool calls** by checking evidence files for MCP server/tool/arguments
3. **Validate completion** by ensuring evidence files exist for claimed completed steps
` + memory.GetWorkflowMemoryRequirements() + `

## Output Format

## Execution Verification Results
### Validated Steps
[For each step claimed by execution agent, verify if it's backed by evidence:]
- **Step Claim**: [What the execution agent claimed was completed]
- **Evidence Found**: [What evidence supports this step claim]
- **Verification Method**: [How it was verified (MCP tools, file analysis, etc.)]
- **Confidence Level**: [High/Medium/Low based on evidence quality]
- **Status**: [VALIDATED/UNSUPPORTED/INCONCLUSIVE]

### Critical Assumptions Requiring Double-Check
[Even evidence-backed assumptions that need additional verification due to potential impact:]
- **Assumption**: [What the execution agent assumed]
- **Evidence Found**: [What evidence supports this assumption]
- **Criticality Level**: [HIGH/MEDIUM - How detrimental if wrong]
- **Potential Impact**: [What could go wrong if this assumption is incorrect]
- **Double-Check Method**: [Additional verification approach used]
- **Second Verification Result**: [Result of double-checking]
- **Final Status**: [CONFIRMED/QUESTIONABLE/INVALIDATED]

### Tool Failure Risk Assessment
[Assumptions that might be based on tool failures or one-time errors:]
- **Assumption**: [What was assumed]
- **Tool Used**: [Which MCP tool was used]
- **Failure Risk**: [HIGH/MEDIUM/LOW - Risk of tool failure affecting result]
- **Single Point of Failure**: [Whether assumption relies on single tool call]
- **Backup Verification**: [Alternative verification method used]
- **Status**: [RELIABLE/QUESTIONABLE/UNRELIABLE]

### Unsupported Claims
[Claims made by execution agent that lack evidence:]
- **Claim**: [What was claimed without evidence]
- **Missing Evidence**: [What evidence would be needed]
- **Impact**: [How this affects the plan validity]
- **Recommendation**: [How to address this issue]

## Evidence Quality Assessment
### Strong Evidence
[Steps with solid, verifiable evidence:]
- **Step**: [Step name]
- **Evidence Files**: [List of evidence files]
- **Evidence Quality**: [High/Medium/Low]
- **Verification**: [How evidence was verified]

### Weak Evidence
[Steps with insufficient or questionable evidence:]
- **Step**: [Step name]
- **Evidence Issues**: [What's wrong with the evidence]
- **Missing Elements**: [What additional evidence is needed]
- **Recommendation**: [How to improve evidence quality]

## Hallucination Detection
### Suspected Hallucinations
[Claims that appear to be fabricated or unsupported:]
- **Claim**: [What was claimed]
- **Why Suspected**: [Reasons for suspicion]
- **Evidence Check**: [What verification was attempted]
- **Status**: [CONFIRMED HALLUCINATION/LIKELY HALLUCINATION/INCONCLUSIVE]

### Fact-Checking Results
[Results of MCP tool verification:]
- **Claim**: [What was claimed]
- **Verification Tool**: [Which MCP tool was used]
- **Result**: [What the verification found]
- **Accuracy**: [ACCURATE/INACCURATE/INCONCLUSIVE]

## Technical Validation
### Command Verification
[Verify if executed commands actually worked:]
- **Command**: [Command that was executed]
- **Expected Output**: [What was expected]
- **Actual Output**: [What was actually produced]
- **Verification**: [How it was verified]
- **Status**: [VERIFIED/FAILED/INCONCLUSIVE]

### Web Scraping Validation
[Verify web scraping claims:]
- **URL**: [URL that was accessed]
- **Selectors**: [Selectors that were used]
- **Data Extracted**: [What data was claimed to be extracted]
- **Evidence**: [Screenshots, files, or other evidence]
- **Verification**: [How the extraction was verified]
- **Status**: [VERIFIED/UNVERIFIED/FAILED]

## Completed Steps Update (Only Validated Steps)
[For each VALIDATED completed step, update execution/completed_steps.md with:]
- **Step Name**: [Name of completed step]
- **Completion Date**: [When it was completed]
- **Evidence Files**: [Reference to evidence files]
- **Verification Method**: [How it was verified]
- **Evidence Quality**: [High/Medium/Low]
- **Status**: [COMPLETED - VALIDATED]

## Rejected Steps (Insufficient Evidence)
[Steps that cannot be marked as completed due to lack of evidence:]
- **Step Name**: [Name of step]
- **Evidence Issues**: [What's wrong with the evidence]
- **Required Evidence**: [What evidence is needed]
- **Status**: [REJECTED - INSUFFICIENT EVIDENCE]

## Validation Summary
- **Total Steps Analyzed**: [Number]
- **Validated Steps**: [Number with strong evidence]
- **Rejected Steps**: [Number with insufficient evidence]
- **Hallucinations Detected**: [Number of suspected hallucinations]
- **Overall Evidence Quality**: [High/Medium/Low]
- **Plan Reliability**: [High/Medium/Low based on evidence]

## Critical Issues Found
[Major issues that affect plan validity:]
1. [Most critical issue with evidence]
2. [Second most critical issue]
3. [Third most critical issue]

## Detrimental Assumption Analysis
[Assumptions that could severely impact objective achievement if wrong:]
- **Assumption**: [Critical assumption made]
- **Detrimental Impact**: [How this could derail the entire objective]
- **Evidence Quality**: [Quality of supporting evidence]
- **Verification Status**: [Whether double-checked and confirmed]
- **Risk Level**: [HIGH/MEDIUM/LOW - Risk to objective if assumption is wrong]
- **Recommendation**: [How to mitigate this risk]

## Tool Failure Impact Assessment
[Assumptions that could be wrong due to tool failures:]
- **Assumption**: [Assumption that might be tool-failure based]
- **Tool Dependency**: [Which tools this assumption depends on]
- **Failure Scenarios**: [What could go wrong with the tools]
- **Impact on Objective**: [How tool failure could affect the goal]
- **Mitigation Strategy**: [How to prevent or handle tool failures]

## Recommendations for Plan Refinement
[Specific recommendations based on validation results:]
- [How to address unsupported claims]
- [How to improve evidence collection]
- [How to verify assumptions before making them]
- [How to prevent hallucinations in future iterations]

Focus on validating execution results and ensuring evidence-backed claims for plan refinement.`

	// Parse and execute the template
	tmpl, err := template.New("validation").Parse(templateStr)
	if err != nil {
		return fmt.Sprintf("Error parsing validation template: %v", err)
	}

	var result strings.Builder
	if err := tmpl.Execute(&result, templateData); err != nil {
		return fmt.Sprintf("Error executing validation template: %v", err)
	}

	return result.String()
}
