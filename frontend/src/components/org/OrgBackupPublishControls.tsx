import React, { useCallback, useEffect, useState } from 'react'
import { BellRing, Cloud } from 'lucide-react'
import { agentApi } from '../../services/api'
import type {
  WorkflowBackupInfoResponse,
  WorkflowBackupStrategyInfo,
} from '../../services/api-types'
import { Tooltip, TooltipContent, TooltipTrigger } from '../ui/tooltip'
import { formatBackupStateLabel, getBackupDotClass } from '../workflow/backupStatus'
import BackupPopup from '../backup-publish/BackupPopup'
import WorkflowNotificationPopup from '../workflow/WorkflowNotificationPopup'
import { loadOrgNotificationInfo, type WorkflowNotificationState } from '../../services/workflow-notifications'
import { formatNotificationStateLabel, getNotificationDotClass } from '../workflow/notificationStatus'

const ORG_BACKUP_COMMAND_MESSAGE = `Help me set up or run org-level backup.

Call read_skill(skills=[{"name":"builder-reference","path":"references/backup-strategy.md"}]) and follow its org-level workflow-style contract. Read pulse/backup.json and pulse/backup/status.json if they exist.

Scope:
- pulse/task.html
- employee/org config files
- multi-agent schedules/config

If org backup is NOT configured yet: recommend a private GitHub repository or another off-device destination first. Ask for the account/org, private visibility, and repository/bucket name before creating or connecting it. A local Git checkpoint may be used temporarily, but describe it as local-only and not durable; it must not be reported as a healthy off-device backup.

If org backup IS configured: run a backup now, skip only if pulse/backup/status.json proves the current source hash is unchanged, and report the result.

Always write pulse/backup/status.json. Never write org backup state into any workflow.json or content HTML file, and never back up secrets.`

const ORG_NOTIFY_COMMAND_MESSAGE = `Help me set up or review notifications for Chief of Staff.
- First review the saved Chief of Staff notification configuration. Explain the effective destinations and whether the Slack webhook secret reference is healthy. Never reveal or write a webhook URL to config files, prompts, logs, or ordinary files.
- Notifications are agentic: Chief of Staff decides when a non-blocking FYI, alert, progress update, or completion notice is useful and chooses the message. Delivery is deterministic: call notify_user and let the backend apply the configured Chief of Staff Slack webhook plus enabled account-level channels. Slack is rich Block Kit by default; structured summaries should set slack_title, factual slack_color, slack_fields, slack_sections, and slack_footer on that same call. Never access or post to a webhook URL directly. This applies to interactive Chief chats and scheduled Chief of Staff runs.
- Ask what events should notify and what a useful message should contain. Treat those as agent guidance, not routing. If I explicitly want the preference remembered, confirm it and use the existing Chief of Staff memory mechanism; never put preferences or credentials in the capabilities JSON.
- To connect Slack, call list_secrets first. If I provide a new Slack Incoming Webhook URL, store it with set_user_secret(name="SLACK_NOTIFICATION_WEBHOOK_URL", value=<url>), then call update_chief_of_staff_notifications(slack_webhook_secret_name="SLACK_NOTIFICATION_WEBHOOK_URL"). The configuration tool validates the encrypted secret and never exposes the URL. To disable the dedicated webhook, call update_chief_of_staff_notifications(slack_webhook_secret_name="").
- Gmail is an inherited account-level notification channel.
- Do not add a routing step or notification schedule merely to choose a channel.
- If I ask to test delivery, call notify_user once with a clearly labeled test and report delivered/skipped/failed channels honestly. Do not test unless requested.
- human_feedback is separate: use it only for short-lived input that must block this run, such as OTP, CAPTCHA, or immediate approval.`

const FALLBACK_ORG_BACKUP_STRATEGIES: WorkflowBackupStrategyInfo[] = [
  {
    id: 'git',
    label: 'GitHub / remote Git (recommended)',
    description: 'Recommended off-device protection for pulse/task HTML, schedule/config snapshots, and small text artifacts.',
    best_for: ['tasks', 'schedules']
  },
  {
    id: 'object_store',
    label: 'R2 / S3 / B2',
    description: 'Best when org artifacts grow into larger generated assets or exported dashboards.',
    best_for: ['large-artifacts', 'exports']
  },
  {
    id: 'local_zip',
    label: 'Local ZIP export',
    description: 'Manual recovery snapshot. Useful as a fallback, not a substitute for remote backup.',
    best_for: ['manual-export', 'restore']
  }
]

