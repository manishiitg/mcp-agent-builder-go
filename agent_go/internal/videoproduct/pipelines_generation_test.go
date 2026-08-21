package videoproduct

import (
	"strings"
	"testing"
)

// The generation pipelines are the only ones that spend the user's money —
// the infographic route's assets stage makes ffmpeg placeholders and costs
// nothing. The gate is the authorization paragraph in the stage's own prompt,
// so this pins exactly which stages carry it. Naming them here is the point:
// a new stage that starts calling a paid API without the gate fails this
// test, and removing the gate from one of these fails it too.
func TestEverySpendingStageCarriesTheApprovalGate(t *testing.T) {
	const gateMarker = "ONLY from the human input for THIS stage"

	wantGated := map[string]bool{
		"longform-characters":  true,
		"longform-narration":   true,
		"longform-generate":    true,
		"shortform-characters": true,
		"shortform-narration":  true,
		"shortform-generate":   true,
	}

	for _, pipeline := range []*Pipeline{longformPipeline, shortformPipeline} {
		for _, stage := range pipeline.Stages {
			hasGate := strings.Contains(stage.Description, gateMarker)
			switch {
			case wantGated[stage.ID] && !hasGate:
				t.Fatalf("%s spends money but its prompt carries no approval gate, so nothing stops it billing the user", stage.ID)
			case !wantGated[stage.ID] && hasGate:
				t.Fatalf("%s carries the approval gate but is not listed as a spending stage; add it to wantGated so the paid surface stays written down in one place", stage.ID)
			}
			delete(wantGated, stage.ID)
		}
	}
	for id := range wantGated {
		t.Fatalf("%s is listed as a spending stage but no longer exists in either pipeline", id)
	}

	// The gate is worthless if it accepts approval granted anywhere else --
	// that is exactly how an approved storyboard becomes an unapproved bill.
	if !strings.Contains(generationSpendGate("x"), "Approval of an earlier stage") {
		t.Fatal("the spend gate no longer rejects approval inherited from an earlier stage")
	}
}

func TestShortformStagesPutDirectionAndMeasuredNarrationBeforeShots(t *testing.T) {
	index := map[string]int{}
	for i, stage := range shortformPipeline.Stages {
		index[stage.ID] = i
	}
	for _, ordered := range [][2]string{
		{"shortform-script", "shortform-characters"},
		{"shortform-characters", "shortform-look-sound"},
		{"shortform-look-sound", "shortform-narration"},
		{"shortform-narration", "shortform-shotlist"},
		{"shortform-shotlist", "shortform-generate"},
		{"shortform-generate", "shortform-assemble"},
		{"shortform-assemble", "shortform-check"},
	} {
		before, after := ordered[0], ordered[1]
		if index[before] >= index[after] {
			t.Fatalf("%s must run before %s", before, after)
		}
	}
	characters := shortformPipeline.Stages[index["shortform-characters"]].Description
	for _, required := range []string{"budget", "recommended", "premium", "explicitly approved", "separately from video-per-second cost", "NEVER silently select it", "selected provider/model"} {
		if !strings.Contains(characters, required) {
			t.Fatalf("short-form character step is missing %q", required)
		}
	}
	generate := shortformPipeline.Stages[index["shortform-generate"]].Description
	for _, forbidden := range []string{"HyperFrames", "HTML panels", "infographic route"} {
		if !strings.Contains(generate, forbidden) {
			t.Fatalf("short-form generation does not explicitly reject %q", forbidden)
		}
	}

	lookSound := shortformPipeline.Stages[index["shortform-look-sound"]].Description
	for _, required := range []string{"Locations and backgrounds", "Wardrobe, props, and continuity", "Lighting and visual palette", "Speech and voices", "Music", "Ambience and sound effects", "Captions", "BEFORE committing the video model", "visible lip-synced dialogue", "off-camera TTS voiceover", "hybrid with native dialogue", "cost", "edit-complexity tradeoff", "explicit choice", "audio-incapable model silently decide", "synchronized native audio", "Separate TTS is for off-camera voiceover", "Never silently turn"} {
		if !strings.Contains(lookSound, required) {
			t.Fatalf("short-form look/sound step is missing %q", required)
		}
	}

	narration := shortformPipeline.Stages[index["shortform-narration"]].Description
	for _, required := range []string{"ffprobe", "REAL measured duration", "Missing, unreadable, silent, or unmeasured", "visible native dialogue", "Off-camera instructional voiceover is not optional"} {
		if !strings.Contains(narration, required) {
			t.Fatalf("short-form narration step is missing %q", required)
		}
	}

	shotlist := shortformPipeline.Stages[index["shortform-shotlist"]].Description
	for _, required := range []string{"shortform-look-sound.md", "shortform-narration.md", "MEASURED", "Never fit narration"} {
		if !strings.Contains(shotlist, required) {
			t.Fatalf("short-form shot list is missing %q", required)
		}
	}

	assemble := shortformPipeline.Stages[index["shortform-assemble"]].Description
	for _, required := range []string{"required narration segment", "Cut visuals to the measured narration timeline", "music, ambience, sound effects, and captions", "selected background/look decisions", "native-dialogue source per beat"} {
		if !strings.Contains(assemble, required) {
			t.Fatalf("short-form assembly is missing %q", required)
		}
	}

	check := shortformPipeline.Stages[index["shortform-check"]].Description
	for _, required := range []string{"missing or silent narration is a deterministic failure", "may not be marked not_applicable", "narration_alignment"} {
		if !strings.Contains(check, required) {
			t.Fatalf("short-form QA is missing %q", required)
		}
	}
}

