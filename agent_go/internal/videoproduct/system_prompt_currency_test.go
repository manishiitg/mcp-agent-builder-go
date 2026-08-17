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

	// These pipelines bill the user per call, unlike the infographic route.
	// The prompt has to say so, because the agent decides when to start.
	if !strings.Contains(strings.ToLower(text), "approval of a storyboard is not approval to spend") {
		t.Fatal("the system prompt no longer warns that an approved plan is not approval to spend")
	}
	if !strings.Contains(text, "project base estimate") || !strings.Contains(text, "approved retry allowance") {
		t.Fatal("the system prompt no longer requires a costed model choice before paid generation")
	}

	// Chat and run_full_workflow are two different contracts: chat pauses at
	// checkpoints, the workflow runs end to end unattended. Losing this line
	// is how chat quietly turns into a silent workflow run.
	if !strings.Contains(text, "run_full_workflow") || !strings.Contains(strings.ToLower(text), "not friction to minimize") {
		t.Fatal("the system prompt no longer distinguishes chat's checkpoints from run_full_workflow's unattended mode")
	}
}
