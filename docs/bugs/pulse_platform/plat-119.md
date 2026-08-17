[← Pulse platform issue index](../pulse_platform_issue_register.md)

# PLAT-119 — every Pulse step is told to load a skill the Pulse session was never given, and proceeds anyway when it is missing

| Coordination | Value |
|---|---|
| Assigned agent | unassigned |
| Ticket state | `new` — reproduced from a live salesoutreach Pulse run on 2026-08-17; root cause confirmed in code |
| Last synchronized | `2026-08-17` |

- **Priority:** P1 — Gate, Review+Fix and Finalize each open with "load this
  skill and follow it exactly", and on an affected workflow the skill is simply
  absent. The step then improvises. Nothing detects it, nothing warns, and the
  pass is recorded as normal, so Pulse's actual procedure silently stops
  applying while every downstream artifact still claims it ran.
- **Owner:** scheduler Pulse orchestration (`scheduler.go` step queries and
  `selected_skills` plumbing), platform skill attachment (`pkg/skills`)

## How it surfaced

A live salesoutreach Pulse pass produced a complete, well-formed Gate verdict
and then volunteered this at the end:

> the builder-reference/pulse-gate skill named in my instructions isn't attached
> to this session (only agent-browser and workflow-learnings are available), so
> I proceeded from the tool specs and this run's own state directly rather than
> that skill's exact procedure.

The only reason this is known is that the model chose to say so. Nothing in the
platform reported it.

## Root cause

Five Pulse step queries in `scheduler.go` instruct the agent to load the
platform's own reference skill, e.g. Gate:

```text
PULSE GATE / WORKLIST. pulse_run_id=%q, run_folder=%q, run_status=%q.
Load read_skill(skills=[{"name":"builder-reference","path":"references/pulse-gate.md"}])
and follow it exactly.
```

But the Pulse session is created with the **workflow's** skills, not the
platform's:

```go
"selected_skills": sctx.Capabilities.SelectedSkills,   // scheduler.go:4002 (also :2046, :3744)
```

`builder-reference` is a platform skill, not something a workflow selects. It is
therefore attached only where a workflow happens to have it materialised for
some other reason. The two workflows compared:

| Workflow | `capabilities.selected_skills` | `builder-reference` materialised? |
|---|---|---|
| salesoutreach | `["agent-browser"]` | no skill directories at all |
| social-media | `[]` | yes — `.agents/skills/builder-reference/` |

salesoutreach's list matches the agent's report exactly: `agent-browser`, plus
the auto-attached `workflow-learnings`. Nothing else. So the instruction to load
`references/pulse-gate.md` could not be satisfied.

## Why this is worse than a missing file

The failure is silent in both directions:

1. **The platform never checks.** No step verifies the skill loaded before
   acting on instructions that assume it. There is no `SKILL_UNAVAILABLE`, no
   warning line, no degraded-mode marker on the pass.
2. **The output looks correct.** The Gate verdict in question was
   well-structured — mode chosen with a reason, per-module due/skip decisions
   with next-check boundaries, observations recorded. It is indistinguishable
   from a pass that did follow the procedure. Everything downstream — worklist,
   findings, the Pulse popup — treats it as a normal pass.

So the guidance the platform relies on to make Pulse behave consistently can be
absent for an entire workflow, indefinitely, with no signal.

## Scope

`builder-reference` is demanded by five Pulse step queries in `scheduler.go`:
Gate (per-run and periodic-backlog variants), Review+Fix dispatch, and the
Finalizer. Any workflow whose `selected_skills` lack it — the default, since it
is a platform skill and not a workflow choice — runs all of those improvised.
This is not salesoutreach-specific; salesoutreach is only where an agent
happened to say so out loud.

## Required repair

1. **Attach the platform's own skills to platform-owned sessions.** A Pulse
   session's skill set is `workflow skills + the skills Pulse itself requires`.
   The step prompts already declare the dependency; the session construction
   must honour it rather than inheriting only `sctx.Capabilities.SelectedSkills`.
2. **Fail loudly when a required skill is unavailable.** A step told to follow a
   procedure it cannot read must not silently substitute its own judgment. Its
   pass should be recorded as degraded (or refused), not as a normal pass —
   consistent with how this register treats "the platform proceeded as if
   something happened when it did not" elsewhere (PLAT-116, PLAT-117).
3. **Make the absence visible in the Pulse record**, so an operator reading a
   worklist can tell whether the procedure was actually in force.

## Acceptance

- A workflow with `selected_skills: []` runs a Pulse pass in which Gate can load
  `references/pulse-gate.md`.
- Removing the skill makes the pass fail or be marked degraded — never a silent
  normal pass.
- The same holds for Review+Fix and the Finalizer, not only Gate.
- A test drives a real Pulse step through the product path and asserts the
  required skill was actually attached to the session — not that the prompt
  string mentions it.

## Not fixed here

- **Why salesoutreach has no materialised skill directories at all** (no
  `.claude/`, `.agents/`, `.pi/`) while other workflows do. That is likely a
  second, independent gap in first-run materialisation; this ticket is about
  Pulse depending on a skill it never requests, which is true regardless.
