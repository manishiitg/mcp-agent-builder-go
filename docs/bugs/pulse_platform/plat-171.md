[← Pulse platform issue index](../pulse_platform_issue_register.md)

# PLAT-171 — a step can be instructed to write another step’s execution output

| Coordination | Value |
|---|---|
| Assigned agent | unassigned |
| Ticket state | `open` |
| Last synchronized | `2026-08-21` |

- **Priority:** P1 — one contradictory handoff instruction can make a long
  workflow turn fail after it has completed the useful work, and the scheduler
  then reports the whole occurrence as failed.
- **Owner:** workflow plan contract/guidance, execution prompt rendering, and
  the typed plan-review surface.
- **Related:** [PLAT-062](plat-062.md) (a prompt named a forbidden write
  target), [PLAT-162](plat-162.md) (plan-tool schema enforcement), and
  [PLAT-169](plat-169.md) (Folder Guard lifecycle).

## Incident

Social Media's `execute-verify` step was correctly limited to writing under:

```text
runs/iteration-0/default/execution/execute-verify/
```

The plan first told it to write `action_queue_refreshed.json` in its **own**
declared output folder. Later, the same description also instructed it to copy
that result to:

```text
runs/iteration-0/default/execution/execute-intent-queue/action_queue.json
```

The latter is owned by `execute-intent-queue`. The Folder Guard correctly
denied the write. The agent then ended with a terminal failure even though it
had produced useful verification work; the parent Codex scheduler conversation
subsequently had no valid final response to present.

This is not a request to widen the guard or manually patch the Social Media
plan. The Guard is enforcing the intended isolation boundary.

## Why the boundary exists

Each execution step owns its output folder. Other steps may read declared
upstream outputs, but cannot mutate them. That prevents two steps — including
parallel or resumed steps — from silently overwriting one another's evidence,
leaving an output that cannot be attributed to its real producer.

Cross-step data flow must therefore be explicit:

```text
producer writes own output → declared dependency/handoff → consumer reads it
```

If a new artifact is required, the producing step owns it in its folder. If an
existing artifact must be revised, the step that owns that artifact (or an
explicitly declared handoff/aggregation step with its own output) performs the
revision. An ordinary downstream verifier cannot patch a sibling's output.

## Root cause

The platform renders exact narrow write grants but lets a plan description
contain arbitrary prose about execution paths. Plan validation checks graph and
schema structure, yet does not give the reviewing agent a first-class view of
the ownership contract for every named execution path. As a result, a plan
editor can add a later “copy it to step B” instruction that contradicts the
step's actual write grant and earlier output instruction.

The current runtime error only says that the write was denied. It does not tell
the agent which step owns the target, whether a read handoff already exists, or
the safe repair choices. Agents can therefore spend turns retrying or turn one
clean plan inconsistency into an opaque scheduler failure.

## Proposed repair

1. **Expose ownership as plan-review evidence.** `validate_plan_change` and
   review guidance should return the selected step's writable output root,
   referenced upstream output roots, declared dependencies, and the owner for
   every named execution path. The agent remains responsible for reading the
   complete plan and deciding the correct handoff; Go does not rewrite the
   plan.
2. **Add an ownership contradiction gate.** When a changed description names a
   concrete `runs/.../execution/<step-id>/...` target outside the edited
   step's own output root, flag it before persistence with the target owner and
   require an agentic correction. This is a narrow structural guard, not a
   natural-language plan generator or a broad mechanical plan reviewer.
3. **Make the runtime denial actionable.** Folder Guard errors for a known
   sibling execution path should identify the owning step and say: write the
   artifact in this step's output folder, declare/read a handoff, or change the
   owner step through the typed plan tools. They must remain terminal for the
   current turn — no automatic guard widening or cross-step write fallback.
4. **Teach the planner and reviewer the contract.** Plan/design and Pulse
   Review guidance must require an explicit producer → handoff → consumer path
   for every cross-step artifact. Reviewers should inspect altered step
   descriptions for contradictory later instructions, including generated
   migration text.

## Acceptance

- A plan change that names a sibling step's execution folder as a write target
  is rejected with the real owner and an actionable repair path.
- A legitimate downstream step can read the declared upstream artifact without
  receiving a sibling write grant.
- The normal execution Folder Guard remains narrow; no step obtains broad
  `execution/` write access.
- The live denial includes the owning step and no retry loop occurs.
- An agentic Pulse Review followed by Pulse Fixer can repair the Social Media
  contradiction using typed plan tools; a producing run proves the repaired
  handoff without manual plan editing.

## Decision history

- **2026-08-21:** Preserve per-step output ownership. The fix is better plan
  evidence, actionable diagnostics, and agentic plan repair — not relaxed
  filesystem authorization or a backend that silently moves artifacts.
