package financeproduct

import "github.com/manishiitg/coding-agent-loop/agent_go/pkg/agentprofiles"

// BuiltinAgentProfile returns the finance profile. Brand new, so only one
// version is registered.
func BuiltinAgentProfile() agentprofiles.Profile {
	manifest := mustFinanceManifest()
	profile := manifest.Profile
	profile.SystemPromptTemplate = renderProductPrompt()
	return profile
}

func BuiltinAgentProfiles() []agentprofiles.Profile {
	return []agentprofiles.Profile{BuiltinAgentProfile()}
}

// RegisterProductSkills is a no-op today -- this profile declares no
// skills in product.yaml (its one tool's instructions live entirely in the
// system prompt). Kept as a real function, not omitted, so server.go's
// registration call shape matches every other product and adding a skill
// later needs no server.go change.
func RegisterProductSkills() error {
	return nil
}
