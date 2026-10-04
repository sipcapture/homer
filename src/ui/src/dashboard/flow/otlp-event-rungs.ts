import type { ExactAliasMap } from '@/lib/ipAliasDisplay'
import { parseJsonLoose } from '@/lib/jsonDisplay'
import type { RawMessage } from './flow-data'

/** One otlp_logs row as returned by POST /transactions/otlp-logs. */
export interface OtlpLogRow {
  timestamp?: string
  body?: string
  service_name?: string
  trace_id?: string
  span_id?: string
  /** Allows JSON text, as lib/jsonDisplay.ts treats this field; attrsOf parses either form. */
  attributes?: AttrMap | string | null
  [key: string]: unknown
}

type AttrMap = Record<string, string | number | boolean | null>
type Attrs = AttrMap | null

function attrsOf(row: OtlpLogRow): Attrs {
  const parsed = parseJsonLoose(row.attributes)
  return parsed !== null && typeof parsed === 'object' && !Array.isArray(parsed) ? (parsed as AttrMap) : null
}

function attr(attrs: Attrs, ...keys: string[]): string {
  for (const key of keys) {
    const v = attrs?.[key]
    if (v !== undefined && v !== null && String(v).trim() !== '') return String(v).trim()
  }
  return ''
}

const MAX_BODY_LABEL = 40
const MAX_CONCURRENT_LOOKUPS = 4

/** cyrb53: a short, stable id for a row's content, so the body never lands in a DOM key or window id. */
function hashOf(text: string): string {
  let h1 = 0xdeadbeef
  let h2 = 0x41c6ce57
  for (let i = 0; i < text.length; i++) {
    const ch = text.charCodeAt(i)
    h1 = Math.imul(h1 ^ ch, 2654435761)
    h2 = Math.imul(h2 ^ ch, 1597334677)
  }
  h1 = Math.imul(h1 ^ (h1 >>> 16), 2246822507) ^ Math.imul(h2 ^ (h2 >>> 13), 3266489909)
  h2 = Math.imul(h2 ^ (h2 >>> 16), 2246822507) ^ Math.imul(h1 ^ (h1 >>> 13), 3266489909)
  return (4294967296 * (2097151 & h2) + (h1 >>> 0)).toString(36)
}

function pathOfUrl(url: string): string {
  if (!url) return ''
  try {
    return new URL(url).pathname
  } catch {
    return ''
  }
}

/** Low-cardinality route first, then the concrete path (server: url.path / http.target; client: url.full / http.url). */
function httpPathOf(attrs: Attrs): string {
  return (
    attr(attrs, 'http.route') ||
    attr(attrs, 'url.path') ||
    attr(attrs, 'http.target').split('?')[0] ||
    pathOfUrl(attr(attrs, 'url.full', 'http.url'))
  )
}

// SIP rows arrive alias-enriched by the backend; these rows are built client-side, so resolve here.
function exactAlias(aliases: ExactAliasMap | null | undefined, ip: string, port: string | number): string {
  return aliases?.get(`${ip}:${Number(port) || 0}`) ?? aliases?.get(`${ip}:0`) ?? ''
}

/** Ladder label from OTel semconv (current, then pre-1.21 names), else event.name, else the body. */
export function otlpRungLabel(row: OtlpLogRow): string {
  const attrs = attrsOf(row)
  const httpMethod = attr(attrs, 'http.request.method', 'http.method')
  const path = httpPathOf(attrs)
  const status = attr(attrs, 'http.response.status_code', 'http.status_code')
  const request = httpMethod ? `${httpMethod} ${path}`.trim() : ''
  if (request && status) return `${request} → ${status}`
  if (status) return `HTTP ${status}`
  if (request) return request
  const rpcMethod = attr(attrs, 'rpc.method')
  if (rpcMethod) return [attr(attrs, 'rpc.service'), rpcMethod].filter(Boolean).join('/')
  const eventName = attr(attrs, 'event.name')
  if (eventName) return eventName
  const body = String(row.body ?? '').trim()
  return body.length > MAX_BODY_LABEL ? `${body.slice(0, MAX_BODY_LABEL - 1)}…` : body
}

const ENDPOINT_PAIRS = [
  ['source', 'destination'],
  ['client', 'server'],
] as const

/**
 * First complete pair: source → destination, else client → server (request direction). net.host/net.peer
 * are skipped: they are relative to whichever side emitted the log, so the arrow direction is unknowable.
 */
