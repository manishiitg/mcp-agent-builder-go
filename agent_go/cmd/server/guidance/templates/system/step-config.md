## STEP CONFIG — planning/step_config.json (per-step tuning)

Every step has an optional config entry that overrides defaults for that step. All of it is set with **`update_step_config(step_id, ...)`** and removed with **`update_step_config(step_id, clear=[...])`** (clearing returns the field to its default). Never hand-edit `planning/step_config.json`. This doc is the one-stop map of the knobs; load the linked deep-dives when a knob needs more than the summary.

### Store access — who can touch the three persistent stores

A step's access to each store is independent and defaults differently. Grant the least access the step actually needs.

| Field | Values | Default | Grant when… |
|---|---|---|---|
| `learnings_access` | `read` · `read-write` · `none` | `read` | `read-write` only for reusable execution HOW (browser selectors/timing/auth, API/MCP quirks, CLI/SDK patterns, parsing/retry/recovery) — also requires a concrete `learning_objective`. `none` only when shared SKILL.md would mislead the step. |
| `knowledgebase_access` | `read` · `write` · `read-write` · `none` | `read` | Since PLAT-055/K, unset now defaults to `read` (mirrors `learnings_access` — every step sees KB context/notes unless told otherwise) and auto-promotes to `read-write` if a `knowledgebase_contribution` is already staged. WRITE stays a deliberate opt-in: set `write`/`read-write` plus a non-empty `knowledgebase_contribution`. Set `none` explicitly only when KB would mislead the step or a routing step needs a lean prompt — an explicit `none` always wins over the default. |
| ~~`db_access`~~ | **retired** | `read-write` | Had no runtime effect — `resolveDBAccess` ignored it and always granted read-write, while the enum invited agents to "restrict" a step that never was restricted. Removed in PLAT-061; existing values still parse and are ignored. Every workflow step gets managed read-write DB access. |

- **DB contract:** every agentic workflow step receives `query_workflow_db` and `mutate_workflow_db`; it does not receive raw SQLite access. Saved scripted/application code retains the absolute `$DB_PATH` compatibility variable and must never use a relative `db/db.sqlite` path.
- **Rule of thumb:** routing, validation, mechanical transforms, aggregation/report-shaping, human approval, pure db/KB readers, and mature scripted steps should usually stay read-only on learnings. DB access is intentionally uniform and is not a per-step tuning decision right now.
- Deep dive on what belongs in each store and the write contracts: `read_skill(skills=[{"name":"builder-reference","path":"references/stores.md"}])`.

### The three locks

| Lock | Scope | Effect | Set when |
|---|---|---|---|
| `lock_learnings` | per-step | Stops this step's post-run learning writes to `learnings/_global/SKILL.md` (writes still allowed while `_global/` is empty, to bootstrap). Reads unaffected | deliberate Workshop/user decision after reviewing stable evidence; runtime never auto-locks it. Since 1.0.22 the upgrade-time `upgrade-learnings-lock-audit` preflight reports (never clears) any locked step whose `review_notes` doesn't justify the freeze, via `record_pulse_finding` with `recommended_route="decision_required"` — **`lock_learnings_reason` is required** (PLAT-059): `update_step_config` rejects `lock_learnings=true` without it. Under the shared topic-organised skill a locked step reads every other step's contributions and can never give anything back, so state what was reviewed and why further contribution would make the skill worse. If the step simply has no reusable HOW to offer, use `learnings_access="read"` instead — that needs no reason. |
| `lock_code` | per-step (scripted) | Freezes `learnings/{step}/main.py`, skips the fix loop | **user asks to lock** → allow it; **Workshop auto-locking on its own** → only after 10+ scenario-covering runs |
| `lock_knowledgebase` | workflow-wide | Freezes `knowledgebase/notes/` auto-updates | when KB is curated and should stop auto-evolving |

Only pass a lock field when you are explicitly changing it — passing `lock_learnings:false` while editing other fields resets a previously set value.

### Execution mode + which model runs the step

