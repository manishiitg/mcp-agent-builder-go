import { useEffect, useState } from 'react'
import { AlertTriangle, ArrowDownRight, ArrowUpRight, Clock, Landmark, ReceiptIndianRupee, TrendingUp } from 'lucide-react'
import { Cell, Pie, PieChart, ResponsiveContainer, Tooltip } from 'recharts'
import { ProductSurfaceSwitcher } from '../../components/ProductSurfaceSwitcher'
import { loadHdfcAccounts } from './adapters/hdfc'
import { loadIciciAccounts, loadIciciTransactions } from './adapters/icici'
import { loadGstAttentionItems, loadGstCards } from './adapters/gst'
import { loadMutualFundAccounts, loadMutualFundTransactions } from './adapters/mutualFund'
import { loadTaxAttentionItems, loadTaxCards } from './adapters/tax'
import { buildFreshnessEntries, combineAttentionItems, computeNetWorth, findStaleSources, mergeRecentActivity } from './synthesize'
import type { AttentionItem, BankAccountCard, GstCard, InvestmentAccountCard, RecentTransaction, TaxCard } from './types'

type LoadState = {
  bankAccounts: BankAccountCard[]
  investmentAccounts: InvestmentAccountCard[]
  tax: TaxCard[]
  gst: GstCard[]
  transactions: RecentTransaction[]
  attentionItems: AttentionItem[]
  loading: boolean
  error: string | null
}

const EMPTY_STATE: LoadState = {
  bankAccounts: [],
  investmentAccounts: [],
  tax: [],
  gst: [],
  transactions: [],
  attentionItems: [],
  loading: true,
  error: null,
}

function useFinanceData(): LoadState {
  const [state, setState] = useState<LoadState>(EMPTY_STATE)

  useEffect(() => {
    let cancelled = false
    ;(async () => {
      try {
        const [
          hdfcAccounts,
          iciciAccounts,
          iciciTransactions,
          investmentAccounts,
          mfTransactions,
          tax,
          taxAttention,
          gst,
          gstAttention,
        ] = await Promise.all([
          loadHdfcAccounts(),
          loadIciciAccounts(),
          loadIciciTransactions(),
          loadMutualFundAccounts(),
          loadMutualFundTransactions(),
          loadTaxCards(),
          loadTaxAttentionItems(),
          loadGstCards(),
          loadGstAttentionItems(),
        ])
        if (cancelled) return
        setState({
          bankAccounts: [...hdfcAccounts, ...iciciAccounts],
          investmentAccounts,
          tax,
          gst,
          transactions: mergeRecentActivity(iciciTransactions, mfTransactions),
          attentionItems: combineAttentionItems(taxAttention, gstAttention),
          loading: false,
          error: null,
        })
      } catch (err) {
        if (cancelled) return
        setState((prev) => ({ ...prev, loading: false, error: err instanceof Error ? err.message : String(err) }))
      }
    })()
    return () => { cancelled = true }
  }, [])

  return state
}

function formatInr(value: number, compact = false): string {
  return new Intl.NumberFormat('en-IN', {
    style: 'currency',
    currency: 'INR',
    maximumFractionDigits: 0,
    notation: compact ? 'compact' : 'standard',
  }).format(value)
}

function formatDate(value: string | undefined): string {
  if (!value) return ''
  const parsed = new Date(value)
  if (Number.isNaN(parsed.getTime())) return value
  return parsed.toLocaleDateString('en-IN', { day: 'numeric', month: 'short', year: 'numeric' })
}

const ALLOCATION_COLORS = { liquid: '#10b981', locked: '#f59e0b', invested: '#6366f1' }

function SectionHeader({ icon: Icon, title, count }: { icon: typeof Landmark; title: string; count?: number }) {
  return (
    <div className="mb-3 flex items-center gap-2">
      <Icon className="h-4 w-4 text-slate-400" strokeWidth={2} />
      <h2 className="text-sm font-semibold text-slate-700 dark:text-slate-200">{title}</h2>
      {count != null && (
        <span className="rounded-full bg-slate-100 px-2 py-0.5 text-[11px] font-medium text-slate-500 dark:bg-slate-800 dark:text-slate-400">
          {count}
        </span>
      )}
    </div>
  )
}

const CARD = 'rounded-2xl border border-slate-200/70 bg-white p-5 shadow-sm transition-shadow hover:shadow-md dark:border-slate-800 dark:bg-slate-900'

