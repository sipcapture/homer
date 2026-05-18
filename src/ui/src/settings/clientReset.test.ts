import { describe, expect, it, beforeEach, afterEach } from 'vitest'
import { clearDashboardLocalStorage } from './clientReset'

describe('clearDashboardLocalStorage', () => {
  beforeEach(() => {
    localStorage.clear()
  })
  afterEach(() => {
    localStorage.clear()
  })

  it('removes known dashboard keys and leaves unrelated keys', () => {
    localStorage.setItem('homer_active_dashboard', 'x')
    localStorage.setItem('results_hidden_cols_w1_p1_e_call', '[]')
    localStorage.setItem('homer_v4_token', 'keep')
    localStorage.setItem('vite-ui-theme', 'dark')

    const n = clearDashboardLocalStorage()
    expect(n).toBeGreaterThanOrEqual(2)
    expect(localStorage.getItem('homer_active_dashboard')).toBeNull()
    expect(localStorage.getItem('results_hidden_cols_w1_p1_e_call')).toBeNull()
    expect(localStorage.getItem('homer_v4_token')).toBe('keep')
    expect(localStorage.getItem('vite-ui-theme')).toBe('dark')
  })
})
