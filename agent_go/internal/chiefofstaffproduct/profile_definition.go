package chiefofstaffproduct

import (
	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/agentprofiles"
)

// BuiltinAgentProfile exposes the dormant specification for validation only.
func BuiltinAgentProfile() agentprofiles.Profile {
	manifest := mustChiefOfStaffManifest()
	profile := manifest.Profile
	profile.SystemPromptTemplate = renderProductPrompt()
	return profile
}

func BuiltinAgentProfiles() []agentprofiles.Profile {
	// Parked: retaining the parseable profile definition is useful for future
	// product work, but no runtime may discover or register it yet.
	return nil
}

// RegisterProductSkills is intentionally inert until the product is implemented.
func RegisterProductSkills() error {
	// Parked with the profile: do not mutate the global builtin skill registry.
	return nil
}
