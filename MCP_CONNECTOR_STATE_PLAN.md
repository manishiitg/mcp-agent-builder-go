# MCP Connector Connection State — Implementation Plan

**Status:** awaiting approval
**Scope:** 8 files, 0 new files, 0 changes to the `mcpagent` repo

---

## 1. Problem

The connector UI cannot distinguish "this server responds" from "the user connected this server." Both the label and the button derive from tool discovery, which succeeds for any reachable server regardless of user action.

Observed symptoms on the current build:

| Server | Label | Button | Cause |
|---|---|---|---|
| Hugging Face | "Connected" | `+` shown | `requires_oauth` false positive |
| Exa | "Connected" | no button at all | `/api/oauth/status` returns 500 |

Two open servers, opposite failures, same root cause.

---

## 2. Evidence

### 2.1 Live endpoint probe

Unauthenticated MCP `initialize` + `tools/list`:

| Endpoint | initialize | anonymous tools |
|---|---|---|
| `https://huggingface.co/mcp` | HTTP 200 | 4 — `hf_whoami`, `hub_repo_search`, `hub_repo_details`, `hf_fs` |
| `https://mcp.exa.ai/mcp` | HTTP 200 | 2 — `web_search_exa`, `web_fetch_exa` |

Neither returns 401. Neither sends `WWW-Authenticate`. Both are **open servers that accept an optional credential** for expanded access.

### 2.2 Why Hugging Face reports OAuth

`oauth/discovery.go:342` builds the discovery URL from scheme + host only, discarding the path:

```go
baseURL := fmt.Sprintf("%s://%s", parsedURL.Scheme, parsedURL.Host)  // discovery.go:342
wellKnownURL := baseURL + "/.well-known/oauth-authorization-server"  // discovery.go:343
```

`https://huggingface.co/mcp` becomes `https://huggingface.co/.well-known/oauth-authorization-server`, which returns 200 because the *website* has OAuth. The MCP endpoint does not.

### 2.3 Why Exa shows no button

`mcp.exa.ai/.well-known/…` 404s, the 401 fallback receives a 200, so `handleOAuthStatus` errors at `oauth_routes.go:601`. The frontend catch sets `hasOAuth = false` (`OAuthStatusBadge.tsx:69`), and `:183` returns `null`.

### 2.4 Current inputs to the card — all three flow from discovery

```
discovery → tools[0].status         → MCPServersSection.tsx:268 → label text
discovery → tools[0].requires_oauth → MCPServersSection.tsx:242 → prop
                                    → OAuthStatusBadge.tsx:78  → hasOAuth
                                    → OAuthStatusBadge.tsx:183 → renders or returns null
/api/oauth/status → status.valid    → OAuthStatusBadge.tsx:267 → + vs green check
```

---

## 3. Design decision

> **Connected ⟺ the server has an entry in `mcp_servers_clean_user.json`.**

No new storage layer, no new file, no change to the `mcpagent` repo.

**Justification — this is already true of the system:**

- The overlay is gitignored (`agent_go/configs/.gitignore:1`), so credentials are safe there
- `LoadMergedConfig` already merges base + overlay (`mcpclient/config.go:276`)
- OAuth already writes to the overlay on callback success (`oauth_routes.go:519`), so overlay presence *already* means connected for OAuth servers
- A hand-added server in the overlay reads as connected, which is the correct semantics for hand-adding

**Merge semantics to respect:** overlay entries replace base entries wholesale, not field-by-field (`mcpclient/config.go:342-351`). Every write must load the merged entry and write the complete struct.

**Identity:** `GetUserIDFromContext` falls through to `GetDefaultUserID()` = `"default"` (`auth_middleware.go:125-129`). Machine-scoped, single identity. Matches the existing token paths.

---

## 4. State machine

```
connection = (name in overlay) AND (no oauth block OR credential present)
```

| Overlay entry | Auth config | Credential | `connection` | UI |
|---|---|---|---|---|
| absent | — | — | `available` | `+` Connect |
| present | none | — | `connected` | ✓ / toggle |
| present | `oauth` | valid | `connected` | ✓ / toggle |
| present | `oauth` | missing, expired-no-refresh, refresh-failed, corrupt | `available` | `+` Connect |

Credential validity reuses `hasOAuthTokenFile` (`tools.go:912`) unchanged.

**Deferred (agreed):** revoked-but-unexpired tokens are undetectable locally — `oauth2.Token.Valid()` only reads the local expiry timestamp. Those show `connected` until a real request 401s. The `Reconnect` state is deliberately out of scope for this phase.

---

## 5. Two fields, two questions

`status` is **not** removed. After this change:

