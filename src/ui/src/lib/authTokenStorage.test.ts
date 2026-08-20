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

  it('never writes the JWT to localStorage, even when remember is true', () => {
    setAuthToken('jwt-abc', true)
    expect(getAuthToken()).toBe('')
    expect(localStorage.getItem(AUTH_TOKEN_KEY)).toBeNull()
    expect(sessionStorage.getItem(AUTH_TOKEN_KEY)).toBeNull()
  })

  it('does not persist token when remember is false', () => {
    setAuthToken('jwt-abc', false)
    expect(getAuthToken()).toBe('')
    expect(localStorage.getItem(AUTH_TOKEN_KEY)).toBeNull()
  })

  it('purges leftover sessionStorage and localStorage JWTs', () => {
    sessionStorage.setItem(AUTH_TOKEN_KEY, 'legacy-jwt')
    localStorage.setItem(AUTH_TOKEN_KEY, 'legacy-ls')
    migrateLegacyAuthToken()
    expect(getAuthToken()).toBe('')
    expect(sessionStorage.getItem(AUTH_TOKEN_KEY)).toBeNull()
    expect(localStorage.getItem(AUTH_TOKEN_KEY)).toBeNull()
  })

  it('clearAuthToken removes any leftover token', () => {
    localStorage.setItem(AUTH_TOKEN_KEY, 'x')
    clearAuthToken()
    expect(getAuthToken()).toBe('')
    expect(localStorage.getItem(AUTH_TOKEN_KEY)).toBeNull()
  })

  it('recognizes cookie session marker', () => {
    expect(isCookieSessionMarker(COOKIE_SESSION_MARKER)).toBe(true)
    expect(isCookieSessionMarker('jwt')).toBe(false)
  })
})
