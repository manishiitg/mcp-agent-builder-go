import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, expect, it } from 'vitest'
import { WORKSPACE_VIEWS } from './workspaceViews'

// The agent's open_workspace_view tool (agent_go/cmd/server/workflow_view_tool.go)
// carries its own copy of the view ids, since Go cannot read this registry.
// Keep the two identical, in order, so a view added here is one the agent can
// open and the agent never offers a view that does not exist.
describe('open_workspace_view mirrors the workspace view registry', () => {
  it('lists exactly the registered view ids', () => {
    const goFile = resolve(__dirname, '../../../../agent_go/cmd/server/workflow_view_tool.go')
    const source = readFileSync(goFile, 'utf8')
    const block = source.slice(source.indexOf('var workflowWorkspaceViews'), source.indexOf('}\n\n', source.indexOf('var workflowWorkspaceViews')))
    const goIds = Array.from(block.matchAll(/\{"([a-z-]+)", "/g)).map(m => m[1])
    expect(goIds).toEqual(WORKSPACE_VIEWS.map(v => v.id))
  })
})
