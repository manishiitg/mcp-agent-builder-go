import React from 'react'
import { FileText, Server, Cpu, Bot, Layers, RefreshCw, GitBranch, CheckCircle, BookOpen, Activity, BellRing, Cloud, Globe, Target } from 'lucide-react'
import type { CommandContext, CommandDefinition } from './types'

function submitGuidedWorkflowCommand(
  ctx: CommandContext,
  kind: string,
  options: { runFolder?: string | null; background?: boolean } = {}
) {
  const focus = ctx.beforeSlash.trim()
  const args = [
    `kind=${JSON.stringify(kind)}`,
    `focus=${JSON.stringify(focus)}`,
  ]
  if (options.runFolder !== undefined) {
    args.push(`run_folder=${JSON.stringify(options.runFolder || '')}`)
  }
  const guidanceCall = `get_workflow_command_guidance(${args.join(', ')})`

  // Expensive review/fix passes run as a background task so the chat stays
  // responsive. completion_mode=present_result is a backend-carried contract:
  // the synthetic parent turn only presents the returned receipt and must not
  // repeat the child's Pulse/SQLite/workspace reads.
  if (options.background) {
    const isFixer = kind === 'pulse-fixer'
    const taskLabel = isFixer ? 'fix pass' : 'review'
    const completionContract = isFixer
      ? 'then present the selected repair objective, changes made, verification proof, lifecycle outcomes, and the remaining canonical queue.'
      : 'then present a short executive summary followed by every finding and recommendation in severity order. Do not truncate the result to a Top 3.'
    const outputContract = ctx.workshopMode === 'run'
      ? 'Return findings in chat only; do not write or edit any workspace file.'
      : isFixer
        ? 'Persist repairs, proof, and lifecycle outcomes through the typed Pulse tools required by the returned guidance; do not write a separate review file.'
        : 'Persist findings, recommendations, decisions, and verification judgments through the typed Pulse tools required by the returned guidance; do not modify implementation files or write a separate review file.'
    const instruction =
      `Call ${guidanceCall} and follow the returned instructions verbatim. ${outputContract} ` +
      `Treat focus as the request context before the slash command. The tool returns the canonical guided-flow text; do not paraphrase or skip its steps.`
    const requiredReviewModule = kind === 'ops-review'
      ? 'llm_ops_review'
      : kind === 'strategy-auditor'
        ? 'strategic_review'
        : null
    const requiredReviewReceipt = requiredReviewModule
      ? `, required_pulse_review_modules=[${JSON.stringify(requiredReviewModule)}]`
      : ''
    ctx.onSubmit(
      `Run the /${kind} ${taskLabel} as a BACKGROUND task so this chat stays responsive. ` +
      `If the run_in_background tool is available: call run_in_background(name=${JSON.stringify(kind + ' ' + taskLabel)}, instruction=${JSON.stringify(instruction)}, completion_mode="present_result"${requiredReviewReceipt}) and do NOT perform the ${taskLabel} yourself this turn — you'll get a presentation-only completion notification, ${completionContract} Do not call tools, reload state, or independently revalidate after that notification. ` +
      `If run_in_background is not available, perform the ${taskLabel} inline this turn instead.`
    )
    return
  }

  ctx.onSubmit(
    `Call ${guidanceCall} and follow the returned instructions verbatim. ` +
    `Treat focus as the conversation/request context that appeared before the slash command, including the user's recent constraints and intent. ` +
    `The tool returns the canonical guided-flow text for this command — do not paraphrase or skip its steps.`
  )
}

