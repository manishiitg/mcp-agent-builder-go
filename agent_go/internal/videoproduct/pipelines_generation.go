package videoproduct

// The two AI-generation pipelines. They are separate from the infographic
// route because they produce footage rather than composed typography, and
// separate from each other because a 12-minute narrated piece and a 30-second
// social cut are different productions, not one production at two lengths:
// long-form needs chapters, retention management, character continuity across
// dozens of clips, and narration that the visuals are cut to. Short-form needs
// one arc landed in the first seconds and nothing else.
//
// Both spend real money per generation call, which the infographic route never
// does — its assets stage makes ffmpeg placeholders. That is why every stage
// here that can spend is gated on explicit authorization in its own human
// input, using the same rule the assets stage established.

// generationSpendGate is the authorization paragraph every paid stage carries.
// Kept in one place so the stages that spend cannot drift apart on what counts
// as approval; a test pins exactly which stages carry it.
//
// The rule is deliberately strict about WHERE approval comes from: this
// product bills the user per generation call, and an agent that reads
// enthusiasm about a storyboard as permission to spend is the failure mode
// worth designing against.
func generationSpendGate(what string) string {
	return "This stage can spend the user's money, so it has exactly two modes and you choose between them ONLY from the human input for THIS stage. " +
		"Treat generation as authorised solely when that input explicitly approves generating " + what + " or spending on it. " +
		"Approval of an earlier stage, enthusiasm about the plan, or silence is NOT authorisation.\n" +
		"MODE A — not explicitly authorised (the default): plan the work completely and produce nothing. Write the artifact this stage owes, with every intended call fully specified — resolved model id, provider, exact input, and the estimated number of paid calls — so the user can approve a concrete, costed plan rather than an idea. Finish by stating plainly what it will cost and what still needs approval.\n" +
		"MODE B — explicitly authorised: do the same planning, then actually make the calls, staying within any cost ceiling or call count this pipeline's brief recorded. Never record an asset as produced unless you created the file and confirmed it exists on disk and is non-empty; a call that fails is recorded as `failed` with its error, never described as if it succeeded. If you reach the approved ceiling before finishing, stop and report what remains rather than continuing to spend."
}

