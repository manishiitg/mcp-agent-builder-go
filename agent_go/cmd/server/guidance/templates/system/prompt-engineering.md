## Writing Optimized Step Descriptions (Prompt Engineering)

A step's `description` is the prompt an execution agent actually runs on. Every word costs context and dilutes the ones that matter. This is a different concern from step-type selection or validation design — it is about writing quality: does this description say exactly what is needed, once, and nothing else. Read this before writing or editing any step's `description`.

### Earn every word

Before finalizing a description, cut anything that:
- Restates what the model already knows from training, its system prompt, or an already-loaded skill — do not re-explain what JSON is, what "verify" means, or what a tool already documents about itself.
- Restates content already reachable through `context_dependencies`, `learnings_access`, `knowledgebase_access`, or `db_access` — reference the store ("read `learnings/_global/SKILL.md` for the login selectors") instead of copying its contents inline.
- Repeats the same constraint from a different angle, hoping one phrasing lands — state it once, precisely.
- Hedges with qualifiers that add no checkable meaning ("carefully," "thoroughly," "properly," "make sure to") — state the actual verifiable criterion instead.

### Name the exact shape, not a category

"Return the semantic balance as an object" tells the model an object exists, not what belongs in it — it has to guess, and a wrong guess fails validation with no signal about what was actually expected. Name every required key up front: `semantic_balance: {real_journey_count: number, synthetic_journey_count: number}`. This applies to the description's prose, not only `validation_schema` — the schema catches the wrong shape after the fact; the description prevents the guess in the first place.

### State the outcome, not a micromanaged procedure — unless the task genuinely needs one

Prefer: "Verify every listed API endpoint returns a 2xx status and its response body matches the declared schema; record failures in `api_failures.json`."

Over: "First, open the API testing tool. Then, for each endpoint, carefully send a request. Check the response. If it looks wrong, note it down. Make sure to be thorough."

The first names one precise, checkable outcome. The second spends four sentences restating the same instruction with decreasing specificity, and its vagueness ("looks wrong," "be thorough") gives the model nothing an evaluator could verify against.

Reach for an explicit step-by-step procedure only when the task has a genuinely fixed sequence a model would otherwise get wrong (browser automation against unstable selectors, a multi-stage auth flow, an ordered API call chain with a required delay) — and even then, state the sequence once, as a numbered list, not as prose that restates itself.

### Do not duplicate across steps

If two steps in the same plan describe the same policy, contract, or procedure, that is not two descriptions — it is one description and a reference. Move the shared text to `learnings/_global/SKILL.md` (durable HOW-to-operate knowledge), the knowledgebase (domain facts), or a validation schema (structural contract), and have each step reference it by name. A copy-pasted paragraph across steps is a maintenance liability — a fix to one copy silently leaves the others stale — as much as it is a length problem.

### A rough self-check before finalizing

Could someone who has never seen the workflow read this description and independently write the exact same `validation_schema` from it? If it is vague enough that they would guess differently, it is not precise — regardless of length. Could you delete a sentence without losing information the model needs? If yes, delete it — regardless of how short the description already is.

### Related

- `references/plan-design.md` for step-type selection, context flow, and validation design.
- `references/workflow-patterns.md` for reusable multi-step shapes.
- The Standalone Technical Review's Prompt-contract health check (`/ops-review`) audits existing plans against these same principles after the fact. Applying them while authoring is cheaper than fixing a finding later.
