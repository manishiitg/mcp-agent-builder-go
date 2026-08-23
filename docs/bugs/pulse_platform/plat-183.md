[← Pulse platform issue index](../pulse_platform_issue_register.md)

# PLAT-183 — proposal: fold multi-llm-provider-go into mcpagent as an internal package

| Coordination | Value |
|---|---|
| Assigned agent | unassigned |
| Ticket state | `open` — proposal only, **not implemented, not scheduled** |
| Last synchronized | `2026-08-23` |

- **Priority:** P3 — no defect, no user-facing symptom; this is a repo-structure
  simplification the user asked to have written down for later, explicitly
  "don't merge it yet."
- **Owner:** repo/module structure — spans `multi-llm-provider-go` (its own
  repo), `mcpagent`, and `mcp-agent-builder-go`'s `agent_go/`.
- **Related:** none — this is a new proposal, not a fix for a prior ticket.

## Proposal

`multi-llm-provider-go` is a separately-tagged Go module (real semver tags
through v0.7.3+) that exists purely to be consumed by `mcpagent` (pinned via a
pseudo-version in `mcpagent/go.mod`) and, directly, by `agent_go` itself. The
user's observation: maintaining it as a second repository adds coordination
overhead (a two-repo pull/push/pin cycle for every change to the coding-agent
adapters) without buying any real isolation, since it has exactly one
consumer family. Proposed: move `multi-llm-provider-go`'s code into `mcpagent`
as an internal package (e.g. `mcpagent/llmprovider/...`), retire the standalone
module, and update every consumer's import path.

**This ticket documents what has to be taken care of before that move is
safe. It does not perform the move.**

## Confirmed findings (as of 2026-08-23)

1. **No external consumer exists.** `gh search code
   "github.com/manishiitg/multi-llm-provider-go" --owner manishiitg` and a
   broader `gh api search/code` sweep (not owner-scoped) both returned matches
   only inside `manishiitg/coding-agent-loop` (this repo,
   `mcp-agent-builder-go`) and `manishiitg/mcpagent`. No third-party or
   external repository imports it. Caveat: GitHub code search only covers
   public repos plus whatever private repos the querying token can see — it is
   evidence, not a provable universal guarantee. Re-run this exact search
   immediately before actually migrating, not just once now, since time will
   have passed.

2. **`agent_go` imports `multi-llm-provider-go` directly, not only
   transitively through `mcpagent`.** Confirmed via `grep -rl
   '"github.com/manishiitg/multi-llm-provider-go' agent_go --include="*.go"`
   — dozens of files across `agent_go/cmd/server/`, `agent_go/cmd/testing/`,
   `agent_go/cmd/family-server/`, and `agent_go/pkg/orchestrator/...` import
   it directly (e.g. `llm_provider_manifest.go`, `cli_security_routes.go`,
   `pkg/clisecurity/store.go`, `pkg/browser/tools.go`,
   `pkg/agentprofiles/skillfiles.go`, `pkg/workflowtypes/types.go`,
   `pkg/orchestrator/agents/workflow/step_based_workflow/*.go`). This means a
   fold-into-mcpagent move changes **two** dependency graphs, not one:
   `mcpagent`'s own internal imports, and every direct import inside
   `agent_go` — both need their import path rewritten to whatever the new
   internal path becomes (e.g.
   `github.com/manishiitg/mcpagent/llmprovider/llmtypes`).

3. **Deployment scripts and Dockerfile reference the standalone module's
   `replace` directive explicitly**, not just `agent_go/go.mod`:
   `deploy/dedicated-vm/deploy.sh`, `deploy/dedicated-vm/quick-deploy.sh`, and
   `agent_go/Dockerfile` all script `go mod edit
   -replace=github.com/manishiitg/multi-llm-provider-go=...` /
   `-dropreplace=...` steps. These need to be found and removed, not just the
   `go.mod`/`go.work` entries.

