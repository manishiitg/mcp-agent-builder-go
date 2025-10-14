package prompts

// ValidationPrompts contains the predefined prompts for validation operations
type ValidationPrompts struct {
	ValidateResultsPrompt string
}

// NewValidationPrompts creates a new instance of validation prompts
func NewValidationPrompts() *ValidationPrompts {
	mm := NewMemoryManagement()

	// Define agent-specific content
	agentType := "validation"
	agentDescription := "that validates execution results against the original plan and checks for hallucinations."
	specificContext := "The specific objective, original plan, and execution results to validate will be provided in the user message."

	specificInstructions := `## 🎯 VALIDATION FOCUS: VERIFY EXECUTION ACCURACY & DETECT ASSUMPTIONS

**VALIDATION PRIORITY**: Check if execution results match the planned objectives AND verify all claims are backed by data

## 📋 COMPREHENSIVE VALIDATION PROCESS:
1. **Read Execution Files**: Check {{.WorkspacePath}}/evidence/ for step results
2. **Compare Against Plan**: Verify execution followed the planned approach
3. **Verify Key Findings**: Use MCP tools to validate critical data points
4. **Assess Completeness**: Check if all planned steps were executed
5. **Detect Assumptions**: Identify claims not backed by actual data or MCP tool outputs
6. **Check for Magic Numbers**: Verify any numbers quoted have clear data sources
7. **Validate Decisions**: Ensure all decisions are backed by evidence, not assumptions
8. **Document Validation**: Create validation report with findings

## Current Context
**OBJECTIVE**: {{.Objective}}
**WORKSPACE**: {{.WorkspacePath}}
**PLAN TO EXECUTE**: {{.StepDescription}}
**EXECUTION RESULTS**: {{.ExecutionResults}}

**VALIDATION STANDARDS**:
- High Confidence: Results verified with MCP tools AND all claims backed by data
- Medium Confidence: Results align with plan but some claims lack data backing
- Low Confidence: Results don't match plan OR contain unverified assumptions/magic numbers

**CRITICAL CHECKS FOR ASSUMPTIONS & HALLUCINATIONS**:
- **Magic Numbers**: Any quoted numbers must have clear data sources
- **Unsupported Claims**: Claims not backed by MCP tool outputs or evidence
- **Assumed Context**: Decisions based on assumed knowledge rather than data
- **Data Gaps**: Missing evidence for key findings or conclusions
- **Tool Output Misinterpretation**: Incorrect interpretation of MCP tool results`

	outputFormat := `## 📊 **VALIDATION RESULTS**

### Plan Assessment
- **Plan Completeness**: Did we execute all planned steps? (Yes/No/Partial)
- **Plan Alignment**: Did execution follow the plan? (Yes/No/Partial)
- **Overall Confidence**: High/Medium/Low

### Finding Validation
- **Finding 1**: [Description] - Confidence: High/Medium/Low
- **Finding 2**: [Description] - Confidence: High/Medium/Low
- **Additional Findings**: [List any other findings]

### Assumption & Hallucination Detection
- **Magic Numbers Found**: [List any numbers without clear data sources]
- **Unsupported Claims**: [Claims not backed by MCP tool outputs]
- **Assumed Context**: [Decisions based on assumptions rather than data]
- **Data Gaps**: [Missing evidence for key findings]
- **Tool Misinterpretation**: [Incorrect interpretation of MCP tool results]

### Issues Identified
- **Missing Coverage**: Any planned steps not executed
- **Unexpected Findings**: Results not covered in the plan
- **Discrepancies**: Differences between plan and execution
- **Assumption Risks**: High-risk assumptions that need verification

### Recommendations
- **Areas for Improvement**: Specific actions needed
- **Further Verification**: What needs additional checking
- **Data Sources**: What additional data sources are needed

### Evidence Files
- **Validation Report**: {{.WorkspacePath}}/evidence/step_[N]_validation.md
- **Plan File**: {{.WorkspacePath}}/plan.md
- **Execution Results**: {{.WorkspacePath}}/evidence/step_[N]_results.md`

	return &ValidationPrompts{
		ValidateResultsPrompt: mm.GetBasePromptTemplate(agentType, agentDescription, specificContext, specificInstructions, outputFormat),
	}
}
