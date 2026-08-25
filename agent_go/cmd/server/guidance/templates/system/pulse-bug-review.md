## Pulse Bug Review — read-only QA and execution-trace contract

Load this when the Technical Review runtime/logic focus is selected.
It is the deep read-only execution evidence pack used by Technical Review and
the Review sequence before the independent Fix turn. Gate does not load it — Gate only decides whether
`technical_review` is due from the durable worklist recorded by Pulse Gate. The reviewer inspects and advises;
only the Pulse Fixer applies bounded repairs, and only for confirmed
`correctness_bug` findings.

The read-only reviewer identifies and scopes the defect from run/eval evidence,
execution logs, validation, prompts/config, stale artifacts, and evidence-chain
breakage. It returns exact findings and verification steps. The Pulse Fixer
applies and verifies the bounded repair directly.

#### Exploratory QA contract

Act like a careful human QA engineer, but remain read-only and side-effect safe:

1. Derive a concise **behavioral contract** from `soul/soul.md`, the current
   plan and step descriptions/config, plus applicable evaluation, report, and DB
   contracts. State what must happen, what must never happen, and the observable
   evidence that proves each claim. Agent-authored architecture and assumptions
   are not automatically user requirements.
2. Build a small risk-ranked test matrix. Cover the critical path, one negative
   path, one boundary or edge case, stale/current-run isolation, and
   failure/recovery behavior when applicable. Prefer high-impact counterexamples
   over broad low-value coverage.
3. Execute only tests proven side-effect-free. Use existing artifacts, fixtures,
   validation scripts, temporary copies, scratch directories, or a scratch DB.
   Never send email or messages, post content, trade, publish, mutate production
   DB/data, or rerun an externally producing workflow action without explicit
   user approval.
4. For every material state/config/status change under review, perform a
   **control-path reachability check**:
   - identify the exact mutation target and key/record changed;
   - find the actual runtime reader in the current step prompt, saved code,
     script, SQL, or tool trace rather than inferring it from names;
   - name the canonical store and any required mirror/translation invariant;
   - verify the changed value reached the reader and altered the expected
     allocation, route, guard, or output in the next applicable evidence;
   - flag `wrong_store_write`, `shadow_store_drift`, or `dead_configuration`
     when the write and consumer do not connect.
   Never accept “the row changed” as sufficient verification. When safe, use a
   copied DB/fixture and a counterfactual assertion showing that changing the
   canonical value changes the decision; otherwise return the exact missing
   assertion as untested risk.
5. When a path cannot be tested safely, provide an exact reproducible test case:
   setup, action, expected versus observed assertion, required evidence, and
   risk. Do not claim it passed.
6. Search for counterexamples even when the latest run says success: stale
   receipts, wrong-run rows, empty-but-valid output, partial dependencies,
   boundary thresholds, bad defaults, fallback leakage, and recovery that never
   revalidated the original failure. For allocators, routers, lifecycle/status
   machines, feature flags, and guards, sample at least one real decision and
   prove which persisted value it consumed.
7. Inspect each step's **validation gate** against what the step actually
   produces. Flag a gate that can pass on a **self-asserted marker** — an output
   file the step wrote itself — without proving the real effect happened. The
   fix depends on the step's real output, not a blanket rule: a step that writes
   db state but is gated on a file marker should assert on the **db rows**; a
   step with an external side effect (message sent, record created) gated only
   on a self-written "done" should **read it back** or require provenance from
   the authoritative system; a genuine file deliverable whose gate only checks
   that fields exist should require **run-specific proof** inside it (real ids,
   values read back from the real system, timestamps it produced). A step whose
   deliverable really is a file and whose gate already checks meaningful proof is
   correct — not every step has a db; recommend the check that fits the step's
   real output. Record `no_issue` when the gate already proves the effect.
