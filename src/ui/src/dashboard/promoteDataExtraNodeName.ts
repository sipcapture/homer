/**
 * Promote HEP hostname from data_extra.node_name onto a top-level row field
 * so Results can show it (data_extra itself is hidden from the table).
 * Issue #922 / heplify -hn.
 */
export function promoteDataExtraNodeName(rows) {
  if (!Array.isArray(rows) || rows.length === 0) return rows || []
  return rows.map((row) => {
    if (!row || typeof row !== 'object') return row
    if (row.node_name != null && String(row.node_name).trim() !== '') return row
    let extra = row.data_extra
    if (extra == null) return row
    if (typeof extra === 'string') {
      const s = extra.trim()
      if (!s) return row
      try {
        extra = JSON.parse(s)
      } catch {
        return row
      }
    }
    if (!extra || typeof extra !== 'object') return row
    const name = extra.node_name
    if (name == null || String(name).trim() === '') return row
    return { ...row, node_name: String(name) }
  })
}

/** Ensure keys list includes node_name when any promoted row has it. */
export function ensureNodeNameKey(keys, rows) {
  const list = Array.isArray(keys) ? [...keys] : []
  const hasName = Array.isArray(rows) && rows.some(
    (r) => r && r.node_name != null && String(r.node_name).trim() !== '',
  )
  if (!hasName || list.includes('node_name')) return list
  const i = list.indexOf('node_id')
  if (i >= 0) {
    list.splice(i + 1, 0, 'node_name')
    return list
  }
  list.push('node_name')
  return list
}
