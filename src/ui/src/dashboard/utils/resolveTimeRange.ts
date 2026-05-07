/**
 * Global dashboard time range: minute preset, calendar preset (Today/Yesterday),
 * or absolute from/to. Stale from/to are ignored when a preset is active.
 */

export type CalendarPreset = 'today' | 'yesterday' | 'this_month' | 'last_month'

export type DashboardTimeRange = {
  from?: number
  to?: number
  activePreset?: number | null
  calendarPreset?: CalendarPreset | null
}

function pad(n: number) {
  return n.toString().padStart(2, '0')
}

/** Wall-clock in IANA zone or browser local → UTC ms (aligned with TimeRangePicker.parseInputToMs). */
export function parseWallClockToMs(
  year: number,
  month: number,
  day: number,
  hour: number,
  minute: number,
  second: number,
  tz: string,
): number {
  const str = `${year}-${pad(month)}-${pad(day)}T${pad(hour)}:${pad(minute)}:${pad(second)}`
  if (tz === 'local') return new Date(str).getTime()
  const utcGuess = Date.UTC(year, month - 1, day, hour, minute, second)
  const f = new Intl.DateTimeFormat('en-US', {
    timeZone: tz,
    hour12: false,
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
  })
  const v = Object.fromEntries(f.formatToParts(new Date(utcGuess)).map((p) => [p.type, p.value]))
  const asUTC = Date.UTC(
    Number(v.year),
    Number(v.month) - 1,
    Number(v.day),
    Number(v.hour === '24' ? 0 : v.hour),
    Number(v.minute),
    Number(v.second),
  )
  const offset = asUTC - new Date(utcGuess).getTime()
  return utcGuess - offset
}

function ymdInZone(ms: number, tz: string): { y: number; m: number; d: number } {
  if (tz === 'local') {
    const d = new Date(ms)
    return { y: d.getFullYear(), m: d.getMonth() + 1, d: d.getDate() }
  }
  const f = new Intl.DateTimeFormat('en-US', {
    timeZone: tz,
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
  })
  const parts = f.formatToParts(new Date(ms))
  const map = Object.fromEntries(parts.filter((p) => p.type !== 'literal').map((p) => [p.type, p.value]))
  return { y: Number(map.year), m: Number(map.month), d: Number(map.day) }
}

/** Pure Gregorian civil date + day delta (for labels from Intl in any zone). */
function addDaysToCalendarDate(y: number, m: number, d: number, add: number): [number, number, number] {
  const x = new Date(Date.UTC(y, m - 1, d + add))
  return [x.getUTCFullYear(), x.getUTCMonth() + 1, x.getUTCDate()]
}

/** Last calendar day (1–31) for month `mo` (1=Jan … 12=Dec) in Gregorian `year`. */
function daysInCalendarMonth(year: number, mo: number): number {
  return new Date(Date.UTC(year, mo, 0)).getUTCDate()
}

function prevCalendarMonth(y: number, m: number): [number, number] {
  if (m > 1) return [y, m - 1]
  return [y - 1, 12]
}

function resolveThisMonth(timeZone: string | undefined): { from: number; to: number } | null {
  const tz = timeZone && timeZone.length > 0 ? timeZone : 'local'
  const now = Date.now()
  if (tz === 'local') {
    const x = new Date(now)
    const y = x.getFullYear()
    const mi = x.getMonth()
    const from = new Date(y, mi, 1, 0, 0, 0, 0).getTime()
    const to = new Date(y, mi + 1, 0, 23, 59, 59, 999).getTime()
    return { from, to }
  }
  const { y, m } = ymdInZone(now, tz)
  const last = daysInCalendarMonth(y, m)
  return {
    from: parseWallClockToMs(y, m, 1, 0, 0, 0, tz),
    to: parseWallClockToMs(y, m, last, 23, 59, 59, tz),
  }
}

