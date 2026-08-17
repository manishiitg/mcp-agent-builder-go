[← Pulse platform issue index](../pulse_platform_issue_register.md)

# PLAT-117 — a dropped UI progress event can permanently prevent a turn from completing

| Coordination | Value |
|---|---|
| Assigned agent | unassigned |
| Ticket state | `open` — root-caused with live evidence; structural fix designed below |
| Last synchronized | `2026-08-17` |

- **Priority:** P0 — this is the mechanism behind the social-media false-failure
  emails. It makes `terminal()` unreachable, so an affected turn is
  *guaranteed* to end in an idle-wait timeout no matter how well the underlying
  work went, and the operator is then told the workflow never ran.
- **Owner:** background-agent registry (`background_agents.go`), conversation
  turn lifecycle (`conversation_turn_lifecycle.go`), workflow progress bridge
  (`planning_exports.go`), workshop execution notifier (`delegation.go`)
- **Related:** [PLAT-116](plat-116.md) (the false-failure symptom this explains),
  [PLAT-071](plat-071.md) (regressed separately, see below),
  [PLAT-091](plat-091.md) (`ReconcileOrphanedProgressChildren`, the compensator
  this makes unnecessary for liveness), [PLAT-114](plat-114.md) (durable log
  rows left permanently `running` by the same omission),
  [PLAT-112](plat-112.md) (turned off the only individual consumer of these
  records)

## The category error

`workflowProgressBridge` (`planning_exports.go:1756-1793`) turns each workflow
step's start/end progress events into a **background-agent record**, so the UI
can show per-step progress. `workshopExecutionBgNotifier` registers them, and
its own doc comment states the intent plainly:

> registering workshop step/background executions in bgAgentRegistry **so that
> HasRunningAgents() returns true and the frontend keeps polling for events**

That intent is a *display* concern. The problem is that `bgAgentRegistry`
answers two very different questions from one data structure:

| Question | Read by | Needs |
|---|---|---|
| "Is background work happening?" (display, polling) | `HasRunningAgents()` — 9 call sites | best-effort is fine |
| "May this turn finish?" (lifecycle) | `conversationTurnTreeSnapshot` → `RunningChildren` → `terminal()` | must be exact |

Progress mirrors are produced by best-effort event pairing: a start mints an ID
cached under `agentType:stepIndex:agentName`, and only a matching end closes it.
Any dropped, mismatched, or never-emitted end leaves a record open forever.
That is tolerable for a spinner. It is not tolerable for turn completion — and
today it feeds both.

**So a dropped UI progress event can permanently prevent a real turn from
completing.** The work itself is meanwhile recorded in two more reliable
places: the run's own durable per-step files on disk
(`runs/<iteration>/<group>/execution/<step>/session.json` — what Pulse Gate
actually reads), and the parent `workflow-full-<id>` agent, which is registered
for the entire run and already supplies the aggregate display signal.

## Live evidence (social-media, 2026-08-16)

Session `schedule-manual--5227790a_1786893680879441000`, schedule
`5227790a` ("Daily Execution x3").

1. `15:22:25Z` parent `workflow-full-msvyee8q01` starts. Two step-0 progress
   mirrors open at `15:25:39Z` and `17:46:32Z` (same step index, different
   agentType/agentName keys) and never receive an end event.
2. `18:57:13Z` the parent completes — the real work is done.
3. `00:27:13 IST` `ReconcileOrphanedProgressChildren` (PLAT-091) settles both
   orphans via `agent.SetError(...)`. That sets `Status=Failed` and
   `CompletedAt`, but **does not** mark them notified and **never calls**
   `emitBackgroundAgentCompleted`.
4. `conversation_turn_lifecycle.go:209-211` deliberately counts a
   completed/failed-but-unnotified child as **running** (an anti-race measure
   for real children). Both orphans therefore stay "running" permanently.
5. `terminal()` requires `RunningChildren == 0`. With two permanent phantoms the
   tree **can never be terminal** — the turn was structurally guaranteed to time
   out regardless of what the provider did.
6. Worse, `RunningChildren > 0` also triggers the three-hour
   `schedulerWorkshopLiveChildCeiling` grace, so the run burned **4h09m**
   instead of failing fast.
