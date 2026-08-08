package videoproduct

import (
	"testing"

	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/skills"
)

// Every stage skill must either resolve from the shared builtin registry or be
// installed by the product's declarative external-skill dependencies. A name
// that is neither silently attaches nothing, which is exactly how a stage ends
// up with no craft guidance and no error.
func TestStageSkillsResolveFromBuiltinsOrProductDependencies(t *testing.T) {
	if err := RegisterProductSkills(); err != nil {
		t.Fatalf("register product skills: %v", err)
	}
	manifest := mustVideoStudioManifest()
	external := map[string]bool{}
	for _, source := range manifest.Dependencies.Skills {
		for _, name := range source.Install {
			external[name] = true
		}
	}
	// A stage carrying no skill is fine and expected: the description says what
	// to do, a skill is an opinionated how, and stages that just write a document
	// from the previous one need no how. Only a NAMED skill has to resolve.
	for _, pipeline := range pipelineRegistry {
		for _, stage := range pipeline.Stages {
			for _, name := range stage.Skills {
				if !skills.IsBuiltinSkill(name) && !external[name] {
					t.Errorf("%s/%s references %q, which is neither builtin nor YAML-installed", pipeline.ID, stage.ID, name)
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

func TestInfographicWorkflowUsesCritiqueAndRefinementGates(t *testing.T) {
	plan := planForAll([]*Pipeline{infographicPipeline})
	steps := plan["steps"].([]map[string]interface{})
	byID := map[string]map[string]interface{}{}
	for _, step := range steps {
		if id, _ := step["id"].(string); id != "" {
			byID[id] = step
		}
	}

	for _, gate := range []struct {
		id       string
		output   string
		artifact string
		depends  []string
	}{
		{
			id:       "infographic-creative-critique",
			output:   "creative-review.md",
			artifact: "creative-scorecard.json",
			depends:  []string{"BRIEF.md", "STORYBOARD.md", "SCRIPT.md", "frame.md"},
		},
		{
			id:       "infographic-composition-critique",
			output:   "composition-critique.md",
			artifact: "hyperframes-project-reviewed.tgz",
			depends:  []string{"creative-review.md", "creative-scorecard.json", "hyperframes-project.tgz"},
		},
		{
			id:       "infographic-render-critique",
			output:   "render-critique.md",
			artifact: "final-reviewed.mp4",
			depends:  []string{"composition-critique.md", "hyperframes-project-reviewed.tgz", "final.mp4", "hyperframes-project-final.tgz"},
		},
	} {
		step := byID[gate.id]
		if step == nil {
			t.Fatalf("missing critique gate %q", gate.id)
		}
		if got := step["context_output"]; got != gate.output {
			t.Fatalf("%s output = %v, want %q", gate.id, got, gate.output)
		}
		deps, _ := step["context_dependencies"].([]string)
		available := map[string]bool{}
		for _, dependency := range deps {
			available[dependency] = true
		}
		for _, dependency := range gate.depends {
			if !available[dependency] {
				t.Fatalf("%s dependencies omit %q: %v", gate.id, dependency, deps)
			}
		}
		required := step["validation_schema"].(map[string]interface{})["files"].([]map[string]interface{})
		foundArtifact := false
		for _, file := range required {
			if file["file_name"] == gate.artifact {
				foundArtifact = true
			}
		}
		if !foundArtifact {
			t.Fatalf("%s does not require its durable handoff %q: %v", gate.id, gate.artifact, required)
		}
		if step["has_loop"] != false {
			t.Fatalf("%s must use its explicit bounded critique/refinement contract, not the removed engine loop: %v", gate.id, step)
		}
	}

	quality := byID["infographic-check"]
	if quality == nil {
		t.Fatal("missing final quality check")
	}
	qualityDeps, _ := quality["context_dependencies"].([]string)
	available := map[string]bool{}
	for _, dependency := range qualityDeps {
		available[dependency] = true
	}
	for _, dependency := range []string{"render-critique.md", "render-scorecard.json", "final-reviewed.mp4", "hyperframes-project-reviewed-final.tgz"} {
		if !available[dependency] {
			t.Fatalf("quality check does not consume the approved critique handoff %q: %v", dependency, qualityDeps)
		}
	}
}