export function FinanceSurface() {
  const { bankAccounts, investmentAccounts, tax, gst, transactions, attentionItems, loading, error } = useFinanceData()

  const netWorth = computeNetWorth(bankAccounts, investmentAccounts)
  const staleSources = findStaleSources(buildFreshnessEntries(bankAccounts, investmentAccounts, tax, gst))
  const allocationData = [
    { key: 'liquid', name: 'Liquid', value: netWorth.liquid, color: ALLOCATION_COLORS.liquid },
    { key: 'locked', name: 'Fixed Deposits', value: netWorth.locked, color: ALLOCATION_COLORS.locked },
    { key: 'invested', name: 'Invested', value: netWorth.invested, color: ALLOCATION_COLORS.invested },
  ].filter((d) => d.value > 0)

  return (
    <div className="flex h-screen min-h-0 flex-col overflow-hidden bg-slate-50 dark:bg-slate-950">
      <header className="flex h-[62px] shrink-0 items-center gap-4 border-b border-slate-200 bg-white px-4 dark:border-slate-800 dark:bg-slate-950">
        <ProductSurfaceSwitcher />
      </header>
      <div className="flex min-h-0 flex-1 flex-col">
      {loading ? (
        <div className="grid flex-1 place-items-center text-sm text-slate-400">Loading finance data…</div>
      ) : error ? (
        <div className="grid flex-1 place-items-center text-sm text-red-500">Failed to load: {error}</div>
      ) : (
        <div className="flex-1 overflow-y-auto">
          <div className="mx-auto max-w-6xl space-y-8 p-8">

            {/* Hero: Net Worth + allocation donut */}
            <section className="grid grid-cols-1 gap-4 lg:grid-cols-[1.4fr_1fr]">
              <div className={`${CARD} flex flex-col justify-center bg-gradient-to-br from-slate-900 to-slate-800 dark:from-slate-900 dark:to-black`}>
                <div className="text-xs font-semibold uppercase tracking-wide text-slate-400">Net Worth</div>
                <div className="mt-1 text-4xl font-bold tracking-tight text-white">{formatInr(netWorth.total)}</div>
                <div className="mt-4 flex gap-6 text-sm">
                  <div>
                    <div className="flex items-center gap-1.5 text-slate-400"><span className="h-2 w-2 rounded-full" style={{ background: ALLOCATION_COLORS.liquid }} />Liquid</div>
                    <div className="font-semibold text-white">{formatInr(netWorth.liquid, true)}</div>
                  </div>
                  <div>
                    <div className="flex items-center gap-1.5 text-slate-400"><span className="h-2 w-2 rounded-full" style={{ background: ALLOCATION_COLORS.locked }} />Fixed Deposits</div>
                    <div className="font-semibold text-white">{formatInr(netWorth.locked, true)}</div>
                  </div>
                  <div>
                    <div className="flex items-center gap-1.5 text-slate-400"><span className="h-2 w-2 rounded-full" style={{ background: ALLOCATION_COLORS.invested }} />Invested</div>
                    <div className="font-semibold text-white">{formatInr(netWorth.invested, true)}</div>
                  </div>
                </div>
              </div>
              <div className={CARD}>
                <div className="mb-1 text-xs font-semibold uppercase tracking-wide text-slate-400">Allocation</div>
                <div className="h-[168px]">
                  {allocationData.length > 0 ? (
                    <ResponsiveContainer width="100%" height="100%">
                      <PieChart>
                        <Pie data={allocationData} dataKey="value" nameKey="name" innerRadius={48} outerRadius={72} paddingAngle={2}>
                          {allocationData.map((d) => <Cell key={d.key} fill={d.color} stroke="none" />)}
                        </Pie>
                        <Tooltip formatter={(value) => formatInr(Number(value ?? 0))} contentStyle={{ borderRadius: 8, fontSize: 12 }} />
                      </PieChart>
                    </ResponsiveContainer>
                  ) : (
                    <div className="grid h-full place-items-center text-xs text-slate-400">No data</div>
                  )}
                </div>
              </div>
            </section>

            {/* Needs Attention */}
            {attentionItems.length > 0 && (
              <section>
                <SectionHeader icon={AlertTriangle} title="Needs Attention" count={attentionItems.length} />
                <div className="space-y-2">
                  {attentionItems.map((item, i) => (
                    <div
                      key={i}
                      className={`flex items-center justify-between rounded-xl border px-4 py-3 text-sm ${
                        item.severity === 'overdue'
                          ? 'border-red-200 bg-red-50 dark:border-red-900/60 dark:bg-red-950/30'
                          : 'border-amber-200 bg-amber-50 dark:border-amber-900/60 dark:bg-amber-950/30'
                      }`}
                    >
                      <div className="flex items-center gap-2">
                        <span
                          className={`rounded-full px-2 py-0.5 text-[10px] font-bold uppercase tracking-wide ${
                            item.severity === 'overdue'
                              ? 'bg-red-600 text-white'
                              : 'bg-amber-500 text-white'
                          }`}
                        >
                          {item.severity}
                        </span>
                        <span className="font-medium text-slate-800 dark:text-slate-100">{item.label}</span>
                      </div>
                      <div className="flex items-center gap-3 text-xs text-slate-500">
                        {item.dueDate && <span>due {item.dueDate}</span>}
                        {item.lastConfirmed && (
                          <span className="flex items-center gap-1 text-slate-400">
                            <Clock className="h-3 w-3" /> confirmed {formatDate(item.lastConfirmed)}
                          </span>
                        )}
                      </div>
                    </div>
                  ))}
                </div>
              </section>
            )}

            {staleSources.length > 0 && (
              <div className="flex items-start gap-2 rounded-xl border border-slate-200 bg-white px-4 py-2.5 text-xs text-slate-500 dark:border-slate-800 dark:bg-slate-900">
                <Clock className="mt-0.5 h-3.5 w-3.5 shrink-0 text-slate-400" />
                <span>Possibly stale relative to peers: {staleSources.map((s) => `${s.group} ${s.source}`).join(', ')}</span>
              </div>
            )}

            {/* Bank Accounts + Investments */}
            <div className="grid grid-cols-1 gap-8 lg:grid-cols-2">
              <section>
                <SectionHeader icon={Landmark} title="Bank Accounts" count={bankAccounts.length} />
                <div className="space-y-3">
                  {bankAccounts.map((a, i) => (
                    <div key={i} className={CARD}>
                      <div className="flex items-start justify-between">
                        <div>
                          <div className="text-xs font-medium text-slate-400">{a.source}</div>
                          <div className="text-sm font-semibold text-slate-800 dark:text-slate-100">{a.accountLabel}</div>
                        </div>
                        <div className="text-right">
                          <div className="text-lg font-bold text-slate-900 dark:text-white">{formatInr(a.currentBalance)}</div>
                          {a.fixedDeposit > 0 && <div className="text-xs text-amber-600 dark:text-amber-400">+ {formatInr(a.fixedDeposit)} FD</div>}
                        </div>
                      </div>
                      {a.transactionCount != null && (
                        <div className="mt-2 border-t border-slate-100 pt-2 text-xs text-slate-400 dark:border-slate-800">
                          {a.transactionCount} transactions · latest {formatDate(a.lastTransactionDate)}
                        </div>
                      )}
                    </div>
                  ))}
                </div>
              </section>

              <section>
                <SectionHeader icon={TrendingUp} title="Investments" count={investmentAccounts.length} />
                <div className="space-y-3">
                  {investmentAccounts.map((a, i) => (
                    <div key={i} className={CARD}>
                      <div className="flex items-start justify-between">
                        <div>
                          <div className="text-xs font-medium text-slate-400">Mutual Fund</div>
                          <div className="text-sm font-semibold text-slate-800 dark:text-slate-100">{a.accountLabel}</div>
                        </div>
                        <div className="text-right">
                          <div className="text-lg font-bold text-slate-900 dark:text-white">{formatInr(a.totalCurrentValue)}</div>
                          {a.xirrPct != null && (
                            <div className={`flex items-center justify-end gap-0.5 text-xs font-medium ${a.xirrPct >= 0 ? 'text-emerald-600' : 'text-red-600'}`}>
                              {a.xirrPct >= 0 ? <ArrowUpRight className="h-3 w-3" /> : <ArrowDownRight className="h-3 w-3" />}
                              {a.xirrPct.toFixed(1)}% XIRR
                            </div>
                          )}
                        </div>
                      </div>
                      <div className="mt-2 border-t border-slate-100 pt-2 text-xs text-slate-400 dark:border-slate-800">
                        {a.totalInvested > 0 ? `invested ${formatInr(a.totalInvested)} · ` : ''}{a.holdings.length} holdings
                        {a.totalInvested === 0 && a.totalCurrentValue > 0 && (
                          <span className="ml-1 text-slate-300 dark:text-slate-600">(cost basis not tracked by source)</span>
                        )}
                      </div>
                    </div>
                  ))}
                </div>
              </section>
            </div>

            {/* Tax + GST */}
            <div className="grid grid-cols-1 gap-8 lg:grid-cols-2">
              <section>
                <SectionHeader icon={ReceiptIndianRupee} title="Tax" count={tax.length} />
                <div className="space-y-3">
                  {tax.map((t, i) => (
                    <div key={i} className={CARD}>
                      <div className="text-xs font-medium text-slate-400">PAN {t.pan}</div>
                      <div className="mt-1 flex items-baseline justify-between">
                        <span className="text-sm text-slate-600 dark:text-slate-300">TDS this year</span>
                        <span className="text-lg font-bold text-slate-900 dark:text-white">{formatInr(t.tdsThisYear)}</span>
                      </div>
                      {t.refundAmount > 0 && (
                        <div className="mt-1 flex items-baseline justify-between text-emerald-600">
                          <span className="text-sm">Refund</span>
                          <span className="text-sm font-semibold">{formatInr(t.refundAmount)}</span>
                        </div>
                      )}
                      {t.pendingNotices > 0 && (
                        <div className="mt-2 border-t border-slate-100 pt-2 text-xs font-medium text-amber-600 dark:border-slate-800">
                          {t.pendingNotices} pending notice{t.pendingNotices === 1 ? '' : 's'}
                        </div>
                      )}
                    </div>
                  ))}
                </div>
              </section>

              <section>
                <SectionHeader icon={ReceiptIndianRupee} title="GST" count={gst.length} />
                <div className="space-y-3">
                  {gst.map((g, i) => (
                    <div key={i} className={CARD}>
                      <div className="text-xs font-medium text-slate-400">{g.legalName}</div>
                      <div className="mt-1 flex items-baseline justify-between">
                        <span className="text-sm text-slate-600 dark:text-slate-300">Turnover</span>
                        <span className="text-lg font-bold text-slate-900 dark:text-white">{formatInr(g.turnoverAggregate)}</span>
                      </div>
                      <div className="mt-2 border-t border-slate-100 pt-2 text-xs dark:border-slate-800">
                        <span className={g.filingStatus === 'Up to date' ? 'text-emerald-600' : 'font-medium text-amber-600'}>
                          {g.filingStatus}
                        </span>
                        {g.nextReturnDue && <span className="text-slate-400"> · due {g.nextReturnDue}</span>}
                      </div>
                    </div>
                  ))}
                </div>
              </section>
            </div>

            {/* Recent Activity */}
            <section>
              <SectionHeader icon={Clock} title="Recent Activity" />
              <div className={`${CARD} divide-y divide-slate-100 p-0 dark:divide-slate-800`}>
                {transactions.slice(0, 20).map((t, i) => (
                  <div key={i} className="flex items-center justify-between px-5 py-3 text-sm">
                    <div className="flex items-center gap-3">
                      <span
                        className={`flex h-7 w-7 shrink-0 items-center justify-center rounded-full ${
                          t.direction === 'credit'
                            ? 'bg-emerald-100 text-emerald-600 dark:bg-emerald-950 dark:text-emerald-400'
                            : t.direction === 'debit'
                            ? 'bg-slate-100 text-slate-500 dark:bg-slate-800 dark:text-slate-400'
                            : 'bg-slate-50 text-slate-300 dark:bg-slate-900 dark:text-slate-600'
                        }`}
                      >
                        {t.direction === 'credit' ? <ArrowDownRight className="h-3.5 w-3.5" /> : <ArrowUpRight className="h-3.5 w-3.5" />}
                      </span>
                      <div>
                        <div className="text-slate-800 dark:text-slate-100">{t.description}</div>
                        <div className="text-xs text-slate-400">{t.source} · {formatDate(t.date)}</div>
                      </div>
                    </div>
                    <div
                      className={`font-semibold ${
                        t.direction === 'credit' ? 'text-emerald-600' : 'text-slate-700 dark:text-slate-300'
                      }`}
                    >
                      {t.direction === 'debit' ? '−' : t.direction === 'credit' ? '+' : ''}{formatInr(t.amount)}
                    </div>
                  </div>
                ))}
                {transactions.length === 0 && (
                  <div className="px-5 py-8 text-center text-sm text-slate-400">No transactions to show.</div>
                )}
              </div>
            </section>
          </div>
        </div>
      )}
      </div>
    </div>
  )
}
