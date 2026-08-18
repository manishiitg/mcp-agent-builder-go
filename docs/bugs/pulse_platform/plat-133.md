[← Pulse platform issue index](../pulse_platform_issue_register.md)

# PLAT-133 — the P0 contract checks that a proof is registered, never that it proves what the contract claims

| Coordination | Value |
|---|---|
| Assigned agent | unassigned |
| Ticket state | `open` — designed, deliberately not implemented (see "Why this was not landed with the fix") |
| Last synchronized | `2026-08-18` |

- **Priority:** P2 — nothing is broken today and the bug that exposed this is
  fixed. This is about the *next* occurrence: the mechanism that was supposed
  to catch it did not, and still would not.
- **Owner:** `multi-llm-provider-go` — `coding_agent_certification.go`,
  `coding_agent_contract.go`, `coding_agent_contract_test.go`
- **Related:** [PLAT-116](plat-116.md) (its deferred Pi CLI exclusion, closed
  2026-08-18 by `multi-llm-provider-go@da13e17`, is what exposed this)

## What happened

`multi-llm-provider-go@da13e17` fixed a Pi CLI hang: the structured adapter's
only teardown trigger was `agent_settled`, which pi 0.84.2 no longer emits
(0 occurrences across a day of production logs vs 57 `agent_end`). Any run
where pi did not exit on its own blocked forever with no timeout — live, a
workflow step held its caller 65 minutes after finishing its real work.

The interesting part is not the bug. It is that **the P0 contract already
required a certification covering exactly this property, and passed anyway.**

## Why the existing framework missed it

The framework is sound: capability flag → required certification → real E2E
proof, enforced by `TestActiveCodingAgentProvidersSatisfyP0Contract`, which
`t.Fatal`s on a missing cert, a non-P0 priority, or a proof that is not a real
CLI E2E.

Pi already qualified. `RequiredP0CodingAgentCertificationIDs` adds
`CertStructuredMultiTurn` for any provider with `UsesPersistentSession &&
SupportsNativeResume` — Pi has both — and the requirement's own comment names
the property precisely:

> *"Every persistent provider must independently prove that the native session
> **survives process exit** and resumes on the next message"*

But the contract only verifies that *a* proof is registered under that ID. It
never checks whether the registered proof tests what the ID claims. The four
registered proofs, all satisfying the same requirement:

| provider | registered proof's own description |
|---|---|
| Claude Code | "…proves the first turn **exits naturally** after persisting a resumable native session" |
| Codex CLI | "…proves the native thread from turn one **resumes** in turn two" |
| Cursor CLI | "…proves the native session from turn one **resumes** in turn two" |
| Pi CLI | "…proves the native session from turn one **resumes** in turn two" |

Only Claude tests termination. Three of four registered a resume-only test
against a certification whose contract says "survives process exit", and the
contract test cannot tell the difference.

Two factors compounded it:
- **The proofs are live-only** (`-coding-cli-p0-live`, nightly/dispatch). A
  CLI upgrade silently removing an event is exactly the drift a live-only proof
  catches late, if at all.
- **The property was conflated.** "Resumes across turns" and "terminates when
  the CLI signals completion without exiting" ride one certification ID, so
  satisfying the easier half satisfies the requirement.

## Proposed repair

1. **Split the property into its own certification.**
   `CertStructuredTerminalEvent` — proves the structured adapter terminates
   when the CLI signals completion **but does not exit on its own**. Named for
   the failure mode so a resume-only test cannot satisfy it by accident.
2. **Gate it** the same way `CertStructuredMultiTurn` is gated
   (`UsesPersistentSession && SupportsNativeResume`) — Claude, Codex, Cursor,
   Pi.
3. **Back it with hermetic proofs, not live ones.** The drift survived because
   the only proof was live and version-sensitive. Pi's proof already exists and
   is hermetic: a fake `pi` on `PATH` that emits a fixed stream and then stays
   alive, reproducing the continued-session case
   (`picli_structured_terminal_event_test.go`, verified fail-before/pass-after —
   reverted, it times out at 60s with the goroutine parked in
   `bufio.(*Scanner).Scan`).
4. **Capture each fake CLI's stream from real output; never invent it.** Commit
   the captured fixture next to the test. See the risk note below — this is the
   step that decides whether the whole exercise is worth anything.
5. **Consider the more general fix:** a check that a registered proof actually
   covers its certification's stated intent. That is the defect class here, and
   it is not specific to this one ID. Even something coarse — requiring the
   registered `Description` to assert the contract's key property, or pairing
   each cert with a machine-checkable claim — would have caught three providers
   at once.

## Why this was not landed with the fix

Two reasons, both deliberate:

**Inventing the fakes would reproduce the exact defect.** Pi's hermetic proof is
trustworthy only because its event stream was verified against a day of real
production logs — `agent_settled` measurably absent, `agent_end` measurably
present. There is no equivalent grounding for Codex or Cursor's structured
streams. Writing fake CLIs from guessed formats yields tests that are green and
meaningless, which is precisely the failure this ticket exists to fix. The
codebase already states the principle: *"a green test that proved nothing is how
this defect shipped in the first place."*

**Landing it half-enforced would break CI on every PR.**
`TestActiveCodingAgentProvidersSatisfyP0Contract` `t.Fatal`s on a missing
required cert, so adding the requirement before Codex and Cursor have proofs
turns the contract test red for everyone. A permanently-red required check
trains people to ignore it, destroying the enforcement value that makes this
framework worth having.

Nothing is currently at risk: the underlying bug is fixed and pushed, and Pi's
hermetic proof exists and passes. This ticket is hardening against the next
occurrence, and it degrades badly if rushed.

## Acceptance

- A certification exists that specifically proves structured-transport
  termination when the CLI does not self-exit, and it cannot be satisfied by a
  resume-only test.
- Every provider that claims persistent structured sessions has a proof for it,
  each built from a captured real stream rather than an invented one.
- The proofs are hermetic and run in the fast PR job, so a CLI upgrade that
  drops or renames a terminal event fails immediately rather than at the next
  nightly — or never.
- Ideally: a registered proof can no longer silently under-cover its
  certification's stated intent.
