# Video Studio frontend

Standalone, chat-first video product UI. It runs independently from the
AgentWorks frontend and connects to the dedicated local backend.

## Run locally

From `frontend/video-app`:

```bash
npm run dev
```

Start the backend from `agent_go`:

```bash
GOWORK=off go run ./cmd/video-server
```

Then open `http://127.0.0.1:3200`. The backend runs on `127.0.0.1:8200`.

## Current UI slice

- simple local username/password entry screen;
- project dashboard;
- one continuous Claude Code conversation per project;
- project assets and generated-video panels;
- SQLite project creation and persistence;
- encrypted local secrets manager and isolated ports;
- Zustand as the central frontend app-state layer.
