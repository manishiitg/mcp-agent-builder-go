[← Pulse platform issue index](../pulse_platform_issue_register.md)

# PLAT-101 — Claude capacity exhaustion can strand a workflow until an operator intervenes

| Coordination | Value |
|---|---|
| Assigned agent | unassigned |
| Ticket state | `open` — design agreed; implementation and live reproduction test pending |
| Last synchronized | `2026-08-13` |

- **Priority:** P0 — a workflow can stop midway and remain falsely running for
  hours after its coding-agent account reaches a usage limit.
- **Owner:** Claude Code adapter error metadata, mcpagent fallback propagation,
  workflow continuation state, durable scheduler wake-up.
- **Observed on:** workflow runs whose Claude pane displayed `You've hit your
  weekly limit · resets 11:30pm (Asia/Calcutta)` while the workflow still had
  unfinished steps.

## Actual defect

Claude Code can exhaust its five-hour or seven-day subscription window during a
workflow rather than before it starts. At that point the coding agent cannot
reason about the failure or ask the platform to recover: the model itself is no
longer available.

The interactive adapter currently recognizes only the exact pane text
`You've hit your limit`. Claude's newer `You've hit your weekly limit` wording
does not contain that exact phrase, so the terminal can stay alive while the
workflow lifecycle waits indefinitely.

Even when quota exhaustion reaches mcpagent correctly, a run with no configured
fallback ends as `all LLMs failed (primary + 0 fallbacks)`. There is no durable
capacity-wait state, reset timestamp, or same-run wake-up. Keeping the tmux pane,
HTTP request, Go goroutine, or workflow agent alive until the reset would not be
safe: it wastes resources, fails across server restarts, and can leave the run
permanently stuck.

## Reliable reset-time source

Text parsing is not the primary contract. Claude Code's status-line JSON already
provides machine-readable subscription windows:

```json
{
  "rate_limits": {
    "five_hour": {
      "used_percentage": 100,
      "resets_at": 1786644000
    },
    "seven_day": {
      "used_percentage": 100,
      "resets_at": 1786721400
    }
  }
}
```

`resets_at` is an absolute Unix timestamp for the exact authenticated Claude
account used by that session. The adapter already reads these fields for
display, but converts them into strings such as `7d 100% → Fri` and discards the
structured timestamp.

Reset-time precedence must be:

1. structured `rate_limits.<window>.resets_at` from Claude's status line;
2. typed provider error metadata if Claude exposes it directly later;
3. visible terminal/result text parsing only as a compatibility fallback;
4. no guessed timestamp — use an unknown-capacity state when none is reliable.

## Proposed repair

### 1. Provider layer: return a typed capacity failure

In `multi-llm-provider-go`:

- preserve each rate-limit window as structured status data (`name`, used
  percentage, absolute reset time);
- recognize daily, weekly, five-hour, and generic usage-limit terminal/result
  variants without depending on one exact sentence;
- return a typed quota error carrying provider, model, exhausted window, and
  `RetryAt`;
- clean up the unusable Claude process after the failure is captured.

Candidate boundaries:

- `llmtypes/types.go`
- `llmerrors/errors.go`
- `pkg/adapters/claudecode/claudecode_interactive_adapter.go`
- `pkg/adapters/claudecode/claudecode_structured_adapter.go`

The provider layer reports the fact and reset time. It does not schedule a
workflow retry.

### 2. Agent layer: try fallbacks, then preserve the typed error

In `mcpagent`:

- skip same-model retries for quota exhaustion;
- try configured same-provider or cross-provider fallbacks immediately;
- preserve `RetryAt` through the final `all LLMs failed` error instead of
  flattening it into text;
- retain the coding-agent session handle needed for an exact continuation.

If a fallback succeeds, the workflow continues normally and no capacity pause
is created.

### 3. Workflow runtime: persist a durable suspension

In `mcp-agent-builder-go`, when no fallback succeeds and a future `RetryAt` is
known, persist the run as `waiting_for_capacity` with:

- provider/model and exhausted window;
- `resume_after` (reset time plus a small safety buffer);
- schedule run ID, workflow run folder, session ID, step ID/path, and phase;
- coding-agent native session handle;
- attempt number and the original typed failure.

The schedule run must no longer say `running`. Pulse, evaluation, backup,
publish, notification, and later sequence messages must not start while the
producing workflow is suspended.

No process, tmux pane, request, or sleeping workflow goroutine remains alive
during the wait.

Candidate boundaries:

- `cmd/server/schedule_runs.go`
- `cmd/server/scheduler.go`
- a focused durable wake-up owner such as `cmd/server/capacity_resume.go`
- `pkg/orchestrator/agents/workflow/step_based_workflow/workflow_continuation_state.go`

### 4. Durable wake-up and exact continuation

The capacity queue must be reconstructed on server startup and wake when its
nearest `resume_after` becomes due. At wake-up it must:

1. acquire the normal workflow/schedule execution lock;
2. atomically claim the suspension so two server loops cannot resume it twice;
3. attach the resumed turn to the original execution lifecycle tree;
4. resume the same native Claude session and same unfinished workflow phase;
5. reuse the existing run folder and outputs rather than start a new workflow;
6. progress through `waiting_for_capacity → resuming → running → completed`;
7. persist a later reset and wait again if capacity is still unavailable.

The next ordinary cron occurrence must not launch a duplicate workflow while an
older run of that schedule is waiting for capacity.

## Side-effect safety

The platform must not blindly replay the entire workflow. That could duplicate
posts, emails, payments, trades, or other external actions.

Automatic recovery is allowed only when the runtime can resume the exact native
session and unfinished phase using the existing run folder and continuation
record. Completed steps and durable outputs remain completed.

If exact continuation cannot be proven, transition to `recovery_required` and
ask the operator to choose a recovery action. Do not silently start over.

## Unknown reset time

When quota exhaustion is certain but no reliable future timestamp exists:

- try configured fallbacks;
- otherwise persist `waiting_for_capacity_unknown`;
- offer **Retry now**, **Use fallback provider**, and **Cancel run**;
- never invent a reset time or keep the run falsely active.

## User experience

Global Monitor and the workflow run should show a clock rather than a spinner:

```text
Waiting for Claude capacity · resumes after 11:35 PM
```

The safety buffer prevents waking exactly on a provider boundary. Controls:

- **Resume now** — recheck capacity and claim the same continuation;
- **Use fallback provider** — continue the unfinished turn using an explicitly
  configured compatible provider;
- **Cancel run** — terminally stop the suspension without running final stages
  that depend on missing workflow output.

## Required tests

### Provider tests

- five-hour and seven-day `resets_at` values survive status parsing as absolute
  timestamps;
- `You've hit your weekly limit` and other supported variants produce typed
  quota failures;
