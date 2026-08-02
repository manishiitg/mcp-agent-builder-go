package step_based_workflow

import (
	"strings"
	"testing"

	"github.com/manishiitg/multi-llm-provider-go/llmtypes"
)

func TestWorkshopStageReferenceSurfaceDoesNotDuplicateExistingSkill(t *testing.T) {
	existing := []*llmtypes.Skill{{Name: "builder-reference"}}

	missing, err := missingWorkshopReferenceSurfaceSkills(existing)
	if err != nil {
		t.Fatalf("build missing workshop reference surface: %v", err)
	}

	if got := strings.Join(backgroundSkillNames(missing), ","); got != "system-tools,workflow-commands" {
		t.Fatalf("missing reference skills = %q, want system-tools,workflow-commands", got)
	}

	combined := append(append([]*llmtypes.Skill(nil), existing...), missing...)
	seen := make(map[string]struct{}, len(combined))
	for _, skill := range combined {
		if skill == nil {
			t.Fatal("combined reference surface contains a nil skill")
		}
		name := strings.TrimSpace(skill.Name)
		if _, duplicate := seen[name]; duplicate {
			t.Fatalf("combined reference surface contains duplicate skill name %q", name)
		}
		seen[name] = struct{}{}
	}
}

func TestWorkshopStageReferenceSurfaceContainsAllThreeSkillBundles(t *testing.T) {
	missing, err := missingWorkshopReferenceSurfaceSkills(nil)
	if err != nil {
		t.Fatalf("build workshop reference surface: %v", err)
	}
	if got := strings.Join(backgroundSkillNames(missing), ","); got != "system-tools,builder-reference,workflow-commands" {
		t.Fatalf("reference surface skills = %q, want system-tools,builder-reference,workflow-commands", got)
	}
}
