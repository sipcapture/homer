import { describe, expect, it } from 'vitest'
import {
  buildFlow, buildHosts, endpointAlias, hostKey, hostKeyFromMessage,
  runtimeFingerprintOf, captureIdOf, consolidateFlowItems, canConsolidateFlowItems,
  payloadTypeOf,
} from './flow-data'
import type { FlowItemData, RawMessage } from './flow-data'

describe('hostKey group-by-alias', () => {
  it('uses alias prefix when enrichment provides alias', () => {
    const row = { src_ip: '10.0.0.1', src_port: 5060, aliasSrc: 'SBC' }
    expect(hostKey('10.0.0.1', 5060, 'group-by-alias', row, 'src')).toBe('alias:SBC')
  })

  it('falls back to ip:port without alias', () => {
    const row = { src_ip: '10.0.0.1', src_port: 5060 }
    expect(hostKey('10.0.0.1', 5060, 'group-by-alias', row, 'src')).toBe('10.0.0.1:5060')
  })
})

describe('buildHosts group-by-alias', () => {
  it('merges different IPs that share the same alias', () => {
    const items = [
      {
        src_ip: '10.0.0.1',
        dst_ip: '10.0.0.2',
        src_port: 5060,
        dst_port: 5060,
        aliasSrc: 'Proxy',
        aliasDst: 'UAS',
      },
      {
        src_ip: '10.0.0.3',
        dst_ip: '10.0.0.2',
        src_port: 5060,
        dst_port: 5060,
        aliasSrc: 'Proxy',
        aliasDst: 'UAS',
      },
    ]
    const hosts = buildHosts(items, 'group-by-alias')
    expect(hosts).toHaveLength(2)
    const proxy = hosts.find((h) => h.displayLabel === 'Proxy')
    expect(proxy).toBeDefined()
    expect(proxy!.ips).toEqual(expect.arrayContaining(['10.0.0.1', '10.0.0.3']))
    expect(hostKeyFromMessage(items[0], 'src', 'group-by-alias')).toBe(proxy!.key)
    expect(hostKeyFromMessage(items[1], 'src', 'group-by-alias')).toBe(proxy!.key)
  })

  it('keeps un-aliased endpoints on separate ip:port columns', () => {
    const hosts = buildHosts(
      [
        { src_ip: '10.0.0.1', dst_ip: '10.0.0.2', src_port: 5060, dst_port: 5060 },
        { src_ip: '10.0.0.1', dst_ip: '10.0.0.2', src_port: 5061, dst_port: 5060 },
      ],
      'group-by-alias',
    )
    expect(hosts.filter((h) => h.key.startsWith('10.0.0.1'))).toHaveLength(2)
  })
})

describe('buildFlow group-by-alias', () => {
  it('places messages on the same column index when aliases match', () => {
    const baseTs = new Date('2026-01-01T12:00:00.000Z').getTime()
    const { hosts, flowItems } = buildFlow(
      [
        {
          src_ip: '10.0.0.1',
          dst_ip: '10.0.0.2',
          src_port: 5060,
          dst_port: 5060,
          aliasSrc: 'SBC-A',
          aliasDst: 'UAS',
          session_id: 'c1',
          timestamp: baseTs,
        },
        {
          src_ip: '10.0.0.3',
          dst_ip: '10.0.0.2',
          src_port: 5060,
          dst_port: 5060,
          aliasSrc: 'SBC-A',
          aliasDst: 'UAS',
          session_id: 'c1',
          timestamp: baseTs + 10,
        },
      ],
      { grouping: 'group-by-alias' },
    )
    expect(hosts).toHaveLength(2)
    expect(flowItems).toHaveLength(2)
    expect(flowItems[0].start).toBe(flowItems[1].start)
    expect(endpointAlias({ src_ip: '1.2.3.4', aliasSrc: 'Edge' }, 'src')).toBe('Edge')
  })
})

function makeFlowItem(overrides: Partial<FlowItemData> & { ts?: number } = {}): FlowItemData {
  const { ts, ...rest } = overrides
  return {
    id: 'x', idx: 0, method: 'INVITE', description: '', srcIp: '10.0.0.1',
    dstIp: '10.0.0.2', srcPort: 5060, dstPort: 5060, callid: 'c1',
    callidColors: { color: '#000', tabColor: '#000', arrowColor: '#000' },
    methodColor: '#000', timeStr: '', fullDateStr: '', diffStr: '',
    protoLabel: '', payloadType: 'SIP', start: 0, middle: 1, rightEnd: 0,
    direction: false, isRadial: false, isLastHost: false, arrowStyleSolid: true,
    raw: { timestamp: ts ?? new Date('2026-01-01T00:00:00Z').getTime() },
    runtimeFingerprint: 'fp1',
    captureId: '1001',
    ...rest,
  }
}

