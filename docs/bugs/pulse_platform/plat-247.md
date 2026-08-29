[← Pulse platform index](../pulse_platform_issue_register.md)

# PLAT-247 — restore `read_image`/`image_gen`/`image_edit`: PLAT-244's media-tool retirement swept a load-bearing inspection tool out with the generation tools

| Coordination | Value |
|---|---|
| Assigned agent | Claude Code |
| Ticket state | `implemented` |
| Last synchronized | `2026-08-29` |

- **Priority:** P1 — user-directed correction, reported live.
- **Owner:** `pkg/workspace/advanced_tools.go`, `pkg/workspace/tools.go`,
  `cmd/server/virtual-tools/workspace_tool_registry.go`,
  `pkg/common/tool_categories.go`, `cmd/server/server.go`,
  `pkg/orchestrator/base_orchestrator_folder_guard.go`,
  `pkg/orchestrator/base_orchestrator_tools.go`, `cmd/server/instructions.go`,
  `interactive_workshop_manager.go`.
- **Related:** [PLAT-244](plat-244.md) (the retirement this partially
  reverses) and the two commits that implemented it —
  `01909fb4b` "feat: focus workspace provider tools on text and search" and
  `7ec055a74` "refactor: remove retired media tool escape hatches".

## What happened

PLAT-244 narrowed the provider-backed workspace tool surface to
`generate_text_llm` and `search_web_llm` only, retiring
`image_gen`/`image_edit`/`generate_video`/`text_to_speech`/`speech_to_text`/
`generate_music` — **and** `read_image` — in the same sweep, with no written
rationale distinguishing why `read_image` specifically needed to go.

Live impact: a coding-agent CLI running a workflow step in confida-login was
asked to review a UI screenshot for a CSS bug and reported it had no way to
read image bytes from a file path at all — the exact ordinary "look at this
screenshot and tell me what's wrong" task these tools exist for.

The generation tools are genuinely optional, provider-cost-bearing creative
features a workflow-automation platform can reasonably decide not to own.
`read_image` is not that — it is read-only inspection, the basic mechanism
by which an automated agent looks at anything visual at all, and it is
exactly the kind of default capability Cursor, Claude Code, and Codex ship
with. Lumping it into the same removal as `generate_music` conflated two
different things.

Restoring the guidance also surfaced something PLAT-244's removal had
already deleted the record of: `read_image`/`image_gen`/`image_edit` already
supported routing `provider` through a coding-agent CLI's own native vision
(`codex-cli`, `cursor-cli`, `claude-code` — passing the local workspace image
path straight to the CLI) as an alternative to a standalone vision-model API
call. This is the "native tool" capability the operator specifically asked
about; it already existed in this tool's design and was never a separate
build.

## Fix — scope: exactly `read_image`, `image_gen`, `image_edit`

Video/audio/music generation and transcription remain retired; that part of
PLAT-244 stands. Restored, verbatim where the deleted code was still
recoverable from git history:

- `pkg/workspace/advanced_tools.go` — `imageToolDef()`,
  `GetImageToolDefinitions()`, wired back into `GetAdvancedToolDefinitions()`.
- `pkg/workspace/tools.go` — the `read_image` executor
  (`client.ReadImage`), which was never deleted, only unregistered.
- `cmd/server/virtual-tools/workspace_tool_registry.go` —
  `CreateWorkspaceImageTools()` (image_gen + image_edit) back in the tool
  list, `MergeImageToolExecutors` back in the executor merge (this file
  gained an executor-visibility filter since PLAT-244 that did not exist
  before; restoring both the tool-list entry and the executor merge together
  was required for the filter to keep them), and the `workspace_image`
  category back in `initWorkspaceToolNamesCache`.
- `pkg/common/tool_categories.go` and `cmd/server/server.go` — `workspace_image`,
  `workspace_image_gen`, `workspace_image_edit` back in the builtin/MCP-bridge
  category allowlists, so these categories are recognized as selectable again.