- **`declared_execution_mode`**: `agentic` vs `scripted`. Two different paths, don't conflate them:
  - **Scripts are the default for DETERMINISTIC execution** — fixed API/SDK calls, CLI commands, known pagination, data fetching, stable parsing/normalization, math, data transforms, mechanical persistence, and fixed SQL. Declare these steps scripted from initial design and author/test `main.py`; no prior run count is required. If behavior **varies run-to-run or needs adaptive judgment** (most browser/UI flows, LLM reasoning, fuzzy extraction, live discovery), keep it `agentic`.
  - **Use coherent scripted fetchers, not micro-scripts** — batch related calls and transforms when they share one source/auth/retry/output contract. Do not create one scripted step per endpoint, command, or tiny transformation, and do not put an entire branching business workflow into one script. The usual architecture is scripted fetcher(s) → durable DB/file output → one large agentic message sequence.
  - **User explicitly asks for a scripted step** (e.g. "make this scripted so I can test it") → set `scripted` right away — the user owns that call; **no run-count gate**. But if the work isn't deterministic, say so plainly first ("this flow reads live UI state, so a frozen script will break often — agentic is more reliable; want me to script it anyway?") and honor their decision.
  - **Workshop mode selection** → move or create obviously deterministic API/CLI/data work as scripted immediately. The 10+ representative-run threshold applies only before freezing it with `lock_code`, not before choosing scripted mode.
  - Set `declared_execution_mode_reason` either way.
- **`use_code_execution_mode`**: per-step override of the preset's code-execution toggle (nil = inherit).
- **Model selection**: `execution_tier` (`high`/`medium`/`low`) maps to the workflow's tiered allocation; `execution_llm` / `validation_llm` pin a specific published model for that role. Prefer tiers over hard pins, and prefer leaving the tier unset over pinning it. Full framework: `read_skill(skills=[{"name":"builder-reference","path":"references/llm-selection.md"}])`.

### Ops-owned decisions need a stated reason (PLAT-060)

Three fields are cost decisions owned by the `technical_review` model-tier or cost focus, and each **requires a
paired reason — `update_step_config` rejects the change without it**:

| Field | Required reason | The consequence the reason must acknowledge |
|---|---|---|
| `execution_tier` | `execution_tier_reason` | Pinning the tier **disables adaptive tiering** for the step — it stops promoting high→medium automatically after 3 stable runs |
| `execution_llm` | `execution_llm_reason` | A pin **outranks `execution_tier` entirely** and will not follow provider-profile updates |
| `declared_execution_mode` | `declared_execution_mode_reason` | Scripted freezes behaviour into `main.py`; agentic pays for judgment every run |

Cite the owning `technical_review` finding id, the current state, and the evidence
— and the `human_input_id` when the change was user-approved. Clearing a field
clears its reason; clearing never requires one.

**If the evidence does not settle it, do not make the change.** Raise a decision
with `create_human_input_request` and park the finding `awaiting_user`.
Uncertainty is a legitimate terminal state; an invented reason is worse than no
change, because a fabricated justification is harder to challenge later than a
missing one.

By contrast `learning_objective` and `knowledgebase_contribution` are **not**
gated — they are expected to be refined continuously as evidence accumulates,
and a write-time gate would suppress exactly that iteration. Record the why in
`review_notes`.

### Other common fields

- **`validation_schema`** — the only automated gate (set via `update_validation_schema`); catches stale files, missing fields, constraint violations. Every step needs one.
- **`enabled_skills`** — step-level skill selection (step execution does NOT inherit workflow-selected skills; set explicitly). `enabled_custom_tools`, `selected_servers`, `selected_tools` — narrow the step's tool surface.
- **`additional_read_paths`** — narrowly grant declared workflow-relative inputs outside the standard execution/db/KB/learnings surfaces (for example `["variables"]`). It is read-only. Absolute paths, `.` and `..` traversal are rejected, and the grant never expands writes.
- **`review_notes`** — one-line WHY for non-obvious config (future Pulse and Workshop reviews read it). Record it whenever you set learning/KB access or designate a db writer.
- **`description_reviewed`**, `coding_agent_tmux_lifecycle`, `disable_parallel_tool_execution` — situational. (`transport` was never a step-config field; naming it in `clear_fields` is reported as a no-op.)

### Workflow

1. `update_step_config(step_id, <field>=<value>, ...)` to set; `update_step_config(step_id, clear=["<field>", ...])` to revert to default.
2. Pair access grants with their prerequisite (`learnings_access: read-write` ⇒ `learning_objective`; KB write ⇒ `knowledgebase_contribution`).
3. For the reliability/strategy decision-making that drives most config changes: `read_skill(skills=[{"name":"builder-reference","path":"references/optimize-playbook.md"}])`.
