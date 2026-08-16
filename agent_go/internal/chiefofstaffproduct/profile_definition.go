package chiefofstaffproduct

import (
	"sync"

	"github.com/manishiitg/coding-agent-loop/agent_go/cmd/server/guidance"
	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/agentprofiles"
)

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

// chiefOfStaffSkillNames are the reference topics this profile declares by
// name in product.yaml's profile.skills[] -- the same 14 non-goal-related
// entries the old mode-gated AttachReferenceSurface bundle already covered
// (org-goals/org-html/org-pulse/chief-task-report excluded: goal-alignment
// content being removed alongside the dropped goals feature). Individually
// named, like Video Studio's own skills, rather than one opaque bundle.
var chiefOfStaffSkillNames = []string{
	"stores", "file-layout", "llm-provider-config", "skill-management",
	"delegation", "schedule-management", "secret-management", "html-output",
	"browser-usage", "mcp-bridge", "workspace-media-tools", "debugging-flow",
	"publish-strategy", "backup-strategy",
}

var registerProductSkillsOnce sync.Once
var registerProductSkillsErr error

// RegisterProductSkills renders chiefOfStaffSkillNames from the shared
// referenceKinds registry (guidance.MaterializeReferenceKindsAsSkills) and
// registers each as its own builtin skill -- the same content the mode-gated
// "builder-reference" bundle already showed under these names, now
// individually declared the way a product-owned skill should be, matching
// product.yaml's profile.skills: [...] list.
func RegisterProductSkills() error {
	registerProductSkillsOnce.Do(func() {
		rendered, err := guidance.MaterializeReferenceKindsAsSkills("multi-agent", chiefOfStaffSkillNames)
		if err != nil {
			registerProductSkillsErr = err
			return
		}
		registerProductSkillsErr = agentprofiles.RegisterSkills(rendered)
	})
	return registerProductSkillsErr
}
