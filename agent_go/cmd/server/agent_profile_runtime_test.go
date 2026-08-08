package server

import (
	"context"
	"strings"
	"testing"

	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/agentprofiles"
	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/orchestrator"
)

func TestResolveAgentProfileForQueryPinsVersionAndSkills(t *testing.T) {
	registry := agentprofiles.NewRegistry()
	profile := agentprofiles.Profile{
		ID: "video-studio", Name: "Video Studio", Version: 2, BuiltIn: true,
		SystemPromptTemplate: "Project {{.ProjectTitle}} at {{.LocalDateTime}}",
		Skills:               []string{"video-creation"},
		Runtime: agentprofiles.RuntimePolicy{
			Transport: "auto", Provider: "claude-code", ModelID: "claude-sonnet-5",
			Capabilities: agentprofiles.RuntimeCapabilities{Browser: agentprofiles.CapabilityRequired},
		},
	}
	if err := registry.RegisterProfile(profile); err != nil {
		t.Fatal(err)
	}
	api := &StreamingAPI{agentProfiles: registry}
	req := QueryRequest{
		AgentMode: "multi-agent", AgentProfileID: "video-studio",
		AgentProfileContext: agentprofiles.PromptContext{ProjectTitle: "Launch"},
		SelectedFolder:      "Chats/Video Studio/projects/launch",
		SelectedSkills:      []string{"existing"},
		Provider:            "codex-cli",
		ModelID:             "gpt-5.4",
		LLMConfig: &orchestrator.LLMConfig{Primary: orchestrator.LLMModel{
			Provider: "codex-cli", ModelID: "gpt-5.4",
		}},
	}
	resolved, err := api.resolveAgentProfileForQuery(context.Background(), &req, "user-1", "session-1")
	if err != nil {
		t.Fatal(err)
	}
	if resolved == nil || !strings.Contains(resolved.Prompt, "Project Launch") {
		t.Fatalf("unexpected resolved profile: %+v", resolved)
	}
	if req.AgentProfileVersion != 2 || strings.Join(req.SelectedSkills, ",") != "existing,video-creation" {
		t.Fatalf("request not pinned: version=%d skills=%v", req.AgentProfileVersion, req.SelectedSkills)
	}
	if req.EnableBrowserAccess == nil || !*req.EnableBrowserAccess || req.BrowserMode != "auto" {
		t.Fatalf("profile browser capability was not applied: enabled=%v mode=%q", req.EnableBrowserAccess, req.BrowserMode)
	}
	if req.SelectedFolder != "Chats/Video Studio/projects/launch" {
		t.Fatalf("selected folder = %q", req.SelectedFolder)
	}
	if req.Provider != "claude-code" || req.ModelID != "claude-sonnet-5" || req.LLMConfig == nil || req.LLMConfig.Primary.Provider != "claude-code" || req.LLMConfig.Primary.ModelID != "claude-sonnet-5" {
		t.Fatalf("profile model was not pinned over the global selection: provider=%q model=%q llm=%+v", req.Provider, req.ModelID, req.LLMConfig)
	}
	if req.LLMConfigSource != llmConfigSourceAgentProfile || !requestLLMConfigOverridesManifest(req) {
		t.Fatalf("profile LLM source is not authoritative: source=%q", req.LLMConfigSource)
	}
	if got := agentProfileRuntimeWorkspace("user-1", req.SelectedFolder); got != "_users/user-1/Chats/Video Studio/projects/launch" {
		t.Fatalf("runtime folder = %q", got)
	}
}

func TestResolveAgentProfileRejectsWorkspaceEscape(t *testing.T) {
	registry := agentprofiles.NewRegistry()
	_ = registry.RegisterProfile(agentprofiles.Profile{
		ID: "video-studio", Name: "Video Studio", Version: 1, BuiltIn: true,
		SystemPromptTemplate: "{{.ProjectTitle}}", Runtime: agentprofiles.RuntimePolicy{Transport: "auto"},
	})
	api := &StreamingAPI{agentProfiles: registry}
	req := QueryRequest{AgentMode: "multi-agent", AgentProfileID: "video-studio", SelectedFolder: "../outside", AgentProfileContext: agentprofiles.PromptContext{ProjectTitle: "Bad"}}
	if _, err := api.resolveAgentProfileForQuery(context.Background(), &req, "user-1", "session-1"); err == nil {
		t.Fatal("workspace escape was accepted")
	}
}

func TestResolveProfileRuntimeModelUsesOnlyYAMLProviderOptions(t *testing.T) {
	runtime := agentprofiles.RuntimePolicy{
		Provider: "claude-code", ModelID: "claude-sonnet-5",
		ProviderOptions: []agentprofiles.ProviderOption{
			{ID: "claude-code", Label: "Claude Code", Provider: "claude-code", ModelID: "claude-sonnet-5", Default: true},
			{ID: "codex", Label: "Codex", Provider: "codex-cli", ModelID: "gpt-5.6-terra"},
			{ID: "cursor", Label: "Cursor", Provider: "cursor-cli", ModelID: "auto"},
		},
	}
	if provider, model := resolveProfileRuntimeModel(runtime, "codex-cli", "gpt-5.6-terra"); provider != "codex-cli" || model != "gpt-5.6-terra" {
		t.Fatalf("approved YAML option was not selected: provider=%q model=%q", provider, model)
	}
	if provider, model := resolveProfileRuntimeModel(runtime, "codex-cli", "gpt-5.6-sol"); provider != "claude-code" || model != "claude-sonnet-5" {
		t.Fatalf("unapproved provider/model escaped profile allow-list: provider=%q model=%q", provider, model)
	}
}
