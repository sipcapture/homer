import { describe, expect, it } from 'vitest'
import { buildFlow, buildHosts, endpointAlias, hostKey, hostKeyFromMessage } from './flow-data'

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
