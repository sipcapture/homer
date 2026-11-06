import { describe, expect, it } from 'vitest'
import {
  COMMON_TIME_ZONES,
  buildTimeZoneList,
  filterTimeZones,
  getTimeZones,
  isValidTimeZone,
  timeZoneLabel,
} from './timezones'

describe('buildTimeZoneList', () => {
  it('starts with local and UTC, then the supported zones sorted', () => {
    const list = buildTimeZoneList(() => ['Europe/Berlin', 'America/Sao_Paulo', 'UTC', 'Asia/Tokyo'])
    expect(list).toEqual(['local', 'UTC', 'America/Sao_Paulo', 'Asia/Tokyo', 'Europe/Berlin'])
  })

  it('renames Europe/Kiev to Europe/Kyiv without duplicates', () => {
    const list = buildTimeZoneList(() => ['Europe/Kiev', 'Europe/Kyiv'])
    expect(list).toEqual(['local', 'UTC', 'Europe/Kyiv'])
  })

  it('drops identifiers the runtime cannot format', () => {
    const list = buildTimeZoneList(() => ['Europe/Berlin', 'Not/AZone'])
    expect(list).toEqual(['local', 'UTC', 'Europe/Berlin'])
  })

  it('falls back when Intl.supportedValuesOf is unavailable', () => {
    expect(buildTimeZoneList(undefined)).toEqual([...COMMON_TIME_ZONES])
  })

  it('falls back when Intl.supportedValuesOf throws or returns nothing', () => {
    expect(buildTimeZoneList(() => { throw new RangeError('nope') })).toEqual([...COMMON_TIME_ZONES])
    expect(buildTimeZoneList(() => [])).toEqual([...COMMON_TIME_ZONES])
  })

  it('getTimeZones uses the real runtime list', () => {
    const list = getTimeZones()
    expect(list.slice(0, 2)).toEqual(['local', 'UTC'])
    expect(list).toContain('America/Sao_Paulo')
    expect(list).not.toContain('Europe/Kiev')
  })
})

describe('common list', () => {
  it('uses the current IANA name for Kyiv', () => {
    expect(COMMON_TIME_ZONES).toContain('Europe/Kyiv')
    expect(COMMON_TIME_ZONES).not.toContain('Europe/Kiev')
  })

  it('only contains valid zones', () => {
    for (const z of COMMON_TIME_ZONES) expect(isValidTimeZone(z)).toBe(true)
  })
})

describe('isValidTimeZone', () => {
  it('accepts local and IANA names, rejects garbage and empty input', () => {
    expect(isValidTimeZone('local')).toBe(true)
    expect(isValidTimeZone(' Asia/Dubai ')).toBe(true)
    expect(isValidTimeZone('Mars/Olympus')).toBe(false)
    expect(isValidTimeZone('  ')).toBe(false)
  })
})

describe('filterTimeZones', () => {
  const zones = ['local', 'UTC', 'America/Sao_Paulo', 'America/New_York', 'Europe/Kyiv']

  it('returns everything for an empty query', () => {
    expect(filterTimeZones(zones, '  ')).toEqual(zones)
  })

  it('matches case-insensitively and treats _ and / as spaces', () => {
    expect(filterTimeZones(zones, 'sao paulo')).toEqual(['America/Sao_Paulo'])
    expect(filterTimeZones(zones, 'SAO_PAULO')).toEqual(['America/Sao_Paulo'])
    expect(filterTimeZones(zones, 'america/')).toEqual(['America/Sao_Paulo', 'America/New_York'])
  })

  it('matches the local pseudo-zone by its label', () => {
    expect(filterTimeZones(zones, 'browser')).toEqual(['local'])
  })
})

describe('timeZoneLabel', () => {
  it('labels the local pseudo-zone', () => {
    expect(timeZoneLabel('local')).toBe('Browser (Local)')
    expect(timeZoneLabel('UTC')).toBe('UTC')
  })
})
