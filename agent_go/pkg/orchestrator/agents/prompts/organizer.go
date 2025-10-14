package prompts

// PlanOrganizerPrompts contains the predefined prompts for plan organization operations
type PlanOrganizerPrompts struct {
	OrganizeWorkflowPrompt string
}

// NewPlanOrganizerPrompts creates a new instance of plan organizer prompts
func NewPlanOrganizerPrompts() *PlanOrganizerPrompts {
	mm := NewMemoryManagement()

	// Define agent-specific content
	agentType := "plan organizer"
	agentDescription := "that manages workspace and workflow coordination."
	specificContext := "The specific workflow context, planning output, execution output, and validation output will be provided in the user message."

	specificInstructions := `## 🎯 ORGANIZATION FOCUS: STRUCTURE WORKSPACE EFFICIENTLY

**ORGANIZATION PRIORITY**: Organize outputs and prepare for next iteration

## 📋 SIMPLIFIED ORGANIZATION:
1. **Read All Agent Outputs**: Check evidence/ folder for all agent results
2. **Update Workspace Memory**: Update plan.md, progress.md, and task index
3. **Organize Evidence**: Structure evidence files properly
4. **Clean Up Duplicates**: Remove redundant files and content
5. **Sync to GitHub**: Commit changes (if sync fails, continue anyway)

## Current Context
**WORKFLOW CONTEXT**: {{.WorkflowContext}}
**WORKSPACE**: {{.WorkspacePath}}
**PLANNING OUTPUT**: {{.PlanningOutput}}
**EXECUTION OUTPUT**: {{.ExecutionOutput}}
**VALIDATION OUTPUT**: {{.ValidationOutput}}

**ORGANIZATION STANDARDS**:
- Preserve all completed work
- Remove only true duplicates
- Maintain clear file structure
- Prepare for next iteration`

	outputFormat := `## 📊 **ORGANIZATION REPORT**

### Operations Completed
- **Files Organized**: [Number] files properly structured
- **Files Cleaned**: [Number] unnecessary files removed
- **Content Consolidated**: [Number] duplicate content merged
- **Workspace Sync**: [Success/Failed] - [Details]

### Workspace Status
- **Task Index**: [Updated/Current] - [Brief description]
- **Plan Status**: [Updated/Current] - [Brief description]
- **Progress Tracking**: [Updated/Current] - [Brief description]
- **Evidence Organization**: [Organized/Current] - [Brief description]

### Next Iteration Ready
- **Structure Optimized**: [Yes/No] - [Description of improvements]
- **Memory Cleaned**: [Yes/No] - [Description of cleanup]
- **Ready for Next Step**: [Yes/No] - [Brief assessment]

**RESPOND WITH ORGANIZATION REPORT** - Organize all outputs into structured memory, clean workspace, and provide organization report.`

	return &PlanOrganizerPrompts{
		OrganizeWorkflowPrompt: mm.GetBasePromptTemplate(agentType, agentDescription, specificContext, specificInstructions, outputFormat),
	}
}
