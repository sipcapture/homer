/** Placeholder shown where a binary application/isup body was taken out of the SIP text. */
export const BINARY_BODY_LABEL = 'binary body - click to decode'

export type SipViewPart = { kind: 'text'; text: string } | { kind: 'binary' }

/**
 * Split a stored SIP payload for the message window.
 * An empty application/isup body (isup_body column) and a 11.0.376 hex body
 * become a binary marker. SDP and ordinary SIP stay text.
 */
export function sipViewParts(payload: string): SipViewPart[] {
  if (!payload || !/application\/isup/i.test(payload)) {
    return [{ kind: 'text', text: payload ?? '' }]
  }
  return mergeText(viewMessage(payload))
}

function viewMessage(payload: string): SipViewPart[] {
  const split = splitSIP(payload)
  if (!split) return [{ kind: 'text', text: payload }]
  const ct = headerValue(split.headers, split.nl, 'content-type') ?? ''
  const boundary = multipartBoundary(ct)
  if (boundary) {
    return [
      { kind: 'text', text: split.headers + split.nl + split.nl },
      ...viewMultipart(split.body, boundary, split.nl),
    ]
  }
  if (!isDetachedISUP(split.headers, split.nl, split.body, ct)) {
    return [{ kind: 'text', text: payload }]
  }
  const headers = removeHeader(split.headers, split.nl, 'x-homer-binary-body')
  return [
    { kind: 'text', text: headers + split.nl + split.nl },
    { kind: 'binary' },
  ]
}

function viewMultipart(body: string, boundary: string, nl: string): SipViewPart[] {
  const marker = `--${boundary}`
  if (!body.includes(marker)) return [{ kind: 'text', text: body }]
  const segments = body.split(marker)
  const out: SipViewPart[] = []
  if (segments[0]) out.push({ kind: 'text', text: segments[0] })
  for (let i = 1; i < segments.length; i++) {
    let seg = segments[i]
    if (seg.startsWith('--')) {
      out.push({ kind: 'text', text: marker + seg })
      continue
    }
    let lead = ''
    if (seg.startsWith(nl)) {
      lead = nl
      seg = seg.slice(nl.length)
    }
    let trail = ''
    if (seg.endsWith(nl)) {
      trail = nl
      seg = seg.slice(0, -nl.length)
    }
    out.push({ kind: 'text', text: marker + lead })
    out.push(...viewMessage(seg))
    if (trail) out.push({ kind: 'text', text: trail })
  }
  return out
}

function isDetachedISUP(headers: string, nl: string, body: string, contentType: string): boolean {
  if (!/application\/isup/i.test(contentType)) return false
  if (headerValue(headers, nl, 'x-homer-binary-body')) return true
  return body.trim() === ''
}

function splitSIP(payload: string): { headers: string; nl: string; body: string } | null {
  const crlf = payload.indexOf('\r\n\r\n')
  if (crlf >= 0) return { headers: payload.slice(0, crlf), nl: '\r\n', body: payload.slice(crlf + 4) }
  const lf = payload.indexOf('\n\n')
  if (lf >= 0) return { headers: payload.slice(0, lf), nl: '\n', body: payload.slice(lf + 2) }
  return null
}

function headerValue(headers: string, nl: string, name: string): string | undefined {
  for (const line of headers.split(nl)) {
    const parsed = splitHeaderLine(line)
    if (parsed && namesMatch(parsed.name, name)) return parsed.value
  }
  return undefined
}

function removeHeader(headers: string, nl: string, name: string): string {
  return headers
    .split(nl)
    .filter((line) => {
      const parsed = splitHeaderLine(line)
      return !parsed || !namesMatch(parsed.name, name)
    })
    .join(nl)
}

function splitHeaderLine(line: string): { name: string; value: string } | null {
  if (!line || line[0] === ' ' || line[0] === '\t') return null
  const colon = line.indexOf(':')
  if (colon <= 0) return null
  return { name: line.slice(0, colon).trim(), value: line.slice(colon + 1).trim() }
}

function namesMatch(got: string, want: string): boolean {
  if (got.toLowerCase() === want.toLowerCase()) return true
  if (want.toLowerCase() === 'content-type' && got.toLowerCase() === 'c') return true
  return false
}

function multipartBoundary(contentType: string): string | null {
  const semi = contentType.indexOf(';')
  const media = (semi >= 0 ? contentType.slice(0, semi) : contentType).trim().toLowerCase()
  if (!media.startsWith('multipart/')) return null
  const params = semi >= 0 ? contentType.slice(semi + 1) : ''
  const match = params.match(/boundary\s*=\s*(?:"([^"]+)"|([^;\s]+))/i)
  if (!match) return null
  return match[1] || match[2] || null
}

function mergeText(parts: SipViewPart[]): SipViewPart[] {
  const out: SipViewPart[] = []
  for (const part of parts) {
    if (part.kind === 'text' && part.text === '') continue
    const last = out[out.length - 1]
    if (part.kind === 'text' && last?.kind === 'text') {
      last.text += part.text
      continue
    }
    out.push(part.kind === 'text' ? { kind: 'text', text: part.text } : { kind: 'binary' })
  }
  return out.length > 0 ? out : [{ kind: 'text', text: '' }]
}
