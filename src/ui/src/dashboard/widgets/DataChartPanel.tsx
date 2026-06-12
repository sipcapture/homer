import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useDashboard } from '../context/DashboardContext'
import { resolveTimeRange } from '../utils/resolveTimeRange'
import uPlot from 'uplot'
import 'uplot/dist/uPlot.min.css'
import { Play, RefreshCw, Settings2, ChevronDown, ChevronUp, Clock, BookOpen } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Textarea } from '@/components/ui/textarea'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'

/**
 * Injects the dashboard time range into a SQL template.
 *
 * Supported placeholders:
 *   {{from}}      → ISO-8601 string   e.g. '2026-04-15T10:00:00.000Z'
 *   {{to}}        → ISO-8601 string
 *   $__from       → Unix milliseconds (integer)
 *   $__to         → Unix milliseconds (integer)
 *   $__fromSec    → Unix seconds      (integer)
 *   $__toSec      → Unix seconds      (integer)
 *
 * Example (single series):
 *   SELECT time_bucket(INTERVAL '1 minute', timestamp) AS ts, COUNT(*) AS cnt ...
 *
 * Example (multiple lines — string column such as `method` is auto-detected as series split):
 *   SELECT ts, method, COUNT(*) AS cnt ... GROUP BY ts, method
 *   Use Y = sum or avg with field `cnt` (or Y = count to count rows per cell).
 */
function injectTimeRange(sql: string, fromMs: number, toMs: number): string {
  const fromISO = new Date(fromMs).toISOString()
  const toISO   = new Date(toMs).toISOString()
  return sql
    .replaceAll('{{from}}',    fromISO)
    .replaceAll('{{to}}',      toISO)
    .replaceAll('$__from',     String(fromMs))
    .replaceAll('$__to',       String(toMs))
    .replaceAll('$__fromSec',  String(Math.floor(fromMs / 1000)))
    .replaceAll('$__toSec',    String(Math.floor(toMs   / 1000)))
}

function fmtTime(ms: number): string {
  return new Date(ms).toLocaleString(undefined, { month: 'short', day: '2-digit', hour: '2-digit', minute: '2-digit' })
}

function hexToRgba(hex: string, alpha: number): string {
  const h = hex.replace('#', '')
  const len = h.length === 3 ? 1 : 2
  const r = parseInt(h.slice(0, len).padEnd(2, h[0]), 16)
  const g = parseInt(h.slice(len, len * 2).padEnd(2, h[len]), 16)
  const b = parseInt(h.slice(len * 2, len * 3).padEnd(2, h[len * 2]), 16)
  return `rgba(${r},${g},${b},${alpha})`
}

type YMode = 'count' | 'sum' | 'avg' | 'last'
type ChartType = 'bars' | 'line' | 'area'

interface DataChartConfig {
  sql?: string
  xField?: string
  yMode?: YMode
  yField?: string
  /** If set, one uPlot series per distinct value in this column (e.g. method). Empty = auto-detect. */
  seriesField?: string
  bucketMs?: number
  chartType?: ChartType
  seriesLabel?: string
  seriesColor?: string   // hex color, e.g. "#3274d9"
  xLabel?: string        // axis label for X
  yLabel?: string        // axis label for Y
  autoRefreshSec?: number
}

interface DataChartPanelProps {
  widgetId: string
  config?: DataChartConfig
  onConfigChange?: (cfg: DataChartConfig) => void
}

const BUCKET_OPTIONS = [
  { label: '10 s',  value: 10_000 },
  { label: '30 s',  value: 30_000 },
  { label: '1 min', value: 60_000 },
  { label: '5 min', value: 300_000 },
  { label: '15 min',value: 900_000 },
  { label: '1 h',   value: 3_600_000 },
]

const AUTO_REFRESH_OPTIONS = [
  { label: 'Off',    value: 0 },
  { label: '30 s',   value: 30 },
  { label: '1 min',  value: 60 },
  { label: '5 min',  value: 300 },
  { label: '15 min', value: 900 },
]

function parseTs(v: unknown): number | null {
  if (typeof v === 'number' && !isNaN(v)) return v
  if (typeof v === 'string') {
    const n = Date.parse(v)
    if (!isNaN(n)) return n
    const f = parseFloat(v)
    if (!isNaN(f)) return f
  }
  return null
}

