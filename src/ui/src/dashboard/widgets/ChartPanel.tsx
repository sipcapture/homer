import { useCallback, useEffect, useRef, useState } from 'react'
import { useDashboard, useWidgetSearch } from '../context/DashboardContext'
import uPlot from 'uplot'
import 'uplot/dist/uPlot.min.css'
import { Settings2 } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'

type YMode = 'count' | 'sum' | 'avg' | 'last'

type ChartType = 'bars' | 'line' | 'area'

interface ChartConfig {
  bucketMs?: number
  xField?: string
  yMode?: YMode
  yField?: string
  chartType?: ChartType
  /** Optional explicit column name to split the chart into one line per distinct value. Empty → auto-detect. */
  seriesField?: string
}

type BuiltChartData =
  | { mode: 'single'; aligned: uPlot.AlignedData; label: string }
  | { mode: 'multi';  aligned: uPlot.AlignedData; seriesKeys: string[] }

interface ChartPanelProps {
  widgetId: string
  config?: ChartConfig
  onConfigChange?: (cfg: ChartConfig) => void
}

interface SearchPayload {
  filter?: Record<string, unknown>
  param?: { limit?: number }
  timestamp?: { from: number; to: number }
  useSqlEndpoint?: boolean
  sql?: string
  nl_query?: string
  nl_mode?: string
  nl_parser?: string
  /** When true (AI tab + nl_query), same path as Results → /mcp/query — not raw transactions/search */
  useMcpEndpoint?: boolean
}

