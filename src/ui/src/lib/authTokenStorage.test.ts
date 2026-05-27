import { afterEach, beforeEach, describe, expect, it } from 'vitest'
import {
  AUTH_TOKEN_KEY,
  clearAuthToken,
  getAuthToken,
  migrateLegacyAuthToken,
  setAuthToken,
} from './authTokenStorage'

describe('authTokenStorage', () => {
  beforeEach(() => {
    sessionStorage.clear()
    localStorage.clear()
  })

  afterEach(() => {
    sessionStorage.clear()
    localStorage.clear()
  })

  it('stores and reads from sessionStorage', () => {
    setAuthToken('jwt-abc')
    expect(getAuthToken()).toBe('jwt-abc')
    expect(sessionStorage.getItem(AUTH_TOKEN_KEY)).toBe('jwt-abc')
  })

  it('migrates legacy localStorage token', () => {
    localStorage.setItem(AUTH_TOKEN_KEY, 'legacy-jwt')
    migrateLegacyAuthToken()
    expect(getAuthToken()).toBe('legacy-jwt')
    expect(localStorage.getItem(AUTH_TOKEN_KEY)).toBeNull()
  })

  it('clearAuthToken removes session and legacy local', () => {
    setAuthToken('x')
    localStorage.setItem(AUTH_TOKEN_KEY, 'y')
    clearAuthToken()
    expect(getAuthToken()).toBe('')
    expect(localStorage.getItem(AUTH_TOKEN_KEY)).toBeNull()
  })
})
