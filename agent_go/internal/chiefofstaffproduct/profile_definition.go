package chiefofstaffproduct

import "github.com/manishiitg/coding-agent-loop/agent_go/pkg/agentprofiles"

// BuiltinAgentProfile returns the chief-of-staff profile. Unlike Video
// Studio's BuiltinAgentProfiles, there is no prior shipped version to keep
// resolvable alongside the current one -- this is a brand new profile, so
// only one version is ever registered.
func BuiltinAgentProfile() agentprofiles.Profile {
	manifest := mustChiefOfStaffManifest()
	profile := manifest.Profile
	profile.SystemPromptTemplate = renderProductPrompt()
	return profile
}

func BuiltinAgentProfiles() []agentprofiles.Profile {
	return []agentprofiles.Profile{BuiltinAgentProfile()}
}
