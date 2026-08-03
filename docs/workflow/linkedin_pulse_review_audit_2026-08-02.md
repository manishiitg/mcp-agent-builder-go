# LinkedIn Pulse Review Audit — 2026-08-02

## Scope

This note audits the Pulse pass associated with:

- Workflow: `Workflow/linkedin`
- Pulse run: `schedule-manual--4e78513d_1785686558465488000`
- Target workflow run: `iteration-0/engage`
- Target run completed: `2026-08-02T17:01:42Z`
- Reviewers run: `bug_review`, `artifact_review`, `eval_health`, `stores_health`, `goal_advisor`
- Reviewers skipped: `llm_ops_review`, `report_health`, `strategy_auditor`

The audit answers four questions:

1. Which reviewer created each class of registered issue?
2. Is each issue relevant and correctly classified?
3. How much duplication or lifecycle noise exists in the backlog?
4. How much time and token usage did each reviewer consume?

## Executive assessment

Pulse registered 42 records. They do not represent 42 independent bugs.

The reviews found several important correctness defects, particularly in Bug Review and Eval Health. However, the backlog is inflated by cross-review duplicates, transient validation failures that remained filed after repair, operational state recorded as issues, and strategy proposals shown alongside defects.

The main quality problem is therefore no longer reviewer reasoning. It is finding normalization, deduplication, classification, and lifecycle closure.

The five reviewers spent 1 hour 13 minutes 50 seconds actively running. Because they ran sequentially with gaps, the complete reviewer window was about 1 hour 25 minutes 54 seconds. Their cumulative model accounting was 35,356,816 tokens, but 33,942,528 input tokens were cached. Fresh input was 1,260,825 tokens.

## Registered record totals

### By creator

| Creator | Records | Assessment |
|---|---:|---|
| Artifact Review | 10 | Generally sensible evidence, but six records duplicate other reviewers |
| Bug Review | 8 | Strongest review; all eight findings were legitimate |
| Eval Health | 8 | Mostly valid; mixes correctness defects with optimization and coverage proposals |
| Stores Health | 7 | Mostly valid; one finding relied on an outdated knowledgebase-field assumption |
| Goal Advisor | 3 | Sensible strategic analysis; only one item is an implementation defect |
| Runtime and pre-validation | 6 | Mostly transient evidence, operational state, or duplicates rather than independent bugs |
| **Total** | **42** | Not 42 unique defects |

### By current lifecycle state

| State | Count |
|---|---:|
| `verification_inconclusive` | 15 |
| `proposal_recorded` | 7 |
| `awaiting_run` | 7 |
| `filed` | 6 |
| `closed` | 4 |
| `external_action_required` | 2 |
| `awaiting_user` | 1 |
| **Total** | **42** |

The UI's open-like count of 22 is composed of 15 `verification_inconclusive`, six `filed`, and one `awaiting_user` record. It should not be interpreted as 22 unrelated repairable bugs.

## Issue types and relevance

### 1. Correctness and runtime defects

These are real workflow bugs whose observed behavior violates the workflow contract. Bug Review found the clearest examples:

- Old LinkedIn profile selectors wasted 31 of 40 profile views. This was fixed and closed after the next run successfully extracted 15 of 15 headlines.
- Retired follow-candidate processing continued to run.
- Relative-time parsing matched `m` before `mo`, allowing months to be interpreted as minutes.
- The scan timestamp was captured late and local time was labelled as UTC.
- Semantically irrelevant targets, including medical and certification posts, passed the relevance filter.
- Malformed browser envelopes caused challenge detection to fail open.
- Pre-validation checked structural JSON shape but missed freshness, timestamp, and relevance errors.
- Generic whole-page text containing `security check` could create a false 24-hour automation block.

Assessment: all are relevant. Seven remain unverified because the producing LinkedIn workflow has not run since the repairs.

### 2. Evaluation correctness and measurement gaps

Eval Health found these correctness issues:

- Publishing evaluation still referenced retired Buffer and JSON behavior.
- Saved evaluator scripts were stale.
- Evaluation could inspect mutable current workspace state instead of the target run.
- Evaluator results lacked a consistent evidence and `max_score` contract.
- The evaluation allowed 150–400 words while `soul/soul.md` specifies 150–350 words.

