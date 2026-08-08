# Video Studio handover

## Current product

Video Studio is an AgentWorks product profile, not a standalone application.
The supported runtime is the shared AgentWorks server with the Video Studio
profile selected. Its backend entry point is the normal `agent_go/cmd/server`
server; its frontend is the Video Studio product surface inside `frontend/src`.

The old `cmd/video-server`, `frontend/video-app`, and
`scripts/run-video-studio.sh` implementation has been removed. Do not restore
or run them.

## Local development

Run the isolated browser-only instance from the repository root:

```bash
./scripts/run-local-instance.sh \
  --instance video-product-dev \
  --app-name 'Video Studio' \
  --favicon-url /video-studio-favicon.svg \
  --browser-only
```

The current local browser URL is normally `http://127.0.0.1:52733/`.
The script starts an isolated AgentWorks API, workspace service, and Vite
frontend. It does not start Electron or the retired standalone product.

## Product-owned files

| Area | Location |
| --- | --- |
| Product definition | `agent_go/internal/videoproduct/product.yaml` |
| System prompt | `agent_go/internal/videoproduct/prompts/system-prompt.md` |
| Profile registration and bundled skills | `agent_go/internal/videoproduct/profile_definition.go` |
| Product workspace initialization and `show_video` | `agent_go/internal/videoproduct/profile_runtime.go` |
| Workflow definition | `agent_go/internal/videoproduct/workflow_definition.go` and `pipelines.go` |
| Managed external skills | `agent_go/internal/videoproduct/managed_skills.go` |
| Product UI | `frontend/src/products/video-studio/` |

## Runtime behavior

- Video Studio uses structured coding-agent streaming; the UI is non-technical
  and does not expose raw terminal or tmux views.
- Claude Code is the default provider. Codex and Cursor are selectable product
  providers through the profile configuration.
- Native coding-agent tools may be enabled according to the product runtime and
  approval policy. Product MCP tools include workflow controls, skills/browser
  access, `show_video`, and secrets.
- `show_video` requires a passing project-relative `quality-report.json` for
  the exact video presented.
- Video Studio currently focuses on HyperFrames product explainers and
  independent video QA. Cinematic/AI-footage production is not an active route.

## Secrets

Secrets are managed through the AgentWorks secret UI or the registered secret
tools. Structured coding-agent turns receive selected secrets only as
`SECRET_<NAME>` subprocess environment variables. Product tools are delivered
through `get_api_spec`; its HTTP bridge uses `execute_shell_command` inside the
guarded workspace executor. Do not paste a secret in chat: a shell payload can
be logged. Use the Secret UI for entering a value whenever possible.

If a key is ever pasted into chat, treat it as exposed: revoke/rotate it and
avoid copying it into test output or issue descriptions.

## Tool-surface invariant

The product tool surface is decided during registration, before a coding CLI
caches `get_api_spec`. Product capabilities must not be registered in one list
and hidden by a second runtime list. See
`docs/design/agent_tool_surface_single_source.md` and
`docs/design/product_tool_registration_and_visibility.md`.

## Outstanding follow-up

Before the next product restart, finish the final cleanup pass:

1. remove the unused cinematic description map in `pipelines.go`;
2. update the historical migration wording in
   `docs/design/video_studio_inside_agentworks.md` so it no longer presents
   the retired standalone app as current; and
3. run an end-to-end embedded-profile test proving that secret tools appear in
   both the effective coding-agent tool catalog and `get_api_spec`.

The legacy local data directory (for example `~/VideoStudio`) is intentionally
not removed by this code cleanup because it may contain user files.
