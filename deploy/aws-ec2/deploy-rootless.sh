#!/usr/bin/env bash
set -euo pipefail

AWS_PROFILE_NAME="${AWS_PROFILE_NAME:-RTS}"
AWS_REGION="${AWS_REGION:-us-west-2}"
STACK_NAME="${STACK_NAME:-video-studio-prod}"
SSH_KEY_PATH="${SSH_KEY_PATH:-/Users/mipl/.ssh/id_ed25519}"
GLOBAL_SECRETS_SECRET_ID="${GLOBAL_SECRETS_SECRET_ID:-video-studio/global-secrets}"
REUSE_CURRENT_AGENT="${REUSE_CURRENT_AGENT:-0}"
SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
WORKSPACE_ROOT="$(cd "$REPO_ROOT/.." && pwd)"

aws_rts() { aws --profile "$AWS_PROFILE_NAME" --region "$AWS_REGION" "$@"; }
for command in aws go npm jq rsync ssh; do command -v "$command" >/dev/null || { echo "Missing $command" >&2; exit 1; }; done

HOST_IP="$(aws_rts cloudformation describe-stacks --stack-name "$STACK_NAME" --query 'Stacks[0].Outputs[?OutputKey==`ElasticIp`].OutputValue | [0]' --output text)"
RELEASE_ID="$(git -C "$REPO_ROOT" rev-parse --short HEAD)-$(date +%Y%m%d%H%M%S)"
BUILD_DIR="$(mktemp -d)"
GLOBAL_FILE="$(mktemp)"
trap 'rm -rf "$BUILD_DIR" "$GLOBAL_FILE"' EXIT
mkdir -p "$BUILD_DIR/bin" "$BUILD_DIR/frontend" "$BUILD_DIR/configs"

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

aws_rts secretsmanager get-secret-value --secret-id "$GLOBAL_SECRETS_SECRET_ID" --query SecretString --output text \
  | jq -er 'to_entries[] | select(.key | test("^[A-Z0-9_]+$")) | select(.value | type == "string" and length > 0) | if .key == "CLAUDE_CODE_OAUTH_TOKEN" then "CLAUDE_CODE_OAUTH_TOKEN=\(.value)" else "GLOBAL_SECRET_\(.key)=\(.value)" end' > "$GLOBAL_FILE"

SSH=(ssh -o BatchMode=yes -o StrictHostKeyChecking=accept-new -i "$SSH_KEY_PATH" "video-studio@$HOST_IP")
REMOTE_APP="/var/lib/video-studio/video-studio"
REMOTE_RELEASE="$REMOTE_APP/releases/$RELEASE_ID"
"${SSH[@]}" 'test -x /usr/bin/google-chrome; command -v agent-browser >/dev/null'
rsync -az -e "ssh -o BatchMode=yes -o StrictHostKeyChecking=accept-new -i $SSH_KEY_PATH" "$BUILD_DIR/" "video-studio@$HOST_IP:$REMOTE_RELEASE/"
rsync -az --chmod=ugo=,u=rw -e "ssh -o BatchMode=yes -o StrictHostKeyChecking=accept-new -i $SSH_KEY_PATH" "$GLOBAL_FILE" "video-studio@$HOST_IP:/var/lib/video-studio/video-studio/.globals-$RELEASE_ID"
"${SSH[@]}" "set -e; env_file='$REMOTE_APP/.env'; global_file='$REMOTE_APP/.globals-$RELEASE_ID'; awk '!/^GLOBAL_SECRET_|^CLAUDE_CODE_OAUTH_TOKEN=/' \"\$env_file\" > \"\$env_file.next\"; grep -q '^MCP_API_URL=' \"\$env_file.next\" || echo 'MCP_API_URL=http://127.0.0.1:8000' >> \"\$env_file.next\"; cat \"\$global_file\" >> \"\$env_file.next\"; chmod 600 \"\$env_file.next\"; mv \"\$env_file.next\" \"\$env_file\"; rm -f \"\$global_file\"; ln -sfn '$REMOTE_RELEASE' '$REMOTE_APP/current'; systemctl --user restart video-studio-workspace video-studio-agent video-studio-gateway; systemctl --user is-active video-studio-agent video-studio-workspace video-studio-gateway"

echo "Rootless Video Studio release deployed: https://video.realtrainingsys.com"
