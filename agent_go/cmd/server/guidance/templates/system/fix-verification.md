## Fix evidence and recurrence monitoring

The single contract for recording the evidence available when a bounded repair
is applied. It is
the same standard whether the fix is applied inside scheduled Pulse, a manual
`/pulse-fixer` pass, or an approved measurement change. Load it
before applying any fix, and judge every "fixed" claim against it.

### Post-change evidence boundary

Every fix is judged against a **post-change evidence boundary**.
Immediately before a mutation, record the mutation start time, the canonical
target identity and the key/record changed, the pre-change hash or version, and
the latest relevant pre-change run/artifact ids. Everything produced before that
boundary is **baseline only, never proof** that the change works — an older
successful artifact is baseline, not verification.

### What counts as valid verification

Accept a fix as verified only from one of:

- **(a)** a side-effect-free deterministic check run *after* the mutation that
  exercises the changed canonical state through its **real runtime consumer
  path** — not the store inspected in isolation; or
- **(b)** a fresh execution, eval, or report artifact created *after* the
  mutation that carries matching run, step, target, and provenance.

Verify that the real runtime consumer actually reads the changed canonical
store: a successful write alone is not proof. File existence, mtime alone, a
successful write, or rereading an older successful artifact is **not** proof.

### When proof needs a future run

If stronger proof requires an externally side-effecting run or the next
scheduled producing run, do **not** trigger that run merely to verify. Record
the successfully applied repair as `changed_unverified`; the issue closes under
the issue-register lifecycle. A later normal run that produces the same
semantic concern must reuse the existing `issue_id`, append its evidence, and
reopen it. Do not create a verification-only run or a second issue.
