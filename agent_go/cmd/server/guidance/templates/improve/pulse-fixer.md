# INDEPENDENT PULSE FIXER

Run only after Technical Review (`/engineering-review`) has classified current workflow evidence.
Do not rerun Technical Review, Strategic Review, or broad
discovery. Do not treat raw workflow observations as repair issues.{{if .Focus}}

Focus especially on: {{.Focus}}. The focus affects priority only; it cannot
promote an observation or bypass evidence.{{end}}{{if .RunFolder}}

Use `{{.RunFolder}}` as the primary retained run when it is relevant to the
selected issue's proof boundary.{{end}}

1. Load `read_skill(skills=[{"name":"builder-reference","path":"references/pulse-fixer-practices.md"},{"name":"builder-reference","path":"references/fix-verification.md"}])`.
2. Use `pulse_run_id="current"`. Read the saved Gate worklist and
   `get_pulse_state(view="backlog", detail="compact")` exactly once. Select the
   coherent objective from that index, then request `detail="full"` only for
   its exact issue IDs. Read its typed review records, attempts, verification,
   and the current Technical Review checkpoint. The compact backlog's `issues` feed
   is the repair queue. Workflow observations are evidence
   from reviewers and must not be repaired unless a reviewer already promoted them.
3. Select at most one highest-value coherent canonical repair objective. It may
   include several issue IDs only when they share one root cause, compatible
   targets, and one proof boundary. Preserve every unselected issue unchanged.
4. Apply the smallest complete safe repair with normal Workflow Builder tools.
   Record exact changed targets, attempts, dispositions, and proportional
   post-change proof. Use `changed_unverified` when the real evidence boundary
   requires a future producing run.
5. Record the repair attempt and each issue disposition, but do not write or
   replace either reviewer's terminal receipt. Review completion and repair
   outcome are separate facts. If no safe canonical objective exists, preserve
   the truthful review receipts without manufacturing a repair.
6. Finish with a concise statement of the selected objective, changes, proof,
   lifecycle outcomes, and the remaining canonical queue.
