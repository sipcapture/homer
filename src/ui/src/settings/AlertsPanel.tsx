import { useEffect, useState } from 'react'
import { Bell, Search } from 'lucide-react'
import { useConfirm } from '@/components/ui/confirm-dialog'
import { Button } from '@/components/ui/button'
import { apiDelete, apiGet } from '../api'
import CrudTable, { type CrudColumn } from './CrudTable'
import {
  alertCanOpenSearch,
  alertSearchSummary,
  navigateToAlertHomerUrl,
  navigateToDashboardSearch,
  parseAlertPayload,
} from '../dashboard/alertSearch'

export interface DashboardAlertRow {
  id: number
  severity?: string
  title?: string
  message?: string
  payload?: unknown
  created_at?: string
}

function formatTime(iso?: string): string {
  if (!iso) return ''
  const d = new Date(iso)
  return Number.isNaN(d.getTime()) ? iso : d.toLocaleString()
}

function openAlertSearch(row: DashboardAlertRow): void {
  const ctx = parseAlertPayload(row.payload)
  if (ctx.spec) {
    navigateToDashboardSearch(ctx.spec)
    return
  }
  if (ctx.homerUrl) {
    navigateToAlertHomerUrl(ctx.homerUrl)
  }
}

const columns: CrudColumn[] = [
  {
    key: 'created_at',
    label: 'Time',
    width: '160px',
    render: (v) => formatTime(typeof v === 'string' ? v : undefined),
  },
  {
    key: 'severity',
    label: 'Severity',
    width: '90px',
    render: (v) => {
      const s = v != null && String(v).trim() !== '' ? String(v) : '—'
      return <span className="font-medium text-amber-600 dark:text-amber-400">{s}</span>
    },
  },
  { key: 'title', label: 'Title' },
  {
    key: 'message',
    label: 'Message',
    render: (v) => {
      const s = v != null ? String(v) : ''
      if (!s) return <span className="text-muted-foreground">—</span>
      return (
        <span className="block max-w-[280px] truncate" title={s}>
          {s}
        </span>
      )
    },
  },
  {
    key: 'payload',
    label: 'Query',
    sortable: false,
    render: (_v, row) => {
      const summary = alertSearchSummary(parseAlertPayload(row.payload))
      if (!summary) return <span className="text-muted-foreground">—</span>
      return (
        <span className="block max-w-[360px] truncate font-mono text-[11px]" title={summary}>
          {summary}
        </span>
      )
    },
  },
]

export default function AlertsPanel() {
  const confirm = useConfirm()
  const [items, setItems] = useState<DashboardAlertRow[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')

  const load = async () => {
    setLoading(true)
    setError('')
    try {
      const data = await apiGet('/alerts', { 'page[limit]': 200 })
      setItems((data?.data?.items || []) as DashboardAlertRow[])
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    void load()
  }, [])

  const handleClear = async () => {
    if (
      !(await confirm({
        message: 'Delete all stored dashboard alerts?',
        variant: 'destructive',
      }))
    ) {
      return
    }
    try {
      await apiDelete('/alerts')
      setItems([])
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    }
  }

  return (
    <div className="space-y-4">
      <CrudTable
        title="Alerts"
        description="Records from POST /api/v4/alerts (Grafana or other sources). Each row keeps the query that fired. Open in search replays those filters on the dashboard."
        columns={columns}
        items={items}
        loading={loading}
        error={error}
        onLoad={() => void load()}
        idField="id"
        showActions
      >
        {(item: DashboardAlertRow) => {
          const canOpen = alertCanOpenSearch(parseAlertPayload(item.payload))
          return (
            <Button
              type="button"
              variant="outline"
              size="icon-xs"
              disabled={!canOpen}
              aria-label="Open in search"
              title={canOpen ? 'Open in search' : 'No stored search filters'}
              onClick={() => openAlertSearch(item)}
            >
              <Search className="size-3" aria-hidden />
            </Button>
          )
        }}
      </CrudTable>
      <div className="flex items-center gap-2 text-sm text-muted-foreground">
        <Bell className="size-4 shrink-0" aria-hidden />
        <span>Grafana contact points should POST here with payload.search or payload.homer_url.</span>
        <Button
          type="button"
          variant="outline"
          size="sm"
          className="ml-auto"
          disabled={items.length === 0 || loading}
          onClick={() => void handleClear()}
        >
          Clear all
        </Button>
      </div>
    </div>
  )
}
