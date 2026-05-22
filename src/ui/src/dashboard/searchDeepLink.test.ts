import { describe, expect, it } from 'vitest'
import {
  buildSearchDeepLinkURL,
  buildSearchPayload,
  parseSearchDeepLink,
  resolveDeepLinkTimestamp,
} from './searchDeepLink'

describe('parseSearchDeepLink', () => {
  it('parses from_user and aliases', () => {
    const p = new URLSearchParams('from_user=alice&minutes=15')
    const spec = parseSearchDeepLink(p)
    expect(spec?.from_user).toBe('alice')
    expect(spec?.minutes).toBe(15)
  })

  it('accepts homer-app user_from alias', () => {
    const p = new URLSearchParams('user_from=bob')
    expect(parseSearchDeepLink(p)?.from_user).toBe('bob')
  })

  it('parses legacy homer-app JSON via q= param', () => {
    const json = JSON.stringify({
      timestamp: { from: 1000, to: 2000 },
      param: { search: { '1_call': { user_from: '123123' } }, limit: 25 },
    })
    const p = new URLSearchParams({ q: json })
    const spec = parseSearchDeepLink(p)
    expect(spec?.from_user).toBe('123123')
    expect(spec?.from_ms).toBe(1000)
    expect(spec?.to_ms).toBe(2000)
    expect(spec?.limit).toBe(25)
  })

  it('returns null when no filter keys', () => {
    expect(parseSearchDeepLink(new URLSearchParams('minutes=10'))).toBeNull()
  })
})

describe('buildSearchPayload', () => {
  it('maps from_user into SIP filter', () => {
    const payload = buildSearchPayload(
      { from_user: 'x', proto_type: 1, event_type: 'call' },
      { from: 1, to: 2 },
      100,
    )
    expect(payload.filter.from_user).toBe('x')
    expect(payload.filter.proto_type).toBe(1)
    expect(payload.param.limit).toBe(100)
  })
})

describe('buildSearchDeepLinkURL', () => {
  it('builds hash dashboard link with query', () => {
    const url = buildSearchDeepLinkURL(
      { origin: 'http://localhost:9080', pathname: '/' },
      { from_user: '123' },
      { from: 100, to: 200 },
    )
    expect(url).toContain('from_user=123')
    expect(url).toContain('#dashboard')
    expect(url).toContain('from=100')
  })
})

describe('resolveDeepLinkTimestamp', () => {
  it('defaults to 60 minutes window', () => {
    const ts = resolveDeepLinkTimestamp({}, 1_000_000)
    expect(ts.to).toBe(1_000_000)
    expect(ts.from).toBe(1_000_000 - 60 * 60 * 1000)
  })
})
