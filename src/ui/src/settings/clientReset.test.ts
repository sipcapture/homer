import { describe, expect, it, beforeEach, afterEach } from 'vitest'
import { AUTH_TOKEN_KEY } from '@/lib/authTokenStorage'
import { clearDashboardLocalStorage } from './clientReset'

describe('clearDashboardLocalStorage', () => {
  beforeEach(() => {
    localStorage.clear()
    sessionStorage.clear()
  })
  afterEach(() => {
    localStorage.clear()
    sessionStorage.clear()
  })

  it('removes known dashboard keys and leaves unrelated keys', () => {
    localStorage.setItem('homer_active_dashboard', 'x')
    localStorage.setItem('results_hidden_cols_w1_p1_e_call', '[]')
    localStorage.setItem('vite-ui-theme', 'dark')
    localStorage.setItem(AUTH_TOKEN_KEY, 'keep')

    const n = clearDashboardLocalStorage()
    expect(n).toBeGreaterThanOrEqual(2)
    expect(localStorage.getItem('homer_active_dashboard')).toBeNull()
    expect(localStorage.getItem('results_hidden_cols_w1_p1_e_call')).toBeNull()
    expect(localStorage.getItem(AUTH_TOKEN_KEY)).toBe('keep')
    expect(localStorage.getItem('vite-ui-theme')).toBe('dark')
  })
})
