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
	"assets":     "This stage is the only one that creates media, and it has exactly two modes. Choose the mode ONLY from the human input for this stage: treat generation as authorised solely when that input explicitly approves generating media or spending on it. Ambiguity, enthusiasm about the plan, or approval of an earlier stage is NOT authorisation.\nMODE A — not explicitly authorised (the default): audit uploads/ against scene-plan.md and write asset-manifest.md listing every asset each scene needs, each marked `reuse` with its exact existing path, or `needs-generation` with the exact description, duration, aspect ratio and consistency controls you would use. Produce nothing yet. Finish by stating plainly what still needs approval.\nMODE B — explicitly authorised: do the same audit, then actually produce every `needs-generation` asset locally with `execute_shell_command` and `ffmpeg` — no paid generation provider is used for this. Solid or gradient backgrounds, drawtext title/caption cards, and sine-tone or silent audio beds are all fair game for a placeholder; match each asset's duration, aspect ratio, and resolution to what the scene plan calls for. Write every file inside this step's own folder, mark it `placeholder: true` in the manifest so nobody downstream mistakes it for finished creative material, and never record an asset as produced unless you created the file and confirmed it exists on disk and is non-empty. If a command fails, record that asset as `failed` with the error rather than describing it as if it existed.\nIn both modes asset-manifest.md must let the compose stage resolve every scene to a concrete file without re-deriving anything: for each asset record the scene id, status (`reuse`, `generated`, `needs-generation`, or `failed`), the exact file path when one exists, and for generated files the duration, resolution, aspect ratio, and whether it is a placeholder.",
	"edit":       "Create edit-plan.md from the script, scene plan, and asset manifest. Define the precise sequence, clip in/out choices, transitions, pacing, voice/music mix, captions, graphics, color treatment, and stitching rules. Record any blockers before composition.",
	"compose":    "Assemble the video with ffmpeg from media that already exists. Resolve every input from the exact file paths recorded in asset-manifest.md — do not write generation prompts, do not call media-generation tools, and do not go looking for files elsewhere. Verify each path exists and is readable before using it. If any required asset is missing, `needs-generation`, or `failed`, stop and write render-report.md naming the scenes and assets that are missing and what has to happen next; never substitute a placeholder of your own or reuse an unrelated file for a *missing* asset. An asset the manifest already marks `placeholder: true` is expected right now and is a normal input, not a reason to stop — assemble with it and record in render-report.md that the candidate is a placeholder assembly, not a finished video. Keep intermediate files inside this step folder and create render-report.md describing inputs, commands, versions, duration, resolution, codecs, and the path of every rendered candidate.",
	"delivery":   "Perform final technical and editorial QA on the rendered candidate: decode integrity, duration, frame size, frame rate, audio, loudness/clipping, black or frozen frames, caption safe areas, legibility, continuity, pacing, factual consistency, and obvious visual defects. If render-report.md says the candidate was assembled from placeholder assets, apply only the technical checks — continuity, legibility, and factual consistency do not apply to a placeholder, and grading it against them is meaningless. Record it as a placeholder-pipeline pass or fail, not a creative verdict on a finished video. Repair safe mechanical issues when possible. Write delivery.md with pass/fail evidence, remaining concerns, and the selected final file. Do not publish.",
}

