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

// fal-ai and google-ai are product-owned skills (embedded via profileSkills,
// not the managed HyperFrames dependency source), and neither is part of the
// infographic pipeline's own stages -- product-infographic never routes to
// them. They exist so a chat session can generate AI video/image/voice/music
// for a long-form production the infographic route does not cover, one per
// provider (fal.ai's hosted third-party models vs. Google's own Gemini/Veo).
// This pins that both register and read back cleanly, and that adding them
// did not silently pull either into every infographic stage's attach list.
func TestGenerationSkillsRegisterAndStayOutOfTheInfographicPipeline(t *testing.T) {
	if err := RegisterProductSkills(); err != nil {
		t.Fatalf("RegisterProductSkills: %v", err)
	}

	cases := []struct {
		name         string
		requiredText []string
	}{
		{"fal-ai", []string{"SECRET_FAL_KEY", "Never invent a model ID", "@fal-ai/client"}},
		{"google-ai", []string{"SECRET_GEMINI_API_KEY", "Never invent a model ID", "@google/genai"}},
		{"video-model-selection", []string{"video-cinematography", "fal-ai", "google-ai", "Shot count vs. budget"}},
		{"video-cinematography", []string{"dolly is not zoom", "reference-image conditioning", "video-model-selection"}},
		{"video-storytelling", []string{"video-cinematography", "video-model-selection", "But-therefore, not and-then", "Scaling to true long-form"}},
	}
	for _, tc := range cases {
		if !skills.IsBuiltinSkill(tc.name) {
			t.Fatalf("%s did not register as a builtin skill", tc.name)
		}
		attached := skills.LoadAttachable("", []string{tc.name})
		if len(attached) != 1 {
			t.Fatalf("LoadAttachable(%s) = %v, want exactly one skill", tc.name, attached)
		}
		for _, want := range tc.requiredText {
			if !strings.Contains(attached[0].Content, want) {
				t.Fatalf("%s skill lost required content %q", tc.name, want)
			}
		}
	}

	manifest, err := VideoStudioManifest()
	if err != nil {
		t.Fatal(err)
	}
	inDefaultSet := map[string]bool{}
	for _, name := range manifest.Profile.Skills {
		inDefaultSet[name] = true
	}
	for _, tc := range cases {
		if !inDefaultSet[tc.name] {
			t.Fatalf("%s is not in the product's default skill set: %v", tc.name, manifest.Profile.Skills)
		}
	}

	generationSkillNames := map[string]bool{"fal-ai": true, "google-ai": true, "video-model-selection": true, "video-cinematography": true, "video-storytelling": true}
	for _, stage := range infographicPipeline.Stages {
		for _, name := range stage.Skills {
			if generationSkillNames[name] {
				t.Fatalf("infographic stage %q attaches %q; the infographic route stays on HyperFrames composition", stage.ID, name)
			}
		}
	}
}

// Character specs and reference images are the load-bearing artifact for
// keeping a subject consistent across a long-form production's many shots,
// and the workspace UI surfaces them by path. video-cinematography owns the
// `characters/` layout, but video-creation owns the file-layout convention
// it has to sit inside (work/ in chat, the step folder as a workflow stage)
// and the production.json record that points at it. A bare top-level
// `characters/` path would contradict both, so this pins that the two
// skills still agree on where the folder lives and who records it.
func TestCharacterArtifactsHaveOneAgreedLocation(t *testing.T) {
	if err := RegisterProductSkills(); err != nil {
		t.Fatalf("RegisterProductSkills: %v", err)
	}
	loadOne := func(name string) string {
		attached := skills.LoadAttachable("", []string{name})
		if len(attached) != 1 {
			t.Fatalf("LoadAttachable(%s) = %v, want exactly one skill", name, attached)
		}
		return attached[0].Content
	}

	cinematography := loadOne("video-cinematography")
	for _, want := range []string{"characters/<character-name>.md", "characters/<character-name>.png", "work/characters/"} {
		if !strings.Contains(cinematography, want) {
			t.Fatalf("video-cinematography no longer establishes %q as the character artifact path", want)
		}
	}
	if !strings.Contains(cinematography, "video-creation") {
		t.Fatal("video-cinematography does not defer to video-creation's file-layout rule for where characters/ sits")
	}

	creation := loadOne("video-creation")
	if !strings.Contains(creation, "work/characters/") {
		t.Fatal("video-creation's chat file layout no longer accounts for work/characters/")
	}
	if !strings.Contains(creation, "`characters/`") {
		t.Fatal("video-creation's production.json record no longer points at the characters/ artifacts")
	}
}

