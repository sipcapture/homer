import type { ExactAliasMap } from '@/lib/ipAliasDisplay'
import type { RawMessage } from './flow-data'

/** One otlp_logs row as returned by POST /transactions/otlp-logs. */
export interface OtlpLogRow {
  timestamp?: string
  body?: string
  service_name?: string
  attributes?: Record<string, string | number | boolean | null> | null
  [key: string]: unknown
}

type Attrs = OtlpLogRow['attributes']

function attr(attrs: Attrs, ...keys: string[]): string {
  for (const key of keys) {
    const v = attrs?.[key]
    if (v !== undefined && v !== null && String(v).trim() !== '') return String(v).trim()
  }
  return ''
}

const MAX_BODY_LABEL = 40

// SIP rows arrive alias-enriched by the backend; these rows are built client-side, so resolve here.
function exactAlias(aliases: ExactAliasMap | null | undefined, ip: string, port: string | number): string {
  return aliases?.get(`${ip}:${Number(port) || 0}`) ?? aliases?.get(`${ip}:0`) ?? ''
}

/** Ladder label from OTel semconv when present (HTTP, RPC), else event.name, else the body. */
export function otlpRungLabel(row: OtlpLogRow): string {
  const attrs = row.attributes
  const status = attr(attrs, 'http.response.status_code')
  if (status) return `HTTP ${status}`
  const httpMethod = attr(attrs, 'http.request.method')
  if (httpMethod) return `${httpMethod} ${attr(attrs, 'url.path')}`.trim()
  const rpcMethod = attr(attrs, 'rpc.method')
  if (rpcMethod) return [attr(attrs, 'rpc.service'), rpcMethod].filter(Boolean).join('/')
  const eventName = attr(attrs, 'event.name')
  if (eventName) return eventName
  const body = String(row.body ?? '').trim()
  return body.length > MAX_BODY_LABEL ? `${body.slice(0, MAX_BODY_LABEL - 1)}…` : body
}

/**
 * A log row with semconv endpoints becomes one arrow at its own timestamp: source.* → destination.*
 * (directional), falling back to client.* → server.*. Rows without both endpoints stay log-only.
 */
export function otlpRowToRung(
  row: OtlpLogRow,
  callId: string,
  aliases?: ExactAliasMap | null,
): RawMessage | null {
  const attrs = row.attributes
  const directional = attr(attrs, 'source.address') && attr(attrs, 'destination.address')
  const src = directional ? attr(attrs, 'source.address') : attr(attrs, 'client.address')
  const dst = directional ? attr(attrs, 'destination.address') : attr(attrs, 'server.address')
  if (!src || !dst || !row.timestamp || Number.isNaN(Date.parse(row.timestamp))) return null

  const srcPort = (directional ? attr(attrs, 'source.port') : attr(attrs, 'client.port')) || 0
  const dstPort = (directional ? attr(attrs, 'destination.port') : attr(attrs, 'server.port')) || 0
  const method = otlpRungLabel(row)
  const description = String(row.body ?? '').trim()
  const aliasSrc = exactAlias(aliases, src, srcPort)
  const aliasDst = exactAlias(aliases, dst, dstPort)
  return {
    ...(aliasSrc ? { aliasSrc } : {}),
    ...(aliasDst ? { aliasDst } : {}),
    uuid: `otlp-${row.timestamp}-${src}-${dst}-${method}-${description}`,
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

/**
 * Looks up every leg and the alias table concurrently. A row matched by several legs is drawn once;
 * a failed lookup only drops that leg's rungs.
 */
export async function collectOtlpRungs(
  legIds: string[],
  fetchLeg: (legId: string) => Promise<OtlpLogRow[]>,
  fetchAliases: () => Promise<ExactAliasMap | null>,
): Promise<RawMessage[]> {
  const aliasesPending = fetchAliases().catch(() => null)
  const legsPending = Promise.all(
    legIds.map(async (legId) => {
      try {
        return { legId, rows: await fetchLeg(legId) }
      } catch {
        return { legId, rows: [] as OtlpLogRow[] }
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
  return [...rungs.values()]
}
