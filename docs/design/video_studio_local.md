# Video Studio local application

Video Studio is developed in the separate worktree `mcp-agent-builder-video` on
branch `feature/video-product`. It does not share runtime ports or application
data with AgentWorks.

## Run

```bash
./scripts/run-video-studio.sh
```

Open `http://127.0.0.1:3200` and sign in with:

- username: `manish`
- password: `12345`

The app binds locally only. Its frontend uses port `3200`, its Go API uses port
`8200`, and its internal workspace sidecar uses port `8201`. Its default data
directory is `~/VideoStudio`.

## Current structure

- `frontend/video-app`: project dashboard, chat, assets, videos, workflows, and settings,
  with Zustand owning shared application state from the start.
- `agent_go/cmd/video-server`: standalone server entry point.
- `agent_go/internal/videoproduct`: SQLite data, local auth, workspaces, assets,
  videos, workflow-run progress, encrypted secrets, streaming chat, and the
  Claude-only runner.
- `agent_go/internal/videoproduct/skills`: built-in video creation and quality
  skills attached to every project agent.

Every project owns `uploads/`, `work/`, and `outputs/` folders plus one stable,
resumable agent-session ID and continuation handle. Project membership is in the
schema so shared projects can be enabled later without changing project storage.
The backend always uses Claude Code with `claude-sonnet-5` as its default model.

Each project also gets one reusable cinematic workflow with eight regular
steps: research, proposal, script, scene plan, assets, edit plan, compose, and
quality check. The persistent project chat invokes AgentWorks' existing
`execute_step`, `query_step`, and `run_full_workflow` handlers. Repeated staged
runs with the same video group continue in one workflow row. Every step sets
`learnings_access`, `knowledgebase_access`, and `db_access` to `none`; the
server's own SQLite tables remain available only for product and progress state.

Deferred integrations remain outside this local slice: Slack, WhatsApp, Google
Workspace, AWS deployment, and publishing.
