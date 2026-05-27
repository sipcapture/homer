/**
 * Shared browser-side reset actions (Settings → Reset and dashboard header menu).
 */
import { toast } from 'sonner'
import { handleUnauthorized } from '../api'
import {
  clearAccessibleCookies,
  clearAllLocalStorage,
  clearAllSessionStorage,
  clearApplicationCaches,
  clearDashboardLocalStorage,
} from './clientReset'

export type ClientResetAction = 'ui_cache' | 'dashboard_local' | 'local_storage' | 'cookies'

type ConfirmOpts = { message: string; variant?: 'default' | 'destructive'; title?: string }

export type ConfirmDialog = (input: ConfirmOpts | string) => Promise<boolean>

const CONFIRM: Record<
  ClientResetAction,
  { title: string; message: string; variant?: 'default' | 'destructive' }
> = {
  ui_cache: {
    title: 'UI cache reset',
    message:
      'Clear Cache Storage for this site? Reload the page afterward if the UI still looks stale.',
  },
  dashboard_local: {
    title: 'Dashboard reset',
    message:
      'Remove dashboard-related keys from local storage (layouts, results columns, mini-game prefs)? This does not change data on the server.',
  },
  local_storage: {
    title: 'Local storage reset',
    message:
      'Clear all local and session storage for this origin? You will be signed out and the page will reload.',
    variant: 'destructive',
  },
  cookies: {
    title: 'Cookie reset',
    message:
      'Clear script-visible cookies for this site? HttpOnly cookies (e.g. some session cookies) cannot be removed from JavaScript.',
  },
}

/**
 * Runs one client reset after confirmation. Returns a line suitable for status UI or toast.
 */
export async function runClientResetWithConfirm(
  confirm: ConfirmDialog,
  action: ClientResetAction,
): Promise<{ ok: boolean; line: string }> {
  const c = CONFIRM[action]
  const okClick = await confirm({
    title: c.title,
    message: c.message,
    variant: c.variant ?? 'default',
  })
  if (!okClick) {
    return { ok: false, line: '' }
  }
  try {
    let detail = ''
    switch (action) {
      case 'ui_cache': {
        const n = await clearApplicationCaches()
        detail = n === 0 ? 'no Cache Storage buckets' : `removed ${n} cache bucket(s); reload if UI looks stale`
        break
      }
      case 'dashboard_local': {
        const n = clearDashboardLocalStorage()
        detail = `removed ${n} localStorage key(s)`
        break
      }
      case 'local_storage': {
        clearAllLocalStorage()
        clearAllSessionStorage()
        handleUnauthorized()
        window.setTimeout(() => window.location.reload(), 200)
        detail = 'page will reload; you will be signed out'
        break
      }
      case 'cookies': {
        const n = clearAccessibleCookies()
        detail =
          n === 0
            ? 'no script-visible cookies (HttpOnly cannot be cleared here)'
            : `cleared ${n} cookie name(s) (best-effort)`
        break
      }
      default:
        detail = 'unknown action'
    }
    const line = `${c.title}: ${detail}`
    toast.success(line)
    return { ok: true, line }
  } catch (err: unknown) {
    const msg = err instanceof Error ? err.message : String(err)
    toast.error(msg)
    return { ok: false, line: `${CONFIRM[action].title} failed: ${msg}` }
  }
}
