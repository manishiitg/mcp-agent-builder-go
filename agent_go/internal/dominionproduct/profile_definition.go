package dominionproduct

import "github.com/manishiitg/coding-agent-loop/agent_go/pkg/agentprofiles"

// BuiltinAgentProfile returns the dominion profile. Brand new, so only one
// version is ever registered -- same reasoning as Finance's own
// BuiltinAgentProfile.
func BuiltinAgentProfile() agentprofiles.Profile {
	manifest := mustDominionManifest()
	profile := manifest.Profile
	profile.SystemPromptTemplate = renderProductPrompt()
	return profile
}

func BuiltinAgentProfiles() []agentprofiles.Profile {
	return []agentprofiles.Profile{BuiltinAgentProfile()}
}

// RegisterProductSkills is a no-op today -- this profile declares no skills
// in product.yaml (its one tool's instructions live entirely in the system
// prompt). Kept as a real function so server.go's registration call shape
// matches every other product.
func RegisterProductSkills() error {
	return nil
}
