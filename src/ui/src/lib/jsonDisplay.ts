/** Fields that often carry JSON as a VARCHAR string (Kamailio hlog, OTLP, etc.). */
export const JSON_EMBEDDED_FIELD_NAMES = new Set([
  'payload',
  'message',
  'data',
  'data_extra',
  'body',
  'raw',
  'attributes',
  'resource_attrs',
  'span_attrs',
  'body_json',
])

export function escapeHtml(str: string): string {
  return str
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
}

export function looksLikeJsonString(str: string): boolean {
  const t = str.trim()
  return (
    (t.startsWith('{') && t.endsWith('}')) ||
    (t.startsWith('[') && t.endsWith(']')) ||
    (t.startsWith('"') && t.endsWith('"'))
  )
}

/**
 * Parse JSON that may be stored as a string, including double-encoded hlog()
 * payloads (`"{\"cause\":...}"`). Returns the inner object/array, or null when
 * the input is not structured JSON.
 */
export function parseJsonDeep(input: unknown, maxDepth = 3): unknown {
  let v: unknown = input
  for (let i = 0; i <= maxDepth; i++) {
    if (v !== null && typeof v === 'object') return v
    if (typeof v !== 'string') return null
    const t = v.trim()
    if (!looksLikeJsonString(t)) return null
    try {
      v = JSON.parse(t)
    } catch {
      return null
    }
  }
  return null
}

/** Parse a JSON string; return the original value when it is not JSON text. */
export function parseJsonLoose(val: unknown): unknown {
  if (val == null) return val
  if (typeof val === 'object') return val
  if (typeof val !== 'string') return val
  const deep = parseJsonDeep(val)
  return deep ?? val
}

export function isJsonDisplayable(val: unknown): boolean {
  if (val == null || val === '') return false
  if (typeof val === 'object') return true
  if (typeof val === 'string') return parseJsonDeep(val) !== null
  return false
}

export function payloadAsString(val: unknown): string {
  if (val == null || val === '') return ''
  if (typeof val === 'string') return val
  if (typeof val === 'object') {
    try {
      return JSON.stringify(val)
    } catch {
      return String(val)
    }
  }
  return String(val)
}

/** Pretty-print a single field that may be JSON text or an object. */
export function formatJsonField(val: unknown): string {
  if (val == null) return '—'
  const parsed = parseJsonLoose(val)
  if (parsed !== val || typeof parsed === 'object') {
    try {
      return JSON.stringify(parsed, null, 2)
    } catch {
      return String(val)
    }
  }
  return String(val)
}

export function expandEmbeddedJsonReplacer(key: string, value: unknown): unknown {
  if (typeof value === 'bigint') return value.toString()
  if (typeof value === 'string' && JSON_EMBEDDED_FIELD_NAMES.has(key)) {
    const parsed = parseJsonLoose(value)
    if (parsed !== value) return parsed
  }
  return value
}

/** Serialize a search/API row for display, expanding nested JSON string fields. */
export function serializeRowForDisplay(row: Record<string, unknown>): string {
  try {
    return JSON.stringify(row, expandEmbeddedJsonReplacer, 2)
  } catch (e) {
    const err = e as { message?: string }
    return `Error serializing row: ${err?.message || e}`
  }
}

function formatJsonForHighlight(payload: unknown): string {
  if (payload !== null && typeof payload === 'object' && !Array.isArray(payload)) {
    return JSON.stringify(payload, expandEmbeddedJsonReplacer, 2)
  }
  const parsed = parseJsonLoose(payload)
  if (parsed !== payload) {
    return JSON.stringify(parsed, null, 2)
  }
  if (typeof payload === 'string') {
    return payload
  }
  return JSON.stringify(parsed, null, 2)
}

export function highlightJSON(payload: unknown): string {
  try {
    const formatted = formatJsonForHighlight(payload)
    let html = escapeHtml(formatted)
    // Value highlighting before keys — the key pass wraps colons and breaks later ": …" patterns.
    html = html.replace(
      /: &quot;([^&]*)&quot;/g,
      ': <span class="json-hl-str">&quot;$1&quot;</span>',
    )
    html = html.replace(/: (-?\d+\.?\d*)/g, ': <span class="json-hl-num">$1</span>')
    html = html.replace(/: (true|false)/g, ': <span class="json-hl-bool">$1</span>')
    html = html.replace(/: (null)/g, ': <span class="json-hl-null">$1</span>')
    html = html.replace(
      /&quot;([^&]+)&quot;(\s*:)/g,
      '<span class="json-hl-key">&quot;$1&quot;</span><span class="json-hl-punct">$2</span>',
    )
    html = html.replace(/([{}\[\],])/g, '<span class="json-hl-punct">$1</span>')
    return html
  } catch {
    return escapeHtml(payloadAsString(payload))
  }
}

/** Primary payload column names on LOG / event rows (first match wins). */
export const EVENT_PAYLOAD_FIELD_KEYS = ['payload', 'message', 'data', 'body'] as const

/** First non-empty payload-like field on a LOG / event row. */
export function eventPayloadField(row: Record<string, unknown> | null | undefined): unknown {
  if (!row) return ''
  const key = eventPayloadFieldKey(row)
  return key ? row[key] : ''
}

/** Which payload column is populated on this row, if any. */
export function eventPayloadFieldKey(
  row: Record<string, unknown> | null | undefined,
): (typeof EVENT_PAYLOAD_FIELD_KEYS)[number] | null {
  if (!row || typeof row !== 'object') return null
  for (const k of EVENT_PAYLOAD_FIELD_KEYS) {
    const v = row[k]
    if (v != null && v !== '') return k
  }
  return null
}

/**
 * Row copy without payload columns shown in the dedicated panel.
 * Strips the primary payload field plus any other populated JSON payload
 * columns (duplicate message/data/body from migration or API quirks).
 */
export function rowWithoutEventPayload(
  row: Record<string, unknown>,
  primaryKey: (typeof EVENT_PAYLOAD_FIELD_KEYS)[number] | null = eventPayloadFieldKey(row),
): Record<string, unknown> {
  if (!primaryKey) return row
  const copy = { ...row }
  delete copy[primaryKey]
  for (const k of EVENT_PAYLOAD_FIELD_KEYS) {
    if (k === primaryKey) continue
    const v = copy[k]
    if (v != null && v !== '' && isJsonDisplayable(v)) delete copy[k]
  }
  return copy
}