/** Match API column names case-insensitively (e.g. TS vs ts, METHOD vs method). */
function findColumnCI(cols: string[], logical: string): string | undefined {
  const want = logical.toLowerCase()
  return cols.find(c => c.toLowerCase() === want)
}

function getCellCI(row: Record<string, unknown>, field: string): unknown {
  if (!field || !row) return undefined
  if (Object.prototype.hasOwnProperty.call(row, field)) return row[field]
  const want = field.toLowerCase()
  const k = Object.keys(row).find(c => c.toLowerCase() === want)
  return k != null ? row[k] : undefined
}

function detectTsField(cols: string[]): string {
  const preferred = [
    'timestamp', 'create_date', 'create_ts', 'micro_ts', 'time', 'ts', 'date',
    'time_bucket', 'bucket', 'bucket_start', 'window_start', 'window',
  ]
  for (const p of preferred) {
    const f = findColumnCI(cols, p)
    if (f) return f
  }
  return cols[0] || 'timestamp'
}

function detectNumericFields(rows: Record<string, unknown>[]): string[] {
  if (!rows.length) return []
  return Object.keys(rows[0]).filter(k => {
    const v = rows[0][k]
    return typeof v === 'number' || (typeof v === 'string' && v.trim() !== '' && !isNaN(parseFloat(v)))
  })
}

function cellLooksNumeric(v: unknown): boolean {
  if (typeof v === 'number' && !isNaN(v)) return true
  if (typeof v === 'string' && v.trim() !== '' && !isNaN(parseFloat(v))) return true
  return false
}

/** SIP / grouping columns: always treat as series dimension (values may be INVITE or 200). */
const SERIES_DIMENSION_NAMES = [
  'method', 'sip_method', 'sip_method_raw', 'verb', 'request_method', 'sip_verb',
  'response_code', 'sip_code', 'code', 'status', 'sip_status',
]

/** Column to split into multiple lines; excludes time X and value Y. */
function detectSeriesSplitField(
  columns: string[],
  xField: string,
  yField: string,
  rows: Record<string, unknown>[],
  explicit: string,
): string | null {
  const xLo = xField.toLowerCase()
  const yLo = yField.toLowerCase()
  const colNotXY = (c: string) => c.toLowerCase() !== xLo && c.toLowerCase() !== yLo

  if (explicit.trim()) {
    const ex = columns.find(c => c.toLowerCase() === explicit.trim().toLowerCase())
    if (ex && colNotXY(ex)) return ex
  }

  const candidates = columns.filter(colNotXY)
  if (!candidates.length || !rows.length) return null
  const sample = rows[0]

  for (const name of SERIES_DIMENSION_NAMES) {
    const k = findColumnCI(columns, name)
    if (k && candidates.some(c => c.toLowerCase() === k.toLowerCase())) return k
  }

  const softPreferred = ['service_name', 'name', 'label', 'series', 'group', 'caller', 'callee']
  for (const p of softPreferred) {
    const k = findColumnCI(columns, p)
    if (k && candidates.some(c => c.toLowerCase() === k.toLowerCase()) && !cellLooksNumeric(sample[k])) return k
  }

  for (const c of candidates) {
    if (!cellLooksNumeric(sample[c])) return c
  }

  for (const c of candidates) {
    const set = new Set<string>()
    let anyNonNum = false
    for (let i = 0; i < Math.min(rows.length, 48); i++) {
      const v = rows[i]?.[c]
      set.add(String(v ?? ''))
      if (!cellLooksNumeric(v)) anyNonNum = true
    }
    if (anyNonNum && set.size > 1) return c
  }

  return null
}

const SERIES_PALETTE = [
  '#3274d9', '#f97316', '#22c55e', '#a855f7', '#ec4899', '#14b8a6', '#eab308', '#ef4444',
  '#6366f1', '#84cc16', '#06b6d4', '#d946ef', '#f43f5e',
]

type BuiltChartData =
  | { mode: 'single'; aligned: uPlot.AlignedData; labels: string[] }
  | { mode: 'multi'; aligned: uPlot.AlignedData; seriesKeys: string[] }