- `pkg/orchestrator/base_orchestrator_folder_guard.go` — `read_image`
  reclassified as a read-only tool in both folder-guard classification maps
  (security correctness: an unclassified tool is not automatically
  permissive, but it must be explicitly read-only, not left unclassified).
- `pkg/orchestrator/base_orchestrator_tools.go` — `workspace_image` restored
  to the category-resolution switch.
- `cmd/server/instructions.go` — restored the Image Generation Defaults and
  Image Analysis Defaults sections (trimmed for the current active scope),
  including the native-CLI-provider routing explanation.
- `pkg/orchestrator/agents/workflow/step_based_workflow/interactive_workshop_manager.go` —
  restored `read_image` in the "Shell & discovery" one-liner and the
  `enabled_custom_tools` category description.

mcpagent (the sibling execution-engine repo) needed no changes: its
`ToolFilter.systemCategories` still listed `workspace_image_gen`/
`workspace_image_edit` throughout — PLAT-244's removal only ever touched the
agent_go side.

## Explicitly not done

- Did not restore `generate_video`/`text_to_speech`/`speech_to_text`/
  `generate_music` or any of their guidance — that narrowing stands.
- Did not revert `controller_agent_factory.go`'s legacy-manifest-entry drop
  logic (old `workspace_image*` workflow manifest entries still get dropped
  as no-ops). Functionally harmless either way: these tools are unconditionally
  bundled into `workspace_advanced`'s auto-inclusion, the same reason the
  pre-PLAT-244 code already treated individual `workspace_image:*` manifest
  entries as no-ops.
