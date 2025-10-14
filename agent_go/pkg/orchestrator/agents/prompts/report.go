package prompts

// ReportPrompts contains the predefined prompts for report generation operations
type ReportPrompts struct {
	GenerateReportPrompt string
}

// NewReportPrompts creates a new instance of report prompts
func NewReportPrompts() *ReportPrompts {
	mm := NewMemoryManagement()

	// Define agent-specific content
	agentType := "report generation"
	agentDescription := "that creates comprehensive reports based on workflow execution history to answer specific objectives."
	specificContext := "The specific objective, workflow context, and execution results will be provided in the user message."

	specificInstructions := `## 🎯 REPORT FOCUS: ANSWER THE OBJECTIVE CLEARLY

**REPORT PRIORITY**: Create a clear, comprehensive answer to the stated objective

## 📋 REPORT GENERATION:
1. **Analyze All Results**: Review planning, execution, validation, and organization outputs
2. **Extract Key Findings**: Identify findings that directly answer the objective
3. **Assess Completeness**: Determine what was accomplished vs. what remains
4. **Generate Report**: Create structured report with clear answers
5. **Save Report**: Store in {{.WorkspacePath}}/report.md

## Current Context
**OBJECTIVE**: {{.Objective}}
**WORKSPACE**: {{.WorkspacePath}}
**PLANNING RESULTS**: {{.PlanningResults}}
**EXECUTION RESULTS**: {{.ExecutionResults}}
**VALIDATION RESULTS**: {{.ValidationResults}}

**REPORT STANDARDS**:
- Direct answer to objective
- Evidence-backed claims
- Honest assessment of gaps
- Actionable recommendations`

	outputFormat := `## 📊 **REPORT OUTPUT**

### Executive Summary
- **Objective**: [Restate the original objective/question]
- **Answer**: [Direct answer based on evidence]
- **Confidence Level**: High/Medium/Low based on evidence quality
- **Key Findings**: [3-5 most important discoveries]

### Evidence-Based Findings
- **Direct Evidence**: [Specific findings that answer the objective]
- **Supporting Data**: [Metrics, outputs, and artifacts]
- **File References**: [Specific file paths and evidence locations]

### Gaps and Limitations
- **Unanswered Questions**: [What parts couldn't be answered?]
- **Missing Evidence**: [What additional data would be needed?]

### Recommendations
- **Immediate Actions**: [Next steps to address the objective]
- **Additional Work**: [Steps to complete the objective]

### File Operations
- **Report File**: {{.WorkspacePath}}/report.md - [Confirmation of successful creation/update]

### Quality Checklist
- [ ] **Objective-Focused**: Every section relates to answering the original objective
- [ ] **Evidence-Based**: All claims backed by specific workflow results
- [ ] **File Saved**: Report successfully saved to report.md`

	return &ReportPrompts{
		GenerateReportPrompt: mm.GetBasePromptTemplate(agentType, agentDescription, specificContext, specificInstructions, outputFormat),
	}
}
