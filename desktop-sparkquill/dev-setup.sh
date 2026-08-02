#!/usr/bin/env bash
# Build everything the Electron shell needs into resources/, then install its
# own deps. Run this once before `npm start`, and again after changing the Go
# server or the frontend.
#
# CI does the same steps inline (see .github/workflows/sparkquill-desktop.yml);
# this exists so a local run is one command and stays in sync with it.
set -euo pipefail

cd "$(dirname "$0")"
ROOT="$(cd .. && pwd)"

echo "==> Building the frontend"
(cd "$ROOT/frontend/learning-app" && npm ci --silent && npm run build)

echo "==> Staging the frontend into resources/web"
rm -rf resources/web
mkdir -p resources/web
cp -R "$ROOT/frontend/learning-app/dist/." resources/web/

echo "==> Building the native voice helper (Swift)"
# Streaming STT — see docs/refactor/native_streaming_stt.md. Optional: the
# server falls back to the Python/MLX path when this binary is absent, so a
# missing Swift toolchain degrades dictation rather than breaking the build.
if command -v swift >/dev/null 2>&1; then
  (cd voice-helper && swift build -c release --arch arm64)
  # --show-bin-path: .build/release is a symlink to the host arch's output.
  cp "$(cd voice-helper && swift build -c release --arch arm64 --show-bin-path)/voice-helper" \
     resources/voice-helper
else
  echo "    swift not found - skipping (dictation will use the slower Python path)"
fi

echo "==> Building family-server"
(cd "$ROOT/agent_go" && go build -o "$OLDPWD/resources/family-server" ./cmd/family-server/)

echo "==> Installing Electron deps"
npm install --silent

echo
echo "Ready. Start it with:  npm start"
