/** Readable text for POST /messages/decoded: built-in ISUP, then tshark JSON. */
export function formatDecoded(decoded: unknown): string {
  if (!decoded || typeof decoded !== 'object') return ''
  const obj = decoded as { isup?: unknown; shark?: unknown; shark_error?: unknown }
  const parts: string[] = []
  const isup = formatISUP(obj.isup)
  if (isup) parts.push(isup)
  if (obj.shark != null) parts.push(JSON.stringify(obj.shark, null, 2))
  if (typeof obj.shark_error === 'string' && obj.shark_error) parts.push(obj.shark_error)
  return parts.join('\n\n')
}

/** Readable text for an ITU-T ISUP decode returned by POST /messages/decoded. */
export function formatISUP(isup: unknown): string {
  if (!isup || typeof isup !== 'object') return ''
  const obj = isup as { error?: unknown; messages?: unknown }
  const messages = Array.isArray(obj.messages) ? obj.messages : []
  const lines = messages
    .map((message) => {
      if (!message || typeof message !== 'object') return ''
      const summary = (message as { summary?: unknown }).summary
      return typeof summary === 'string' ? summary : ''
    })
    .filter((line) => line !== '')
  if (lines.length > 0) return lines.join('\n\n')
  return typeof obj.error === 'string' ? obj.error : ''
}
