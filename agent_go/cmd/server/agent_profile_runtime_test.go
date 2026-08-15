package server

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"strings"
	"testing"

	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/agentprofiles"
	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/chathistory"
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

func TestResolveAgentProfileInjectsProjectScopedSecretsIntoNativeEnvironment(t *testing.T) {
	t.Setenv("AUTH_SECRET", "test-auth-secret-with-enough-entropy")
	store, err := chathistory.NewFilesystemStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	const userID = "user-1"
	const workspacePath = "Chats/Video Studio/projects/launch"
	if err := store.UpsertWorkflowSecret(context.Background(), userID, workspacePath, "PEXELS_API_KEY", encryptProfileSecretForTest(t, userID, "test-key")); err != nil {
		t.Fatalf("store project secret: %v", err)
	}

	registry := agentprofiles.NewRegistry()
	if err := registry.RegisterProfile(agentprofiles.Profile{
		ID: "video-studio", Name: "Video Studio", Version: 1, BuiltIn: true,
		SystemPromptTemplate: "{{.ProjectTitle}}",
		Runtime:              agentprofiles.RuntimePolicy{Transport: "structured", Capabilities: agentprofiles.RuntimeCapabilities{Secrets: agentprofiles.CapabilityRequired}},
	}); err != nil {
		t.Fatal(err)
	}
	api := &StreamingAPI{agentProfiles: registry, chatStore: store}
	req := QueryRequest{AgentMode: "multi-agent", AgentProfileID: "video-studio", SelectedFolder: workspacePath, AgentProfileContext: agentprofiles.PromptContext{ProjectTitle: "Launch"}}
	if _, err := api.resolveAgentProfileForQuery(context.Background(), &req, userID, "session-1"); err != nil {
		t.Fatal(err)
	}
	if len(req.DecryptedSecrets) != 1 || req.DecryptedSecrets[0].Name != "PEXELS_API_KEY" || req.DecryptedSecrets[0].Value != "test-key" {
		t.Fatalf("profile secret selection = %#v, want only the project-scoped secret", req.DecryptedSecrets)
	}
}

func encryptProfileSecretForTest(t *testing.T, userID, value string) string {
	t.Helper()
	block, err := aes.NewCipher(deriveSecretsKey())
	if err != nil {
		t.Fatal(err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		t.Fatal(err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		t.Fatal(err)
	}
	sealed := gcm.Seal(nil, nonce, []byte(value), []byte(userID))
	return base64.StdEncoding.EncodeToString(append(nonce, sealed...))
}

// A per-project Cursor API key must reach the agent the same way a per-project
// Claude Code token does. This is the regression for a live bug: the
// credential branch here originally checked strings.EqualFold(provider,
// "claude-code") explicitly, so a project scoped to cursor-cli saved a key
// through the UI, the save succeeded, and every following turn still failed
// "Authentication required" because this function silently never loaded it.
func TestResolveAgentProfileInjectsProjectScopedCursorAPIKey(t *testing.T) {
	store, err := chathistory.NewFilesystemStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	const userID = "user-1"
	const workspacePath = "Chats/Video Studio/projects/launch"
	const cursorKey = "crsr_project_scoped_key"
	if err := store.UpsertWorkflowProviderCredential(context.Background(), userID, workspacePath, cursorCLIProviderID, encryptProviderCredentialForTest(t, cursorKey, userID)); err != nil {
		t.Fatalf("store project Cursor credential: %v", err)
	}

	registry := agentprofiles.NewRegistry()
	if err := registry.RegisterProfile(agentprofiles.Profile{
		ID: "video-studio", Name: "Video Studio", Version: 1, BuiltIn: true,
		SystemPromptTemplate: "{{.ProjectTitle}}",
		Runtime: agentprofiles.RuntimePolicy{
			Transport: "auto", Provider: "cursor-cli", ModelID: "auto",
		},
	}); err != nil {
		t.Fatal(err)
	}
	api := &StreamingAPI{agentProfiles: registry, chatStore: store}
	req := QueryRequest{AgentMode: "multi-agent", AgentProfileID: "video-studio", SelectedFolder: workspacePath, AgentProfileContext: agentprofiles.PromptContext{ProjectTitle: "Launch"}}
	if _, err := api.resolveAgentProfileForQuery(context.Background(), &req, userID, "session-1"); err != nil {
		t.Fatal(err)
	}
	if req.LLMConfig == nil || req.LLMConfig.APIKeys == nil || req.LLMConfig.APIKeys.CursorCLI == nil || *req.LLMConfig.APIKeys.CursorCLI != cursorKey {
		t.Fatalf("project-scoped Cursor key was not injected: %+v", req.LLMConfig)
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

// A profile turn's selected_folder becomes the agent's read/write root, and
// agentProfileRuntimeWorkspace only re-scopes paths beginning with "Chats" --
// anything else is passed through verbatim. Blocking "../" traversal alone
// therefore left an explicit "_users/<victim>/..." fully usable, which is a
// cross-user workspace read/write, so this pins the authorization gate itself.
func TestCleanAgentProfileWorkspaceRejectsCrossUserPaths(t *testing.T) {
	const caller = "alice"

	for _, tc := range []struct {
		name string
		path string
	}{
		{"another user's chats", "_users/bob/Chats/Video Studio/projects/x"},
		{"another user's root", "_users/bob"},
		{"the users root itself", "_users"},
		{"traversal out of own space", "_users/alice/../bob/Chats"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got, err := cleanAgentProfileWorkspace(tc.path, caller); err == nil {
				t.Fatalf("cleanAgentProfileWorkspace(%q, %q) = %q, want an error", tc.path, caller, got)
			}
		})
	}

	// The legitimate shapes must keep working: the user-relative form the
	// frontend actually sends, and the caller's own canonical _users path.
	for _, tc := range []struct {
		name string
		path string
		want string
	}{
		{"user-relative chats path", "Chats/Video Studio/projects/x", "Chats/Video Studio/projects/x"},
		{"caller's own canonical path", "_users/alice/Chats/x", "_users/alice/Chats/x"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := cleanAgentProfileWorkspace(tc.path, caller)
			if err != nil {
				t.Fatalf("cleanAgentProfileWorkspace(%q, %q) error = %v, want success", tc.path, caller, err)
			}
			if got != tc.want {
				t.Fatalf("cleanAgentProfileWorkspace(%q, %q) = %q, want %q", tc.path, caller, got, tc.want)
			}
		})
	}
}
