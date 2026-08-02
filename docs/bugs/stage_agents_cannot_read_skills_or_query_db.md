# Report: Stage-Agent Skill and DB Access — Wrong Allow-List Diagnosis, Real Attachment Gap

## Status

**Fixed and corrected 2026-08-02.** This investigation mixed two different
questions and originally gave one answer to both:

1. Does `read_skill` need to appear in a builder-owned stage allow-list? **No.**
   mcpagent owns the intrinsic grant and injects it per turn.
2. Did these stage agents have any attached skills for that intrinsic tool to
   read? **They did not.** That was a real wiring bug.

The fix attaches the complete workshop reference surface to Pulse/Goal Advisor
stage agents. `read_skill` correctly remains absent from builder allow-lists.
The duplicate `query_workflow_db` entry introduced during the incorrect first
fix was also removed.

## Symptom

Every Pulse and Goal Advisor stage agent was instructed to load reference docs
and reason about the finding backlog. The original investigation measured the
five builder allow-lists:

Measured membership across the five stage allow-lists, before:

```text
                          query_workflow_db  mutate_workflow_db  read_skill
pulseFixerStage                  true              true            false
goalAdvisorCommonMut             true              false           false
goalAdvisorReadOnly              true              false           false
goalAdvisorFinalProposal         true              false           false
goalAdvisorFinalApproved         true              false           false
```

`read_skill` is in **none** of them. That is correct and is not evidence that the
tool is unavailable. But the same stage construction path also attached no
skills, so the intrinsic tool had no identity surface from which to exist. The
allow-list observation was a false diagnosis of a real symptom.

## The argument that was wrong

It ran like this:

> `read_skill` is intrinsic, but `SetToolAllowList` also gates the code-execution
> registry. Therefore an unlisted intrinsic tool must be refused over the bridge,
> just as `get_api_spec` would be. Because the agents use an isolated CLI cwd,
> projected skills must also be unreachable.

Two premises are wrong.

First, mcpagent injects the grant itself, per turn, in
`agent/turn_session.go`:

```go
allowed := policy.allowedMap()
if allowed != nil && s.agent.isIntrinsicIdentityTool(readSkillToolName) {
    allowed[readSkillToolName] = true
}
codeexec.SetSessionToolAllowList(s.agent.sessionID, allowed)
```

`read_skill` is therefore reachable over the bridge with **no builder grant**
when the agent has attached skills. `get_api_spec` has no such injection, which
is why that analogy breaks.

Second, isolation does not make projected skills unreachable. Attached skills
are projected into the isolated CLI directory itself, where native CLI skill
loading can read them. Projection is an optimization; `read_skill` is the
transport-neutral contract. In the failing construction path there was simply
nothing attached, so neither projection nor `read_skill` had content to expose.

Two existing tests already asserted this and were treated as obstacles rather
than as evidence:

```text
goal_advisor_review_test.go:149  approved builder allowlist should not own mcpagent's intrinsic read_skill tool
reviewer_surface_test.go:38      reviewer allowlist should not own mcpagent's intrinsic read_skill identity tool
```

A third caught the same change from the other side — `TestToolSetInvariants`
rejected allow-listing a tool with no builder-declared registration path.

**The lesson worth keeping:** a test that blocks a change is a claim about the
system, and the first move is to find out why it was written. Three independent
tests failing in agreement is not friction to route around.

## The real skill-access bug

The intrinsic grant is conditional. In `mcpagent/agent/skill.go`:

```go
func (a *Agent) isIntrinsicIdentityTool(name string) bool {
    return a != nil && name == readSkillToolName && len(a.attachedSkills) > 0
}
```

Before the fix, `runGoalAdvisorStageAgent` created the isolated agent, registered
tools, and ran it without attaching the reference bundles its prompt named.
`read_skill` therefore did not exist for that identity.

The stage now calls `guidance.AttachReferenceSurface("workshop", ...)`, attaching
the `system-tools`, `builder-reference`, and `workflow-commands` bundles before
the first turn. Those bundles include all five documents named by the Pulse
review/fixer prompt, including `review-artifact-drift`, which lives outside the
main reference bundle.