function buildSeries(
  rows: Record<string, unknown>[],
  xField: string,
  yMode: YMode,
  yField: string,
  bucketMs: number,
): [number[], number[]] | null {
  if (!rows.length) return null
  const buckets = new Map<number, number[]>()
  for (const row of rows) {
    let ts = parseTs(getCellCI(row, xField))
    if (ts === null) continue
    if (ts < 1e12) ts *= 1000
    const key = Math.floor(ts / bucketMs) * bucketMs
    const val = yMode !== 'count' ? parseFloat(String(getCellCI(row, yField) ?? '')) : 1
    const arr = buckets.get(key) ?? []
    arr.push(isNaN(val) ? 1 : val)
    buckets.set(key, arr)
  }
  if (!buckets.size) return null
  const sorted = [...buckets.entries()].sort((a, b) => a[0] - b[0])
  const yVals = sorted.map(([, vals]) => {
    switch (yMode) {
      case 'sum':  return vals.reduce((a, b) => a + b, 0)
      case 'avg':  return vals.reduce((a, b) => a + b, 0) / vals.length
      case 'last': return vals[vals.length - 1]
      default:     return vals.length
    }
  })
  return [sorted.map(([t]) => t / 1000), yVals]
}

/**
 * One line per distinct `splitField` (e.g. method). Buckets X by `bucketMs` like single-series,
 * but never merges different series keys in the same bucket.
 */
function buildMultiSeries(
  rows: Record<string, unknown>[],
  xField: string,
  yMode: YMode,
  yField: string,
  bucketMs: number,
  splitField: string,
): BuiltChartData | null {
  if (!rows.length) return null
  type Cell = { vals: number[] }
  const grid = new Map<string, Map<number, Cell>>()
  for (const row of rows) {
    let ts = parseTs(getCellCI(row, xField))
    if (ts === null) continue
    if (ts < 1e12) ts *= 1000
    const bucket = Math.floor(ts / bucketMs) * bucketMs
    const sk = String(getCellCI(row, splitField) ?? '').trim() || '(empty)'
    let val = yMode !== 'count' ? parseFloat(String(getCellCI(row, yField) ?? '')) : 1
    if (isNaN(val)) val = yMode === 'count' ? 1 : 0
    let m = grid.get(sk)
    if (!m) {
      m = new Map()
      grid.set(sk, m)
    }
    let cell = m.get(bucket)
    if (!cell) {
      cell = { vals: [] }
      m.set(bucket, cell)
    }
    cell.vals.push(val)
  }
  if (!grid.size) return null

  const reduceVals = (vals: number[]) => {
    switch (yMode) {
      case 'sum':  return vals.reduce((a, b) => a + b, 0)
      case 'avg':  return vals.reduce((a, b) => a + b, 0) / vals.length
      case 'last': return vals[vals.length - 1]
      default:     return vals.length
    }
  }

  const bucketSet = new Set<number>()
  for (const m of grid.values()) {
    for (const b of m.keys()) bucketSet.add(b)
  }
  const bucketMsSorted = [...bucketSet].sort((a, b) => a - b)
  const xsSec = bucketMsSorted.map(t => t / 1000)
  if (!xsSec.length) return null

  const seriesKeys = [...grid.keys()].sort((a, b) => a.localeCompare(b))
  const aligned: number[][] = [xsSec]
  for (const sk of seriesKeys) {
    const m = grid.get(sk)!
    const yArr = bucketMsSorted.map((bucketMsKey) => {
      const cell = m.get(bucketMsKey)
      if (!cell || !cell.vals.length) return NaN
      return reduceVals(cell.vals)
    })
    aligned.push(yArr)
  }

  return { mode: 'multi', aligned: aligned as uPlot.AlignedData, seriesKeys }
}

function buildChartPayload(
  rows: Record<string, unknown>[],
  columns: string[],
  xField: string,
  yMode: YMode,
  yField: string,
  bucketMs: number,
  seriesFieldCfg: string,
): BuiltChartData | null {
  if (!rows.length) return null
  const split = detectSeriesSplitField(columns, xField, yField, rows, seriesFieldCfg)
  const yReady = yMode === 'count' || (yField && yField.length > 0)
  if (split && yReady) {
    const multi = buildMultiSeries(rows, xField, yMode, yField, bucketMs, split)
    if (multi) return multi
  }
  const single = buildSeries(rows, xField, yMode, yField, bucketMs)
  if (!single) return null
  const label = yMode === 'count' ? 'Count' : `${yMode}(${yField})`
  return { mode: 'single', aligned: single as unknown as uPlot.AlignedData, labels: [label] }
}

