# Skill system — current state and design

**Date:** 2026-08-04
**Status:** design, not implemented
**Scope:** shared AgentWorks code (`agent_go/pkg/skills`, `step_based_workflow`) + `workspace/`. Consumed by AgentWorks, Sparkquill, and Video Studio.

## Why this doc

Wiring per-stage skills for Video Studio surfaced what looked like a broken skill architecture. On investigation most of it is sound, and the real gaps are narrow. This records what is actually true so the next change is small and correct instead of a rewrite.

## What already exists (verified)

**One resolver, two origins.** `skills.LoadAttachable(workspaceAPIURL, names) []*llmtypes.Skill` turns skill *names* into full skill *definitions*. Per name (`pkg/skills/runtime_loader.go:95`):

```go
func loadOneAttachable(workspaceAPIURL, folderName string) (*llmtypes.Skill, error) {
	if skill := builtinAttachableSkill(folderName); skill != nil {
		return skill, nil                              // builtin: no disk, no network
	}
	parsed, err := GetSkill(workspaceAPIURL, folderName) // fallback: workspace registry
	...
	skill.SupportingFiles = loadSkillSupportingFiles(workspaceAPIURL, folderName)
}
```

Builtin wins; workspace is the fallback. Supporting files (`scripts/`, `references/`, `assets/`) come along, so adapters that project skills to disk get the whole bundle.

**Every consumer goes through it.** Chat (`cmd/server/server.go:4998`), delegation (`cmd/server/delegation.go:331`), and — importantly — workflow **step** agents (`step_based_workflow/supplementary_prompts.go:45`, resolving a step's `enabled_skills`). Step agents attach real definitions via `BaseAgent.ApplyIdentity`, not prompt text.

**mcpagent materialises them.** `SkillProjector.ProjectSkills(workdir, skills)` writes each skill into the provider's own working directory (`.claude/skills/` for claude-code, `.agents/skills/` otherwise). `llmtypes.Skill` carries `Content` plus `SupportingFiles []SkillFile{RelPath, Content []byte}`, so a skill is a folder represented as data.

**Builtins already bypass disk.** `builtinAttachableSkill` serves a skill by name from a hardcoded registry. Its contract is explicit (`pkg/skills/builtin_browser_skills.go:5`): *"Builtin names must not exist on disk — a disk copy would be shadowed at attach time and could carry contradictory guidance."*

### Consequences worth stating plainly

- Shipping a skill inside a product binary and referencing it from `enabled_skills` **already works**. It needs no workspace file, no `npx`, no GitHub.
- There is **no** "definitions vs names" split. Names are the interface; definitions are the resolved form.
- `SelectedSkills` (workshop preset) and `enabled_skills` (step-level, "overrides preset if specified") are both name lists feeding the same resolver.

## The actual gaps

**1. The builtin registry has no registration API.** `builtinAttachableSkill` is a hardcoded `switch` in the shared package. A product cannot contribute its own builtins without editing shared code, so Video Studio's embedded pipeline skills have no supported way in. *This is the only gap blocking Video Studio.*

**2. The namespace is flat, with one vestigial exception.** `DiscoverSkills` (`pkg/skills/discovery.go:222`) lists `skills/*/` and recurses into exactly one folder — literally named `custom` — one level deep, naming those skills `custom/<name>`. Nothing in the codebase writes to `custom/`; only the reader knows it exists. Deeper trees (`skills/pipelines/cinematic/research-director/`) are silently skipped: `pipelines` is treated as a skill folder, has no `SKILL.md`, and is dropped.

**3. No layering.** Every skill is peer-level, so there is nowhere to express "shared by all pipelines" vs "specific to this pipeline". OpenMontage already solves this with `meta/` (cross-cutting), `core/` (technical), `pipelines/<pipeline>/<role>-director` — and its reuse comes precisely from that split.

**4. Products inherit skills they never use.** `workspace/skill_sync.go:26` hardcodes `{Source: "anthropics/skills@skill-creator"}` and installs it into whatever `--docs-dir` the sidecar is given. Video Studio's launcher points that at `~/VideoStudio`, so a video product ships a skill-authoring skill. The installer is also `npx`-only and silently no-ops when `npx` is absent (`workspace/skill_sync.go:35`), so system skills are best-effort in any environment without node.

**5. Naming.** The generic builtin registry lives in a file called `builtin_browser_skills.go`, which reads as browser-specific and hides a general mechanism.

## Design

### D1 — Builtin skill registration (unblocks Video Studio)

Replace the hardcoded switch with a registry a product can add to at init:

```go
// pkg/skills
func RegisterBuiltin(skill *llmtypes.Skill)   // panics on duplicate name
func builtinAttachableSkill(name string) *llmtypes.Skill  // reads the registry
```

Products register at package init:

```go
// internal/videoproduct
//go:embed skills/...
var skillFiles embed.FS

func init() {
    for _, s := range parseEmbeddedSkills() { skills.RegisterBuiltin(s) }
}
```

`enabled_skills: ["cinematic-research-director"]` then resolves with no disk write and no installer. Keeps the existing "builtins must not exist on disk" invariant — now enforceable, since registration is explicit.

Additive: existing hardcoded builtins move into the registry at init; every current caller is unchanged.

### D2 — Layered naming

Adopt OpenMontage's layering as the naming convention:

| Layer | Name shape | Example | Owner |
|---|---|---|---|
| meta | `meta/<name>` | `meta/checkpoint-protocol` | shared |
| core | `core/<name>` | `core/ffmpeg` | shared |
| pipeline | `pipelines/<pipeline>/<role>` | `pipelines/cinematic/research-director` | product |

Requires D3. Until then, flat prefixed names (`cinematic-research-director`) are the compatible subset — the layering is then a convention, recoverable later by renaming.

### D3 — Nested discovery

Replace the `custom` special case with a bounded recursive walk: any folder containing `SKILL.md` is a skill, named by its path relative to `skills/`. Cap depth (3 is enough for `pipelines/<pipeline>/<role>`) to keep listing cheap.

`GetSkill` already handles this — it does `path.Join(SkillsBasePath, folderName, SkillFileName)`, so a slashed name reads correctly today. Only *discovery* is the constraint. `custom/<name>` keeps working unchanged, since it is just one case of the general rule.

### D4 — System skills become opt-in

`workspace/skill_sync.go` should take its skill list from configuration rather than a hardcoded slice, so a product declares what it wants. Video Studio would declare none. Low priority — `skill-creator` is inert — but it removes a confusing artifact and the silent `npx` dependency.

## Migration

1. **D1** — registry + move existing builtins into it. No behaviour change; unblocks Video Studio immediately.
2. Video Studio registers its pipeline skills as builtins and sets `enabled_skills` per stage. Flat names (D2 subset).
3. **D3** — nested discovery, then rename to layered names.
4. **D4** — config-driven system skills.

## Blast radius

- **D1** additive; existing callers untouched. Risk: duplicate registration between a builtin and a disk skill — mitigated by panicking on duplicate names, which surfaces the existing "must not exist on disk" invariant at startup instead of silently shadowing.
- **D3** changes what `DiscoverSkills` returns for nested folders that previously produced nothing. Anything today relying on nested folders being *ignored* would change — none found.
- **D2** renames are visible in stored step configs (`enabled_skills` values), so migrate names and configs together.
- **D4** touches `workspace/`, shared by every product using the sidecar.

## What this does not change

The resolver, the name-based interface, `ApplyIdentity`, and mcpagent's projector all stay as they are. They already work; the gaps are registration, namespace shape, and defaults.