var cinematicPipeline = &Pipeline{
	ID:          "cinematic",
	Name:        "Cinematic video",
	Description: "How an idea moves from a brief to a finished, checked video.",
	WhenToUse:   "Story-led films, brand and product teasers, launch pieces, and anything whose value comes from footage, mood, and pacing rather than explanation.",
	Stages: []PipelineStage{
		{ID: "research", Title: "Research", Summary: "Understand the brief, audience, sources, and open questions.", Output: "research.md", Skills: []string{"video-creation"}},
		{ID: "proposal", Title: "Creative proposal", Summary: "Choose the story, tone, format, and visual direction.", Output: "proposal.md"},
		{ID: "script", Title: "Script", Summary: "Write the narration, dialogue, timing, and on-screen text.", Output: "script.md"},
		{ID: "scene-plan", Title: "Scene plan", Summary: "Break the script into timed shots and visual moments.", Output: "scene-plan.md"},
		// The only stage that can spend money, so it is the only one gated on approval.
		{ID: "assets", Title: "Assets", Summary: "Identify or create the approved visuals, footage, and audio.", Output: "asset-manifest.md", RequiresApproval: true, Skills: []string{"video-shot-generation"}},
		{ID: "edit", Title: "Edit plan", Summary: "Set the sequence, pacing, transitions, captions, and sound.", Output: "edit-plan.md", Skills: []string{"video-editing"}},
		{ID: "compose", Title: "Compose", Summary: "Assemble the approved material and render the video.", Output: "render-report.md", Skills: []string{"video-editing"}},
		{ID: "delivery", Title: "Quality check", Summary: "Review the final video for technical and creative issues.", Output: "delivery.md", Skills: []string{"video-quality"}},
	},
}

// Stage descriptions for the infographic branch. Its stage ids are prefixed
// because both branches live in one plan.json and step ids must be unique
// there; the cinematic ids stay unprefixed so existing run history keeps
// resolving.
var infographicStageDescriptions = map[string]string{
	"infographic-research": "Study the user's brief and every relevant file in uploads/ for the product being explained. Write infographic-research.md capturing what the product is, who it is for, the specific claims and numbers worth showing, the source for each one, and anything asserted but unverified. Separate verified facts from assumptions — a number that reaches a panel without a source here becomes a false claim on screen. Do not design anything yet.",
	"infographic-concept":  "Use infographic-research.md to choose the piece's angle. Write infographic-concept.md fixing the one message the video must land, the single hero number or claim it is built around, the panel count and rough duration, the aspect ratio, and a concrete visual system: an exact palette (hex values), a type stack that degrades without webfonts, and the shape language. Name what is deliberately left out. Do not write final copy or build panels.",
	"infographic-copy":     "Turn the concept into infographic-copy.md: the exact words for every panel — headline, supporting line, any label or unit, and the closing call to action. Keep a panel's headline short enough to read in the time it is on screen, and carry each number's source note beside it. This is the last stage where wording changes cheaply; after layout, a rewrite means re-rendering.",
	"infographic-layout":   "Turn the approved copy into infographic-layout.md: one entry per panel with its id, on-screen duration, exact text placement, which element is the focal point, the motion intent (hold, push-in, or cross-fade to the next), and the safe-area margins every panel must respect. Confirm the durations sum to the concept's target length. Do not write HTML yet.",
	"infographic-design":   "This stage BUILDS the panels — it is not a design document and there is no later build stage. Nothing downstream can render a panel you did not create here.\nFor every panel in infographic-layout.md, write one .html file inside this step's own folder and render it to a PNG with headless Chrome, following the html-composition skill: exact canvas size, box-sizing, safe-area margins, and a font stack that degrades honestly. Then open each PNG and confirm it is non-trivial in size and actually shows the panel — a blank page still writes a valid file, so a zero exit code and an existing file prove nothing on their own.\ninfographic-design.md is a RECORD of what you built, written last: for each panel its html path, its png path, and the pixel dimensions you verified. Recording a panel whose file does not exist on disk is a failure, not a plan. If a panel genuinely cannot be built, record it as `failed` with the error rather than describing the design you would have made.",
	"infographic-render":   "Assemble the rendered panels into the final video with ffmpeg, using the exact PNG paths recorded in infographic-design.md. Give each panel the duration infographic-layout.md specifies, apply only the motion that layout calls for, and keep every intermediate inside this step's folder. Do not redesign a panel here and do not substitute a placeholder for a missing one — if a PNG is missing or unreadable, stop and write infographic-render-report.md naming exactly which panel and what has to happen next. Otherwise write infographic-render-report.md with the inputs, the ffmpeg commands, and the path, duration, resolution and codecs of the rendered file.",
	"infographic-check":    "Perform final QA on the rendered file: decode integrity, duration against the layout's target, frame size, frame rate, and audio if present. Then check what only matters for this format — every number on screen traces to a source in infographic-research.md, no text is clipped or inside the reserved top/bottom bands, and each panel is on screen long enough to actually read. Write infographic-delivery.md with pass/fail evidence per check, the exact file validated, and any remaining concern. Do not publish.",
}

