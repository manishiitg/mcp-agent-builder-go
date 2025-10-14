package memory

import "mcp-agent/agent_go/pkg/utils"

// GetWorkflowMemoryRequirements returns memory management requirements for workflow agents
func GetWorkflowMemoryRequirements() string {
	return `
## 📁 WORKSPACE MEMORY MANAGEMENT

### **Workspace Directory Structure**
- **Workflow Root**: Workflow/[FolderName]/ (specified in objective input)
- **Evidence Storage**: Workflow/[FolderName]/evidence/ (execution outputs, results, artifacts)
- **Progress Tracking**: Workflow/[FolderName]/progress/ (completion status, step tracking)
- **Context Storage**: Workflow/[FolderName]/context/ (requirements, constraints, specifications)
- **Reports**: Workflow/[FolderName]/reports/ (final deliverables, summaries)
- **Archive**: Workflow/[FolderName]/archive/ (completed work, historical data)

### **Memory Integration Requirements**
- **Read Operations**: Check existing files in Workflow/[FolderName]/ for context and previous work
- **Write Operations**: Store results in appropriate subdirectories (evidence/, progress/, etc.)
- **Update Operations**: Modify existing files to track progress and maintain state
- **Archive Operations**: Move completed work to archive/ to maintain clean workspace

### **File Organization Guidelines**
- **Evidence Files**: Store execution outputs, tool results, and artifacts in evidence/
- **Progress Files**: Track completion status and step progress in progress/
- **Context Files**: Maintain requirements and specifications in context/
- **Report Files**: Generate final deliverables in reports/
- **Naming Convention**: Use descriptive names with timestamps when appropriate

### **Memory Boundaries**
- **STRICT BOUNDARY**: ONLY work within the specified Workflow/[FolderName] folder - do not access other folders
- **Write Bounds**: Only read/write within Workflow/[FolderName]/; never touch .env, root configs, secrets
- **Subdirectory Access**: Can access all subdirectories within the specified Workflow/[FolderName] folder
- **Cross-Folder Access**: Do not access other Workflow/[OtherFolder] directories unless explicitly specified

### **🔍 File Discovery & Search**
**Use these tools to efficiently find files:**

- **list_workspace_files**: List files and directories to check existence and structure
  - Use for: checking if files/folders exist, exploring directory structure, finding file names
  - Example: list_workspace_files(folder="Workflow/MyWorkflow") to see what's in the workflow folder
  
- **regex_search_workspace_files**: Regex/text-based search for exact matches
  - Use for: finding specific text, file names, patterns, exact keywords, complex regex patterns
  - Example: regex_search_workspace_files(query="TODO", folder="Workflow/MyWorkflow")
  
- **semantic_search_workspace_files**: AI-powered semantic search for meaning-based discovery
  - Use for: finding conceptually related content, understanding context
  - Example: semantic_search_workspace_files(query="error handling patterns", folder="Workflow/MyWorkflow")

**Search Strategy:**
1. **Start with listing** to see what files and folders exist in the directory
2. **Use semantic search** to understand what files exist and their content
3. **Use regex search** for specific text patterns, file names, or exact matches
4. **Use regex patterns** for complex pattern matching and multi-line searches
5. **Combine all tools** for comprehensive file discovery
6. **Filter by folder** to search within specific workflow directories

### **Memory Operations Protocol**
1. **Search First**: Use search tools to locate relevant files before reading
2. **Check Existing**: Always check for existing files before creating new ones
3. **Preserve History**: Maintain chronological order and preserve previous work
4. **Update Progress**: Regularly update progress tracking files
5. **Archive Completed**: Move finished work to archive/ to keep workspace clean
6. **Document Changes**: Include timestamps and change descriptions in file updates

### **🚨 CRITICAL FILE OPERATION GUIDELINES**
- **PREFER diff_patch_workspace_file**: Use for targeted, surgical changes to existing files
- **AVOID update_workspace_file**: Only use for creating completely new files or full replacements
- **EFFICIENCY**: diff_patch_workspace_file is more efficient and preserves file history
- **WORKFLOW**: 1) Read file with read_workspace_file 2) Generate unified diff 3) Apply with diff_patch_workspace_file
- **PRECISION**: Diff patching allows precise modifications without overwriting entire files

` + utils.GetCommonFileInstructions() + `

`
}