var longformStageDescriptions = map[string]string{
	"longform-brief": "Own the brief before any production work. Read the request, the recent conversation, and every relevant file in uploads/. " +
		"This pipeline is for genuinely long-form video — roughly 8 minutes or more — so confirm the target duration explicitly rather than assuming; if the user actually wants something short, say so and stop rather than running a long-form production on a short brief. " +
		"Follow video-creation's request-understanding rules, and resolve the production-level unknowns video-model-selection lists before the first paid call, because every one of them is expensive to unwind later: provider or model preference, whether any character/presenter/product must recognisably recur across shots, the cost ceiling or maximum number of paid generation calls, and how many re-prompts a disappointing shot gets before you stop and ask. " +
		"Use video-storytelling to decide the narrative shape at this length — which arc, which hook type, roughly how many chapters. " +
		"Write longform-brief.md with the message, audience, destination, target duration, aspect ratio, tone, narrative shape, every resolved production decision above, the exact claims and their sources, assumptions, and blockers. Do not generate media and do not write the script yet.",

	"longform-script": "Turn longform-brief.md into longform-script.md, the piece's narrative spine. Follow video-storytelling: one arc across the whole piece, chapters of roughly 2-4 minutes each carrying their own smaller arc, beats connected with `but`/`therefore` rather than `and then`, and the hook and stakes fully landed inside the first 30 seconds. " +
		"Place the retention work deliberately: the first major payoff before 2:00, a re-hook every 3-4 minutes after the two-minute mark, an open loop planted early and paid off late, and pattern interrupts at the cadence the skill gives. Reserve the last ~20 seconds for the close and put no essential content there. " +
		"Write the narration verbatim — this exact text becomes the voiceover and drives every downstream duration, so it must be the words to be spoken, not a summary of them. Mark chapter boundaries, and for each beat state what the viewer should see, what is said, and which claim or source it rests on. " +
		"Name every character, presenter, or product that recurs across more than one beat; the next stage defines them. Do not generate media.",

	"longform-characters": "Define every recurring character, presenter, or product BEFORE any shot of it exists. This is the stage that prevents the most common viewer-visible defect in AI-generated video — a face, outfit, or product appearance drifting between shots — and it cannot be done retroactively. " +
		"Read longform-brief.md and longform-script.md and follow video-cinematography's character rules. For each recurring subject produce both deliverables: a written spec at characters/<name>.md carrying 3-6 specific disambiguating visual attributes as one exact phrase to be reused verbatim in every later prompt, and a generated reference image at characters/<name>.png. Keep both inside this step's own folder. " +
		"Choose one model and one provider per character and commit the character's whole arc to it — record that choice, because later stages are bound by it and a mid-arc switch needs the user's explicit sign-off. Use video-model-selection to pick, and fal-ai or google-ai to generate; both skills show how to send and reuse a local reference image. " +
		"Write longform-characters.md listing every subject, its spec path, its reference image path, its committed model and provider, and any subject the brief mentions that deliberately does NOT recur. If the brief needs no recurring subject, say so explicitly in longform-characters.md and generate nothing. " +
		generationSpendGate("character reference images"),

	"longform-narration": "Generate the narration, because at this length the voiceover is the spine the visuals are cut to — deriving shot durations from an estimate instead of from measured audio is the most expensive mistake in this pipeline, forcing either regenerated clips or a rewritten script. " +
		"Read longform-script.md and follow video-editing's narration guidance. Generate the voiceover in per-beat or per-chapter segments, never as one long pass, so a single bad beat can be regenerated later without re-timing everything after it -- both providers' own guidance independently recommends chunking for the same reason. " +
		"Use whichever provider longform-brief.md committed to: `fal-ai` and `google-ai` both generate speech, so a production with only one provider's key configured is not blocked here. If the brief settled on a provider whose credential is missing, say so and stop rather than silently switching providers mid-production. " +
		"Then MEASURE what you actually got: run ffprobe on every segment and record its real duration. Do not record an estimate from the word count. " +
		"Write longform-narration.md listing each segment's beat id, chapter, script text, file path, and measured duration in seconds, plus the measured total. The next stage derives the entire shot plan from these numbers, so an unmeasured or missing segment is a blocker, not a rounding detail. " +
		generationSpendGate("narration or voiceover audio"),

	"longform-shotlist": "Turn the measured narration into the shot plan. Read longform-brief.md, longform-script.md, longform-characters.md, and longform-narration.md. " +
		"Derive every duration from longform-narration.md's MEASURED numbers — a chapter whose narration measures 94 seconds needs 94 seconds of visuals, and how many clips that is falls out of the chosen model's per-call duration, not the other way round. Do the arithmetic explicitly and show it. " +
		"For each shot record: its beat and chapter, its duration, what the shot must prove, the resolved model id and provider, and the full generation prompt built with video-cinematography's five-aspect formula — subject, subject motion, scene, spatial framing, camera — using precise camera vocabulary rather than mood adjectives. Any shot containing a recurring subject MUST use that subject's committed model and provider from longform-characters.md and repeat its spec phrase verbatim; where that model's audio support is wrong for the shot, plan it silent and add audio at assembly rather than switching models. " +
		"Where consecutive clips must flow together, overlap their prompts at the seam so clip N's last described moment matches clip N+1's first. Choose each shot's model with video-model-selection and confirm the model id against the provider's own live reference — never invent one. " +
		"Write longform-shotlist.md with the total paid call count and estimated cost stated up front, since the next stage asks the user to approve exactly that. Generate nothing in this stage.",

	"longform-generate": "Produce the shots. Read longform-shotlist.md, longform-characters.md, and longform-narration.md, and generate strictly what longform-shotlist.md specifies — this stage executes a plan the user approved, it does not re-plan. Use fal-ai or google-ai per the model recorded for each shot. " +
		"Condition every shot containing a recurring subject on that subject's reference image from longform-characters.md, using the committed model and provider and the exact spec phrase; upload each reference once and reuse the returned handle across shots rather than re-uploading per call. " +
		"Respect the re-prompt budget recorded in longform-brief.md: when a shot misses, re-prompt with more specific technical direction rather than a stronger adjective, and stop at the budget to ask rather than spending indefinitely on one stubborn shot. " +
		"Write longform-generation.md recording, per shot: status (`generated`, `failed`, or `skipped`), the exact local file path when one exists, the resolved model id and provider, the prompt actually sent, the seed if any, and the measured duration of what came back. A shot recorded as generated without a file on disk is a failure, not a bookkeeping detail. " +
		generationSpendGate("video, image, or audio clips"),

	"longform-assemble": "Assemble the finished video from media that already exists. Read longform-shotlist.md, longform-narration.md, and longform-generation.md, and resolve every input from the exact paths recorded there — do not write generation prompts, do not call any generation provider, and do not look for files elsewhere. If a required shot is `failed` or missing, stop and write longform-render.md naming what is missing; never substitute an unrelated clip. " +
		"Follow video-editing throughout. Cut the visuals to the measured narration rather than the reverse. Verify every clip's specs with ffprobe before concatenating and normalise the minority that does not match, since one differently-specced clip breaks a stream-copy concat for the whole batch. Force constant frame rate where a source is variable, or audio drift accumulates audibly across a piece this long. " +
		"Choose transitions by video-editing's rules, including its two overrides for independently-generated footage. Keep one continuous music bed across the whole runtime rather than restarting a cue per chapter, hit the stated per-stem levels and the -14 LUFS / -1 dBTP delivery target, and keep per-chapter loudness within about 2 LUFS so no chapter reads louder than its neighbours. Put exact text, titles, and captions in a deterministic overlay layer, never baked into generated footage. " +
		"Render longform-final.mp4 in this step folder, verify it by full decode with ffprobe, and write longform-render.md with every input path, the commands used, and the measured duration, dimensions, frame rate, codecs, and loudness of the result.",

	"longform-check": qualityReviewDescription("longform-final.mp4 as named by longform-render.md", "longform-delivery.md") +
		" This is a long-form piece assembled from many independently-generated clips, so weight the review accordingly: produce one contact sheet per chapter rather than one unreadable sheet for the whole runtime, and cover every edit boundary across the set rather than sampling a few. " +
		"Check each recurring subject against its reference image in longform-characters.md rather than from memory — drifting identity is this pipeline's characteristic defect and it is invisible unless compared directly. " +
		"Also verify the narrative promises longform-script.md made: the hook lands inside the first 30 seconds, each chapter pays off, the open loop planted early is actually resolved, and the ending delivers the message the brief asked for.",
}

