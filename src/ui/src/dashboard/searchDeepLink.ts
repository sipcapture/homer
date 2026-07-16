/**
 * Dashboard search deep links (homer-app Share-style URLs, simplified).
 * Supports query before hash (?from_user=x#dashboard) or in hash (#dashboard?from_user=x).
 */

export interface SearchDeepLinkSpec {
  from_user?: string
  to_user?: string
  call_id?: string
  method?: string
  src_ip?: string
  dst_ip?: string
  proto_type?: number
  event_type?: string
  limit?: number
  minutes?: number
  from_ms?: number
  to_ms?: number
}

export interface SearchPayload {
  filter: Record<string, unknown>
  param: { limit: number }
  timestamp: { from?: number; to?: number }
}

const FILTER_PARAM_KEYS = [
  'from_user',
  'user_from',
  'caller',
  'to_user',
  'user_to',
  'callee',
  'call_id',
  'callid',
  'method',
  'src_ip',
  'dst_ip',
  'proto_type',
  'proto',
  'event_type',
  'event',
  'limit',
  'minutes',
  'm',
  'from',
  'to',
  'auto_search',
] as const

/** Params that trigger a deep-link search when present. */
const TRIGGER_KEYS = new Set([
  'from_user',
  'user_from',
  'caller',
  'to_user',
  'user_to',
  'callee',
  'call_id',
  'callid',
  'method',
  'src_ip',
  'dst_ip',
])

export function getDeepLinkSearchParams(): URLSearchParams {
  const hash = window.location.hash || ''
  const qInHash = hash.indexOf('?')
  if (qInHash >= 0) {
    return new URLSearchParams(hash.slice(qInHash + 1))
  }
  const search = window.location.search || ''
  if (search.startsWith('?')) {
    return new URLSearchParams(search)
  }
  return new URLSearchParams()
}

function firstNonEmpty(...vals: (string | null | undefined)[]): string | undefined {
  for (const v of vals) {
    const t = v?.trim()
    if (t) return t
  }
  return undefined
}

function parsePositiveInt(raw: string | null, fallback?: number): number | undefined {
  if (raw == null || raw === '') return fallback
  const n = Number(raw)
  if (!Number.isFinite(n) || n <= 0) return fallback
  return Math.floor(n)
}

/**
 * Parse homer-app legacy blob: /search/result?{json} — entire query string is JSON.
 */
function tryParseHomerAppLegacyRawSearch(): SearchDeepLinkSpec | null {
  const search = window.location.search || ''
  if (search.length < 2) return null
  const raw = search.startsWith('?') ? search.slice(1) : search
  if (!raw.trimStart().startsWith('{')) return null
  return parseHomerAppLegacyJson(raw)
}

function tryParseHomerAppLegacySearch(params: URLSearchParams): SearchDeepLinkSpec | null {
  const fromRaw = tryParseHomerAppLegacyRawSearch()
  if (fromRaw) return fromRaw
  const raw = params.get('') ?? params.get('q')
  if (!raw || !raw.trimStart().startsWith('{')) return null
  return parseHomerAppLegacyJson(raw)
}

function parseHomerAppLegacyJson(raw: string): SearchDeepLinkSpec | null {
  try {
    const doc = JSON.parse(decodeURIComponent(raw)) as {
      timestamp?: { from?: number; to?: number }
      param?: {
        search?: Record<string, Record<string, string>>
        limit?: number
      }
    }
    const spec: SearchDeepLinkSpec = {}
    if (doc.timestamp?.from != null && doc.timestamp?.to != null) {
      spec.from_ms = Number(doc.timestamp.from)
      spec.to_ms = Number(doc.timestamp.to)
    }
    const search = doc.param?.search
    if (search && typeof search === 'object') {
      for (const profile of Object.values(search)) {
        if (!profile || typeof profile !== 'object') continue
        const p = profile as Record<string, string>
        spec.from_user = firstNonEmpty(spec.from_user, p.from_user, p.user_from, p.caller)
        spec.to_user = firstNonEmpty(spec.to_user, p.to_user, p.user_to, p.callee)
        spec.call_id = firstNonEmpty(spec.call_id, p.call_id, p.callid)
        spec.method = firstNonEmpty(spec.method, p.method)
        spec.src_ip = firstNonEmpty(spec.src_ip, p.src_ip)
        spec.dst_ip = firstNonEmpty(spec.dst_ip, p.dst_ip)
      }
    }
    if (doc.param?.limit != null) spec.limit = Number(doc.param.limit)
    return hasDeepLinkFilters(spec) ? spec : null
  } catch {
    return null
  }
}

export function hasDeepLinkFilters(spec: SearchDeepLinkSpec | null | undefined): boolean {
  if (!spec) return false
  return !!(
    spec.from_user ||
    spec.to_user ||
    spec.call_id ||
    spec.method ||
    spec.src_ip ||
    spec.dst_ip
  )
}

