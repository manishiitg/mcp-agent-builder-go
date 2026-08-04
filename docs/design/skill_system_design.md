# Skill system — current state and narrow product-skill design

**Date:** 2026-08-04

**Status:** D1 implemented, verified, and pushed to `main` in `43d74f14e`; Video Studio adoption remains pending

**Scope:** AgentWorks skill resolution, ordinary workflow steps, and prepackaged products such as Video Studio.

> Video Studio currently lives in the separate `feature/video-product` worktree. Its main chat already attaches embedded skill definitions directly. The unresolved case is a Video Studio stage executed through the ordinary AgentWorks workflow-step runner.

## Decision

There was one genuine missing capability:

> A prepackaged product cannot currently make one of its embedded skills resolvable by an ordinary workflow step's `enabled_skills` name without copying that skill into the workspace or editing the shared AgentWorks builtin switch.

D1 solves it with a small **flat builtin/product skill registry**. It deliberately does not combine the fix with nested skill names, recursive discovery, skill inheritance, Workflow Builder changes, or a migration of existing workflows.

## What a workflow step accepts today

A step does not accept “npx skills”. It accepts a list of **skill names**:

```json
{
  "agent_configs": {
    "enabled_skills": ["agent-browser", "ffmpeg"]
  }
}
```

For each name, `skills.LoadAttachable` resolves a complete `llmtypes.Skill` bundle from one of two runtime origins:

1. A builtin known by AgentWorks. Today the only hardcoded builtin is `agent-browser`.
2. A folder in the workspace at `skills/<name>/SKILL.md`, including readable text files under `references/`, `scripts/`, and `assets/`.

`npx` is only one installation mechanism that can put a skill into the workspace. A workspace skill may instead have been imported, copied, generated, or installed by another mechanism. Once the folder exists, step execution does not invoke `npx`.

Two additional identity skills may be attached independently of `enabled_skills`:

- AgentWorks' workflow reference skill;
- the workflow's `learnings/_global/` pointer when global learnings are enabled.

Those are runtime identity attachments, not members of the step's configured skill-name list.

## How this reaches coding agents

Coding agents do not resolve AgentWorks skill names themselves. AgentWorks and mcpagent prepare the skill bundle first:

```text
step_config.json
  enabled_skills: ["ffmpeg"]
             |
             v
AgentWorks LoadAttachable("ffmpeg")
             |
             v
llmtypes.Skill { name, description, SKILL.md body, supporting files }
             |
             v
mcpagent agent identity
             |
             v
provider projects the bundle into its isolated working directory
```

Provider projection currently uses:

- Claude Code: `.claude/skills/<name>/`
- Codex CLI: `.agents/skills/<name>/`
- Cursor CLI: `.cursor/skills/<name>/`
- Pi CLI: `.pi/skills/<name>/`
- API transports: system-prompt listing plus mcpagent's transport-neutral skill-reading path; no native CLI folder is required.

This projection is load-bearing for workflow and background agents because they can run from isolated temporary directories. A skill located elsewhere on the host filesystem is not automatically visible to the coding CLI.

## Three different selection paths

These paths must remain distinct.

### 1. Workflow Builder

Workflow-level `selected_skills` gives the workshop/builder agent its selected workspace capabilities. It is not inherited by execution steps.

### 2. Ordinary workflow step

Step-level `enabled_skills` is the only configurable skill-name list used by that execution step. The no-cascade rule is intentional and covered by tests.

### 3. Product-owned main agent

A product can already pass complete `[]*llmtypes.Skill` definitions directly in its agent-session configuration. Video Studio's main chat currently does this with `builtinSkills()` and therefore does not need the new registry.

## The genuine gap D1 closes

Video Studio renders its stages as ordinary AgentWorks workflow steps. Its pipeline model already has:

```go
type PipelineStage struct {
    // ...
    Skills []string
}
```

and its step-config renderer already translates a non-empty list to:

```json
{
  "enabled_skills": ["cinematic-research-director"]
}
```

However, `enabled_skills` contains names, while Video Studio's product skills are embedded definitions. Before D1, the shared resolver had no supported way for a product to contribute those definitions: it knew only its hardcoded builtin switch and workspace folders. `RegisterBuiltin` now provides that missing startup boundary.

All current cinematic stages also have empty `Skills` lists, so no stage-specific product skill is attached today. Adding the registry alone is insufficient: Video Studio must subsequently assign the intended names to its stages.

## D1 — flat builtin/product skill registration

Replace the browser-specific hardcoded switch with a small shared registry. Existing callers continue calling `LoadAttachable` by name.

Illustrative API:

```go
// pkg/skills
func RegisterBuiltin(skill *llmtypes.Skill) error
```

