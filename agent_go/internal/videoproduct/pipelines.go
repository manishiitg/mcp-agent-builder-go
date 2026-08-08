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
	// Artifacts are additional files the stage must leave on disk, enforced by
	// its validation schema. A stage whose only required output is its own
	// report can satisfy validation by describing work it never did — which is
	// exactly how a build stage shipped design notes and no panels.
	Artifacts []string
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
	"delivery":   qualityReviewDescription("render-report.md", "delivery.md"),
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
		{ID: "delivery", Title: "Quality check", Summary: "Review the final video for technical and creative issues.", Output: "delivery.md", Artifacts: []string{"quality-report.json", "qa-contact-sheet.jpg"}, Skills: []string{"video-quality"}},
	},
}

// Stage descriptions for the infographic branch. Its stage ids are prefixed
// because both branches live in one plan.json and step ids must be unique
// there; the cinematic ids stay unprefixed so existing run history keeps
// resolving.
var infographicStageDescriptions = map[string]string{
	"infographic-research":             "Own the brief before production. Read the current request, recent conversation, and every relevant file in uploads/. Follow the product-infographic skill's adaptive interview rules: do not ask again for settled information, and if the user said to build immediately, choose and record sensible non-factual defaults. Select exactly one primary HyperFrames route: product-launch-video when real UI/product evidence is central, faceless-explainer when source information should become invented explanatory visuals, motion-graphics for one short visual beat, or general-video only when no specialist fits. Write BRIEF.md with the message, audience, destination, duration, aspect ratio, authoritative inputs, exact claims and sources, route, visual/sound direction, review mode, exclusions, assumptions, and blockers. Do not design or build yet.",
	"infographic-concept":              "Turn BRIEF.md into STORYBOARD.md. Establish a teaching or product-proof spine rather than translating each paragraph into a scene. The opening must earn attention, the main idea must be clear by scene two, every body scene must advance one mechanism/feature/step/example/proof, and the ending must land one remembered sentence or action. For each scene record purpose, timing, exact evidence, visual form, on-screen copy intent, narration intent, transition purpose, and the HyperFrames composition or reusable system it will need. Confirm total duration and flag unsupported claims. Do not author HTML yet.",
	"infographic-copy":                 "Use BRIEF.md and STORYBOARD.md to write SCRIPT.md. Even for a silent piece, lock the exact on-screen words scene by scene and explicitly state that narration is not applicable. When narration exists, include timed narration, captions, pronunciation notes, music/SFX intent, and the relationship between spoken and visible information. Preserve any verbatim wording the brief locks. Keep every number and product claim tied to its source. Do not build the composition yet.",
	"infographic-layout":               "Use BRIEF.md, STORYBOARD.md, and SCRIPT.md to write frame.md: the production's visual and motion system. Define the exact palette, type stack, canvas/safe areas, spacing/grid, shape and image language, product-UI treatment, chart/diagram grammar, scene transition grammar, motion intensity, caption treatment, audio rules, and reusable variables such as title/logo/colors/prices. Use a chart only to prove a claim, a diagram to reveal a mechanism, and motion to show change. Record which installed HyperFrames technical skills and optional registry items the build actually needs. Do not write HTML yet.",
	"infographic-creative-critique":    "Act as the independent pre-production critic for this high-quality workflow. Read BRIEF.md, STORYBOARD.md, SCRIPT.md, and frame.md; do not build the composition. Score the proposed work against the production rubric: audience/message clarity by scene two, evidence and claim traceability, narrative progression, density and playback readability, visual hierarchy, motion purpose, visual-system coherence, accessibility/caption intent, and a memorable ending. Write creative-review.md and creative-scorecard.json. The scorecard must contain schema_version 1, verdict (pass, revise, or blocked), category scores and evidence, blocking findings, targeted repair instructions, and the exact acceptance criteria the builder must prove. Be demanding: a vague compliment is not a review. A blocked verdict must name the missing decision or evidence and tell the orchestrator to pause for the user rather than inventing facts. A revise verdict must be specific enough for the builder to repair without reinterpreting the brief. This review is a gate: the downstream build must treat every finding as mandatory work, not optional advice.",
	"infographic-design":               "Build one native, editable HyperFrames project in this step folder from BRIEF.md, STORYBOARD.md, SCRIPT.md, and frame.md. This is not a panel-to-PNG or FFmpeg slideshow stage. Read the selected route and the installed hyperframes-core, hyperframes-creative, hyperframes-animation, media-use, registry, and keyframes skills as applicable. Use the managed HyperFrames CLI non-interactively and do not reinstall skills. Create index.html, hyperframes.json, compositions/, and assets/ using seek-safe deterministic timelines; copy or derive working assets without modifying uploads/. Run the CLI's structural lint during authoring and create representative snapshots to inspect the opening, main proof, dense text/data moments, transitions, and ending. Fix blank, clipped, unreadable, non-deterministic, or structurally invalid results. Package the complete source project as hyperframes-project.tgz so the next isolated stage receives compositions and assets as one immutable handoff. Write build-report.md last with the exact project root, composition IDs, source files, assets, selected route/skills, snapshots inspected, lint results, archive path, remaining blockers, and commands used. A report or archive without real source files is failure.",
	"infographic-composition-critique": "Run the composition critique-and-refinement gate. Unpack hyperframes-project.tgz and read the creative-review.md/creative-scorecard.json together with the brief, storyboard, script, frame system, and representative snapshots. You are the workflow's builder-and-critic loop: first inspect the composition against every acceptance criterion, then repair the editable source, lint/check it, create fresh snapshots, and inspect again. Make up to two bounded repair passes; do not merely write a review of known defects. Focus on the opening, scene-two message clarity, proof scenes, dense typography/data, transitions, and ending. Preserve verified claims and never replace the native HyperFrames composition with a static-panel or FFmpeg workaround. Write composition-critique.md and composition-scorecard.json with the initial findings, each repair pass, evidence, and final verdict (pass, revise, or blocked). Repack the corrected editable source as hyperframes-project-reviewed.tgz. Only a pass verdict may hand off the reviewed archive. If the work cannot pass because the brief is ambiguous, evidence is missing, or a defect cannot be safely repaired, return a failed step so the orchestrator stops and asks for direction; do not let rendering continue on a known weak composition.",
	"infographic-render":               "Unpack hyperframes-project-reviewed.tgz into this step folder and validate that exact reviewed HyperFrames project; do not replace it with a new composition or assemble static panels with FFmpeg. Read hyperframes-cli and hyperframes-keyframes. Run the current CLI doctor checks needed by this production, then lint and check; use keyframe diagnostics and additional snapshots where animation or dense layouts need them. Fix failures in the unpacked editable source and repeat the affected checks before rendering. Render a playable final.mp4 in this step folder, then verify it with ffprobe/full decode and measure duration, dimensions, frame rate, streams, and codecs. Repack the final corrected source as hyperframes-project-final.tgz. Write render-report.md with the exact source project path, output path, CLI version, validation results, snapshots/key moments, render command, measurements, corrected-source archive, and whether any placeholder remains. A successful command without final.mp4, the corrected source archive, and passing checks is failure.",
	"infographic-render-critique":      "Run the final render critique-and-refinement gate. Inspect final.mp4, render-report.md, BRIEF.md, STORYBOARD.md, SCRIPT.md, frame.md, and the reviewed source archive. Act as a demanding delivery critic before the workflow can reach QA: use a contact sheet and playback checks to assess legibility at speed, framing/safe areas, hierarchy, continuity, purposeful motion, pacing, claim/product accuracy, captions/audio where applicable, and the promised ending. If a material issue appears, unpack hyperframes-project-final.tgz, repair the editable source, rerun the relevant HyperFrames checks, render a replacement final-reviewed.mp4, and repeat inspection. Perform at most two bounded repair passes and keep all evidence. Write render-critique.md and render-scorecard.json with initial findings, repairs, final verdict (pass, revise, or blocked), and the exact candidate path. Package the corrected source as hyperframes-project-reviewed-final.tgz. Only a pass may hand off final-reviewed.mp4. If a problem remains or requires a user decision, fail the step so the orchestrator stops rather than passing an unreviewed render to QA.",
	"infographic-check":                qualityReviewDescription("the final-reviewed.mp4 named by render-critique.md", "infographic-delivery.md") + " Also compare the result with BRIEF.md, STORYBOARD.md, SCRIPT.md, frame.md, creative-review.md, composition-critique.md, render-critique.md, and the HyperFrames snapshots. Verify that the main idea is clear by scene two, every product state is realistic, every number and claim traces to its source, important UI/text is readable at playback size, motion teaches rather than decorates, and the ending lands the approved message or action.",
}

