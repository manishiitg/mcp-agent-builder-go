// Mirrors maskCredentialPreview in agent_go/cmd/server/workflow_provider_auth.go.
// Used only right after the user has typed a credential into the form — the
// plaintext is already in the browser at that point, so masking it locally adds
// no new exposure. The server is the only source of truth once the form is
// reopened; this is purely so the "Saved" state updates instantly.
export function maskCredentialPreviewClient(token: string): string | null {
  const affixLen = 4;
  if (token.length < affixLen * 2 + 4) return null;
  return `${token.slice(0, affixLen)}...${token.slice(-affixLen)}`;
}