A product registers its embedded definitions during startup:

```go
for _, skill := range parseEmbeddedSkills() {
    if err := skills.RegisterBuiltin(skill); err != nil {
        return err
    }
}
```

Then a stage continues using the existing persisted format:

```json
{
  "enabled_skills": ["cinematic-research-director"]
}
```

### Implementation status (2026-08-04)

Implemented in `agent_go/pkg/skills/builtin_skills.go` and pushed to `main` in commit `43d74f14e`:

- exported `RegisterBuiltin(*llmtypes.Skill) error` startup API;
- the existing `agent-browser` definition now registers through the same catalog;
- lowercase/hyphenated identity validation;
- duplicate registration rejection;
- concurrency-safe lookup and registration;
- defensive cloning of paths, metadata, supporting-file entries, and supporting-file bytes;
- the existing builtin-first `LoadAttachable` resolution path is preserved.

The focused package, race-detector, workflow-step skill, server skill, and complete `agent_go` test suites pass. This completes the shared AgentWorks capability. Video Studio still needs to register its embedded definitions during startup and populate the desired `PipelineStage.Skills` values before a real stage consumes them.

### Registry requirements

- Keep names flat, lowercase, and hyphenated. Do not use `/` in skill identities.
- Reject nil skills, empty/invalid names, and duplicate builtin registrations.
- Clone registered and returned definitions so callers cannot mutate shared registry state.
- Make registration concurrency-safe and finish it during application startup.
- Return an error to the product startup path rather than panicking inside the shared library.
- Preserve the existing `agent-browser` definition and resolution behavior exactly.
- Keep builtin-first resolution for backward compatibility.
- Do not expose registry mutation to agents or workflow configuration tools.

## Explicitly deferred ideas

### Slash-based layered names

Do not introduce names such as `pipelines/cinematic/research-director` now. Skill names are currently documented as lowercase plus hyphens, provider projection sanitizes `/`, API routes assume one path segment, and persisted configuration and folder guards also consume the name. A slash identity would not remain stable end to end.

If products need organization in the UI, represent `product`, `layer`, `pipeline`, and `role` as catalog metadata while keeping a stable flat runtime ID such as `cinematic-research-director`.

### Recursive workspace discovery

Recursive discovery is not needed for embedded product skills. Supporting nested workspace identities would require coordinated changes to discovery, reads, CRUD routes, frontend operations, folder guards, projection, and migrations. It is not merely a discovery-loop change.

### Skill inheritance

Do not make workflow-selected skills cascade into steps. The current separation prevents a builder's broad capability set from silently reaching every execution agent. Products and workflows should assign step skills explicitly.

### System-skill installation defaults

Making startup-installed system skills configurable is reasonable but independent. If pursued, AgentWorks must retain its existing default; a prepackaged product may explicitly choose a different set or none. Missing `npx` currently produces a warning rather than a true silent no-op.

## Effect on existing workflows and Workflow Builder

D1 is intended to be additive:

- existing `enabled_skills` arrays remain unchanged;
- existing workspace skill folders resolve as before;
- `agent-browser` resolves as before;
- Workflow Builder discovery, installation, and `selected_skills` behavior remain unchanged;
- no existing workflow migration is required;
- mcpagent and coding-agent projection remain unchanged;
- only products that register new builtins gain additional resolvable names.

Video Studio changes only when it both registers a product skill and assigns that name to a stage's `Skills` list.

## Verification and remaining product proof

Verified in the shared implementation:

1. Existing `agent-browser` builtin resolution remains available through the registry.
2. Existing workspace skills still resolve with supporting text files.
3. An ordinary workflow step with no `enabled_skills` receives no newly selected product skill.
4. Workflow-level `selected_skills` still do not cascade into steps.
5. A registered product skill resolves through the same `LoadAttachable` call used by a workflow step without contacting the workspace.
6. Duplicate and invalid builtin registrations fail clearly.
7. Registry input and output are defensively cloned, including supporting-file bytes.
8. Concurrent registry access passes the race detector.

Remaining product-level acceptance proof:

1. Video Studio registers its four embedded skill definitions during startup.
2. The intended `PipelineStage.Skills` names are populated explicitly.
3. A real isolated Video Studio stage reads an assigned skill and supporting reference through Claude Code's normal projected skill directory.

## Non-goals

This change is not a general skill-system rewrite. It does not change:

- what a skill contains;
- how users install workspace skills;
- how Workflow Builder selects skills;
- the step `enabled_skills` schema;
- how mcpagent represents skills;
- how coding providers project native skill folders;
- the workflow-learnings skill pointer;
- skill invocation semantics inside Claude Code, Codex, Cursor, or Pi.
