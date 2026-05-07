import { describe, expect, it, vi } from 'vitest'
import { resolveTimeRange, timeRangeLogicalKey, hasRollingTimeSelection } from './resolveTimeRange'

describe('resolveTimeRange', () => {
  it('hasRollingTimeSelection is true for minute and calendar presets', () => {
    expect(hasRollingTimeSelection({ from: 1, to: 2, activePreset: 10 })).toBe(true)
    expect(hasRollingTimeSelection({ from: 1, to: 2, calendarPreset: 'today' })).toBe(true)
    expect(hasRollingTimeSelection({ from: 1, to: 2, calendarPreset: 'this_month' })).toBe(true)
    expect(hasRollingTimeSelection({ from: 1, to: 2, activePreset: null })).toBe(false)
    expect(hasRollingTimeSelection({ from: 1, to: 2 })).toBe(false)
    expect(hasRollingTimeSelection(null)).toBe(false)
  })

  it('uses rolling window when activePreset minutes is set', () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-01-15T12:00:00.000Z'))
    const r = resolveTimeRange({
      from: 100,
      to: 200,
      activePreset: 60,
    })
    expect(r).toEqual({
      from: new Date('2026-01-15T11:00:00.000Z').getTime(),
      to: new Date('2026-01-15T12:00:00.000Z').getTime(),
    })
    vi.useRealTimers()
  })

  it('returns stored from/to when no preset', () => {
    expect(
      resolveTimeRange({ from: 10, to: 20, activePreset: null }),
    ).toEqual({ from: 10, to: 20 })
    expect(resolveTimeRange({ from: 10, to: 20 })).toEqual({ from: 10, to: 20 })
  })

  it('returns null when range is empty or incomplete', () => {
    expect(resolveTimeRange(null)).toBeNull()
    expect(resolveTimeRange({})).toBeNull()
    expect(resolveTimeRange({ from: 1 })).toBeNull()
  })

  it('calendar today: start of calendar day in zone through now (rolling upper bound)', () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-01-15T14:05:30.000Z'))
    const r = resolveTimeRange(
      { from: 1, to: 2, calendarPreset: 'today' },
      'UTC',
    )
    expect(r).toEqual({
      from: new Date('2026-01-15T00:00:00.000Z').getTime(),
      to: new Date('2026-01-15T14:05:30.000Z').getTime(),
    })
    vi.useRealTimers()
  })

  it('calendar yesterday: full previous calendar day in zone', () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-01-15T14:05:30.000Z'))
    const r = resolveTimeRange({ calendarPreset: 'yesterday' }, 'UTC')
    expect(r).toEqual({
      from: new Date('2026-01-14T00:00:00.000Z').getTime(),
      to: new Date('2026-01-14T23:59:59.000Z').getTime(),
    })
    vi.useRealTimers()
  })

  it('calendar this_month: full current calendar month in UTC', () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-01-15T14:00:00.000Z'))
    const r = resolveTimeRange({ calendarPreset: 'this_month' }, 'UTC')
    expect(r).toEqual({
      from: new Date('2026-01-01T00:00:00.000Z').getTime(),
      to: new Date('2026-01-31T23:59:59.000Z').getTime(),
    })
    vi.useRealTimers()
  })

  it('calendar last_month: full previous calendar month in UTC', () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-03-10T12:00:00.000Z'))
    const r = resolveTimeRange({ calendarPreset: 'last_month' }, 'UTC')
    expect(r).toEqual({
      from: new Date('2026-02-01T00:00:00.000Z').getTime(),
      to: new Date('2026-02-28T23:59:59.000Z').getTime(),
    })
    vi.useRealTimers()
  })

  it('timeRangeLogicalKey distinguishes calendar vs minute preset', () => {
    expect(timeRangeLogicalKey({ activePreset: 10, calendarPreset: 'today' })).toBe('c:today')
    expect(timeRangeLogicalKey({ activePreset: 10 })).toBe('p:10')
    expect(timeRangeLogicalKey({ calendarPreset: 'today' })).toBe('c:today')
    expect(timeRangeLogicalKey({ calendarPreset: 'this_month' })).toBe('c:this_month')
    expect(timeRangeLogicalKey({ calendarPreset: 'last_month' })).toBe('c:last_month')
  })
})
