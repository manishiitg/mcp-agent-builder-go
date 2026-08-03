# Pulse Platform-Issue Register

## Status

Triage snapshot captured 2026-08-03 from the workflow-local Pulse databases.
This is the canonical cross-workflow register for defects that a workflow-level
Pulse Fixer cannot repair. It is not evidence that every historical finding is
still reproducible on the latest binary: items marked **reverify** have an
implementation change but no post-change producing run yet.

This document records platform ownership and deduplication. The authoritative
per-workflow lifecycle remains `db/db.sqlite`; detailed single-defect incident
documents remain authoritative where linked.

## Why this register exists

Pulse currently stores a platform defect inside the workflow that observed it.
The same harness root cause can therefore appear as several workflow findings,
consume reviewer slots, recur forever, and invite a Fixer that has no authority
to change the platform. A cross-workflow register is needed until the lifecycle
has a first-class platform issue and linkage table.

A finding belongs here only when the failed boundary is owned by the workflow
runtime, scheduler, bridge, tool contract, shared persistence, or shared UI—not
by the workflow plan or its data.

## Evidence collection

The snapshot scanned all 11 databases under:

```text
workspace-docs/Workflow/*/db/db.sqlite
```

The structured set is the join of:

```sql
SELECT c.status, c.seen_count, d.finding_id, d.issue_kind,
       d.detail_json, c.step_id, c.text
FROM run_concerns c
JOIN pulse_finding_details d USING (fingerprint)
WHERE c.status NOT IN ('resolved', 'rejected')
  AND d.issue_kind = 'harness_issue';
```

That returned 12 active or acknowledged records across Build-in-public,
LinkedIn, RTS Latency, and Upwork. A second compatibility scan included older
records whose text describes a scheduler, bridge, Folder Guard, working
directory, tool-registration, or media-tool failure but predates
`issue_kind=harness_issue`.

## Priority summary

| Platform key | Priority | Current workflow evidence | State |
|---|---:|---|---|
| PLAT-001 Human-input propagation | P0 | Upwork | **implementation fixed; runtime reverify** |
| PLAT-002 Tool-failure status precedence | P0 | Upwork, Build-in-public | **canonical CLI/runtime fix implemented; runtime reverify** |
| PLAT-003 Workflow DB tool exposure | P0 | Build-in-public, Instagram, RTS Latency | **implemented on current main; runtime reverify** |
| PLAT-004 Scheduler completion detection | P0 | RTS Latency | **fixed; linked finding needs reverify** |
| PLAT-005 `get_api_spec` multi-name contract | P1 | RTS Latency | **fixed in mcpagent; runtime reverify** |
| PLAT-006 Workflow-step shell cwd contract | P1 | RTS Latency | **reverify** implemented fix |
| PLAT-007 Workflow image verification | P1 | Instagram | implemented; runtime/E2E reverify |
| PLAT-008 Phase cost pricing | P1 | Build-in-public | **implemented; runtime reverify** |
| PLAT-009 `get_cost_summary` run resolution | P1 | Build-in-public | **implemented; runtime reverify** |
| PLAT-010 Finding identity split | P1 | RTS Latency | **implemented with migration; runtime reverify** |
| PLAT-011 LLM role visibility | P2 | Build-in-public | **implemented; runtime reverify** |
| PLAT-012 Changelog mutation coverage | P2 | LinkedIn | **implemented; runtime reverify** |
| PLAT-013 Legacy regular-step editing | P1 | RTS Latency | **reverify** implemented fix |
| PLAT-014 Reviewer reference loading | P1 | RTS Latency, Tectonicus | **reverify** after `read_skill` migration |

The two tool-error findings below are one family, not two independent repair
projects. The database-tool symptoms across three workflows are also one shared
capability defect until evidence proves otherwise.

## Detailed issues

### PLAT-001 — `run_full_workflow` drops keyed human input

