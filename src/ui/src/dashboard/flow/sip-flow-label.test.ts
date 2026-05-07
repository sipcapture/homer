import { describe, expect, it } from 'vitest'
import {
  computeSipFlowLabels,
  parseSipFirstLine,
  sipHasBodySdp,
  sipRawFromMessage,
  sipSyntheticRawFromMetadata,
  SIP_FLOW_DESCRIPTION_MAX_LEN,
  truncateFlowDescription,
} from './sip-flow-label'
import { buildFlow } from './flow-data'

describe('sipRawFromMessage', () => {
  it('prefers raw then message then data', () => {
    expect(sipRawFromMessage({ raw: 'INVITE sip:a SIP/2.0', message: 'OTHER' })).toContain('INVITE')
    expect(sipRawFromMessage({ message: 'BYE sip:b SIP/2.0' })).toContain('BYE')
    expect(sipRawFromMessage({ Data: 'OPTIONS sip:c SIP/2.0' })).toContain('OPTIONS')
  })

  it('reads DuckLake/hepic-lake payload column', () => {
    const sip = 'INVITE sip:alice@10.0.0.2 SIP/2.0\r\n'
    expect(sipRawFromMessage({ payload: sip })).toBe(sip)
    expect(sipRawFromMessage({ Payload: sip })).toBe(sip)
  })

  it('returns empty when no text fields', () => {
    expect(sipRawFromMessage({ foo: 1 })).toBe('')
  })
})

describe('sipSyntheticRawFromMetadata', () => {
  it('builds status line from response_code', () => {
    expect(sipSyntheticRawFromMetadata({ response_code: '200' })).toBe('SIP/2.0 200 OK')
    expect(sipSyntheticRawFromMetadata({ response_code: '401' })).toBe('SIP/2.0 401 Unauthorized')
  })

  it('builds request line from method + ruri fields', () => {
    expect(
      sipSyntheticRawFromMetadata({
        method: 'INVITE',
        ruri_user: 'bob',
        ruri_domain: 'example.com',
      }),
    ).toBe('INVITE sip:bob@example.com SIP/2.0')
  })
})

describe('computeSipFlowLabels', () => {
  it('uses payload when raw is absent', () => {
    const payload =
      'INVITE sip:test@localhost SIP/2.0\r\n' +
      'Content-Type: application/sdp\r\n' +
      '\r\n' +
      'v=0\r\n'
    const labels = computeSipFlowLabels({ payload })
    expect(labels?.method).toBe('INVITE (SDP)')
    expect(labels?.description).toContain('INVITE sip:test@localhost SIP/2.0')
  })
})

describe('parseSipFirstLine', () => {
  it('parses request method and full line', () => {
    const raw =
      'INVITE sip:+491511234@sbc.example.com SIP/2.0\r\n' +
      'Via: SIP/2.0/UDP 10.0.0.1\r\n' +
      '\r\n'
    const p = parseSipFirstLine(raw)
    expect(p?.kind).toBe('request')
    expect(p?.primaryShort).toBe('INVITE')
    expect(p?.fullFirstLine).toMatch(/^INVITE sip:\+491511234@sbc\.example\.com SIP\/2\.0$/)
  })

  it('parses response code and keeps full status line', () => {
    const raw =
      'SIP/2.0 183 Session Progress\r\n' +
      'Content-Type: application/sdp\r\n' +
      '\r\n' +
      'v=0\r\n'
    const p = parseSipFirstLine(raw)
    expect(p?.kind).toBe('response')
    expect(p?.primaryShort).toBe('183')
    expect(p?.fullFirstLine).toBe('SIP/2.0 183 Session Progress')
  })

  it('returns null for empty input', () => {
    expect(parseSipFirstLine('')).toBeNull()
    expect(parseSipFirstLine('   \n  \t')).toBeNull()
  })
})