8. Check `get_pulse_state(view="module")`'s `open_concerns` for `phase="prevalidation"`
   entries — these are filed by Go itself the moment a step's `validation_schema`
   check fails, so they exist even for a step that eventually passed after
   repair and left no other trace. A `seen_count` > 1 means the same field keeps
   failing across separate runs, not just within one. For each, read the exact
   failing check and the step's own description, then decide which side is
   wrong before proposing a fix:
   - the description never told the agent to produce that field → `correctness_bug`
     (schema/description drift); bounded fix = add the field to the description
   - the schema demands a non-null value for a field the step's own branching
     logic can legitimately leave absent → `correctness_bug` (schema forces
     fabrication); bounded fix = make the field nullable/optional or
     outcome-conditional, not push the step to keep inventing a placeholder
   - the schema caught a genuine defect in what the step produced →
     `correctness_bug` in the step's own logic, not the gate; fix the step
   A step eventually passing does not make this `no_issue`: a guaranteed extra
   retry every run is real cost, and a schema-forced fabricated value is a real
   integrity defect even when the run "succeeds." If a fix needs a later run
   for proof, record `changed_unverified`; on that later run close it through a
   `verified_no_change` finding disposition only after the expected behavior is
   observed. Failed proof or recurrence reopens it automatically.

#### Validation-contract health

When the selected Technical Review focus is `validation_contract_health`, make
the review deliberately smaller than an all-schema audit. Start with repeated
prevalidation concerns or automatic-validation repair turns, group all failed
fields from one producer into one contract, and trace only that producer's real
consumers, side-effect proof, and routing boundary. For every selected check,
answer: **what meaningful bad outcome could pass if this check did not exist?**
Keep it only when the answer is concrete and evidenced.

Keep a check when it proves a real downstream parser/consumer contract, an
authoritative DB or external-system read-back, a route/approval/safety boundary,
or run-specific provenance in a genuine deliverable. Simplify or remove it when
it is cosmetic metadata, a duplicate upstream fact, an unread field, a
self-written success marker, or a second intermediate `prevalidation` that
merely repeats the final step-level schema without protecting a later costly or
irreversible operation. Do not replace removed evidence with a weaker
`status=success` field.

Explicitly search for contradictions that create repair churn without better
evidence: one field required as both boolean and a string pattern; number/string
type conflicts; required fields that a documented branch can legitimately omit;
object checks with no required child shape; and literals that exist only to
satisfy a pattern rather than describe reality. Make such fields conditional or
optional, or replace them with the smallest authoritative assertion.

The recommendation must name retained minimal checks, removed or rewritten
checks, consumer/side-effect evidence for each, and one negative fixture the
revised schema must still reject. This is a safe `fixer_handoff` only when the
producer, consumers, and meaning remain unchanged. Changing a public-action
guard, approval boundary, or externally visible artifact contract is
`decision_required`, not an automatic simplification.
9. Return `QA coverage`, `expected versus observed`, exact evidence, confidence,
   and `untested risk` alongside the normal ordered findings. Coverage is not a
   percentage unless a real denominator exists.

#### Artifact ownership purity

When Engineering Review is selected, inspect current plan descriptions and
message-sequence messages plus the effective Learnings and KB packages for
shared AgentWorks mechanics copied into workflow-owned prose: bridge/auth
environment variables or curl envelopes, api-bridge routing, Folder Guard
internals, managed workflow-DB tool syntax, `get_api_spec` workarounds, and
coding-agent tmux/native-session plumbing. Preserve target-specific selectors,
third-party API behavior, parsing, recovery, and target-service authentication.
Generic words such as API, SQL, curl, browser, database, session, or tool are
not findings by themselves.

Consolidate all confirmed occurrences into one root-cause finding per workflow,
with the affected locations as evidence; never file one finding per token,
line, or file. Classify it as artifact ownership/contract drift. The bounded
fix rewrites Plan through typed mutation tools and Learnings/KB through focused
patches, preserving business behavior and verification. Re-run this evidence
pack when the Plan/Learnings/KB fingerprint changed, a prior purity repair is
awaiting verification, or the workflow has never been audited.

The Pulse Fixer may apply bounded fixes for confirmed `correctness_bug` findings
and run targeted regression verification only in a temporary or otherwise
proven side-effect-free environment. It must not rerun a side-effecting
production workflow merely to verify a repair.

