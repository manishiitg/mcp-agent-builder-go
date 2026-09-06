'use strict'

const { test } = require('node:test')
const assert = require('node:assert/strict')
const { buildAgentServerEnv } = require('./agentEnv')

test('buildAgentServerEnv never introduces AGENT_PRODUCTS or MULTI_USER_MODE', () => {
  const env = buildAgentServerEnv(
    {},
    { authSecret: 'secret', workspacePort: 45779, docsDir: '/tmp/docs', logFile: '/tmp/agent.log', workspaceApiToken: 'token' }
  )
  assert.equal('AGENT_PRODUCTS' in env, false, 'AGENT_PRODUCTS must stay unset — see A3 in the design doc')
  assert.equal('MULTI_USER_MODE' in env, false)
})

test('buildAgentServerEnv sets the fields the agent server needs', () => {
  const env = buildAgentServerEnv(
    { PATH: '/usr/bin' },
    { authSecret: 'secret', workspacePort: 45779, docsDir: '/tmp/docs', logFile: '/tmp/agent.log', workspaceApiToken: 'token' }
  )
  assert.equal(env.PATH, '/usr/bin')
  assert.equal(env.AUTH_SECRET, 'secret')
  assert.equal(env.WORKSPACE_API_URL, 'http://127.0.0.1:45779')
  assert.equal(env.WORKSPACE_DOCS_PATH, '/tmp/docs')
  assert.equal(env.DOCS_DIR, '/tmp/docs')
  assert.equal(env.LOG_FILE, '/tmp/agent.log')
  assert.equal(env.NATIVE_WORKSPACE, 'true')
  assert.equal(env.WORKSPACE_API_TOKEN, 'token')
  assert.equal(env.AGENTWORKS_SKIP_GLOBAL_BROWSER_CLEANUP, 'true')
  assert.equal(env.AGENTWORKS_BROWSER_SESSION_PREFIX, 'sparkquill')
})

test('buildAgentServerEnv pins the shared frontend build to the SparkQuill surface', () => {
  const env = buildAgentServerEnv(
    {},
    { authSecret: 'secret', workspacePort: 45779, docsDir: '/tmp/docs', logFile: '/tmp/agent.log', workspaceApiToken: 'token' }
  )
  // The main frontend ships every product surface; without these the window
  // would open on AgentWorks' automation overview instead of the family's app.
  assert.equal(env.AGENTWORKS_ENABLED_PRODUCT_SURFACES, 'sparkquill')
  assert.equal(env.AGENTWORKS_DEFAULT_PRODUCT_SURFACE, 'sparkquill')
  assert.equal(env.AGENTWORKS_APP_NAME, 'SparkQuill')
  assert.equal(env.AGENTWORKS_FAVICON_URL, '/sparkquill-mark.svg')
})