var longformPipeline = &Pipeline{
	ID:          "longform",
	Name:        "Long-form AI video",
	Description: "A genuinely long-form (8+ minute) narrated piece built from AI-generated footage, with character continuity and narration-driven timing.",
	WhenToUse:   "Long-form narrative or explainer video — roughly 8 minutes or more — where the footage must be generated rather than composed from uploads, especially anything with a recurring character, presenter, or product.",
	Stages: []PipelineStage{
		{ID: "longform-brief", Title: "Brief and narrative shape", Summary: "Confirm the message, duration, cost ceiling, and consistency requirements before anything is generated.", Output: "longform-brief.md", Skills: []string{"video-creation", "video-storytelling", "video-model-selection"}},
		{ID: "longform-script", Title: "Script and chapters", Summary: "Write the verbatim narration, chapter structure, and retention beats.", Output: "longform-script.md", Skills: []string{"video-storytelling", "video-creation"}},
		{ID: "longform-characters", Title: "Define characters", Summary: "Lock every recurring subject's spec and reference image before any shot exists.", Output: "longform-characters.md", Skills: []string{"video-cinematography", "video-model-selection", "fal-ai", "google-ai"}},
		{ID: "longform-narration", Title: "Generate narration", Summary: "Produce the voiceover in segments and measure its real duration.", Output: "longform-narration.md", Skills: []string{"video-editing", "fal-ai", "google-ai", "video-storytelling"}},
		{ID: "longform-shotlist", Title: "Shot list and prompts", Summary: "Derive every shot and its prompt from the measured narration, and cost the run.", Output: "longform-shotlist.md", Skills: []string{"video-cinematography", "video-model-selection", "video-editing"}},
		{ID: "longform-generate", Title: "Generate shots", Summary: "Produce the approved shots, conditioned on each character's reference.", Output: "longform-generation.md", Skills: []string{"fal-ai", "google-ai", "video-cinematography", "video-model-selection"}},
		{ID: "longform-assemble", Title: "Assemble and mix", Summary: "Cut the visuals to the narration, stitch, mix, and render the candidate.", Output: "longform-render.md", Artifacts: []string{"longform-final.mp4"}, Skills: []string{"video-editing"}},
		{ID: "longform-check", Title: "Quality check", Summary: "Verify the render, the character continuity, and the narrative promises.", Output: "longform-delivery.md", Artifacts: []string{"quality-report.json", "qa-contact-sheet.jpg"}, Skills: []string{"video-quality", "video-cinematography"}},
	},
}

