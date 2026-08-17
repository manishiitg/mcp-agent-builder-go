import { agentApi } from '../../../services/api'
import type { AttentionItem, TaxCard } from '../types'

const DB_PATH = 'Workflow/check-form-26as-xspaces/db/db.sqlite'

export async function loadTaxCards(): Promise<TaxCard[]> {
  const response = await agentApi.queryWorkflowDB(
    DB_PATH,
    `SELECT pan, total_tds_current_ay, pending_notice_count, total_refund_amount, last_checked
     FROM tax_summary`
  )
  if (!response.success || !response.data) {
    throw new Error(response.error || 'Tax: failed to load summary.')
  }
  // total_refund_amount is the source's own field; it is sometimes 0 even
  // when refunds_by_ay (a per-year JSON breakdown, not queried here) shows
  // real values -- a known upstream data-quality gap in this workflow, not
  // something this adapter silently corrects by re-deriving the total.
  return response.data.rows.map((row) => ({
    pan: String(row.pan ?? ''),
    tdsThisYear: Number(row.total_tds_current_ay ?? 0),
    pendingNotices: Number(row.pending_notice_count ?? 0),
    refundAmount: Number(row.total_refund_amount ?? 0),
    lastChecked: String(row.last_checked ?? ''),
  }))
}

export async function loadTaxAttentionItems(): Promise<AttentionItem[]> {
  const response = await agentApi.queryWorkflowDB(
    DB_PATH,
    `SELECT pan, title, status, issue_date, last_seen
     FROM notices
     WHERE action_required = 1`
  )
  if (!response.success || !response.data) {
    throw new Error(response.error || 'Tax: failed to load notices.')
  }
  // notices has no resolved/closed status to read -- an old issue_date does
  // NOT mean this is stale or handled, Indian tax notices can genuinely
  // stay open for years. dueDate is deliberately not set here: issue_date
  // is when the notice was raised, not a deadline, and labeling it "due"
  // would be wrong the way GST's due_date is a real deadline. lastConfirmed
  // (last_seen) is what actually matters for trust: how recently this was
  // reconfirmed against the source.
  return response.data.rows.map((row) => {
    const issueDate = row.issue_date == null ? '' : String(row.issue_date)
    return {
      source: 'Tax' as const,
      severity: 'pending' as const,
      label: `${String(row.title || row.status || 'Notice')} — ${String(row.status ?? '')}${issueDate ? ` (issued ${issueDate})` : ''}`.trim(),
      lastConfirmed: row.last_seen == null ? undefined : String(row.last_seen),
    }
  })
}
