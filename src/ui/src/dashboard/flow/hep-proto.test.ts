import { describe, expect, it } from 'vitest'
import { hepProtoTypeOf, mergeFlowMessagesByTimestamp, tagHepProtoType } from './hep-proto'
import type { RawMessage } from './flow-data'

describe('hepProtoTypeOf', () => {
  it('reads a tagged hep_proto_type field', () => {
    expect(hepProtoTypeOf({ hep_proto_type: 5 })).toBe(5)
  })

  it('reads proto_type from data_extra JSON', () => {
    expect(hepProtoTypeOf({ data_extra: '{"version":3,"proto_type":5}' })).toBe(5)
  })

  it('returns null when no HEP type is present', () => {
    expect(hepProtoTypeOf({ protocol: 17 })).toBeNull()
  })
})

describe('tagHepProtoType', () => {
  it('stamps every row', () => {
    const tagged = tagHepProtoType([{ uuid: 'a' }, { uuid: 'b' }], 5)
    expect(tagged.map((m) => m.hep_proto_type)).toEqual([5, 5])
  })
})

describe('mergeFlowMessagesByTimestamp', () => {
  it('interleaves SIP and RTCP in capture-time order', () => {
    const sip: RawMessage[] = [
      { uuid: 'invite', method: 'INVITE', timestamp: 1_000 },
      { uuid: 'bye', method: 'BYE', timestamp: 3_000 },
    ]
    const rtcp: RawMessage[] = [{ uuid: 'sr', timestamp: 2_000, hep_proto_type: 5 }]
    expect(mergeFlowMessagesByTimestamp(sip, rtcp).map((m) => m.uuid)).toEqual([
      'invite',
      'sr',
      'bye',
    ])
  })
})
