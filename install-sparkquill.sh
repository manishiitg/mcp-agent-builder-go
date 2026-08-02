#!/usr/bin/env bash
#
# SparkQuill installer.
#
#   curl -fsSL https://raw.githubusercontent.com/manishiitg/coding-agent-loop/main/install-sparkquill.sh | bash
#
# Downloads the latest signed-ad-hoc .dmg from GitHub Releases, installs it to
# /Applications, clears the quarantine flag, and launches it.
#
# Env overrides (used by the in-app updater, and handy for testing):
#   SPARKQUILL_VERSION    install this tag instead of latest, e.g. sparkquill-v0.2.0
#   SPARKQUILL_DMG_PATH   use an already-downloaded dmg instead of fetching one
set -euo pipefail

REPO="manishiitg/coding-agent-loop"
APP_NAME="SparkQuill"
INSTALL_DIR="/Applications"

BOLD=$'\033[1m'; DIM=$'\033[2m'; RED=$'\033[31m'; GREEN=$'\033[32m'; YELLOW=$'\033[33m'; RESET=$'\033[0m'
log()  { printf '%s==>%s %s\n' "$BOLD" "$RESET" "$*"; }
warn() { printf '%s warn%s %s\n' "$YELLOW" "$RESET" "$*" >&2; }
die()  { printf '%serror%s %s\n' "$RED" "$RESET" "$*" >&2; exit 1; }

# --- preflight ---------------------------------------------------------------
[ "$(uname -s)" = "Darwin" ] || die "SparkQuill is macOS-only right now."
[ "$(uname -m)" = "arm64" ] || die "SparkQuill ships for Apple Silicon (arm64) only.
On an Intel Mac, build it yourself: see desktop-sparkquill/README.md."

for tool in curl hdiutil xattr; do
  command -v "$tool" >/dev/null 2>&1 || die "'$tool' is required but not on PATH."
done

# --- resolve the version -----------------------------------------------------
if [ -n "${SPARKQUILL_VERSION:-}" ]; then
  VERSION="$SPARKQUILL_VERSION"
else
  log "Finding the latest release"
  # This repo also ships AgentWorks, which releases far more often — the
  # plain /releases/latest redirect would almost always resolve to ITS latest
  # release, not SparkQuill's. SparkQuill releases are tagged sparkquill-v*,
  # so filter the release list to that prefix and take the newest (the API
  # returns releases newest-first). No jq dependency: a plain grep/sed on the
  # JSON is enough for one field, and this is a one-off, unauthenticated call.
  VERSION="$(curl -fsSL "https://api.github.com/repos/$REPO/releases" \
    | grep -m1 -o '"tag_name": *"sparkquill-v[^"]*"' | sed -E 's/.*"(sparkquill-v[^"]+)"/\1/')"
  [ -n "$VERSION" ] || die "Could not determine the latest version."
fi
log "Installing $APP_NAME $VERSION"

# --- stop a running copy -----------------------------------------------------
if pgrep -fq "${APP_NAME}.app/Contents/MacOS"; then
  log "Quitting the running $APP_NAME"
  osascript -e "tell application \"$APP_NAME\" to quit" >/dev/null 2>&1 || true
  sleep 1
  pkill -f "${APP_NAME}.app" >/dev/null 2>&1 || true
fi

# --- fetch -------------------------------------------------------------------
TMP="$(mktemp -d)"
cleanup() { [ -n "${MOUNT:-}" ] && hdiutil detach -quiet "$MOUNT" >/dev/null 2>&1 || true; rm -rf "$TMP"; }
trap cleanup EXIT

DMG_NAME="${APP_NAME}-${VERSION#sparkquill-v}-arm64.dmg"
DMG="$TMP/$DMG_NAME"

if [ -n "${SPARKQUILL_DMG_PATH:-}" ] && [ -s "${SPARKQUILL_DMG_PATH}" ]; then
  log "Using the already-downloaded installer"
  DMG="$SPARKQUILL_DMG_PATH"
else
  log "Downloading $DMG_NAME"
  curl -fL# -o "$DMG" "https://github.com/$REPO/releases/download/$VERSION/$DMG_NAME" \
    || die "Download failed. Check that $DMG_NAME exists on the $VERSION release."
fi

# --- install -----------------------------------------------------------------
log "Mounting the installer"
MOUNT="$(hdiutil attach -nobrowse -noverify -noautoopen "$DMG" | tail -1 \
  | awk '{ $1=""; $2=""; sub(/^  */, ""); print }')"
[ -n "$MOUNT" ] || die "Could not mount $DMG_NAME."

SRC_APP="$MOUNT/${APP_NAME}.app"
[ -d "$SRC_APP" ] || die "${APP_NAME}.app not found inside the installer."
DEST_APP="$INSTALL_DIR/${APP_NAME}.app"

log "Installing to $DEST_APP"
rm -rf "$DEST_APP"
cp -R "$SRC_APP" "$DEST_APP"

# The build is ad-hoc signed rather than notarized, so Gatekeeper would refuse
# to open it while the download's quarantine flag is set. Remove the flag
# per-file: `xattr -cr` would strip the signing xattrs too, and -r isn't
# available on every macOS version we support.
log "Clearing the download quarantine flag"
find "$DEST_APP" -exec xattr -d com.apple.quarantine {} \; 2>/dev/null || \
  warn "Could not clear quarantine automatically. If macOS refuses to open it, run:
    xattr -cr \"$DEST_APP\""

[ -n "${SPARKQUILL_DMG_PATH:-}" ] && rm -f "$SPARKQUILL_DMG_PATH" 2>/dev/null || true

printf '\n%s%s %s installed.%s\n' "$GREEN" "✓" "$APP_NAME $VERSION" "$RESET"
printf '%sYour family'"'"'s learning data lives in ~/.sunlit-learning%s\n\n' "$DIM" "$RESET"
log "Opening $APP_NAME"
open "$DEST_APP"
