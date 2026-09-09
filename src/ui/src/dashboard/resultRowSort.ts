/** Default search-results table order: oldest packet first (INVITE before BYE). */
export const DEFAULT_RESULT_SORT_COL = 'timestamp'
export const DEFAULT_RESULT_SORT_DIR = 'asc' as const

export type ResultSortDir = 'asc' | 'desc'

function isTimestampSortCol(col: string): boolean {
  const c = col.toLowerCase()
  return c === 'timestamp' || c === 'ts' || c === 'create_date'
}

/** Parse a lake/API timestamp cell to epoch milliseconds. */
export function timestampToMs(val: unknown): number | null {
  if (val == null) return null
  if (val instanceof Date) return val.getTime()
  if (typeof val === 'number') {
    if (val > 1e15) return Math.round(val / 1e6)
    if (val > 1e12) return Math.round(val)
    if (val > 1e9) return Math.round(val * 1000)
    return val
  }
  if (typeof val === 'string') {
    let s = val.trim()
    if (/^\d{4}-\d{2}-\d{2}$/.test(s)) s = `${s}T00:00:00Z`
    else if (s.includes(' ') && !s.includes('T')) s = s.replace(' ', 'T')
    const d = new Date(s)
    return Number.isNaN(d.getTime()) ? null : d.getTime()
  }
  return null
}

/** Row event time for sorting and /messages (never the dashboard picker). */
export function pickRowTimestampMs(row: Record<string, unknown> | null | undefined): number | null {
  if (!row || typeof row !== 'object') return null
  for (const k of ['timestamp', 'TIMESTAMP', 'ts', 'TS', 'create_date', 'CREATE_DATE']) {
    if (!Object.prototype.hasOwnProperty.call(row, k)) continue
    const ms = timestampToMs(row[k])
    if (ms != null) return ms
  }
  return null
}

export function compareSearchResultRows(
  a: Record<string, unknown>,
  b: Record<string, unknown>,
  sortCol: string,
  sortDir: ResultSortDir,
): number {
  if (isTimestampSortCol(sortCol)) {
    const na = pickRowTimestampMs(a) ?? 0
    const nb = pickRowTimestampMs(b) ?? 0
    return sortDir === 'asc' ? na - nb : nb - na
  }
  const vaRaw = a[sortCol] ?? ''
  const vbRaw = b[sortCol] ?? ''
  const na = Number(vaRaw)
  const nb = Number(vbRaw)
  if (!Number.isNaN(na) && !Number.isNaN(nb) && vaRaw !== '' && vbRaw !== '') {
    return sortDir === 'asc' ? na - nb : nb - na
  }
  const va = String(vaRaw).toLowerCase()
  const vb = String(vbRaw).toLowerCase()
  if (va < vb) return sortDir === 'asc' ? -1 : 1
  if (va > vb) return sortDir === 'asc' ? 1 : -1
  return 0
}