const SQL_EXAMPLES = [
  {
    group: 'SIP',
    label: 'Calls per minute',
    sql: `SELECT
  time_bucket(INTERVAL '1 minute', timestamp) AS ts,
  COUNT(*) AS calls
FROM hep_proto_1_call
WHERE timestamp BETWEEN '{{from}}' AND '{{to}}'
GROUP BY 1
ORDER BY 1`,
  },
  {
    group: 'SIP',
    label: 'Calls by method',
    sql: `SELECT
  time_bucket(INTERVAL '1 minute', timestamp) AS ts,
  method,
  COUNT(*) AS cnt
FROM hep_proto_1_call
WHERE timestamp BETWEEN '{{from}}' AND '{{to}}'
GROUP BY 1, 2
ORDER BY 1`,
  },
  {
    group: 'SIP',
    label: 'Response codes distribution',
    sql: `SELECT
  CAST(response_code AS VARCHAR) AS code,
  COUNT(*) AS cnt
FROM hep_proto_1_call
WHERE timestamp BETWEEN '{{from}}' AND '{{to}}'
  AND response_code > 0
GROUP BY 1
ORDER BY cnt DESC`,
  },
  {
    group: 'SIP',
    label: 'Top 10 callers',
    sql: `SELECT
  caller,
  COUNT(*) AS calls
FROM hep_proto_1_call
WHERE timestamp BETWEEN '{{from}}' AND '{{to}}'
GROUP BY caller
ORDER BY calls DESC
LIMIT 10`,
  },
  {
    group: 'RTCP',
    label: 'Packet loss over time',
    sql: `SELECT
  time_bucket(INTERVAL '1 minute', timestamp) AS ts,
  AVG(CAST(json_extract_string(payload, '$.report_blocks[0].fraction_lost') AS DOUBLE)) AS avg_loss
FROM hep_proto_5_default
WHERE timestamp BETWEEN '{{from}}' AND '{{to}}'
GROUP BY 1
ORDER BY 1`,
  },
  {
    group: 'LOG',
    label: 'Log events per minute',
    sql: `SELECT
  time_bucket(INTERVAL '1 minute', timestamp) AS ts,
  COUNT(*) AS events
FROM hep_proto_100_default
WHERE timestamp BETWEEN '{{from}}' AND '{{to}}'
GROUP BY 1
ORDER BY 1`,
  },
]

