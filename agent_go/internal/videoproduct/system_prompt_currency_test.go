package videoproduct

import (
	"strings"
	"testing"
)

// The system prompt is the one place describing the product that no compiler
// checks, so it drifts silently: it still called show_video "the one Video
// Studio presentation tool" after two more were added, still listed two
// workflows after four existed, and still sent users to a "Videos panel"
// that had been renamed. Each of those is invisible until an agent acts on
// it in front of a user. These assertions tie the prompt to the code it
// describes.
func TestSystemPromptMatchesTheProductItDescribes(t *testing.T) {
	manifest, err := VideoStudioManifest()
	if err != nil {
		t.Fatal(err)
	}
	prompt, err := productConfigFiles.ReadFile(manifest.Prompt.File)
	if err != nil {
		t.Fatalf("read system prompt %q: %v", manifest.Prompt.File, err)
	}
	text := string(prompt)

	// Every presentation tool the product admits must be one the prompt tells
	// the agent when to call. A registered tool nobody is told to use is dead
	// weight; the character one especially, since its whole value is being
	// called before generation rather than after.
	for _, tool := range []string{"show_video", "show_character", "show_document"} {
		if !strings.Contains(text, tool) {
			t.Fatalf("the system prompt never mentions %s, so the agent will not call it", tool)
		}
	}

	// Every routable pipeline must be named, or the agent cannot offer it.
	for _, pipeline := range pipelineRegistry {
		if !strings.Contains(text, "`"+pipeline.ID+"`") {
			t.Fatalf("the system prompt does not name the %s workflow, so it is unreachable in conversation", pipeline.ID)
		}
	}

	// The panel name is user-facing: the prompt tells the agent to say where
	// the video is, and naming a panel that no longer exists sends the user
	// looking for a tab that is not there.
	if strings.Contains(text, "Videos panel") {
		t.Fatal("the system prompt still refers to the Videos panel, which is now the Production panel")
	}
	if !strings.Contains(text, "Production panel") {
		t.Fatal("the system prompt no longer tells the agent where finished work appears")
	}

	// Paid generation bills the user per call. HyperFrames can be used for a
	// deterministic insert without becoming a separate product route.
	if !strings.Contains(strings.ToLower(text), "approval of a storyboard is not approval to spend") {
		t.Fatal("the system prompt no longer warns that an approved plan is not approval to spend")
	}
	if !strings.Contains(text, "project base estimate") || !strings.Contains(text, "approved retry allowance") {
		t.Fatal("the system prompt no longer requires a costed model choice before paid generation")
	}

	if strings.Contains(text, "run_full_workflow") || !strings.Contains(text, "execute_step") {
		t.Fatal("Video Studio must describe individual stage execution only")
	}
	if !strings.Contains(strings.ToLower(text), "only creative product") || !strings.Contains(text, "longform-cinematic-video") {
		t.Fatal("the system prompt no longer defaults fresh productions to cinematic direction")
	}
	if strings.Contains(text, "The available stage plans are `infographic`") || !strings.Contains(text, "optional technique inside a cinematic production") {
		t.Fatal("the system prompt must keep HyperFrames inside cinematic production rather than exposing an infographic product")
	}
	for _, skill := range manifest.Profile.Skills {
		if skill == "product-infographic" {
			t.Fatal("product-infographic remains exposed in the Video Studio profile")
		}
	}
	for _, audioContract := range []string{
		"For both direct chat and individual workflow steps",
		"video-look-sound",
		"locations/backgrounds",
		"native synchronized dialogue",
		"explicitly show the user the speech-design tradeoff",
		"visible lip-synced dialogue",
		"hybrid with native dialogue",
		"Never let an audio-incapable endpoint silently decide",
		"Use separate TTS only for off-camera voiceover",
		"measure its real duration",
		"build the shot list around the measured audio",
		"without its promised audio",
	} {
		if !strings.Contains(text, audioContract) {
			t.Fatalf("the system prompt lost the direct-chat look/sound contract: missing %q", audioContract)
		}
	}
	for _, contract := range []string{"specialist skills", "context dependencies", "validation schema", "human_input", "Do not ask the user for a step ID or group name"} {
		if !strings.Contains(text, contract) {
			t.Fatalf("the system prompt no longer explains execute_step's recipe contract: missing %q", contract)
		}
	}
	for _, source := range []string{"planning/plan.json", "planning/step_config.json", "validation_schema.files", "enabled_skills"} {
		if !strings.Contains(text, source) {
			t.Fatalf("the system prompt no longer gives the agent a source-of-truth step discovery query: missing %q", source)
		}
	}
}