describe('sipHasBodySdp', () => {
  it('detects SDP via Content-Type header', () => {
    const raw =
      'ACK sip:x SIP/2.0\r\n' +
      'Content-Type: application/sdp\r\n' +
      '\r\n' +
      'v=0\r\n'
    expect(sipHasBodySdp(raw)).toBe(true)
  })

  it('detects SDP via v=0 body without header Content-Type', () => {
    const raw =
      'INVITE sip:x SIP/2.0\r\n' +
      'Via: SIP/2.0/UDP 10.0.0.1\r\n' +
      '\r\n' +
      'v=0\r\n' +
      'o=- 0 0 IN IP4 10.0.0.1\r\n'
    expect(sipHasBodySdp(raw)).toBe(true)
  })

  it('is false for ACK without body sdp', () => {
    const raw =
      'ACK sip:user@host SIP/2.0\r\n' +
      'Via: SIP/2.0/UDP 10.0.0.1\r\n' +
      '\r\n'
    expect(sipHasBodySdp(raw)).toBe(false)
  })

  it('is false for 100 Trying without body', () => {
    const raw = 'SIP/2.0 100 Trying\r\n' + 'Via: SIP/2.0/UDP 10.0.0.1\r\n' + '\r\n'
    expect(sipHasBodySdp(raw)).toBe(false)
  })
})

describe('truncateFlowDescription', () => {
  it('truncates long strings', () => {
    const long = 'x'.repeat(SIP_FLOW_DESCRIPTION_MAX_LEN + 50)
    const out = truncateFlowDescription(long)
    expect(out.length).toBeLessThanOrEqual(SIP_FLOW_DESCRIPTION_MAX_LEN)
    expect(out.endsWith('\u2026')).toBe(true)
  })
})

describe('buildFlow SIP labels', () => {
  const baseTs = new Date('2026-01-01T12:00:00.000Z').getTime()

  it('uses parsed SIP line when raw present', () => {
    const invite =
      'INVITE sip:bob@proxy SIP/2.0\r\n' +
      'Content-Type: application/sdp\r\n' +
      '\r\n' +
      'v=0\r\n'
    const { flowItems } = buildFlow(
      [
        {
          src_ip: '10.0.0.1',
          dst_ip: '10.0.0.2',
          src_port: 5060,
          dst_port: 5060,
          protocol: 17,
          session_id: 'c1',
          timestamp: baseTs,
          raw: invite,
        },
      ],
      { grouping: 'ungrouped' },
    )
    expect(flowItems).toHaveLength(1)
    expect(flowItems[0].method).toBe('INVITE (SDP)')
    expect(flowItems[0].description).toContain('INVITE sip:bob@proxy SIP/2.0')
  })

  it('falls back to ruri_user when raw missing', () => {
    const { flowItems } = buildFlow(
      [
        {
          src_ip: '10.0.0.1',
          dst_ip: '10.0.0.2',
          src_port: 5060,
          dst_port: 5060,
          protocol: 17,
          session_id: 'c1',
          timestamp: baseTs,
          ruri_user: 'normalized-user',
        },
      ],
      { grouping: 'ungrouped' },
    )
    expect(flowItems[0].description).toBe('normalized-user')
  })

  it('uses payload column like DuckLake when raw absent', () => {
    const invite =
      'INVITE sip:alice@10.0.0.2 SIP/2.0\r\n' +
      'Via: SIP/2.0/UDP 127.0.0.1\r\n' +
      '\r\n'
    const { flowItems } = buildFlow(
      [
        {
          src_ip: '127.0.0.1',
          dst_ip: '127.0.0.1',
          src_port: 41982,
          dst_port: 9060,
          protocol: 17,
          session_id: 'c1',
          timestamp: baseTs,
          payload: invite,
        },
      ],
      { grouping: 'ungrouped' },
    )
    expect(flowItems[0].method).toBe('INVITE')
    expect(flowItems[0].description).toBe('INVITE sip:alice@10.0.0.2 SIP/2.0')
  })

  it('uses qosRouteArrow instead of localhost -> localhost when no URI fields', () => {
    const { flowItems } = buildFlow(
      [
        {
          src_ip: '127.0.0.1',
          dst_ip: '127.0.0.1',
          src_port: 41982,
          dst_port: 9060,
          protocol: 17,
          session_id: 'c1',
          timestamp: baseTs,
        },
      ],
      { grouping: 'ungrouped' },
    )
    expect(flowItems[0].description).toBe('127.0.0.1:41982 → 127.0.0.1:9060')
  })
})
