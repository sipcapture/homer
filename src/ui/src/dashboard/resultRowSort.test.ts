import { describe, expect, it } from 'vitest'
import {
  compareSearchResultRows,
  DEFAULT_RESULT_SORT_COL,
  DEFAULT_RESULT_SORT_DIR,
} from './resultRowSort'

describe('compareSearchResultRows', () => {
  it('defaults to oldest-first timestamp order', () => {
    expect(DEFAULT_RESULT_SORT_COL).toBe('timestamp')
    expect(DEFAULT_RESULT_SORT_DIR).toBe('asc')
  })

  it('sorts INVITE before BYE when timestamps are ascending', () => {
    const invite = { method: 'INVITE', timestamp: '2026-05-01T12:00:00Z' }
    const bye = { method: 'BYE', timestamp: '2026-05-01T12:00:05Z' }
    expect(compareSearchResultRows(invite, bye, 'timestamp', 'asc')).toBeLessThan(0)
    expect(compareSearchResultRows(invite, bye, 'timestamp', 'desc')).toBeGreaterThan(0)
  })

  it('sorts a call so the first row is the earliest packet', () => {
    const rows = [
      { method: 'BYE', timestamp: '2026-05-01T12:00:05Z' },
      { method: 'INVITE', timestamp: '2026-05-01T12:00:00Z' },
      { method: '200', timestamp: '2026-05-01T12:00:01Z' },
    ]
    const sorted = [...rows].sort((a, b) =>
      compareSearchResultRows(a, b, DEFAULT_RESULT_SORT_COL, DEFAULT_RESULT_SORT_DIR),
    )
    expect(sorted.map((r) => r.method)).toEqual(['INVITE', '200', 'BYE'])
  })
})
