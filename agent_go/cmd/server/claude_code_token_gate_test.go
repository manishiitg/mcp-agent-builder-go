package server

import (
	"testing"

	"github.com/manishiitg/mcpagent/llm"
)

// TestClaudeCodeTokenMissingForSingleProductDeploymentGatesCorrectly pins the
// exact incident this gate closes: a dedicated single-product deployment
// (Dominion, Video Studio, Finance) resolving to claude-code with no
// configured token must refuse instead of silently falling back to whatever
// Claude Code CLI login happens to exist on that host.
func TestClaudeCodeTokenMissingForSingleProductDeploymentGatesCorrectly(t *testing.T) {
	token := "sk-ant-oat01-test-token"
	blank := "   "

	cases := []struct {
		name            string
		agentProducts   string
		resolvedProfile *resolvedAgentProfile
		finalProvider   string
		wantMissing     bool
	}{
		{
			name:            "single-product deployment, claude-code, no profile keys at all",
			agentProducts:   "dominion",
			resolvedProfile: &resolvedAgentProfile{APIKeys: nil},
			finalProvider:   "claude-code",
			wantMissing:     true,
		},
		{
			name:            "single-product deployment, claude-code, keys present but token nil",
			agentProducts:   "dominion",
			resolvedProfile: &resolvedAgentProfile{APIKeys: &llm.ProviderAPIKeys{}},
			finalProvider:   "claude-code",
			wantMissing:     true,
		},
		{
			name:            "single-product deployment, claude-code, token blank",
			agentProducts:   "dominion",
			resolvedProfile: &resolvedAgentProfile{APIKeys: &llm.ProviderAPIKeys{ClaudeCodeOAuthToken: &blank}},
			finalProvider:   "claude-code",
			wantMissing:     true,
		},
		{
			name:            "single-product deployment, claude-code, token present",
			agentProducts:   "dominion",
			resolvedProfile: &resolvedAgentProfile{APIKeys: &llm.ProviderAPIKeys{ClaudeCodeOAuthToken: &token}},
			finalProvider:   "claude-code",
			wantMissing:     false,
		},
		{
			name:            "single-product deployment, non-claude-code provider, no token",
			agentProducts:   "dominion",
			resolvedProfile: &resolvedAgentProfile{APIKeys: nil},
			finalProvider:   "openai",
			wantMissing:     false,
		},
		{
			name:            "no active product profile (profile-less chat)",
			agentProducts:   "dominion",
			resolvedProfile: nil,
			finalProvider:   "claude-code",
			wantMissing:     false,
		},
		{
			name:            "shared desktop/multi-product server (AGENT_PRODUCTS unset) must stay exempt",
			agentProducts:   "",
			resolvedProfile: &resolvedAgentProfile{APIKeys: nil},
			finalProvider:   "claude-code",
			wantMissing:     false,
		},
		{
			name:            "server hosting more than one product is not a dedicated single-product deployment",
			agentProducts:   "video-studio,dominion",
			resolvedProfile: &resolvedAgentProfile{APIKeys: nil},
			finalProvider:   "claude-code",
			wantMissing:     false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("AGENT_PRODUCTS", tc.agentProducts)
			if got := claudeCodeTokenMissingForSingleProductDeployment(tc.resolvedProfile, tc.finalProvider); got != tc.wantMissing {
				t.Fatalf("claudeCodeTokenMissingForSingleProductDeployment() = %v, want %v", got, tc.wantMissing)
			}
		})
	}
}
