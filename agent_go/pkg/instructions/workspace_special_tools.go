package instructions

// GetSpecialWorkspaceToolsInstructions returns the cheat-sheet section
// for the active workspace LLM tools (text generation and web search) and capability
// discovery). The full reference — signatures, parameters, defaults,
// provider-setup discipline — lives in the workspace-media-tools skill
// loaded on demand via read_skill(skills=[{"name":"builder-reference","path":"references/workspace-media-tools.md"}]).
//
// Used by both chat agents and workflow-builder agents.
func GetSpecialWorkspaceToolsInstructions() string {
	return `## Special Workspace Tools (cheat sheet)

Provider-backed capabilities you can call directly instead of general chat reasoning. **Path contract**: every file-path argument must be a full absolute path under the workspace docs root. **Provider/model contract**: pass ` + "`provider`" + ` and ` + "`model_id`" + ` together from the same ` + "`list_llm_capabilities(capability=\"...\", include_models=true)`" + ` result — do not pass only ` + "`model_id`" + ` and ask the backend to infer.

Available tools:
- **Discovery + cost**: ` + "`list_llm_capabilities`" + `, ` + "`estimate_llm_cost`" + `, ` + "`set_provider_auth`" + ` (always use this for API keys — never paste into shell, scripts, or config files).
- **Text + search**: ` + "`generate_text_llm(user_message, tier)`" + ` · ` + "`search_web_llm(query, provider)`" + `.

Provider-setup essentials (do not hand-edit provider-auth storage — it's encrypted and managed via ` + "`set_provider_auth`" + `; audio/video/image/music providers are workspace **tool** capabilities, not published-LLM entries — call ` + "`list_llm_capabilities(capability=\"...\")`" + ` for the authoritative availability answer).

Provider media tools are deprecated and hidden from agents while these two text/search tools are the active testing focus.`
}

// GetSpecialWorkspaceToolsPointer returns the compact coding-CLI form of the
// workspace media/search guidance. Coding CLIs receive the complete
// workspace-media-tools reference through the projected builder-reference
// skill, so repeating the full catalog in CLAUDE.md/AGENTS.md wastes the
// provider's fixed instruction budget.
func GetSpecialWorkspaceToolsPointer() string {
	return `## Special Workspace Tools

The active provider-backed text and web-search tools are available through the MCP bridge. Provider media tools are deprecated and hidden from agents while text/search testing is the focus.`
}
