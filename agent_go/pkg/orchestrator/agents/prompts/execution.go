package prompts

// ExecutionPrompts contains the predefined prompts for execution operations
type ExecutionPrompts struct {
	ExecuteStepPrompt string
}

// NewExecutionPrompts creates a new instance of execution prompts
func NewExecutionPrompts() *ExecutionPrompts {
	mm := NewMemoryManagement()

	// Define agent-specific content
	agentType := "execution"
	agentDescription := "that executes a single step using MCP servers."
	specificContext := "The specific objective, step description, and required MCP servers will be provided in the user message."

	specificInstructions := `## 🎯 STEP-BY-STEP EXECUTION: EXECUTE NEXT INCOMPLETE STEP

**EXECUTION PRIORITY**: Execute the next incomplete step in sequence, then continue if appropriate

## 📋 EXECUTION CHECKLIST:
1. **Read Plan**: Check {{.WorkspacePath}}/plan.md for step details
2. **Find Next Incomplete**: Identify the next step in sequence that is not yet completed
3. **Execute That Step**: Use specified MCP servers and tools for that step
4. **Meet Success Criteria**: Verify all criteria are satisfied for this step
5. **Document Results**: Create evidence file with findings for this step
6. **Continue if Ready**: If this step completed successfully, consider executing the next step

## Current Context
**OBJECTIVE**: {{.Objective}}
**WORKSPACE**: {{.WorkspacePath}}

**EXECUTION GUIDELINES**:
- Execute the next incomplete step in the planned sequence
- Focus on thorough completion of each step before moving to the next
- Use MCP tools as specified in the plan for each step
- Document all findings and evidence for each completed step
- Handle errors gracefully with fallback options
- Continue with subsequent steps if the current step completed successfully
- Keep narrative concise and focused on step-by-step progress`

	outputFormat := `## 📊 **EXECUTION RESULTS**

### Step-by-Step Completion
- **Steps Executed**: [List of step numbers and descriptions completed]
- **Current Step**: [Step number and description]
- **Status**: Completed/Failed/Partial
- **Success Criteria Met**: Yes/No/Partial
- **Key Findings**: Main discoveries from completed steps

### Tools Used
- **Tool Name**: [Tool name and parameters]
- **Command/Query**: [Specific command or query used]
- **Output**: [Relevant output snippet]

### 📁 **CRITICAL FILE OPERATIONS**
- **Evidence Files**: {{.WorkspacePath}}/evidence/step_[N]_results.md - [Status: Created/Updated for each step]

### File Operations Confirmation
- **Evidence Created**: [Confirmation of evidence file creation for each completed step]

### Execution Summary
- **Issues**: Any problems encountered and how they were resolved`

	return &ExecutionPrompts{
		ExecuteStepPrompt: mm.GetBasePromptTemplate(agentType, agentDescription, specificContext, specificInstructions, outputFormat),
	}
}
