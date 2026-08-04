package videoproduct

// A pipeline is a workflow: an ordered set of visible stages that take a brief
// to a finished video. Pipelines are data so a new one (explainer, screen demo,
// documentary) is a definition rather than a code change. The shape is ported
// from OpenMontage's pipeline_defs/*.yaml — see docs/handover/video_studio_handover.md.

// PipelineStage is one visible stage and the contract for the agent that runs it.
type PipelineStage struct {
	ID          string
	Title       string // user-facing stage name
	Description string // the stage agent's instruction — internal, never shown to the user
	// Summary is the one-line, user-facing explanation of this stage. Kept
	// separate from Description because Description is a prompt full of tool
	// names and internal rules, and this product deliberately shows users the
	// work rather than the framework.
	Summary string
	Output  string // artifact this stage must produce, and the next stage's dependency
	// Skills scopes what this stage's agent knows. The main project chat agent
	// gets every skill so it can route and answer anything; a stage agent gets
	// only its own director skill plus the shared meta/core ones. Empty means
	// "no stage-specific skill yet" (wired in a later phase).
	Skills []string
	// RequiresApproval marks a stage that must not spend money or produce
	// irreversible output without explicit user approval in its human input.
	RequiresApproval bool
}

// Pipeline is one named workflow.
type Pipeline struct {
	ID          string
	Name        string // user-facing, e.g. "Cinematic video"
	Description string
	WhenToUse   string // routing hint: how a brief maps to this pipeline
	Stages      []PipelineStage
}

// Stage descriptions are the stage agents' prompts. They live here rather than
// in the plan builder so a pipeline is readable as one definition.
var cinematicStageDescriptions = map[string]string{
	"research":   "Study the user's brief, conversation constraints, and every relevant file in uploads/. Separate verified facts from assumptions. Write research.md with the audience, goal, factual source notes, risks, open decisions, and a concise recommended direction. Do not generate media.",
	"proposal":   "Use research.md to create a concrete creative proposal. Write proposal.md with the central idea, tone, format, target duration, aspect ratio, narrative arc, visual language, audio direction, and explicit approval questions. Do not generate media.",
	"script":     "Use the approved research and proposal to write script.md. Include voiceover or dialogue, on-screen text, approximate timing, and source notes. Keep claims grounded and make the pacing realistic. Do not generate media.",
	"scene-plan": "Turn the approved script into scene-plan.md. For every scene specify timing, purpose, shot framing, motion, continuity anchors, narration, text overlays, transition intent, and the source asset or generation requirement. Do not generate media.",
	"assets":     "This stage is the only one that creates media, and it has exactly two modes. Choose the mode ONLY from the human input for this stage: treat generation as authorised solely when that input explicitly approves generating media or spending on it. Ambiguity, enthusiasm about the plan, or approval of an earlier stage is NOT authorisation.\nMODE A — not explicitly authorised (the default): audit uploads/ against scene-plan.md and write asset-manifest.md listing every asset each scene needs, each marked `reuse` with its exact existing path, or `needs-generation` with the exact prompt, reference, duration, aspect ratio and consistency controls you would use. Call no media-generation tool and spend nothing. Finish by stating plainly what still needs approval.\nMODE B — explicitly authorised: do the same audit, then actually generate every `needs-generation` asset by calling the registered media tools directly (image_gen, image_edit, generate_video, text_to_speech, generate_music), writing each file inside this step's own folder. Call those tools as tools — never through shell commands or raw HTTP, and never inspect environment variables or credentials to reach them. Never record an asset as generated unless you created the file and confirmed it exists on disk and is non-empty; if a generation call fails, record it as `failed` with the error rather than describing the asset as if it existed.\nIn both modes asset-manifest.md must let the compose stage resolve every scene to a concrete file without re-deriving prompts: for each asset record the scene id, status (`reuse`, `generated`, `needs-generation`, or `failed`), the exact file path when one exists, and for generated files the provider/model, duration, resolution and aspect ratio.",
	"edit":       "Create edit-plan.md from the script, scene plan, and asset manifest. Define the precise sequence, clip in/out choices, transitions, pacing, voice/music mix, captions, graphics, color treatment, and stitching rules. Record any blockers before composition.",
	"compose":    "Assemble the video with ffmpeg from media that already exists. Resolve every input from the exact file paths recorded in asset-manifest.md — do not write generation prompts, do not call media-generation tools, and do not go looking for files elsewhere. Verify each path exists and is readable before using it. If any required asset is missing, `needs-generation`, or `failed`, stop and write render-report.md naming the scenes and assets that are missing and what has to happen next; never substitute a placeholder, reuse an unrelated file, or describe a render you did not produce. Keep intermediate files inside this step folder and create render-report.md describing inputs, commands, versions, duration, resolution, codecs, and the path of every rendered candidate.",
	"delivery":   "Perform final technical and editorial QA on the rendered candidate: decode integrity, duration, frame size, frame rate, audio, loudness/clipping, black or frozen frames, caption safe areas, legibility, continuity, pacing, factual consistency, and obvious visual defects. Repair safe mechanical issues when possible. Write delivery.md with pass/fail evidence, remaining concerns, and the selected final file. Do not publish.",
}

