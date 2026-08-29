[← Pulse platform index](../pulse_platform_issue_register.md)

# PLAT-229 — a JSON array value could pass a `value_type=string` pre-validation check

| Coordination | Value |
|---|---|
| Assigned agent | Claude Code |
| Ticket state | `implemented; runtime reverify` |
| Last synchronized | `2026-08-29` |

- **Priority:** P2 in the audit queue, but `severity: high` on the finding
  itself — a shared correctness bug in message-sequence/step pre-validation,
  not a cosmetic gap.
- **Owner:** `agent_go/pkg/orchestrator/agents/workflow/step_based_workflow/pre_validation.go`,
  `validateJSONCheck`/`validateValueType` — the shared JSON schema validator
  behind every step's `validation_schema`/`pre_validation` checks
  platform-wide, not workflow-scoped code.
- **Finding:** LinkedIn `PUL-61C84987`.

## The reported symptom

A retained `engagement_targets.json` artifact had `$.notes` typed `array` in
practice. Its automatic pre-validation receipt recorded
`$.notes value_type Passed=true` under `value_type=string` and an overall
`overall_pass=true` — a wrong-shaped artifact passed as if it matched its
declared schema.

## Root cause, confirmed in code

`validateJSONCheck` extracts the value at `check.Path` before type-checking
it against `check.ValueType`. `jsonpath.Get` (PaesslerAG/jsonpath) returns
Go's `[]interface{}` for two genuinely different situations that are
indistinguishable by type alone:

1. The path names exactly one location, and that location's own JSON value
   happens to be an array (e.g. `$.notes` where `notes: ["some note"]`).
2. The path contains wildcard/filter/slice syntax and matched multiple
   separate locations (e.g. `$.checks[*].name`).

The existing code resolved that ambiguity using `check.ValueType` as a
heuristic: if the expected type was `"array"`, treat the `[]interface{}` as
the value itself; otherwise, treat it as a multi-match collection and take
its first element for a scalar check. That heuristic is exactly backwards
in the failure case that matters: when the *actual* data is an array but the
*schema* expects `"string"` — precisely a type mismatch the check exists to
catch — the code took the array's first element instead. For a
string-containing array like `["some note"]`, that first element is itself
a string, so `validateValueType` saw a string and passed. The wrong
container type slipped through undetected.

## Fix

Added `jsonPathHasMultipleMatches(path string) bool` — a small pattern
match for genuine JSONPath multi-match syntax (`*`, `..`, `?(`, slice/union
index groups like `[0:2]`/`[0,1]`). A plain numeric index (`[0]`, per the
struct's own doc-comment example `"$.databases[0].name"`) or a bare field
path does not match; the struct's own example of a definite path only
proves one location. The extraction now only applies the multi-match /
"take first element" handling when the path genuinely can match more than
one location. For a definite path, whatever `jsonpath.Get` returns *is* that
one location's value, whatever its type — never unwrapped.

## Verification

```text
go test ./pkg/orchestrator/agents/workflow/step_based_workflow/...
go test ./pkg/orchestrator/... ./cmd/server/...
```

New coverage:

- `TestValueTypeCheckRejectsAnActualArrayValueOnADefinitePath` — reproduces
  the exact reported shape (a non-empty string-array, an empty array, and a
  multi-element array), all now correctly rejected against
  `value_type=string`.
- `TestValueTypeCheckStillPassesARealString` — the finding's own positive
  control.
- `TestValueTypeCheckStillAcceptsAnArrayValueWhenArrayIsExpected` — proves
  the pre-existing, correct `value_type=array` case is untouched.
- `TestValueTypeCheckStillUnwrapsGenuineWildcardMatches` — proves the fix is
  scoped to definite paths only; a real wildcard multi-match still takes its
  first result exactly as before.
- `TestJSONPathHasMultipleMatchesDistinguishesDefiniteFromWildcardPaths` —
  pins the exact boundary, including that `[0]` (a definite index) is not
  mistaken for multi-match syntax merely because it contains brackets.

Full existing package and broader orchestrator/`cmd/server` suites pass
unchanged.

## Reverify

No live pre-validation run has exercised the corrected path through the
deployed server yet. Reverify per the finding's own `next_check`: run
scratch `engagement_targets.json` fixtures under the retained schema and
confirm `notes=[]` now fails `$.notes value_type=string` with
`overall_pass=false`, while `notes:"..."` (string) still passes.