4. **Docs reference the standalone repo directly**, including at least
   `install.sh` (fetches a release tarball) and `deploy/k8s/README.md`
   (documents `go get github.com/manishiitg/multi-llm-provider-go@vX.Y.Z`).
   These need auditing for anything that still assumes the module is
   independently fetchable/taggable after the move.

## What to take care of before/during the actual migration

- [ ] **Full P0/real-contract test coverage across all four coding-agent
      adapters** (claude-code, codex-cli, cursor-cli, pi-cli) must pass
      post-move, not just `go build`. This repo has extensive real-tmux
      contract tests already (e.g. `picli_real_contract_test.go`'s
      `TestPiCLIRealMCPBridgeToolCallReportsRealToolName`, added this
      session) — these are the actual safety net for a move like this and
      must be re-run live (not skipped/mocked) against the moved code before
      calling the migration done.
- [ ] **Git history preservation.** Decide whether to `git subtree
      add`/`git filter-repo`-merge `multi-llm-provider-go`'s commit history
      into `mcpagent` (preserves blame/history) versus a flat copy-paste
      (loses it, simpler). This should be a deliberate choice, not a default.
- [ ] **Import path rewrite, both repos.** Every `mcpagent` internal import of
      `multi-llm-provider-go/...` AND every `agent_go` direct import (see
      confirmed finding #2's file list) needs a mechanical but
      exhaustive rewrite to the new internal path. A `goimports`/`gofmt`
      pass plus a full `go build ./...` in both repos is the correctness
      check, but the file list should be enumerated up front so nothing is
      missed silently.
- [ ] **`agent_go/go.mod` and `agent_go/go.work` cleanup.** Remove the
      `replace` directive(s) for `multi-llm-provider-go`, re-point at the
      updated `mcpagent` module/version instead. Regenerate
      `go.work.sum` via `go work sync` afterward (this repo just did exactly
      that cleanup this session for an unrelated drift — same command
      applies).
- [ ] **Deployment script updates.** `deploy/dedicated-vm/deploy.sh`,
      `deploy/dedicated-vm/quick-deploy.sh`, `agent_go/Dockerfile` — remove
      the now-dead `-replace=.../multi-llm-provider-go=...` /
      `-dropreplace=...` steps for that module specifically (they may still
      need the equivalent for `mcpagent` itself, unchanged).
- [ ] **Docs updates.** `install.sh`, `deploy/k8s/README.md`, and any other
      doc referencing `go get github.com/manishiitg/multi-llm-provider-go` or
      the repo directly — audit and update or remove.
- [ ] **Versioning/release implications.** `multi-llm-provider-go` currently
      has real, independently-tagged releases (up to v0.7.3+). Decide whether
      that tagging discipline continues to matter post-merge (probably not,
      since it becomes an internal package with no separate consumers per
      finding #1) — but this should be a stated decision, not silently
      dropped.
- [ ] **CI/build pipeline.** Check whether `multi-llm-provider-go`'s own repo
      has any GitHub Actions/CI workflows that would need porting into
      `mcpagent`'s CI (or can simply be retired if `mcpagent`'s existing CI
      already covers the same test paths once the code moves in).
- [ ] **Re-confirm "no external consumer" immediately before migrating**, not
      only now — re-run the `gh search code`/`gh api search/code` check from
      finding #1 as the literal last step before starting the move, since
      this ticket may sit open for a while.
- [ ] **Decide the fate of the old `multi-llm-provider-go` repo itself**
      (archive read-only vs. delete) as an explicit, separate decision at
      migration time — not assumed here. Deletion is hard to reverse and
      should get its own confirmation regardless of what this ticket
      recommends.

## Explicitly out of scope for this ticket

- The merge itself is **not being performed**. This ticket is proposal +
  checklist only, per explicit instruction.
- No code changes, no import rewrites, no repo deletions in this ticket.

## Verification

N/A — no code changed. This ticket exists to be read and executed against
when the user decides to actually do the migration.
