import { describe, expect, it } from 'vitest'
import { isSafeHttpUrl, normalizeHttpUrl } from './safeUrl'

describe('safeUrl', () => {
  it('accepts http and https', () => {
    expect(isSafeHttpUrl('https://grafana.example/d/1')).toBe(true)
    expect(isSafeHttpUrl('http://localhost:3000/')).toBe(true)
  })

  it('rejects dangerous schemes', () => {
    expect(isSafeHttpUrl('javascript:alert(1)')).toBe(false)
    expect(isSafeHttpUrl('data:text/html,<script>alert(1)</script>')).toBe(false)
  })

  it('normalizeHttpUrl adds https and validates', () => {
    expect(normalizeHttpUrl('grafana.example')).toBe('https://grafana.example')
    expect(normalizeHttpUrl('javascript:alert(1)')).toBe('')
  })
})