export const builtinCommands: CommandDefinition[] = [
  {
    command: 'design-plan',
    description: 'Comprehensively review the plan, dependent artifacts, and better design options',
    icon: <GitBranch className="w-4 h-4" />,
    modes: ['workflow'],
    requiredWorkflowMode: 'plan',
    requiredWorkshopMode: ['workshop', 'run'],
    source: 'builtin',
    execute: (ctx) => {
      // design-plan already delegates its expensive audit to the dedicated
      // review_plan background tool. Keep the coordinating turn in the main
      // conversation so its completion notification can resume synthesis and
      // persist the final open findings.
      submitGuidedWorkflowCommand(ctx, 'design-plan')
    }
  },
  {
    command: 'review-artifact-drift',
    description: 'Check whether artifacts drifted from recent plan changes',
    icon: <RefreshCw className="w-4 h-4" />,
    modes: ['workflow'],
    requiredWorkflowMode: 'plan',
    requiredWorkshopMode: ['workshop'],
    source: 'builtin',
    execute: (ctx) => {
      submitGuidedWorkflowCommand(ctx, 'review-artifact-drift', { background: true })
    }
  },
  {
    command: 'improve-knowledge',
    description: 'Improve knowledge notes with targeted cleanup or cross-step consolidation',
    icon: <Layers className="w-4 h-4" />,
    modes: ['workflow'],
    requiredWorkflowMode: 'plan',
    requiredWorkshopMode: ['workshop'],
    source: 'builtin',
    execute: (ctx) => {
      submitGuidedWorkflowCommand(ctx, 'improve-knowledge')
    }
  },
  {
    command: 'improve-learnings',
    description: 'Improve global learnings with targeted cleanup or current-plan consolidation',
    icon: <BookOpen className="w-4 h-4" />,
    modes: ['workflow'],
    requiredWorkflowMode: 'plan',
    requiredWorkshopMode: ['workshop'],
    source: 'builtin',
    execute: (ctx) => {
      submitGuidedWorkflowCommand(ctx, 'improve-learnings')
    }
  },
  {
    command: 'improve-database',
    description: 'Improve durable data contracts, schemas, and report compatibility',
    icon: <Server className="w-4 h-4" />,
    modes: ['workflow'],
    requiredWorkflowMode: 'plan',
    requiredWorkshopMode: ['workshop'],
    source: 'builtin',
    execute: (ctx) => {
      submitGuidedWorkflowCommand(ctx, 'improve-database')
    }
  },
  {
    command: 'design-reporting-ui',
    description: 'Design the reporting UI from scratch: pick HTML (live data) or Markdown documents and build them',
    icon: <CheckCircle className="w-4 h-4" />,
    modes: ['workflow'],
    requiredWorkflowMode: 'plan',
    requiredWorkshopMode: ['workshop'],
    source: 'builtin',
    execute: (ctx) => {
      submitGuidedWorkflowCommand(ctx, 'design-reporting-ui')
    }
  },
  {
    command: 'improve-report',
    description: 'Improve the report dashboard for goal tracking, plan context, issues, and live data clarity',
    icon: <CheckCircle className="w-4 h-4" />,
    modes: ['workflow'],
    requiredWorkflowMode: 'plan',
    requiredWorkshopMode: ['workshop'],
    source: 'builtin',
    execute: (ctx) => {
      submitGuidedWorkflowCommand(ctx, 'improve-report')
    }
  },
  {
    command: 'improve-evaluation',
    description: 'Validate evaluation/evaluation_plan.json and improve goal/criteria coverage',
    icon: <CheckCircle className="w-4 h-4" />,
    modes: ['workflow'],
    requiredWorkflowMode: 'plan',
    requiredWorkshopMode: 'workshop',
    source: 'builtin',
    execute: (ctx) => {
      const runFolder = ctx.getWorkflowStore().selectedRunFolder
      submitGuidedWorkflowCommand(ctx, 'improve-evaluation', { runFolder })
    }
  },
  {
    command: 'pulse',
    description: 'Run one complete Pulse now against the latest retained run',
    icon: <Activity className="w-4 h-4" />,
    modes: ['workflow'],
    requiredWorkflowMode: 'plan',
    requiredWorkshopMode: 'workshop',
    source: 'builtin',
    execute: (ctx) => {
      const runFolder = ctx.getWorkflowStore().selectedRunFolder
      submitGuidedWorkflowCommand(ctx, 'pulse', { runFolder })
    }
  },
  {
    command: 'pulse-backlog',
    description: 'Semantically consolidate the Pulse issue backlog without changing workflow artifacts',
    icon: <Layers className="w-4 h-4" />,
    modes: ['workflow'],
    requiredWorkflowMode: 'plan',
    requiredWorkshopMode: 'workshop',
    source: 'builtin',
    execute: (ctx) => {
      const focus = ctx.beforeSlash.trim()
      ctx.onSubmit(
        `Consolidate the durable Pulse backlog${focus ? ` with this focus: ${focus}` : ''}. ` +
        `Load get_pulse_state(view="backlog", detail="compact") exactly once first. Work only in the typed Pulse lifecycle: do not edit workflow artifacts, run steps, or create a Markdown report. ` +
        `Request detail="full" only for the bounded issue_ids whose semantic identity is genuinely uncertain; never reload the complete backlog merely to filter it differently. ` +
        `Group issues by semantic root cause, repair owner, and verification boundary—not wording, module, evidence path, or repeated symptom. ` +
        `For each proven duplicate group, call merge_pulse_issues with one canonical PUL issue ID and the duplicate PUL IDs. Do not merge uncertain cases. ` +
        `Then give a compact receipt: active count before and after, duplicates merged, distinct root causes retained, and any ambiguous groups left for a later review.`
      )
    }
  },
  {
    command: 'pulse-setup',
    description: 'Enable Pulse and configure the recurring workflow run schedule',
    icon: <RefreshCw className="w-4 h-4" />,
    modes: ['workflow'],
    requiredWorkflowMode: 'plan',
    requiredWorkshopMode: 'workshop',
    source: 'builtin',
    execute: (ctx) => {
      submitGuidedWorkflowCommand(ctx, 'pulse-setup')
    }
  },
  {
    command: 'ops-review',
    description: 'Agentically review cost, timing, tool/runtime reliability, model routing, and setup',
    icon: <Cpu className="w-4 h-4" />,
    modes: ['workflow'],
    requiredWorkflowMode: 'plan',
    requiredWorkshopMode: 'workshop',
    source: 'builtin',
    execute: (ctx) => {
      const runFolder = ctx.getWorkflowStore().selectedRunFolder
      submitGuidedWorkflowCommand(ctx, 'ops-review', { runFolder, background: true })
    }
  },
  {
    command: 'strategy-auditor',
    description: 'Diagnose whether the current plan is moving the goal using cross-run evidence',
    icon: <Target className="w-4 h-4" />,
    modes: ['workflow'],
    requiredWorkflowMode: 'plan',
    requiredWorkshopMode: 'workshop',
    source: 'builtin',
    execute: (ctx) => {
      const runFolder = ctx.getWorkflowStore().selectedRunFolder
      submitGuidedWorkflowCommand(ctx, 'strategy-auditor', { runFolder, background: true })
    }
  },
  {
    command: 'engineering-review',
    description: 'Review Engineering and Ops and classify evidence into canonical issues',
    icon: <CheckCircle className="w-4 h-4" />,
    modes: ['workflow'],
    requiredWorkflowMode: 'plan',
    requiredWorkshopMode: 'workshop',
    source: 'builtin',
    execute: (ctx) => {
      const runFolder = ctx.getWorkflowStore().selectedRunFolder
      submitGuidedWorkflowCommand(ctx, 'engineering-review', { runFolder, background: true })
    }
  },
  {
    command: 'pulse-fixer',
    description: 'Independently repair one coherent objective from reviewed Pulse issues',
    icon: <RefreshCw className="w-4 h-4" />,
    modes: ['workflow'],
    requiredWorkflowMode: 'plan',
    requiredWorkshopMode: 'workshop',
    source: 'builtin',
    execute: (ctx) => {
      const runFolder = ctx.getWorkflowStore().selectedRunFolder
      submitGuidedWorkflowCommand(ctx, 'pulse-fixer', { runFolder, background: true })
    }
  },
  {
    command: 'goal-advisor',
    description: 'Run a one-off strategic Goal Advisor review without changing Pulse setup',
    icon: <Bot className="w-4 h-4" />,
    modes: ['workflow'],
    requiredWorkflowMode: 'plan',
    requiredWorkshopMode: 'workshop',
    source: 'builtin',
    execute: (ctx) => {
      submitGuidedWorkflowCommand(ctx, 'goal-advisor', { background: true })
    }
  },
  {
    command: 'review-code',
    description: 'Review saved scripts (main.py) against step descriptions to detect drift',
    icon: <FileText className="w-4 h-4" />,
    modes: ['workflow'],
    requiredWorkflowMode: 'plan',
    requiredWorkshopMode: 'workshop',
    source: 'builtin',
    execute: (ctx) => {
      submitGuidedWorkflowCommand(ctx, 'review-code', { background: true })
    }
  },
  {
    command: 'backup',
    description: 'Set up, run, or restore this automation’s backup',
    icon: <Cloud className="w-4 h-4" />,
    modes: ['workflow'],
    requiredWorkflowMode: 'plan',
    requiredWorkshopMode: 'workshop',
    source: 'builtin',
    execute: (ctx) => {
      const instruction = `Help me set up or run backup for this workflow. Call read_skill(skills=[{"name":"builder-reference","path":"references/backup-strategy.md"}]), then read workflow.json.backup and backup/status.json.
- If backup is NOT configured yet: recommend a private GitHub repository or another off-device destination first. Ask for the account/org, private visibility, and repository/bucket name before creating or connecting it. A local Git checkpoint is acceptable temporarily, but label it local-only and not durable; do not report it as a healthy backup.
- If backup IS configured: run a backup now and report the result (destinations, commit/ref).
- If I asked to restore: restore the tracked files from the latest backup (or a commit I name) instead.
Always write backup/status.json; never write operational status into workflow.json.`
      ctx.onSubmit(ctx.beforeSlash ? `${ctx.beforeSlash}\n\n${instruction}` : instruction)
    }
  },
  {
    command: 'publish',
    description: 'Set up or publish this automation’s report to a public URL',
    icon: <Globe className="w-4 h-4" />,
    modes: ['workflow'],
    requiredWorkflowMode: 'plan',
    requiredWorkshopMode: 'workshop',
    source: 'builtin',
    execute: (ctx) => {
      const instruction = `Help me set up or run publish for this workflow. Call read_skill(skills=[{"name":"builder-reference","path":"references/publish-strategy.md"}]) and follow it exactly, then read workflow.json.publish and publish/status.json.
- If publish is NOT configured: set it up — ask me which static host (Netlify / Vercel / Cloudflare Pages / Cloudflare R2 / S3 / any). As soon as I pick one, AUTO-CHECK its CLI (command -v) and INSTALL it for me if missing (announce it, e.g. "Installing the Vercel CLI…", then run npm i -g <cli>); do NOT ask me for an access token/API key — the path is install CLI → I run <cli> login once → you deploy. Default visibility is PRIVATE via a simple password gate (StatiCrypt with $SECRET_PUBLISH_PASSWORD and the Runloop dark gate styling from the reference doc); ask me to set a PUBLISH_PASSWORD secret, or confirm if I want it fully public instead. Then write workflow.json.publish and publish/status.json with state "configured_not_verified". Do not publish yet.
- If publish IS configured: publish the report dashboard now. Deploy dashboard.html and the nav index.html wrapper per the reference doc. Pulse is in-app only and must not be published as a separate artifact. Force every page to **DARK only** (matching the app) — set BOTH class="dark" and data-theme="dark" on the html element per the reference doc; no toggle, do NOT use prefers-color-scheme. Stage the files in a /tmp dir; if visibility is private, encrypt them with StatiCrypt ($SECRET_PUBLISH_PASSWORD) and apply the Runloop dark password-gate styling before deploying; run the deploy CLI from /tmp. Then give me the URL and confirm visibility + what's public.
CRITICAL — after deploying, come BACK to the workflow folder and persist state there (never in the /tmp staging dir): set workflow.json.publish.enabled=true with the destination + top-level url, AND write publish/status.json with state "published", the url, and last_source_hash (= the current_source_hash the backend reports; leave empty if unknown). A deploy that doesn't write these shows a grey "not configured" dot even though the site is live.
Always write publish/status.json.`
      ctx.onSubmit(ctx.beforeSlash ? `${ctx.beforeSlash}\n\n${instruction}` : instruction)
    }
  },
  {
    command: 'notify',
    description: 'Set up, review, or test agentic notifications',
    icon: <BellRing className="w-4 h-4" />,
    modes: ['workflow'],
    requiredWorkflowMode: 'plan',
    requiredWorkshopMode: 'workshop',
    source: 'builtin',
    execute: (ctx) => {
      const instruction = `Help me set up or review notifications for this workflow.
- First read the current workflow configuration. Explain the current effective destinations, saved notification instructions, and whether the Slack webhook secret reference is healthy. Never reveal or write a webhook URL to workflow.json, prompts, logs, or ordinary files.
- Notifications are agentic: the agent decides when a non-blocking FYI, alert, progress update, or completion notice is useful and chooses the content. Delivery is deterministic: the agent calls notify_user and the backend automatically applies the workflow Slack webhook plus enabled account-level notification channels. Slack is rich Block Kit by default; for structured summaries set slack_title, factual slack_color, slack_fields, slack_sections, and slack_footer on that same call. Never access a SECRET_* webhook variable, post with curl, or disable automatic Slack delivery to avoid a duplicate. Do not add a routing step merely to choose a notification channel.
- Ask separately what the workflow run summary should contain and what the Pulse review summary should contain, and whether they should use different channels or go to different people. Store only explicit, durable user-approved preferences with update_workflow_config(run_notification_instructions="...", pulse_notification_instructions="...", run_notification_channels=[...], pulse_notification_channels=[...]). workflow.json notifications.run_summary_instructions, notifications.pulse_summary_instructions, notifications.run_summary_channels, and notifications.pulse_summary_channels are authoritative; never put notification preferences in soul/soul.md. If the user says a preference applies to every notification, save it in both matching fields. Do not store temporary choices or credentials there.
- To configure a workflow Slack Incoming Webhook, use list_secrets first. If I provide a new URL, store it with set_workflow_secret(name="SLACK_NOTIFICATION_WEBHOOK_URL", value=<url>), then call update_workflow_config(slack_webhook_secret_name="SLACK_NOTIFICATION_WEBHOOK_URL"). The configuration tool validates the encrypted secret, makes it backend-only, and removes it from agent-visible secret injection. To disable workflow webhook delivery, call update_workflow_config(slack_webhook_secret_name="").
- SLACK CHANNELS: a Slack Incoming Webhook is tied to ONE channel when it is created and cannot be pointed at another, so "send this to a different channel" always means "use a different webhook". If I want the run summary and Pulse review in different channels, ask me for a webhook URL for EACH channel, store each under its own descriptive secret name with set_workflow_secret, then call update_workflow_config(run_notification_slack_webhooks=["SECRET_NAME_A"], pulse_notification_slack_webhooks=["SECRET_NAME_B"]). List several names to post one summary to several channels. Pass an empty array to fall back to the single workflow webhook. Never ask me to "pick a channel name" as if one webhook could reach several — tell me plainly that each channel needs its own webhook URL.
- Gmail is an inherited account-level notification channel. If I name who should receive this workflow's email, SAVE it — do not just use it for one send. Store it with update_workflow_config(run_notification_recipients=[...], pulse_notification_recipients=[...]); those persist to workflow.json notifications.run_summary_recipients / pulse_summary_recipients and the backend addresses every matching send from them automatically. Ask whether the run outcome and the Pulse review should go to the same people, since they are separate lists. Recipient lists say where mail GOES; they never unblock a blocked address, so if I name an address that is on a denylist, tell me rather than silently saving a list that will be skipped. Pass an empty array to clear a list back to the account default. Use the one-off email_to argument only for a single send I asked for explicitly.
- If I asked to test delivery, call notify_user once with a clearly labeled test message and report its returned delivered/skipped/failed channels honestly. Do not send a test unless I requested one.
- human_feedback is separate: use it only for short-lived input that must block this run, such as OTP, CAPTCHA, or immediate approval.`
      ctx.onSubmit(ctx.beforeSlash ? `${ctx.beforeSlash}\n\n${instruction}` : instruction)
    }
  },
]