- **Priority:** P0
- **Owner:** workflow orchestration/handoff
- **Source finding:** `HARNESS-RUN-FULL-WORKFLOW-HUMAN-INPUT-LOSS`
- **Source database:** `Workflow/upwork/db/db.sqlite`
- **Recorded state:** `external_action_required`, severity `high`
- **Problem:** a human-input override supplied to `run_full_workflow` did not
  appear in the target child step's opening prompt.
- **Impact:** run-specific safety or scope constraints can be silently ignored
  while the workflow continues and reports success.
- **Evidence:** the saved scheduler conversation contains the two-feed
  override; the child `search-find-and-shortlist` session does not; its raw
  artifact shows that `most_recent` was scraped anyway.
- **Implementation (2026-08-03):** `run_full_workflow` now parses
  `human_inputs` strictly, rejects malformed and unknown step IDs before
  launch, copies the map into immutable execution/batch contexts, and derives
  one isolated context per dispatched step. The keyed value reaches regular,
  message-sequence, todo, and human-input steps without leaking to siblings;
  an explicit `execute_step(human_input=...)` remains higher priority.
- **Verification:** focused tests cover both MCP-decoded map shapes, fail-closed
  parsing, unknown IDs, two distinct child values, sibling isolation, map-copy
  isolation, and explicit single-step precedence. A real scheduled full-run
  replay remains required before closing the Upwork finding.
- **Current workaround:** compare each producing child's prompt and raw
  provenance with the requested override.
- **Acceptance:** an E2E passes distinct keyed values to at least two child
  steps and proves each child sees only its intended value; missing or unknown
  keys fail before execution.

### PLAT-002 — nested tool failures remain semantically successful

- **Priority:** P0
- **Owner:** tool bridge, timing telemetry, and terminal status
- **Source findings:** `HARNESS-NESTED-ERROR-STATUS-PRECEDENCE` (Upwork) and
  `HARNESS-TOOL-ENVELOPE-ISERROR-2026-08-03` (Build-in-public)
- **Problem:** the outer transport succeeds while the nested tool payload says
  `ERROR`, carries a non-zero nested exit code, or reports an HTTP failure.
  Stored traces still set `IsError=false` and `errored_count=0`.
- **Impact:** retries, alerts, validation, reviewers, and terminal status can
  treat a real failed operation as clean.
- **Important distinction:**
  [tool_failures_invisible_in_backend_logs.md](tool_failures_invisible_in_backend_logs.md)
  fixed visibility with `[TOOL_ERROR]` logs and red UI rendering. It did not by
  itself make the canonical runtime/timing result an error.
- **Implementation (2026-08-03):** `mcpagent/toolerr` now has a narrow canonical
  classifier separate from the broad log-only suspect detector. The CLI stream
  adapter emits `ToolCallErrorEvent` instead of `ToolCallEndEvent` for nested
  failure envelopes, and saved CLI conversation history sets `IsError=true`.
  Sequential and parallel in-process tool paths use the same classifier.
  Problem-reporting/query tools are excluded from payload promotion so a
  returned domain row such as `status=failed` is not confused with transport
  failure.
- **Verification:** fixtures pass for nested `ERROR`, non-zero shell exit,
  permission denial, HTTP 4xx, `success=false`, and MCP `isError`; negative
  controls pass for prose discussing errors and historical failed DB rows. The
  real post-build timing artifact still needs to prove `errored_count` changes.
- **Current workaround:** agents and reviewers parse nested stdout/content and
  apply explicit error precedence themselves.
- **Acceptance:** fixtures for nested `ERROR`, HTTP failure, permission denial,
  and non-zero shell exit all set canonical error state, increment error counts,
  and prevent an unrecovered parent execution from being clean. Text merely
  discussing an error remains a success.

### PLAT-003 — granted DB access does not produce a reachable DB tool

