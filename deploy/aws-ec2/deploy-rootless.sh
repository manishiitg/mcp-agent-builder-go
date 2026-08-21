#!/usr/bin/env bash
set -euo pipefail

AWS_PROFILE_NAME="${AWS_PROFILE_NAME:-RTS}"
AWS_REGION="${AWS_REGION:-us-west-2}"
STACK_NAME="${STACK_NAME:-video-studio-prod}"
SSH_KEY_PATH="${SSH_KEY_PATH:-/Users/mipl/.ssh/id_ed25519}"
GLOBAL_SECRETS_SECRET_ID="${GLOBAL_SECRETS_SECRET_ID:-video-studio/global-secrets}"
REUSE_CURRENT_AGENT="${REUSE_CURRENT_AGENT:-0}"
HYPERFRAMES_VERSION="${HYPERFRAMES_VERSION:-0.8.6}"
SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
WORKSPACE_ROOT="$(cd "$REPO_ROOT/.." && pwd)"

# This host has one fixed deployment contract: AgentWorks supplies the shared
# application shell and Video Studio is the only product backend. Fail before
# building or touching the server if either checked-in allowlist drifts.
grep -Fq 'enabledProductSurfaces: ["agentworks", "video-studio"]' "$SCRIPT_DIR/server/runtime-config.js" || {
  echo "Video Studio deployment must expose exactly AgentWorks and Video Studio" >&2
  exit 1
}
grep -Fq 'Environment=AGENT_PRODUCTS=video-studio' "$SCRIPT_DIR/rootless/video-studio-agent.service" || {
  echo "Video Studio deployment must load only the video-studio product backend" >&2
  exit 1
}

aws_rts() { aws --profile "$AWS_PROFILE_NAME" --region "$AWS_REGION" "$@"; }
for command in aws go npm jq rsync ssh; do command -v "$command" >/dev/null || { echo "Missing $command" >&2; exit 1; }; done

HOST_IP="$(aws_rts cloudformation describe-stacks --stack-name "$STACK_NAME" --query 'Stacks[0].Outputs[?OutputKey==`ElasticIp`].OutputValue | [0]' --output text)"
RELEASE_ID="$(git -C "$REPO_ROOT" rev-parse --short HEAD)-$(date +%Y%m%d%H%M%S)"
BUILD_DIR="$(mktemp -d)"
GLOBAL_FILE="$(mktemp)"
trap 'rm -rf "$BUILD_DIR" "$GLOBAL_FILE"' EXIT
mkdir -p "$BUILD_DIR/bin" "$BUILD_DIR/frontend" "$BUILD_DIR/configs" "$BUILD_DIR/systemd" "$BUILD_DIR/claude-skills" "$BUILD_DIR/browser"

if [[ "$REUSE_CURRENT_AGENT" == "1" ]]; then
  rsync -az -e "ssh -o BatchMode=yes -o StrictHostKeyChecking=accept-new -i $SSH_KEY_PATH" \
    "video-studio@$HOST_IP:/var/lib/video-studio/video-studio/current/bin/video-studio-agent" \
    "$BUILD_DIR/bin/video-studio-agent"
else
  (cd "$WORKSPACE_ROOT" && GOWORK="$WORKSPACE_ROOT/go.work" GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o "$BUILD_DIR/bin/video-studio-agent" ./mcp-agent-builder-go/agent_go)
fi
(cd "$WORKSPACE_ROOT" && GOWORK="$WORKSPACE_ROOT/go.work" GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o "$BUILD_DIR/bin/video-studio-workspace" ./mcp-agent-builder-go/workspace)
(cd "$WORKSPACE_ROOT" && GOWORK="$WORKSPACE_ROOT/go.work" GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o "$BUILD_DIR/bin/video-studio-landlock-runner" ./mcp-agent-builder-go/workspace/cmd/landlock-runner)
(cd "$WORKSPACE_ROOT" && GOWORK="$WORKSPACE_ROOT/go.work" GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o "$BUILD_DIR/bin/mcpbridge" ./mcpagent/cmd/mcpbridge)
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o "$BUILD_DIR/bin/video-studio-gateway" "$SCRIPT_DIR/server/auth-gateway.go"
(cd "$REPO_ROOT/frontend" && VITE_API_BASE_URL='' VITE_WORKSPACE_API_URL=/api/wp npm run build)
cp -R "$REPO_ROOT/frontend/dist/." "$BUILD_DIR/frontend/"
install -m 0644 "$SCRIPT_DIR/server/runtime-config.js" "$BUILD_DIR/frontend/runtime-config.js"
install -m 0644 "$SCRIPT_DIR/server/mcp_servers_video_studio.json" "$BUILD_DIR/configs/mcp_servers_video_studio.json"
install -m 0755 "$SCRIPT_DIR/server/chrome-headless-wrapper.sh" "$BUILD_DIR/browser/agentworks-chrome-headless"
install -m 0644 "$SCRIPT_DIR/rootless/video-studio-workspace.service" "$BUILD_DIR/systemd/video-studio-workspace.service"
install -m 0644 "$SCRIPT_DIR/rootless/video-studio-agent.service" "$BUILD_DIR/systemd/video-studio-agent.service"
install -m 0644 "$SCRIPT_DIR/rootless/video-studio-gateway.service" "$BUILD_DIR/systemd/video-studio-gateway.service"
cp -R "$REPO_ROOT/agent_go/internal/videoproduct/skills/." "$BUILD_DIR/claude-skills/"