export default function DataChartPanel({ widgetId, config, onConfigChange }: DataChartPanelProps) {
  const { apiBase, authHeader, timeRange, timeZone, subscribeToTimeRange } = useDashboard()

  const sql          = config?.sql ?? ''
  const xField       = config?.xField ?? ''
  const yMode: YMode = (config?.yMode as YMode) ?? 'count'
  const yField       = config?.yField ?? ''
  const bucketMs     = config?.bucketMs ?? 60_000
  const chartType: ChartType = (config?.chartType as ChartType) ?? 'bars'
  const seriesField  = config?.seriesField ?? ''
  const seriesLabel  = config?.seriesLabel ?? ''
  const seriesColor  = config?.seriesColor ?? '#3274d9'
  const xLabel       = config?.xLabel ?? ''
  const yLabel       = config?.yLabel ?? ''
  const autoRefreshSec = config?.autoRefreshSec ?? 0

  const [sqlDraft, setSqlDraft] = useState(sql)
  const [showEditor, setShowEditor] = useState(!sql)
  const [showSettings, setShowSettings] = useState(false)
  const [rows, setRows] = useState<Record<string, unknown>[]>([])
  const [columns, setColumns] = useState<string[]>([])
  const [numericCols, setNumericCols] = useState<string[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')

  const containerRef = useRef<HTMLDivElement | null>(null)
  const chartRef = useRef<uPlot | null>(null)
  const autoRefreshTimer = useRef<ReturnType<typeof setInterval> | null>(null)

  const handleConfig = (partial: Partial<DataChartConfig>) => {
    onConfigChange?.({ ...config, ...partial })
  }

  // Resolve effective time range (quick preset = rolling from now; else absolute or last hour)
  const getEffectiveRange = useCallback(() => {
    const resolved = resolveTimeRange(timeRange, timeZone)
    if (resolved) return resolved
    const now = Date.now()
    return { from: now - 3_600_000, to: now }
  }, [timeRange, timeZone])

  const resolvedDisplayRange = useMemo(() => resolveTimeRange(timeRange, timeZone), [timeRange, timeZone])

  const runQuery = useCallback(async (querySql: string) => {
    const q = querySql.trim()
    if (!q) return
    setLoading(true)
    setError('')
    try {
      const { from, to } = getEffectiveRange()
      const finalSql = injectTimeRange(q, from, to)
      const res = await fetch(`${apiBase}/query`, {
        method: 'POST',
        headers: { ...authHeader, 'Content-Type': 'application/json' },
        credentials: 'include',
        body: JSON.stringify({ sql: finalSql, limit: 50000 }),
      })
      if (!res.ok) {
        const msg = await res.text()
        throw new Error(msg || `HTTP ${res.status}`)
      }
      const data = await res.json()
      const items: Record<string, unknown>[] = data?.data?.items ?? []
      setRows(items)
      const cols = items.length ? Object.keys(items[0]) : []
      const numCols = detectNumericFields(items)
      setColumns(cols)
      setNumericCols(numCols)
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : String(e))
    } finally {
      setLoading(false)
    }
  }, [apiBase, authHeader, getEffectiveRange])

  // Run on mount if SQL already saved
  useEffect(() => {
    if (sql) runQuery(sql)
  }, []) // eslint-disable-line react-hooks/exhaustive-deps

  // Re-run when global time range changes
  useEffect(() => {
    if (!sql) return
    const unsub = subscribeToTimeRange(widgetId, () => runQuery(sql))
    return unsub
  }, [widgetId, sql, subscribeToTimeRange, runQuery])

  // Auto-refresh
  useEffect(() => {
    if (autoRefreshTimer.current) clearInterval(autoRefreshTimer.current)
    if (autoRefreshSec > 0 && sql) {
      autoRefreshTimer.current = setInterval(() => runQuery(sql), autoRefreshSec * 1000)
    }
    return () => { if (autoRefreshTimer.current) clearInterval(autoRefreshTimer.current) }
  }, [autoRefreshSec, sql, runQuery])

  // Render chart whenever rows or viz config changes
  useEffect(() => {
    if (!containerRef.current) return
    if (chartRef.current) { chartRef.current.destroy(); chartRef.current = null }
    if (!rows.length) return

    const effectiveX = xField || detectTsField(columns)
    const effectiveY = yField || numericCols[0] || ''

    const payload = buildChartPayload(rows, columns, effectiveX, yMode, effectiveY, bucketMs, seriesField)
    if (!payload) return

    const el = containerRef.current
    const showLegend =
      (payload.mode === 'multi' && payload.seriesKeys.length > 0) || !!seriesLabel
    // uPlot puts <div class="u-legend"> inside `el` *below* the canvas. If we let the canvas
    // take the full container height the legend overflows and gets clipped by the parent's
    // overflow-hidden. Reserve a strip at the bottom for it.
    const legendRowH = 22
    const legendRows = payload.mode === 'multi'
      ? Math.max(1, Math.ceil(payload.seriesKeys.length / 6))
      : 1
    const LEGEND_RESERVE = showLegend ? legendRowH * legendRows + 8 : 0
    const width  = el.clientWidth  || 400
    const rawH   = el.clientHeight || 200
    const height = Math.max(80, rawH - LEGEND_RESERVE)

    const cs = getComputedStyle(el)
    const axisStroke  = cs.getPropertyValue('--muted-foreground').trim()  || 'rgba(150,150,150,0.7)'
    const gridStroke  = cs.getPropertyValue('--border').trim()            || 'rgba(255,255,255,0.08)'

    const pathFn = chartType === 'bars' ? uPlot.paths?.bars?.({ size: [0.7, 100] }) : undefined

    const seriesUplot: uPlot.Series[] = [{}]
    if (payload.mode === 'single') {
      const serieLabel = seriesLabel || (yMode === 'count' ? 'Count' : `${yMode}(${effectiveY})`)
      const serieFill = hexToRgba(seriesColor, 0.18)
      seriesUplot.push({
        label: serieLabel,
        stroke: seriesColor,
        fill: chartType !== 'line' ? serieFill : undefined,
        width: chartType === 'bars' ? 2 : 2,
        paths: chartType === 'bars' ? pathFn : undefined,
        points: { show: false },
      })
    } else {
      for (let i = 0; i < payload.seriesKeys.length; i++) {
        const key = payload.seriesKeys[i]
        const stroke = SERIES_PALETTE[i % SERIES_PALETTE.length]
        seriesUplot.push({
          label: key,
          stroke,
          fill: chartType !== 'line' ? hexToRgba(stroke, 0.12) : undefined,
          width: chartType === 'bars' ? 2 : 2,
          paths: chartType === 'bars' ? pathFn : undefined,
          points: { show: false },
        })
      }
    }

    const opts: uPlot.Options = {
      width,
      height,
      cursor: { show: true, drag: { x: false, y: false } },
      select: { show: false, left: 0, top: 0, width: 0, height: 0 },
      legend: { show: showLegend, live: false },
      axes: [
        {
          stroke: axisStroke,
          grid: { stroke: gridStroke, width: 1 },
          ticks: { stroke: gridStroke, width: 1 },
          font: '10px Inter,sans-serif',
          label: xLabel || undefined,
          labelFont: '11px Inter,sans-serif',
          values: (_u, vals) => vals.map(v => {
            const d = new Date(v * 1000)
            return `${String(d.getHours()).padStart(2, '0')}:${String(d.getMinutes()).padStart(2, '0')}`
          }),
        },
        {
          stroke: axisStroke,
          grid: { stroke: gridStroke, width: 1 },
          ticks: { stroke: gridStroke, width: 1 },
          font: '10px Inter,sans-serif',
          label: yLabel || undefined,
          labelFont: '11px Inter,sans-serif',
          size: 50,
        },
      ],
      series: seriesUplot,
    }

    chartRef.current = new uPlot(opts, payload.aligned, el)

    const ro = new ResizeObserver(() => {
      if (chartRef.current && el.clientWidth > 0 && el.clientHeight > 0) {
        const h = Math.max(80, el.clientHeight - LEGEND_RESERVE)
        chartRef.current.setSize({ width: el.clientWidth, height: h })
      }
    })
    ro.observe(el)
    return () => { ro.disconnect(); chartRef.current?.destroy(); chartRef.current = null }
  }, [rows, xField, yMode, yField, bucketMs, chartType, seriesLabel, seriesColor, seriesField, xLabel, yLabel, columns, numericCols])

  const handleRun = () => {
    const q = sqlDraft.trim()
    if (!q) return
    handleConfig({ sql: q })
    runQuery(q)
  }

  const effectiveX     = xField || detectTsField(columns)
  const effectiveYFld  = yField || numericCols[0] || ''

  return (
    <div className="flex h-full flex-col overflow-hidden text-[11px]">

      {/* SQL editor bar */}
      <div className="flex shrink-0 items-center gap-1 border-b border-border/50 bg-muted/20 px-1.5 py-1">
        <Button
          variant="ghost" size="icon"
          className="h-5 w-5 shrink-0 text-muted-foreground"
          onClick={() => setShowEditor(v => !v)}
          title={showEditor ? 'Hide SQL editor' : 'Show SQL editor'}
        >
          {showEditor ? <ChevronUp className="size-3" /> : <ChevronDown className="size-3" />}
        </Button>
        <span className="flex-1 truncate font-mono text-[10px] text-muted-foreground">
          {!showEditor && sql ? sql.slice(0, 80) + (sql.length > 80 ? '…' : '') : 'SQL Query'}
        </span>
        {/* Time range indicator */}
        {resolvedDisplayRange && (
          <span className="flex shrink-0 items-center gap-0.5 rounded bg-muted px-1 text-[9px] text-muted-foreground">
            <Clock className="size-2.5" />
            {fmtTime(resolvedDisplayRange.from)} – {fmtTime(resolvedDisplayRange.to)}
          </span>
        )}
        <Button
          variant="ghost" size="icon"
          className={`h-5 w-5 shrink-0 ${showSettings ? 'text-primary' : 'text-muted-foreground'}`}
          onClick={() => setShowSettings(v => !v)}
          title="Chart settings"
        >
          <Settings2 className="size-3" />
        </Button>
      </div>

      {/* Expanded SQL editor */}
      {showEditor && (
        <div className="shrink-0 border-b border-border/50 bg-muted/10 p-1.5 space-y-1">
          <Textarea
            value={sqlDraft}
            onChange={e => setSqlDraft(e.target.value)}
            placeholder={`SELECT time_bucket(INTERVAL '1 minute', timestamp) AS ts, COUNT(*) AS cnt\nFROM main.hep_proto_1_call\nWHERE timestamp BETWEEN '{{from}}' AND '{{to}}'\nGROUP BY 1 ORDER BY 1\n\n-- Time placeholders: {{from}} {{to}}  →  ISO-8601\n--                   $__fromSec $__toSec  →  Unix seconds`}
            className="h-28 resize-none font-mono text-[11px] leading-relaxed"
            onKeyDown={e => {
              if ((e.ctrlKey || e.metaKey) && e.key === 'Enter') { e.preventDefault(); handleRun() }
            }}
          />
          <div className="flex items-center gap-1.5 flex-wrap">
            <Button size="sm" className="h-6 gap-1 px-2 text-[11px]" onClick={handleRun} disabled={!sqlDraft.trim() || loading}>
              <Play className="size-3" />
              {loading ? 'Running…' : 'Run'}
            </Button>
            {sql && !loading && (
              <Button variant="outline" size="sm" className="h-6 gap-1 px-2 text-[11px]" onClick={() => runQuery(sql)}>
                <RefreshCw className="size-3" />
                Refresh
              </Button>
            )}
            {/* Examples dropdown */}
            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <Button variant="outline" size="sm" className="h-6 gap-1 px-2 text-[11px]">
                  <BookOpen className="size-3" />
                  Examples
                </Button>
              </DropdownMenuTrigger>
              <DropdownMenuContent align="start" className="w-56 max-h-72 overflow-y-auto">
                {['SIP', 'RTCP', 'LOG'].map(group => {
                  const items = SQL_EXAMPLES.filter(e => e.group === group)
                  return (
                    <div key={group}>
                      <DropdownMenuLabel className="text-[10px] uppercase tracking-wider text-muted-foreground">
                        {group}
                      </DropdownMenuLabel>
                      {items.map(ex => (
                        <DropdownMenuItem
                          key={ex.label}
                          className="text-[11px] cursor-pointer"
                          onClick={() => setSqlDraft(ex.sql)}
                        >
                          {ex.label}
                        </DropdownMenuItem>
                      ))}
                      <DropdownMenuSeparator />
                    </div>
                  )
                })}
              </DropdownMenuContent>
            </DropdownMenu>

            {rows.length > 0 && (
              <span className="text-muted-foreground">{rows.length} rows</span>
            )}
            {error && <span className="truncate text-destructive">{error}</span>}
          </div>
        </div>
      )}

      {/* Settings bar */}
      {showSettings && (
        <div className="flex shrink-0 flex-wrap items-center gap-x-2 gap-y-1 border-b border-border/50 bg-muted/10 px-1.5 py-1">
          {/* X field */}
          <label className="text-muted-foreground">X:</label>
          <Select value={effectiveX} onValueChange={v => handleConfig({ xField: v })}>
            <SelectTrigger className="h-5 min-w-[90px] max-w-[140px] text-[10px] px-1.5 py-0">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {(columns.length ? columns : ['timestamp']).map(c => (
                <SelectItem key={c} value={c}>{c}</SelectItem>
              ))}
            </SelectContent>
          </Select>

          {/* Y mode */}
          <label className="text-muted-foreground">Y:</label>
          <Select value={yMode} onValueChange={v => handleConfig({ yMode: v as YMode })}>
            <SelectTrigger className="h-5 w-[70px] text-[10px] px-1.5 py-0">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {(['count','sum','avg','last'] as YMode[]).map(m => (
                <SelectItem key={m} value={m}>{m}</SelectItem>
              ))}
            </SelectContent>
          </Select>

          {yMode !== 'count' && (
            <Select value={effectiveYFld} onValueChange={v => handleConfig({ yField: v })}>
              <SelectTrigger className="h-5 min-w-[90px] max-w-[140px] text-[10px] px-1.5 py-0">
                <SelectValue placeholder="field" />
              </SelectTrigger>
              <SelectContent>
                {(numericCols.length ? numericCols : columns).map(c => (
                  <SelectItem key={c} value={c}>{c}</SelectItem>
                ))}
              </SelectContent>
            </Select>
          )}

          <label className="text-muted-foreground" title="Separate line per value (e.g. method). Auto picks first non-numeric column besides X/Y.">
            Series:
          </label>
          <Select
            value={
              seriesField.trim() && columns.some(c => c.toLowerCase() === seriesField.trim().toLowerCase())
                ? (findColumnCI(columns, seriesField.trim()) ?? '__auto__')
                : '__auto__'
            }
            onValueChange={v => handleConfig({ seriesField: v === '__auto__' ? '' : v })}
          >
            <SelectTrigger className="h-5 min-w-[72px] max-w-[120px] text-[10px] px-1.5 py-0">
              <SelectValue placeholder="auto" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="__auto__">Auto</SelectItem>
              {columns.filter(c => {
                const clo = c.toLowerCase()
                return clo !== effectiveX.toLowerCase() && clo !== effectiveYFld.toLowerCase()
              }).map(c => (
                <SelectItem key={c} value={c}>{c}</SelectItem>
              ))}
            </SelectContent>
          </Select>

          {/* Chart type */}
          <label className="text-muted-foreground">Type:</label>
          <Select value={chartType} onValueChange={v => handleConfig({ chartType: v as ChartType })}>
            <SelectTrigger className="h-5 w-[68px] text-[10px] px-1.5 py-0">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="bars">Bars</SelectItem>
              <SelectItem value="line">Line</SelectItem>
              <SelectItem value="area">Area</SelectItem>
            </SelectContent>
          </Select>

          {/* Bucket */}
          <label className="text-muted-foreground">Bucket:</label>
          <Select value={String(bucketMs)} onValueChange={v => handleConfig({ bucketMs: Number(v) })}>
            <SelectTrigger className="h-5 w-[62px] text-[10px] px-1.5 py-0">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {BUCKET_OPTIONS.map(o => (
                <SelectItem key={o.value} value={String(o.value)}>{o.label}</SelectItem>
              ))}
            </SelectContent>
          </Select>

          {/* Auto-refresh */}
          <label className="text-muted-foreground">Auto:</label>
          <Select value={String(autoRefreshSec)} onValueChange={v => handleConfig({ autoRefreshSec: Number(v) })}>
            <SelectTrigger className="h-5 w-[65px] text-[10px] px-1.5 py-0">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {AUTO_REFRESH_OPTIONS.map(o => (
                <SelectItem key={o.value} value={String(o.value)}>{o.label}</SelectItem>
              ))}
            </SelectContent>
          </Select>

          {/* Series color */}
          <label className="text-muted-foreground">Color:</label>
          <input
            type="color"
            value={seriesColor}
            onChange={e => handleConfig({ seriesColor: e.target.value })}
            className="h-5 w-7 cursor-pointer rounded border border-border bg-transparent p-0.5"
            title="Series color"
          />

          {/* Series label */}
          <label className="text-muted-foreground">Legend:</label>
          <input
            className="h-5 w-[90px] rounded border border-border bg-background px-1.5 text-[10px] outline-none focus:ring-1 focus:ring-ring"
            value={seriesLabel}
            placeholder="auto"
            onChange={e => handleConfig({ seriesLabel: e.target.value })}
          />

          {/* X axis label */}
          <label className="text-muted-foreground">X label:</label>
          <input
            className="h-5 w-[80px] rounded border border-border bg-background px-1.5 text-[10px] outline-none focus:ring-1 focus:ring-ring"
            value={xLabel}
            placeholder="e.g. Time"
            onChange={e => handleConfig({ xLabel: e.target.value })}
          />

          {/* Y axis label */}
          <label className="text-muted-foreground">Y label:</label>
          <input
            className="h-5 w-[80px] rounded border border-border bg-background px-1.5 text-[10px] outline-none focus:ring-1 focus:ring-ring"
            value={yLabel}
            placeholder="e.g. Count"
            onChange={e => handleConfig({ yLabel: e.target.value })}
          />
        </div>
      )}

      {/* Chart area */}
      <div className="relative min-h-0 flex-1">
        {loading && (
          <div className="absolute inset-0 z-10 flex items-center justify-center bg-card/60 text-xs text-muted-foreground backdrop-blur-sm">
            Running query…
          </div>
        )}
        {!loading && !rows.length && !error && (
          <div className="absolute inset-0 flex flex-col items-center justify-center gap-1 text-xs text-muted-foreground">
            <Play className="size-6 opacity-30" />
            <span>{sql ? 'No data returned' : 'Write a SQL query and press Run'}</span>
            {!showEditor && (
              <button className="text-primary underline" onClick={() => setShowEditor(true)}>
                Open editor
              </button>
            )}
          </div>
        )}
        {!loading && error && (
          <div className="absolute inset-0 flex items-start p-3 text-xs text-destructive">
            <pre className="whitespace-pre-wrap break-all">{error}</pre>
          </div>
        )}
        <div ref={containerRef} className="h-full w-full" />
      </div>
    </div>
  )
}