| | `connection` (new) | `status` (existing) |
|---|---|---|
| Question | is it mine? | is it working? |
| Source | config + credential | discovery |
| Drives | Connect/Connected button | tool list, error text, loading spinner |

`connection: connected` + `status: error` is a valid, useful combination — token fine, server down.

### Live consumers of `status` that must keep working

**Frontend**

- `ToolList.tsx:110` — disables the tool-test button when `status !== 'ok'`
- `ToolList.tsx:42,55,100,121` — dot colour, label, error message
- `MCPDetailsModal.tsx:109` — health dot
- `ToolSelectionSection.tsx:56`
- `useMCPStore.ts:168` — re-poll while any server is `loading`

**Backend**

- `tools.go:811` — decides cache-entry validity
- `tools.go:973-980` — discovery-failure tracking and per-server logs
- `tools.go:1072` — `not_connected` → `loading` remap

---

## 6. Backend changes

### 6.1 `agent_go/cmd/server/tools.go`

**Add to `ToolStatus`** (struct at `:62-74`):

```go
Connection string `json:"connection"` // "available" | "connected"
```

**New helper**, placed beside `hasOAuthTokenFile` at `:912`:

```go
func (api *StreamingAPI) connectionState(name string, cfg mcpclient.MCPServerConfig) string
```

Loads the overlay via `mcpclient.LoadConfig(api.getUserConfigPath(), api.logger)`, checks name membership, then `hasOAuthTokenFile(cfg)`. Returns `"connected"` or `"available"`.

> Overlay read is cached per request — `handleGetTools` iterates 24 servers and must not re-read the file 24 times.

**Set the field** in both branches of the `handleGetTools` loop (`:325-342`): the cached branch at `:327` and the fallback branch at `:333`.

`status` is not modified anywhere.

### 6.2 `agent_go/cmd/server/oauth_routes.go`

**`handleConnectServer`** (new). Request: `{server_name, api_key?}`

1. `isMCPConfigLocked()` guard (`mcp_config_routes.go:17`)
2. `mcpclient.LoadMergedConfig` → `config.GetServer(name)`
3. If `api_key` supplied → set `cfg.Headers["Authorization"] = "Bearer " + key`
4. If `cfg.OAuth != nil` → delegate to the existing `handleOAuthStart` flow (`:248`); the overlay write already happens on callback success at `:519`
5. Otherwise → `persistOAuthConfig(name, cfg)` (`:790`) and return

**`handleDisconnectServer`** (new)

1. Remove the entry from the overlay, save via `mcpclient.SaveConfig`
2. If OAuth, delete the token (`oauthMgr.Logout()`)
3. Reuse the cache-invalidation block from `handleOAuthLogout` (`:765-778`) — `InvalidateByServer`, `delete(api.toolStatus, name)`, `go api.startBackgroundDiscovery()`

**Modify `handleOAuthLogout`** (`:689`): also remove the overlay entry. Today it deletes the token but leaves the entry, which under the new rule would still read as connected.

> **Reuse note.** `persistOAuthConfig` (`:790`) is misnamed but generic — it loads the overlay, upserts a whole `MCPServerConfig`, and saves. Nothing in it is OAuth-specific except log strings. It stores `Headers` with no modification.

> **Do not use** `handleSaveMCPConfig` (`mcp_config_routes.go:131`) for this. It only persists servers absent from the base config (`:160-164`), so a write for Hugging Face would be silently dropped.

### 6.3 `agent_go/cmd/server/server.go`

Two route lines beside `:2013-2016`:

```go
apiRouter.HandleFunc("/mcp/connect", api.handleConnectServer).Methods("POST", "OPTIONS")
apiRouter.HandleFunc("/mcp/disconnect", api.handleDisconnectServer).Methods("POST", "OPTIONS")
```

---

## 7. Frontend changes

### 7.1 `frontend/src/stores/types.ts`

Add `connection?: string` to `ToolDefinition` (`:7`).

### 7.2 `frontend/src/services/mcpConfigApi.ts`

Add `connectServer(name, apiKey?)` and `disconnectServer(name)` following the existing method shape (`:54-128`).

### 7.3 `frontend/src/stores/useMCPStore.ts`

- `:119` — `availableServers` filters `connection === 'connected'` instead of `status === 'ok'`
- `:260` — same change to the second getter
- `:154` — remove the auto-enable fallback
- `:168` — unchanged, still polls on `status === 'loading'`

### 7.4 `frontend/src/components/sidebar/MCPServersSection.tsx`

- `statusLabel` (`:27-30`) reads `connection`
- Sort keys (`:111`, `:121-122`) read `connection`
- Pass `connection` to the badge (`:275`)