Assessment: these are relevant correctness defects. The 350-versus-400 mismatch should be handled as a contract repair rather than merely a proposal.

Eval Health also raised broader improvements:

- Explicitly select a cheaper model tier for evaluation.
- Measure follower and inbound-comment trends, grounded relevant comments, theme compliance, and consumption of winning signals.
- Separate operational tactic checks from actual goal achievement.

Assessment: these are useful, but they are optimization or evaluation-design proposals, not bugs.

### 3. Artifact and configuration drift

Artifact Review reported ten records. Six describe real problems but duplicate canonical findings already created by Bug Review, Eval Health, or Stores Health:

- Weak semantic target validation.
- Retired follow-candidate work.
- Incorrect scan timestamps.
- Retired Buffer/JSON evaluation behavior.
- Stale evaluator scripts.
- Oversized duplicated workflow skill instructions.

Assessment: the evidence is relevant, but these should have been linked as corroborating evidence rather than filed as separate issues.

The remaining Artifact Review records were:

- Artifact Sync Cursor absent from `builder/improve.html`.
- Changelog entries omit some evaluation and learning edits.
- Explore and Measure work have no schedules, leaving promotion, performance refresh, and follower sampling unreachable.
- Global research refers to deleted web-search routes and `web_search_raw` behavior.

Assessment: these are reasonable findings. The first two are platform/dashboard gaps. The latter two are workflow-design or reachability decisions and may require user policy rather than an automatic repair.

### 4. Store, database, knowledgebase, and retained-state issues

Stores Health reported:

- Some `image_assets` rows pointed to volatile run-folder paths.
- The root workflow skill had grown into a duplicated 656-line instruction body.
- The database README omitted framework-managed tables.
- Deprecated safety and engagement guidance referenced retired storage formats.
- Protected voice and AgentWorks rules live in auto-maintained notes instead of clearly user-owned context.
- Step ownership and least-privilege database access were insufficiently explicit.

Assessment:

- Volatile image paths, oversized duplicated skills, incomplete database documentation, and retired storage guidance are relevant.
- The location of protected voice rules is a design decision; `awaiting_user` is appropriate.
- The database least-privilege concern was valid.
- The claim that an absent `knowledgebase_write_method` was itself a defect was outdated. That field is retired and intentionally absent under the current contract.

### 5. Strategy and goal gaps

Goal Advisor reported:

- Approval controls contradicted one another: the resolver did not approve, the parent required approval, and a worker could force live behavior after exact approval.
- The workflow lacks a bounded comparison of ordinary posts against participant-seeded posts.
- Existing evidence does not establish a clean causal link between broadcast or engagement activity and inbound comments or follower growth.

Assessment:

- The approval-control contradiction is a real implementation defect and has been repaired, but still awaits a producing run.
- The other two items are valid strategy and experiment proposals. They should remain separate from the bug backlog.

### 6. Runtime and pre-validation events

Six records came from evaluators or the engagement scan rather than the five review modules:

- The configured `workflow_db` route was unavailable, but the fallback database query succeeded.
- `automation_safety` contained a challenge and `blocked_until` value.
- The engagement scan initially omitted `posts_examined`.
- Seven follow candidates appeared despite the allowed maximum being zero.
- A text-only challenge appeared at a normal LinkedIn URL.
- The engagement scan initially omitted `notes`.

Assessment:

- Successful database fallback is operational telemetry, not a workflow issue.
- A current safety block is state/evidence, not an independent defect.
- The text-only challenge duplicates the canonical challenge-detection finding.
- Missing fields that were subsequently repaired should close automatically.
- Forbidden follow candidates are evidence for the retired-follow bug, not a new issue.

## Reviewer-by-reviewer quality assessment

### Bug Review

High relevance and good evidence quality. It identified concrete behavioral failures with clear expected and observed outcomes. Its active findings should remain canonical.

### Artifact Review

Good at collecting corroborating evidence, but poor cross-module deduplication caused six duplicate records. It also mixes workflow defects with platform-owned dashboard and changelog gaps.

### Eval Health

Mostly correct and valuable. Its defect findings are strong, but optimization and coverage recommendations should be typed separately from correctness issues.