## The DB-list finding

`query_workflow_db` was also reported missing from the Pulse Fixer. It was not:
it already came from the inherited common list. The initial fix added it again,
creating a silent duplicate.

Grepping a function body cannot answer "what is in this list" when lists are
composed by `append`, and the grep window matched the newly-added comment text as
well as the entry. The correct check is to call the function and test membership.
That is now a test rather than an exercise:

```go
for name, list := range map[string][]string{ … } {
    if !slices.Contains(list, "query_workflow_db") { … }
}
```

## What actually changed

1. The complete reference surface is attached to every Pulse/Goal Advisor stage
   identity before its first turn.
2. `read_skill` stays out of every builder allow-list because mcpagent owns the
   intrinsic grant.
3. Every stage allow-list includes `query_workflow_db`.
4. Only the Pulse Fixer includes `mutate_workflow_db`.
5. The duplicate `query_workflow_db` entry was removed.

Four invariants are now pinned in `tool_surface_invariants_test.go`, including
the one that would have prevented the incorrect allow-list fix:

1. **No builder list claims `read_skill`** — mcpagent owns that grant, and a
   second owner is two sources for one fact.
2. **Every stage agent can `query_workflow_db`** — reading workflow data is
   symmetric.
3. **Only the single writer can `mutate_workflow_db`** — writing is not.
4. **No duplicates** — harmless at runtime, but it means two places believe they
   own the same grant, which is how these lists drift.

mcpagent separately tests that an attached skill registers `read_skill`, that
the tool survives filtering, and that it appears in the coding-agent MCP bridge.
The guidance package tests that the attached workshop surface contains all five
reference files named by the Pulse prompt.

## Remaining test gap

The current tests prove the components but not the final stage wiring. No test
constructs the `runGoalAdvisorStageAgent` identity and asserts that its completed
definition contains the three reference bundles and its intrinsic `read_skill`
bridge tool. Removing the `AttachReferenceSurface` call could therefore leave
the allow-list and materialization tests green.

Add a stage-construction regression, or extract the identity-attachment helper
and test that helper at the stage boundary. The assertion should be about the
completed agent definition/tool surface, not source text.

There is also redundant attachment today: `builder-reference` is added once in
`registerWorkshopMutationToolsForToolAgent` and then supplied again by
`AttachReferenceSurface`. Skill attachment replaces by name, so this is harmless,
but one construction owner would be clearer.

## Still open: agents outside this family

`runReviewPlanAgent` still renders a narrow surface:

```json
{"human_tools": ["human_feedback","notify_user"],
 "workspace_advanced": ["execute_shell_command"]}
```

The log did **not** say `read_skill` was unavailable. It said the automated
plan-review endpoint was unavailable, while the agent was already executing the
plan-review contract itself. That is not evidence of a skill failure.

There is real DB evidence: the prompt tells this reviewer to inspect
`db/db.sqlite`, but it has no `query_workflow_db`. In the observed run it tried
to copy the database through shell and received `Operation not permitted`, then
completed from other artifacts. That is a silently degraded DB review and should
be resolved independently by granting the read-only DB tool or removing the live
DB inspection requirement.

## Why tool-error visibility matters here

This investigation is exactly why bridge failures need explicit UI and backend
signals. A failed HTTP-backed tool can be wrapped as stdout with
`exit_code: 0`; before the logging/UI fix it looked like a successful call and
encouraged architectural guesses from the agent's later workaround. The stable
backend marker is `[TOOL_ERROR]`, and the UI recognizes the structured
`tool execution failed|canceled|timed out: layer=...` envelope even when the
outer shell exit code is zero.

## Related

- `docs/bugs/custom_tool_category_as_agent_addressing.md` — the `read_skill`
  migration and why `get_reference_doc` was removed.
- `docs/bugs/tool_failures_invisible_in_backend_logs.md` — why failed bridge
  calls previously looked successful during this diagnosis.
- `docs/bugs/workflow_step_shell_working_directory.md` — why the isolated CLI
  cwd, projected skills, and the server-side bridge-shell cwd are separate.
