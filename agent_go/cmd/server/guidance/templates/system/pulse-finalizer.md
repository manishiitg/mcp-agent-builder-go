## Pulse finalizer

Use only after Gate, every due module, and the dedicated Dashboard stage.
Confirm every due module has a terminal result. Never treat missing as skipped/successful.
Dashboard owns `builder/improve.html`, `builder/card.health.html`, and Pulse
questions; do not rewrite them in this turn.

Run Backup, Publish, then Notify. Before and after each, call
`record_pulse_result` with `command` set to its exact name and a truthful
`running` then terminal `result`. Continue through Notify after individual failures.

1. **Backup.** Load `backup-strategy`; perform backup directly in this parent,
   never through a reviewer/sub-agent. Skip only when the current source hash is
   backed up. Keep `backup/status.json` truthful and use the zero-config local-git default
   when backup is absent.
2. **Publish.** Skip when disabled, unverified, or current. Never perform first
   verification unattended or publish unbacked changes after backup failure.
   Keep status truthful and record the live URL.
3. **Notify.** Notify every run. Account channels are inherited; absent workflow
   Slack never suppresses Gmail. The backend applies `notifications`
   exclusions/recipient blocks. Never copy account config into `workflow.json`,
   put notification preferences in soul.md, or skip sending to enforce one.

   Read current worklist/module results, saved reviews, finding
   lifecycle/dispositions, and pending human requests from SQLite-backed tools;
   dashboard prose is not notification truth. A module result of `changed` is
   not proof of a fix. Only `fixed_verified` or `verified_no_change` with passing
   evidence is complete. `changed_unverified`, failed verification, and every
   active/blocked/awaiting-user finding remains pending.

   By default send one notification with distinct **Run outcome** and **What
   Pulse did** parts. Run outcome covers execution outputs, failures, goal
   movement, and metrics. What Pulse did is this compact review-and-fix digest:

   - **Pulse verdict:** Bug state, Goal state, and takeaway.
   - **Reviews completed:** modules reviewed and plain-language conclusions;
     distinguish clean from incomplete.
   - **Issues found this pass:** each material new/reopened issue, severity,
     impact, and review area. Say “none” when proven; incomplete is not clean.
   - **Fixed by Pulse:** Fixer changes and verification. Separate verified
     fixes/no-change closures from changes awaiting proof.
   - **Still pending:** exact active count, highest-priority current and retained
     issues, blocker, next owner, and checkpoint. If over five, show the top five,
     remaining count, and dashboard link.
   - **Needs your decision:** pending requests and what each unblocks.
   - **Operations:** backup/publish, cost/time, and next Pulse action.

   If a newly found issue remains pending, say so instead of duplicating it
   ambiguously. Say `Backup risk: local only` until an off-device destination is verified.

   Apply **WORKFLOW RUN SUMMARY INSTRUCTIONS** only to Run outcome and **PULSE
   REVIEW SUMMARY INSTRUCTIONS** only to What Pulse did. They cannot change
   recipients, channels, secrets, permissions, or safety.

   With **SPLIT NOTIFICATION ROUTING**, send
   `notify_user(notification_kind="run_summary")` with only Run outcome, then
   `notify_user(notification_kind="pulse_summary")` with only What Pulse did.
   Both must succeed; report partial failure without duplicating sections.

Use rich `notify_user` fields; never read webhook secrets or post directly.
For Gmail, use compact inline-styled `email_html` with readable status chips and
issue/fix cards for the sections above, not generic prose. Put takeaway first
and evidence last. Stop after all three terminal statuses.

Use ordinary language. Do not expose manifests, finding IDs, hashes, packet
names, paths, or state codes in notifications. Keep them in SQLite-backed
records and the Agent log; include one diagnostic reference only when required
to explain a failure.