### Stores Health

Mostly correct. One part of the step-config finding was based on a retired field and should not have survived current-contract validation.

### Goal Advisor

The reasoning is useful and appropriately cautious. Its strategy proposals should not contribute to a count presented as repairable bugs.

### Runtime/pre-validation filing

This is the weakest source of backlog quality. The system is retaining transient gate failures and observations rather than linking them to canonical findings or closing them after successful repair.

## Runtime and token usage

All five reviewers used `gpt-5.6-sol`. The terminal UI sometimes displayed a generic `Claude Code is working` status, but the underlying Codex rollout records identify the actual model as `gpt-5.6-sol`.

| Reviewer | Wall time | Input tokens | Cached input | Fresh input | Output tokens | Total tokens |
|---|---:|---:|---:|---:|---:|---:|
| Bug Review | 13m 06s | 6,489,875 | 6,262,784 | 227,091 | 28,182 | 6,518,057 |
| Artifact Review | 18m 43s | 8,107,760 | 7,833,344 | 274,416 | 38,414 | 8,146,174 |
| Eval Health | 13m 26s | 5,254,888 | 5,033,728 | 221,160 | 29,090 | 5,283,978 |
| Stores Health | 16m 33s | 8,545,417 | 8,271,872 | 273,545 | 33,262 | 8,578,679 |
| Goal Advisor | 12m 02s | 6,805,413 | 6,540,800 | 264,613 | 24,515 | 6,829,928 |
| **Total** | **1h 13m 50s** | **35,203,353** | **33,942,528** | **1,260,825** | **153,463** | **35,356,816** |

Token interpretation:

- Approximately 96.4% of input tokens were cache reads.
- The 35.36 million total is cumulative accounting across many agent turns, where context is repeatedly presented to the model. It is not 35 million newly processed uncached tokens.
- Fresh input was still substantial at approximately 1.26 million tokens.
- Artifact Review was the slowest reviewer.
- Stores Health consumed the most tokens.
- Reviewers ran sequentially. Their active time totalled 1h 13m 50s; gaps increased the first-review-to-last-review wall window to approximately 1h 25m 54s.
- Skipped modules consumed no reviewer runtime or tokens in this Pulse pass.

Token usage was read from the final `token_count` event in the five matching Codex rollout records under `~/.codex/sessions/2026/08/02/`. Wall time was measured from the corresponding Pulse terminal lifecycle.

## Fixer resolution audit

### Did the fixer fix every registered issue?

No. It made real source/configuration changes for most automatically repairable canonical defects, but it did not and should not treat every registered record as a repairable bug.

The 42 records currently divide into four practical groups:

| Practical outcome | Count | Meaning |
|---|---:|---|
| Closed | 4 | One fix was verified immediately; three findings were closed because fresh evidence showed no current change was required |
| Real changes awaiting proof | 16 | Fifteen canonical findings are `changed_unverified`; the approval-control repair is `awaiting_run` |
| Duplicate records linked to canonical changes | 6 | Artifact Review duplicates were not separately changed; they await the same producing-run proof as their canonical findings |
| Not resolved by the fixer | 16 | Seven proposals, two platform handoffs, one user decision, and six raw runtime/pre-validation filings |
| **Total** | **42** | |

This means the fixer performed substantive work, but only four records reached a terminal closed state. Source changes are not equivalent to verified resolutions.

### Exact resolution types

| Resolution type | Count | Interpretation |
|---|---:|---|
| `fixed_verified` | 1 | The fixer changed the artifact and proved the repaired result immediately |
| `verified_no_change` | 3 | Current evidence showed the reported behavior was already absent or no longer affected the live path |
| `changed_unverified` / current event `verification_inconclusive` | 15 | Files changed and static/inert checks passed, but a real producing workflow/evaluation run is still required |
| `awaiting_run` | 7 | Six cross-review duplicates point at canonical pending repairs; one approval-control repair needs a real approved Post run |
| `proposal_only` | 7 | Intentionally not applied because it changes strategy, rubric meaning, cadence, or source policy |
| `external_action_required` | 2 | Owned by the dashboard or shared platform rather than the workflow fixer |
| `awaiting_user` | 1 | Requires a user decision about the authoritative scope of protected voice/AgentWorks rules |
| Raw `filed` | 6 | Runtime/evaluator observations remain untriaged and were not resolved by the fixer |
| **Total** | **42** | |

