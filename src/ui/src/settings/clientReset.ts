/** Client-side reset helpers (Settings → Reset). */

/** Clears Cache Storage entries for this origin (service worker / fetch caches). */
export async function clearApplicationCaches(): Promise<number> {
  if (typeof window === 'undefined' || !('caches' in window)) return 0
  const w = window as Window & { caches?: CacheStorage }
  const names = await w.caches!.keys()
  await Promise.all(names.map((name) => w.caches!.delete(name)))
  return names.length
}

/** Keys and prefixes used by the dashboard / results / mini-games (not auth or theme). */
export function clearDashboardLocalStorage(): number {
  if (typeof localStorage === 'undefined') return 0
  const exact = new Set([
    'homer_active_dashboard',
    'homer_chess_state_v1',
    'homer_sipetris_sidebar_w',
  ])
  const prefixes = [
    'results_hidden_cols_',
    'results_col_order_',
    'results_otlp_metrics_tab_',
    'results_otlp_metrics_chart_type_',
    'homer_game_best_',
    'homer_game_zoom_',
  ]
  let removed = 0
  const keys: string[] = []
  for (let i = 0; i < localStorage.length; i++) {
    const k = localStorage.key(i)
    if (k) keys.push(k)
  }
  for (const k of keys) {
    if (exact.has(k) || prefixes.some((p) => k.startsWith(p))) {
      localStorage.removeItem(k)
      removed++
    }
  }
  return removed
}

/** Clears all keys in localStorage for this origin. */
export function clearAllLocalStorage(): void {
  if (typeof localStorage === 'undefined') return
  localStorage.clear()
}

/** Clears sessionStorage (legacy JWT keys if any). HttpOnly session cookies require server logout. */
export function clearAllSessionStorage(): void {
  if (typeof sessionStorage === 'undefined') return
  sessionStorage.clear()
}

/**
 * Deletes non-HttpOnly cookies visible to document.cookie for the current
 * document path. HttpOnly cookies cannot be cleared from JavaScript.
 */
export function clearAccessibleCookies(): number {
  if (typeof document === 'undefined' || !document.cookie) return 0
  const hostParts = window.location.hostname.split('.')
  const domains: string[] = [window.location.hostname]
  if (hostParts.length > 1) {
    domains.push('.' + hostParts.slice(-2).join('.'))
  }
  const names = new Set<string>()
  for (const part of document.cookie.split(';')) {
    const name = part.split('=')[0]?.trim()
    if (name) names.add(name)
  }
  const paths = [...new Set(['/', window.location.pathname])]
  const exp = 'Thu, 01 Jan 1970 00:00:00 GMT'
  for (const name of names) {
    for (const path of paths) {
      document.cookie = `${encodeURIComponent(name)}=; expires=${exp}; path=${path}`
      for (const domain of domains) {
        document.cookie = `${encodeURIComponent(name)}=; expires=${exp}; path=${path}; domain=${domain}`
      }
    }
  }
  return names.size
}