func TestCharacterModelIsUserSelectedBeforeAnyReferenceSpend(t *testing.T) {
	for _, pipeline := range []*Pipeline{longformPipeline, shortformPipeline} {
		var characters, shotlist string
		for _, stage := range pipeline.Stages {
			switch stage.ID {
			case pipeline.ID + "-characters":
				characters = stage.Description
			case pipeline.ID + "-shotlist":
				shotlist = stage.Description
			}
		}
		for _, required := range []string{
			"live-verified viable character-model choices",
			"NEVER silently select it",
			"selected provider/model",
			"explicit approval to spend",
			"show_character",
		} {
			if !strings.Contains(characters, required) {
				t.Fatalf("%s character stage is missing %q", pipeline.ID, required)
			}
		}
		if !strings.Contains(shotlist, "explicitly approves its displayed reference") {
			t.Fatalf("%s shot list can proceed without explicit approval of the displayed character reference", pipeline.ID)
		}
	}
}

// Stage outputs are resolved by filename against prior steps and disambiguated
// by which file exists on disk. That is safe while exactly one pipeline runs,
// but a workshop or partial re-run reuses the run folder, so a stale same-named
// file from a different pipeline can win on array order and be read silently.
// Keeping every context_output distinct across pipelines removes the hazard
// rather than relying on the folder always being clean.
func TestPipelineOutputsDoNotCollideAcrossPipelines(t *testing.T) {
	owner := map[string]string{}
	for _, pipeline := range pipelineRegistry {
		for _, stage := range pipeline.Stages {
			if stage.Output == "" {
				continue
			}
			if prior, seen := owner[stage.Output]; seen {
				t.Fatalf("%q is the context_output of both %s and %s/%s; a re-run can resolve it to the wrong pipeline's file", stage.Output, prior, pipeline.ID, stage.ID)
			}
			owner[stage.Output] = pipeline.ID + "/" + stage.ID
		}
	}
}

// The long-form pipeline encodes an ordering the skills argue for and that is
// expensive to get wrong: narration is generated and MEASURED before the shot
// list exists, because visuals are cut to real audio durations rather than
// estimates. Characters are defined before any shot of them is generated, for
// the same reason in the visual dimension. Reordering these stages silently
// reintroduces the two defects the pipeline exists to prevent.
func TestLongformStagesKeepTheirLoadBearingOrder(t *testing.T) {
	index := map[string]int{}
	for i, stage := range longformPipeline.Stages {
		index[stage.ID] = i
	}

	for _, ordered := range [][2]string{
		{"longform-script", "longform-characters"},
		{"longform-characters", "longform-generate"},
		{"longform-script", "longform-narration"},
		{"longform-narration", "longform-shotlist"},
		{"longform-shotlist", "longform-generate"},
		{"longform-generate", "longform-assemble"},
		{"longform-assemble", "longform-check"},
	} {
		before, after := ordered[0], ordered[1]
		if index[before] >= index[after] {
			t.Fatalf("%s must run before %s", before, after)
		}
	}

	shotlist := longformPipeline.Stages[index["longform-shotlist"]].Description
	if !strings.Contains(shotlist, "MEASURED") {
		t.Fatal("the shot list stage no longer derives durations from measured narration, which is what stops visuals being cut to an estimate")
	}
}

