import { useEffect, useState } from 'react'
import { Play } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import SqlEditor, {
  HOMER_SQL_WRAP_LINES_SYNC,
  loadSqlWrapLinesPreference,
  persistSqlWrapLinesPreference,
} from '@/components/ui/sql-editor'
import { useDashboard } from '../context/DashboardContext'
import { Checkbox } from '@/components/ui/checkbox'
import { Label } from '@/components/ui/label'

export default function SmartInputPanel({ config, onConfigChange }) {
  const { widgets, publishSearch, timeRange } = useDashboard()
  const [sql, setSql] = useState(() => config?.sql ?? '')
  const [sqlWrapLines, setSqlWrapLines] = useState(() => loadSqlWrapLinesPreference())

  useEffect(() => {
    const onSync = (e) => {
      if (typeof e?.detail === 'boolean') setSqlWrapLines(e.detail)
    }
    window.addEventListener(HOMER_SQL_WRAP_LINES_SYNC, onSync)
    return () => window.removeEventListener(HOMER_SQL_WRAP_LINES_SYNC, onSync)
  }, [])

  useEffect(() => {
    if (typeof config?.sql === 'string') {
      setSql(config.sql)
    }
  }, [config?.sql])

  const savedTarget = config?.targetWidget || ''
  const targetId =
    savedTarget &&
    widgets.some(
      (w) => w.id === savedTarget && (w.type === 'results' || w.type === 'chart'),
    )
      ? savedTarget
      : ''

  useEffect(() => {
    const t = config?.targetWidget
    if (!t || !onConfigChange) return
    if (!widgets.some((w) => w.id === t)) {
      onConfigChange({ ...(config || {}), targetWidget: '' })
    }
  }, [config, widgets, onConfigChange])

  const limit = Number(config?.limit || 500)
  const resultWidgets = widgets.filter((w) => w.type === 'results' || w.type === 'chart')

  const run = () => {
    if (!sql.trim()) return
    const payload = {
      sql: sql.trim(),
      useSqlEndpoint: true,
      param: { limit },
      timestamp: timeRange,
      filter: { proto_type: 1, event_type: 'call' },
    }
    if (targetId) {
      publishSearch(targetId, payload)
    } else {
      resultWidgets.forEach((w) => publishSearch(w.id, payload))
    }
  }

  return (
    <div className="flex h-full flex-col gap-2 p-2">
      <div className="flex items-center gap-2">
        <Select
          value={targetId || '__all'}
          onValueChange={(v) =>
            onConfigChange?.({ ...config, targetWidget: v === '__all' ? '' : v, sql, limit })
          }
        >
          <SelectTrigger className="h-7 flex-1 text-xs">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="__all">All result widgets</SelectItem>
            {resultWidgets.map((w) => (
              <SelectItem key={w.id} value={w.id}>
                {w.title || w.type}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <Input
          type="number"
          min={1}
          max={50000}
          value={limit}
          onChange={(e) =>
            onConfigChange?.({
              ...config,
              targetWidget: targetId,
              sql,
              limit: Number(e.target.value),
            })
          }
          className="h-7 w-24 text-xs"
        />
        <Button size="sm" onClick={run} className="h-7">
          <Play className="mr-1 h-3.5 w-3.5" />
          Run
        </Button>
      </div>
      <div className="flex items-center gap-2 px-0.5">
        <Checkbox
          id="si-sql-wrap"
          checked={sqlWrapLines}
          onCheckedChange={(c) => {
            const v = c === true
            setSqlWrapLines(v)
            persistSqlWrapLinesPreference(v)
          }}
        />
        <Label htmlFor="si-sql-wrap" className="cursor-pointer text-[11px] font-normal text-muted-foreground">
          Wrap long lines
        </Label>
      </div>
      <div className="flex min-h-0 flex-1 flex-col">
        <SqlEditor
          value={sql}
          onChange={(v) => {
            setSql(v)
            onConfigChange?.({ ...config, targetWidget: targetId, sql: v, limit })
          }}
          onSubmit={run}
          minHeight={160}
          height="100%"
          wrapLines={sqlWrapLines}
          placeholder="SELECT uuid, method, timestamp FROM homer_lake.main.hep_proto_1_call ORDER BY timestamp DESC LIMIT 200 (Ctrl+Enter to run)"
          className="min-h-0 flex-1 font-mono text-xs"
        />
      </div>
    </div>
  )
}