function resolveLastMonth(timeZone: string | undefined): { from: number; to: number } | null {
  const tz = timeZone && timeZone.length > 0 ? timeZone : 'local'
  const now = Date.now()
  if (tz === 'local') {
    const x = new Date(now)
    const y = x.getFullYear()
    const mi = x.getMonth()
    const prev = new Date(y, mi - 1, 1)
    const py = prev.getFullYear()
    const pmi = prev.getMonth()
    const from = new Date(py, pmi, 1, 0, 0, 0, 0).getTime()
    const to = new Date(py, pmi + 1, 0, 23, 59, 59, 999).getTime()
    return { from, to }
  }
  const { y, m } = ymdInZone(now, tz)
  const [yy, mm] = prevCalendarMonth(y, m)
  const last = daysInCalendarMonth(yy, mm)
  return {
    from: parseWallClockToMs(yy, mm, 1, 0, 0, 0, tz),
    to: parseWallClockToMs(yy, mm, last, 23, 59, 59, tz),
  }
}

function resolveCalendarPreset(
  preset: CalendarPreset,
  timeZone: string | undefined,
): { from: number; to: number } | null {
  const tz = timeZone && timeZone.length > 0 ? timeZone : 'local'
  const now = Date.now()

  if (preset === 'this_month') return resolveThisMonth(timeZone)
  if (preset === 'last_month') return resolveLastMonth(timeZone)

  if (preset === 'today') {
    if (tz === 'local') {
      const d = new Date(now)
      const start = new Date(d.getFullYear(), d.getMonth(), d.getDate(), 0, 0, 0, 0)
      return { from: start.getTime(), to: now }
    }
    const { y, m, d } = ymdInZone(now, tz)
    const from = parseWallClockToMs(y, m, d, 0, 0, 0, tz)
    return { from, to: now }
  }

  // yesterday — полные сутки предыдущего календарного дня в зоне
  if (tz === 'local') {
    const d = new Date(now)
    const start = new Date(d.getFullYear(), d.getMonth(), d.getDate() - 1, 0, 0, 0, 0)
    const end = new Date(d.getFullYear(), d.getMonth(), d.getDate() - 1, 23, 59, 59, 999)
    return { from: start.getTime(), to: end.getTime() }
  }
  const { y, m, d } = ymdInZone(now, tz)
  const [yy, mm, dd] = addDaysToCalendarDate(y, m, d, -1)
  const from = parseWallClockToMs(yy, mm, dd, 0, 0, 0, tz)
  const to = parseWallClockToMs(yy, mm, dd, 23, 59, 59, tz)
  return { from, to }
}

/** True when the user chose a preset that must stay selected while from/to are recomputed on each request. */
export function hasRollingTimeSelection(range?: DashboardTimeRange | null): boolean {
  if (!range) return false
  if (typeof range.activePreset === 'number' && range.activePreset > 0) return true
  const c = range.calendarPreset
  return (
    c === 'today' ||
    c === 'yesterday' ||
    c === 'this_month' ||
    c === 'last_month'
  )
}

/** Stable key for “did the user change the logical range?” (rolling presets must not compare resolved ms). */
export function timeRangeLogicalKey(range?: DashboardTimeRange | null): string {
  if (!range) return ''
  if (
    range.calendarPreset === 'today' ||
    range.calendarPreset === 'yesterday' ||
    range.calendarPreset === 'this_month' ||
    range.calendarPreset === 'last_month'
  ) {
    return `c:${range.calendarPreset}`
  }
  if (typeof range.activePreset === 'number' && range.activePreset > 0) {
    return `p:${range.activePreset}`
  }
  if (range.from != null && range.to != null) return `a:${range.from}|${range.to}`
  return ''
}

/**
 * Returns effective { from, to } in milliseconds for API calls and SQL.
 * @param timeZone dashboard header zone ('local' or IANA); used for Today/Yesterday.
 */
export function resolveTimeRange(
  range?: DashboardTimeRange | null,
  timeZone?: string,
): { from: number; to: number } | null {
  if (!range) return null
  if (
    range.calendarPreset === 'today' ||
    range.calendarPreset === 'yesterday' ||
    range.calendarPreset === 'this_month' ||
    range.calendarPreset === 'last_month'
  ) {
    return resolveCalendarPreset(range.calendarPreset, timeZone)
  }
  const ap = range.activePreset
  if (typeof ap === 'number' && ap > 0) {
    const now = Date.now()
    return { from: now - ap * 60 * 1000, to: now }
  }
  if (range.from != null && range.to != null) {
    return { from: Number(range.from), to: Number(range.to) }
  }
  return null
}
