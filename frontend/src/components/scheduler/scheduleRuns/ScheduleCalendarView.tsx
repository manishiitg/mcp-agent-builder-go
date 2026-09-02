import React from 'react'
import { X, ChevronLeft, ChevronRight } from 'lucide-react'
import { formatLocalDayLabel } from './cron'
import type { ScheduleRunsPanelState } from './useScheduleRunsData'

type ScheduleCalendarViewProps = {
  panel: ScheduleRunsPanelState
}

export const ScheduleCalendarView: React.FC<ScheduleCalendarViewProps> = ({ panel }) => {
  const {
    setCalendarMonth,
    monthlyCalendar,
    selectedCalendarDate,
    setSelectedCalendarDate,
    showJobInWorkflowGroups,
    selectedCalendarCell,
  } = panel

  return (
    <div className="px-5 py-4 space-y-4">
      <div className="flex items-center justify-between gap-3">
        <button
          onClick={() => setCalendarMonth(prev => new Date(prev.getFullYear(), prev.getMonth() - 1, 1))}
          className="rounded-md border border-border p-1.5 text-muted-foreground hover:bg-muted hover:text-foreground"
        >
          <ChevronLeft className="h-4 w-4" />
        </button>
        <div className="text-center">
          <div className="text-sm font-semibold text-foreground">{monthlyCalendar.label}</div>
          <div className="text-xs text-muted-foreground">
            {monthlyCalendar.total} scheduled item{monthlyCalendar.total === 1 ? '' : 's'} · local time ({monthlyCalendar.localTimeZone})
          </div>
        </div>
        <button
          onClick={() => setCalendarMonth(prev => new Date(prev.getFullYear(), prev.getMonth() + 1, 1))}
          className="rounded-md border border-border p-1.5 text-muted-foreground hover:bg-muted hover:text-foreground"
        >
          <ChevronRight className="h-4 w-4" />
        </button>
      </div>

      <div className="rounded-xl border border-border bg-background p-3">
        <div className="grid grid-cols-7 gap-1 text-center text-[11px] font-medium text-muted-foreground">
          {['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat'].map(day => (
            <div key={day} className="rounded-md bg-muted/40 py-1">{day}</div>
          ))}
        </div>
        <div className="mt-1 grid grid-cols-7 gap-1">
          {monthlyCalendar.cells.map((cell) => (
            <div
              key={cell.key}
              onClick={() => cell.date && setSelectedCalendarDate(cell.date)}
              role={cell.date ? 'button' : undefined}
              tabIndex={cell.date ? 0 : undefined}
              onKeyDown={(event) => {
                if (!cell.date) return
                if (event.key === 'Enter' || event.key === ' ') {
                  event.preventDefault()
                  setSelectedCalendarDate(cell.date)
                }
              }}
              className={`min-h-[112px] rounded-lg border p-2 text-left transition-colors ${
                cell.day
                  ? cell.items.length
                    ? selectedCalendarDate === cell.date
                      ? 'border-amber-500 bg-amber-500/10 shadow-[inset_0_0_0_1px_rgba(245,158,11,0.16)]'
                      : 'border-amber-500/30 bg-card shadow-[inset_0_0_0_1px_rgba(245,158,11,0.08)] hover:border-amber-500/60 hover:bg-amber-500/5'
                    : selectedCalendarDate === cell.date
                      ? 'border-amber-500/60 bg-amber-500/5'
                      : 'border-border bg-card/70 hover:border-amber-500/40 hover:bg-card'
                  : 'border-transparent'
              }`}
            >
              {cell.day && (
                <>
                  <div className="mb-1 flex items-center justify-between">
                    <span className="text-xs font-medium text-foreground">{cell.day}</span>
                    {cell.items.length > 0 && (
                      <span className="rounded-full border border-amber-500/30 bg-amber-500/10 px-1.5 py-0.5 text-[10px] font-medium text-amber-600 dark:text-amber-300">
                        {cell.items.length}
                      </span>
                    )}
                  </div>
                  <div className="space-y-1">
                    {cell.items.slice(0, 4).map((item, index) => (
                      <button
                        key={`${cell.date}-${item.job.id}-${index}`}
                        onClick={(event) => {
                          event.stopPropagation()
                          showJobInWorkflowGroups(item.job)
                        }}
                        className="block w-full truncate rounded-md border border-border bg-muted/40 px-1.5 py-1 text-left text-[11px] leading-tight text-foreground hover:border-amber-500/30 hover:bg-muted"
                        title={`${item.time} ${item.label} - ${item.note || ''}`}
                      >
                        <span className="font-medium text-amber-600 dark:text-amber-300">{item.time}</span>
                        <span className="ml-1">{item.label}</span>
                      </button>
                    ))}
                    {cell.items.length > 4 && (
                      <button
                        type="button"
                        onClick={(event) => {
                          event.stopPropagation()
                          if (cell.date) setSelectedCalendarDate(cell.date)
                        }}
                        className="rounded px-1 text-left text-[11px] text-muted-foreground hover:bg-muted hover:text-foreground"
                      >
                        +{cell.items.length - 4} more
                      </button>
                    )}
                  </div>
                </>
              )}
            </div>
          ))}
        </div>
      </div>

      {selectedCalendarDate && (
        <div className="rounded-xl border border-border bg-card">
          <div className="flex items-center justify-between gap-3 border-b border-border px-4 py-3">
            <div>
              <div className="text-sm font-semibold text-foreground">{formatLocalDayLabel(selectedCalendarDate)}</div>
              <div className="text-xs text-muted-foreground">
                {(selectedCalendarCell?.items.length ?? 0)} scheduled item{(selectedCalendarCell?.items.length ?? 0) === 1 ? '' : 's'} · local time
              </div>
            </div>
            <button
              type="button"
              onClick={() => setSelectedCalendarDate(null)}
              className="rounded-md p-1.5 text-muted-foreground hover:bg-muted hover:text-foreground"
              aria-label="Close day detail"
            >
              <X className="h-4 w-4" />
            </button>
          </div>
          <div className="max-h-72 overflow-y-auto p-3">
            {selectedCalendarCell?.items.length ? (
              <div className="space-y-2">
                {selectedCalendarCell.items.map((item, index) => (
                  <button
                    key={`${selectedCalendarDate}-${item.job.id}-${index}`}
                    type="button"
                    onClick={() => showJobInWorkflowGroups(item.job)}
                    className="flex w-full items-start gap-3 rounded-lg border border-border bg-background px-3 py-2 text-left hover:border-amber-500/40 hover:bg-muted/40"
                  >
                    <div className="min-w-14 rounded-md bg-amber-500/10 px-2 py-1 text-center text-xs font-semibold text-amber-600 dark:text-amber-300">
                      {item.time}
                    </div>
                    <div className="min-w-0 flex-1">
                      <div className="truncate text-sm font-medium text-foreground">{item.label}</div>
                      <div className="mt-0.5 truncate text-xs text-muted-foreground">{item.note || item.job.name}</div>
                      {item.timezone && item.timezone !== monthlyCalendar.localTimeZone && (
                        <div className="mt-1 text-[11px] text-muted-foreground">
                          Source: {item.sourceTime} {item.timezone}
                        </div>
                      )}
                    </div>
                  </button>
                ))}
              </div>
            ) : (
              <div className="rounded-lg border border-dashed border-border px-3 py-6 text-center text-sm text-muted-foreground">
                No schedules on this day.
              </div>
            )}
          </div>
        </div>
      )}
    </div>
  )
}
