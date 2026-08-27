[← Pulse platform issue index](../pulse_platform_issue_register.md)

# PLAT-190 — proposal: hard-enforce "read the related skill before calling this tool"

| Coordination | Value |
|---|---|
| Assigned agent | unassigned |
| Ticket state | `open` — proposal + design only, **not implemented, not scheduled** |
| Last synchronized | `2026-08-27` |

- **Priority:** P2 — no live defect; a requested reliability improvement. Today
  every "read the reference doc/skill before doing X" rule (plan-mutation
  tools, report tools, LLM-config tools, schedule tools, secret tools) is
  prompt-steering only — there is no hard runtime check that the agent
  actually did it, and [PLAT-189](plat-189.md) found at least one case
  (`message_sequence`'s prior lack of proactive schema surfacing) where an
  agent could plausibly act without the context it needed.
- **Owner:** `agent_go/pkg/agentwrapper/llm_agent.go` (enforcement
  chokepoint), a new session-scoped tracker (new file, not yet created),
  `mcpagent/agent/agent.go`'s `AgentEventListener`/`AddObserver` (read
  detection).
- **Related:** grew directly out of [PLAT-189](plat-189.md)'s investigation —
  that ticket found `get_reference_doc` no longer exists at all (fully
  retired since commit `9eddc087` "guidance: drop doc-read gate," which had
  removed the *previous* hard gate, `WithDocPrecondition`/`DocReadTracker`,
  for a specific reason worth restating below).

## Why the previous gate was removed (do not repeat this)

`WithDocPrecondition`/`DocReadTracker` (removed in `9eddc087`, -456 lines)
blocked `set_workflow_llm_config` unless `get_reference_doc` had been called.
It broke because it verified that **one specific tool call** happened, not
that the content was actually read — once the mega-skill projection started
writing the same reference-doc bytes to `references/<kind>.md` on disk, an
agent reading the doc via a direct file read (or, later, `read_skill`) was
doing the right thing, but the gate only recognized the old
`get_reference_doc` tool call and refused a compliant agent. Its refusal text
told agents that reading the projected skill file "does not register" —
penalizing the exact behavior the skill system is designed to produce.

**Any new gate must recognize every current valid read channel, or it will
repeat this exact failure with different tool names.**

## Current state (confirmed 2026-08-27)

- `get_reference_doc` is fully gone from production code in all three repos
  (`agent_go`, `mcpagent`, `multi-llm-provider-go`) — confirmed by grep,
  zero non-test references. It survives only as negative-invariant test
  assertions (`render_all_test.go`, `materialize_test.go`) that fail if any
  guidance template still calls it, and one stale, unexercised name in
  `toolset_invariant_test.go`'s "known tools" list.
- `read_skill` is the intrinsic replacement (`mcpagent/agent/skill.go:19`),
  described in its own tool description as working "on every transport."
- **A second valid read channel still exists**: `instructions.go:68`
  explicitly tells agents both are legitimate — *"Files such as
  `skills/<name>/SKILL.md` live on local disk... Read them with the declared
  local tools/shell, or load canonical reference docs with
  `read_skill(...)`."* A CLI-native session with shell access can legally
  `cat`/`Read` the projected file directly instead of calling `read_skill`,
  and the harness would see only a generic shell/file-read event — not a
  "skill was read" signal — unless a gate specifically pattern-matches that
  path.
- A side finding while running `TestToolSetInvariants` during this
  investigation: it currently fails for an unrelated reason
  (`get_plan_prompt_health` missing from the test's known-tools list) — a
  small, separate pre-existing gap, not part of this proposal.

## Proposed design

1. **Enforcement chokepoint**: `LLMAgentWrapper.RegisterCustomToolWithTimeout`
   (`pkg/agentwrapper/llm_agent.go:749`) — the same single registration
   chokepoint `productToolGate` already uses for allowlisting. Wrap the
   registered `execute` handler for any tool present in a new tool→skill
   config table: before running the real handler, check a session-scoped
   tracker; if the required `(skill, path)` hasn't been read, return an
   instructive error naming the exact `read_skill(skills=[{"name":...,
   "path":...}])` call to make — not a bare "not registered"/"denied"
   message, learning directly from the old gate's UX mistake.
2. **Read detection, channel 1 (ships first)**: a new session-scoped
   `SkillReadTracker` implementing `mcpagent.AgentEventListener`
   (`HandleEvent(ctx, *events.AgentEvent) error`), registered via
   `w.AddObserver(...)`. Correlates `ToolCallStartEvent`/`ToolCallEndEvent`
   pairs (by `ToolCallID`/`CorrelationID`) for `read_skill` calls that
   complete without error, parses the `skills[].name`/`path` arguments from
   the start event, and marks `(skill, path)` as read for the session.
3. **Read detection, channel 2 (explicit fast-follow, not in v1)**: the same
   tracker also watching shell/file-read tool-call events
   (`execute_shell_command` or equivalent) whose target path matches a known
   `skills/<name>/SKILL.md` or `references/<kind>.md` path. Deferred
   specifically because it adds meaningfully more false-negative surface
   (path-pattern matching against arbitrary shell command text) for real but
   probably rarer coverage — v1 ships with channel 1 only, and this ticket's
   evidence log should note whether v1 was ever observed under-covering a
   direct-shell-read session before channel 2 gets built.
4. **Config table**: tool name → required `(skill, path)`, expandable by
   adding rows, not engine changes. Proposed initial seed, broad per the
   user's explicit direction ("all tools and skills"):
   - Plan-mutation tools (`update_message_sequence_step`,
     `add_message_sequence_step`, `update_scripted_step`,
     `add_scripted_step`, todo_task route mutation tools) →
     `builder-reference:references/plan-design.md`,
     `builder-reference:references/message-sequence.md`,
     `builder-reference:references/prompt-engineering.md` as applicable per
     tool.
   - Report tools (`validate_report_html` and any report-authoring tools) →
     `builder-reference:references/design-reporting-ui.md`,
     `references/improve-report.md`, `references/reporting-policy.md`.
   - `set_workflow_llm_config` → `references/llm-selection.md` (the
     original, single call site the old gate protected).
   - Schedule tools (`create_schedule`, `update_schedule`,
     `create_calendar_schedule`) → `references/schedule-management.md`.
   - Secret-management tools → `references/secret-management.md`.
   - Exact tool-to-doc pairing needs a full audit pass at implementation
     time — this list is a starting point from what this session already
     grounded, not exhaustive.

## Explicitly out of scope for this ticket

- No code has been written. This is a design proposal only, matching the
  user's request ("create plat ticket for this").
- Channel 2 (shell-path read detection) is deliberately deferred past v1 —
  see above.
- The exact full tool→skill mapping table is not finalized here; it needs an
  audit pass across every tool schema that currently contains a
  `read_skill(...)` prompt-steering pointer, at implementation time.

## Risk

This is a fail-closed chokepoint change affecting every gated tool call
across every session. A bug in the tracker (missed event correlation, wrong
skill/path parsing) blocks legitimate work platform-wide, not just one
workflow. Implementation must ship with fail-before/pass-after tests proving
both directions: a session that never read the skill is blocked with a
correct, actionable message, and a session that read it (via the `read_skill`
channel) is not.

## Verification

N/A — no code changed. This ticket exists to record the design so
implementation can proceed against it once approved.
