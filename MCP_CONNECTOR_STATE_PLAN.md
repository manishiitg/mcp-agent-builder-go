# MCP Connector Connection State — Implementation Plan

**Status:** implemented
**Scope:** 8 code files + 1 config file, 0 new files, 0 changes to the `mcpagent` repo
**Includes:** connection state (§3-§5, §7-§8) and total removal of runtime OAuth discovery (§6)

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

## 6. OAuth discovery removal

> **The catalog JSON is the sole source of OAuth endpoint truth. Runtime discovery is deleted, not gated.**

### 6.1 Why removal, not a fix

§2.2 blamed `discovery.go:342`. The bug is structural, not incidental:

- `DiscoverFromWellKnown` (`discovery.go:211`) is a four-line wrapper over `FetchAuthServerMetadata` (`:334`). Every `.well-known` path in the codebase funnels through the same host-only URL construction, so every one of them inherits the same false positive.
- Only the 401 path (`DiscoverFromResponse` → RFC 9728) resolves correctly, and it costs a full failed request per server.

Discovery also cannot learn anything that cannot be written down once. Verified against the live catalog — a path-aware RFC 9728 → RFC 8414 probe resolves **16 of 16** OAuth servers:

| Server | `auth_url` | `token_url` |
|---|---|---|
| Notion | `https://mcp.notion.com/authorize` | `https://mcp.notion.com/token` |
| Linear | `https://mcp.linear.app/authorize` | `https://mcp.linear.app/token` |
| Sentry | `https://mcp.sentry.dev/oauth/authorize` | `https://mcp.sentry.dev/oauth/token` |
| Canva | `https://mcp.canva.com/authorize` | `https://mcp.canva.com/token` |
| Airtable | `https://airtable.com/oauth2/v1/authorize` | `https://airtable.com/oauth2/v1/token` |
| PostHog | `https://oauth.posthog.com/oauth/authorize/` | `https://oauth.posthog.com/oauth/token/` |
| Grafana | `https://mcp.grafana.com/mcp/oauth/authorize` | `https://mcp.grafana.com/mcp/oauth/token` |
| Honeycomb | `https://ui.honeycomb.io/oauth/authorize` | `https://ui.honeycomb.io/oauth/token` |
| MongoDB | `https://cloud.mongodb.com/oauth/authorize` | `https://authorize.mongodb.com/tokens` |
| Apify | `https://console.apify.com/authorize/oauth` | `https://console-backend.apify.com/oauth/apps/token` |
| WorkOS | `https://signin.workos.com/oauth2/authorize` | `https://signin.workos.com/oauth2/token` |
| Resend | `https://api.resend.com/oauth/authorize` | `https://api.resend.com/oauth/token` |
| Paddle | `https://id.paddle.com/oauth2/authorize` | `https://id.paddle.com/oauth2/token` |
| Port | `https://mcp.port.io/v1/authorize` | `https://mcp.port.io/v1/token` |
| Indeed | `https://secure.indeed.com/oauth/v2/authorize` | `https://apis.indeed.com/oauth/v2/tokens` |
| Morningstar | `https://mcp.morningstar.com/authorize` | `https://mcp.morningstar.com/token` |

MongoDB is the only one advertising no `registration_endpoint` — irrelevant, since DCR is being removed anyway.

### 6.2 Code deleted

**`agent_go/cmd/server/oauth_routes.go`** — five blocks:

| Lines | Block | Replaced by |
|---|---|---|
| `:290-318` | the two `AutoDiscover: true` `OAuthConfig` initializers in `handleOAuthStart` | 400 — server has no `oauth` block, so it is open |
| `:333-415` | the `needsDiscovery` block, including **both** `oauth.RegisterClient` DCR calls (`:375`, `:404`) | nothing — endpoints come from config |
| `:588-614` | `handleOAuthStatus`, `OAuth == nil` auto-discovery branch | `connection: available`, no probe |
| `:620-645` | `handleOAuthStatus`, "auto-discover so refresh works" block | nothing — `token_url` is in config |
| `:720-747` | `handleOAuthLogout`, `OAuth == nil` auto-discovery branch | 400 |