- **Priority:** P0
- **Owner:** workflow-step capability materialization/API bridge
- **Source finding:** `HARNESS-DBTOOL-NOT-EXPOSED-EXEC-2026-08-03`
- **Primary source database:** `Workflow/build-in-public/db/db.sqlite`
- **Related evidence:** Instagram `route-design-plan`; RTS collectors denied
  direct SQLite and unable to discover the sanctioned DB path.
- **Problem:** steps with `db_access=read-write` cannot resolve
  `query_workflow_db`/`mutate_workflow_db` as callable tools. The same tools may
  exist behind an undocumented raw `$MCP_CUSTOM` curl route.
- **Impact:** the permission contract and actual capability disagree. Agents
  burn failed calls, abandon persistence, or publish a false claim that the DB
  capability does not exist.
- **Current implementation:** managed DB tools are already capability-derived
  from trusted `db_access`, even when a step has a narrower explicit custom-tool
  list. `read` materializes query only; `read-write` materializes query and
  mutation. Direct SQLite/WAL/SHM paths remain blocked for managed agentic
  sessions, while the mutation executor independently fails closed without an
  explicit read-write grant.
- **Verification (2026-08-03):** the real stdio MCP bridge → custom executor →
  workspace HTTP API → WAL-mode SQLite E2E passes for query and mutation.
  Focused capability tests pass for read-only/read-write exposure and for
  read-only/no-grant mutation denial. The three source findings should be moved
  to platform reverify rather than prompting another workflow-level repair.
- **Current workaround:** raw `$MCP_CUSTOM/query_workflow_db` and
  `$MCP_CUSTOM/mutate_workflow_db` calls when their exact API is known.
- **Acceptance:** real workflow-step bridge E2E tests for read-only and
  read-write grants prove the matching tools are discoverable and callable;
  no-grant and read-only mutation attempts fail closed.

### PLAT-004 — scheduler success can precede actual workflow completion

- **Priority:** P0
- **Owner:** scheduler/background execution completion barrier
- **Legacy source:** RTS Latency `run_concerns`, `bug_review`,
  `external_action_required`, seen twice
- **Problem:** `schedule-runs.json` recorded a dev run as success after 84.6
  seconds even though the pipeline ended at `step-daily-latency-report` and
  later security, cost, digest, and checkpoint work never completed.
- **Impact:** Pulse reviews incomplete evidence, finalization may start early,
  and missed producing steps are reported as a healthy run.