export function parseSearchDeepLink(params?: URLSearchParams): SearchDeepLinkSpec | null {
  const p = params ?? getDeepLinkSearchParams()
  const legacy = tryParseHomerAppLegacySearch(p)
  if (legacy) return legacy

  let hasTrigger = false
  for (const key of p.keys()) {
    if (TRIGGER_KEYS.has(key)) {
      hasTrigger = true
      break
    }
  }
  if (!hasTrigger) return null

  const spec: SearchDeepLinkSpec = {}
  spec.from_user = firstNonEmpty(
    p.get('from_user'),
    p.get('user_from'),
    p.get('caller'),
  )
  spec.to_user = firstNonEmpty(p.get('to_user'), p.get('user_to'), p.get('callee'))
  spec.call_id = firstNonEmpty(p.get('call_id'), p.get('callid'))
  spec.method = p.get('method')?.trim() || undefined
  spec.src_ip = p.get('src_ip')?.trim() || undefined
  spec.dst_ip = p.get('dst_ip')?.trim() || undefined

  const proto = parsePositiveInt(p.get('proto_type') ?? p.get('proto'))
  if (proto != null) spec.proto_type = proto
  const event = p.get('event_type') ?? p.get('event')
  if (event?.trim()) spec.event_type = event.trim()

  spec.limit = parsePositiveInt(p.get('limit'), 50)
  spec.minutes = parsePositiveInt(p.get('minutes') ?? p.get('m'))

  const fromMs = parsePositiveInt(p.get('from'))
  const toMs = parsePositiveInt(p.get('to'))
  if (fromMs != null && toMs != null && toMs > fromMs) {
    spec.from_ms = fromMs
    spec.to_ms = toMs
  }

  return hasDeepLinkFilters(spec) ? spec : null
}

export function buildSearchPayload(
  spec: SearchDeepLinkSpec,
  timestamp: { from?: number; to?: number },
  limit = 50,
): SearchPayload {
  const filter: Record<string, unknown> = {
    proto_type: spec.proto_type ?? 1,
    // Default to "all" so deep-linked/shared searches span call, registration
    // and default SIP tables (backend merges them). See issue #870.
    event_type: spec.event_type ?? 'all',
  }
  if (spec.from_user) filter.from_user = spec.from_user
  if (spec.to_user) filter.to_user = spec.to_user
  if (spec.call_id) filter.call_id = spec.call_id
  if (spec.method) filter.method = spec.method
  if (spec.src_ip) filter.src_ip = spec.src_ip
  if (spec.dst_ip) filter.dst_ip = spec.dst_ip
  return {
    filter,
    param: { limit: spec.limit ?? limit },
    timestamp: timestamp.from != null && timestamp.to != null ? timestamp : {},
  }
}

export function resolveDeepLinkTimestamp(
  spec: SearchDeepLinkSpec,
  nowMs = Date.now(),
): { from: number; to: number } {
  if (spec.from_ms != null && spec.to_ms != null && spec.to_ms > spec.from_ms) {
    return { from: spec.from_ms, to: spec.to_ms }
  }
  const minutes = spec.minutes ?? 60
  const to = nowMs
  const from = to - minutes * 60 * 1000
  return { from, to }
}

/** Build a shareable URL; omits auto_search (default is run when filters present). */
export function buildSearchDeepLinkURL(
  base: { origin: string; pathname: string },
  fields: {
    from_user?: string
    to_user?: string
    call_id?: string
    method?: string
    src_ip?: string
    dst_ip?: string
    proto_type?: string | number
    event_type?: string
    limit?: number
  },
  time: { from: number; to: number },
): string {
  const q = new URLSearchParams()
  if (fields.from_user?.trim()) q.set('from_user', fields.from_user.trim())
  if (fields.to_user?.trim()) q.set('to_user', fields.to_user.trim())
  if (fields.call_id?.trim()) q.set('call_id', fields.call_id.trim())
  if (fields.method?.trim()) q.set('method', String(fields.method).trim())
  if (fields.src_ip?.trim()) q.set('src_ip', fields.src_ip.trim())
  if (fields.dst_ip?.trim()) q.set('dst_ip', fields.dst_ip.trim())
  if (fields.proto_type != null && String(fields.proto_type) !== '') {
    q.set('proto_type', String(fields.proto_type))
  }
  if (fields.event_type?.trim()) q.set('event_type', fields.event_type.trim())
  if (fields.limit != null && fields.limit > 0) q.set('limit', String(fields.limit))
  if (time.from > 0 && time.to > time.from) {
    q.set('from', String(Math.floor(time.from)))
    q.set('to', String(Math.floor(time.to)))
  }
  const qs = q.toString()
  return `${base.origin}${base.pathname}${qs ? `?${qs}` : ''}#dashboard`
}

/** Remove deep-link query keys from the address bar after applying. */
export function stripDeepLinkParamsFromURL(): void {
  const hash = window.location.hash || ''
  const qInHash = hash.indexOf('?')
  let newHash = hash
  if (qInHash >= 0) {
    newHash = hash.slice(0, qInHash)
  }
  const params = new URLSearchParams(window.location.search)
  for (const key of [...params.keys()]) {
    if ((FILTER_PARAM_KEYS as readonly string[]).includes(key)) {
      params.delete(key)
    }
  }
  const qs = params.toString()
  const path = window.location.pathname
  window.history.replaceState(
    {},
    '',
    `${path}${qs ? `?${qs}` : ''}${newHash || '#dashboard'}`,
  )
}

export function deepLinkSpecToFormFields(spec: SearchDeepLinkSpec): Record<string, string> {
  const out: Record<string, string> = {}
  if (spec.from_user) out.from_user = spec.from_user
  if (spec.to_user) out.to_user = spec.to_user
  if (spec.call_id) out.call_id = spec.call_id
  if (spec.method) out.method = spec.method
  if (spec.src_ip) out.src_ip = spec.src_ip
  if (spec.dst_ip) out.dst_ip = spec.dst_ip
  if (spec.proto_type != null) out.proto_type = String(spec.proto_type)
  if (spec.event_type) out.event_type = spec.event_type
  return out
}
