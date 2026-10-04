import { describe, expect, it } from 'vitest'
import { buildFlow } from './flow-data'
import { collectOtlpRungs, otlpRowToRung, otlpRungLabel, type OtlpLogRow } from './otlp-event-rungs'
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
  it('labels status-only rows, then method + path, then RPC, then event.name, then body', () => {
    expect(otlpRungLabel({ attributes: { 'http.response.status_code': 200 } })).toBe('HTTP 200')
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

describe('rung identity', () => {
  it('keeps two same-millisecond rows with the same label apart when their bodies differ', () => {
    const a = otlpRowToRung({ ...request, body: 'first' }, 'cid-1')!
    const b = otlpRowToRung({ ...request, body: 'second' }, 'cid-1')!
    expect(a.uuid).not.toBe(b.uuid)
  })
})

describe('collectOtlpRungs', () => {
  it('queries every leg concurrently and merges rows matched by more than one leg', async () => {
    const started: string[] = []
    const releases: Array<() => void> = []
    const fetchLeg = (legId: string) => {
      started.push(legId)
      return new Promise<OtlpLogRow[]>((resolve) => releases.push(() => resolve([request])))
    }
    const pending = collectOtlpRungs(['leg-a', 'leg-b'], fetchLeg, async () => null)
    await Promise.resolve()
    expect(started).toEqual(['leg-a', 'leg-b'])
    releases.forEach((release) => release())
    expect((await pending).rungs).toHaveLength(1)
  })

  it('keeps other legs when one lookup fails', async () => {
    const fetchLeg = async (legId: string) => {
      if (legId === 'bad') throw new Error('boom')
      return [request]
    }
    expect((await collectOtlpRungs(['bad', 'good'], fetchLeg, async () => null)).rungs).toHaveLength(1)
  })
})

describe('review follow-ups', () => {
  it('parses attributes that arrive as a JSON string', () => {
    const row: OtlpLogRow = { ...request, attributes: JSON.stringify(request.attributes) }
    expect(otlpRowToRung(row, 'cid-1')).toMatchObject({ src_ip: '10.0.0.1', dst_ip: '10.0.0.2', method: 'POST /webhook' })
  })

  it('treats unparseable string attributes as no attributes', () => {
    expect(otlpRowToRung({ ...request, attributes: 'not json' }, 'cid-1')).toBeNull()
  })

  it('keeps events with distinct span ids apart even when everything else matches', () => {
    const a = otlpRowToRung({ ...request, span_id: 'aaaa' }, 'cid-1')!
    const b = otlpRowToRung({ ...request, span_id: 'bbbb' }, 'cid-1')!
    expect(a.uuid).not.toBe(b.uuid)
  })

  it('gives the same row the same id whichever leg found it', () => {
    expect(otlpRowToRung(request, 'leg-a')!.uuid).toBe(otlpRowToRung(request, 'leg-b')!.uuid)
  })

  it('keeps method and path next to the status', () => {
    const attributes = { 'http.request.method': 'POST', 'url.path': '/webhook', 'http.response.status_code': 200 }
    expect(otlpRungLabel({ attributes })).toBe('POST /webhook → 200')
  })

  it('reads older HTTP semconv names and drops the query string from http.target', () => {
    expect(otlpRungLabel({ attributes: { 'http.method': 'GET', 'http.target': '/calls?token=secret' } })).toBe('GET /calls')
    expect(otlpRungLabel({ attributes: { 'http.method': 'GET', 'http.target': '/calls', 'http.status_code': 404 } })).toBe(
      'GET /calls → 404',
    )
  })

  it('does not draw from net.host.* / net.peer.*, which are relative to whoever emitted the log', () => {
    const row = { timestamp: request.timestamp, attributes: { 'net.host.ip': '10.0.0.5', 'net.peer.ip': '10.0.0.6' } }
    expect(otlpRowToRung(row, 'cid-1')).toBeNull()
  })

  it('prefers http.route over the concrete path', () => {
    const attributes = { 'http.request.method': 'GET', 'http.route': '/calls/{id}', 'url.path': '/calls/abc123' }
    expect(otlpRungLabel({ attributes })).toBe('GET /calls/{id}')
  })

  it('takes the path from url.full or http.url on client-side logs, ignoring the query', () => {
    expect(otlpRungLabel({ attributes: { 'http.request.method': 'POST', 'url.full': 'https://cp.example:9099/webhook?k=v' } })).toBe(
      'POST /webhook',
    )
    expect(otlpRungLabel({ attributes: { 'http.method': 'POST', 'http.url': 'http://10.0.0.2/hook' } })).toBe('POST /hook')
    expect(otlpRungLabel({ attributes: { 'http.request.method': 'POST', 'url.full': 'not a url' } })).toBe('POST')
  })

  it('prefers newer endpoint names over older ones', () => {
    const row = { ...request, attributes: { ...request.attributes, 'net.host.ip': '10.9.9.9', 'net.peer.ip': '10.8.8.8' } }
    expect(otlpRowToRung(row, 'cid-1')).toMatchObject({ src_ip: '10.0.0.1', dst_ip: '10.0.0.2' })
  })

  it('reports how many leg lookups failed', async () => {
    const fetchLeg = async (legId: string) => {
      if (legId === 'bad') throw new Error('boom')
      return [request]
    }
    const result = await collectOtlpRungs(['bad', 'good'], fetchLeg, async () => null)
    expect(result.rungs).toHaveLength(1)
    expect(result.failedLegs).toBe(1)
  })
})
