import type { RawMessage } from './flow-data'

function parseJSONLike(value: unknown): Record<string, unknown> | null {
  if (!value) return null
  if (typeof value === 'object' && !Array.isArray(value)) return value as Record<string, unknown>
  if (typeof value !== 'string') return null
  try {
    const parsed = JSON.parse(value)
    return parsed && typeof parsed === 'object' && !Array.isArray(parsed)
      ? (parsed as Record<string, unknown>)
      : null
  } catch {
    return null
  }
}

function positiveInt(value: unknown): number | null {
  if (value === undefined || value === null || value === '') return null
  const n = typeof value === 'number' ? value : Number(value)
  if (!Number.isFinite(n) || n <= 0) return null
  return n
}

/** HEP proto_type from a tagged row or data_extra — not the IP `protocol` column. */
export function hepProtoTypeOf(msg: RawMessage): number | null {
  const direct = positiveInt(msg.hep_proto_type ?? msg.proto_type)
  if (direct != null) return direct
  const extra = parseJSONLike(msg.data_extra)
  if (!extra) return null
  return positiveInt(extra.proto_type ?? extra.hep_proto_type)
}

export function tagHepProtoType(items: RawMessage[] | null | undefined, protoType: number): RawMessage[] {
  return (items ?? []).map((m) => ({ ...m, hep_proto_type: protoType }))
}

function timestampMsOf(msg: RawMessage): number {
  const value = msg.timestamp ?? msg.create_ts
  if (value === undefined || value === null || value === '') return 0
  if (value instanceof Date) {
    const t = value.getTime()
    return Number.isFinite(t) ? t : 0
  }
  if (typeof value === 'number') {
    if (!Number.isFinite(value)) return 0
    if (value > 1e15) return Math.round(value / 1e6)
    if (value > 1e12) return Math.round(value)
    if (value > 1e9) return Math.round(value * 1000)
    return value
  }
  if (typeof value === 'string') {
    let s = value.trim()
    if (/^\d{4}-\d{2}-\d{2}$/.test(s)) s = `${s}T00:00:00Z`
    else if (s.includes(' ') && !s.includes('T')) s = s.replace(' ', 'T')
    if (!/[zZ]|[+-]\d{2}:?\d{2}$/.test(s)) s = `${s}Z`
    const d = new Date(s)
    return Number.isNaN(d.getTime()) ? 0 : d.getTime()
  }
  return 0
}

/** Merge two already-tagged message lists into capture-time order. */
export function mergeFlowMessagesByTimestamp(
  primary: RawMessage[] | null | undefined,
  extra: RawMessage[] | null | undefined,
): RawMessage[] {
  const merged = [...(primary ?? []), ...(extra ?? [])]
  merged.sort((a, b) => timestampMsOf(a) - timestampMsOf(b))
  return merged
}
