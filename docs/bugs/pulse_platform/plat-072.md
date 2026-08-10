[← Pulse platform issue index](../pulse_platform_issue_register.md)

# PLAT-072 — reused `iteration-0` makes per-run cost unattributable and overwrites run provenance

| Coordination | Value |
|---|---|
| Assigned agent | unassigned |
| Ticket state | `open` — root cause identified from six independent workflow reports; storage layer already correct, readers and provenance are not |
| Last synchronized | `2026-08-10` |

- **Priority:** P1 — blocks per-run cost measurement, and destroys artifacts that other findings depend on
- **Owner:** cost ledger readers (`get_cost_summary`, `cost_storage.go`), run-folder rotation
- **Found on:** six workflows independently, over two weeks

## Why this ticket exists

Six workflows filed the same defect in six different wordings, none of them referencing each other. All are `external_action_required`, so runs will never close them, and no platform ticket owned them:

| workflow | first seen | reported |
|---|---|---|
| social-media | 2026-07-29 | `costs/execution/default/*.json` attributes every scheduled run to `run_folders["iteration-0/default"]`, so per-run and per-slot LLM cost cannot be read from the ledger |
| hetznerssh | 2026-08-05 | Reusable run-folder keys conflate separate executions |
| rtslatency | 2026-08-05 | two daily files hold parts of the same `iteration-0/dev` run under an identical `run_folders` key; one charges $14.79 to a step id `10` that does not exist in `plan.json` |
| linkedin | 2026-08-06 | `get_cost_summary(run_folder)` aggregates historical executions from a reused run folder instead of isolating the current execution, so its per-run costs and totals are false |
| build-in-public | 2026-08-03 | `get_cost_summary` returns *"missing evidence: no run token usage data found at `runs/iteration-0/default/token_usage.json`"* while `costs/execution/default/2026-08-02.json` holds a fully priced $3.4158 per-step breakdown |
| upwork | 2026-08-04 | every scheduled run writes to the same `run_folder iteration-0`, so `runs/` and `evaluation/runs/` are overwritten in place and eval rows carry no distinguishing run identity |

Six teams-of-one arriving at the same conclusion independently is strong evidence. That it went unowned for two weeks is a property of `external_action_required` having no exit path, tracked separately.

## The storage layer is already right — do not "fix" it

`orchestrator/cost_storage.go` carries **both** keys:

```go
Executions map[string]*ExecutionTokenUsage `json:"executions,omitempty"`   // keyed by immutable execution_id
RunFolders map[string]*TokenUsageFile      `json:"run_folders,omitempty"`  // v1 projection
```

and `cost_storage.go:337` already quarantines the legacy side deliberately:

> `run_folders` is the v1 projection. It may be historically merged, so keep it separate instead of letting it contaminate a UUID record.

reading them into a `legacy:`-prefixed namespace. `ops-review.md` documents the same rule for reviewers: *"the immutable `execution_id` (or `evaluation_id`) is the record identity ... `run_folder` and `archived_run_folder` are display metadata only. Never merge or compare records merely because they share an `iteration-0/...` path: that path is reused after rotation."*

Verified on live data: upwork's `costs/execution/daily-bid/2026-08-10.json` keys its record by UUID (`1f0f36fb-c246-...`), with a correct `by_step_and_model` breakdown.

**So the format and the reviewer contract are correct.** Three things around them are not.

## The three real defects

1. **A reader looks in a path the writer never uses.** build-in-public's report is exact: `get_cost_summary` reports *no data at `runs/iteration-0/default/token_usage.json`* while a fully priced breakdown sits in `costs/execution/default/*.json`. The tool reports "missing evidence" for data that exists, which is worse than an error — reviewers conclude the run was free.

2. **Readers still aggregate the legacy projection.** linkedin's report — `get_cost_summary(run_folder)` summing historical executions from a reused folder — is the failure the storage comment predicts. Keying a *query* on `run_folder` cannot isolate a run when the folder is reused by design.

3. **Provenance is destroyed, which is not a cost problem at all.** social-media and upwork report that re-using `runs/iteration-0/default` **deletes the previous occupant's artifacts** — social-media notes only one archived file survived a 2026-08-04 rotation, taking `propose_new` findings, eval provenance, learnings freshness and the run-mode router's input with it. This is the more damaging half: cost can be recomputed, deleted evidence cannot, and other findings' `next_check` evidence silently disappears.

## Why it matters beyond the ledger

- **PLAT-069 depends on this.** A per-run cost/duration/score trend cannot be built while runs are not separable. Fixing PLAT-069 without this produces a confidently wrong trend line.
- **Today's tier retune is unmeasurable for the same reason**, and the baseline captured in `baseline-2026-08-10-pre-tier-retune.json` inherits the flaw — it aggregates `by_step_and_model` across records that may be legacy-merged.
- It compounds a separate known issue: cost totals before 2026-07-28 price output tokens only and omit cache-read, so historical comparison is already restricted (`0a1afaf8767f89ac`, social-media, seen 3).

## Suggested fix

1. **Point `get_cost_summary` at the real store** and key it by `execution_id`, with `run_folder` accepted only as a lookup hint that may resolve to several executions — in which case return them separately rather than summing. Never report "missing evidence" without checking `costs/execution/<group>/<date>.json`.
2. **Make the legacy projection visibly legacy at the read boundary**, so a caller cannot silently receive merged v1 data as if it were a single run. The `legacy:` prefix already exists internally; it should survive to the caller.
3. **Stop destroying artifacts on rotation** — decide deliberately between archiving the previous occupant or allocating a fresh folder. `execution_defaults.always_use_same_run` already exists as the intended control; social-media has it `false` and was still overwritten, so the control is either not honoured or not the one that governs this.

## Acceptance

- `get_cost_summary` returns per-execution costs for a workflow whose runs share `iteration-0`, and never reports missing data that exists in the ledger.
- Legacy run-folder-keyed records are distinguishable from execution-keyed ones at the API boundary.
- A rotation does not delete the prior run's artifacts, or does so only where explicitly configured.
- The six source findings can then be closed against this ticket rather than aging on the external-action board.
