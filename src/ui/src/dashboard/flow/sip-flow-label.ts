/** Prefer full SIP text from API row (column names vary by backend / DuckLake). */
export function sipRawFromMessage(msg: Record<string, unknown>): string {
  const candidates = [
    'raw',
    'Raw',
    'payload',
    'Payload', // hepic-lake / DuckLake sip_messages
    'message',
    'Message',
    'data',
    'Data',
    'body',
    'Body',
    'sip_raw',
    'sipRaw',
  ]
  for (const key of candidates) {
    const v = msg[key]
    if (typeof v === 'string' && v.trim().length > 0) return v
  }
  return ''
}

function strTrim(v: unknown): string {
  if (v == null) return ''
  return String(v).trim()
}

function pick(msg: Record<string, unknown>, ...keys: string[]): string {
  for (const k of keys) {
    const s = strTrim(msg[k])
    if (s !== '') return s
  }
  return ''
}

/** Minimal status line when only response_code is present (no full payload). */
function syntheticStatusLine(code: string): string {
  const phrase =
    (
      {
        '100': 'Trying',
        '180': 'Ringing',
        '181': 'Call Is Being Forwarded',
        '182': 'Queued',
        '183': 'Session Progress',
        '200': 'OK',
        '202': 'Accepted',
        '300': 'Multiple Choices',
        '301': 'Moved Permanently',
        '302': 'Moved Temporarily',
        '400': 'Bad Request',
        '401': 'Unauthorized',
        '403': 'Forbidden',
        '404': 'Not Found',
        '407': 'Proxy Authentication Required',
        '408': 'Request Timeout',
        '486': 'Busy Here',
        '487': 'Request Terminated',
        '488': 'Not Acceptable Here',
        '500': 'Server Internal Error',
        '503': 'Service Unavailable',
      } as Record<string, string>
    )[code] ?? ''
  return phrase ? `SIP/2.0 ${code} ${phrase}` : `SIP/2.0 ${code}`
}

/**
 * When payload/raw is missing, build a minimal first line from structured columns
 * (same spirit as classic Homer when only parsed fields exist).
 */
export function sipSyntheticRawFromMetadata(msg: Record<string, unknown>): string | null {
  const rc = pick(msg, 'response_code', 'responseCode', 'respc')
  if (rc && /^\d{3}$/.test(rc)) {
    return syntheticStatusLine(rc)
  }

  const method = pick(msg, 'method', 'sip_method', 'Method')
  const ru = pick(msg, 'ruri_user', 'ruriUser')
  const rd = pick(msg, 'ruri_domain', 'ruriDomain')

  if (method && /^[A-Za-z][A-Za-z0-9_-]*$/.test(method) && ru) {
    if (rd) return `${method} sip:${ru}@${rd} SIP/2.0`
    return `${method} sip:${ru} SIP/2.0`
  }

  return null
}

export interface SipFirstLineParsed {
  primaryShort: string
  fullFirstLine: string
  kind: 'request' | 'response' | 'unknown'
}

/** First non-empty line of SIP message; short label for Call Flow header row. */
export function parseSipFirstLine(raw: string): SipFirstLineParsed | null {
  const trimmed = raw.trim()
  if (!trimmed) return null

  const nl = /\r?\n/
  const firstLine = trimmed.split(nl)[0]?.trim() ?? ''
  if (!firstLine) return null

  if (firstLine.startsWith('SIP/2.0')) {
    const m = /^SIP\/2\.0\s+(\d{3})\b/.exec(firstLine)
    const code = m?.[1]
    return {
      kind: 'response',
      primaryShort: code ?? firstLine,
      fullFirstLine: firstLine,
    }
  }

  const methodMatch = /^([A-Za-z][A-Za-z0-9._~-]*)\s/.exec(firstLine)
  if (methodMatch) {
    return {
      kind: 'request',
      primaryShort: methodMatch[1],
      fullFirstLine: firstLine,
    }
  }

  return {
    kind: 'unknown',
    primaryShort: firstLine.length > 40 ? `${firstLine.slice(0, 37)}...` : firstLine,
    fullFirstLine: firstLine,
  }
}

function rowHasSdpFlag(msg: Record<string, unknown>): boolean {
  const v = msg.sdp ?? msg.Sdp
  return v === true || v === 1 || v === '1' || v === 'true'
}

/** SIP bodies often start after CRLF CRLF; detect SDP by Content-Type or v=0 fallback. */
export function sipHasBodySdp(raw: string): boolean {
  const lower = raw.toLowerCase()
  if (lower.includes('content-type: application/sdp')) return true

  const sep = /\r?\n\r?\n/
  const m = sep.exec(raw)
  const body = m ? raw.slice(m.index + m[0].length) : ''
  if (!body) return false

  const bodyLower = body.toLowerCase()
  if (bodyLower.includes('content-type: application/sdp')) return true

  const trimmedBody = body.trimStart()
  return trimmedBody.startsWith('v=0')
}

export const SIP_FLOW_DESCRIPTION_MAX_LEN = 220

export function truncateFlowDescription(s: string, maxLen = SIP_FLOW_DESCRIPTION_MAX_LEN): string {
  if (s.length <= maxLen) return s
  return `${s.slice(0, Math.max(0, maxLen - 1))}\u2026`
}

/** Primary + secondary lines for Call Flow when SIP metadata or payload is available. */
export function computeSipFlowLabels(msg: Record<string, unknown>): { method: string; description: string } | null {
  const rawText = sipRawFromMessage(msg) || sipSyntheticRawFromMetadata(msg)
  if (!rawText) return null
  const parsed = parseSipFirstLine(rawText)
  if (!parsed) return null
  const sdp = sipHasBodySdp(rawText) || rowHasSdpFlag(msg)
  return {
    method: parsed.primaryShort + (sdp ? ' (SDP)' : ''),
    description: truncateFlowDescription(parsed.fullFirstLine),
  }
}
