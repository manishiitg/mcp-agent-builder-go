// PLAT-169: hyphen and underscore MCP server names are compatibility aliases
// throughout MCP resolution — mirrors the backend's own normalization in
// ValidateManifest (agent_go/cmd/server/workflow_manifest.go). A manifest
// saved under a legacy spelling (e.g. "google_sheets") must still be
// recognized as the same server as the current catalog's canonical spelling
// (e.g. "google-sheets"), or a UI comparing them by exact string produces a
// checkbox that looks unchecked when the server IS already selected —
// leading a user to "select" it again and create a duplicate the backend
// then permanently rejects.

export const normalizeServerAlias = (name: string) => name.trim().replace(/_/g, '-');

export const serverNamesMatch = (a: string, b: string) => normalizeServerAlias(a) === normalizeServerAlias(b);

export const isSelectedServer = (selectedServers: string[], serverName: string) =>
  selectedServers.some(s => serverNamesMatch(s, serverName));

// A "server:tool" entry belongs to serverName if its own prefix (before the
// first ":") is an alias match — not just an exact string match — so
// clearing a server's tools also clears any legacy-spelled entries for it.
export const toolBelongsToServer = (tool: string, serverName: string) => {
  const separatorIndex = tool.indexOf(':');
  if (separatorIndex === -1) return false;
  return serverNamesMatch(tool.slice(0, separatorIndex), serverName);
};

// Alias-aware equivalent of `tools.includes(`${serverName}:${toolName}`)`.
export const hasServerTool = (tools: string[], serverName: string, toolName: string) => {
  return tools.some(t => {
    const separatorIndex = t.indexOf(':');
    if (separatorIndex === -1) return false;
    return serverNamesMatch(t.slice(0, separatorIndex), serverName) && t.slice(separatorIndex + 1) === toolName;
  });
};

// dedupeServerNames collapses alias-equivalent duplicates to a single entry
// (first occurrence wins), so a manifest that already has both spellings —
// e.g. from before this alias-awareness existed — self-heals the next time
// it is saved, instead of permanently failing the backend's duplicate
// validation until someone hand-edits the JSON.
export const dedupeServerNames = (servers: string[]): string[] => {
  const seen = new Set<string>();
  const result: string[] = [];
  for (const server of servers) {
    const key = normalizeServerAlias(server);
    if (seen.has(key)) continue;
    seen.add(key);
    result.push(server);
  }
  return result;
};