var cinematicPipeline = &Pipeline{
	ID:          "cinematic",
	Name:        "Cinematic video",
	Description: "How an idea moves from a brief to a finished, checked video.",
	WhenToUse:   "Story-led films, brand and product teasers, launch pieces, and anything whose value comes from footage, mood, and pacing rather than explanation.",
	Stages: []PipelineStage{
		{ID: "research", Title: "Research", Summary: "Understand the brief, audience, sources, and open questions.", Output: "research.md"},
		{ID: "proposal", Title: "Creative proposal", Summary: "Choose the story, tone, format, and visual direction.", Output: "proposal.md"},
		{ID: "script", Title: "Script", Summary: "Write the narration, dialogue, timing, and on-screen text.", Output: "script.md"},
		{ID: "scene-plan", Title: "Scene plan", Summary: "Break the script into timed shots and visual moments.", Output: "scene-plan.md"},
		// The only stage that can spend money, so it is the only one gated on approval.
		{ID: "assets", Title: "Assets", Summary: "Identify or create the approved visuals, footage, and audio.", Output: "asset-manifest.md", RequiresApproval: true},
		{ID: "edit", Title: "Edit plan", Summary: "Set the sequence, pacing, transitions, captions, and sound.", Output: "edit-plan.md"},
		{ID: "compose", Title: "Compose", Summary: "Assemble the approved material and render the video.", Output: "render-report.md"},
		{ID: "delivery", Title: "Quality check", Summary: "Review the final video for technical and creative issues.", Output: "delivery.md"},
	},
}

// pipelineRegistry holds every pipeline the product can run. Routing picks one.
var pipelineRegistry = []*Pipeline{cinematicPipeline}

func init() {
	// Attach descriptions by stage id so the definition above stays scannable.
	for _, pipeline := range pipelineRegistry {
		for i := range pipeline.Stages {
			if text := cinematicStageDescriptions[pipeline.Stages[i].ID]; text != "" && pipeline.Stages[i].Description == "" {
				pipeline.Stages[i].Description = text
			}
		}
	}
}

// DefaultPipeline is used until routing selects one explicitly.
func DefaultPipeline() *Pipeline { return cinematicPipeline }

// PipelineByID returns a pipeline by id, falling back to the default.
func PipelineByID(id string) *Pipeline {
	for _, pipeline := range pipelineRegistry {
		if pipeline.ID == id {
			return pipeline
		}
	}
	return DefaultPipeline()
}

// Steps renders the pipeline as the per-run step rows the product stores and
// the UI displays.
func (p *Pipeline) Steps() []WorkflowStep {
	steps := make([]WorkflowStep, 0, len(p.Stages))
	for i, stage := range p.Stages {
		steps = append(steps, WorkflowStep{ID: stage.ID, Title: stage.Title, Summary: stage.Summary, Position: i + 1, Status: "pending"})
	}
	return steps
}

// Outputs lists each stage's artifact in order.
func (p *Pipeline) Outputs() []string {
	outputs := make([]string, 0, len(p.Stages))
	for _, stage := range p.Stages {
		outputs = append(outputs, stage.Output)
	}
	return outputs
}