describe('runtimeFingerprintOf', () => {
  it('returns empty string when payload is missing', () => {
    expect(runtimeFingerprintOf({ src_ip: '10.0.0.1', src_port: 5060, dst_ip: '10.0.0.2', dst_port: 5060 })).toBe('')
  })

  it('returns empty string when ports are missing', () => {
    expect(runtimeFingerprintOf({ src_ip: '10.0.0.1', dst_ip: '10.0.0.2', payload: 'INVITE sip:bob SIP/2.0' })).toBe('')
  })

  it('produces the same hash for identical inputs', () => {
    const msg: RawMessage = { src_ip: '10.0.0.1', src_port: 5060, dst_ip: '10.0.0.2', dst_port: 5060, payload: 'INVITE sip:bob SIP/2.0' }
    expect(runtimeFingerprintOf(msg)).toBe(runtimeFingerprintOf({ ...msg }))
  })

  it('produces different hashes for different payloads', () => {
    const base: RawMessage = { src_ip: '10.0.0.1', src_port: 5060, dst_ip: '10.0.0.2', dst_port: 5060 }
    const a = runtimeFingerprintOf({ ...base, payload: 'INVITE sip:bob SIP/2.0' })
    const b = runtimeFingerprintOf({ ...base, payload: 'BYE sip:bob SIP/2.0' })
    expect(a).not.toBe('')
    expect(a).not.toBe(b)
  })

  it('returns a pre-existing fingerprint field without recomputing', () => {
    const msg: RawMessage = {
      src_ip: '10.0.0.1', src_port: 5060, dst_ip: '10.0.0.2', dst_port: 5060,
      payload: 'INVITE sip:bob SIP/2.0', fingerprint: 'precomputed-abc123',
    }
    expect(runtimeFingerprintOf(msg)).toBe('precomputed-abc123')
  })
})

describe('captureIdOf', () => {
  it('reads capture_id from the top-level row', () => {
    expect(captureIdOf({ capture_id: '1001' } as RawMessage)).toBe('1001')
  })

  it('falls back to node_id', () => {
    expect(captureIdOf({ node_id: 42 } as RawMessage)).toBe('42')
  })

  it('reads capture_id from data_extra JSON string', () => {
    expect(captureIdOf({ data_extra: JSON.stringify({ capture_id: '2002' }) } as RawMessage)).toBe('2002')
  })

  it('returns empty string when nothing is present', () => {
    expect(captureIdOf({} as RawMessage)).toBe('')
  })
})

describe('consolidateFlowItems', () => {
  it('returns items without subItems when disabled', () => {
    const items = [makeFlowItem({ id: 'a' }), makeFlowItem({ id: 'b', captureId: '1002' })]
    const result = consolidateFlowItems(items, { enabled: false, timeThresholdMs: 500 })
    expect(result).toHaveLength(2)
    expect(result.every((i) => i.subItems === undefined)).toBe(true)
  })

  it('consolidates items with same fingerprint and different capture IDs within threshold', () => {
    const base = new Date('2026-01-01T00:00:00Z').getTime()
    const a = makeFlowItem({ id: 'a', captureId: '1001', ts: base })
    const b = makeFlowItem({ id: 'b', captureId: '1002', ts: base + 100 })
    const result = consolidateFlowItems([a, b], { enabled: true, timeThresholdMs: 500 })
    expect(result).toHaveLength(1)
    expect(result[0].subItems).toHaveLength(1)
    expect(result[0].subItems![0].captureId).toBe('1002')
  })

  it('does not consolidate items outside the time threshold', () => {
    const base = new Date('2026-01-01T00:00:00Z').getTime()
    const a = makeFlowItem({ id: 'a', captureId: '1001', ts: base })
    const b = makeFlowItem({ id: 'b', captureId: '1002', ts: base + 1000 })
    const result = consolidateFlowItems([a, b], { enabled: true, timeThresholdMs: 500 })
    expect(result).toHaveLength(2)
    expect(result.every((i) => i.subItems === undefined)).toBe(true)
  })

  it('does not consolidate items with the same capture ID', () => {
    const base = new Date('2026-01-01T00:00:00Z').getTime()
    const a = makeFlowItem({ id: 'a', captureId: '1001', ts: base })
    const b = makeFlowItem({ id: 'b', captureId: '1001', ts: base + 100 })
    const result = consolidateFlowItems([a, b], { enabled: true, timeThresholdMs: 500 })
    expect(result).toHaveLength(2)
  })

  it('does not consolidate items with different fingerprints', () => {
    const base = new Date('2026-01-01T00:00:00Z').getTime()
    const a = makeFlowItem({ id: 'a', captureId: '1001', runtimeFingerprint: 'fp1', ts: base })
    const b = makeFlowItem({ id: 'b', captureId: '1002', runtimeFingerprint: 'fp2', ts: base + 100 })
    const result = consolidateFlowItems([a, b], { enabled: true, timeThresholdMs: 500 })
    expect(result).toHaveLength(2)
  })

  it('does not mutate original items', () => {
    const base = new Date('2026-01-01T00:00:00Z').getTime()
    const a = makeFlowItem({ id: 'a', captureId: '1001', ts: base })
    const b = makeFlowItem({ id: 'b', captureId: '1002', ts: base + 100 })
    const original = [a, b]
    consolidateFlowItems(original, { enabled: true, timeThresholdMs: 500 })
    expect(original[0].subItems).toBeUndefined()
    expect(original[1].subItems).toBeUndefined()
  })
})

