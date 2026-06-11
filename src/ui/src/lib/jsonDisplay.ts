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
  return (t.startsWith('{') && t.endsWith('}')) || (t.startsWith('[') && t.endsWith(']'))
}

/** Parse a JSON string; return the original value when it is not JSON text. */
export function parseJsonLoose(val: unknown): unknown {
  if (val == null) return val
  if (typeof val === 'object') return val
  if (typeof val !== 'string') return val
  const t = val.trim()
  if (!looksLikeJsonString(t)) return val
  try {
    return JSON.parse(t)
  } catch {
    return val
  }
}

export function isJsonDisplayable(val: unknown): boolean {
  if (val == null || val === '') return false
  if (typeof val === 'object') return true
  if (typeof val === 'string') return looksLikeJsonString(val)
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

export function highlightJSON(payload: unknown): string {
  try {
    const obj = parseJsonLoose(payload)
    const formatted = JSON.stringify(obj, null, 2)
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

/** First non-empty payload-like field on a LOG / event row. */
export function eventPayloadField(row: Record<string, unknown> | null | undefined): unknown {
  if (!row || typeof row !== 'object') return ''
  for (const k of ['payload', 'message', 'data', 'body']) {
    const v = row[k]
    if (v != null && v !== '') return v
  }
  return ''
}
