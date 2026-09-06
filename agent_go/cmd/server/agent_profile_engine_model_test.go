package server

import (
	"strings"
	"testing"

	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/agentprofiles"
)

func engineModelTestProfile() agentprofiles.Profile {
	return agentprofiles.Profile{
		ID: "p", Name: "P", Version: 1,
		Runtime: agentprofiles.RuntimePolicy{ProviderOptions: []agentprofiles.ProviderOption{
			{ID: "claude-code", Label: "Claude Code", Provider: "claude-code", ModelID: "claude-fable-5-1", Default: true},
			{ID: "codex-cli", Label: "Codex", Provider: "codex-cli", ModelID: "gpt-6-astra"},
		}},
	}
}

// A model choice must be one the platform's catalog lists for the engine's
// provider — the same list the composer's switcher was filled from.
func TestProfileQueryModelMustBelongToEngineProvider(t *testing.T) {
	profile := engineModelTestProfile()
	conversation := ProductConversationRecord{SessionID: "s", WorkspacePath: "Chats/P", ConversationKey: "main"}

	req, err := queryRequestForAgentProfileChat(profile, AgentProfileChatRequest{Message: "hi", Engine: "claude-code", ModelID: "claude-opus-5"}, conversation)
	if err != nil {
		t.Fatalf("catalog model rejected: %v", err)
	}
	if req.Provider != "claude-code" || req.ModelID != "claude-opus-5" {
		t.Fatalf("provider/model = %q/%q", req.Provider, req.ModelID)
	}

	req, err = queryRequestForAgentProfileChat(profile, AgentProfileChatRequest{Message: "hi", Engine: "claude-code"}, conversation)
	if err != nil || req.ModelID != "claude-fable-5-1" {
		t.Fatalf("no model_id should keep the option's own model: %q, %v", req.ModelID, err)
	}

	_, err = queryRequestForAgentProfileChat(profile, AgentProfileChatRequest{Message: "hi", Engine: "claude-code", ModelID: "gpt-6-astra"}, conversation)
	if err == nil || !strings.Contains(err.Error(), "not offered") {
		t.Fatalf("a Codex model under Claude Code should be refused, got %v", err)
	}
}

// The provider is bound by a conversation's first turn and reported back
// unchanged afterwards; a rotated (new) conversation starts unbound.
func TestConversationProviderBindsOnFirstTurn(t *testing.T) {
	store, _ := memoryProductConversationStore()
	profile := engineModelTestProfile()
	binding := productConversationBinding{ConversationKey: "main", WorkspacePath: "Chats/P", Title: "P"}
	ctx := t.Context()
	if _, err := store.resolveOrCreate(ctx, "u", profile, binding, ""); err != nil {
		t.Fatalf("resolveOrCreate: %v", err)
	}
	bound, err := store.bindProvider(ctx, "u", profile, "main", "codex-cli")
	if err != nil || bound != "codex-cli" {
		t.Fatalf("first bind = %q, %v", bound, err)
	}
	bound, err = store.bindProvider(ctx, "u", profile, "main", "claude-code")
	if err != nil || bound != "codex-cli" {
		t.Fatalf("second bind should report the bound provider, got %q, %v", bound, err)
	}
	if _, err := store.rotate(ctx, "u", profile, binding); err != nil {
		t.Fatalf("rotate: %v", err)
	}
	bound, err = store.bindProvider(ctx, "u", profile, "main", "claude-code")
	if err != nil || bound != "claude-code" {
		t.Fatalf("a new conversation should bind afresh, got %q, %v", bound, err)
	}
}
