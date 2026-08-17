import { agentApi } from '../../../services/api'
import { normalizeDdMmYyyy } from '../normalizeDdMmYyyy'
import type { AttentionItem, GstCard } from '../types'

const DB_PATH = 'Workflow/gstdatacollection/db/db.sqlite'

export async function loadGstCards(): Promise<GstCard[]> {
  const [snapshotResponse, ledgerResponse, returnResponse] = await Promise.all([
    agentApi.queryWorkflowDB(
      DB_PATH,
      `SELECT gstin, legal_name, turnover_aggregate, collected_at
       FROM gst_snapshot
       ORDER BY snapshot_date DESC
       LIMIT 1`
    ),
    agentApi.queryWorkflowDB(
      DB_PATH,
      `SELECT igst, cgst, sgst, cess
       FROM gst_ledger_balance
       ORDER BY snapshot_date DESC
       LIMIT 1`
    ),
    agentApi.queryWorkflowDB(
      DB_PATH,
      `SELECT due_date, status
       FROM gst_return_status
       WHERE status = 'Not Filed' AND snapshot_date = (SELECT MAX(snapshot_date) FROM gst_return_status)
       ORDER BY due_date ASC
       LIMIT 1`
    ),
  ])
  if (!snapshotResponse.success || !snapshotResponse.data) {
    throw new Error(snapshotResponse.error || 'GST: failed to load snapshot.')
  }
  const snapshotRow = snapshotResponse.data.rows[0]
  if (!snapshotRow) return []

  const ledgerRow = ledgerResponse.success ? ledgerResponse.data?.rows[0] : undefined
  const nextDueRow = returnResponse.success ? returnResponse.data?.rows[0] : undefined

  return [
    {
      gstin: String(snapshotRow.gstin ?? ''),
      legalName: String(snapshotRow.legal_name ?? ''),
      turnoverAggregate: Number(snapshotRow.turnover_aggregate ?? 0),
      ledgerBalance: {
        igst: Number(ledgerRow?.igst ?? 0),
        cgst: Number(ledgerRow?.cgst ?? 0),
        sgst: Number(ledgerRow?.sgst ?? 0),
        cess: Number(ledgerRow?.cess ?? 0),
      },
      nextReturnDue: nextDueRow ? String(nextDueRow.due_date ?? '') : undefined,
      filingStatus: nextDueRow ? String(nextDueRow.status ?? '') : 'Up to date',
      lastUpdated: String(snapshotRow.collected_at ?? ''),
    },
  ]
}

export async function loadGstAttentionItems(): Promise<AttentionItem[]> {
  // The same (fin_year, period, return_type) obligation is re-checked at
  // every snapshot -- an unfiled return observed on two different snapshot
  // dates is one obligation, not two, so this only reads the most recent
  // snapshot rather than every historical check-in (confirmed real: the
  // June GSTR-3B return appeared "Not Filed" at both 2026-06-28 and
  // 2026-07-12 snapshots, producing a visible duplicate before this fix).
  const response = await agentApi.queryWorkflowDB(
    DB_PATH,
    `SELECT fin_year, period, return_type, due_date, snapshot_date
     FROM gst_return_status
     WHERE status = 'Not Filed' AND snapshot_date = (SELECT MAX(snapshot_date) FROM gst_return_status)
     ORDER BY due_date ASC`
  )
  if (!response.success || !response.data) {
    throw new Error(response.error || 'GST: failed to load return status.')
  }
  const today = new Date().toISOString().slice(0, 10)
  return response.data.rows.map((row) => {
    const dueDate = String(row.due_date ?? '')
    // due_date is DD/MM/YYYY in this source; normalize before comparing to
    // today's ISO date so "overdue" is a real comparison, not a guess.
    const normalized = normalizeDdMmYyyy(dueDate)
    return {
      source: 'GST' as const,
      severity: normalized && normalized < today ? 'overdue' : 'pending',
      label: `${String(row.return_type ?? 'Return')}, ${String(row.period ?? '')} ${String(row.fin_year ?? '')} — not filed`,
      dueDate,
      lastConfirmed: row.snapshot_date == null ? undefined : String(row.snapshot_date),
    }
  })
}