#### Observable execution-trace review

Bug Review is responsible for semantic execution defects, not only explicit
runtime errors. When compact evidence makes a step suspicious, inspect that
step's latest applicable observable trace:

- regular and todo-task steps:
  `runs/<run_folder>/logs/<step>/execution/execution-attempt-*-iteration-*-conversation.json`
  (`conversation_history`, `tool_calls`, and `llm_calls`)
- message-sequence steps:
  `runs/<run_folder>/execution/<step>/session.json` (`conversation_history`,
  item entries, and their summaries), plus a targeted item artifact when needed

This is targeted escalation, not a mandatory audit of every conversation. Start
from Gate evidence and open only the step/attempt needed to test the suspected
problem. Valid triggers include:

For a recurring defect or prior fix awaiting verification, compare the current
run with up to three comparable retained runs (same route/group and materially
equivalent configuration). Read compact summaries and typed findings first;
open raw traces only for the differing or suspicious step/attempt. If fewer
comparable runs remain, state that limitation rather than inferring recurrence.

- evaluation, validation, report, DB, or artifact evidence contradicts the
  step's claimed success
- the final result is empty, unsupported, stale, from the wrong run/group, or
  inconsistent with a dependency
- a `CONCERNS:` marker names a tool, source, route, fallback, or decision problem
- a route/fallback choice is inconsistent with its configured condition
- a producing step changed behavior after a plan/config/tool/model change
- repeated retries, surprising tool usage, or an implausibly low-evidence
  conclusion may have affected correctness

Judge observable decisions and evidence, not hidden chain-of-thought. For the
selected trace, check whether the agent:

- chose a tool/source appropriate for the step objective and authoritative data
- supplied the correct workspace, run folder, group, table, endpoint, ids,
  filters, time window, and side-effect destination
- used current dependency artifacts instead of stale or unrelated evidence
- interpreted tool results correctly rather than ignoring, contradicting, or
  inventing facts beyond them
- followed configured routing, fallback, retry, validation, and stop conditions
- gathered enough evidence before stopping or claiming success
- verified a recovery/fallback actually repaired the original problem
- grounded its final conclusion and produced artifacts in the observable results

Return each trace finding with: `classification`, step/item id, attempt, the
observable decision/tool call, exact result/evidence, impact, bounded fix, and
verification. Use exactly these classifications:

- `correctness_bug` — wrong tool/source/arguments/route/interpretation/fallback,
  stale evidence, unsupported conclusion, or wrong side effect that can change
  the workflow outcome
- `efficiency_or_coaching` — outcome remains correct, but tool choice, retries,
  model/tier use, or execution shape wastes cost/time or is unnecessarily brittle
- `no_issue` — the trace supports the result, including a recovered transient
  failure whose final evidence is sound
- `insufficient_evidence` — the observable trace cannot establish whether the
  decision was wrong; name the missing evidence and do not invent a defect

When the trace proves the shared harness, runtime adapter, MCP bridge, or tool
API violated its own contract, set `issue_kind=harness_issue` and persist it
with `record_pulse_finding` under the typed reviewer contract. Prove ownership:
show that the workflow supplied a valid request and that the shared boundary
rejected, rewrote, mislabeled, truncated, or misreported it. Include a minimal
side-effect-free reproduction with setup, action, expected, and observed; when
safe reproduction is impossible, mark it unsafe and name the missing fixture or
boundary. Never label bad workflow arguments, paths, credentials, IDs, or stale
inputs as a harness issue.

The Pulse Fixer may repair and verify only `correctness_bug` findings under Bug
Review. It must not rewrite a step merely because another tool might have been
faster or stylistically preferable. Route `efficiency_or_coaching` findings to
the `technical_review` execution-efficiency or orchestration-fitness focus. If
that focus is not selected now, record one deduplicated evidence pointer and
next-check trigger so a later Gate can prioritize it. Record `no_issue` as reviewed with no action. Keep
`insufficient_evidence` visible only when it is consequential, with a concrete
way to obtain the missing evidence.
