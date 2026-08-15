package videoproduct

import (
	"strings"
	"testing"
)

// The generation pipelines are the only ones that spend the user's money —
// the infographic route's assets stage makes ffmpeg placeholders and costs
// nothing. RequiresApproval is a documentation flag the runtime never reads,
// so the gate that actually works is the authorization paragraph in the
// stage's own prompt. This pins the two together: a stage marked as spending
// must carry the gate, and a stage carrying the gate must be marked. Either
// half alone is a stage that can quietly bill someone.
func TestEverySpendingStageCarriesTheApprovalGate(t *testing.T) {
	const gateMarker = "ONLY from the human input for THIS stage"

	for _, pipeline := range []*Pipeline{longformPipeline, shortformPipeline} {
		for _, stage := range pipeline.Stages {
			hasGate := strings.Contains(stage.Description, gateMarker)
			switch {
			case stage.RequiresApproval && !hasGate:
				t.Fatalf("%s/%s is marked RequiresApproval but its prompt carries no approval gate, so nothing stops it spending", pipeline.ID, stage.ID)
			case !stage.RequiresApproval && hasGate:
				t.Fatalf("%s/%s carries the approval gate but is not marked RequiresApproval; the flag is the only place a reader sees which stages cost money", pipeline.ID, stage.ID)
			}
		}
	}

	// The gate is worthless if it accepts approval granted anywhere else --
	// that is exactly how an approved storyboard becomes an unapproved bill.
	if !strings.Contains(generationSpendGate("x"), "Approval of an earlier stage") {
		t.Fatal("the spend gate no longer rejects approval inherited from an earlier stage")
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
	for _, name := range []string{"fal-ai", "google-ai", "video-model-selection", "video-cinematography", "video-storytelling"} {
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
