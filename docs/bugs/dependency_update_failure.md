# Bug: Dependency Updates in Sibling Module Not Reflected in Agent Build

## Symptom
When modifying code in the sibling module `../multi-llm-provider-go` (e.g., adding debug logs, fixing bugs in Azure adapter), the changes are **not** reflected in the deployed `mcp-agent` container, even when using `go.work` and local builds.

## verification Steps
1. Added `[AZURE_PATCH_V5_ACTIVE]` log to `azure_adapter.go`.
2. Redeployed using `./deploy.sh agent --local`.
3. Logs showed `Initialized Azure AI LLM` but **missing** the patch logs.
4. Health check showed `status: healthy` but no indication of the new code version.

## Root Cause (confirmed 2026-08-02, local builds)

**A sibling module needs a `replace` directive. `go.work` does not affect a
module build, and none of the Docker or Go-cache theories below are needed to
explain it.**

Hit twice in one day and diagnosed both times only after the symptom recurred:

1. A fix was written in `multi-llm-provider-go` (codex adapter teardown),
   verified by its own tests, and had **no effect** on the running server.
   `agent_go/go.mod` pinned the tagged version, so the build resolved from the
   module cache and never saw the working tree.
2. The identical thing happened again hours later with `mcpagent`.

The fix is one line per sibling:

```text
replace github.com/manishiitg/multi-llm-provider-go => ../../multi-llm-provider-go
replace github.com/manishiitg/mcpagent               => ../../mcpagent
```

Verify resolution rather than assuming, since a build against the wrong source
succeeds and looks identical:

```bash
go list -m -f '{{.Path}} -> {{.Dir}}' github.com/manishiitg/mcpagent
# want: -> /Users/…/mcpagent      not a path under $GOMODCACHE
```

And confirm the change actually linked, which is what finally settled it both
times — a distinctive string from the new code:

```bash
strings <binary> | grep -c "some new log line"
```

Both replaces are temporary and should be dropped when the modules are tagged;
`agent_go/go.mod` carries a comment saying so.

**Generalizable lesson:** a build against stale sources produces no error, no
warning, and a healthy process. Two hours were lost to "the fix doesn't work"
when the fix was never in the binary. Before debugging why a change had no
effect, verify the change is present in the artifact.

## Original Root Cause Analysis (Suspected — kept for the container case)

The below concerned container deployment and was never confirmed. The local-build
cause above is proven; if the container case recurs, start by applying the same
"is it actually in the binary" check before pursuing Docker-layer theories.
- **Docker Build Context:** The `Dockerfile.agent` copies `multi-llm-provider-go/` but might be caching the `COPY` layer if the file modification timestamps aren't propagated or if `go build` is hitting a cached intermediate layer that ignores the new files.
- **Go Cache:** `go build` inside the container might be using a cached module state if `go.work` isn't fully respected or if `go.mod` sums match (even if code changed).
- **Orphaned Containers:** Azure Container Apps might be rolling back to a previous healthy revision if the new one fails startup checks (though logs suggest it started).

## Proposed Solution: Version Verification
To definitively prove which code is running:
1.  Add a `const VERSION` string in `multi-llm-provider-go/llmtypes/version.go`.
2.  Update `agent_go/cmd/server/server.go` to import `llmtypes` and include `llmtypes.VERSION` in the `/api/health` response.
3.  Increment this version string on every significant patch.
4.  Check `/api/health` after deployment. If the version matches, the code is present.

## Workaround
- Force cache bust by modifying `multi-llm-provider-go/go.mod` (adding a comment) before build.
- Ensure `deploy.sh` effectively cleans or ignores Docker cache for the relevant layers.
