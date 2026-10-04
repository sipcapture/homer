import { describe, expect, it } from 'vitest'
import { buildFlow } from './flow-data'
import { otlpRowToRung, otlpRungLabel } from './otlp-event-rungs'
import { DEFAULT_FILTERS } from './flowFilterPrefs'
import { applyFlowFilters } from './useFlowFilters'

const request = {
  timestamp: '2026-10-04T04:00:11.468Z',
  body: 'event sent: incoming call',
  attributes: {
    'source.address': '10.0.0.1',
    'destination.address': '10.0.0.2',
    'destination.port': 9099,
    'http.request.method': 'POST',
    'url.path': '/webhook',
  },
}

describe('otlpRowToRung', () => {
  it('draws source → destination at the row timestamp', () => {
    expect(otlpRowToRung(request, 'cid-1')).toMatchObject({
      src_ip: '10.0.0.1',
      dst_ip: '10.0.0.2',
      dst_port: '9099',
      timestamp: '2026-10-04T04:00:11.468Z',
      session_id: 'cid-1',
      method: 'POST /webhook',
    })
  })

  it('falls back to client → server when there is no directional pair', () => {
    const row = { timestamp: request.timestamp, attributes: { 'client.address': '10.0.0.3', 'server.address': '10.0.0.4' } }
    expect(otlpRowToRung(row, 'cid-1')).toMatchObject({ src_ip: '10.0.0.3', dst_ip: '10.0.0.4' })
  })

  it('labels endpoints from the alias table, exact ip:port before ip:0', () => {
    const aliases = new Map([['10.0.0.1:0', 'b2bua-client'], ['10.0.0.2:9099', 'webhook_server']])
    expect(otlpRowToRung(request, 'cid-1', aliases)).toMatchObject({ aliasSrc: 'b2bua-client', aliasDst: 'webhook_server' })
    const { hosts } = buildFlow([otlpRowToRung(request, 'cid-1', aliases)!], { grouping: 'ungrouped' })
    expect(hosts.map((h) => h.displayLabel)).toEqual(['b2bua-client', 'webhook_server'])
    expect(otlpRowToRung(request, 'cid-1', new Map())).not.toHaveProperty('aliasSrc')
  })

  it('leaves rows without both endpoints log-only', () => {
    expect(otlpRowToRung({ timestamp: request.timestamp, body: 'x', attributes: null }, 'cid-1')).toBeNull()
    expect(otlpRowToRung({ ...request, attributes: { 'source.address': '10.0.0.1' } }, 'cid-1')).toBeNull()
    expect(otlpRowToRung({ ...request, timestamp: 'not-a-date' }, 'cid-1')).toBeNull()
  })
})

describe('otlpRungLabel', () => {
  it('prefers HTTP status, then HTTP method, then RPC, then event.name, then body', () => {
    expect(otlpRungLabel({ attributes: { 'http.response.status_code': 200, 'http.request.method': 'POST' } })).toBe('HTTP 200')
    expect(otlpRungLabel(request)).toBe('POST /webhook')
    expect(otlpRungLabel({ attributes: { 'rpc.service': 'Agent', 'rpc.method': 'Bridge' } })).toBe('Agent/Bridge')
    expect(otlpRungLabel({ attributes: { 'event.name': 'bridge.connected' } })).toBe('bridge.connected')
    expect(otlpRungLabel({ body: 'x'.repeat(60), attributes: {} })).toHaveLength(40)
  })
})

describe('OTLP rungs on the ladder', () => {
  const rung = otlpRowToRung(request, 'cid-1')!

  it('renders as a dashed OTLP arrow described by the log body', () => {
    const { flowItems } = buildFlow([rung], { grouping: 'ungrouped' })
    expect(flowItems[0]).toMatchObject({ payloadType: 'OTLP', arrowStyleSolid: false, description: 'event sent: incoming call' })
  })

  it('is hidden unless showOtlpEvents is on', () => {
    expect(applyFlowFilters([rung], DEFAULT_FILTERS)).toHaveLength(0)
    expect(applyFlowFilters([rung], { ...DEFAULT_FILTERS, showOtlpEvents: true })).toHaveLength(1)
  })
})
