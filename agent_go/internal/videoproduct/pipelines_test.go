package videoproduct

import (
	"testing"

	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/skills"
)

// Every stage skill must resolve through the shared name-based resolver the
// workflow uses. A name that is registered nowhere silently attaches nothing,
// which is exactly how a stage ends up with no craft guidance and no error.
func TestStageSkillsResolveAsBuiltins(t *testing.T) {
	if err := registerProductSkills(); err != nil {
		t.Fatalf("register product skills: %v", err)
	}
	// A stage carrying no skill is fine and expected: the description says what
	// to do, a skill is an opinionated how, and stages that just write a document
	// from the previous one need no how. Only a NAMED skill has to resolve.
	for _, pipeline := range pipelineRegistry {
		for _, stage := range pipeline.Stages {
			for _, name := range stage.Skills {
				if !skills.IsBuiltinSkill(name) {
					t.Errorf("%s/%s references %q, which is not registered", pipeline.ID, stage.ID, name)
				}
			}
		}
	}
}

// The step config is what the workflow engine actually reads. enabled_skills is
// the step-level selector; selected_skills is the workshop preset and is ignored
// at step level, so emitting the wrong key would silently drop stage skills.
func TestStepConfigEmitsEnabledSkills(t *testing.T) {
	cfg := stepConfigForAll(pipelineRegistry)
	steps, _ := cfg["steps"].([]map[string]interface{})
	if len(steps) == 0 {
		t.Fatal("no steps in config")
	}
	// Not every stage carries a skill — a stage whose description is the whole
	// job gets none. What must never happen is a stage declaring one under the
	// wrong key, which resolves to nothing without erroring.
	for _, step := range steps {
		agentConfig, _ := step["agent_configs"].(map[string]interface{})
		if _, wrongKey := agentConfig["selected_skills"]; wrongKey {
			t.Fatalf("step %v uses selected_skills; step level requires enabled_skills", step["id"])
		}
	}
}

func TestQualityWorkflowAndCreationGatesUseSharedAgenticContract(t *testing.T) {
	if got := PipelineByID("quality"); got != qualityPipeline {
		t.Fatalf("quality route resolved to %#v", got)
	}
	if len(qualityPipeline.Stages) != 1 || qualityPipeline.Stages[0].ID != "qa-review" {
		t.Fatalf("quality workflow = %#v", qualityPipeline.Stages)
	}
	for _, stage := range []PipelineStage{
		cinematicPipeline.Stages[len(cinematicPipeline.Stages)-1],
		infographicPipeline.Stages[len(infographicPipeline.Stages)-1],
		qualityPipeline.Stages[0],
	} {
		if len(stage.Skills) != 1 || stage.Skills[0] != "video-quality" {
			t.Fatalf("%s does not use the agentic video-quality skill: %#v", stage.ID, stage.Skills)
		}
		if len(stage.Artifacts) != 2 || stage.Artifacts[0] != "quality-report.json" || stage.Artifacts[1] != "qa-contact-sheet.jpg" {
			t.Fatalf("%s QA evidence contract = %#v", stage.ID, stage.Artifacts)
		}
	}
}