var infographicPipeline = &Pipeline{
	ID:          "infographic",
	Name:        "Product explainer / infographic",
	Description: "A high-quality HyperFrames product explainer with independent critique and bounded refinement gates.",
	WhenToUse:   "Product explainers, feature breakdowns, stat and data pieces, comparison or pricing videos — anything whose value comes from typography, numbers and layout rather than footage and mood.",
	Stages: []PipelineStage{
		{ID: "infographic-research", Title: "Brief and evidence", Summary: "Confirm the message, authoritative evidence, format, and HyperFrames route.", Output: "BRIEF.md", Skills: []string{"product-infographic", "video-creation", "hyperframes", "product-launch-video", "faceless-explainer", "motion-graphics", "general-video"}},
		{ID: "infographic-concept", Title: "Storyboard", Summary: "Shape the teaching or product-proof sequence scene by scene.", Output: "STORYBOARD.md", Skills: []string{"product-infographic", "hyperframes-creative"}},
		{ID: "infographic-copy", Title: "Script and copy", Summary: "Lock the exact on-screen words, narration, captions, and evidence.", Output: "SCRIPT.md", Skills: []string{"product-infographic", "hyperframes-creative", "media-use"}},
		{ID: "infographic-layout", Title: "Visual system", Summary: "Define the frame, typography, layout, motion, variables, and sound grammar.", Output: "frame.md", Skills: []string{"product-infographic", "hyperframes-creative", "hyperframes-animation"}},
		{ID: "infographic-creative-critique", Title: "Creative critique", Summary: "Independently score the narrative and visual system before anything is built.", Output: "creative-review.md", Artifacts: []string{"creative-scorecard.json"}, Skills: []string{"product-infographic", "hyperframes-quality", "hyperframes-creative", "hyperframes-animation"}},
		{ID: "infographic-design", Title: "Build composition", Summary: "Build and inspect one editable native HyperFrames project.", Output: "build-report.md", Artifacts: []string{"index.html", "hyperframes.json", "hyperframes-project.tgz"}, Skills: []string{"product-infographic", "hyperframes", "hyperframes-core", "hyperframes-creative", "hyperframes-animation", "media-use", "hyperframes-registry", "hyperframes-keyframes"}},
		{ID: "infographic-composition-critique", Title: "Composition critique and refine", Summary: "Inspect, repair, and re-check the editable composition before render.", Output: "composition-critique.md", Artifacts: []string{"composition-scorecard.json", "hyperframes-project-reviewed.tgz"}, Skills: []string{"product-infographic", "hyperframes-quality", "hyperframes-cli", "hyperframes-core", "hyperframes-creative", "hyperframes-animation", "hyperframes-keyframes"}},
		{ID: "infographic-render", Title: "Validate and render", Summary: "Run HyperFrames checks, inspect key moments, and render the candidate video.", Output: "render-report.md", Artifacts: []string{"final.mp4", "hyperframes-project-final.tgz"}, Skills: []string{"product-infographic", "hyperframes-cli", "hyperframes-keyframes"}},
		{ID: "infographic-render-critique", Title: "Render critique and refine", Summary: "Critique playback, repair the source if needed, and approve the delivery candidate.", Output: "render-critique.md", Artifacts: []string{"render-scorecard.json", "final-reviewed.mp4", "hyperframes-project-reviewed-final.tgz"}, Skills: []string{"product-infographic", "hyperframes-quality", "hyperframes-cli", "hyperframes-keyframes", "video-quality"}},
		{ID: "infographic-check", Title: "Quality check", Summary: "Verify the render, the claims, and the readability.", Output: "infographic-delivery.md", Artifacts: []string{"quality-report.json", "qa-contact-sheet.jpg"}, Skills: []string{"video-quality"}},
	},
}

