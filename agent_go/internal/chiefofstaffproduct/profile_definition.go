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
// name in product.yaml's profile.skills[] -- individually named, like Video
// Studio's own skills, rather than one opaque mode-gated bundle. Two things
// were deliberately excluded from the old bundle's full set, not just
// carried over wholesale:
//   - org-goals/org-html/org-pulse/chief-task-report: goal-alignment content,
//     removed alongside the dropped goals feature (chief-task-report is
//     unrelated to goals and stays -- see the scheduler's task-report flow --
//     but doesn't need declaring here since it's read via a direct read_skill
//     call in the scheduler's own generated prompt, not agent-browsed).
//   - llm-provider-config: workspace-wide LLM/credential catalog management
//     (list_published_llms, test_llm, save_published_llm, set_provider_auth)
//     -- not tier-routing (that never applied here; see llm-selection, which
//     is workshop-mode only), just scope: managing the LLM/credential catalog
//     is a builder/workshop task, not a read-only ops-chat one.
var chiefOfStaffSkillNames = []string{
	"stores", "file-layout", "skill-management",
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