**`agent_go/cmd/server/tools.go`**:

- `tryOAuthDiscovery` (`:1109-…`) and the local `OAuthEndpoints` type — delete
- call sites `:179-181` and `:272-273` — these are what set `RequiresOAuth` from a network probe, i.e. the direct cause of the Hugging Face false positive

**The inversion that makes this safe:** today `serverConfig.OAuth == nil` *triggers* discovery. After this change it *is the answer* — no `oauth` block means an open server. That single rule removes every remaining probe.

### 6.3 Config becomes authoritative

Each of the 16 OAuth entries gains `auth_url` and `token_url` alongside the existing `client_id` and `token_file`, plus `scopes`/`resource` where the AS advertises them. All four fields already exist on `oauth.OAuthConfig` (`mcpagent/oauth/config.go:9-21`) — **no schema change.**

```json
"Notion": {
  "url": "https://mcp.notion.com/mcp",
  "oauth": {
    "client_id":  "https://agentworkshq.com/.well-known/mcp-client.json",
    "auth_url":   "https://mcp.notion.com/authorize",
    "token_url":  "https://mcp.notion.com/token",
    "token_file": "~/.config/mcpagent/tokens/default/Notion.json"
  }
}
```

### 6.4 Consequences

- **`requires_oauth` stops being probe-derived.** It becomes `cfg.OAuth != nil` — a config read with no network call. Hugging Face reports false, correctly, and the `+` button matches the label.
- **Exa's 500 disappears.** `handleOAuthStatus` no longer errors at `:601` on a server that was never going to have OAuth, so `OAuthStatusBadge.tsx:183` stops returning `null`.
- **DCR is gone entirely.** Already implied by the CIMD-only rule that drops Vercel (§10). Any future DCR server must be added with a pre-registered `client_id`.
- **`needs_client_id` (`:423`) still compiles and still fires** — but only for a hand-edited entry missing `client_id`, and it now reports the configured URLs instead of discovered ones.
- **No custom-server-add UI exists.** `MCPConfigEditor.tsx` edits raw JSON, and `/api/mcp-config/discover` (`:196`) is *tool* discovery, unaffected. Hand-added servers must supply their own endpoints — consistent with the new rule.
- **`mcpagent` still gets 0 changes.** `FetchAuthServerMetadata` and `DiscoverFromWellKnown` lose their only callers but remain exported library functions. `DiscoverFromResponse` stays live via `mcpclient/client.go:958,987`. Deleting the two now-dead functions is a follow-up in that repo.

### 6.5 Resolving endpoints by hand

All 16 entries already carry `auth_url` and `token_url`. This is the procedure to
re-resolve one when a provider moves a URL, or to add a new OAuth connector.

**Order matters, and the path-aware form must come first** — trying only the root
form is exactly the bug in `discovery.go:342` that this section removes.

```bash
BASE=https://mcp.notion.com     # scheme + host of the server's url
MCP_PATH=/mcp                   # its path, if any

# 1. RFC 9728 — find the authorization server. Path-aware first, then root.
curl -s $BASE/.well-known/oauth-protected-resource$MCP_PATH \
  || curl -s $BASE/.well-known/oauth-protected-resource
# → read "authorization_servers": ["https://as.example.com"]

# 2. RFC 8414 on THAT server — again path-aware first, then root,
#    then OpenID discovery as a last resort.
AS=https://as.example.com
curl -s $AS/.well-known/oauth-authorization-server \
  || curl -s $AS/.well-known/openid-configuration
# → read "authorization_endpoint" and "token_endpoint"

# 3. If step 1 returns nothing, try step 2 against $BASE directly.
```

Take `authorization_endpoint` → `auth_url` and `token_endpoint` → `token_url`.
A `registration_endpoint` in the metadata is irrelevant here; DCR is removed.

### 6.6 Rollout order

Ship the JSON first, the deletion second. `needsDiscovery` is `AutoDiscover || AuthURL == "" || TokenURL == ""`, and no catalog entry sets `auto_discover`, so **populating the endpoints alone already makes every discovery branch dead at runtime.** Step 1 is behaviourally verifiable on its own; step 2 removes code that is by then provably unreachable.

