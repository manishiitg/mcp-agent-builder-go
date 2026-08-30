#!/bin/sh
set -eu

wrapper_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
browser="$wrapper_dir/chrome-headless-shell"
test -x "$browser"

# HyperFrames already supplies the normal headless/no-sandbox flags. These
# extra switches keep Chromium in one process so it can run inside the
# workspace's intentionally narrow Landlock policy.
exec "$browser" \
  --single-process \
  --disable-crash-reporter \
  --disable-dev-shm-usage \
  --disable-gpu \
  "$@"
