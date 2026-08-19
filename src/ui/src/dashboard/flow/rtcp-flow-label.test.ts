import { describe, expect, it } from 'vitest'
import { computeRtcpFlowLabels } from './rtcp-flow-label'

const base = {
  src_ip: '10.0.0.1',
  dst_ip: '10.0.0.2',
  src_port: 4000,
  dst_port: 4001,
}

describe('computeRtcpFlowLabels', () => {
  it('labels SR with jitter and fraction-lost percent', () => {
    const labels = computeRtcpFlowLabels({
      ...base,
      payload: JSON.stringify({
        type: 200,
        report_blocks: [{ ia_jitter: 12, fraction_lost: 1, packets_lost: 3 }],
      }),
    })
    expect(labels.method).toBe('RTCP SR')
    expect(labels.description).toBe('jitter=12 loss=0.4%')
  })

  it('labels RR and uses packets_lost when fraction_lost is missing', () => {
    const labels = computeRtcpFlowLabels({
      ...base,
      payload: {
        type: 201,
        report_blocks: [{ ia_jitter: 4, packets_lost: 7 }],
      },
    })
    expect(labels.method).toBe('RTCP RR')
    expect(labels.description).toBe('jitter=4 loss=7')
  })

  it('labels XR reports', () => {
    expect(
      computeRtcpFlowLabels({
        ...base,
        payload: JSON.stringify({ type: 207, report_blocks: [] }),
      }).method,
    ).toBe('RTCP XR')
  })

  it('falls back to RTCP and a route description when payload is invalid', () => {
    const labels = computeRtcpFlowLabels({ ...base, payload: 'not-json' })
    expect(labels.method).toBe('RTCP')
    expect(labels.description).toContain('10.0.0.1')
    expect(labels.description).toContain('10.0.0.2')
  })
})