### What the fixer actually changed

The changes are present in workflow commit `fd7a30f` (`pulse: back up review and maintenance state`). That commit changed 16 files, with 1,598 insertions and 2,028 deletions. The substantive changes include:

- Scanner contract repairs for challenge provenance, UTC scan start, `mo` before `m`, relevance evidence, semantic validation, and retired follow work.
- Approval-path changes in `step-post-approval-resolve`, `step-p4-publisher`, and the publishing worker so a fresh exact-draft approval and atomic claim control the live path.
- Target-run and group binding for publish and draft evaluation.
- Removal of four stale saved evaluator scripts.
- Explicit lower-tier agentic evaluator configuration.
- Evidence and exact `max_score=10` validation across ten evaluators.
- Replacement of the oversized global skill body with a 43-line selective index.
- Database README coverage for the missing Pulse lifecycle tables.
- Read-only drafter database access and explicit writer ownership.
- Future image generation changed to persist bytes under `db/assets/` with size and SHA-256 before writing the SQLite path.

The module audit recorded:

| Module | Result | Changed files | Verification state |
|---|---|---:|---|
| Bug Review | `changed` | 2 | Runtime proof awaits the next safety-permitted Engage run |
| Eval Health | `changed` | 6 | Evaluation runtime proof awaits a new applicable evaluation |
| Artifact Review | `done` | 0 | Reconciled duplicates, proposals, and external ownership |
| Stores Health | `changed` | 7 | One repair verified; three changes await real consumers |
| Goal Advisor | `done` | 0 | Strategy proposals preserved; approval repair recorded as an operational handoff |

### What is genuinely verified

Only `PUL-C7FDCD96` is `fixed_verified`. The fixer updated `db/README.md`, compared it with the live SQLite schema, and proved that all non-internal tables have documentation and ownership coverage.

Three records are `verified_no_change`:

- The deprecated headline selectors were not used by the current path and the fresh run populated 15 of 15 target headlines.
- The corresponding Stores Health headline record was the same resolved behavior.
- Current safety and engagement guidance already used SQLite instead of the deprecated JSON consumers.

These closures are legitimate, but two headline records represent the same underlying behavior.

### What changed but is not yet verified

Fifteen canonical findings have real edits plus successful structural or inert checks:

- Seven Bug Review findings covering scanner safety, time parsing, relevance, validation, and retired-follow behavior.
- Five Eval Health findings covering model configuration, stale evaluator code, publish evidence, target-run binding, and result schema.
- Three Stores Health findings covering the global skill index, step access/ownership, and future image durability.

The verification records explicitly say no post-change LinkedIn-producing run, applicable evaluation, normal workflow-step consumer, or image-producing Post has run yet. Their `verification_inconclusive` state is therefore correct.

### Approval-control repair lifecycle inconsistency

The approval-control defect `PUL-4A9661D7` was changed in `planning/plan.json`, and the changelog records modifications to the resolver and publisher control path. An inert six-case gate model and structural consistency check passed.

However, its attempt `fix-a4509c7516db6bd9` still has:

- attempt status: `fixing`
- `completed_at`: empty
- `changed_files_json`: empty
- no attempt-scoped verification record

The finding itself says `awaiting_run` and refers to the same attempt. This is a real lifecycle-recording bug: the implementation changed, but attempt completion, changed-file evidence, and finding transition were not committed atomically.

### Items intentionally not fixed

Seven proposal-only records were preserved:

- Add Explore/Measure schedules.
- Decide whether global web research remains part of the source policy.
- Change the draft hard gate from 150–400 to the soul contract of 150–350.
- Reweight tactic checks versus business outcomes.
- Expand evaluation coverage for actual goal measures.
- Run a participant-seeded comparison experiment.
- Establish a causal comparison for channel-driven follower/comment growth.

Most are correctly approval-gated. The 150–400 versus 150–350 mismatch is more questionable: the stable soul contract already says 150–350, so retaining 400 as the hard gate leaves a known correctness mismatch rather than a purely strategic choice.

