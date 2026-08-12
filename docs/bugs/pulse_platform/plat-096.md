[← Pulse platform issue index](../pulse_platform_issue_register.md)

# PLAT-096 — contract upgrades had two delivery paths, an unfenced stamp, and no way to finish unattended

| Coordination | Value |
|---|---|
| Assigned agent | Claude Code |
| Ticket state | `implemented` — runtime reverify pending |
| Last synchronized | `2026-08-12` |

- **Priority:** P1 integrity — a contract migration was recorded as done without
  being performed, and the mechanism that allowed it is reachable from any turn
  in a scheduled session.
- **Observed on:** `Workflow/confida-login` (label `confida-qa-testing`), stuck
  at contract `1.0.20` against a current `1.0.25` while its QA workflow had not
  executed for days.

Four defects in one subsystem. They were found together because each was hiding
the next.

## 1. The stamp was not fenced to its own turn

`set_workflow_contract_version` wrote `workflow.json.version` whenever it was
called. Nothing bounded *when*.

Timeline, session `schedule-manual--9d61046d_1786501703795408000`:

| Time | Event |
|---|---|
| 07:58:23 | Scheduler opens turn 1/7 `upgrade-current-artifact-contract`, target `1.0.21` |
| 08:01:35 | `waitForWorkshopIdle` returns a genuine idle; manifest still `1.0.20` → **turn adjudicated failed** |
| 08:11:10 | The same session, now on an unrelated Pulse turn, stamps `1.0.21` |
| 08:18:27 | Next preflight reads `1.0.21`, trusts it, asks for `1.0.22` |
| 08:23:01 | Backup commit `826d435` sweeps the orphan stamp into git and flags it |
| 08:31:58 | Commit `521208d` reverts it by hand as "unauthorized" |

The `1.0.21` artifact-purity migration was skipped outright. Only the manual
revert stopped that from sticking.

Idle detection was **not** at fault — `schedulerWorkshopMaxInactivity` is 10
minutes, the turn took 3.2, and the wait returned correctly. Two other things
were:

- The scheduler sent the failure reason back into the same live session three
  seconds after adjudicating it: *"The scheduler's reason is: … did not stamp
  required version 1.0.21."* The finalizer turn then acted on it.
- The write arrived as a shell curl to `$MCP_CUSTOM/set_workflow_contract_version`
  through `execute_shell_command`, not as a native tool call. **No prompt-level
  rule could have refused it.**

**Fix.** `pkg/contractupgrade` holds a process-local grant: the scheduler mints
one when it opens an upgrade turn and revokes it the moment it adjudicates — on
the passing path as much as the failing one. The check lives in the tool
executor, which the curl path also reaches, so both are covered by construction.
`pulseSafeRunFailureReason` keeps the preflight label in the finalizer's text but
drops the target version and the stamp verb.

Sessions are deliberately **not** killed at adjudication. These rungs delete
files and rewrite plans; truncating one mid-write is worse than the bug.

## 2. Two delivery paths for the same migrations

`b4e4fc14` (2026-07-08) delivered contract upgrades through the Pulse Review+Fix
turn. `f58ac5b5` (2026-07-16) replaced that with the blocking pre-run preflight —
its own comment says *"contract upgrades are a blocking preflight, not post-run
cleanup"* — and did not remove the older path. Both ran for four weeks.

They are not equivalent. The preflight runs one rung per turn and re-reads the
manifest to verify that exact target before advancing. The Pulse path
concatenated every outstanding rung into a single review turn with no
verification, no ordering, and no stamp gating.

Surfaced on 2026-08-12 when confida-login's preflight failed open after three
strikes, skipped all four rungs, and Pulse then handed the agent `1.0.21`,
`1.0.22`, `1.0.23` and `1.0.25` bundled into one dispatch. The agent asked
whether they were genuine and whether it could take them one at a time — the
right question. `1.0.23` deletes report sources after consolidating them;
running it unverified alongside three others is how a half-applied migration
becomes unreconstructable.

**Fix.** Pulse no longer carries contract upgrades.
`postRunMonitorStepsForManifest`, `postRunMonitorUpgradeStepsForManifest` and the
Review+Fix `preflight` parameter are removed. The order is now single:
**upgrades → workflow → Pulse.** The version pair the Pulse path appended
(`Current workflow.json version seen by scheduler … Target …`) was moved into
`workflowVersionUpgradePlan` — the preflight, which actually gates on the stamp,
had never been told which versions it was moving between.

