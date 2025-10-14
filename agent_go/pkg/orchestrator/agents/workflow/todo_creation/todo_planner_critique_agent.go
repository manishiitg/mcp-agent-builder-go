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

// TodoPlannerCritiqueTemplate holds template variables for todo planning critique prompts
type TodoPlannerCritiqueTemplate struct {
	Objective         string
	InputData         string // The execution/validation data to critique
	InputPrompt       string // The prompt used to generate the data
	RefinementHistory string
	Iteration         int
	WorkspacePath     string // Added for todo_creation context
}

// TodoPlannerCritiqueAgent extends BaseOrchestratorAgent with todo planning critique functionality
type TodoPlannerCritiqueAgent struct {
	*agents.BaseOrchestratorAgent // ✅ REUSE: All base functionality
}

// NewTodoPlannerCritiqueAgent creates a new todo planner critique agent
func NewTodoPlannerCritiqueAgent(config *agents.OrchestratorAgentConfig, logger utils.ExtendedLogger, tracer observability.Tracer, eventBridge interface{}) *TodoPlannerCritiqueAgent {
	baseAgent := agents.NewBaseOrchestratorAgentWithEventBridge(
		config,
		logger,
		tracer,
		agents.TodoPlannerCritiqueAgentType, // 🆕 NEW: Agent type
		eventBridge,
	)

	return &TodoPlannerCritiqueAgent{
		BaseOrchestratorAgent: baseAgent, // ✅ REUSE: All base functionality
	}
}

// GetBaseAgent implements the OrchestratorAgent interface
func (tpca *TodoPlannerCritiqueAgent) GetBaseAgent() *agents.BaseAgent {
	return tpca.BaseOrchestratorAgent.BaseAgent()
}

// Execute implements the OrchestratorAgent interface
func (tpca *TodoPlannerCritiqueAgent) Execute(ctx context.Context, templateVars map[string]string, conversationHistory []llms.MessageContent) (string, error) {
	// Extract required parameters
	objective, ok := templateVars["objective"]
	if !ok {
		objective = "No objective provided"
	}

	workspacePath, ok := templateVars["workspace_path"]
	if !ok {
		workspacePath = "No workspace path provided"
	}

	iteration := 1
	if iterStr, ok := templateVars["iteration"]; ok {
		var iter int
		if _, err := fmt.Sscanf(iterStr, "%d", &iter); err == nil {
			iteration = iter
		}
	}

	// Prepare template variables
	critiqueTemplateVars := map[string]string{
		"objective":      objective,
		"iteration":      fmt.Sprintf("%d", iteration),
		"workspace_path": workspacePath,
	}

	// Execute using input processor
	return tpca.ExecuteWithInputProcessor(ctx, critiqueTemplateVars, tpca.todoPlannerCritiqueInputProcessor, conversationHistory)
}

