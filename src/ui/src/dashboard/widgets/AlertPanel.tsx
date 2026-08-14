import { useCallback, useEffect, useState } from 'react'
import { Search, Settings2, Trash2 } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Textarea } from '@/components/ui/textarea'
import { useDashboard } from '../context/DashboardContext'
import {
  alertCanOpenSearch,
  alertSearchSummary,
  navigateToAlertHomerUrl,
  navigateToDashboardSearch,
  parseAlertPayload,
} from '../alertSearch'
import { useApplyDeepLinkSearch } from '../useApplyDeepLinkSearch'

const DEFAULT_SQL =
  'SELECT method, count(*) as cnt FROM homer_lake.main.hep_proto_1_call GROUP BY method ORDER BY cnt DESC LIMIT 10'

export type AlertPanelSource = 'sql' | 'db'

export interface AlertPanelConfig {
  source?: AlertPanelSource
  query?: string
  intervalSec?: number
}

interface DashboardAlertRow {
  id: number
  severity?: string
  title?: string
  message?: string
  payload?: unknown
  created_at?: string
}

interface AlertPanelProps {
  widgetId?: string
  config?: AlertPanelConfig
  onConfigChange?: (cfg: AlertPanelConfig) => void
}

export default function AlertPanel({ config, onConfigChange }: AlertPanelProps) {
  const { apiBase, authHeader } = useDashboard()
  const applySearch = useApplyDeepLinkSearch()
  const [alerts, setAlerts] = useState<Record<string, unknown>[] | DashboardAlertRow[]>([])
  const [loading, setLoading] = useState(false)
  const [showSettings, setShowSettings] = useState(false)

  const source: AlertPanelSource = config?.source === 'db' ? 'db' : 'sql'
  const intervalSec = config?.intervalSec && config.intervalSec > 0 ? config.intervalSec : 60
  const sql = (config?.query && config.query.trim()) || DEFAULT_SQL

  const patchConfig = useCallback(
    (partial: Partial<AlertPanelConfig>) => {
      onConfigChange?.({ ...config, ...partial })
    },
    [config, onConfigChange],
  )

  const fetchSql = useCallback(async () => {
    const res = await fetch(`${apiBase}/query`, {
      method: 'POST',
      headers: { ...authHeader, 'Content-Type': 'application/json' },
      credentials: 'include',
      body: JSON.stringify({ sql, limit: 100 }),
    })
    if (!res.ok) return
    const data = await res.json()
    const items = (data?.data?.items || []) as Record<string, unknown>[]
    setAlerts(items)
  }, [apiBase, authHeader, sql])

  const fetchDb = useCallback(async () => {
    const q = new URLSearchParams({ 'page[limit]': '100' })
    const res = await fetch(`${apiBase}/alerts?${q}`, { headers: { ...authHeader }, credentials: 'include' })
    if (!res.ok) return
    const data = await res.json()
    const items = (data?.data?.items || []) as DashboardAlertRow[]
    setAlerts(items)
  }, [apiBase, authHeader])

  useEffect(() => {
    let cancelled = false
    const run = async () => {
      setLoading(true)
      try {
        if (source === 'db') await fetchDb()
        else await fetchSql()
      } catch {
        // silent
      } finally {
        if (!cancelled) setLoading(false)
      }
    }
    void run()
    const id = setInterval(() => {
      void run()
    }, intervalSec * 1000)
    return () => {
      cancelled = true
      clearInterval(id)
    }
  }, [source, intervalSec, sql, fetchDb, fetchSql])

  const handleClear = async () => {
    if (source === 'db') {
      try {
        const res = await fetch(`${apiBase}/alerts`, { method: 'DELETE', headers: { ...authHeader }, credentials: 'include' })
        if (res.ok) setAlerts([])
      } catch {
        // silent
      }
    } else {
      setAlerts([])
    }
  }

  const formatTime = (iso?: string) => {
    if (!iso) return ''
    const d = new Date(iso)
    return Number.isNaN(d.getTime()) ? iso : d.toLocaleString()
  }

  const openAlertSearch = (row: DashboardAlertRow) => {
    const ctx = parseAlertPayload(row.payload)
    if (ctx.spec) {
      if (!applySearch(ctx.spec)) {
        navigateToDashboardSearch(ctx.spec)
      }
      return
    }
    if (ctx.homerUrl) {
      navigateToAlertHomerUrl(ctx.homerUrl)
    }
  }

  return (
    <div className="flex h-full min-h-0 flex-col gap-1 overflow-hidden p-2">
      <div className="flex shrink-0 flex-wrap items-center gap-1 border-b border-border/50 pb-1">
        <Button
          type="button"
          variant="ghost"
          size="icon"
          className={`h-6 w-6 shrink-0 ${showSettings ? 'text-primary' : 'text-muted-foreground'}`}
          title="Alert settings"
          onClick={() => setShowSettings((v) => !v)}
        >
          <Settings2 className="size-3.5" />
        </Button>
        <Button
          type="button"
          variant="ghost"
          size="icon-xs"
          className="h-6 w-6"
          aria-label="Clear"
          title={source === 'db' ? 'Clear alerts in dashboard store' : 'Clear list until next refresh'}
          onClick={() => void handleClear()}
        >
          <Trash2 className="size-3" aria-hidden />
        </Button>
        {loading && (
          <span className="text-[10px] text-muted-foreground">Updating…</span>
        )}
      </div>

      {showSettings && (
        <div className="flex max-h-[45%] shrink-0 flex-col gap-2 overflow-auto rounded border border-border/60 bg-card/40 p-2 text-[10px]">
          <div className="flex flex-wrap items-end gap-2">
            <div className="grid gap-1">
              <Label className="text-muted-foreground">Source</Label>
              <Select
                value={source}
                onValueChange={(v) => patchConfig({ source: v as AlertPanelSource })}
              >
                <SelectTrigger className="h-7 w-[120px] text-xs">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="sql">SQL (lake)</SelectItem>
                  <SelectItem value="db">Alert store (POST /alerts)</SelectItem>
                </SelectContent>
              </Select>
            </div>
            {source === 'db' ? (
              <p className="max-w-xs text-[10px] leading-snug text-muted-foreground">
                Click a row with stored filters to open dashboard search.
              </p>
            ) : null}
            <div className="grid gap-1">
              <Label htmlFor="alert-interval" className="text-muted-foreground">
                Interval (sec)
              </Label>
              <Input
                id="alert-interval"
                type="number"
                min={5}
                max={86400}
                className="h-7 w-24 text-xs"
                value={intervalSec}
                onChange={(e) => {
                  const n = Number(e.target.value)
                  if (!Number.isFinite(n)) return
                  patchConfig({ intervalSec: Math.max(5, Math.min(86400, Math.floor(n))) })
                }}
              />
            </div>
          </div>
          {source === 'sql' && (
            <div className="grid min-h-0 flex-1 gap-1">
              <Label htmlFor="alert-sql" className="text-muted-foreground">
                SQL
              </Label>
              <Textarea
                id="alert-sql"
                value={sql}
                onChange={(e) => patchConfig({ query: e.target.value })}
                className="min-h-[72px] flex-1 resize-y font-mono text-[11px] leading-snug"
                spellCheck={false}
              />
            </div>
          )}
        </div>
      )}

      <div className="min-h-0 flex-1 space-y-1 overflow-auto">
        {loading && alerts.length === 0 && (
          <div className="flex flex-1 items-center justify-center text-xs text-muted-foreground">
            Loading…
          </div>
        )}
        {alerts.length === 0 && !loading && (
          <div className="flex flex-1 items-center justify-center text-xs text-muted-foreground">
            No alerts
          </div>
        )}
        {source === 'db'
          ? (alerts as DashboardAlertRow[]).map((a) => {
              const ctx = parseAlertPayload(a.payload)
              const canOpen = alertCanOpenSearch(ctx)
              const summary = alertSearchSummary(ctx)
              return (
              <div
                key={a.id}
                className={`space-y-0.5 border border-border bg-card/60 px-2 py-1.5 text-[11px] ${canOpen ? 'cursor-pointer hover:bg-accent/40' : ''}`}
                role={canOpen ? 'button' : undefined}
                tabIndex={canOpen ? 0 : undefined}
                onClick={() => {
                  if (canOpen) openAlertSearch(a)
                }}
                onKeyDown={(e) => {
                  if (!canOpen) return
                  if (e.key === 'Enter' || e.key === ' ') {
                    e.preventDefault()
                    openAlertSearch(a)
                  }
                }}
              >
                <div className="flex flex-wrap items-baseline gap-x-2 gap-y-0.5">
                  {a.severity ? (
                    <span className="font-medium text-amber-600 dark:text-amber-400">{a.severity}</span>
                  ) : null}
                  {a.title ? <span className="font-medium text-foreground">{a.title}</span> : null}
                  <span className="text-muted-foreground">{formatTime(a.created_at)}</span>
                  {canOpen ? (
                    <Search className="ml-auto size-3 shrink-0 text-muted-foreground" aria-hidden />
                  ) : null}
                </div>
                {a.message ? <div className="text-foreground/90">{a.message}</div> : null}
                {summary ? (
                  <div className="truncate font-mono text-[10px] text-muted-foreground" title={summary}>
                    {summary}
                  </div>
                ) : a.payload != null && a.payload !== '' ? (
                  <pre className="max-h-24 overflow-auto whitespace-pre-wrap break-all font-mono text-[10px] text-muted-foreground">
                    {typeof a.payload === 'string' ? a.payload : JSON.stringify(a.payload)}
                  </pre>
                ) : null}
              </div>
              )
            })
          : (alerts as Record<string, unknown>[]).map((a, i) => (
              <div
                key={i}
                className="flex flex-wrap gap-x-3 gap-y-0.5 border border-border bg-card/60 px-2 py-1.5 text-[11px]"
              >
                {Object.entries(a)
                  .filter(([k]) => !k.startsWith('storage_'))
                  .map(([k, v]) => (
                    <span key={k} className="text-foreground">
                      <span className="font-medium text-muted-foreground">{k}:</span> {String(v)}
                    </span>
                  ))}
              </div>
            ))}
      </div>
    </div>
  )
}
