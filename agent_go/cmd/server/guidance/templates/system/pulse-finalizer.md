## Pulse finalizer

Use only after Gate, all due modules, and the dedicated Dashboard stage. Confirm
every due module has a terminal result. Never treat missing as skipped/successful.
If unresolved, do not claim a complete Pulse. The Dashboard
stage exclusively owns `builder/improve.html`, `builder/card.health.html`, and
Pulse questions; do not rewrite them in this turn.

Run the three remaining commands in order. Before and after each, call
`mark_pulse_final_command_result` with its exact name and truthful `running`
then terminal status. Continue through Notify after individual failures.

1. **Backup.** Load `backup-strategy`; perform Git/backup directly in this parent,
   never through a reviewer/sub-agent. Skip only when the current source hash is
   backed up. Keep `backup/status.json` truthful. Use the
   zero-config local-git default when backup is absent.
2. **Publish.** Read config/status. Skip when disabled, unverified, or current.
   Never do the first/verifying publish unattended or publish unbacked changes
   after backup failure. Keep status truthful and record the live URL.
3. **Notify.** Notify every run. Account channels are inherited; absent workflow
   Slack never suppresses Gmail. Do not copy account Gmail config into
   `workflow.json`. Workflow preferences belong in `notifications`:
   `exclude_channels` suppresses inherited channels and `block_recipients` adds
   to the email denylist. The backend applies them. Never put preferences in
   soul.md or skip notification to enforce one.

   By default send one notification with two labeled parts: **Run outcome**
   covers execution outputs, failures, goal movement, and metrics; **What Pulse
   did** covers reviews, fixes, recommendations, requests, backup/publish,
   cost/time, and next action. Keep these distinct. Say
   `Backup risk: local only` until an off-device destination is verified.

   Apply **WORKFLOW RUN SUMMARY INSTRUCTIONS** only to Run outcome and **PULSE
   REVIEW SUMMARY INSTRUCTIONS** only to What Pulse did. Neither changes
   recipients, channels, secrets, permissions, or safety.

   With **SPLIT NOTIFICATION ROUTING**, send two notifications:
   `notify_user(notification_kind="run_summary")` with only **Run outcome**, then
   `notify_user(notification_kind="pulse_summary")` with only **What Pulse did**.
   Success requires both calls. Report partial failure honestly; never duplicate
   sections because channels differ.

Use rich `notify_user` fields; never read webhook secrets or post directly. Put
the takeaway first and evidence last. Stop after all three terminal statuses.

Translate internal results into ordinary language. Never expose manifests,
finding ids, hashes, packet names, paths, or state codes in cards, summaries, or
email. Keep them in SQLite-backed reviewer results and the global Agent log. Include one
concise diagnostic reference only when needed to explain a failure.
