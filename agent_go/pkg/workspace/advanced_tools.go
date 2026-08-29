package workspace

import (
	"github.com/manishiitg/multi-llm-provider-go/llmtypes"
)

// shellToolDef returns the execute_shell_command tool definition (single source of truth).
func shellToolDef() llmtypes.Tool {
	return llmtypes.Tool{
		Type: "function",
		Function: &llmtypes.FunctionDefinition{
			Name: "execute_shell_command",
			// The working directory is per-session and is NOT always the docs root:
			// a workshop tool-agent runs in its workflow folder, a step in its run
			// execution folder. Claiming the docs root here made agents prefix their
			// own workflow path onto every relative path and fail with a bare
			// "No such file or directory". Failed commands now report their cwd.
			Description: "Execute a shell command and return stdout, stderr, and exit code. Runs via shell (`sh -c`). The working directory is session-specific — often your workflow or run folder, not the workspace docs root — so run `pwd` before assuming, and never prefix a relative path with a directory you may already be inside. Any command that fails reports the directory it ran in. Absolute paths under the docs root are accepted. Other host paths are rejected unless the current session explicitly grants one read-only. With Local Chrome (CDP), use a supplied `~/Downloads/...` or `/Users/<user>/Downloads/...` path directly: the workspace `Downloads/` folder is separate and must not be substituted for the host path. Copy a granted host file into a writable workspace folder before modifying it.",
			Parameters: llmtypes.NewParameters(map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					// NOTE: use_shell was removed from the tool definition to simplify the interface for the LLM.
					// It is now hardcoded to true internally in ExecuteShellCommand.
					"command": map[string]interface{}{
						"type":        "string",
						"description": "Shell command to execute as a single string, including any arguments and shell operators.",
					},
					"timeout": map[string]interface{}{
						"type":        "integer",
						"description": "Optional timeout in seconds.",
					},
				},
				"required": []string{"command"},
			}),
		},
	}
}

// generateTextLLMToolDef returns the generate_text_llm tool definition (single source of truth).
func generateTextLLMToolDef() llmtypes.Tool {
	return llmtypes.Tool{
		Type: "function",
		Function: &llmtypes.FunctionDefinition{
			Name:        "generate_text_llm",
			Description: "Generate text with the workspace tiered LLM configuration. Provide the user message and choose the 'high', 'medium', or 'low' tier to run it.",
			Parameters: llmtypes.NewParameters(map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"user_message": map[string]interface{}{
						"type":        "string",
						"description": "The prompt to send to the selected tier model.",
					},
					"tier": map[string]interface{}{
						"type":        "string",
						"description": "Reasoning tier to use for text generation.",
						"enum":        []string{"high", "medium", "low"},
					},
				},
				"required": []string{"user_message", "tier"},
			}),
		},
	}
}

// searchWebLLMToolDef returns the search_web_llm tool definition (single source of truth).
func searchWebLLMToolDef() llmtypes.Tool {
	return llmtypes.Tool{
		Type: "function",
		Function: &llmtypes.FunctionDefinition{
			Name:        "search_web_llm",
			Description: "Search the web through a hosted MCP provider. Provider is required: parallel, exa, or firecrawl. Do not pass model_id. Parallel and Exa use their anonymous free MCP tiers; Firecrawl keyless availability is service-controlled.",
			Parameters: llmtypes.NewParameters(map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"query": map[string]interface{}{
						"type":        "string",
						"description": "The web search query.",
					},
					"provider": map[string]interface{}{
						"type":        "string",
						"description": "Required hosted MCP provider: parallel, exa, or firecrawl. Firecrawl keyless availability is service-controlled.",
					},
				},
				"required": []string{"query", "provider"},
			}),
		},
	}
}

// diffPatchToolDef returns the diff_patch_workspace_file tool definition.
func diffPatchToolDef() llmtypes.Tool {
	return llmtypes.Tool{
		Type: "function",
		Function: &llmtypes.FunctionDefinition{
			Name:        "diff_patch_workspace_file",
			Description: "Apply a unified diff patch to a workspace file and return the result. The filepath may be workspace-relative or an absolute path under the workspace docs root.",
			Parameters: llmtypes.NewParameters(map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"filepath": map[string]interface{}{
						"type":        "string",
						"description": "Path to the file to patch. Accepts workspace-relative paths like \"Workflow/my-flow/learnings/_global/SKILL.md\" and absolute paths under the workspace docs root like \"/Users/.../workspace-docs/Workflow/my-flow/learnings/_global/SKILL.md\". Absolute paths outside the docs root are rejected.",
					},
					"diff": map[string]interface{}{
						"type":        "string",
						"description": "Unified diff patch string to apply.\n\nFormat:\n- Headers: --- a/file and +++ b/file\n- Hunk headers: @@ -startLine,lineCount +startLine,lineCount @@\n- Context lines: ' ' prefix (space + content)\n- Removals: '-' prefix\n- Additions: '+' prefix\n- End with a trailing newline\n\nExample:\n--- a/file\n+++ b/file\n@@ -5,1 +5,1 @@\n-- [ ] task-1\n+- [x] task-1\n\nContext and removal lines must be byte-exact copies of the current file content, not retyped from memory. If the target text contains em dashes (—), curly quotes (“ ”), or arrows (→), do not hand-copy it — retyping commonly substitutes plain ASCII equivalents (-, \", ->) without you noticing, which this tool will correctly reject as a context mismatch. Read the file first and copy the exact bytes, or generate the diff programmatically from the file you just read.",
					},
				},
				"required": []string{"filepath", "diff"},
			}),
		},
	}
}

// GetShellToolDefinitions returns only the shell (execute_shell_command) tool.
func GetShellToolDefinitions() []llmtypes.Tool {
	return []llmtypes.Tool{shellToolDef()}
}

// GetGenerateTextLLMToolDefinitions returns only the text generation tool.
func GetGenerateTextLLMToolDefinitions() []llmtypes.Tool {
	return []llmtypes.Tool{generateTextLLMToolDef()}
}

// GetSearchWebLLMToolDefinitions returns only the web search tool.
func GetSearchWebLLMToolDefinitions() []llmtypes.Tool {
	return []llmtypes.Tool{searchWebLLMToolDef()}
}

// GetDiffPatchToolDefinitions returns only the diff_patch_workspace_file tool definition.
func GetDiffPatchToolDefinitions() []llmtypes.Tool {
	return []llmtypes.Tool{diffPatchToolDef()}
}

// GetAdvancedToolDefinitions returns the active agent-facing tools. Provider
// media tools are retained in code for compatibility but are deprecated and
// deliberately hidden while testing focuses on text generation and web search.
func GetAdvancedToolDefinitions() []llmtypes.Tool {
	var tools []llmtypes.Tool
	tools = append(tools, GetShellToolDefinitions()...)
	tools = append(tools, GetGenerateTextLLMToolDefinitions()...)
	tools = append(tools, GetSearchWebLLMToolDefinitions()...)
	tools = append(tools, GetDiffPatchToolDefinitions()...)
	return tools
}
