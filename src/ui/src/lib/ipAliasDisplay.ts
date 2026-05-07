/**
 * Helpers for showing IP aliases returned by the API (aliasSrc / aliasDst).
 */

export function displaySrcIp(row: Record<string, unknown> | null | undefined): string {
  if (!row) return ''
  const a = row.aliasSrc ?? row.alias_src
  if (a != null && String(a).trim() !== '') return String(a)
  const raw = row.src_ip
  return raw == null || raw === '' ? '' : String(raw)
}

export function displayDstIp(row: Record<string, unknown> | null | undefined): string {
  if (!row) return ''
  const a = row.aliasDst ?? row.alias_dst
  if (a != null && String(a).trim() !== '') return String(a)
  const raw = row.dst_ip
  return raw == null || raw === '' ? '' : String(raw)
}

function rowTrimStr(row: Record<string, unknown>, key: string): string {
  const v = row[key]
  if (v == null || v === '') return ''
  return String(v).trim()
}

/** Image URL + tag values from row enrichment (aliasSrc_*). */
export function aliasSrcEnrichment(row: Record<string, unknown> | null | undefined): {
  image: string
  tags: string[]
} {
  if (!row) return { image: '', tags: [] }
  const image = rowTrimStr(row, 'aliasSrc_image')
  const tags = [1, 2, 3, 4]
    .map((i) => rowTrimStr(row, `aliasSrc_tag${i}`))
    .filter((t) => t.length > 0)
  return { image, tags }
}

/** Image URL + tag values from row enrichment (aliasDst_*). */
export function aliasDstEnrichment(row: Record<string, unknown> | null | undefined): {
  image: string
  tags: string[]
} {
  if (!row) return { image: '', tags: [] }
  const image = rowTrimStr(row, 'aliasDst_image')
  const tags = [1, 2, 3, 4]
    .map((i) => rowTrimStr(row, `aliasDst_tag${i}`))
    .filter((t) => t.length > 0)
  return { image, tags }
}

/** QoS / RTCP labels: "alias:port → alias:port" using enriched rows when present. */
export function qosRouteArrow(row: Record<string, unknown>): string {
  const src = displaySrcIp(row)
  const dst = displayDstIp(row)
  const sp = Number(row.src_port ?? 0) || 0
  const dp = Number(row.dst_port ?? 0) || 0
  return `${src}:${sp} → ${dst}:${dp}`
}

export type ExactAliasMap = Map<string, string>

/** Build ip:port → alias map from GET /aliases items (exact match; port 0 = wildcard row). */
export function buildExactAliasMapFromApiItems(
  items: Array<{ ip?: string; port?: number; alias?: string; status?: boolean }>,
): ExactAliasMap {
  const m = new Map<string, string>()
  for (const it of items) {
    if (it.status === false || !it.ip || !it.alias) continue
    const port = typeof it.port === 'number' ? it.port : 0
    m.set(`${it.ip}:${port}`, String(it.alias))
  }
  return m
}

/** Resolve label for live HEP stream lines (no CIDR on client — exact ip:port then ip:0). */
export function resolveExactStreamIp(
  ip: string | undefined,
  port: number | undefined,
  map: ExactAliasMap | null | undefined,
): string {
  if (!ip) return '?'
  const p = port ?? 0
  if (!map || map.size === 0) return `${ip}:${p}`
  const byPort = map.get(`${ip}:${p}`)
  if (byPort) return byPort
  const wild = map.get(`${ip}:0`)
  if (wild) return wild
  return `${ip}:${p}`
}
