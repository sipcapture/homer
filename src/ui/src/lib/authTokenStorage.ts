/** Session JWT for the bundled UI (tab-scoped; not localStorage). */
export const AUTH_TOKEN_KEY = 'homer_v4_token'

/** Deduplicate OAuth one-time → JWT exchange when React Strict Mode runs effects twice. */
const oauthTokenExchangeInflight = new Map<string, Promise<void>>()

/**
 * Exchange OAuth one-time query token for JWT and persist in sessionStorage.
 * Callers should refresh React state via getAuthToken() (avoids passing JWT through App state sinks).
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
      setAuthToken(jwt)
    } finally {
      oauthTokenExchangeInflight.delete(oneTime)
    }
  })()
  oauthTokenExchangeInflight.set(oneTime, p)
  return p
}

function storage(): Storage | null {
  if (typeof sessionStorage === 'undefined') return null
  return sessionStorage
}

/** One-time migration from legacy localStorage key. */
export function migrateLegacyAuthToken(): void {
  if (typeof localStorage === 'undefined') return
  const legacy = localStorage.getItem(AUTH_TOKEN_KEY)
  if (!legacy) return
  const s = storage()
  if (s && !s.getItem(AUTH_TOKEN_KEY)) {
    s.setItem(AUTH_TOKEN_KEY, legacy)
  }
  localStorage.removeItem(AUTH_TOKEN_KEY)
}

export function getAuthToken(): string {
  migrateLegacyAuthToken()
  return storage()?.getItem(AUTH_TOKEN_KEY) || ''
}

export function setAuthToken(token: string): void {
  const s = storage()
  if (!s) return
  if (!token) {
    s.removeItem(AUTH_TOKEN_KEY)
    return
  }
  s.setItem(AUTH_TOKEN_KEY, token)
}

export function clearAuthToken(): void {
  setAuthToken('')
  if (typeof localStorage !== 'undefined') {
    localStorage.removeItem(AUTH_TOKEN_KEY)
  }
}
