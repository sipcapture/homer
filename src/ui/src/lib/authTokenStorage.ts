/**
 * Browser session helpers for the bundled UI.
 *
 * Primary auth: HttpOnly cookie `homer_session` set by the coordinator on login
 * (shared across tabs; not readable from JS).
 *
 * Optional: "Remember me" persists the JWT in localStorage for Bearer header /
 * WebSocket `access_token` fallback (less safe on XSS; see docs).
 */

export const AUTH_TOKEN_KEY = 'homer_v4_token'

/** React/API sentinel when authenticated via HttpOnly cookie only. */
export const COOKIE_SESSION_MARKER = '__homer_cookie__'

/** Deduplicate OAuth one-time → JWT exchange when React Strict Mode runs effects twice. */
const oauthTokenExchangeInflight = new Map<string, Promise<void>>()

/**
 * Exchange OAuth one-time query token for JWT; coordinator sets HttpOnly cookie.
 */
export async function exchangeOAuthOneTimeAndPersist(
  oneTime: string,
  apiBase: string,
): Promise<void> {
  const existing = oauthTokenExchangeInflight.get(oneTime)
  if (existing) {
    return existing
  }
  const p = (async () => {
    try {
      const res = await fetch(`${apiBase}/auth/oauth2/token`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        credentials: 'include',
        body: JSON.stringify({ token: oneTime }),
      })
      let detail = `Request failed (${res.status})`
      if (!res.ok) {
        try {
          const payload = (await res.json()) as { error?: { detail?: string; title?: string } }
          if (payload?.error?.detail) {
            detail = payload.error.detail
          } else if (payload?.error?.title) {
            detail = payload.error.title
          }
        } catch {
          // ignore
        }
        throw new Error(detail)
      }
      const payload = (await res.json()) as { data?: { token?: string } }
      const jwt = payload?.data?.token
      if (!jwt || typeof jwt !== 'string') {
        throw new Error('Invalid OAuth2 response: missing data.token')
      }
      setAuthToken(jwt, false)
    } finally {
      oauthTokenExchangeInflight.delete(oneTime)
    }
  })()
  oauthTokenExchangeInflight.set(oneTime, p)
  return p
}

function rememberStorage(): Storage | null {
  if (typeof localStorage === 'undefined') return null
  return localStorage
}

/** One-time migration from legacy sessionStorage key to localStorage (remember path). */
export function migrateLegacyAuthToken(): void {
  if (typeof sessionStorage === 'undefined') return
  const legacySession = sessionStorage.getItem(AUTH_TOKEN_KEY)
  if (!legacySession) return
  const ls = rememberStorage()
  if (ls && !ls.getItem(AUTH_TOKEN_KEY)) {
    ls.setItem(AUTH_TOKEN_KEY, legacySession)
  }
  sessionStorage.removeItem(AUTH_TOKEN_KEY)
}

export function isCookieSessionMarker(token: string | null | undefined): boolean {
  return token === COOKIE_SESSION_MARKER
}

/** Persisted JWT for Bearer / WebSocket (only when user chose Remember me). */
export function getAuthToken(): string {
  migrateLegacyAuthToken()
  return rememberStorage()?.getItem(AUTH_TOKEN_KEY) || ''
}

export function setAuthToken(token: string, remember = false): void {
  const ls = rememberStorage()
  if (!ls) return
  if (!token) {
    ls.removeItem(AUTH_TOKEN_KEY)
    if (typeof sessionStorage !== 'undefined') {
      sessionStorage.removeItem(AUTH_TOKEN_KEY)
    }
    return
  }
  if (remember) {
    ls.setItem(AUTH_TOKEN_KEY, token)
  } else {
    ls.removeItem(AUTH_TOKEN_KEY)
  }
  if (typeof sessionStorage !== 'undefined') {
    sessionStorage.removeItem(AUTH_TOKEN_KEY)
  }
}

export function clearAuthToken(): void {
  setAuthToken('')
}