// Quality assurance is also independently routable. This lets a creator ask to
// inspect or re-check an existing render without rerunning its production
// workflow. Creation pipelines still end with their own mandatory QA stage.
var qualityPipeline = &Pipeline{
	ID:          "quality",
	Name:        "Video quality assurance",
	Description: "How a finished video is checked before it is shown or delivered.",
	WhenToUse:   "Reviewing an existing render, diagnosing a video problem, or re-checking a revised export without rebuilding the production.",
	Stages: []PipelineStage{
		{
			ID:          "qa-review",
			Title:       "Quality assurance",
			Summary:     "Inspect the exact video, record evidence, and decide whether it is ready.",
			Description: qualityReviewDescription("the exact candidate named in the current request", "qa-delivery.md"),
			Output:      "qa-delivery.md",
			Artifacts:   []string{"quality-report.json", "qa-contact-sheet.jpg"},
			Skills:      []string{"video-quality"},
		},
	},
}

func qualityReviewDescription(candidateSource, markdownOutput string) string {
	return "Perform mandatory post-render QA on the exact candidate identified by " + candidateSource + ". " +
		"First resolve and record one project-relative candidate path; never validate one file and select another. " +
		"Run technical checks for file existence, full decode, duration, dimensions, frame rate, streams, loudness/clipping, black frames, frozen sections, and truncated ending. " +
		"Create qa-contact-sheet.jpg from at least four representative timestamps including the opening and ending, then open and inspect it for broken layouts, missing media, unsafe or unreadable text, visual defects, continuity, pacing, and promise preservation. " +
		"Check audio, captions, factual claims, names, prices, and calls to action when they are expected; mark a category not_applicable only when the brief genuinely does not require it. " +
		"If the candidate uses placeholder assets, do the technical gate and label the result placeholder-pass rather than claiming the creative video is finished. Repair safe mechanical issues and re-run the affected checks. " +
		"Write " + markdownOutput + " for people and quality-report.json for the product. The JSON must contain schema_version 1, candidate_path, contact_sheet_path, verdict (pass, placeholder-pass, revise, or fail), ready_to_present, checks named technical/visual/audio/content/captions/promise with status and evidence, at least four sampled_frames with timestamp_seconds and path, issues, and recommended_action. " +
		"Only pass or placeholder-pass may set ready_to_present true, every required check must be pass or not_applicable, and recommended_action must be present. Do not publish or call the video complete when any required check is unverified."
}

// Stage skills are deliberately narrow: a stage agent gets the one skill for
// its craft, while the main project chat agent carries all of them so it can
// route and answer anything. Names must be flat and hyphenated — the builtin
// registry rejects slashes, so layered names wait on nested discovery.

// pipelineRegistry holds every pipeline the product can run. Routing picks one.
var pipelineRegistry = []*Pipeline{infographicPipeline, qualityPipeline}

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
func DefaultPipeline() *Pipeline { return infographicPipeline }

// PipelineByName resolves persisted run labels back to their pipeline. Keep the
// former infographic display name as an alias so existing project history still
// drives the correct creator-facing Workflow panel after the rename.
func PipelineByName(name string) *Pipeline {
	for _, pipeline := range pipelineRegistry {
		if name == pipeline.Name || name == pipeline.ID {
			return pipeline
		}
	}
	if name == "Product infographic" {
		return infographicPipeline
	}
	return nil
}

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
