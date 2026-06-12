import { afterEach, beforeEach, describe, expect, it } from 'vitest'
import {
  AUTH_TOKEN_KEY,
  COOKIE_SESSION_MARKER,
  clearAuthToken,
  getAuthToken,
  isCookieSessionMarker,
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

  it('stores remembered token in localStorage only', () => {
    setAuthToken('jwt-abc', true)
    expect(getAuthToken()).toBe('jwt-abc')
    expect(localStorage.getItem(AUTH_TOKEN_KEY)).toBe('jwt-abc')
    expect(sessionStorage.getItem(AUTH_TOKEN_KEY)).toBeNull()
  })

  it('does not persist token when remember is false', () => {
    setAuthToken('jwt-abc', false)
    expect(getAuthToken()).toBe('')
    expect(localStorage.getItem(AUTH_TOKEN_KEY)).toBeNull()
  })

  it('migrates legacy sessionStorage token to localStorage', () => {
    sessionStorage.setItem(AUTH_TOKEN_KEY, 'legacy-jwt')
    migrateLegacyAuthToken()
    expect(getAuthToken()).toBe('legacy-jwt')
    expect(sessionStorage.getItem(AUTH_TOKEN_KEY)).toBeNull()
  })

  it('clearAuthToken removes persisted token', () => {
    setAuthToken('x', true)
    clearAuthToken()
    expect(getAuthToken()).toBe('')
    expect(localStorage.getItem(AUTH_TOKEN_KEY)).toBeNull()
  })

  it('recognizes cookie session marker', () => {
    expect(isCookieSessionMarker(COOKIE_SESSION_MARKER)).toBe(true)
    expect(isCookieSessionMarker('jwt')).toBe(false)
  })
})
