import { isSafeHttpUrl } from '@/lib/safeUrl'
import {
  buildSearchDeepLinkURL,
  hasDeepLinkFilters,
  parseSearchDeepLink,
  resolveDeepLinkTimestamp,
  type SearchDeepLinkSpec,
} from './searchDeepLink'

/** Parsed drill-down context stored in POST /api/v4/alerts payload JSON. */
export interface AlertSearchContext {
  spec: SearchDeepLinkSpec | null
  query: string
  homerUrl: string
  source: string
}

function firstNonEmpty(...vals: unknown[]): string {
  for (const v of vals) {
    if (v == null) continue
    const t = String(v).trim()
    if (t) return t
  }
  return ''
}

function asRecord(v: unknown): Record<string, unknown> | null {
  if (v && typeof v === 'object' && !Array.isArray(v)) {
    return v as Record<string, unknown>
  }
  if (typeof v === 'string') {
    const t = v.trim()
    if (!t.startsWith('{')) return null
    try {
      const parsed: unknown = JSON.parse(t)
      if (parsed && typeof parsed === 'object' && !Array.isArray(parsed)) {
        return parsed as Record<string, unknown>
      }
    } catch {
      return null
    }
  }
  return null
}

function positiveInt(v: unknown): number | undefined {
  if (v == null || v === '') return undefined
  const n = typeof v === 'number' ? v : Number(v)
  if (!Number.isFinite(n) || n <= 0) return undefined
  return Math.floor(n)
}

function specFromSearchObject(search: Record<string, unknown>): SearchDeepLinkSpec | null {
  const spec: SearchDeepLinkSpec = {}
  const fromUser = firstNonEmpty(search.from_user, search.user_from, search.caller)
  const toUser = firstNonEmpty(search.to_user, search.user_to, search.callee)
  const callId = firstNonEmpty(search.call_id, search.callid)
  const method = firstNonEmpty(search.method)
  const srcIp = firstNonEmpty(search.src_ip)
  const dstIp = firstNonEmpty(search.dst_ip)
  if (fromUser) spec.from_user = fromUser
  if (toUser) spec.to_user = toUser
  if (callId) spec.call_id = callId
  if (method) spec.method = method
  if (srcIp) spec.src_ip = srcIp
  if (dstIp) spec.dst_ip = dstIp

  const proto = positiveInt(search.proto_type ?? search.proto)
  if (proto != null) spec.proto_type = proto
  const event = firstNonEmpty(search.event_type, search.event)
  if (event) spec.event_type = event
  const limit = positiveInt(search.limit)
  if (limit != null) spec.limit = limit
  const minutes = positiveInt(search.minutes ?? search.m)
  if (minutes != null) spec.minutes = minutes

  const fromMs = positiveInt(search.from)
  const toMs = positiveInt(search.to)
  if (fromMs != null && toMs != null && toMs > fromMs) {
    spec.from_ms = fromMs
    spec.to_ms = toMs
  }

  return hasDeepLinkFilters(spec) ? spec : null
}

function specFromHomerUrl(raw: string): SearchDeepLinkSpec | null {
  const t = raw.trim()
  if (!t) return null
  const absolute = /^https?:\/\//i.test(t)
  if (absolute && !isSafeHttpUrl(t)) return null
  if (!absolute) {
    const lower = t.toLowerCase()
    if (
      lower.startsWith('javascript:') ||
      lower.startsWith('data:') ||
      lower.startsWith('vbscript:')
    ) {
      return null
    }
  }
  try {
    const u = absolute ? new URL(t) : new URL(t, 'http://homer.invalid')
    const params = new URLSearchParams(u.search)
    const hash = u.hash || ''
    const qInHash = hash.indexOf('?')
    if (qInHash >= 0) {
      const hp = new URLSearchParams(hash.slice(qInHash + 1))
      for (const [k, v] of hp) {
        if (!params.has(k)) params.set(k, v)
      }
    }
    return parseSearchDeepLink(params)
  } catch {
    return null
  }
}

/** Read retained query / search filters from an alert payload (object or JSON string). */
export function parseAlertPayload(payload: unknown): AlertSearchContext {
  const empty: AlertSearchContext = { spec: null, query: '', homerUrl: '', source: '' }
  const rec = asRecord(payload)
  if (!rec) return empty

  const nested = asRecord(rec.search)
  const specFromSearch = nested ? specFromSearchObject(nested) : null
  const specFromTop = specFromSearchObject(rec)
  const homerUrl = firstNonEmpty(rec.homer_url)
  const specFromUrl = homerUrl ? specFromHomerUrl(homerUrl) : null

  return {
    spec: specFromSearch ?? specFromUrl ?? specFromTop,
    query: firstNonEmpty(rec.query, rec.sql, nested?.query),
    homerUrl,
    source: firstNonEmpty(rec.source),
  }
}

export function alertCanOpenSearch(ctx: AlertSearchContext): boolean {
  return ctx.spec != null || (!!ctx.homerUrl && (isSafeHttpUrl(ctx.homerUrl) || ctx.homerUrl.startsWith('/')))
}

/** One-line label for the retained query / filters (table column). */
export function alertSearchSummary(ctx: AlertSearchContext): string {
  if (ctx.query) return ctx.query
  if (ctx.spec) {
    const parts: string[] = []
    if (ctx.spec.call_id) parts.push(`call_id=${ctx.spec.call_id}`)
    if (ctx.spec.src_ip) parts.push(`src_ip=${ctx.spec.src_ip}`)
    if (ctx.spec.dst_ip) parts.push(`dst_ip=${ctx.spec.dst_ip}`)
    if (ctx.spec.from_user) parts.push(`from_user=${ctx.spec.from_user}`)
    if (ctx.spec.to_user) parts.push(`to_user=${ctx.spec.to_user}`)
    if (ctx.spec.method) parts.push(`method=${ctx.spec.method}`)
    return parts.join(' ')
  }
  if (ctx.homerUrl) return ctx.homerUrl
  return ''
}

export function dashboardSearchHref(
  spec: SearchDeepLinkSpec,
  loc: { origin: string; pathname: string } = window.location,
): string {
  const ts = resolveDeepLinkTimestamp(spec)
  return buildSearchDeepLinkURL(
    { origin: loc.origin, pathname: loc.pathname || '/' },
    {
      from_user: spec.from_user,
      to_user: spec.to_user,
      call_id: spec.call_id,
      method: spec.method,
      src_ip: spec.src_ip,
      dst_ip: spec.dst_ip,
      proto_type: spec.proto_type,
      event_type: spec.event_type,
      limit: spec.limit,
    },
    ts,
  )
}

/** Leave settings and open dashboard search (full navigation so deep-link bootstrap runs). */
export function navigateToDashboardSearch(spec: SearchDeepLinkSpec): void {
  window.location.assign(dashboardSearchHref(spec))
}

/** Open a stored homer_url only when it is http(s) or a same-app relative path. */
export function navigateToAlertHomerUrl(raw: string): boolean {
  const t = raw.trim()
  if (t.startsWith('/') && !t.startsWith('//')) {
    window.location.assign(t)
    return true
  }
  if (isSafeHttpUrl(t)) {
    window.location.assign(t)
    return true
  }
  return false
}
