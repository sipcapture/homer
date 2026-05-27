/** Allow only http(s) URLs for iframe src / anchor href (blocks javascript:, data:, etc.). */
export function isSafeHttpUrl(raw: string): boolean {
  const t = raw.trim()
  if (!t) return false
  try {
    const u = new URL(t)
    return u.protocol === 'http:' || u.protocol === 'https:'
  } catch {
    return false
  }
}

export function normalizeHttpUrl(raw: string): string {
  const t = raw.trim()
  if (!t) return ''
  const withScheme = /^https?:\/\//i.test(t) ? t : `https://${t}`
  return isSafeHttpUrl(withScheme) ? withScheme : ''
}