// Two mechanics the whole long-form route depends on, both of which read as
// prose an edit could quietly drop. Reference-image conditioning is the most
// reliable consistency technique there is, but it is inert unless the
// provider skills say how to actually send a local file -- fal.ai wants an
// uploaded URL, Gemini wants inline base64, and neither is guessable from
// the other. Narration ordering is the expensive one to get wrong: visuals
// are cut to measured narration, so a pipeline that generates clips first
// has to redo them.
func TestGenerationSkillsCarryTheMechanicsTheirGuidanceAssumes(t *testing.T) {
	if err := RegisterProductSkills(); err != nil {
		t.Fatalf("RegisterProductSkills: %v", err)
	}
	loadOne := func(name string) string {
		attached := skills.LoadAttachable("", []string{name})
		if len(attached) != 1 {
			t.Fatalf("LoadAttachable(%s) = %v, want exactly one skill", name, attached)
		}
		return attached[0].Content
	}

	falAI := loadOne("fal-ai")
	for _, want := range []string{"fal.storage.upload", "image_url", "characters/"} {
		if !strings.Contains(falAI, want) {
			t.Fatalf("fal-ai no longer shows how to send a local reference image (missing %q)", want)
		}
	}

	googleAI := loadOne("google-ai")
	for _, want := range []string{"inlineData", "base64", "characters/"} {
		if !strings.Contains(googleAI, want) {
			t.Fatalf("google-ai no longer shows how to send a local reference image (missing %q)", want)
		}
	}
	// Speech is what lets Google alone carry a narrated production. The PCM
	// warning earns its place: the bytes come back as raw samples, and writing
	// them straight to .wav yields a file nothing plays.
	for _, want := range []string{"responseModalities", "PCM", "speechConfig"} {
		if !strings.Contains(googleAI, want) {
			t.Fatalf("google-ai no longer covers text-to-speech (missing %q), which a Google-only production needs for narration", want)
		}
	}

	editing := loadOne("video-editing")
	for _, want := range []string{"Narration drives the timeline", "ffprobe", "video-storytelling"} {
		if !strings.Contains(editing, want) {
			t.Fatalf("video-editing no longer establishes narration-first assembly (missing %q)", want)
		}
	}
}

// video-editing is shared between both routes (infographic and long-form),
// so its content changes for one route's sake if not careful. This pins the
// AI-clip-stitching addition landed and stayed cross-referenced from the
// skill that surfaces the underlying problem (a generated clip's duration
// rarely matches the storyboard beat exactly).
func TestVideoEditingCoversStitchingIndependentlyGeneratedClips(t *testing.T) {
	if err := RegisterProductSkills(); err != nil {
		t.Fatalf("RegisterProductSkills: %v", err)
	}
	attached := skills.LoadAttachable("", []string{"video-editing"})
	if len(attached) != 1 {
		t.Fatalf("LoadAttachable(video-editing) = %v, want exactly one skill", attached)
	}
	content := attached[0].Content
	for _, want := range []string{"concat demuxer", "Duration mismatch is normal", "color mismatch"} {
		if !strings.Contains(content, want) {
			t.Fatalf("video-editing skill lost required content %q", want)
		}
	}

	modelSelection := skills.LoadAttachable("", []string{"video-model-selection"})
	if len(modelSelection) != 1 || !strings.Contains(modelSelection[0].Content, "video-editing") {
		t.Fatal("video-model-selection does not point to video-editing for stitching mismatched clip durations")
	}
}
