/** Shared types + parser for GET /api/v4/auth/providers (login UI). */

export interface PasswordAuthMethodRow {
  type: string
  name: string
  enable: boolean
  position: number
}

export interface ParsedLoginProviders {
  passwordMethods: PasswordAuthMethodRow[]
  oauthProviderNames: string[]
  /** Single OAuth provider with auto_redirect (only one provider is supported). */
  autoOAuthProvider: string | null
}

/**
 * GET /api/v4/auth/providers returns
 * { data: { internal, ldap, oauth2: [...] or single object } } with at most one OAuth2 row.
 */
export function parseLoginProvidersPayload(payload: unknown): ParsedLoginProviders {
  const out: ParsedLoginProviders = {
    passwordMethods: [],
    oauthProviderNames: [],
    autoOAuthProvider: null,
  }
  if (!payload || typeof payload !== 'object') {
    out.passwordMethods = [{ type: 'internal', name: 'Internal', enable: true, position: 0 }]
    return out
  }
  const data = (payload as { data?: unknown }).data
  if (!data || typeof data !== 'object') {
    out.passwordMethods = [{ type: 'internal', name: 'Internal', enable: true, position: 0 }]
    return out
  }
  const d = data as Record<string, unknown>

  const internal = d.internal
  if (internal && typeof internal === 'object') {
    const row = internal as { enable?: boolean; name?: string; type?: string; position?: number }
    out.passwordMethods.push({
      type: String(row.type || 'internal'),
      name: String(row.name || 'Internal'),
      enable: Boolean(row.enable),
      position: typeof row.position === 'number' ? row.position : 0,
    })
  } else {
    out.passwordMethods.push({ type: 'internal', name: 'Internal', enable: true, position: 0 })
  }

  const ldap = d.ldap
  if (ldap && typeof ldap === 'object') {
    const row = ldap as { enable?: boolean; name?: string; type?: string; position?: number }
    out.passwordMethods.push({
      type: String(row.type || 'ldap'),
      name: String(row.name || 'LDAP'),
      enable: Boolean(row.enable),
      position: typeof row.position === 'number' ? row.position : 1,
    })
  }

  out.passwordMethods.sort((a, b) => a.position - b.position)

  const oauth2 = d.oauth2
  const consumeOAuthRow = (p: unknown) => {
    if (!p || typeof p !== 'object') return
    const row = p as { enable?: boolean; name?: string; auto_redirect?: boolean }
    if (!row.enable || typeof row.name !== 'string') return
    const n = row.name.trim()
    if (!n) return
    out.oauthProviderNames.push(n)
    if (row.auto_redirect) {
      out.autoOAuthProvider = n
    }
  }
  if (Array.isArray(oauth2)) {
    for (const p of oauth2) {
      consumeOAuthRow(p)
      break
    }
  } else if (oauth2 && typeof oauth2 === 'object') {
    consumeOAuthRow(oauth2)
  }

  const items = d.items
  if (Array.isArray(items) && out.oauthProviderNames.length === 0) {
    for (const x of items) {
      const n =
        typeof x === 'string'
          ? x
          : x && typeof x === 'object' && typeof (x as { name?: string }).name === 'string'
            ? (x as { name: string }).name
            : ''
      if (n) {
        out.oauthProviderNames.push(n)
        break
      }
    }
  }

  return out
}