const OrgBackupPopup: React.FC<{
  isOpen: boolean
  onClose: () => void
  onSubmitCommand?: (query: string) => void
  onStateLoaded?: (state: string) => void
}> = ({ isOpen, onClose, onSubmitCommand, onStateLoaded }) => {
  const runInChat = useCallback(() => {
    onSubmitCommand?.(ORG_BACKUP_COMMAND_MESSAGE)
    onClose()
  }, [onClose, onSubmitCommand])

  const getSummary = useCallback((info: WorkflowBackupInfoResponse | null): string => {
    const state = info?.effective_state
    if (!info?.config?.enabled) return 'No off-device org backup is configured. Add GitHub, another remote Git host, or an object store.'
    if (state === 'local_only') return 'The org backup exists only on this laptop. Add an off-device destination for durable recovery.'
    if (info?.status?.summary) return info.status.summary
    if (state === 'stale') return 'Org task or config data changed since the last healthy backup.'
    return 'Org backup status is waiting for the Chief of Staff to update pulse/backup/status.json.'
  }, [])

  return (
    <BackupPopup
      isOpen={isOpen}
      onClose={onClose}
      loadInfo={agentApi.getOrgBackup}
      onStateLoaded={onStateLoaded}
      fallbackStrategies={FALLBACK_ORG_BACKUP_STRATEGIES}
      subtitle="Org tasks, schedules, and config"
      emptyDestinationsText="Use setup to configure GitHub, another remote Git host, or an object store. Local-only copies are not durable backups."
      destinationsHelp="The Chief of Staff writes destination status after each backup."
      statusPathFallback="pulse/backup/status.json"
      setupAction={{
        onClick: runInChat,
        label: <>Set up · run · restore in chat with <code className="ml-1 rounded bg-background px-1 font-medium text-foreground">/org-backup</code></>
      }}
      getSummary={getSummary}
      loadErrorMessage="Failed to load org backup status"
      showEnabledBadge
    />
  )
}

export const OrgBackupPublishControls: React.FC<{ onSubmitCommand?: (query: string) => void }> = ({ onSubmitCommand }) => {
  const [backupState, setBackupState] = useState('loading')
  const [notificationState, setNotificationState] = useState<WorkflowNotificationState | 'loading'>('loading')
  const [showBackupPopup, setShowBackupPopup] = useState(false)
  const [showNotificationPopup, setShowNotificationPopup] = useState(false)

  const loadOrgOps = useCallback(async () => {
    const [backup, notifications] = await Promise.allSettled([
      agentApi.getOrgBackup(),
      loadOrgNotificationInfo()
    ])
    if (backup.status === 'fulfilled') {
      setBackupState(backup.value.effective_state || 'not_configured')
    }
    if (notifications.status === 'fulfilled') {
      setNotificationState(notifications.value.effectiveState)
    }
  }, [])

  useEffect(() => { void loadOrgOps() }, [loadOrgOps])

  const buttonClass = 'relative rounded-md bg-muted p-1.5 text-muted-foreground transition-colors hover:bg-accent hover:text-accent-foreground'

  return (
    <>
      <div className="flex items-center gap-1">
        <Tooltip>
          <TooltipTrigger asChild>
            <button
              type="button"
              onClick={() => setShowBackupPopup(true)}
              title={`Backup - ${formatBackupStateLabel(backupState)}`}
              aria-label="Org backup"
              className={buttonClass}
            >
              <Cloud className="h-3.5 w-3.5" />
              <span className={`absolute -right-0.5 -top-0.5 h-2 w-2 rounded-full border border-background ${getBackupDotClass(backupState)}`} />
            </button>
          </TooltipTrigger>
          <TooltipContent>
            <p>Backup &middot; {formatBackupStateLabel(backupState)}</p>
          </TooltipContent>
        </Tooltip>
        <Tooltip>
          <TooltipTrigger asChild>
            <button
              type="button"
              onClick={() => setShowNotificationPopup(true)}
              title={`Notify - ${formatNotificationStateLabel(notificationState)}`}
              aria-label="Chief of Staff notify"
              className={buttonClass}
            >
              <BellRing className="h-3.5 w-3.5" />
              <span className={`absolute -right-0.5 -top-0.5 h-2 w-2 rounded-full border border-background ${getNotificationDotClass(notificationState)}`} />
            </button>
          </TooltipTrigger>
          <TooltipContent>
            <p>Notify &middot; {formatNotificationStateLabel(notificationState)}</p>
          </TooltipContent>
        </Tooltip>
      </div>
      <OrgBackupPopup
        isOpen={showBackupPopup}
        onClose={() => setShowBackupPopup(false)}
        onSubmitCommand={onSubmitCommand}
        onStateLoaded={setBackupState}
      />
      <WorkflowNotificationPopup
        isOpen={showNotificationPopup}
        onClose={() => setShowNotificationPopup(false)}
        workspacePath={null}
        loadInfo={loadOrgNotificationInfo}
        scopeKind="chief-of-staff"
        onStateLoaded={setNotificationState}
        onSetup={() => {
          setShowNotificationPopup(false)
          onSubmitCommand?.(ORG_NOTIFY_COMMAND_MESSAGE)
        }}
      />
    </>
  )
}
