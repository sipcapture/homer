/**
 * Sort dashboards for tabs and settings: lower `weight` first (same default as create: 10).
 * Tie-breaker: name, then id/param.
 */
export function sortDashboardsByWeight<
  T extends { weight?: number; name?: string; id?: string; param?: string },
>(items: T[]): T[] {
  const def = 10
  return [...items].sort((a, b) => {
    const wa = a.weight ?? def
    const wb = b.weight ?? def
    if (wa !== wb) return wa - wb
    const na = String(a.name ?? a.id ?? a.param ?? '')
    const nb = String(b.name ?? b.id ?? b.param ?? '')
    const c = na.localeCompare(nb)
    if (c !== 0) return c
    return String(a.id ?? a.param ?? '').localeCompare(String(b.id ?? b.param ?? ''))
  })
}