7. The turn's real answer arrived `00:50:43 IST` — *"Done — the pinned execution
   route ran exclusively... One original post landed and was verified"* — with a
   real post live on X. Ten minutes of no further progress later, the wait
   timed out at `01:00:43` reporting `running_children=2`, and the operator was
   emailed **"Workflow did not start. No results were produced."**

Corroboration from PLAT-114's durable log: those exact two agent ids
(`workflow-full-msvyee8q01-step-0-msvyijui03`, `-step-0-msw3jq3i0f`) are still
stored `status='running', completed_at=NULL`, because step 3's omission also
skips the durable-log completion hook. Same omission, two visible symptoms.

## Why the existing compensator did not save it

`ReconcileOrphanedProgressChildren` exists precisely to settle orphans left by
missing end events ("a finished parent cannot still have live progress
children"). It is a compensator for unreliable event pairing, and it has its own
gap: it settles the record's *status* but not its *notified* flag, and the
liveness snapshot treats settled-but-unnotified as running anyway. Making the
compensator more correct is not the fix — the fix is that a progress mirror
should not be able to hold a turn open in the first place.

## Required repair

**1. Make the distinction first-class, not an inline filter.** One authoritative
predicate for "this record is a progress mirror, not work", derived from the
declared `Kind` (`workflow_step`) with the legacy `execution_type` metadata
fallback, mirroring the existing `isWorkflowStepTrackingExecution`
classification. Not re-sniffed per call site.

**2. Split the two registry questions explicitly** so a future consumer must
choose rather than inherit the wrong one by default:
   - `HasRunningAgents()` — unchanged, the *display* question. Progress mirrors
     still count, so no UI/polling regression at any of its 9 call sites.
   - a new *work* predicate — excludes progress mirrors — documented as the
     question lifecycle code must ask.

**3. `conversationTurnTreeSnapshot` counts only real work in `RunningChildren`.**
Progress mirrors still contribute `LastProgressAt`: an advancing step is real
evidence the turn is alive, it just cannot hold the turn open by itself.
Excluding them cannot end a turn early, because a step record is minted from its
parent's id (`fmt.Sprintf("%s-step-%d-%s", b.parentID, ...)`) and so can never
outlive a parent that is not itself registered.

**4. Fix `ReconcileOrphanedProgressChildren` to settle completely** — mark
notified and emit the completion — so PLAT-114's durable log stops accumulating
permanently-`running` rows. Still required after (3), because the log is wrong
today independently of liveness.

**5. Restore PLAT-071 (separate regression, found while investigating this).**
Its fix shipped 2026-08-10, was extended by PLAT-084 on the 11th, and was
dropped whole on 2026-08-13 by `d18e071e1` ("Unify scheduled turn lifecycle and
runtime tab routing") when `waitForWorkshopIdle` was replaced by
`waitForConversationTurnTree`. `workshopRunStartedDuringInvocation` has had zero
callers since, and `ProducedRunEvidence` is once again only computed on the
success path — so any failed turn reports "the workflow did not run" without
anything having looked. Both its unit tests still pass, because they call the
helper directly instead of driving `executeWorkshopJob`.

## Not fixed here

- **Whether these registry records need to exist at all.** They exist to keep
  the frontend polling, but the parent already supplies that aggregate, the
  per-step *events* reach the UI independently of the registry, and PLAT-112
  (2026-08-16) turned off the diagnostic rail that was their only individual
  consumer. Deleting the mirror — and `ReconcileOrphanedProgressChildren` with
  it — is likely correct and strictly simpler, but requires proving the runtime
  tab's live progress is event-driven rather than registry-driven. Deliberately
  not assumed.
- **PLAT-116's own root cause** is narrowed but not closed by this. This
  explains the `running_children=2` incident completely; the earlier
  `running_children=0` incident (2026-08-16 19:08) has a different shape and
  still needs its own trace.

## Acceptance

- A turn whose only outstanding children are progress mirrors reaches
  `terminal()` and completes normally.
- `HasRunningAgents()` behaviour is unchanged at all 9 call sites.
- An orphaned progress mirror settles completely: not counted as running, and
  written to the durable log as terminal.
- A failed turn that produced real run evidence never reports "the workflow did
  not run".
- Every one of the above is proven by a test that reaches the state through the
  product path, not by constructing it — the specific gap that let PLAT-071 be
  deleted with its tests still green.