var shortformStageDescriptions = map[string]string{
	"shortform-brief": "Own the brief for a short social cut. Read the request, the recent conversation, and every relevant file in uploads/. " +
		"This pipeline is for short-form video — seconds to a couple of minutes. If the brief actually calls for something genuinely long-form, say so and stop rather than compressing a long-form idea into a shape that cannot hold it. " +
		"Follow video-creation's request-understanding rules and resolve the same pre-generation unknowns video-model-selection lists — provider or model preference, whether any subject must recur across shots, the cost ceiling, and the re-prompt budget — because they are just as expensive to unwind here, only faster. " +
		"Confirm the destination platform and aspect ratio explicitly: a vertical cut and a landscape cut are different productions, and safe areas differ per platform. " +
		"Write shortform-brief.md with the message, audience, platform, duration, aspect ratio, tone, the resolved production decisions, exact claims and sources, assumptions, and blockers. Do not generate media.",

	"shortform-script": "Turn shortform-brief.md into shortform-script.md. At this length there is one arc and no chapters: follow video-storytelling's short shape — hook, then value or proof, then the action — and land the hook in the opening seconds, not after a build. Every second must earn the next one. " +
		"Write the narration or on-screen copy verbatim, since it drives the timing of everything downstream, and mark for each beat what the viewer sees and what is said. Keep the piece to the brief's duration; if the idea does not fit, cut the idea rather than rushing the delivery. " +
		"Name any subject that recurs across more than one shot. Do not generate media.",

	"shortform-shotlist": "Turn shortform-script.md into shortform-shotlist.md. For each shot record its beat, duration, what it must prove, the resolved model id and provider, and the full generation prompt built with video-cinematography's five-aspect formula — subject, subject motion, scene, spatial framing, camera — in precise camera vocabulary rather than mood adjectives. " +
		"Do the duration arithmetic explicitly against the chosen model's per-call limit, and confirm every model id against the provider's own live reference rather than inventing one. Where a subject recurs across shots, follow video-cinematography's consistency rules: define it and its reference image first, keep its shots on one model and provider, and repeat its spec phrase verbatim. " +
		"Reserve exact text, prices, logos, and captions for the overlay layer at assembly rather than baking them into generated footage. State the total paid call count and estimated cost up front, since the next stage asks the user to approve exactly that. Generate nothing in this stage.",

	"shortform-generate": "Produce the shots exactly as shortform-shotlist.md specifies, using fal-ai or google-ai per the model recorded for each shot. This stage executes an approved plan; it does not re-plan. " +
		"Where a subject recurs, generate its reference image first and condition every later shot of it on that same reference, keeping those shots on one model and provider. Respect the re-prompt budget from shortform-brief.md rather than spending indefinitely on one shot. " +
		"Write shortform-generation.md recording, per shot: status (`generated`, `failed`, or `skipped`), the exact local file path when one exists, the resolved model id and provider, the prompt actually sent, and the measured duration of what came back. A shot recorded as generated without a file on disk is a failure. " +
		generationSpendGate("video, image, or audio clips"),

	"shortform-assemble": "Assemble the cut from media that already exists. Read shortform-shotlist.md and shortform-generation.md and resolve every input from the exact recorded paths — do not generate anything here. If a required shot is `failed` or missing, stop and write shortform-render.md naming it rather than substituting an unrelated clip. " +
		"Follow video-editing. Normalise every clip's specs with ffprobe before concatenating. Cut tight: at this length hold shots only as long as they earn, and use video-editing's transition rules including its bias toward hard cuts between independently-generated clips. " +
		"Respect the platform's safe areas from shortform-brief.md so captions and essential subjects survive platform UI overlays, and put exact text in a deterministic overlay layer rather than baking it into footage. Mix to the stated per-stem levels and delivery target. " +
		"Render shortform-final.mp4 in this step folder, verify it by full decode with ffprobe, and write shortform-render.md with every input path, the commands used, and the measured duration, dimensions, frame rate, codecs, and loudness of the result.",

	"shortform-check": qualityReviewDescription("shortform-final.mp4 as named by shortform-render.md", "shortform-delivery.md") +
		" Weight the review for short-form: confirm the hook actually lands in the opening seconds, that captions and key subjects sit inside the destination platform's safe areas at the target aspect ratio, and that the piece reads clearly at phone size and at speed. " +
		"Where a subject recurs across shots, check it against its reference rather than from memory.",
}