## 3. An unattended turn was asked to ask a question

Three separate turns identified the same real blocker — `builder/improve-archive/2026-06.html`
holds 19 `F-2026-06-*` finding IDs that appear in none of `pulse_finding_events`
(209 rows), `pulse_finding_details` (2), or `pulse_review_log` (25) — correctly
refused to destroy it, and ended with *"Which would you like?"*. These turns are
scheduler-fired outside working hours. Nobody answered, so the stall repeated on
every trigger and the QA workflow never ran.

**Fix, in two parts.**

- The `1.0.21` instruction now decides instead of asking, and does it without
  data loss: it **moves** `builder/improve.html` and `builder/improve-archive/`
  into `migration-backups/artifact-purity-<UTC timestamp>/` — an existing
  convention in four workflows — reads them back at the new path, and treats a
  move that cannot preserve a file as a genuine blocker. It also answers the
  recorded revert on its merits rather than talking over it: the objection was to
  destroying history, and a relocation does not destroy it.
- Every upgrade query carries an execution-context note: this is automated
  platform maintenance, the turn owns the decision, and **stopping without
  stamping is explicitly an acceptable outcome**. What it removes is only the
  ineffective form of stopping — a question in reply text that no one receives.
  `create_human_input_request` is the durable channel, and PLAT-092/093's
  pre-run drain applies the answer before the next run.

**Wording is load-bearing here.** A first draft said *"do not re-open it as a
judgement call"* and *"a question reaches nobody"*. An agent that had correctly
blocked this migration refused again and named the urgency framing as its reason
— it read as pressure to override a safety pause, which is the shape of an
injection attempt. It was right to refuse. `TestEveryUpgradeQueryCarriesTheUnattendedContract`
fails if those phrasings return.

A parallel answer channel (`workflow.json.contract_upgrade_decisions` plus two
`update_workflow_config` parameters) was built and then **removed as a
duplicate**: PLAT-092/093's operator-decision lifecycle already exists, already
reaches the owner, and already drains *before* the run rather than a cycle late.
Its own doc comment says it "mirrors the contract-upgrade preflight".
`TestNoParallelContractUpgradeDecisionChannel` pins the removal.

## 4. Pulse ran on a run that never happened

A blocked preflight still triggered a full Pulse pass, which could only spend an
LLM turn restating the blocker — on every trigger, for as long as the workflow
waited. A declined stamp now returns an error wrapping
`errWorkflowUpgradePreflightBlocked` and the caller skips Pulse:

```
[PULSE] skipped for <schedule>: the contract-upgrade preflight blocked, so the
        workflow did not run and there is no evidence to review
```

Deliberate limits: a missing upgrade path counts (it blocks permanently);
**fail-open does not** (the schedule runs, so there is a real run to review); and
transient manifest-read faults still notify, being one-off infrastructure errors
rather than a standing decision.

## Verification boundary

Source coverage proves the stamp is refused after adjudication, for a version
other than the open target, on a second use of a spent grant, and with no
session; that a blocked preflight is distinguishable from an ordinary run
failure; and that every upgrade query carries the execution-context note without
the coercive phrasings. `pkg/contractupgrade` covers the store directly.

**Not yet proven at runtime:** that confida-login climbs `1.0.20 → 1.0.25` under
the new instruction. As of 2026-08-12 the workflow is still at `1.0.20` with both
archive files intact; an operator answered `relocate-and-stamp` on
`contract-version-1021-migration-conflict` at 05:36Z and that decision shows
`consumed_at` empty, so the drain has not applied it. Live proof is that climb
plus a `set_workflow_contract_version` success line falling **between** a turn's
start and its completed line in `server_debug.log`, rather than trailing it by
minutes as at 08:11:10.

## Related

- PLAT-086 — the schedule execution-model contract whose `1.0.25` rung is the
  last of these migrations. Its message validator produced a false rejection
  during this investigation; see that ticket.
- PLAT-092 / PLAT-093 — the operator-decision lifecycle this ticket routes
  blocked upgrades into instead of building a second one.
- PLAT-051 — added `set_workflow_contract_version` to give the upgrade turn a
  real tool to stamp with. This ticket bounds when that tool may be used.
