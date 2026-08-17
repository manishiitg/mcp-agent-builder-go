Help me set up or review notifications for Chief of Staff.

{{context}}

- First review the saved Chief of Staff notification configuration. Explain the effective destinations and whether the Slack webhook secret reference is healthy. Never reveal or write a webhook URL to config files, prompts, logs, or ordinary files.
- Notifications are agentic: Chief of Staff decides when a non-blocking FYI, alert, progress update, or completion notice is useful and chooses the content. Delivery is deterministic: call notify_user and let the backend apply the configured Chief of Staff Slack webhook plus enabled account-level notification channels. Slack is rich Block Kit by default; for structured summaries set slack_title, factual slack_color, slack_fields, slack_sections, and slack_footer on that same call. Never access or post to a webhook URL directly. The same setting applies to interactive Chief chats and scheduled Chief of Staff runs.
- Ask what events should notify and what a useful message should contain. Treat those as agent guidance, not routing. If I explicitly want the preference remembered, confirm it and use the existing Chief of Staff memory mechanism; never put preferences or credentials in the capabilities JSON.
- To configure Slack, call list_secrets first. If I provide a new Slack Incoming Webhook URL, store it with set_user_secret(name="SLACK_NOTIFICATION_WEBHOOK_URL", value=<url>), then call update_chief_of_staff_notifications(slack_webhook_secret_name="SLACK_NOTIFICATION_WEBHOOK_URL"). The configuration tool validates the encrypted secret. To disable the dedicated webhook, call update_chief_of_staff_notifications(slack_webhook_secret_name="").
- Gmail is an inherited account-level notification channel.
- Do not add a routing step or notification schedule merely to choose a channel.
- If I ask to test delivery, call notify_user once with a clearly labeled test message and report its delivered/skipped/failed channels honestly. Do not test unless requested.
- human_feedback is separate: use it only for short-lived input that must block this run, such as OTP, CAPTCHA, or immediate approval.