var shortformPipeline = &Pipeline{
	ID:          "shortform",
	Name:        "Short-form AI video",
	Description: "A short social cut built from AI-generated footage, shaped for one arc and a destination platform's safe areas.",
	WhenToUse:   "Short social video — reels, shorts, ads, teasers, typically under two minutes — where the footage must be generated rather than composed from uploads.",
	Stages: []PipelineStage{
		{ID: "shortform-brief", Title: "Brief and platform", Summary: "Confirm the message, platform, duration, and cost ceiling before anything is generated.", Output: "shortform-brief.md", Skills: []string{"video-creation", "video-storytelling", "video-model-selection"}},
		{ID: "shortform-script", Title: "Script", Summary: "Write the verbatim hook, value, and action beats.", Output: "shortform-script.md", Skills: []string{"video-storytelling", "video-creation"}},
		{ID: "shortform-shotlist", Title: "Shot list and prompts", Summary: "Turn each beat into a costed shot with its generation prompt.", Output: "shortform-shotlist.md", Skills: []string{"video-cinematography", "video-model-selection"}},
		{ID: "shortform-generate", Title: "Generate shots", Summary: "Produce the approved shots.", Output: "shortform-generation.md", Skills: []string{"fal-ai", "google-ai", "video-cinematography", "video-model-selection"}},
		{ID: "shortform-assemble", Title: "Assemble and mix", Summary: "Stitch, mix, overlay text, and render the candidate.", Output: "shortform-render.md", Artifacts: []string{"shortform-final.mp4"}, Skills: []string{"video-editing"}},
		{ID: "shortform-check", Title: "Quality check", Summary: "Verify the render, the hook, and platform safe areas.", Output: "shortform-delivery.md", Artifacts: []string{"quality-report.json", "qa-contact-sheet.jpg"}, Skills: []string{"video-quality"}},
	},
}
