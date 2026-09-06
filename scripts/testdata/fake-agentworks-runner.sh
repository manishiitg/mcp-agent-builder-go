#!/usr/bin/env bash

set -euo pipefail

case "$*" in
    "--with-workspace --with-frontend --without-electron --build"|\
    "--with-workspace --with-frontend") ;;
    *)
        echo "unexpected runner arguments: $*" >&2
        exit 1
        ;;
esac

required_variables=(
    AGENTWORKS_INSTANCE_ID
    CLI_UPDATE_ENABLED
    AGENTWORKS_ISOLATE_WORKFLOW_CLI
    AGENTWORKS_STATE_ROOT
    AGENTWORKS_ENV_FILE
    AGENTWORKS_RUNTIME_CONFIG_PATH
    AGENTWORKS_APP_NAME
    AGENTWORKS_FAVICON_URL
    AGENTWORKS_LOG_DIR
    AGENTWORKS_BROWSER_SESSION_PREFIX
    AGENTWORKS_ELECTRON_RSS_LIMIT_MB
    AGENT_BROWSER_CONFIG
    AGENT_PORT
    WORKSPACE_PORT
    FRONTEND_PORT
    ELECTRON_REMOTE_DEBUG_PORT
    RUNLOOP_USER_DATA_DIR
    RUNLOOP_DOCS_DIR
    WORKSPACE_DOCS_PATH
    MCP_CACHE_DIR
    TMUX_TMPDIR
    GOBIN
)

for variable_name in "${required_variables[@]}"; do
    if [ -z "${!variable_name:-}" ]; then
        echo "missing isolated runner variable: $variable_name" >&2
        exit 1
    fi
done

if [ "${AGENTWORKS_STRICT_PROCESS_OWNERSHIP:-}" != "true" ] || \
   [ "${AGENTWORKS_SKIP_GLOBAL_BROWSER_CLEANUP:-}" != "true" ] || \
   [ "${AGENTWORKS_SKIP_GLOBAL_DEPENDENCY_UPDATES:-}" != "true" ] || \
   [ "${AGENTWORKS_ELECTRON_RSS_LIMIT_MB:-}" != "3072" ]; then
    echo "isolated safety flags were not enabled" >&2
    exit 1
fi

if [ "$RUNLOOP_DOCS_DIR" != "$WORKSPACE_DOCS_PATH" ]; then
    echo "desktop and workspace services received different document roots" >&2
    exit 1
fi

for directory in "$RUNLOOP_USER_DATA_DIR" "$WORKSPACE_DOCS_PATH" "$AGENTWORKS_LOG_DIR" "$MCP_CACHE_DIR" "$TMUX_TMPDIR" "$GOBIN"; do
    if [ ! -d "$directory" ]; then
        echo "isolated directory was not created: $directory" >&2
        exit 1
    fi
done

echo "AGENTWORKS_ISOLATE_WORKFLOW_CLI=${AGENTWORKS_ISOLATE_WORKFLOW_CLI}"
echo "CLI_UPDATE_ENABLED=${CLI_UPDATE_ENABLED}"
echo "fake isolated runner passed"
