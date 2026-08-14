import { describe, expect, it } from 'vitest'
import {
  alertCanOpenSearch,
  alertSearchSummary,
  dashboardSearchHref,
  parseAlertPayload,
} from './alertSearch'

describe('parseAlertPayload', () => {
  it('reads payload.search filters and query', () => {
    const ctx = parseAlertPayload({
      source: 'grafana',
      query: 'SELECT count(*) FROM hep_proto_1_call',
      search: {
        src_ip: '10.0.0.1',
        call_id: 'abc',
        from: 1000,
        to: 2000,
        proto_type: 1,
        event_type: 'call',
      },
    })
    expect(ctx.source).toBe('grafana')
    expect(ctx.query).toContain('hep_proto_1_call')
    expect(ctx.spec?.src_ip).toBe('10.0.0.1')
    expect(ctx.spec?.call_id).toBe('abc')
    expect(ctx.spec?.from_ms).toBe(1000)
    expect(ctx.spec?.to_ms).toBe(2000)
    expect(alertCanOpenSearch(ctx)).toBe(true)
  })

  it('parses JSON string payload', () => {
    const ctx = parseAlertPayload(JSON.stringify({ search: { from_user: 'alice' } }))
    expect(ctx.spec?.from_user).toBe('alice')
  })

  it('parses homer_url query params', () => {
    const ctx = parseAlertPayload({
      homer_url: 'https://homer.example/?src_ip=10.1.1.1&from=10&to=20#dashboard',
    })
    expect(ctx.spec?.src_ip).toBe('10.1.1.1')
    expect(ctx.homerUrl).toContain('src_ip=10.1.1.1')
  })

  it('rejects javascript homer_url', () => {
    const ctx = parseAlertPayload({ homer_url: 'javascript:alert(1)' })
    expect(ctx.spec).toBeNull()
    expect(alertCanOpenSearch(ctx)).toBe(false)
  })

  it('returns empty when payload has no search', () => {
    const ctx = parseAlertPayload({ note: 'plain text' })
    expect(ctx.spec).toBeNull()
    expect(alertCanOpenSearch(ctx)).toBe(false)
    expect(alertSearchSummary(ctx)).toBe('')
  })
})

describe('alertSearchSummary', () => {
  it('prefers retained SQL query', () => {
    expect(
      alertSearchSummary({
        spec: { src_ip: '1.1.1.1' },
        query: 'SELECT 1',
        homerUrl: '',
        source: '',
      }),
    ).toBe('SELECT 1')
  })

  it('falls back to filter summary', () => {
    expect(
      alertSearchSummary({
        spec: { src_ip: '10.0.0.1', call_id: 'x' },
        query: '',
        homerUrl: '',
        source: '',
      }),
    ).toContain('src_ip=10.0.0.1')
  })
})

describe('dashboardSearchHref', () => {
  it('builds a dashboard deep link from the spec', () => {
    const href = dashboardSearchHref(
      { src_ip: '10.0.0.1', from_ms: 100, to_ms: 200 },
      { origin: 'https://homer.example', pathname: '/' },
    )
    expect(href).toContain('src_ip=10.0.0.1')
    expect(href).toContain('from=100')
    expect(href).toContain('#dashboard')
  })
})
