package agentprofiles

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func validProfile() Profile {
	return Profile{
		ID:                   "video-studio",
		Name:                 "Video Studio",
		Version:              1,
		SystemPromptTemplate: "You are working on {{.ProjectTitle}} at {{.LocalDateTime}}.",
		Skills:               []string{"video-creation", "video-quality"},
		Tools:                []ToolBinding{{ID: "video.show-video", Config: json.RawMessage(`{"strict":true}`)}},
		Runtime: RuntimePolicy{
			Transport: "auto",
			Capabilities: RuntimeCapabilities{
				LiveInput:   CapabilityRequired,
				RawTerminal: CapabilityDisabled,
				WarmSession: CapabilityPreferred,
			},
		},
		BuiltIn: true,
	}
}

func TestValidateRejectsStructuredLiveInput(t *testing.T) {
	profile := validProfile()
	profile.Runtime.Transport = "structured"
	if err := Validate(profile); err == nil || !strings.Contains(err.Error(), "live_input") {
		t.Fatalf("expected structured/live-input contradiction, got %v", err)
	}
}

func TestValidateRequiresCompleteRuntimeModelBinding(t *testing.T) {
	profile := validProfile()
	profile.Runtime.Provider = "claude-code"
	if err := Validate(profile); err == nil || !strings.Contains(err.Error(), "model_id") {
		t.Fatalf("expected incomplete runtime model binding to fail, got %v", err)
	}
	profile.Runtime.ModelID = "claude-sonnet-5"
	if err := Validate(profile); err != nil {
		t.Fatalf("complete runtime model binding failed: %v", err)
	}
}

func TestValidateHybridRejectsExecuteShellCommand(t *testing.T) {
	profile := validProfile()
	profile.ToolPolicy = ToolPolicy{Mode: ToolPolicyModeAllowlist, Enabled: []string{"execute_shell_command"}}
	profile.Runtime.AgentTools.Mode = "hybrid"
	if err := Validate(profile); err == nil || !strings.Contains(err.Error(), "cannot enable") {
		t.Fatalf("expected hybrid/execute_shell_command contradiction, got %v", err)
	}

	profile.ToolPolicy.Enabled = []string{"show_video"}
	profile.Runtime.APITransport.Mode = "native_shell"
	if err := Validate(profile); err != nil {
		t.Fatalf("hybrid native-shell profile should be valid: %v", err)
	}
}

func TestRenderPromptUsesAllowListedContext(t *testing.T) {
	profile := validProfile()
	rendered, err := RenderPrompt(profile, PromptContext{ProjectTitle: "Launch Film", LocalDateTime: "Friday"})
	if err != nil {
		t.Fatal(err)
	}
	if rendered != "You are working on Launch Film at Friday." {
		t.Fatalf("unexpected rendered prompt: %q", rendered)
	}
}

func TestRegistryVersionsAreImmutableAndOwnerScoped(t *testing.T) {
	registry := NewRegistry()
	builtin := validProfile()
	if err := registry.RegisterProfile(builtin); err != nil {
		t.Fatal(err)
	}
	if err := registry.RegisterProfile(builtin); err == nil {
		t.Fatal("duplicate immutable version was accepted")
	}

	owned := validProfile()
	owned.ID = "my-agent"
	owned.BuiltIn = false
	owned.OwnerID = "user-1"
	if err := registry.RegisterProfile(owned); err != nil {
		t.Fatal(err)
	}
	if _, err := registry.Resolve("my-agent", 1, "user-2"); err == nil {
		t.Fatal("another user's profile was exposed")
	}
	if got, err := registry.Resolve("my-agent", 0, "user-1"); err != nil || got.Version != 1 {
		t.Fatalf("owner could not resolve latest profile: profile=%+v err=%v", got, err)
	}
}

func TestToolFactoriesAreRegisteredCodeNotProfilePayloads(t *testing.T) {
	registry := NewRegistry()
	if err := registry.RegisterToolFactory("video.show-video", func(runtime ToolRuntimeContext, config json.RawMessage) (ToolSpec, error) {
		return ToolSpec{
			Name:        "show_video",
			Description: runtime.WorkspacePath,
			Execute: func(context.Context, map[string]interface{}) (string, error) {
				return string(config), nil
			},
		}, nil
	}); err != nil {
		t.Fatal(err)
	}
	tool, err := registry.BuildTool(ToolBinding{ID: "video.show-video", Config: json.RawMessage(`{"strict":true}`)}, ToolRuntimeContext{WorkspacePath: "Workflow/video"})
	if err != nil {
		t.Fatal(err)
	}
	if tool.Name != "show_video" || tool.Description != "Workflow/video" {
		t.Fatalf("unexpected tool: %+v", tool)
	}
	result, err := tool.Execute(context.Background(), nil)
	if err != nil || result != `{"strict":true}` {
		t.Fatalf("unexpected tool result %q: %v", result, err)
	}
}

func TestProfileInitializerReceivesTrustedRuntime(t *testing.T) {
	registry := NewRegistry()
	var received RuntimeContext
	if err := registry.RegisterInitializer("video-studio", func(_ context.Context, runtime RuntimeContext) error {
		received = runtime
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	want := RuntimeContext{UserID: "user-1", SessionID: "session-1", WorkspacePath: "Chats/Video Studio/projects/demo"}
	if err := registry.Initialize(context.Background(), "video-studio", want); err != nil {
		t.Fatal(err)
	}
	if received != want {
		t.Fatalf("initializer runtime = %+v, want %+v", received, want)
	}
	if err := registry.Initialize(context.Background(), "profile-without-initializer", want); err != nil {
		t.Fatalf("missing initializer should be a no-op: %v", err)
	}
}
