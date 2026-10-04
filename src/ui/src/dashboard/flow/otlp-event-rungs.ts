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
  /** An object from /transactions/otlp-logs today; other lake paths return embedded JSON text. */
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
  const aliasSrc = exactAlias(aliases, src, srcPort)
  const aliasDst = exactAlias(aliases, dst, dstPort)
  return {
    ...(aliasSrc ? { aliasSrc } : {}),
    ...(aliasDst ? { aliasDst } : {}),
    uuid: ['otlp', row.timestamp, row.trace_id, row.span_id, src, dst, method, description].map((p) => p ?? '').join('|'),
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

/**
 * Looks up every leg and the alias table concurrently. A row matched by several legs is drawn once;
 * a failed leg lookup is counted rather than thrown, so the other legs still draw.
 */
export async function collectOtlpRungs(
  legIds: string[],
  fetchLeg: (legId: string) => Promise<OtlpLogRow[]>,
  fetchAliases: () => Promise<ExactAliasMap | null>,
): Promise<CollectedOtlpRungs> {
  const aliasesPending = fetchAliases().catch(() => null)
  const legsPending = Promise.all(
    legIds.map(async (legId) => {
      try {
        return { legId, rows: await fetchLeg(legId), failed: false }
      } catch {
        return { legId, rows: [] as OtlpLogRow[], failed: true }
      }
    }),
  )
  const [aliases, legs] = await Promise.all([aliasesPending, legsPending])
  const rungs = new Map<string, RawMessage>()
  for (const { legId, rows } of legs) {
    for (const row of rows) {
      const rung = otlpRowToRung(row, legId, aliases)
      if (rung?.uuid && !rungs.has(rung.uuid)) rungs.set(rung.uuid, rung)
    }
  }
  return { rungs: [...rungs.values()], failedLegs: legs.filter((leg) => leg.failed).length }
}
