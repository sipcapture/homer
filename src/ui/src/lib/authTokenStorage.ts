/**
 * Browser session helpers for the bundled UI.
 *
 * Auth is the HttpOnly cookie `homer_session` set by the coordinator on login
 * (shared across tabs; not readable from JS). The JWT is never written to
 * localStorage or sessionStorage (GHSA-rqwc-fmx3-95j8).
 */

export const AUTH_TOKEN_KEY = 'homer_v4_token'

/** React/API sentinel when authenticated via HttpOnly cookie only. */
export const COOKIE_SESSION_MARKER = '__homer_cookie__'

/** Deduplicate OAuth one-time → JWT exchange when React Strict Mode runs effects twice. */
const oauthTokenExchangeInflight = new Map<string, Promise<void>>()

function purgeScriptVisibleTokens(): void {
  try {
    if (typeof localStorage !== 'undefined') {
      localStorage.removeItem(AUTH_TOKEN_KEY)
    }
  } catch {
    // ignore quota / disabled storage
  }
  try {
    if (typeof sessionStorage !== 'undefined') {
      sessionStorage.removeItem(AUTH_TOKEN_KEY)
    }
  } catch {
    // ignore
  }
}

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

/** Drop leftover JWTs from older UI versions that stored them in web storage. */
export function migrateLegacyAuthToken(): void {
  purgeScriptVisibleTokens()
}

export function isCookieSessionMarker(token: string | null | undefined): boolean {
  return token === COOKIE_SESSION_MARKER
}

/** Always empty: the session JWT is HttpOnly and not readable from JS. */
export function getAuthToken(): string {
  migrateLegacyAuthToken()
  return ''
}

/** remember is ignored; JWT is never persisted in script-visible storage. */
export function setAuthToken(_token: string, _remember = false): void {
  purgeScriptVisibleTokens()
}

export function clearAuthToken(): void {
  setAuthToken('')
}
