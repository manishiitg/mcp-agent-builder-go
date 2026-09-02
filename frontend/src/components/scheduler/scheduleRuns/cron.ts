import cronstrue from 'cronstrue'
import type { ScheduledJob } from '../../../services/api-types'

export function parseCronField(field: string, min: number, max: number, normalize?: (n: number) => number): number[] | null {
  const values = new Set<number>()
  const addValue = (n: number) => {
    const value = normalize ? normalize(n) : n
    if (value >= min && value <= max) values.add(value)
  }

  for (const rawPart of field.split(',')) {
    const part = rawPart.trim()
    if (!part) return null
    const [rangePart, stepPart] = part.split('/')
    const step = stepPart ? Number(stepPart) : 1
    if (!Number.isInteger(step) || step <= 0) return null

    let start: number
    let end: number
    if (rangePart === '*') {
      start = min
      end = max
    } else if (rangePart.includes('-')) {
      const [a, b] = rangePart.split('-').map(Number)
      if (!Number.isInteger(a) || !Number.isInteger(b)) return null
      start = a
      end = b
    } else {
      const single = Number(rangePart)
      if (!Number.isInteger(single)) return null
      start = single
      end = single
    }

    if (start > end) return null
    for (let n = start; n <= end; n += step) addValue(n)
  }

  return [...values].sort((a, b) => a - b)
}

export function isWildcardCronField(field: string): boolean {
  return field.trim() === '*'
}

export function expandCronForMonth(job: ScheduledJob, year: number, month: number): Array<{ date: string; time: string }> {
  if (!job.cron_expression) return []
  const parts = job.cron_expression.trim().split(/\s+/)
  if (parts.length !== 5) return []

  const minutes = parseCronField(parts[0], 0, 59)
  const hours = parseCronField(parts[1], 0, 23)
  const dom = parseCronField(parts[2], 1, 31)
  const months = parseCronField(parts[3], 1, 12)
  const dow = parseCronField(parts[4], 0, 6, n => n === 7 ? 0 : n)
  if (!minutes || !hours || !dom || !months || !dow) return []

  const monthNumber = month + 1
  if (!months.includes(monthNumber)) return []

  const domWildcard = isWildcardCronField(parts[2])
  const dowWildcard = isWildcardCronField(parts[4])
  const daysInMonth = new Date(year, month + 1, 0).getDate()
  const out: Array<{ date: string; time: string }> = []

  for (let day = 1; day <= daysInMonth; day += 1) {
    const d = new Date(year, month, day)
    const domMatches = dom.includes(day)
    const dowMatches = dow.includes(d.getDay())
    const dayMatches = domWildcard && dowWildcard
      ? true
      : domWildcard
        ? dowMatches
        : dowWildcard
          ? domMatches
          : domMatches || dowMatches
    if (!dayMatches) continue

    const date = `${year}-${String(monthNumber).padStart(2, '0')}-${String(day).padStart(2, '0')}`
    for (const hour of hours) {
      for (const minute of minutes) {
        out.push({ date, time: `${String(hour).padStart(2, '0')}:${String(minute).padStart(2, '0')}` })
      }
    }
  }

  return out.slice(0, 250)
}

export function dateKeyFromLocalDate(date: Date): string {
  return `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, '0')}-${String(date.getDate()).padStart(2, '0')}`
}

export function formatLocalTimeFromDate(date: Date): string {
  return date.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', hour12: false })
}

export function formatLocalDayLabel(dateKey: string): string {
  const [year, month, day] = dateKey.split('-').map(Number)
  return new Date(year, (month || 1) - 1, day || 1).toLocaleDateString([], {
    weekday: 'long',
    month: 'long',
    day: 'numeric',
    year: 'numeric',
  })
}

export function timeZoneOffsetMs(date: Date, timeZone: string): number {
  try {
    const parts = new Intl.DateTimeFormat('en-US', {
      timeZone,
      hourCycle: 'h23',
      year: 'numeric',
      month: '2-digit',
      day: '2-digit',
      hour: '2-digit',
      minute: '2-digit',
      second: '2-digit',
    }).formatToParts(date)
    const values = Object.fromEntries(parts.map(part => [part.type, part.value]))
    const asUTC = Date.UTC(
      Number(values.year),
      Number(values.month) - 1,
      Number(values.day),
      Number(values.hour),
      Number(values.minute),
      Number(values.second),
    )
    return asUTC - date.getTime()
  } catch {
    return 0
  }
}

export function scheduledDateTimeInLocal(dateKey: string, time: string, timeZone?: string): Date {
  const [year, month, day] = dateKey.split('-').map(Number)
  const [hour, minute] = time.split(':').map(Number)
  if (!timeZone) {
    return new Date(year, (month || 1) - 1, day || 1, hour || 0, minute || 0)
  }

  const guess = new Date(Date.UTC(year, (month || 1) - 1, day || 1, hour || 0, minute || 0))
  const first = new Date(guess.getTime() - timeZoneOffsetMs(guess, timeZone))
  const second = new Date(guess.getTime() - timeZoneOffsetMs(first, timeZone))
  return second
}

export function addMonths(date: Date, delta: number): Date {
  return new Date(date.getFullYear(), date.getMonth() + delta, 1)
}

export function describeCron(expr: string): string {
  try {
    return cronstrue.toString(expr, { throwExceptionOnParseError: true })
  } catch {
    return expr
  }
}
