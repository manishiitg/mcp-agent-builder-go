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
