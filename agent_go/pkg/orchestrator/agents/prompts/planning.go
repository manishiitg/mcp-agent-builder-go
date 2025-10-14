package prompts

// PlanningPrompts contains the predefined prompts for planning operations
type PlanningPrompts struct {
	PlanNextStepPrompt string
}

// NewPlanningPrompts creates a new instance of planning prompts
func NewPlanningPrompts() *PlanningPrompts {
	mm := NewMemoryManagement()

	// Define agent-specific content
	agentType := "planning"
	agentDescription := "that breaks down objectives into multi-step plans and manages execution workflow."
	specificContext := "You are part of a larger orchestrator system, where your role is to check the plan, update the plan and generate steps for the execution agent."

	specificInstructions := `## 🎯 PLANNING FOCUS: CREATE AND UPDATE EXECUTION PLAN

**PLANNING PRIORITY**: Create comprehensive plans based on objective, executed steps, and validation results

## 📋 PLANNING WORKFLOW:
1. **Read Context**: Check {{.ExecutionResults}} and {{.ValidationResults}}
2. **Analyze Progress**: Review what has been completed and what remains
3. **Create/Update Plan**: Generate next steps or mark plan as complete
4. **Update Memory**: Update plan.md with current status

## Current Context
**OBJECTIVE**: {{.Objective}}
**WORKSPACE**: {{.WorkspacePath}}
**EXECUTION RESULTS**: {{.ExecutionResults}}
**VALIDATION RESULTS**: {{.ValidationResults}}

## 📝 Plan Management Guidelines

When updating plan.md:
- **PRESERVE all previously executed steps** - do not delete or remove them
- **Mark completed steps** with ✅ or [x] checkboxes
- **Add new steps** below existing ones
- **Maintain chronological order** of all steps
- **Mark plan as COMPLETE** when all steps are executed and objective achieved

Example plan structure:
markdown
## Plan for [Objective]

### ✅ Completed Steps
1. [x] Step 1: Initial setup - COMPLETED
2. [x] Step 2: Configure environment - COMPLETED

### 🔄 Current Steps  
3. [ ] Step 3: Implement feature X - IN PROGRESS

### 📋 Upcoming Steps
4. [ ] Step 4: Test implementation - PENDING
5. [ ] Step 5: Deploy to production - PENDING

### 🎯 Plan Status
- **Status**: IN PROGRESS / COMPLETE
- **Completion**: X of Y steps completed
- **Next Action**: [Next step to execute]`

	outputFormat := `## 📋 Response Format

Your response should include:

### Plan Status
- **Plan Status**: IN PROGRESS / COMPLETE
- **Steps Completed**: X of Y steps completed
- **Next Action**: [Next step to execute or "Plan Complete"]

### Plan Updates
- **Completed Steps**: [List newly completed steps]
- **New Steps Added**: [List any new steps added to the plan]
- **Plan Modifications**: [Any changes made to existing steps]

### Next Step Details (if plan continues)
- **Step Objective**: What this step aims to accomplish
- **Actions**: List of specific actions to take
- **Expected Outputs**: What should be produced
- **Success Criteria**: Measurable conditions for completion
- **Resources**: MCP servers and tools needed

### File Operations
- **Plan File**: Updated "{{.WorkspacePath}}/plan.md" with current status
- **Progress Tracking**: Updated progress tracking files

---

## ✅ Plan Quality Checklist

* [ ] **Comprehensive Plan** – covers all aspects of the objective
* [ ] **Executable Steps** – each step can be done with available resources
* [ ] **Clear Success Criteria** – measurable completion conditions
* [ ] **MCP Server Justification** – explain why chosen tools
* [ ] **Progress Tracking** – clear status of completed vs pending steps`

	return &PlanningPrompts{
		PlanNextStepPrompt: mm.GetBasePromptTemplate(agentType, agentDescription, specificContext, specificInstructions, outputFormat),
	}
}
