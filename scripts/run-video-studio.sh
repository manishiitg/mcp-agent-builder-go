#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_dir="$(cd "$script_dir/.." && pwd)"
video_data_dir="${VIDEO_STUDIO_DATA_DIR:-$HOME/VideoStudio}"
export VIDEO_STUDIO_DATA_DIR="$video_data_dir"
export VIDEO_STUDIO_WORKSPACE_API_URL="${VIDEO_STUDIO_WORKSPACE_API_URL:-http://127.0.0.1:8201}"
export WORKSPACE_API_URL="$VIDEO_STUDIO_WORKSPACE_API_URL"
export WORKSPACE_DOCS_PATH="$video_data_dir"

(
  cd "$repo_dir/workspace"
  GOWORK=off go run . server --port 8201 --host 127.0.0.1 --docs-dir "$video_data_dir"
) &
workspace_server_pid=$!

(
  cd "$repo_dir/agent_go"
  GOWORK=off go run ./cmd/video-server
) &
video_server_pid=$!

cleanup() {
  kill "$video_server_pid" 2>/dev/null || true
  kill "$workspace_server_pid" 2>/dev/null || true
}
trap cleanup EXIT INT TERM

cd "$repo_dir/frontend/video-app"
npm run dev