const BUCKET_OPTIONS = [
  { label: '10 s', value: 10_000 },
  { label: '30 s', value: 30_000 },
  { label: '1 min', value: 60_000 },
  { label: '5 min', value: 300_000 },
  { label: '15 min', value: 900_000 },
  { label: '1 h', value: 3_600_000 },
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

function cellLooksNumeric(v: unknown): boolean {
  if (typeof v === 'number' && !isNaN(v)) return true
  if (typeof v === 'string' && v.trim() !== '' && !isNaN(parseFloat(v))) return true
  return false
}

function detectColumns(rows: Record<string, unknown>[]): string[] {
  if (!rows || rows.length === 0) return []
  return Object.keys(rows[0])
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
  if (!rows || rows.length === 0) return []
  return Object.keys(rows[0]).filter(k => {
    const v = rows[0][k]
    return typeof v === 'number' || (typeof v === 'string' && !isNaN(parseFloat(v)) && v.trim() !== '')
  })
}

/** Prefer OTLP / line-protocol style value columns for Time Chart default Y. */
function pickDefaultNumericY(candidates: string[]): string {
  if (!candidates.length) return ''
  const preferred = ['value_double', 'value_int', 'VALUE_DOUBLE', 'VALUE_INT', 'cnt', 'count', 'value', 'total']
  for (const p of preferred) {
    const k = findColumnCI(candidates, p)
    if (k) return k
  }
  return candidates[0]
}

/** When SQL already returns an aggregated column (cnt, count…), prefer `sum` over `count` of rows. */
function autoYMode(yField: string): YMode {
  return /^(cnt|count|total|sum|num|value|val)$/i.test(yField) ? 'sum' : 'count'
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

function hexToRgba(hex: string, alpha: number): string {
  const h = hex.replace('#', '')
  const len = h.length === 3 ? 1 : 2
  const r = parseInt(h.slice(0, len).padEnd(2, h[0]), 16)
  const g = parseInt(h.slice(len, len * 2).padEnd(2, h[len]), 16)
  const b = parseInt(h.slice(len * 2, len * 3).padEnd(2, h[len * 2]), 16)
  return `rgba(${r},${g},${b},${alpha})`
}

function buildSingleSeries(
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

/** One line per distinct `splitField` value (e.g. method). */
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
  const single = buildSingleSeries(rows, xField, yMode, yField, bucketMs)
  if (!single) return null
  const label = yMode === 'count' ? 'Count' : `${yMode}(${yField})`
  return { mode: 'single', aligned: single as unknown as uPlot.AlignedData, label }
}

export default function ChartPanel({ widgetId, config, onConfigChange }: ChartPanelProps) {
  const { apiBase, authHeader } = useDashboard()
  const searchData = useWidgetSearch<SearchPayload>(widgetId)
  const containerRef = useRef<HTMLDivElement | null>(null)
  const chartRef = useRef<uPlot | null>(null)
  const [rows, setRows] = useState<Record<string, unknown>[]>([])
  const [loading, setLoading] = useState(false)
  const [showSettings, setShowSettings] = useState(false)
  const [columns, setColumns] = useState<string[]>([])
  const [numericColumns, setNumericColumns] = useState<string[]>([])

  const xField = config?.xField || ''
  const explicitYMode = config?.yMode as YMode | undefined
  const yField = config?.yField || ''
  const bucketMs = config?.bucketMs || 60_000
  const seriesField = config?.seriesField || ''
  const chartType: ChartType = (config?.chartType as ChartType) || 'bars'

  const runQuery = useCallback(async (sd: SearchPayload) => {
    if (!sd) return
    setLoading(true)
    try {
      const useSql = Boolean(sd.useSqlEndpoint && sd.sql?.trim())
      const useMcp = Boolean(sd.useMcpEndpoint && sd.nl_query?.trim())
      const endpoint = useMcp
        ? `${apiBase}/mcp/query`
        : useSql
          ? `${apiBase}/query`
          : `${apiBase}/transactions/search`
      const body = useMcp
        ? JSON.stringify({
            query_text: sd.nl_query!.trim(),
            mode: sd.nl_mode || 'auto',
            parser: sd.nl_parser || 'auto',
            limit: sd.param?.limit ?? 100,
            timestamp: sd.timestamp || {},
            now_utc_unix_ms: Date.now(),
          })
        : useSql
          ? JSON.stringify({ sql: sd.sql!.trim(), limit: sd.param?.limit ?? 1000 })
          : JSON.stringify({ filter: sd.filter, param: sd.param, timestamp: sd.timestamp })
      const res = await fetch(endpoint, {
        method: 'POST',
        headers: { ...authHeader, 'Content-Type': 'application/json' },
        credentials: 'include',
        body,
      })
      if (!res.ok) return
      const data = await res.json()
      const items: Record<string, unknown>[] = data?.data?.items || []

      const cols = detectColumns(items)
      const numCols = detectNumericFields(items)
      setColumns(cols)
      setNumericColumns(numCols)
      setRows(items)
    } catch {
      // ignore
    } finally {
      setLoading(false)
    }
  }, [apiBase, authHeader])

  useEffect(() => {
    if (searchData) runQuery(searchData)
  }, [searchData, runQuery])

  useEffect(() => {
    if (!containerRef.current) return
    if (chartRef.current) {
      chartRef.current.destroy()
      chartRef.current = null
    }
    if (!rows.length) return

    const effectiveX = xField || detectTsField(columns)
    const effectiveY = yField || pickDefaultNumericY(numericColumns) || ''
    const effectiveMode: YMode = explicitYMode || autoYMode(effectiveY)

    const payload = buildChartPayload(rows, columns, effectiveX, effectiveMode, effectiveY, bucketMs, seriesField)
    if (!payload) return

    const el = containerRef.current
    const showLegend = true
    const LEGEND_RESERVE = 32
    const width = el.clientWidth || 400
    const rawHeight = el.clientHeight || 200
    const height = Math.max(80, rawHeight - (showLegend ? LEGEND_RESERVE : 0))

    const cs = getComputedStyle(el)
    const axisStroke = cs.getPropertyValue('--muted').trim() || 'rgba(204,220,231,0.4)'
    const gridStroke = cs.getPropertyValue('--border').trim() || 'rgba(255,255,255,0.06)'

    const barsPath = uPlot.paths?.bars?.({ size: [0.7, 100] })
    const pathFn = chartType === 'bars' ? barsPath : undefined

    const seriesUplot: uPlot.Series[] = [{}]
    if (payload.mode === 'single') {
      const stroke = SERIES_PALETTE[0]
      seriesUplot.push({
        label: payload.label,
        stroke,
        fill: chartType !== 'line' ? hexToRgba(stroke, 0.15) : undefined,
        width: 2,
        paths: pathFn,
        points: { show: false },
      })
    } else {
      for (let i = 0; i < payload.seriesKeys.length; i++) {
        const stroke = SERIES_PALETTE[i % SERIES_PALETTE.length]
        seriesUplot.push({
          label: payload.seriesKeys[i],
          stroke,
          fill: chartType !== 'line' ? hexToRgba(stroke, 0.12) : undefined,
          width: 2,
          paths: pathFn,
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
          font: '10px Inter, sans-serif',
          values: (_u, vals) => vals.map(v => {
            const d = new Date(v * 1000)
            return `${String(d.getHours()).padStart(2, '0')}:${String(d.getMinutes()).padStart(2, '0')}`
          }),
          labelFont: '10px Inter, sans-serif',
        },
        {
          stroke: axisStroke,
          grid: { stroke: gridStroke, width: 1 },
          ticks: { stroke: gridStroke, width: 1 },
          font: '10px Inter, sans-serif',
          labelFont: '10px Inter, sans-serif',
          size: 50,
        },
      ],
      series: seriesUplot,
    }

    chartRef.current = new uPlot(opts, payload.aligned, el)

    const ro = new ResizeObserver(() => {
      if (chartRef.current && el.clientWidth > 0 && el.clientHeight > 0) {
        const h = Math.max(80, el.clientHeight - (showLegend ? LEGEND_RESERVE : 0))
        chartRef.current.setSize({ width: el.clientWidth, height: h })
      }
    })
    ro.observe(el)

    return () => {
      ro.disconnect()
      if (chartRef.current) {
        chartRef.current.destroy()
        chartRef.current = null
      }
    }
  }, [rows, columns, numericColumns, xField, explicitYMode, yField, bucketMs, seriesField, chartType])

  const handleConfig = (partial: Partial<ChartConfig>) => {
    onConfigChange?.({ ...config, ...partial })
  }

  const effectiveX = xField || detectTsField(columns)
  const effectiveYField = yField || pickDefaultNumericY(numericColumns) || ''
  const yMode: YMode = explicitYMode || autoYMode(effectiveYField)

  return (
    <div className="relative flex h-full w-full flex-col">
      {/* Settings toolbar */}
      <div className="flex items-center gap-1 px-1 py-0.5 border-b border-border/50">
        <Button
          variant="ghost"
          size="icon"
          className={`h-5 w-5 shrink-0 ${showSettings ? 'text-primary' : 'text-muted-foreground'}`}
          onClick={() => setShowSettings(v => !v)}
          title="Chart settings"
        >
          <Settings2 className="size-3" />
        </Button>

        {showSettings && (
          <div className="flex flex-wrap items-center gap-1 text-[10px]">
            {/* X field */}
            <span className="text-muted-foreground shrink-0">X:</span>
            <Select value={effectiveX} onValueChange={v => handleConfig({ xField: v })}>
              <SelectTrigger className="h-5 min-w-[90px] max-w-[130px] text-[10px] px-1.5 py-0">
                <SelectValue placeholder="timestamp" />
              </SelectTrigger>
              <SelectContent>
                {columns.length === 0 && <SelectItem value="timestamp">timestamp</SelectItem>}
                {columns.map(c => <SelectItem key={c} value={c}>{c}</SelectItem>)}
              </SelectContent>
            </Select>

            {/* Y mode */}
            <span className="text-muted-foreground shrink-0">Y:</span>
            <Select value={yMode} onValueChange={v => handleConfig({ yMode: v as YMode })}>
              <SelectTrigger className="h-5 w-[70px] text-[10px] px-1.5 py-0">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="count">count</SelectItem>
                <SelectItem value="sum">sum</SelectItem>
                <SelectItem value="avg">avg</SelectItem>
                <SelectItem value="last">last</SelectItem>
              </SelectContent>
            </Select>

            {/* Y field (hidden when mode is count) */}
            {yMode !== 'count' && (
              <Select value={effectiveYField} onValueChange={v => handleConfig({ yField: v })}>
                <SelectTrigger className="h-5 min-w-[90px] max-w-[130px] text-[10px] px-1.5 py-0">
                  <SelectValue placeholder="field" />
                </SelectTrigger>
                <SelectContent>
                  {numericColumns.map(c => <SelectItem key={c} value={c}>{c}</SelectItem>)}
                  {numericColumns.length === 0 && columns.map(c => <SelectItem key={c} value={c}>{c}</SelectItem>)}
                </SelectContent>
              </Select>
            )}

            {/* Series split (one line per distinct value, e.g. method) */}
            <span className="text-muted-foreground shrink-0" title="Separate line per value (e.g. method). Auto picks first non-numeric column besides X/Y.">Series:</span>
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
                  return clo !== effectiveX.toLowerCase() && clo !== effectiveYField.toLowerCase()
                }).map(c => (
                  <SelectItem key={c} value={c}>{c}</SelectItem>
                ))}
              </SelectContent>
            </Select>

            {/* Chart type */}
            <span className="text-muted-foreground shrink-0">Type:</span>
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

            {/* Bucket size */}
            <span className="text-muted-foreground shrink-0">Bucket:</span>
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
          </div>
        )}
      </div>

      {loading && (
        <div className="absolute inset-0 z-10 flex items-center justify-center bg-card/60 text-xs text-muted-foreground backdrop-blur-sm">
          Loading...
        </div>
      )}
      {!rows.length && !loading && (
        <div className="absolute inset-0 flex items-center justify-center text-xs text-muted-foreground">
          Waiting for search data...
        </div>
      )}
      <div ref={containerRef} className="relative flex-1 min-h-0" />
    </div>
  )
}
