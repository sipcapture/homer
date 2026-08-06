import { describe, expect, it } from 'vitest'
import { ensureNodeNameKey, promoteDataExtraNodeName } from './promoteDataExtraNodeName'

describe('promoteDataExtraNodeName', () => {
  it('lifts node_name from data_extra object', () => {
    const rows = promoteDataExtraNodeName([
      { node_id: '2002', data_extra: { version: 3, node_name: 'voice' } },
    ])
    expect(rows[0].node_name).toBe('voice')
    expect(rows[0].node_id).toBe('2002')
  })

  it('parses data_extra JSON string', () => {
    const rows = promoteDataExtraNodeName([
      { data_extra: JSON.stringify({ node_name: 'edge-1' }) },
    ])
    expect(rows[0].node_name).toBe('edge-1')
  })

  it('keeps existing top-level node_name', () => {
    const rows = promoteDataExtraNodeName([
      { node_name: 'keep', data_extra: { node_name: 'other' } },
    ])
    expect(rows[0].node_name).toBe('keep')
  })

  it('skips when name absent', () => {
    const rows = promoteDataExtraNodeName([{ node_id: '2002', data_extra: { version: 3 } }])
    expect(rows[0].node_name).toBeUndefined()
  })
})

describe('ensureNodeNameKey', () => {
  it('inserts after node_id', () => {
    expect(ensureNodeNameKey(['uuid', 'node_id', 'cid'], [{ node_name: 'voice' }])).toEqual([
      'uuid',
      'node_id',
      'node_name',
      'cid',
    ])
  })

  it('no-op when unused', () => {
    expect(ensureNodeNameKey(['uuid', 'node_id'], [{ node_id: '1' }])).toEqual(['uuid', 'node_id'])
  })
})
