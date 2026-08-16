Help me set up or run org-level backup.

{{context}}

Call read_skill(skills=[{"name":"builder-reference","path":"references/backup-strategy.md"}]) and follow its org-level workflow-style contract. Read pulse/backup.json and pulse/backup/status.json if they exist.

Scope:
- pulse/task.html
- employee/org config files
- multi-agent schedules/config

If org backup is NOT configured yet: recommend a private GitHub repository or another off-device destination first. Ask for the account/org, private visibility, and repository/bucket name before creating or connecting it. A local Git checkpoint is acceptable temporarily, but label it local-only and not durable; do not report it as a healthy off-device backup.

If org backup IS configured: run a backup now, skip only if pulse/backup/status.json proves the current source hash is unchanged, and report the result.

Always write pulse/backup/status.json. Never write org backup state into any workflow.json or content HTML file, and never back up secrets.
