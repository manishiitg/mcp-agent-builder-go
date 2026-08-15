package videoproduct

import (
	"strings"
	"testing"

	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/skills"
)

func TestVideoStudioDeclaresManagedHyperFramesSkills(t *testing.T) {
	manifest, err := VideoStudioManifest()
	if err != nil {
		t.Fatal(err)
	}
	if len(manifest.Dependencies.Skills) != 1 {
		t.Fatalf("managed sources = %+v", manifest.Dependencies.Skills)
	}
	source := manifest.Dependencies.Skills[0]
	if source.ID != "hyperframes-skills" || source.Source != "heygen-com/hyperframes" || source.RefreshHours != 24 {
		t.Fatalf("HyperFrames source = %+v", source)
	}
	installed := map[string]bool{}
	for _, name := range source.Install {
		installed[name] = true
	}
	for _, required := range []string{"hyperframes", "hyperframes-core", "hyperframes-animation", "hyperframes-creative", "hyperframes-cli", "media-use", "hyperframes-registry", "hyperframes-keyframes", "general-video", "product-launch-video", "faceless-explainer", "motion-graphics"} {
		if !installed[required] {
			t.Fatalf("HyperFrames installed skills omit %q: %+v", required, source.Install)
		}
	}
	if len(source.Attach) != 1 || source.Attach[0] != "hyperframes" {
		t.Fatalf("HyperFrames always-attached skills = %+v", source.Attach)
	}
	if len(manifest.Dependencies.CLI) != 1 {
		t.Fatalf("CLI dependencies = %+v", manifest.Dependencies.CLI)
	}
	cli := manifest.Dependencies.CLI[0]
	if cli.ID != "hyperframes-cli" || cli.Package.Name != "hyperframes" || cli.Execution.Mode != "npx" || cli.Execution.Binary != "hyperframes" {
		t.Fatalf("HyperFrames CLI = %+v", cli)
	}
	if len(manifest.Dependencies.MCPServers) != 0 {
		t.Fatalf("Video Studio MCP servers = %+v", manifest.Dependencies.MCPServers)
	}
}

func TestInfographicWorkflowAttachesManagedHyperFramesSkills(t *testing.T) {
	stages := infographicPipeline.Stages
	for _, stage := range stages {
		if stage.ID != "infographic-design" && stage.ID != "infographic-render" && stage.ID != "infographic-composition-critique" && stage.ID != "infographic-render-critique" {
			continue
		}
		found := map[string]bool{}
		for _, skill := range stage.Skills {
			found[skill] = true
		}
		if stage.ID == "infographic-design" && !found["hyperframes"] {
			t.Fatalf("%s does not attach the HyperFrames authoring skill: %v", stage.ID, stage.Skills)
		}
		if stage.ID == "infographic-render" && !found["hyperframes-cli"] {
			t.Fatalf("%s does not attach the HyperFrames CLI skill: %v", stage.ID, stage.Skills)
		}
		if (stage.ID == "infographic-composition-critique" || stage.ID == "infographic-render-critique") && !found["hyperframes-quality"] {
			t.Fatalf("%s does not attach the HyperFrames quality gate: %v", stage.ID, stage.Skills)
		}
	}
}

func TestInfographicWorkflowCarriesDurableArtifactsForward(t *testing.T) {
	plan := planForAll([]*Pipeline{infographicPipeline})
	steps := plan["steps"].([]map[string]interface{})
	var render, quality map[string]interface{}
	for _, step := range steps {
		switch step["id"] {
		case "infographic-render":
			render = step
		case "infographic-check":
			quality = step
		}
	}
	if render == nil || quality == nil {
		t.Fatalf("missing infographic render/quality steps: %+v", steps)
	}
	renderDeps := map[string]bool{}
	for _, name := range render["context_dependencies"].([]string) {
		renderDeps[name] = true
	}
	for _, name := range []string{"BRIEF.md", "STORYBOARD.md", "SCRIPT.md", "frame.md", "build-report.md", "hyperframes-project.tgz"} {
		if !renderDeps[name] {
			t.Fatalf("render dependencies omit %q: %+v", name, renderDeps)
		}
	}
	qualityDeps := map[string]bool{}
	for _, name := range quality["context_dependencies"].([]string) {
		qualityDeps[name] = true
	}
	for _, name := range []string{"BRIEF.md", "render-report.md", "final.mp4", "hyperframes-project-final.tgz"} {
		if !qualityDeps[name] {
			t.Fatalf("quality dependencies omit %q: %+v", name, qualityDeps)
		}
	}
}

// fal-ai is a product-owned skill (embedded via profileSkills, not the
// managed HyperFrames dependency source), and it is not part of the
// infographic pipeline's own stages -- product-infographic never routes to
// it. It exists so a chat session can generate AI video/image/voice/music
// for a long-form production the infographic route does not cover. This
// pins that it registers and reads back cleanly, and that adding it did not
// silently pull it into every infographic stage's attach list.
func TestFalAISkillRegistersAndStaysOutOfTheInfographicPipeline(t *testing.T) {
	if err := RegisterProductSkills(); err != nil {
		t.Fatalf("RegisterProductSkills: %v", err)
	}
	if !skills.IsBuiltinSkill("fal-ai") {
		t.Fatal("fal-ai did not register as a builtin skill")
	}
	attached := skills.LoadAttachable("", []string{"fal-ai"})
	if len(attached) != 1 {
		t.Fatalf("LoadAttachable(fal-ai) = %v, want exactly one skill", attached)
	}
	if !strings.Contains(attached[0].Content, "SECRET_FAL_KEY") {
		t.Fatal("fal-ai skill lost the SECRET_ prefix translation guidance")
	}
	if !strings.Contains(attached[0].Content, "Never invent a model ID") {
		t.Fatal("fal-ai skill lost its no-guessed-model-ID rule")
	}

	manifest, err := VideoStudioManifest()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, name := range manifest.Profile.Skills {
		if name == "fal-ai" {
			found = true
		}
	}
	if !found {
		t.Fatalf("fal-ai is not in the product's default skill set: %v", manifest.Profile.Skills)
	}

	for _, stage := range infographicPipeline.Stages {
		for _, name := range stage.Skills {
			if name == "fal-ai" {
				t.Fatalf("infographic stage %q attaches fal-ai; the infographic route stays on HyperFrames composition", stage.ID)
			}
		}
	}
}
