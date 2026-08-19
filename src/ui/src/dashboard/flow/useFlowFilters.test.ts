import { describe, expect, it } from 'vitest'
import { DEFAULT_FILTERS } from './flowFilterPrefs'
import { applyFlowFilters } from './useFlowFilters'
import type { RawMessage } from './flow-data'

const sip: RawMessage = {
  uuid: 'sip1',
  src_ip: '10.0.0.1',
  dst_ip: '10.0.0.2',
  method: 'INVITE',
  session_id: 'c1',
  hep_proto_type: 1,
}

const rtcp: RawMessage = {
  uuid: 'rtcp1',
  src_ip: '10.0.0.1',
  dst_ip: '10.0.0.2',
  session_id: 'c1',
  protocol: 17,
  hep_proto_type: 5,
}

describe('applyFlowFilters showRtcp', () => {
  it('hides RTCP when showRtcp is false and keeps SIP', () => {
    const out = applyFlowFilters([sip, rtcp], { ...DEFAULT_FILTERS, showRtcp: false })
    expect(out.map((m) => m.uuid)).toEqual(['sip1'])
  })

  it('shows RTCP when showRtcp is true', () => {
    const out = applyFlowFilters([sip, rtcp], { ...DEFAULT_FILTERS, showRtcp: true })
    expect(out.map((m) => m.uuid)).toEqual(['sip1', 'rtcp1'])
  })
})
