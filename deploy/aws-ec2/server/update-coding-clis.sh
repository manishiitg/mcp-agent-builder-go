#!/usr/bin/env bash
# Keep the coding-agent CLIs the platform shells out to (Claude Code, Cursor,
# Codex, Pi) current, on a schedule, independently of releases.
#
# Runs as the service user from a systemd user timer (see
# rootless/video-studio-cli-update.timer). For every CLI that is installed it
# records the version, upgrades, re-checks the version and runs a smoke test
# (a real round trip where a credential is available); a CLI that fails its
# smoke test is rolled back to the previous version. Nothing is installed that
# was not already there -- which CLIs a box has is a deployment decision.
#
# Skips entirely while any platform agent session (tmux "mlp-*") is running:
# an upgrade under a live session is the one thing that can break a turn.
#
# Environment (from the unit / .env):
#   CLI_UPDATE_ENABLED   "false" disables the run (default: enabled)
#   CLI_UPDATE_DRY_RUN   "true" only reports versions and what would happen
#   TOOLS_PREFIX         npm prefix holding the CLIs (default: $HOME/.local)
#   CLAUDE_CODE_OAUTH_TOKEN, CURSOR_API_KEY  used for the smoke round trips
set -euo pipefail

TOOLS_PREFIX="${TOOLS_PREFIX:-$HOME/.local}"
DRY_RUN="${CLI_UPDATE_DRY_RUN:-false}"
LOCK_DIR="${XDG_RUNTIME_DIR:-/tmp}/update-coding-clis.lock"
export PATH="$TOOLS_PREFIX/bin:/usr/local/bin:/usr/bin:/bin"
export XDG_CONFIG_HOME="${XDG_CONFIG_HOME:-$HOME/.local/state/xdg-config}"
install -d -m 0700 "$XDG_CONFIG_HOME"

log() { printf '%s %s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "$*"; }

if [[ "${CLI_UPDATE_ENABLED:-true}" == "false" ]]; then
  log "disabled (CLI_UPDATE_ENABLED=false)"
  exit 0
fi
if ! mkdir "$LOCK_DIR" 2>/dev/null; then
  log "another update run is in progress; exiting"
  exit 0
fi
trap 'rmdir "$LOCK_DIR" 2>/dev/null || true' EXIT

active="$(tmux list-sessions -F '#S' 2>/dev/null | grep -c '^mlp-' || true)"
if [[ "${active:-0}" -gt 0 ]]; then
  log "skipping: $active platform agent session(s) active"
  exit 0
fi

summary=()

npm_version() { npm ls -g --prefix "$TOOLS_PREFIX" --depth=0 --json 2>/dev/null | jq -r --arg p "$1" '.dependencies[$p].version // empty'; }

# update_npm <package> <command> <smoke-command...>
update_npm() {
  local pkg="$1" cmd="$2"; shift 2
  local old new
  old="$(npm_version "$pkg")"
  if [[ -z "$old" ]]; then
    return 0
  fi
  if [[ "$DRY_RUN" == "true" ]]; then
    log "$pkg $old installed; would run: npm install -g --prefix $TOOLS_PREFIX $pkg@latest"
    summary+=("$cmd: $old (dry run)")
    return 0
  fi
  if ! npm install -g --prefix "$TOOLS_PREFIX" "$pkg@latest" >/dev/null 2>&1; then
    log "$pkg: npm install failed; keeping $old"
    summary+=("$cmd: install failed, kept $old")
    return 0
  fi
  new="$(npm_version "$pkg")"
  if "$@" >/dev/null 2>&1; then
    if [[ "$new" == "$old" ]]; then summary+=("$cmd: unchanged $old"); else summary+=("$cmd: $old -> $new"); fi
    return 0
  fi
  log "$cmd $new failed its smoke test; rolling back to $old"
  npm install -g --prefix "$TOOLS_PREFIX" "$pkg@$old" >/dev/null 2>&1 || log "$cmd: rollback to $old FAILED"
  summary+=("$cmd: $new failed smoke test, rolled back to $old")
}

claude_smoke() {
  claude --version >/dev/null || return 1
  if [[ -n "${CLAUDE_CODE_OAUTH_TOKEN:-}" ]]; then
    local reply
    reply="$(timeout 120 claude -p --output-format json <<< 'say OK' 2>/dev/null || true)"
    if printf '%s' "$reply" | jq -e '.is_error == false' >/dev/null 2>&1; then return 0; fi
    # A capped account is a valid install; only a non-answer is a failure.
    printf '%s' "$reply" | jq -r '.result // empty' 2>/dev/null | grep -qiE 'session limit|usage limit|rate limit|capacity' && return 0
    return 1
  fi
  return 0
}

update_cursor() {
  command -v cursor-agent >/dev/null 2>&1 || return 0
  local link old_target old new
  link="$TOOLS_PREFIX/bin/cursor-agent"
  old_target="$(readlink -f "$link" 2>/dev/null || true)"
  old="$(cursor-agent --version 2>/dev/null | head -n1 || true)"
  if [[ "$DRY_RUN" == "true" ]]; then
    log "cursor-agent $old installed; would run the cursor.com installer"
    summary+=("cursor-agent: $old (dry run)")
    return 0
  fi
  if ! curl -fsS https://cursor.com/install | bash >/dev/null 2>&1; then
    log "cursor-agent: installer failed; keeping $old"
    summary+=("cursor-agent: install failed, kept $old")
    return 0
  fi
  new="$(cursor-agent --version 2>/dev/null | head -n1 || true)"
  local ok=1
  if [[ -n "${CURSOR_API_KEY:-}" ]]; then
    if timeout 120 cursor-agent -p --trust "say OK" >/dev/null 2>&1; then ok=0; fi
  else
    cursor-agent --version >/dev/null 2>&1 && ok=0
  fi
  if [[ $ok -eq 0 ]]; then
    if [[ "$new" == "$old" ]]; then summary+=("cursor-agent: unchanged $old"); else summary+=("cursor-agent: $old -> $new"); fi
    return 0
  fi
  log "cursor-agent $new failed its smoke test; rolling back to $old"
  if [[ -n "$old_target" && -x "$old_target" ]]; then
    ln -sfn "$old_target" "$link"
    summary+=("cursor-agent: $new failed smoke test, rolled back to $old")
  else
    summary+=("cursor-agent: $new failed smoke test and no previous binary to roll back to")
  fi
}

log "start (dry_run=$DRY_RUN prefix=$TOOLS_PREFIX)"
update_npm "@anthropic-ai/claude-code" claude claude_smoke
update_npm "@openai/codex" codex codex --version
update_npm "@earendil-works/pi-coding-agent" pi pi --version
update_cursor
if [[ ${#summary[@]} -gt 0 ]]; then
  for line in "${summary[@]}"; do log "$line"; done
else
  log "no coding-agent CLIs installed under $TOOLS_PREFIX"
fi
log "done"
