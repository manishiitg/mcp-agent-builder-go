#!/usr/bin/env bash
# One-time switch of a Video Studio box from the shared gateway password to
# per-user accounts (docs/design/user_accounts_and_workflow_sharing.md, phase 2).
# Run ON the box as the video-studio user, BEFORE deploying the release that
# carries the user directory; the deploy's restart picks the new env up.
#
#   bash migrate-to-user-accounts.sh <admin-username> [initial-password]
#
# What it does, idempotently:
#   1. .env: MULTI_USER_MODE=true, ADMIN_USERS=<admin>, AUTH_USERS=<admin>:<pw>
#      (bootstrap only — the agent imports it hashed into config/users.json;
#      remove AUTH_USERS afterwards), GATEWAY_DISABLE_PASSWORD_GATE=true.
#      The initial password defaults to the current ACCESS_PASSWORD so the
#      admin's first login uses the password they already know.
#   2. Moves everything that lived under the gateway's single identity
#      (_users/default: projects, chat history, secrets, memories) to the
#      admin's own user id, and rewrites the stored paths inside its JSON.
set -euo pipefail
ADMIN="${1:?admin username}"
ENV_FILE="${ENV_FILE:-$HOME/video-studio/.env}"
DOCS="${DOCS:-/data/video-studio/docs}"
ADMIN_ID="$(printf 'user:%s' "$ADMIN" | sha256sum | cut -c1-32)"
PW="${2:-$(sed -n 's/^ACCESS_PASSWORD=//p' "$ENV_FILE" | head -n1 | tr -d '"')}"
test -n "$PW" || { echo "no initial password (pass one, or set ACCESS_PASSWORD)" >&2; exit 1; }

set_env() { # key value
  local tmp; tmp="$(mktemp)"
  awk -v k="$1" '!(index($0, k"=")==1)' "$ENV_FILE" > "$tmp"
  printf '%s=%s\n' "$1" "$2" >> "$tmp"
  chmod 600 "$tmp"; mv "$tmp" "$ENV_FILE"
}
set_env MULTI_USER_MODE true
set_env ADMIN_USERS "$ADMIN"
set_env AUTH_USERS "$ADMIN:$PW"
set_env GATEWAY_DISABLE_PASSWORD_GATE true
echo "env: MULTI_USER_MODE=true ADMIN_USERS=$ADMIN GATEWAY_DISABLE_PASSWORD_GATE=true (AUTH_USERS set for bootstrap)"

SRC="$DOCS/_users/default"
DST="$DOCS/_users/$ADMIN_ID"
if [ -d "$SRC" ] && [ ! -e "$DST" ]; then
  mv "$SRC" "$DST"
  # Stored workspace paths (conversation registry, chat index, sessions).
  grep -rl --include='*.json' "_users/default" "$DST" | while IFS= read -r f; do
    sed -i "s#_users/default#_users/$ADMIN_ID#g" "$f"
  done
  echo "moved $SRC -> $DST and rewrote stored paths"
elif [ -e "$DST" ]; then
  echo "already migrated: $DST exists"
else
  echo "nothing to migrate: $SRC absent"
fi
echo "admin user id: $ADMIN_ID"