function endpointsOf(attrs: Attrs) {
  for (const [from, to] of ENDPOINT_PAIRS) {
    const src = attr(attrs, `${from}.address`)
    const dst = attr(attrs, `${to}.address`)
    if (src && dst) return { src, dst, srcPort: attr(attrs, `${from}.port`), dstPort: attr(attrs, `${to}.port`) }
  }
  return null
}

/** A log row with semconv endpoints becomes one arrow at its own timestamp; rows without them stay log-only. */
export function otlpRowToRung(
  row: OtlpLogRow,
  callId: string,
  aliases?: ExactAliasMap | null,
): RawMessage | null {
  const endpoints = endpointsOf(attrsOf(row))
  if (!endpoints || !row.timestamp || Number.isNaN(Date.parse(row.timestamp))) return null
  const { src, dst } = endpoints
  const srcPort = endpoints.srcPort || 0
  const dstPort = endpoints.dstPort || 0
  const method = otlpRungLabel(row)
  const description = String(row.body ?? '').trim()
  const contentKey = [row.timestamp, row.trace_id, row.span_id, src, dst, method, description]
    .map((p) => p ?? '')
    .join('\u0000')
  const aliasSrc = exactAlias(aliases, src, srcPort)
  const aliasDst = exactAlias(aliases, dst, dstPort)
  return {
    ...(aliasSrc ? { aliasSrc } : {}),
    ...(aliasDst ? { aliasDst } : {}),
    uuid: `otlp-${hashOf(contentKey)}`,
    otlp_content_key: contentKey,
    timestamp: row.timestamp,
    session_id: callId,
    cid: callId,
    src_ip: src,
    src_port: srcPort,
    dst_ip: dst,
    dst_port: dstPort,
    method,
    otlp_description: description,
    flow_payload_type: 'OTLP',
    otlp_row: row,
  }
}

export interface CollectedOtlpRungs {
  rungs: RawMessage[]
  failedLegs: number
}

async function mapWithLimit<T, R>(items: T[], limit: number, fn: (item: T) => Promise<R>): Promise<R[]> {
  const out = new Array<R>(items.length)
  let next = 0
  const worker = async () => {
    while (next < items.length) {
      const i = next++
      out[i] = await fn(items[i])
    }
  }
  await Promise.all(Array.from({ length: Math.min(limit, items.length) }, worker))
  return out
}

/**
 * Looks up legs at most 4 at a time, alongside the alias table. Identical rows are drawn as many times as
 * the leg that returned the most of them, so a retry stays visible but a row found by two legs draws once.
 * A failed leg lookup is counted rather than thrown, so the other legs still draw.
 */
export async function collectOtlpRungs(
  legIds: string[],
  fetchLeg: (legId: string) => Promise<OtlpLogRow[]>,
  fetchAliases: () => Promise<ExactAliasMap | null>,
): Promise<CollectedOtlpRungs> {
  const aliasesPending = fetchAliases().catch(() => null)
  const legsPending = mapWithLimit(legIds, MAX_CONCURRENT_LOOKUPS, async (legId) => {
    try {
      return { legId, rows: await fetchLeg(legId), failed: false }
    } catch {
      return { legId, rows: [] as OtlpLogRow[], failed: true }
    }
  })
  const [aliases, legs] = await Promise.all([aliasesPending, legsPending])

  const byContent = new Map<string, { rung: RawMessage; count: number }>()
  for (const { legId, rows } of legs) {
    const perLeg = new Map<string, number>()
    for (const row of rows) {
      const rung = otlpRowToRung(row, legId, aliases)
      if (!rung) continue
      const key = String(rung.otlp_content_key)
      const seen = (perLeg.get(key) ?? 0) + 1
      perLeg.set(key, seen)
      const entry = byContent.get(key)
      if (!entry) byContent.set(key, { rung, count: seen })
      else entry.count = Math.max(entry.count, seen)
    }
  }

  const rungs: RawMessage[] = []
  const usedIds = new Set<string>()
  for (const { rung, count } of byContent.values()) {
    for (let n = 0; n < count; n++) {
      let uuid = `${rung.uuid}-${n}`
      // Different content hashing to the same id would otherwise share a React key.
      for (let k = 1; usedIds.has(uuid); k++) uuid = `${rung.uuid}-${n}-${k}`
      usedIds.add(uuid)
      rungs.push({ ...rung, uuid })
    }
  }
  return { rungs, failedLegs: legs.filter((leg) => leg.failed).length }
}