### 7.5 `frontend/src/components/OAuthStatusBadge.tsx`

Add optional `connection?: string` prop.

**When supplied:**

- skip `checkTokenStatus` polling (`:46-85`)
- skip the `hasOAuth === false` early return (`:183`)
- drive the glyph from `connection` instead of `tokenValid` (`:267`)
- route clicks to the new endpoints

**When absent: behaviour unchanged.** `ServerSelectionDropdown.tsx` and `MCPDetailsModal.tsx` both use `variant="label"` and pass no `connection`, so they are unaffected.

Reuse the existing `clientIdDialog` (`:196`) as the API-key input rather than adding a modal.

> **Side benefit:** the icon variant stops polling `/api/oauth/status` every 10s per card — currently ~24 requests per 10s just to render buttons.

---

## 8. API key handling

The connect dialog offers an optional key for any server with no `oauth` block. Supplied → stored as `Authorization: Bearer <key>` in `Headers`. Skipped → connects anonymously.

Plumbing already exists — no schema change:

```
Headers (mcpclient/config.go:92)
  → transport.WithHTTPHeaders (http_manager.go:36)
  → transport.WithHeaders     (sse_manager.go:36)
```

**To verify during implementation:**

- whether Exa accepts `Authorization: Bearer` (its CORS list includes `Authorization` but it also advertises `x-api-key`)
- whether an HF token expands beyond the 4 anonymous tools

### Config note

The HF snippet from HF's docs uses `"type": "sse"`. Both parts are wrong here:

- the field is `protocol` (`mcpclient/config.go:90`), not `type`
- HF is Streamable HTTP, not SSE

Go silently ignores unknown JSON keys, so `type` would fail invisibly. Omit the field — `GetProtocol()` (`:187`) already resolves it correctly:

```json
"Hugging Face": {
  "url": "https://huggingface.co/mcp",
  "headers": { "Authorization": "Bearer hf_xxx" }
}
```

---

## 9. Migration

Remove `Vercel` from `mcp_servers_clean_user.json` — it is DCR-only, violates the CIMD-only rule, and under the new rule would render as connected.

Canva and Linear stay: both are in the overlay with valid tokens, so they read as connected on first load with no migration step.

**Behaviour change:** dropping the auto-enable means all other servers start at zero. This is the intended model. If a softer rollout is wanted, seed the overlay once from servers currently reporting `status === 'ok'`.

---

## 10. Risk register

| Risk | Mitigation |
|---|---|
| Overlay write clobbers base fields | Always `LoadMergedConfig` → `GetServer` → write full struct. `persistOAuthConfig` already expects this. |
| Shared badge breaks two other call sites | New prop is optional; absent → existing code path. |
| 24 overlay file reads per `/api/tools` | Read once per request, pass into the loop. |
| `isMCPConfigLocked` bypassed | Guard at the top of both new handlers, matching `mcp_config_routes.go:133`. |
| Users lose all connectors on upgrade | Intended; optional one-time seed available. |

---

## 11. Out of scope

- `Reconnect` state and 401-at-use demotion — deferred by decision
- `oauth/discovery.go:342` path-stripping fix — lives in the `mcpagent` repo; no longer affects the UI once `connection` lands
- Retiring `status: "not_connected"` (`tools.go:180`, `:926`) — becomes vestigial but is read in five places; safer as a follow-up once `connection` is proven
- `requires_oauth` — stops driving UI, still used internally by `getToolStatusForUser` (`tools.go:1060`)

---

## 12. Test plan

1. Fresh overlay (only Canva/Linear) → all 22 other servers show `+`; Canva and Linear show connected
2. Connect Wolfram (no auth) → overlay entry appears, card flips to connected, no browser redirect
3. Connect Hugging Face with an API key → `Authorization` header lands in the overlay; tool count increases above 4
4. Connect Notion → OAuth redirect runs, token written, card flips on return
5. Disconnect Notion → overlay entry gone, token gone, card returns to `+`
6. Stop a connected server → `connection: connected` with `status: error`; tool-test button disabled, connect button still shows connected
7. `ServerSelectionDropdown` and `MCPDetailsModal` unchanged
8. Restart the server → connection states persist from the overlay

---

## 13. Summary

**8 files, 0 new files, 0 changes to `mcpagent`.**

| Layer | Files |
|---|---|
| Backend | `tools.go`, `oauth_routes.go`, `server.go` |
| Frontend | `types.ts`, `mcpConfigApi.ts`, `useMCPStore.ts`, `MCPServersSection.tsx`, `OAuthStatusBadge.tsx` |
| Config | one Vercel removal |