// todoPlannerCritiqueInputProcessor processes inputs specifically for todo planning critique
func (tpca *TodoPlannerCritiqueAgent) todoPlannerCritiqueInputProcessor(templateVars map[string]string) string {
	// Create template data
	templateData := TodoPlannerCritiqueTemplate{
		Objective:         templateVars["objective"],
		InputData:         templateVars["input_data"],
		InputPrompt:       templateVars["input_prompt"],
		RefinementHistory: templateVars["refinement_history"],
		WorkspacePath:     templateVars["workspace_path"],
		Iteration: func() int {
			var iter int
			if _, err := fmt.Sscanf(templateVars["iteration"], "%d", &iter); err == nil {
				return iter
			}
			return 1
		}(),
	}

	// Define the template
	templateStr := `## PRIMARY TASK - EVALUATE TODO LIST QUALITY, REPRODUCIBILITY, AND VALIDATE OPTIMIZATION CLAIMS

You are a todo planner critique agent. Your ONLY job is to evaluate if the todo list covers ALL aspects of the objective, has properly broken down reproducible steps, and represents the best possible optimized approach.

**OBJECTIVE**: {{.Objective}}
**PLANNING ITERATION**: {{.Iteration}}
**WORKSPACE PATH**: {{.WorkspacePath}}

## First: Read All Relevant Files for Full Context

Use workspace tools to read the following files to understand the complete planning process:

### 1. Planning Files
- **{{.WorkspacePath}}/todo_creation/planning/plan.md** - The final plan created

### 2. Execution Files  
- **{{.WorkspacePath}}/todo_creation/execution/execution_results.md** - Comprehensive execution results
- **{{.WorkspacePath}}/todo_creation/execution/completed_steps.md** - Steps that were successfully executed
- **{{.WorkspacePath}}/todo_creation/execution/evidence/** - Any files or outputs created during execution

### 3. Validation Files
- **{{.WorkspacePath}}/todo_creation/validation/execution_validation_report.md** - Execution validation results

### 4. Final Todo List
- **{{.WorkspacePath}}/todo_creation/todo.md** - The final todo list to evaluate

## Core Evaluation Questions

### 1. Objective Coverage Assessment
- **Does the todo list cover ALL aspects of the objective?** (Every requirement, goal, and expectation addressed?)
- **Are there missing elements?** (Gaps in the objective that aren't covered by steps?)
- **Is the scope complete?** (All necessary work included to achieve the objective?)

### 2. Step Quality Assessment
- **Are steps properly broken down?** (Right level of detail, not too high-level or too granular?)
- **Are steps clear and specific?** (Can anyone understand and follow them?)
- **Are steps reproducible?** (Will they work consistently when executed?)
- **Are dependencies clear?** (What needs to happen first, what depends on what?)

### 3. Optimization Assessment
- **Are these the best possible steps?** (Optimal approach based on execution experience?)
- **Are optimization claims backed by evidence?** (Do claimed optimal methods have proof?)
- **Are steps efficient?** (No unnecessary work, streamlined approach?)
- **Does it incorporate execution learnings?** (Uses insights from what worked/didn't work?)

### 4. Execution Completeness Assessment
- **Was the objective fully executed?** (Did execution actually achieve the complete objective?)
- **Are all critical steps completed?** (Were all essential steps successfully executed?)
- **Is the objective achievement verified?** (Is there evidence that the objective was met?)
- **Are there execution gaps?** (Steps that failed or weren't completed that prevent objective achievement?)

## Output Format
**IMPORTANT: Return ONLY clean markdown, not JSON or structured data**

# Todo List Assessment Report

## Objective Coverage Assessment
- **Complete Coverage**: [Excellent/Good/Fair/Poor] - Does the todo list cover ALL aspects of the objective?
- **Missing Elements**: [None/Minor/Major] - Are there gaps in the objective coverage?
- **Scope Completeness**: [Complete/Mostly Complete/Incomplete] - Is all necessary work included?

## Step Quality Assessment
- **Step Breakdown**: [Excellent/Good/Fair/Poor] - Are steps properly broken down with right level of detail?
- **Clarity**: [Excellent/Good/Fair/Poor] - Are steps clear and specific?
- **Reproducibility**: [Excellent/Good/Fair/Poor] - Can anyone follow these steps consistently?
- **Dependencies**: [Clear/Somewhat Clear/Unclear] - Are step dependencies clear?

## Optimization Assessment
- **Best Possible Steps**: [Excellent/Good/Fair/Poor] - Are these the best possible optimized steps?
- **Evidence-Backed Claims**: [Excellent/Good/Fair/Poor] - Are optimization claims supported by evidence?
- **Efficiency**: [Excellent/Good/Fair/Poor] - Is the approach streamlined and efficient?
- **Execution Learning Integration**: [Excellent/Good/Fair/Poor] - Does it incorporate execution insights?
- **Reproducibility**: [Excellent/Good/Fair/Poor] - Can claimed optimizations be consistently reproduced?

## Execution Completeness Assessment
- **Objective Achievement**: [Complete/Partial/Failed] - Was the objective fully executed and achieved?
- **Critical Steps Completed**: [All/Most/Some/None] - Were all essential steps successfully executed?
- **Verification Evidence**: [Strong/Weak/None] - Is there evidence that the objective was met?
- **Execution Gaps**: [None/Minor/Major] - Are there steps that failed or weren't completed?
- **Overall Execution Success**: [Excellent/Good/Fair/Poor] - How successful was the execution overall?

## Key Findings
### Coverage Strengths
- [What aspects of the objective are well covered]

### Coverage Gaps
- [What aspects of the objective are missing or incomplete]

### Step Quality Issues
- [Steps that are unclear, too high-level, or hard to reproduce]

### Optimization Issues
- [Optimization claims that lack evidence or aren't actually optimal]

### Execution Completeness Issues
- [Steps that failed or weren't completed that prevent objective achievement]
- [Gaps in execution that need to be addressed]
- [Evidence issues that prevent verification of objective achievement]

## Recommendations
### Coverage Improvements
1. [Most important missing element to add]
2. [Second most important gap to address]

### Step Quality Improvements
- [Steps that need better breakdown or clarity]
- [Dependencies that need to be clearer]

### Optimization Improvements
- [How to verify optimization claims with evidence]
- [How to ensure steps are actually optimal]

### Execution Completeness Improvements
- [How to address failed or incomplete steps]
- [How to fill execution gaps]
- [How to improve verification evidence for objective achievement]

## Final Assessment
- **Does the todo list cover ALL aspects of the objective?** [Yes/No]
- **Are the steps properly broken down and reproducible?** [Yes/No]
- **Are these the best possible optimized steps?** [Yes/No]
- **Was the objective fully executed and achieved?** [Yes/No]
- **Is this todo list ready for execution?** [Yes/No]
- **Key reason**: [Brief explanation focusing on coverage, step quality, optimization, and execution completeness]

Focus on objective coverage, step quality, optimization assessment, and execution completeness with full context from planning, execution, and validation phases.

` + memory.GetWorkflowMemoryRequirements() + ``

	// Parse and execute the template
	tmpl, err := template.New("todoPlannerCritique").Parse(templateStr)
	if err != nil {
		return fmt.Sprintf("Error parsing template: %v", err)
	}

	var result strings.Builder
	err = tmpl.Execute(&result, templateData)
	if err != nil {
		return fmt.Sprintf("Error executing template: %v", err)
	}

	return result.String()
}

// GetPrompts returns nil since we use input processor
func (tpca *TodoPlannerCritiqueAgent) GetPrompts() interface{} {
	return nil
}
