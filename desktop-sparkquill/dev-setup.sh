#!/usr/bin/env bash
# Build everything the Electron shell needs into resources/, then install its
# own deps. Run this once before `npm start`, and again after changing the Go
# servers or the frontend.
#
# M1 of docs/design/sparkquill_desktop_on_platform_plan.md: this now builds
# the same two platform binaries the AgentWorks desktop builds
# (workspace-server + agent-server) instead of the standalone family-server,
# and stages the existing learning-app dist as resources/static (the agent
# server's `./static/` mount, server.go:2533) instead of resources/web.
#
# Voice (agent_go/pkg/voicestt) needs cgo + the shared engine's dylibs, hence
# build-darwin-voice-binary.sh rather than a plain `go build` — see that
# script's header. There is no equivalent step for workspace-server; it has
# no cgo dependency.
set -euo pipefail

cd "$(dirname "$0")"
ROOT="$(cd .. && pwd)"

echo "==> Building the frontend"
(cd "$ROOT/frontend/learning-app" && npm ci --silent && npm run build)

echo "==> Staging the frontend into resources/static"
rm -rf resources/static
mkdir -p resources/static
cp -R "$ROOT/frontend/learning-app/dist/." resources/static/

echo "==> Staging the default MCP config into resources/configs"
mkdir -p resources/configs
cp "$ROOT/agent_go/configs/mcp_servers_clean.json" resources/configs/mcp_servers_clean.json

echo "==> Building agent-server (with the shared speech engine)"
bash "$ROOT/scripts/build-darwin-voice-binary.sh" "$ROOT/agent_go" resources/agent-server

echo "==> Building workspace-server"
RESOURCES_DIR="$(pwd)/resources"
(cd "$ROOT/workspace" && go build -o "$RESOURCES_DIR/workspace-server" .)

echo "==> Installing Electron deps"
npm install --silent

echo
echo "Ready. Start it with:  npm start"
