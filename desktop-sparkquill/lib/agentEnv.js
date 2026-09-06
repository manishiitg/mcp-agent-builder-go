'use strict'

// Builds the agent-server spawn env. Pulled out of main.js so it can be unit
// tested without Electron (see agentEnv.test.js, run with `node --test lib/`).
//
// The one property that must never regress here: this function must never
// introduce AGENT_PRODUCTS or MULTI_USER_MODE. Setting AGENT_PRODUCTS to
// "sparkquill" alone would make isSingleProductServerDeployment() true on the
// server and 400 every Claude Code turn with no stored OAuth token — see A3 in
// docs/design/sparkquill_desktop_on_platform_plan.md. The desktop case must
// fall back to the user's locally logged-in CLI, like AgentWorks' desktop does.
function buildAgentServerEnv(baseEnv, { authSecret, workspacePort, docsDir, logFile, workspaceApiToken }) {
  return {
    ...baseEnv,
    AUTH_SECRET: authSecret,
    WORKSPACE_API_URL: `http://127.0.0.1:${workspacePort}`,
    WORKSPACE_DOCS_PATH: docsDir,
    DOCS_DIR: docsDir,
    LOG_FILE: logFile,
    // workspace-server and the agent server both execute host shell commands
    // (/api/execute, MCP tool calls). Mark them native so the safe shell env
    // preserves the imported login-shell PATH/HOME instead of the
    // Docker-style minimal PATH.
    NATIVE_WORKSPACE: 'true',
    WORKSPACE_API_TOKEN: workspaceApiToken,
    // Two AgentWorks-family desktops running at once must not fight over
    // browser cleanup/kill-all or a shared session-prefix namespace.
    AGENTWORKS_SKIP_GLOBAL_BROWSER_CLEANUP: 'true',
    AGENTWORKS_BROWSER_SESSION_PREFIX: 'sparkquill',
    // One frontend build serves every product; these pin it to SparkQuill
    // (runtime_frontend_config.go emits them into runtime-config.js, which the
    // frontend reads before it mounts): only the SparkQuill surface exists
    // here, it is the default, and the window carries SparkQuill's name and mark.
    AGENTWORKS_ENABLED_PRODUCT_SURFACES: 'sparkquill',
    AGENTWORKS_DEFAULT_PRODUCT_SURFACE: 'sparkquill',
    AGENTWORKS_APP_NAME: 'SparkQuill',
    AGENTWORKS_FAVICON_URL: '/sparkquill-mark.svg',
  }
}

module.exports = { buildAgentServerEnv }