---

## 7. Backend changes

### 7.1 `agent_go/cmd/server/tools.go`

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

### 7.2 `agent_go/cmd/server/oauth_routes.go`

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

### 7.3 `agent_go/cmd/server/server.go`

Two route lines beside `:2013-2016`:

```go
apiRouter.HandleFunc("/mcp/connect", api.handleConnectServer).Methods("POST", "OPTIONS")
apiRouter.HandleFunc("/mcp/disconnect", api.handleDisconnectServer).Methods("POST", "OPTIONS")
```

---

## 8. Frontend changes

### 8.1 `frontend/src/stores/types.ts`

Add `connection?: string` to `ToolDefinition` (`:7`).

### 8.2 `frontend/src/services/mcpConfigApi.ts`

Add `connectServer(name, apiKey?)` and `disconnectServer(name)` following the existing method shape (`:54-128`).

### 8.3 `frontend/src/stores/useMCPStore.ts`

- `:119` — `availableServers` filters `connection === 'connected'` instead of `status === 'ok'`
- `:260` — same change to the second getter
- `:154` — remove the auto-enable fallback
- `:168` — unchanged, still polls on `status === 'loading'`

### 8.4 `frontend/src/components/sidebar/MCPServersSection.tsx`

- `statusLabel` (`:27-30`) reads `connection`
- Sort keys (`:111`, `:121-122`) read `connection`
- Pass `connection` to the badge (`:275`)

### 8.5 `frontend/src/components/OAuthStatusBadge.tsx`

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

## 9. API key handling

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

## 10. Migration

Remove `Vercel` from `mcp_servers_clean_user.json` — it is DCR-only, violates the CIMD-only rule, and under the new rule would render as connected.

Canva and Linear stay in the overlay, but **neither has a token file on disk** — `~/.config/mcpagent/tokens/default/` held only `Vercel.json`. Under the state machine they therefore read as `available`, not `connected`. The assumption that they carried valid tokens was wrong; the observed result is the intended zero-state below, and reconnecting them is a normal OAuth flow.

Their `oauth.auto_discover: true` flags were also stripped from the overlay. Overlay entries replace base entries wholesale, so those flags — not the catalog — were what would have kept discovery alive for these two servers.

The orphaned `Vercel.json` token is left on disk; removing a credential is the user's call.

**Behaviour change:** dropping the auto-enable means all other servers start at zero. This is the intended model. If a softer rollout is wanted, seed the overlay once from servers currently reporting `status === 'ok'`.

---

## 11. Risk register

| Risk | Mitigation |
|---|---|
| Overlay write clobbers base fields | Always `LoadMergedConfig` → `GetServer` → write full struct. `persistOAuthConfig` already expects this. |
| Shared badge breaks two other call sites | New prop is optional; absent → existing code path. |
| 24 overlay file reads per `/api/tools` | Read once per request, pass into the loop. |
| `isMCPConfigLocked` bypassed | Guard at the top of both new handlers, matching `mcp_config_routes.go:133`. |
| Users lose all connectors on upgrade | Intended; optional one-time seed available. |
| A baked endpoint rots when a provider moves its authorize/token URL | No runtime fallback by design. Symptom is a failed authorize, not a silent wrong state; the handler names the server and the missing field. Fix is a one-line JSON edit using the procedure in §6.5. |
| A DCR-only server can no longer be connected | Already policy — the CIMD-only rule drops Vercel for exactly this reason (§10). Such a server needs a pre-registered `client_id`. |
| Deleting discovery breaks a hand-added server in `MCPConfigEditor` | Hand-added entries must now carry `auth_url`/`token_url`. Same rule as the catalog; no hidden behaviour. |

---

## 12. Out of scope