var infographicPipeline = &Pipeline{
	ID:          "infographic",
	Name:        "Product infographic",
	Description: "How a product's facts become a laid-out, readable explainer video.",
	WhenToUse:   "Product explainers, feature breakdowns, stat and data pieces, comparison or pricing videos — anything whose value comes from typography, numbers and layout rather than footage and mood.",
	Stages: []PipelineStage{
		{ID: "infographic-research", Title: "Research", Summary: "Gather the product's facts, claims, numbers, and sources.", Output: "infographic-research.md", Skills: []string{"video-creation"}},
		{ID: "infographic-concept", Title: "Concept", Summary: "Fix the angle, hero number, palette, and type system.", Output: "infographic-concept.md"},
		{ID: "infographic-copy", Title: "Copy", Summary: "Write the exact words for every panel.", Output: "infographic-copy.md"},
		{ID: "infographic-layout", Title: "Layout", Summary: "Break the copy into timed panels with placement and motion.", Output: "infographic-layout.md"},
		{ID: "infographic-design", Title: "Build panels", Summary: "Build each panel in HTML/CSS and render it to an image.", Output: "infographic-design.md", Skills: []string{"html-composition"}},
		{ID: "infographic-render", Title: "Render", Summary: "Assemble the panels into the finished video.", Output: "infographic-render-report.md", Skills: []string{"html-composition"}},
		{ID: "infographic-check", Title: "Quality check", Summary: "Verify the render, the claims, and the readability.", Output: "infographic-delivery.md", Skills: []string{"video-quality"}},
	},
}

// Stage skills are deliberately narrow: a stage agent gets the one skill for
// its craft, while the main project chat agent carries all of them so it can
// route and answer anything. Names must be flat and hyphenated — the builtin
// registry rejects slashes, so layered names wait on nested discovery.

// pipelineRegistry holds every pipeline the product can run. Routing picks one.
var pipelineRegistry = []*Pipeline{cinematicPipeline, infographicPipeline}

func init() {
	// Attach descriptions by stage id so the definitions above stay scannable.
	for _, pipeline := range pipelineRegistry {
		for i := range pipeline.Stages {
			if pipeline.Stages[i].Description != "" {
				continue
			}
			id := pipeline.Stages[i].ID
			if text := cinematicStageDescriptions[id]; text != "" {
				pipeline.Stages[i].Description = text
				continue
			}
			if text := infographicStageDescriptions[id]; text != "" {
				pipeline.Stages[i].Description = text
			}
		}
	}
}

// DefaultPipeline is the branch a run takes when nothing selects one.
func DefaultPipeline() *Pipeline { return cinematicPipeline }

// AllPipelineSteps returns every stage across every pipeline, numbered
// continuously. A run is seeded with all of them because which branch it takes
// is decided by the routing step while the run executes, not when the run row
// is created — and a status written for a step with no row is silently dropped,
// since SetWorkflowStep only updates.
func AllPipelineSteps() []WorkflowStep {
	steps := make([]WorkflowStep, 0, 16)
	position := 1
	for _, pipeline := range pipelineRegistry {
		for _, stage := range pipeline.Stages {
			steps = append(steps, WorkflowStep{ID: stage.ID, Title: stage.Title, Summary: stage.Summary, Position: position, Status: "pending"})
			position++
		}
	}
	return steps
}

// pipelineForStage reports which pipeline owns a stage id.
func pipelineForStage(stageID string) *Pipeline {
	for _, pipeline := range pipelineRegistry {
		for _, stage := range pipeline.Stages {
			if stage.ID == stageID {
				return pipeline
			}
		}
	}
	return nil
}

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