- Did not investigate whether the "native CLI tool passthrough" alternative
  (letting `codex-cli`/`cursor-cli`/`claude-code`'s own built-in tools
  operate directly instead of routing through this platform's bridge tools)
  is feasible — that would require per-CLI headless-mode research across
  four different third-party products. Restoring the existing, already-built
  provider-routing bridge (which already supports native-CLI routing as one
  of its providers) was the lower-risk, immediately-available fix.

## Verification

- `go build ./...` clean.
- `go test ./pkg/workspace/...`, `./cmd/server/`, `./pkg/orchestrator/`,
  `./pkg/orchestrator/agents/workflow/step_based_workflow/...` all pass.
- `TestCreateWorkspaceToolRegistryIncludesOnlyActiveTextAndSearchTools`
  renamed to `TestCreateWorkspaceToolRegistryIncludesActiveTextSearchAndImageTools`
  and rewritten: asserts `read_image`/`image_gen`/`image_edit` are present
  with executors and correct category, `generate_video`/`text_to_speech`/
  `speech_to_text`/`generate_music` remain absent, and `workspace_image`
  resolves tools again (inverted from PLAT-244's assertion that it resolved
  none).
- Restored `pkg/workspace/read_image_test.go`'s deleted
  `TestLLMBackedToolDefinitionsReferenceCapabilityDiscovery` case.
- `cmd/server/virtual-tools` package could not be test-run directly — it has
  a pre-existing, unrelated compile failure (`generate_text_llm_tool_p0_reviews_test.go`
  imports `github.com/manishiitg/mcpagent/agentreview`, an internal mcpagent
  package that cannot be imported from outside its module; confirmed this
  reproduces identically with this session's own changes fully reverted, so
  it predates this ticket). Verified this package's non-test code compiles
  via the full `go build ./...`, and `gofmt -l` is clean on every changed
  file in that package.

## Reverify

Start a fresh Builder/Workshop or step agent, confirm `read_image`,
`image_gen`, and `image_edit` appear in its tool list, and confirm a
`read_image` call against a real workspace screenshot succeeds end to end
(this was the exact live failure that prompted this ticket).

## Follow-up (2026-08-29) — the native-CLI-passthrough question this ticket
left open, now answered with live verification

"Explicitly not done" above flagged that whether native-CLI passthrough
(`provider=codex-cli`/`cursor-cli`/`claude-code`) actually *works* was
never investigated — restoring the bridge-routing code path was the
lower-risk fix at the time, but nobody had driven it against the real
CLIs. This follow-up did exactly that, live, in the sibling
`multi-llm-provider-go` repo (pulled into `agent_go` via its local
`replace` directive — no version bump needed for these fixes to be live
here).

**How native passthrough actually works** (was undocumented before this):
there is no dedicated function for it, unlike `search_web_llm`'s
`SearchWeb()`. `wrapReadImageWithLLM` / the `image_gen`/`image_edit` tool
wrappers just send a plain-text prompt ("here's a local file path /
attached reference image, do X") through the provider's normal
`GenerateContent()` / `GenerateImages()` call, and rely entirely on that
CLI's own agentic judgment to invoke its own native tool. Verification
had to be per-provider because there is no shared mechanism to reason
about.

**`read_image` native passthrough:**
- `codex-cli`, `claude-code`: verified live, work correctly, including
  under the same lockdown flags a real workflow-step session runs under
  (`WithDisableShellTool()`).
- `cursor-cli`: found a real bug. Under `WithDenyBuiltinTools(true)` (same
  lockdown a real workflow-step session uses), every `read_image` call
  hung for the full 180s context deadline instead of failing or
  succeeding. Root cause: the `beforeReadFile` hook cursor installs to
  force MCP-bridge routing denies *every* file read unconditionally (no
  matcher, unlike the tool-name-matched `preToolUse` deny) — and since the
  MCP bridge has no vision equivalent, the denied agent just flails
  (e.g. trying to `cat` binary PNG bytes through the bridge) until
  timeout instead of getting redirected anywhere useful. Fixed by having
  the deny script allow image-extension (`.png`/`.jpg`/`.jpeg`/`.webp`)
  reads through while leaving every other denial unchanged. Verified live:
  180s timeout → 20-30s pass, lockdown still fully in place.
  (`multi-llm-provider-go` commit `7a0e17d`.)

**`image_gen`/`image_edit` native passthrough (codex-cli only —
cursor-cli/claude-code/agy-cli/vertex not investigated this pass):**
- `image_gen` worked, but ran under
  `--dangerously-bypass-approvals-and-sandbox` (zero lockdown at all,
  independent of whatever session lockdown was requested). Verified live
  that codex's built-in `image_gen` tool needs no approval prompt and
  completes fine under `--sandbox workspace-write --ask-for-approval
  never` instead — real security improvement, no behavior change.
  Deliberately did not also disable `shell_tool`: codex's image skill
  saves the generated file under `$CODEX_HOME/generated_images/` (outside
  any sandbox) and shells out to `cp` it into the workspace path;
  disabling shell would strand the file with no way to relocate it.
- `image_edit` was **completely broken** — every single call failed.
  `runSingleImageCommand` passed `--image <path>` as two separate argv
  entries; codex's `--image` flag is clap-variadic (`<FILE>...`) and
  greedily swallowed the prompt string that followed as another file
  argument, leaving codex with no prompt at all ("No prompt provided via
  stdin", exit 1). Reproduced manually against the real CLI before
  touching code, fixed by passing a single `--image=<path>` token,
  verified live (127s PASS, image content-checked as actually recolored
  per the edit prompt, not just "returned some bytes").
  (`multi-llm-provider-go` commit `1c55f59`.)

Both fixes shipped with new live tests
(`TestCursorCLIRealImagePathAnalysis` pre-existed and now passes;
`TestCodexCLIRealImagePathAnalysis`, `TestClaudeCodeRealImagePathAnalysis`,
`TestCodexCLIRealImageGeneration`, `TestCodexCLIRealImageEditing` are new)
and full-package regression sweeps (0 failures) in
`multi-llm-provider-go`. Not yet done: the same live-verification pass for
`image_gen`/`image_edit` on `cursor-cli`/`claude-code`/`agy-cli`, and for
`read_image` on `agy-cli`.
