[← Pulse platform index](../pulse_platform_issue_register.md)

# PLAT-244 — Retired provider-media tools could still be reintroduced through legacy workspace registries

| Coordination | Value |
|---|---|
| Assigned agent | Codex |
| Ticket state | `implemented; runtime reverify` |
| Last synchronized | `2026-08-29` |

- **Priority:** harness_issue, severity medium.
- **Findings:** No workflow finding is linked yet. This is a platform-surface
  defect discovered while narrowing the supported provider-backed tools to
  `generate_text_llm` and `search_web_llm`.

## Root cause

The primary workspace tool registry had stopped exposing provider media tools,
but several independent legacy paths could still reintroduce them:

- `pkg/workspace/advanced_tools.go` retained a `read_image` definition;
- `pkg/workspace/tools.go` retained a `read_image` executor;
- `workspace_image`, `workspace_image_gen`, and `workspace_image_edit`
  remained selectable built-in categories; and
- old workflow manifests containing a `workspace_image*` entry were migrated
  in a way that still auto-enabled the broad workspace tool category.

This split the apparent tool surface from the actual selectable surface. It
also left stale provider-media guidance in active builder instructions.

## Fix

Removed the legacy shared-agent image definition, executor, and media category
registrations; category expansion no longer recognizes `workspace_image`; and
old `workspace_image*` workflow entries are explicitly dropped as retired.
The active shared-agent provider surface is now limited to
`generate_text_llm` and `search_web_llm` (alongside shell and diff tools).
Active guidance no longer presents media configuration as callable.

This deliberately does **not** remove Family Server's independent uploaded
image-reading capability or the separate Video product. Those are distinct
product features, not routes in the shared MCP-agent workspace registry.

## Verification

- `TestCreateWorkspaceToolRegistryIncludesOnlyActiveTextAndSearchTools`
  asserts that every media tool is absent from definitions and executors and
  that `workspace_image` resolves no tools.
- Focused `go test` passed for `cmd/server/virtual-tools`, `pkg/workspace`,
  and `pkg/orchestrator`.
- Focused `golangci-lint` passed for the same packages.

## Reverify

Start a fresh Builder/Workshop agent with both a current and a legacy workflow
manifest. Confirm its MCP tool list contains neither `read_image` nor any
image/video/audio/music tool, and that a legacy `workspace_image*` selection
cannot cause a media provider credential/setup prompt.
