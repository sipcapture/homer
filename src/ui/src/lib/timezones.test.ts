import { describe, expect, it } from 'vitest'
import {
  COMMON_TIME_ZONES,
  LEGACY_TIME_ZONE_ALIASES,
  buildCommonTimeZones,
  buildTimeZoneList,
  canonicalTimeZone,
  filterTimeZones,
  getTimeZones,
  isValidTimeZone,
  suggestTimeZones,
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

  it('lists the current name for every legacy ID V8 reports, once', () => {
    // What Chromium/Node return from Intl.supportedValuesOf('timeZone').
    const v8 = ['Asia/Calcutta', 'Asia/Katmandu', 'Asia/Saigon', 'Europe/Kiev', 'America/Buenos_Aires', 'Asia/Kolkata']
    expect(buildTimeZoneList(() => v8)).toEqual([
      'local',
      'UTC',
      'America/Argentina/Buenos_Aires',
      'Asia/Ho_Chi_Minh',
      'Asia/Kathmandu',
      'Asia/Kolkata',
      'Europe/Kyiv',
    ])
  })

  it('keeps the legacy name when the runtime does not know the current one (old ICU)', () => {
    const noKyiv = (z: string) => z !== 'Europe/Kyiv' && isValidTimeZone(z)
    expect(buildTimeZoneList(() => ['Europe/Kiev', 'Europe/Berlin'], noKyiv)).toEqual([
      'local',
      'UTC',
      'Europe/Berlin',
      'Europe/Kiev',
    ])
  })

  it('getTimeZones uses the real runtime list, with no legacy names left', () => {
    const list = getTimeZones()
    expect(list.slice(0, 2)).toEqual(['local', 'UTC'])
    expect(list).toContain('America/Sao_Paulo')
    expect(list).toContain('Asia/Kolkata')
    expect(list.filter((z) => z in LEGACY_TIME_ZONE_ALIASES)).toEqual([])
    expect(new Set(list).size).toBe(list.length)
  })
})

describe('legacy aliases', () => {
  it('only map to names this runtime accepts', () => {
    for (const current of Object.values(LEGACY_TIME_ZONE_ALIASES)) expect(isValidTimeZone(current)).toBe(true)
  })
})

describe('canonicalTimeZone', () => {
  it('fixes case, since Intl accepts zone IDs case-insensitively', () => {
    expect(canonicalTimeZone('europe/berlin')).toBe('Europe/Berlin')
    expect(canonicalTimeZone('  AMERICA/SAO_PAULO ')).toBe('America/Sao_Paulo')
    expect(canonicalTimeZone('LOCAL')).toBe('local')
  })

  it('returns the current name for legacy aliases, whatever spelling Intl resolves to', () => {
    expect(canonicalTimeZone('Asia/Calcutta')).toBe('Asia/Kolkata')
    expect(canonicalTimeZone('Asia/Kolkata')).toBe('Asia/Kolkata')
    expect(canonicalTimeZone('asia/saigon')).toBe('Asia/Ho_Chi_Minh')
  })

  it('keeps the legacy name when the current one is unknown to the runtime', () => {
    const noKyiv = (z: string) => z !== 'Europe/Kyiv' && isValidTimeZone(z)
    expect(canonicalTimeZone('Europe/Kiev', noKyiv)).toBe('Europe/Kiev')
  })

  it('returns null for unknown or empty input', () => {
    expect(canonicalTimeZone('Mars/Olympus')).toBeNull()
    expect(canonicalTimeZone('  ')).toBeNull()
  })
})

describe('buildCommonTimeZones', () => {
  it('is the common list on a current runtime', () => {
    expect(buildCommonTimeZones()).toEqual([...COMMON_TIME_ZONES])
  })

  it('swaps Europe/Kyiv for Europe/Kiev on old ICU instead of offering a zone that throws', () => {
    const noKyiv = (z: string) => z !== 'Europe/Kyiv' && isValidTimeZone(z)
    const list = buildCommonTimeZones(noKyiv)
    expect(list).toContain('Europe/Kiev')
    expect(list).not.toContain('Europe/Kyiv')
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

describe('suggestTimeZones', () => {
  const all = ['local', 'UTC', 'Africa/Cairo', 'America/Sao_Paulo', 'Europe/Kyiv']

  it('returns the short common list when the query is blank', () => {
    expect(suggestTimeZones('', all)).toEqual([...COMMON_TIME_ZONES])
    expect(suggestTimeZones('   ', all)).toEqual([...COMMON_TIME_ZONES])
  })

  it('searches the full list once the user types', () => {
    expect(suggestTimeZones('cairo', all)).toEqual(['Africa/Cairo'])
    expect(suggestTimeZones('xyzzy', all)).toEqual([])
  })

  it('searches the runtime list by default', () => {
    expect(suggestTimeZones('cairo')).toEqual(['Africa/Cairo'])
  })
})

describe('search with legacy names (V8)', () => {
  const v8List = buildTimeZoneList(() => ['Asia/Calcutta', 'Asia/Katmandu', 'Asia/Saigon', 'Europe/Kiev'])

  it.each([
    ['kolkata', ['Asia/Kolkata']],
    ['calcutta', ['Asia/Kolkata']],
    ['kathmandu', ['Asia/Kathmandu']],
    ['katmandu', ['Asia/Kathmandu']],
    ['ho chi minh', ['Asia/Ho_Chi_Minh']],
    ['saigon', ['Asia/Ho_Chi_Minh']],
    ['kyiv', ['Europe/Kyiv']],
    ['kiev', ['Europe/Kyiv']],
  ])('%s finds %j', (query, expected) => {
    expect(suggestTimeZones(query, v8List)).toEqual(expected)
  })
})