describe('canConsolidateFlowItems', () => {
  it('returns true when the same fingerprint appears with different capture IDs', () => {
    const items = [
      makeFlowItem({ id: 'a', captureId: '1001', runtimeFingerprint: 'fp1' }),
      makeFlowItem({ id: 'b', captureId: '1002', runtimeFingerprint: 'fp1' }),
    ]
    expect(canConsolidateFlowItems(items)).toBe(true)
  })

  it('returns false when fingerprints exist but capture IDs are unique per fingerprint', () => {
    const items = [
      makeFlowItem({ id: 'a', captureId: '1001', runtimeFingerprint: 'fp1' }),
      makeFlowItem({ id: 'b', captureId: '1001', runtimeFingerprint: 'fp1' }),
      makeFlowItem({ id: 'c', captureId: '1002', runtimeFingerprint: 'fp2' }),
    ]
    expect(canConsolidateFlowItems(items)).toBe(false)
  })

  it('returns false when capture IDs or fingerprints are missing', () => {
    expect(canConsolidateFlowItems([
      makeFlowItem({ id: 'a', captureId: '', runtimeFingerprint: 'fp1' }),
      makeFlowItem({ id: 'b', captureId: '1002', runtimeFingerprint: '' }),
    ])).toBe(false)
  })
})

describe('payloadTypeOf', () => {
  it('treats untagged UDP SIP (IP protocol 17) as SIP', () => {
    expect(payloadTypeOf({ protocol: 17, method: 'INVITE' })).toBe('SIP')
  })

  it('classifies HEP proto 5 as RTCP even when protocol is UDP 17', () => {
    expect(payloadTypeOf({ protocol: 17, hep_proto_type: 5 })).toBe('RTCP')
  })

  it('reads proto_type from data_extra', () => {
    expect(payloadTypeOf({ protocol: 17, data_extra: { proto_type: 5 } })).toBe('RTCP')
  })
})

describe('buildFlow RTCP rows', () => {
  it('uses compact RTCP labels and a dotted arrow', () => {
    const { flowItems } = buildFlow(
      [
        {
          uuid: 'sr1',
          src_ip: '10.0.0.1',
          dst_ip: '10.0.0.2',
          src_port: 4000,
          dst_port: 4001,
          protocol: 17,
          session_id: 'c1',
          timestamp: new Date('2026-01-01T12:00:00.000Z').getTime(),
          hep_proto_type: 5,
          payload: JSON.stringify({
            type: 200,
            report_blocks: [{ ia_jitter: 9, fraction_lost: 0 }],
          }),
        },
      ],
      { grouping: 'ungrouped' },
    )
    expect(flowItems).toHaveLength(1)
    expect(flowItems[0].method).toBe('RTCP SR')
    expect(flowItems[0].description).toBe('jitter=9 loss=0%')
    expect(flowItems[0].payloadType).toBe('RTCP')
    expect(flowItems[0].arrowStyleSolid).toBe(false)
  })
})
