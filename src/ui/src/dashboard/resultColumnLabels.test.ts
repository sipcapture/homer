import { describe, expect, it } from 'vitest'
import { columnDisplayLabel, columnDisplayTitle } from './resultColumnLabels'

describe('resultColumnLabels', () => {
  it('aliases node_id as Capture ID (Homer 7 CaptureID)', () => {
    expect(columnDisplayLabel('node_id')).toBe('Capture ID')
    expect(columnDisplayTitle('node_id')).toBe('Capture ID (node_id)')
  })

  it('passes through unknown columns unchanged', () => {
    expect(columnDisplayLabel('session_id')).toBe('session_id')
    expect(columnDisplayTitle('session_id')).toBe('session_id')
  })
})
