[← Pulse platform index](../pulse_platform_issue_register.md)

# PLAT-236 — A validation_schema could pair a non-string `value_type` with a `pattern`, which no value can ever satisfy

| Coordination | Value |
|---|---|
| Assigned agent | Claude Code |
| Ticket state | `implemented; runtime reverify` |
| Last synchronized | `2026-08-29` |

- **Priority:** harness_issue (write-time guardrail gap), severity high on
  the findings.
- **Findings:** Twitter/social-media `PUL-08AC60BB`, `PUL-ED75E920` — both
  describe the identical mechanism: `$.reach_snapshot_table_updated`'s
  validation check had both `value_type=boolean` and a string-only
  `pattern` (`^true$`), so a real boolean `true` failed the pattern check
  ("Pattern validation only applies to strings") while coercing the value
  to a string to satisfy the pattern failed the type check instead — no
  JSON value could ever pass.

## The underlying finding was already resolved by the workflow itself

Both findings were first seen `2026-08-25T03:34–44Z`. The workflow's own
`planning/changelog/changelog-2026-08-27-15-24-56.json` shows an
`update_validation_schema` call at `2026-08-27T15:24:56Z`, reason:
*"PUL-155B3EBD: remove only the string-only `^true$` pattern from the
boolean `reach_snapshot_table_updated` check; preserve boolean typing and
every companion file/DB guard."* The live `planning/plan.json` today has
only `{"path":"$.reach_snapshot_table_updated","must_exist":true,
"value_type":"boolean"}` — no `pattern`. Same "real when filed, fixed by
the workflow's own subsequent config edit, never re-verified in the
findings" shape as PLAT-228/PLAT-230.

That fix is workflow-authored config content (`validation_schema` edited
via `update_validation_schema`), not this repo's code — the correct owner
for the original bug is the workflow's own Pulse Fixer, which already
acted. What remained a genuine platform gap: nothing in this repo's schema
write path would have caught the contradiction before it reached a live
step, and nothing stops the same shape from being authored again.

## Fix

Added `validateValueTypePatternCompatibility`, alongside the three existing
write-time schema validators (`validateRegexPatternsInSchema`,
`validateJSONPathSyntax`, `validateArrayLengthConsistencyChecks`) at all
four call sites that accept a `validation_schema`/`pre_validation` payload:
`update_validation_schema`'s executor, the generic step-update path
(`updateSingleStep`), step-creation validation, and evaluation-plan
normalization. Rejects any check pairing `pattern` with a `value_type`
other than `""`/`"string"`, naming the offending path and value_type.

Deliberately not touched: `update_evaluation_plan`'s own `pre_validation`
write path (`evaluation_plan_tool.go`, PLAT-227) calls none of these four
validators at all — it edits the raw JSON map directly rather than through
`PartialPlanStep`. That is a real, separate gap (any of the four
contradiction classes could reach a live evaluation step through that
tool untouched), out of scope for these two findings and left for a
follow-up.

## Verification

4 new tests in `value_type_pattern_compatibility_test.go`: rejects the
exact boolean+pattern shape from the findings, allows `string`+pattern,
allows a non-string `value_type` with no pattern, and a nil-schema no-op.
`go build ./...`, `go test ./pkg/orchestrator/agents/workflow/step_based_workflow/...`,
and `go test ./cmd/server/...` all pass.

## Reverify

No live schema-authoring call has exercised this guard against a real
contradictory schema yet. Reverify by confirming a future
`update_validation_schema`/step-creation call that pairs a non-string
`value_type` with a `pattern` is rejected with a clear error instead of
silently landing an unsatisfiable check.
