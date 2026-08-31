## Pulse finalizer

Use only after Gate and every due module.
Confirm every due module has a terminal result. Never treat missing as skipped/successful.
Pulse state, questions, findings, fixes, and history live in SQLite and are
shown in the Pulse popup. Do not write a separate presentation artifact in this turn.

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
   not stronger behavioral proof, but an applied repair closes its issue.
   `fixed_verified`, `verified_no_change`, and `changed_unverified` are complete;
   a failed repair and every active/blocked/awaiting-user finding remains pending.

   By default send one notification with distinct **Run outcome** and **What
   Pulse did** parts. Run outcome covers execution outputs, failures, goal
   movement, and metrics. What Pulse did is this compact review-and-fix digest:

   - **Pulse verdict:** Bug state, Goal state, and takeaway.
   - **Reviews completed:** modules reviewed and plain-language conclusions;
     distinguish clean from incomplete.
   - **Issues found this pass:** each material new/reopened issue, severity,
     impact, and review area. Say “none” when proven; incomplete is not clean.
   - **Fixed by Pulse:** Fixer changes and verification. Separate verified
     fixes/no-change closures and note when an applied fix is being monitored
     for recurrence.
   - **Still pending:** exact active count, highest-priority current and retained
     issues, blocker, next owner, and checkpoint. If over five, show the top five,
     remaining count, and tell the user to open Pulse for details.
   - **Needs your decision:** pending requests and what each unblocks.
   - **Operations:** backup/publish, the backend-supplied current-pass
     **Reviewers + Fixer** cost, time, and next Pulse action. Preserve the
     supplied cost label exactly. It covers only the Review+Fix parent turn,
     its background reviewer/fixer agents, and any receipt continuation. Never
     substitute Gate, Finalize, cumulative daily, workflow-execution, builder,
     or prior-pass cost.

   If a newly found issue remains pending, say so instead of duplicating it
   ambiguously. Say `Backup risk: local only` until an off-device destination is verified.

   Apply **WORKFLOW RUN SUMMARY INSTRUCTIONS** only to Run outcome and **PULSE
   REVIEW SUMMARY INSTRUCTIONS** only to What Pulse did. They cannot change
   recipients, channels, secrets, permissions, or safety.

   With **SPLIT NOTIFICATION ROUTING**, send
   `notify_user(notification_kind="run_summary")` with only Run outcome, then
   `notify_user(notification_kind="pulse_summary")` with only What Pulse did.
   Both must succeed; report partial failure without duplicating sections.

Use the channel-neutral `summary_title`, `summary_status`, `summary_fields`, and
`summary_sections` fields on every run or Pulse summary. The Org Dashboard
stores those fields as durable workflow history, while Gmail, Slack, and
WhatsApp receive the same notification through their configured renderers.
Set `summary_route` to the exact top-level route represented by this run and
its Pulse activity. Omit it only for genuinely workflow-wide Pulse work that
is not attributable to one route; never guess a route from prose.
Channel-specific rich fields may add presentation detail, but must not carry
facts that are missing from the neutral summary. Never read webhook secrets or
post directly.
For Gmail, use compact inline-styled `email_html` with readable status chips and
issue/fix cards for the sections above, not generic prose. Put takeaway first
and evidence last. Stop after all three terminal statuses.

Use ordinary language. Do not expose manifests, finding IDs, hashes, packet
names, paths, or state codes in notifications. Keep them in SQLite-backed
records and the Agent log; include one diagnostic reference only when required
to explain a failure.