- structured `is_error:true` quota results produce the same typed contract;
- reset metadata belongs to the active authenticated session and is not read
  from another account/session;
- pane cleanup happens after the typed error is captured.

### Agent tests

- quota exhaustion skips same-model retries and tries the fallback chain;
- successful fallback does not create a suspension;
- no successful fallback preserves the typed `RetryAt` through the final error.

### Runtime tests

- a mid-step quota failure changes the run from `running` to
  `waiting_for_capacity`;
- the suspension survives a backend restart and wakes once;
- a second scheduler instance cannot claim the same suspension;
- no later sequence/Pulse/finalization stage runs before recovery;
- the due wake-up resumes the same run folder, session, step, and phase;
- completed side-effecting steps are not replayed;
- another cron fire does not duplicate the waiting run;
- a second capacity failure safely updates `resume_after`;
- missing exact continuation proof yields `recovery_required`, not a replay.

## Acceptance

- A real Claude usage-limit event cannot leave a workflow indefinitely
  `running`.
- With a configured fallback, the unfinished turn continues through that
  fallback.
- Without a fallback, the workflow releases all live resources and visibly
  waits until the structured reset timestamp.
- Restarting the server during the wait neither loses nor duplicates the run.
- At reset, only the unfinished work resumes; completed external actions are
  never repeated.
- The run reaches its normal terminal stages only after the recovered workflow
  itself finishes.
