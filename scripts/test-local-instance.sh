#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
LAUNCHER="${SCRIPT_DIR}/run-local-instance.sh"
FAKE_RUNNER="${SCRIPT_DIR}/testdata/fake-agentworks-runner.sh"
TEST_ROOT="$(mktemp -d "${TMPDIR:-/tmp}/agentworks-local-instance-test.XXXXXX")"
TEST_ROOT="$(cd "$TEST_ROOT" && pwd)"

cleanup() {
    rm -rf "$TEST_ROOT"
}
trap cleanup EXIT

fail() {
    echo "FAIL: $*" >&2
    exit 1
}

assert_contains() {
    local output="$1"
    local expected="$2"
    [[ "$output" == *"$expected"* ]] || fail "expected output to contain: $expected"
}

DRY_ROOT="${TEST_ROOT}/dry-run"
dry_output="$($LAUNCHER \
    --instance video-product-test \
    --state-root "$DRY_ROOT" \
    --agent-port 29743 \
    --workspace-port 29744 \
    --frontend-port 52743 \
    --electron-debug-port 29233 \
    --app-name "Video Studio (Test)" \
    --favicon-url /video-studio-favicon.svg \
    --dry-run)"

assert_contains "$dry_output" "AGENTWORKS_INSTANCE_ID=video-product-test"
assert_contains "$dry_output" "RUNLOOP_USER_DATA_DIR=${DRY_ROOT}/electron"
assert_contains "$dry_output" "WORKSPACE_DOCS_PATH=${DRY_ROOT}/workspace-docs"
assert_contains "$dry_output" "AGENTWORKS_BROWSER_SESSION_PREFIX=video-product-test"
assert_contains "$dry_output" "AGENTWORKS_APP_NAME=Video Studio (Test)"
assert_contains "$dry_output" "AGENTWORKS_FAVICON_URL=/video-studio-favicon.svg"
assert_contains "$dry_output" "BROWSER_ONLY=true"
[ -f "${DRY_ROOT}/browser/config.json" ] || fail "browser config was not created"
[ ! -d "${DRY_ROOT}/instance.lock" ] || fail "dry run left an instance lock"

if "$LAUNCHER" --instance Invalid_Name --state-root "${TEST_ROOT}/invalid" --dry-run >/dev/null 2>&1; then
    fail "invalid instance name was accepted"
fi
if "$LAUNCHER" --instance valid --state-root relative/path --dry-run >/dev/null 2>&1; then
    fail "relative state root was accepted"
fi
if "$LAUNCHER" --instance valid --state-root "${TEST_ROOT}/ports" --agent-port 30000 --workspace-port 30000 --dry-run >/dev/null 2>&1; then
    fail "duplicate ports were accepted"
fi

LIVE_ROOT="${TEST_ROOT}/live"
live_output="$(AGENTWORKS_RUNNER="$FAKE_RUNNER" "$LAUNCHER" \
    --instance video-product-test \
    --state-root "$LIVE_ROOT" \
    --agent-port 30743 \
    --workspace-port 30744 \
    --frontend-port 53733 \
    --electron-debug-port 30233 \
    --app-name "Video Studio (Test)" \
    --favicon-url /video-studio-favicon.svg \
    --build)"
assert_contains "$live_output" "fake isolated runner passed"
[ ! -d "${LIVE_ROOT}/instance.lock" ] || fail "completed run left an instance lock"

ELECTRON_ROOT="${TEST_ROOT}/electron-opt-in"
electron_output="$(AGENTWORKS_RUNNER="$FAKE_RUNNER" "$LAUNCHER" \
    --instance video-product-electron-test \
    --state-root "$ELECTRON_ROOT" \
    --agent-port 31743 \
    --workspace-port 31744 \
    --frontend-port 54733 \
    --electron-debug-port 31233 \
    --electron)"
assert_contains "$electron_output" "fake isolated runner passed"
[ ! -d "${ELECTRON_ROOT}/instance.lock" ] || fail "Electron opt-in run left an instance lock"

echo "PASS: isolated local instance launcher"