- **Resolution:** fixed by commit `f69de7b6c` ("Stop the reconciler calling an
  in-flight run successful"). The reconciler no longer treats a temporarily
  `completed` workshop turn as completion of the multi-turn schedule. Only the
  scheduler's own completed turn loop can record success; an abandoned run is
  eventually recorded as interrupted/error. The normal scheduler path also
  waits on the consolidated runtime state, which includes foreground work,
  tracked child executions, background agents, and tmux activity.
- **Verification:** scheduler reconciliation and workshop-idle regression tests
  pass on current main. RTS Latency needs one uninterrupted post-fix scheduled
  run to close its historical finding.
- **Acceptance:** the scheduler cannot emit terminal success until every
  required child has an authoritative terminal completion; a lost/truncated
  child yields failed or timed-out status with its identity and last evidence.

### PLAT-005 — `get_api_spec` does not honor its multi-name input

- **Priority:** P1
- **Owner:** API bridge tool-spec lookup
- **Source finding:** `HARNESS-GET-API-SPEC-ARRAY`
- **Source database:** `Workflow/rtslatency/db/db.sqlite`
- **Problem:** an array of valid tool names is coerced into one literal unknown
  name instead of returning several specs.
- **Impact:** an agent concludes that working tools do not exist and can publish
  that false diagnosis downstream.
- **Resolution:** fixed in mcpagent commit `ea60eb2` ("Stop get_api_spec failing
  on shape and routing"). `tool_name` accepts one string, a decoded JSON array,
  or a coding-CLI-serialized JSON-array string; canonical tool names are sorted,
  resolved, and authorized independently of the compatibility-only
  `server_name` field.
- **Verification (2026-08-03):** array, serialized-array, mixed known/unknown,
  custom/MCP routing, and unavailable-server tests pass. The RTS finding is
  historical evidence and should move to platform reverify on the rebuilt
  binary.
- **Current workaround:** one lookup call per tool name.
- **Acceptance:** string and string-array inputs return the same canonical specs;
  mixed known/unknown input identifies only the unknown names without hiding
  the known results.

### PLAT-006 — workflow-step shell cwd disagreed with its contract

- **Priority:** P1
- **Owner:** workflow step session/bridge context
- **Legacy source:** RTS Latency `step-daily-latency-collect-dev-voice` finding,
  currently `awaiting_verification`
- **Incident:**
  [workflow_step_shell_working_directory.md](workflow_step_shell_working_directory.md)
- **Problem:** a dedicated child shell ran from the run execution folder while
  some prompts/skills claimed docs-root cwd; the earlier inverse failure also
  existed when dedicated sessions lost their run cwd.
- **Current state:** code now assigns a workflow-step run cwd directly and
  fails closed when it is absent. The remaining finding must be replayed on the
  rebuilt runtime to distinguish stale guidance from a runtime regression.
- **Acceptance:** regular, message-sequence, todo, reviewer, and Fixer session
  tests state their cwd contract explicitly and observe exactly that directory.
  Guidance contains no contradictory relative-path examples.

### PLAT-007 — image verification cannot reliably read workflow images

- **Priority:** P1
- **Owner:** media tool path normalization and model selection
- **Legacy source:** Instagram `route-build-carousel`, currently
  `awaiting_verification`
- **Problem:** `read_image` rejected existing absolute workflow paths as
  relative because it expected a `_users/default/...` layout. Earlier runtime
  evidence also showed a retired default vision model returning 404.
- **Impact:** image-producing workflows can pass only renderer provenance and
  hashes, not direct visual/OCR verification.
- **Current state:** implementation exists for absolute workspace-path
  normalization, rejection of relative/out-of-workspace paths, and dynamic
  provider/model discovery via `list_llm_capabilities`. The focused workspace
  unit tests pass. This item is not awaiting implementation; it is awaiting a
  rebuilt-runtime E2E against an actual workflow image so the linked finding
  can be verified and closed.
- **Acceptance:** an E2E creates an image under a real workflow execution
  folder, reads it by the exact absolute and workflow-qualified paths, and uses
  a supported configured model. A bad path and unavailable model produce
  distinct actionable errors.

### PLAT-008 — phase costs omit input and can use the wrong rate card

- **Priority:** P1
- **Owner:** cost observer/phase ledger pricing
- **Source finding:** `HARNESS-PHASE-COST-PRICING-2026-08-03`
- **Source database:** `Workflow/build-in-public/db/db.sqlite`
- **Problem:** phase rows omitted input cost and one Opus row used Sonnet output
  and cached-input rates.
- **Impact:** historical and daily spend is materially understated and changes
  without a workload change.
- **Implementation (2026-08-03):** Claude transcript usage now reports total
  prompt input (fresh + cache create + cache read), while retaining the raw
  cache components. Phase persistence carries cache read/write separately,
  recalculates every component from the effective model, and stamps
  `pricing_model_id` plus `pricing_version`. Claude Opus 5 and Sonnet 5 golden
  cases prove distinct input/output/cache-read/cache-write rates and totals.
- **Acceptance:** one immutable model identity selects one versioned rate card;
  input, output, reasoning, cache-read, and cache-write components reconcile to
  the total. Golden tests cover both Opus and Sonnet on adjacent dates.

### PLAT-009 — `get_cost_summary` loses grouped and historical run spend

- **Priority:** P1
- **Owner:** cost-query projection
- **Source finding:** `HARNESS-COSTSUMMARY-RUNFOLDER-2026-08-03`
- **Source database:** `Workflow/build-in-public/db/db.sqlite`
- **Problem:** grouped run folders are looked up through a legacy
  `runs/.../token_usage.json` path even when the authoritative execution ledger
  has the data, and an ungrouped query omits spend recorded on another date.
- **Impact:** the tool reports missing evidence or only part of the true run
  cost.
- **Current workaround:** read and sum `costs/execution/<group>/<date>.json`.
- **Implementation (2026-08-03):** the query now resolves the run's scope and
  group, enumerates every authoritative daily shard, selects only the requested
  run projection, and merges each shard once. Current and historical runs use
  the same path; the retired `runs/.../token_usage.json` lookup is migration
  input only.
- **Acceptance:** query results reconcile to authoritative cost events across
  dates, groups, models, and execution IDs without double-counting aggregate
  and per-step views.

### PLAT-010 — one logical finding can have two lifecycle fingerprints

- **Priority:** P1
- **Owner:** Pulse finding identity/deduplication
- **Source finding:** `HARNESS-FINDING-FINGERPRINT-SPLIT`
- **Source database:** `Workflow/rtslatency/db/db.sqlite`
- **Problem:** two rows share finding ID
  `HARNESS-REFDOC-REVIEW-ARTIFACT-DRIFT` but have different fingerprints and
  split recurrence counts.
- **Impact:** closing one row leaves its twin open; Gate and reviewers keep
  rediscovering an issue already handled.
- **Current workaround:** reconcile every row with the same finding ID and
  semantic behavior together.
- **Implementation (2026-08-03):** structured `finding_id` is now the
  workflow-global lifecycle identity; reviewer module and wording no longer
  participate. Schema startup canonicalizes old single rows and merges twins,
  sums recurrence, moves attempts/verifications, preserves colliding events as
  explicit identity-merge events, and enforces one case-insensitive finding ID.
- **Acceptance:** one stable platform issue can link several observations and
  fingerprints, while one workflow lifecycle row cannot claim the same
  finding ID twice. Migration merges existing twins without deleting events.

### PLAT-011 — model configuration hides non-tier roles

- **Priority:** P2
- **Owner:** LLM configuration API/UI
- **Source finding:** `HARNESS-GETLLMCONFIG-ROLES-HIDDEN-2026-08-03`
- **Source database:** `Workflow/build-in-public/db/db.sqlite`
- **Problem:** `get_llm_config` shows high/medium/low execution tiers but omits
  builder, maintenance, Pulse, and Chief-of-Staff roles.
- **Impact:** the operator cannot see the role responsible for major review or
  maintenance spend.
- **Implementation (2026-08-03):** `get_llm_config` resolves and renders
  Builder, execution high/medium/low, Maintenance, Pulse, and Chief of Staff
  with provider/model, reasoning effort, inheritance source, and override
  status. Provider profiles expand through the same provider-owned defaults as
  runtime; an explicit but missing Chief-of-Staff role is honestly shown as
  unconfigured rather than assigned an invented fallback.
- **Acceptance:** resolved configuration returns every effective role, source
  of inheritance, provider/model, reasoning level, and override status.

### PLAT-012 — changelog coverage excludes material dependent artifacts

- **Priority:** P2
- **Owner:** managed mutation audit/changelog
- **Source finding:** `HARNESS-CHANGELOG-COVERAGE-001`
- **Source database:** `Workflow/linkedin/db/db.sqlite`
- **Problem:** evaluation-plan and learning mutations were absent from the
  canonical changelog Artifact Review uses.
- **Impact:** a grading-contract or runtime-guidance change can escape dependent
  artifact review.
- **Implementation (2026-08-03):** every managed changelog entry is completed
  with a canonical target, before/after SHA-256 refs, actor, and dependency
  class. Evaluation-plan edits already use the managed tool; runtime learning
  turns now hash the complete `learnings/_global` tree (including references)
  before and after the serialized write turn and append a typed mutation event
  whenever the tree changes, even if the turn itself later reports an error.
- **Acceptance:** every sanctioned material mutation emits one typed changelog
  event with target, before/after references, actor, and dependency class.

### PLAT-013 — legacy regular steps lacked a semantics-preserving edit path

- **Priority:** P1
- **Owner:** plan/step editing API
- **Source finding:** `HARNESS-LEGACY-REGULAR-DESC-EDIT`
- **Source database:** `Workflow/rtslatency/db/db.sqlite`
- **Current state:** **reverify**. The editing path has since been changed, but
  the stored platform finding has not observed a successful repair on both RTS
  voice collector steps.
- **Acceptance:** edit the description of a fixture and one real legacy
  agentic `regular` step without changing its type, schedule, model, tools, or
  other persisted fields; read it back and run plan validation.

### PLAT-014 — reviewer prompts named unavailable reference documents

- **Priority:** P1
- **Owner:** reviewer reference/skill delivery
- **Source finding:** `HARNESS-REFDOC-REVIEW-ARTIFACT-DRIFT`; legacy copies in
  RTS Latency and Tectonicus, plus a Tectonicus Goal Advisor variant
- **Current state:** **reverify**. Reviewer guidance has migrated from
  `get_reference_doc(kind=...)` to attached skills loaded with `read_skill`.
- **Impact:** old reviewers started without their required method and silently
  substituted a different checklist.
- **Acceptance:** each scheduled and slash-command reviewer loads every named
  skill/reference in its isolated stage session; no retired
  `get_reference_doc` instruction remains on a live path.

## Explicitly not platform issues

The following remain workflow-owned or evidence-state items even when they are
currently blocked:

- a collector catches an exception and ignores the loaded variables;
- a step hardcodes an incorrect log group or database probe result;
- a plan's editable Folder Guard configuration omits a required folder;
- a fix is waiting for the next producing run;
- an upstream service has not produced data;
- a strategy requires a user decision or deployment approval;
- a workflow writes through direct `sqlite3` despite having a documented,
  reachable sanctioned mutation tool.

If the configuration is not editable because the platform exposes no sanctioned
edit path, that missing edit path is the platform defect—not the workflow's
desired configuration.

## Lifecycle behavior required

Until the platform lifecycle is implemented, these records should be treated as
one of two states:

- **platform_open:** confirmed on the current binary and owned outside the
  workflow;
- **platform_reverify:** a platform change exists, and the next applicable real
  run must prove the observation is gone.

They must remain visible to the operator but must not consume normal workflow
Fixer capacity, count as recurring workflow-strategy evidence, or occupy the
reviewer's new-finding cap. A new observation should link to the platform key
above rather than filing another independent platform issue.

After a platform fix ships:

1. move every linked workflow finding to awaiting verification;
2. run the smallest real or E2E producer that exercises the boundary;
3. close all linked findings together when it passes;
4. reopen the single platform issue with the new evidence when it fails.

## Recommended implementation order

1. PLAT-001 human-input propagation — implementation complete; real full-run reverify pending.
2. PLAT-002 canonical tool-error precedence — implementation and focused tests complete; persisted runtime evidence pending.
3. PLAT-003 DB capability exposure — already implemented and bridge/capability tests pass; source workflows need post-build reverify.
4. PLAT-004 scheduler completion barrier — fixed in `f69de7b6c`; RTS scheduled-run reverify pending.
5. Add the platform issue/link lifecycle so the backlog stops duplicating them.
6. Reverify PLAT-006, PLAT-013, and PLAT-014 on the rebuilt runtime.
7. Repair/query-test the remaining P1 observability and media defects.
8. Address the P2 configuration and changelog completeness gaps.

The first four are ordered ahead of cost and UI completeness because they can
change what a workflow does while still allowing it to report success.