Two platform-owned issues remain external:

- Artifact Sync Cursor missing from the Dashboard-owned `builder/improve.html`.
- The shared changelog model does not cover all evaluation and learning mutations.

One Stores Health question remains with the user: where protected voice and AgentWorks rules should be authoritative.

### Incomplete or stale remaining work

- Six raw runtime/pre-validation records remain merely `filed`; several should have been linked or automatically closed.
- Historical `image_assets` rows were deliberately not migrated. The future writer was repaired, but old absolute run-folder paths remain in SQLite.
- The dashboard remains stale because its 21-item count did not match SQLite's 22-item open-like count.
- The approval attempt remains stuck in `fixing` despite the corresponding plan changes.
- No post-fix producing run has yet converted the 16 changed findings into verified passes or failures.

### Resolution conclusion

The fixer did real and generally sensible implementation work. It did not fix everything, and the current database correctly avoids claiming that it did. The most accurate summary is:

- 1 changed and verified.
- 3 closed from fresh no-change evidence.
- 16 genuinely changed but awaiting real-run proof.
- 6 duplicate records tied to those pending changes.
- 16 proposal, external, user-decision, or raw-filed records remain unresolved.

The next meaningful verification step is a safety-permitted Engage/Post producing run followed by reviewer verification against the existing backlog. It should not create reworded duplicate findings.

## Pulse lifecycle and presentation problems exposed by this run

### Cross-review deduplication is insufficient

Artifact Review registered six issues already owned by other reviewers. Identity should be based on affected behavior, expected outcome, and observed failure. Corroborating evidence should append to the canonical issue.

### Temporary failures remain filed

Pre-validation concerns such as missing fields remained visible after the agent repaired the artifact and the final gate passed. These should close automatically or become attempt evidence.

### Observations are being treated as defects

Safety state and successful fallback behavior were filed alongside repairable bugs. They should be telemetry or evidence linked to a finding.

### Proposals and defects share one backlog presentation

Strategy hypotheses, measurement proposals, platform gaps, user decisions, and correctness bugs currently inflate the same headline count. The UI should expose their different types and owners.

### Verification is waiting on a producing run

The reviewed Engage run occurred before the Pulse fixes. The repaired behavior therefore has not executed yet. `verification_inconclusive` and `awaiting_run` are accurate for those repairs; they should not be described as failed fixes.

### One fix-attempt lifecycle appears inconsistent

The approval-control attempt `fix-a4509c7516db6bd9` remains recorded as `fixing` without `completed_at`, even though its finding is `awaiting_run` and the module reports the repair as applied. Attempt and finding state should transition atomically.

### The generated dashboard is stale

The dashboard stage wrote 21 open items while SQLite contained 22. Contract validation failed, the previous `builder/improve.html` was restored, and the visible dashboard therefore shows older July-era counts. The SQLite finding tables are the source of truth for this audit.

## Recommended system corrections

1. Use one canonical issue per affected behavior across all reviewers.
2. Append reviewer evidence and recurrence to that issue instead of filing duplicates.
3. Give every record an explicit type: defect, verification, proposal, platform gap, user decision, or telemetry.
4. Auto-close transient pre-validation findings after the repaired artifact passes its final gate.
5. Store safety state and successful fallbacks as observations, not open findings.
6. Keep strategy proposals and user decisions out of the repairable-bug headline count.
7. Make attempt-state and finding-state updates atomic.
8. Derive dashboard counts directly from the same canonical SQLite query used by validation.
9. Consider parallelizing independent read-only reviewers after shared resource and rate-limit behavior is verified.
10. Track reviewer wall time, fresh input, cached input, output, tool calls, and cost as first-class Pulse run metrics.

## Final conclusion

The review pass was valuable: it found genuine LinkedIn automation, evaluation, data-retention, and approval-control defects. Bug Review and Eval Health provided the most actionable correctness evidence.

The raw backlog count is nevertheless misleading. It combines real bugs with duplicates, proposals, platform handoffs, user decisions, transient validation events, and operational observations. Pulse should retain all of that evidence, but present and count it according to type and canonical issue identity.