- `Reconnect` state and 401-at-use demotion — deferred by decision
- Deleting the now-dead `FetchAuthServerMetadata` / `DiscoverFromWellKnown` from `mcpagent/oauth/discovery.go` — §6 removes their last callers, but they stay as exported library functions so this phase keeps its 0-changes-to-`mcpagent` property. `DiscoverFromResponse` remains live (`mcpclient/client.go:958,987`) and is not touched.
- Retiring `status: "not_connected"` (`tools.go:180`, `:926`) — becomes vestigial but is read in five places; safer as a follow-up once `connection` is proven
- `requires_oauth` — stops driving UI and, per §6.2, stops being probe-derived; still read internally by `getToolStatusForUser` (`tools.go:1060`)

---

## 13. Test plan

1. Fresh overlay (only Canva/Linear) → all 22 other servers show `+`; Canva and Linear show connected
2. Connect Wolfram (no auth) → overlay entry appears, card flips to connected, no browser redirect
3. Connect Hugging Face with an API key → `Authorization` header lands in the overlay; tool count increases above 4
4. Connect Notion → OAuth redirect runs, token written, card flips on return
5. Disconnect Notion → overlay entry gone, token gone, card returns to `+`
6. Stop a connected server → `connection: connected` with `status: error`; tool-test button disabled, connect button still shows connected
7. `ServerSelectionDropdown` and `MCPDetailsModal` unchanged
8. Restart the server → connection states persist from the overlay

**Discovery removal (§6)**

9. Boot with the endpoints baked in → server logs contain zero `.well-known` fetches and zero `Auto-discovering OAuth endpoints` lines
10. Connect each of the 16 OAuth servers → authorize round-trips end to end using only configured URLs; confirm on the wire that no `/.well-known/*` request is issued
11. Hugging Face → `requires_oauth: false`, card shows `+`, no OAuth prompt (the §1 symptom, gone at the source)
12. Exa → `/api/oauth/status` returns 200 instead of 500; the button renders instead of `null`
13. Expire a token and let it refresh → refresh uses the configured `token_url` with no discovery fallback
14. Point a catalog entry at a deliberately wrong `auth_url` → the flow fails loudly at authorize rather than silently re-discovering

### Results

Verified against a live server on port 18799 (24 catalogue servers, `LOCAL_MODE`):

| # | Test | Result |
|---|---|---|
| 1 | Fresh overlay so nothing is connected | 24/24 `available` |
| 2 | Connect Wolfram (no auth) | overlay entry written, `connected`, 3 tools, no redirect |
| 3 | Connect Hugging Face with a key | `Authorization: Bearer` persisted to the overlay |
| 5 | Disconnect Hugging Face | overlay entry gone, card back to `available` |
| 6 | Connected server failing | observed naturally: HF with a bad key gave `connection: connected` + `status: error` |
| 8 | Restart the server | Wolfram + HF still `connected` from the overlay |
| 9 | No discovery on boot | zero `.well-known` / auto-discover / DCR lines in the log |
| 11 | Hugging Face | `requires_oauth` absent (false), `status: ok`, shows `+` |
| 12 | Exa | `/api/oauth/status` 200 with `has_oauth: false` (was 500) |
| 14 | Missing endpoint | 500 naming the server and the fix, no silent re-discovery |

Not run: 4, 10, 13 (need a real interactive OAuth grant) and 7 (visual check of the two unchanged call sites).

**No unit tests retained.** A `connection_state_test.go` covering the §4 state machine, per-user token isolation, and overlay loading was written and passing during implementation, then removed by decision. The per-user case guarded a real bug found while building (trusting `token_file` from config rather than the requesting user's path); a refactor of `connectionState` now has no automated cover.

---

## 14. Summary

**9 code files + 2 config files, 0 new files, 0 changes to `mcpagent`.**

Two changes ship together: `connection` stops the UI inferring ownership from reachability, and §6 stops the backend inferring auth from network probes. Both replace a guess with a fact that is already written down.

| Layer | Files |
|---|---|
| Backend | `tools.go`, `oauth_routes.go`, `server.go` |
| Frontend | `types.ts`, `mcpConfigApi.ts`, `useMCPStore.ts`, `MCPServersSection.tsx`, `OAuthStatusBadge.tsx` |
| Config | `mcp_servers_clean.json` — `auth_url`/`token_url` on 16 OAuth entries (§6.3); `mcp_servers_clean_user.json` — Vercel removed, `auto_discover` stripped (§10) |