aws_rts secretsmanager get-secret-value --secret-id "$GLOBAL_SECRETS_SECRET_ID" --query SecretString --output text \
  | jq -er 'to_entries[] | select(.key | test("^[A-Z0-9_]+$")) | select(.value | type == "string" and length > 0) | if .key == "CLAUDE_CODE_OAUTH_TOKEN" then "CLAUDE_CODE_OAUTH_TOKEN=\(.value)" else "GLOBAL_SECRET_\(.key)=\(.value)" end' > "$GLOBAL_FILE"

SSH=(ssh -o BatchMode=yes -o StrictHostKeyChecking=accept-new -i "$SSH_KEY_PATH" "video-studio@$HOST_IP")
REMOTE_APP="/var/lib/video-studio/video-studio"
REMOTE_RELEASE="$REMOTE_APP/releases/$RELEASE_ID"
REMOTE_BROWSER_PATH="$("${SSH[@]}" "set -e; export HOME=/var/lib/video-studio; npx --yes 'hyperframes@$HYPERFRAMES_VERSION' browser ensure >/dev/null; npx --yes 'hyperframes@$HYPERFRAMES_VERSION' browser path | tail -n 1")"
case "$REMOTE_BROWSER_PATH" in
  /var/lib/video-studio/.cache/hyperframes/chrome/*/chrome-headless-shell) ;;
  *) echo "Unexpected HyperFrames browser path: $REMOTE_BROWSER_PATH" >&2; exit 1 ;;
esac
"${SSH[@]}" "test -x '$REMOTE_BROWSER_PATH'; command -v agent-browser >/dev/null"
rsync -az -e "ssh -o BatchMode=yes -o StrictHostKeyChecking=accept-new -i $SSH_KEY_PATH" "$BUILD_DIR/" "video-studio@$HOST_IP:$REMOTE_RELEASE/"
rsync -az --chmod=ugo=,u=rw -e "ssh -o BatchMode=yes -o StrictHostKeyChecking=accept-new -i $SSH_KEY_PATH" "$GLOBAL_FILE" "video-studio@$HOST_IP:/var/lib/video-studio/video-studio/.globals-$RELEASE_ID"
"${SSH[@]}" "set -e; browser_dir='$(dirname "$REMOTE_BROWSER_PATH")'; browser_wrapper=\"\$browser_dir/agentworks-chrome-headless\"; install -m 0755 '$REMOTE_RELEASE/browser/agentworks-chrome-headless' \"\$browser_wrapper\"; env_file='$REMOTE_APP/.env'; global_file='$REMOTE_APP/.globals-$RELEASE_ID'; awk '!/^GLOBAL_SECRET_|^CLAUDE_CODE_OAUTH_TOKEN=|^AGENT_BROWSER_EXECUTABLE_PATH=/' \"\$env_file\" > \"\$env_file.next\"; echo \"AGENT_BROWSER_EXECUTABLE_PATH=\$browser_wrapper\" >> \"\$env_file.next\"; grep -q '^MCP_API_URL=' \"\$env_file.next\" || echo 'MCP_API_URL=http://127.0.0.1:8000' >> \"\$env_file.next\"; cat \"\$global_file\" >> \"\$env_file.next\"; chmod 600 \"\$env_file.next\"; mv \"\$env_file.next\" \"\$env_file\"; rm -f \"\$global_file\"; find /data/video-studio/docs/_users -type d -path '*/Chats/Video Studio/projects' -print0 | while IFS= read -r -d '' projects_root; do find \"\$projects_root\" -mindepth 1 -maxdepth 1 -type d -print0 | while IFS= read -r -d '' project; do install -d -m 0755 \"\$project/.claude/skills\"; rsync -a --delete '$REMOTE_RELEASE/claude-skills/' \"\$project/.claude/skills/\"; rm -rf \"\$project/skills/video-studio\"; done; done; install -d -m 0755 \"\$HOME/.config/systemd/user\"; install -m 0644 '$REMOTE_RELEASE/systemd/video-studio-workspace.service' \"\$HOME/.config/systemd/user/video-studio-workspace.service\"; install -m 0644 '$REMOTE_RELEASE/systemd/video-studio-agent.service' \"\$HOME/.config/systemd/user/video-studio-agent.service\"; install -m 0644 '$REMOTE_RELEASE/systemd/video-studio-gateway.service' \"\$HOME/.config/systemd/user/video-studio-gateway.service\"; systemctl --user daemon-reload; ln -sfn '$REMOTE_RELEASE' '$REMOTE_APP/current'; systemctl --user restart video-studio-workspace video-studio-agent video-studio-gateway; systemctl --user is-active video-studio-agent video-studio-workspace video-studio-gateway"

echo "Rootless Video Studio release deployed: https://video.realtrainingsys.com"
