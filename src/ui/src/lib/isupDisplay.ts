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