// Both generation pipelines route to skills the product owns, and neither may
// reach for the HyperFrames composition stack -- that is the infographic
// route's job, and pulling it in here is how a footage production quietly
// turns into a slideshow.
func TestGenerationPipelinesUseGenerationSkillsOnly(t *testing.T) {
	generationSkills := map[string]bool{}
	for _, name := range []string{"fal-ai", "google-ai", "seeddance-api", "longform-cinematic-video", "video-model-selection", "video-cinematography", "video-storytelling"} {
		generationSkills[name] = true
	}

	for _, pipeline := range []*Pipeline{longformPipeline, shortformPipeline} {
		sawGenerationSkill := false
		for _, stage := range pipeline.Stages {
			for _, skill := range stage.Skills {
				if strings.HasPrefix(skill, "hyperframes") {
					t.Fatalf("%s/%s attaches %q; generation pipelines produce footage, not HyperFrames compositions", pipeline.ID, stage.ID, skill)
				}
				if generationSkills[skill] {
					sawGenerationSkill = true
				}
			}
			if stage.Description == "" {
				t.Fatalf("%s/%s has no description; its agent would run with no instruction", pipeline.ID, stage.ID)
			}
			if stage.Output == "" {
				t.Fatalf("%s/%s declares no output, so nothing downstream can depend on it", pipeline.ID, stage.ID)
			}
		}
		if !sawGenerationSkill {
			t.Fatalf("%s attaches none of the generation skills it exists to run", pipeline.ID)
		}
	}
}

func TestLongformPipelineOwnsCinematicContinuityAndSeamEvidence(t *testing.T) {
	requiredSkillStages := map[string]bool{
		"longform-brief":    true,
		"longform-script":   true,
		"longform-shotlist": true,
		"longform-generate": true,
		"longform-assemble": true,
		"longform-check":    true,
	}
	requiredArtifacts := map[string][]string{
		"longform-shotlist": {"longform-sequence-plan.json"},
		"longform-generate": {"longform-continuity-ledger.json"},
		"longform-assemble": {"longform-final.mp4", "longform-edit-decision-list.json"},
		"longform-check":    {"quality-report.json", "longform-seam-report.json"},
	}

	for _, stage := range longformPipeline.Stages {
		if requiredSkillStages[stage.ID] {
			attached := false
			for _, skill := range stage.Skills {
				if skill == "longform-cinematic-video" {
					attached = true
					break
				}
			}
			if !attached {
				t.Fatalf("%s must attach longform-cinematic-video: %v", stage.ID, stage.Skills)
			}
		}

		for _, required := range requiredArtifacts[stage.ID] {
			found := false
			for _, artifact := range stage.Artifacts {
				if artifact == required {
					found = true
					break
				}
			}
			if !found {
				t.Fatalf("%s must require %s: %v", stage.ID, required, stage.Artifacts)
			}
		}
	}
}

// A user with only one provider's key must still be able to finish a whole
// production. Narration is where that nearly broke: the stage used to attach
// only fal-ai and tell the agent to use it for TTS, which left a Google-only
// user stuck halfway through a long-form run with a script and no voiceover.
// Both providers generate speech, so both are attached and the prompt defers
// to whichever the brief committed to.
func TestNarrationIsNotLockedToOneProvider(t *testing.T) {
	var narration PipelineStage
	for _, stage := range longformPipeline.Stages {
		if stage.ID == "longform-narration" {
			narration = stage
		}
	}
	if narration.ID == "" {
		t.Fatal("longform-narration stage is missing")
	}

	attached := map[string]bool{}
	for _, skill := range narration.Skills {
		attached[skill] = true
	}
	for _, provider := range []string{"fal-ai", "google-ai"} {
		if !attached[provider] {
			t.Fatalf("longform-narration does not attach %s, so an agent with only that provider's key cannot generate narration: %v", provider, narration.Skills)
		}
	}
	if strings.Contains(narration.Description, "Use fal-ai for TTS") {
		t.Fatal("longform-narration hard-defaults to fal-ai again, which blocks a Google-only production")
	}
}

// Routing is what makes the new pipelines reachable at all; without a route
// the stages sit in plan.json and never execute. The default deliberately
// stays on the pipeline that cannot spend money.
func TestGenerationPipelinesAreRoutableAndNotTheDefault(t *testing.T) {
	plan := planForAll(pipelineRegistry)
	steps := plan["steps"].([]map[string]interface{})
	if len(steps) == 0 || steps[0]["type"] != "routing" {
		t.Fatalf("plan does not open with a routing step: %+v", steps[0])
	}

	routes := steps[0]["routes"].([]map[string]interface{})
	routed := map[string]string{}
	for _, route := range routes {
		routed[route["route_id"].(string)] = route["next_step_id"].(string)
	}
	for _, want := range []string{"longform", "shortform", "infographic", "quality"} {
		if routed[want] == "" {
			t.Fatalf("no route reaches the %s pipeline: %+v", want, routed)
		}
	}
	if routed["longform"] != "longform-brief" || routed["shortform"] != "shortform-brief" {
		t.Fatalf("generation routes must enter at their brief stage, got %+v", routed)
	}
	if got := steps[0]["default_route_id"]; got != "infographic" {
		t.Fatalf("default route = %v, want infographic -- the default must not be a pipeline that spends money", got)
	}

	if DefaultPipeline().ID != "infographic" {
		t.Fatalf("DefaultPipeline() = %s, want infographic", DefaultPipeline().ID)
	}
}
