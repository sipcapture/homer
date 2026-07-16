// @ts-nocheck - TODO: rewrite with shadcn/ui + TanStack; typed when refactored
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import 'uplot/dist/uPlot.min.css'
import {
  buildMetricChartPayload,
  dominantMetricKind,
  mountOtlpMetricSeriesChart,
  normalizeOtlpMetricChartType,
  rowTimestampMs,
} from '../otlpMetricsSeriesChart'
import { ArrowDown, ArrowUp, ArrowUpDown, Download, Palette, SlidersHorizontal } from 'lucide-react'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { ScrollArea } from '@/components/ui/scroll-area'
import { newModalKey } from '@/lib/modalKey'
import { cn } from '@/lib/utils'
import { toast } from 'sonner'
import { displayDstIp, displaySrcIp } from '@/lib/ipAliasDisplay'
import { useDashboard, useWidgetSearch } from '../context/DashboardContext'
import { resolveTimeRange, timeRangeLogicalKey } from '../utils/resolveTimeRange'
import { handleUnauthorized } from '../../api'
import TransactionModal from '../TransactionModal'
import OTLPTraceModal from '../OTLPTraceModal'
import OTLPLogsTraceModal from '../OTLPLogsTraceModal'
import OTLPLogRowModal from '../OTLPLogRowModal'
import OTLPMetricsSeriesModal from '../OTLPMetricsSeriesModal'
import MessageModal from '../MessageModal'
import { useLocale } from '@/components/locale/locale-provider'

const OTLP_TRACES_PROTO = 200
const OTLP_METRICS_PROTO = 201
const OTLP_LOGS_PROTO = 202

/** IP-alias enrichment for call flow / tooltips — not shown as separate result columns (alias text stays in src_ip/dst_ip). */
function isHiddenIpAliasDetailColumn(name) {
  const n = String(name)
  if (/^aliasSrc_(image|tag[1-4])$/i.test(n)) return true
  if (/^aliasDst_(image|tag[1-4])$/i.test(n)) return true
  if (/^alias_src_(image|tag[1-4])$/i.test(n)) return true
  if (/^alias_dst_(image|tag[1-4])$/i.test(n)) return true
  return false
}

function isOtlpProtoType(pt) {
  const n = Number(pt) || 0
  return n === OTLP_TRACES_PROTO || n === OTLP_METRICS_PROTO || n === OTLP_LOGS_PROTO
}

const priorityCols = [
  'timestamp', 'method', 'caller', 'callee', 'src_ip', 'dst_ip',
  'src_port', 'dst_port', 'session_id', 'cid', 'uuid',
]

function strHue(s) {
  if (!s) return 0
  let h = 5381
  for (let i = 0; i < s.length; i++) h = ((h << 5) + h) ^ s.charCodeAt(i)
  return Math.abs(h) % 360
}

function callColor(callId, dark) {
  if (!callId) return undefined
  const hue = strHue(String(callId))
  return dark
    ? `hsla(${hue}, 70%, 28%, 0.62)`
    : `hsla(${hue}, 72%, 72%, 0.58)`
}

function useIsDark() {
  const [dark, setDark] = useState(() =>
    typeof document !== 'undefined' && document.documentElement.classList.contains('dark'),
  )
  useEffect(() => {
    if (typeof document === 'undefined') return
    const obs = new MutationObserver(() => {
      setDark(document.documentElement.classList.contains('dark'))
    })
    obs.observe(document.documentElement, { attributes: true, attributeFilter: ['class'] })
    return () => obs.disconnect()
  }, [])
  return dark
}

const LS_KEY_HIDDEN = (id, proto, event) => `results_hidden_cols_${id}_p${proto}_e${event}`
const LS_KEY_ORDER  = (id, proto, event) => `results_col_order_${id}_p${proto}_e${event}`
const LS_KEY_OTLP_METRICS_TAB = (id) => `results_otlp_metrics_tab_${id}`
const LS_KEY_OTLP_METRICS_CHART = (id) => `results_otlp_metrics_chart_type_${id}`

function loadLS(key, fallback) {
  try { const v = localStorage.getItem(key); return v != null ? JSON.parse(v) : fallback }
  catch { return fallback }
}
function saveLS(key, value) {
  try { localStorage.setItem(key, JSON.stringify(value)) } catch { /* ignore */ }
}

/** Lake / API message id (POST /messages); DuckDB may expose column as UUID. */
function pickRowUuid(row) {
  if (!row || typeof row !== 'object') return ''
  const v = row.uuid ?? row.UUID
  if (v != null && String(v).trim() !== '') return String(v).trim()
  return ''
}

function pickSessionForTx(row, protoType) {
  const pt = Number(protoType) || 1
  if (!row || typeof row !== 'object') return ''
  if (pt === OTLP_TRACES_PROTO || pt === OTLP_LOGS_PROTO) {
    const t = row.trace_id ?? row.TRACE_ID
    if (t != null && String(t).trim() !== '') return String(t).trim()
    return ''
  }
  if (pt === OTLP_METRICS_PROTO) {
    const n = row.name ?? row.NAME
    if (n != null && String(n).trim() !== '') return String(n).trim()
    return ''
  }
  const v = row.session_id ?? row.cid ?? row.SESSION_ID ?? row.CID
  if (v != null && String(v).trim() !== '') return String(v).trim()
  return ''
}

function timestampToMs(val) {
  if (val == null && val !== 0) return null
  if (val instanceof Date) return val.getTime()
  if (typeof val === 'number') {
    if (val > 1e15) return Math.round(val / 1e6)
    if (val > 1e12) return Math.round(val)
    if (val > 1e9) return Math.round(val * 1000)
    return val
  }
  if (typeof val === 'string') {
    let s = val.trim()
    if (/^\d{4}-\d{2}-\d{2}$/.test(s)) s = `${s}T00:00:00Z`
    else if (s.includes(' ') && !s.includes('T')) s = s.replace(' ', 'T')
    const d = new Date(s)
    return Number.isNaN(d.getTime()) ? null : d.getTime()
  }
  return null
}

/** Row event time for /messages (never use dashboard time picker here). */
function pickRowTimestampMs(row) {
  if (!row || typeof row !== 'object') return null
  for (const k of ['timestamp', 'TIMESTAMP', 'ts', 'TS', 'create_date', 'CREATE_DATE']) {
    if (!Object.prototype.hasOwnProperty.call(row, k)) continue
    const ms = timestampToMs(row[k])
    if (ms != null) return ms
  }
  return null
}

/** Stable key for row selection: uuid when present, else index in filteredRows. */
function rowSelectionKey(row, indexInFiltered) {
  const u = pickRowUuid(row)
  if (u !== '') return u
  return `i:${indexInFiltered}`
}

/** OTLP metrics preview — same uPlot payload as OTLPMetricsSeriesModal (per-point, dual axis for summary/histogram). */
function OtlpMetricsInlineChart({ rows, widgetId }) {
  const containerRef = useRef(null)
  const disposeRef = useRef(null)

  const [chartType, setChartType] = useState(() =>
    normalizeOtlpMetricChartType(loadLS(LS_KEY_OTLP_METRICS_CHART(widgetId), 'area')),
  )

  useEffect(() => {
    setChartType(normalizeOtlpMetricChartType(loadLS(LS_KEY_OTLP_METRICS_CHART(widgetId), 'area')))
  }, [widgetId])

  const sorted = useMemo(() => {
    const list = Array.isArray(rows) ? [...rows] : []
    list.sort((a, b) => rowTimestampMs(a) - rowTimestampMs(b))
    return list
  }, [rows])

  const kind = useMemo(() => dominantMetricKind(sorted), [sorted])
  const chartPayload = useMemo(() => buildMetricChartPayload(sorted, kind), [sorted, kind])

  useEffect(() => {
    if (disposeRef.current) {
      disposeRef.current()
      disposeRef.current = null
    }
    if (!chartPayload || !containerRef.current) return
    const el = containerRef.current
    disposeRef.current = mountOtlpMetricSeriesChart(el, chartPayload, {
      chartType,
    })
    return () => {
      if (disposeRef.current) {
        disposeRef.current()
        disposeRef.current = null
      }
    }
  }, [chartPayload, chartType])

  if (!rows?.length) {
    return <div className="p-3 text-xs text-muted-foreground">No rows — run a search.</div>
  }
  if (!chartPayload) {
    return (
      <div className="p-3 text-xs text-muted-foreground">
        No plottable numeric column (e.g. value_double / value_int). Use the Table tab for raw rows.
      </div>
    )
  }
  return (
    <div className="flex min-h-0 flex-col gap-2">
      <div className="flex flex-wrap items-center justify-end gap-2">
        <Select
          value={chartType}
          onValueChange={(v) => {
            const next = normalizeOtlpMetricChartType(v)
            setChartType(next)
            saveLS(LS_KEY_OTLP_METRICS_CHART(widgetId), next)
          }}
        >
          <SelectTrigger className="h-7 w-[132px] text-xs" aria-label="Chart type">
            <SelectValue placeholder="Chart type" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="line">Line</SelectItem>
            <SelectItem value="area">Area</SelectItem>
            <SelectItem value="histogram">Histogram</SelectItem>
          </SelectContent>
        </Select>
      </div>
      <div
        ref={containerRef}
        className="h-[min(40vh,320px)] min-h-[180px] w-full min-w-0"
      />
    </div>
  )
}

// ParserBadge renders the meta.parser block returned by /api/v4/mcp/query.
// It is kept inline (vs. a shared component) so the hot path doesn't pay for
// an import that's only useful when the AI tab is in use.
function ParserBadge({ meta }) {
  const used = String(meta?.used || '').toLowerCase()
  const palette = used === 'llm'
    ? 'border-emerald-500/40 bg-emerald-500/10 text-emerald-300'
    : used === 'regex_fallback'
    ? 'border-amber-500/40 bg-amber-500/10 text-amber-300'
    : 'border-zinc-500/40 bg-zinc-500/10 text-zinc-300'
  const label = used === 'llm'
    ? 'LLM'
    : used === 'regex_fallback'
    ? 'Regex (LLM fallback)'
    : 'Regex'
  const tooltipParts = []
  if (meta?.requested) tooltipParts.push(`requested=${meta.requested}`)
  if (meta?.model) tooltipParts.push(`model=${meta.model}`)
  if (meta?.latency_ms) tooltipParts.push(`${meta.latency_ms}ms`)
  if (meta?.error) tooltipParts.push(`error=${meta.error}`)
  return (
    <span
      title={tooltipParts.join(' · ') || undefined}
      className={`inline-flex items-center gap-1 rounded-sm border px-1.5 py-0.5 font-mono text-[10px] ${palette}`}
    >
      <span>parser: {label}</span>
      {meta?.model && <span className="opacity-70">· {meta.model}</span>}
      {meta?.latency_ms ? <span className="opacity-70">· {meta.latency_ms}ms</span> : null}
      {used === 'regex_fallback' && meta?.error && (
        <span className="opacity-70 truncate max-w-[180px]">· {meta.error}</span>
      )}
    </span>
  )
}

export default function ResultsPanel({ widgetId, config: _config }) {
  const { apiBase, authHeader, timeRange, timeZone, requestTimeRange, subscribeToTimeRange } = useDashboard()
  const { resolved: locale } = useLocale()
  const searchData = useWidgetSearch(widgetId)
  const lastSearchDataRef = useRef(null)
  const lastTimerangeBroadcastKeyRef = useRef(null)
  const timeRangeRef = useRef(timeRange)
  timeRangeRef.current = timeRange
  const dark = useIsDark()
  const [rows, setRows] = useState([])
  const [keys, setKeys] = useState([])
  const [loading, setLoading] = useState(false)
  const [status, setStatus] = useState('')
  const [statusType, setStatusType] = useState('info')
  const [generatedSql, setGeneratedSql] = useState('')
  // parserMeta surfaces /api/v4/mcp/query meta.parser so the user can tell
  // whether the LLM bridge actually answered the query, fell back to regex,
  // or never ran at all.
  const [parserMeta, setParserMeta] = useState(null)
  const [scrollTop, setScrollTop] = useState(0)
  const [viewportHeight, setViewportHeight] = useState(300)
  const containerRef = useRef(null)
  const [messageModals, setMessageModals] = useState([])
  const [transactionModals, setTransactionModals] = useState([])
  const [otlpTraceModals, setOtplTraceModals] = useState([])
  const [otlpLogModals, setOtplLogModals] = useState([])
  const [otlpLogRowModals, setOtplLogRowModals] = useState([])
  const [otlpMetricModals, setOtplMetricModals] = useState([])
  const [filterText, setFilterText] = useState('')
  const [currentProto, setCurrentProto] = useState('1')
  const [currentEvent, setCurrentEvent] = useState('call')
  const [hiddenColumns, setHiddenColumns] = useState(() => loadLS(LS_KEY_HIDDEN(widgetId, '1', 'call'), []))
  const [columnOrder, setColumnOrder] = useState(() => loadLS(LS_KEY_ORDER(widgetId, '1', 'call'), []))
  const [sortCol, setSortCol] = useState(null)
  const [sortDir, setSortDir] = useState('asc')
  const [colorByCall, setColorByCall] = useState(() => loadLS(`results_color_by_call_${widgetId}`, true))
  const [otlpMetricsTab, setOtlpMetricsTab] = useState(() => {
    const t = loadLS(LS_KEY_OTLP_METRICS_TAB(widgetId), 'chart')
    return t === 'table' ? 'table' : 'chart'
  })
  const [columnsModalOpen, setColumnsModalOpen] = useState(false)
  const [selectedKeys, setSelectedKeys] = useState(() => new Set())
  const rowHeight = 32

  useEffect(() => {
    const node = containerRef.current
    if (!node) return
    const update = () => setViewportHeight(node.clientHeight || 300)
    update()
    const ro = new ResizeObserver(update)
    ro.observe(node)
    return () => ro.disconnect()
  }, [rows.length])

  const runSearch = useCallback(async (sd) => {
    if (!sd) return
    setSelectedKeys(new Set())
    lastSearchDataRef.current = sd
    const proto = String(sd.filter?.proto_type ?? sd.filter?.proto ?? '1')
    const event = String(sd.filter?.event_type ?? sd.filter?.event ?? 'all')
    setCurrentProto(proto)
    setCurrentEvent(event)
    setHiddenColumns(loadLS(LS_KEY_HIDDEN(widgetId, proto, event), []))
    setColumnOrder(loadLS(LS_KEY_ORDER(widgetId, proto, event), []))
    setSortCol(null)
    setSortDir('asc')
    setLoading(true)
    setStatus('')
    setGeneratedSql('')
    setParserMeta(null)
    const t0 = performance.now()
    try {
      const useSql = sd.useSqlEndpoint && sd.sql?.trim()
      const useMcp = sd.useMcpEndpoint && sd.nl_query?.trim()
      const endpoint = useMcp
        ? `${apiBase}/mcp/query`
        : (useSql ? `${apiBase}/query` : `${apiBase}/transactions/search`)
      const body = useMcp
        ? JSON.stringify({
          query_text: sd.nl_query.trim(),
          mode: sd.nl_mode || 'auto',
          parser: sd.nl_parser || 'auto',
          limit: sd.param?.limit || 100,
          timestamp: sd.timestamp || {},
          now_utc_unix_ms: Date.now(),
        })
        : useSql
        ? JSON.stringify({ sql: sd.sql.trim(), limit: sd.param?.limit || 1000 })
        : JSON.stringify({ filter: sd.filter, param: sd.param, timestamp: sd.timestamp })
      const res = await fetch(endpoint, {
        method: 'POST',
        headers: { ...authHeader, 'Content-Type': 'application/json' },
        credentials: 'include',
        body,
      })
      const elapsed = (performance.now() - t0).toFixed(1)
      if (res.status === 401) {
        handleUnauthorized()
        return
      }
      if (!res.ok) {
        const err = await res.json().catch(() => ({}))
        setStatus(err?.error?.detail || `Error ${res.status}`)
        setStatusType('error')
        setRows([]); setKeys([])
        return
      }
      const data = await res.json()
      const items = data?.data?.items || []
      setRows(items)
      setKeys(data?.data?.keys || [])
      setStatus(`${items.length} rows in ${elapsed}ms`)
      setStatusType('success')
      if (useMcp && (sd.nl_mode === 'sql' || sd.nl_mode === 'auto')) {
        const sqlText = data?.meta?.message
        if (typeof sqlText === 'string' && sqlText.trim()) {
          setGeneratedSql(sqlText.trim())
        }
      }
      if (useMcp) {
        const pm = data?.meta?.parser
        if (pm && typeof pm === 'object') setParserMeta(pm)
        const tr = data?.meta?.time_range
        if (tr?.from != null && tr?.to != null && requestTimeRange) {
          const rawFrom = Number(tr.from)
          const rawTo = Number(tr.to)
          if (Number.isFinite(rawFrom) && Number.isFinite(rawTo) && rawTo > rawFrom) {
            const fromMs = Math.floor(rawFrom / 1000) * 1000
            const toMs = Math.ceil(rawTo / 1000) * 1000
            const nextKey = timeRangeLogicalKey({
              from: fromMs,
              to: toMs,
              activePreset: null,
              calendarPreset: null,
            })
            const curKey = timeRangeLogicalKey(timeRangeRef.current)
            if (nextKey !== curKey) {
              lastTimerangeBroadcastKeyRef.current = nextKey
              requestTimeRange(null, fromMs, toMs)
            }
          }
        }
      }
    } catch (err) {
      setStatus(err.message)
      setStatusType('error')
      setRows([]); setKeys([])
      setGeneratedSql('')
    } finally {
      setLoading(false)
    }
  }, [apiBase, authHeader, widgetId, requestTimeRange, timeZone])

  useEffect(() => {
    if (searchData) runSearch(searchData)
  }, [searchData, runSearch])

  useEffect(() => {
    if (!widgetId) return
    const unsub = subscribeToTimeRange(widgetId, (newRange) => {
      const last = lastSearchDataRef.current
      if (!last) return
      const key = timeRangeLogicalKey(newRange)
      if (lastTimerangeBroadcastKeyRef.current !== null && key === lastTimerangeBroadcastKeyRef.current) return
      lastTimerangeBroadcastKeyRef.current = key
      const ts = resolveTimeRange(newRange, timeZone)
      if (!ts) return
      runSearch({ ...last, timestamp: ts })
    })
    return unsub
  }, [widgetId, subscribeToTimeRange, runSearch, timeZone])

  const allCols = useMemo(() => {
    const baseCols = keys?.length ? keys : (rows[0] ? Object.keys(rows[0]) : [])
    const filtered = baseCols.filter(
      (c) => c !== 'payload' && c !== 'data_extra' && !isHiddenIpAliasDetailColumn(c),
    )
    if (!filtered.length) return columnOrder.length ? columnOrder : []
    const saved = columnOrder.filter((c) => filtered.includes(c))
    const newCols = filtered.filter((c) => !saved.includes(c))
    const newOrdered = [...priorityCols.filter((c) => newCols.includes(c)), ...newCols.filter((c) => !priorityCols.includes(c))]
    return [...saved, ...newOrdered]
  }, [keys, rows, columnOrder])

  useEffect(() => {
    if (!allCols.length) return
    setColumnOrder((prev) => {
      const same = prev.length === allCols.length && allCols.every((c, i) => prev[i] === c)
      if (same) return prev
      saveLS(LS_KEY_ORDER(widgetId, currentProto, currentEvent), allCols)
      return allCols
    })
  }, [allCols, widgetId, currentProto, currentEvent])

  const columns = useMemo(() => {
    return [...allCols, '_actions'].filter((c) => !hiddenColumns.includes(c))
  }, [allCols, hiddenColumns])

  const tableColumns = useMemo(() => ['_select', ...columns], [columns])

  const handleSort = (col) => {
    if (col === '_actions' || col === '_select') return
    setSortCol((prev) => {
      if (prev === col) {
        setSortDir((d) => d === 'asc' ? 'desc' : 'asc')
        return col
      }
      setSortDir('asc')
      return col
    })
    setScrollTop(0)
    if (containerRef.current) containerRef.current.scrollTop = 0
  }

  const filteredRows = useMemo(() => {
    let result = rows || []
    if (filterText.trim()) {
      const q = filterText.trim().toLowerCase()
      result = result.filter((row) =>
        Object.values(row).some((v) => String(v ?? '').toLowerCase().includes(q)),
      )
    }
    if (!sortCol) return result
    return [...result].sort((a, b) => {
      let va = a[sortCol] ?? ''
      let vb = b[sortCol] ?? ''
      const na = Number(va), nb = Number(vb)
      if (!isNaN(na) && !isNaN(nb) && va !== '' && vb !== '') {
        return sortDir === 'asc' ? na - nb : nb - na
      }
      va = String(va).toLowerCase()
      vb = String(vb).toLowerCase()
      if (va < vb) return sortDir === 'asc' ? -1 : 1
      if (va > vb) return sortDir === 'asc' ? 1 : -1
      return 0
    })
  }, [rows, filterText, sortCol, sortDir])

  const totalRows = filteredRows?.length || 0
  const visibleCount = Math.ceil(viewportHeight / rowHeight) + 6
  const startIndex = Math.max(0, Math.floor(scrollTop / rowHeight) - 3)
  const endIndex = Math.min(totalRows, startIndex + visibleCount)
  const topSpacer = startIndex * rowHeight
  const bottomSpacer = Math.max(0, (totalRows - endIndex) * rowHeight)
  const visibleRows = useMemo(() => (filteredRows || []).slice(startIndex, endIndex), [filteredRows, startIndex, endIndex])

  const searchProtoNum = Number(currentProto) || 1

  const selectedCountOnFiltered = useMemo(() => {
    let n = 0
    filteredRows.forEach((row, idx) => {
      if (selectedKeys.has(rowSelectionKey(row, idx))) n += 1
    })
    return n
  }, [filteredRows, selectedKeys])

  const allFilteredSelected =
    filteredRows.length > 0 && selectedCountOnFiltered === filteredRows.length
  const someFilteredSelected =
    selectedCountOnFiltered > 0 && selectedCountOnFiltered < filteredRows.length

  const toggleRowSelected = (row, indexInFiltered, checked) => {
    const k = rowSelectionKey(row, indexInFiltered)
    setSelectedKeys((prev) => {
      const next = new Set(prev)
      if (checked) next.add(k)
      else next.delete(k)
      return next
    })
  }

  const toggleSelectAllFiltered = (checked) => {
    if (!checked) {
      setSelectedKeys(new Set())
      return
    }
    const next = new Set()
    filteredRows.forEach((row, idx) => next.add(rowSelectionKey(row, idx)))
    setSelectedKeys(next)
  }

  const exportCsv = () => {
    if (!filteredRows.length) return
    const cols = columns.filter((c) => c !== '_actions')
    const lines = [
      cols.join(','),
      ...filteredRows.map((row) =>
        cols
          .map((col) => {
            let v = row[col]
            if (col === 'src_ip') v = displaySrcIp(row)
            else if (col === 'dst_ip') v = displayDstIp(row)
            const s = v == null ? '' : String(v).replaceAll('"', '""')
            return `"${s}"`
          })
          .join(','),
      ),
    ]
    const blob = new Blob([lines.join('\n')], { type: 'text/csv;charset=utf-8;' })
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = `results-${Date.now()}.csv`
    a.click()
    URL.revokeObjectURL(url)
  }

  const formatTs = (val) => {
    if (!val) return val
    try {
      const d = val instanceof Date ? val : new Date(val)
      if (isNaN(d)) return val
      const opts = { year: 'numeric', month: 'numeric', day: 'numeric', hour: 'numeric', minute: 'numeric', second: 'numeric', fractionalSecondDigits: 3 }
      if (timeZone && timeZone !== 'local') opts.timeZone = timeZone
      return new Intl.DateTimeFormat(locale, opts).format(d)
    } catch { return val }
  }

  const genModalKey = () => newModalKey()

  function cloneRowSnapshot(row) {
    try {
      if (typeof structuredClone === 'function') return structuredClone(row)
    } catch {
      /* fall through */
    }
    try {
      return JSON.parse(JSON.stringify(row))
    } catch {
      return row && typeof row === 'object' ? { ...row } : {}
    }
  }

  const openMessage = async (row) => {
    const lastSearchEarly = lastSearchDataRef.current
    const protoTypeEarly = Number(lastSearchEarly?.filter?.proto_type ?? lastSearchEarly?.filter?.proto ?? 1) || 1
    if (protoTypeEarly === OTLP_LOGS_PROTO) {
      const modalKey = genModalKey()
      setOtplLogRowModals((prev) => [...prev, { modalKey, row: cloneRowSnapshot(row) }])
      return
    }

    const uuid = pickRowUuid(row)
    if (!uuid) {
      const protoType = Number(lastSearchEarly?.filter?.proto_type ?? lastSearchEarly?.filter?.proto ?? 1) || 1
      if (protoType === OTLP_TRACES_PROTO) {
        toast.error('OTLP trace rows have no message uuid — use Trace to open the full trace.', { duration: 7000 })
        return
      }
      if (protoType === OTLP_METRICS_PROTO) {
        toast.error('OTLP metric rows have no message uuid — use Series to open all points for this metric name.', { duration: 7000 })
        return
      }
      const sid = pickSessionForTx(row, protoType)
      if (sid) {
        toast.info('No message uuid in this row — opening transaction. Add uuid to your SELECT to open the raw SIP message.', {
          duration: 6000,
        })
        void openTransaction(sid, row)
        return
      }
      toast.error(
        'Cannot open SIP message: add uuid to your SELECT. For time-scoped load use the row time: SELECT uuid, method, timestamp FROM homer_lake.main.hep_proto_1_call …',
        { duration: 9000 },
      )
      return
    }
    const modalKey = genModalKey()
    const lastSearch = lastSearchDataRef.current
    const protoType = lastSearch?.filter?.proto_type ?? 1
    const eventType = lastSearch?.filter?.event_type ?? 'all'
    const rowMs = pickRowTimestampMs(row)
    const WIN_MS = 300 * 1000
    const messageContext = { proto_type: protoType, event_type: eventType }
    // Only send timestamp when it comes from the result row. Dashboard time range
    // often does not match SQL rows and causes /messages 404 (partition filter).
    if (rowMs != null) {
      messageContext.timestamp = { from: rowMs - WIN_MS, to: rowMs + WIN_MS }
    }
    const initial = { modalKey, uuid, loading: true, data: null, error: '', messageContext }
    setMessageModals(prev => [...prev, initial])
    try {
      const body = { uuid, proto_type: protoType, event_type: eventType }
      if (messageContext.timestamp) body.timestamp = messageContext.timestamp
      const res = await fetch(`${apiBase}/messages`, {
        method: 'POST',
        headers: { ...authHeader, 'Content-Type': 'application/json' },
        credentials: 'include',
        body: JSON.stringify(body),
      })
      const data = await res.json().catch(() => ({}))
      if (res.status === 401) {
        handleUnauthorized()
        return
      }
      if (!res.ok) {
        const detail = typeof data?.detail === 'string' ? data.detail : null
        throw new Error(detail ? `${res.status}: ${detail}` : `Error ${res.status}`)
      }
      const result = { modalKey, uuid, loading: false, data: data?.data || {}, error: '', messageContext }
      setMessageModals(prev => prev.map(m => m.modalKey === modalKey ? result : m))
    } catch (err) {
      const result = { modalKey, uuid, loading: false, data: null, error: err.message, messageContext }
      setMessageModals(prev => prev.map(m => m.modalKey === modalKey ? result : m))
    }
  }

  const openTransaction = async (value, row) => {
    const modalKey = genModalKey()
    const lastSearch = lastSearchDataRef.current
    const protoType = Number(lastSearch?.filter?.proto_type ?? lastSearch?.filter?.proto ?? 1) || 1
    const eventType = String(lastSearch?.filter?.event_type ?? lastSearch?.filter?.event ?? 'all')
    const WIN_MS = 300 * 1000

    let sessionIds = []
    let titleId = ''
    let timeRangeTx = timeRange

    if (selectedKeys.size > 0) {
      const idSet = new Set()
      const tsMs = []
      filteredRows.forEach((r, idx) => {
        if (!selectedKeys.has(rowSelectionKey(r, idx))) return
        const sid = pickSessionForTx(r, protoType)
        if (sid) idSet.add(sid)
        const m = pickRowTimestampMs(r)
        if (m != null) tsMs.push(m)
      })
      sessionIds = [...idSet]
      if (sessionIds.length === 0) {
        toast.error(
          protoType === OTLP_TRACES_PROTO || protoType === OTLP_LOGS_PROTO
            ? 'Cannot open: selected rows have no trace_id.'
            : protoType === OTLP_METRICS_PROTO
              ? 'Cannot open: selected rows have no metric name.'
              : 'Cannot open transaction: selected rows have no session_id / cid.',
          { duration: 5000 },
        )
        return
      }
      if (
        (protoType === OTLP_TRACES_PROTO || protoType === OTLP_LOGS_PROTO || protoType === OTLP_METRICS_PROTO) &&
        sessionIds.length > 1
      ) {
        toast.error(
          protoType === OTLP_METRICS_PROTO
            ? 'Select rows from a single metric (multiple name values).'
            : 'Select rows from a single trace (multiple trace_id values).',
          { duration: 6000 },
        )
        return
      }
      titleId = sessionIds.length === 1 ? sessionIds[0] : `${sessionIds.length} sessions`
      if (tsMs.length > 0) {
        timeRangeTx = { from: Math.min(...tsMs) - WIN_MS, to: Math.max(...tsMs) + WIN_MS }
      } else if (timeRange?.from != null && timeRange?.to != null) {
        timeRangeTx = timeRange
      }
    } else {
      const tid = String(pickSessionForTx(row, protoType) || value || '').trim()
      if (!tid) {
        toast.error(
          protoType === OTLP_TRACES_PROTO || protoType === OTLP_LOGS_PROTO
            ? 'Cannot open: no trace_id in this row.'
            : protoType === OTLP_METRICS_PROTO
              ? 'Cannot open: no metric name in this row.'
              : 'Cannot open transaction: no session_id / cid in this row.',
          { duration: 6000 },
        )
        return
      }
      sessionIds = [tid]
      titleId = tid
      const rowMs = pickRowTimestampMs(row)
      timeRangeTx = rowMs != null
        ? { from: rowMs - WIN_MS, to: rowMs + WIN_MS }
        : timeRange
    }

    const initialCommon = {
      modalKey,
      loading: true,
      items: [],
      error: '',
      timeRange: timeRangeTx,
    }

    if (protoType === OTLP_TRACES_PROTO) {
      const initial = {
        ...initialCommon,
        traceId: sessionIds[0] || titleId,
      }
      setOtplTraceModals((prev) => [...prev, initial])
      try {
        const body = { proto_type: protoType, event_type: eventType, session_id: sessionIds[0] }
        if (timeRangeTx) {
          const resolvedTx = resolveTimeRange(timeRangeTx, timeZone)
          if (resolvedTx) body.timestamp = { from: resolvedTx.from, to: resolvedTx.to }
        }
        const res = await fetch(`${apiBase}/transactions/messages`, {
          method: 'POST',
          headers: { ...authHeader, 'Content-Type': 'application/json' },
        credentials: 'include',
          body: JSON.stringify(body),
        })
        if (res.status === 401) {
          handleUnauthorized()
          return
        }
        if (!res.ok) throw new Error(`Error ${res.status}`)
        const data = await res.json()
        const result = {
          ...initial,
          loading: false,
          items: data?.data?.items || [],
          error: '',
        }
        setOtplTraceModals((prev) => prev.map((m) => (m.modalKey === modalKey ? result : m)))
      } catch (err) {
        const result = { ...initial, loading: false, items: [], error: err.message }
        setOtplTraceModals((prev) => prev.map((m) => (m.modalKey === modalKey ? result : m)))
      }
      return
    }

    if (protoType === OTLP_LOGS_PROTO) {
      const initial = {
        ...initialCommon,
        traceId: sessionIds[0] || titleId,
      }
      setOtplLogModals((prev) => [...prev, initial])
      try {
        const tid = sessionIds[0] || titleId
        const body = {
          session_id: tid,
          trace_id: tid,
        }
        if (timeRangeTx) {
          const resolvedTx = resolveTimeRange(timeRangeTx, timeZone)
          if (resolvedTx) body.timestamp = { from: resolvedTx.from, to: resolvedTx.to }
        }
        const res = await fetch(`${apiBase}/transactions/otlp-logs-trace`, {
          method: 'POST',
          headers: { ...authHeader, 'Content-Type': 'application/json' },
        credentials: 'include',
          body: JSON.stringify(body),
        })
        if (res.status === 401) {
          handleUnauthorized()
          return
        }
        if (!res.ok) throw new Error(`Error ${res.status}`)
        const data = await res.json()
        const result = {
          ...initial,
          loading: false,
          items: data?.data?.items || [],
          error: '',
        }
        setOtplLogModals((prev) => prev.map((m) => (m.modalKey === modalKey ? result : m)))
      } catch (err) {
        const result = { ...initial, loading: false, items: [], error: err.message }
        setOtplLogModals((prev) => prev.map((m) => (m.modalKey === modalKey ? result : m)))
      }
      return
    }

    if (protoType === OTLP_METRICS_PROTO) {
      const initial = {
        ...initialCommon,
        metricName: sessionIds[0] || titleId,
      }
      setOtplMetricModals((prev) => [...prev, initial])
      try {
        const body = { proto_type: protoType, event_type: eventType, session_id: sessionIds[0] }
        if (timeRangeTx) {
          const resolvedTx = resolveTimeRange(timeRangeTx, timeZone)
          if (resolvedTx) body.timestamp = { from: resolvedTx.from, to: resolvedTx.to }
        }
        const res = await fetch(`${apiBase}/transactions/messages`, {
          method: 'POST',
          headers: { ...authHeader, 'Content-Type': 'application/json' },
        credentials: 'include',
          body: JSON.stringify(body),
        })
        if (res.status === 401) {
          handleUnauthorized()
          return
        }
        if (!res.ok) throw new Error(`Error ${res.status}`)
        const data = await res.json()
        const result = {
          ...initial,
          loading: false,
          items: data?.data?.items || [],
          error: '',
        }
        setOtplMetricModals((prev) => prev.map((m) => (m.modalKey === modalKey ? result : m)))
      } catch (err) {
        const result = { ...initial, loading: false, items: [], error: err.message }
        setOtplMetricModals((prev) => prev.map((m) => (m.modalKey === modalKey ? result : m)))
      }
      return
    }

    if (isOtlpProtoType(protoType)) {
      return
    }

    const primarySessionId = sessionIds[0]
    const initial = {
      ...initialCommon,
      id: titleId,
      primarySessionId,
      sessionIds: [...sessionIds],
      protoType,
      eventType,
    }
    setTransactionModals((prev) => [...prev, initial])
    try {
      const body = { proto_type: protoType, event_type: eventType }
      if (sessionIds.length > 1) {
        body.session_ids = sessionIds
      } else {
        body.session_id = sessionIds[0]
      }
      if (timeRangeTx) {
        const resolvedTx = resolveTimeRange(timeRangeTx, timeZone)
        if (resolvedTx) body.timestamp = { from: resolvedTx.from, to: resolvedTx.to }
      }
      const res = await fetch(`${apiBase}/transactions/messages`, {
        method: 'POST',
        headers: { ...authHeader, 'Content-Type': 'application/json' },
        credentials: 'include',
        body: JSON.stringify(body),
      })
      if (res.status === 401) {
        handleUnauthorized()
        return
      }
      if (!res.ok) throw new Error(`Error ${res.status}`)
      const data = await res.json()
      const result = {
        ...initial,
        loading: false,
        items: data?.data?.items || [],
        error: '',
      }
      setTransactionModals((prev) => prev.map((m) => (m.modalKey === modalKey ? result : m)))
    } catch (err) {
      const result = { ...initial, loading: false, items: [], error: err.message }
      setTransactionModals((prev) => prev.map((m) => (m.modalKey === modalKey ? result : m)))
    }
  }

  return (
    <div className="flex h-full min-h-0 flex-col">
      <div className="flex items-center gap-2 border-b border-border px-2 py-1.5">
        <Input
          value={filterText}
          onChange={(e) => setFilterText(e.target.value)}
          placeholder="Filter rows..."
          className="h-7 min-w-0 flex-1 text-xs"
        />
        {selectedKeys.size > 0 && (
          <Button
            type="button"
            size="sm"
            variant="ghost"
            className="h-7 shrink-0 text-xs"
            onClick={() => setSelectedKeys(new Set())}
          >
            Clear ({selectedKeys.size})
          </Button>
        )}
        <Button
          type="button"
          size="sm"
          variant="outline"
          onClick={exportCsv}
          disabled={!filteredRows.length}
        >
          <Download className="mr-1 h-3.5 w-3.5" />
          CSV
        </Button>
        <Button
          type="button"
          size="icon-sm"
          variant={colorByCall ? 'secondary' : 'outline'}
          title="Color rows by Call-ID"
          onClick={() => {
            const next = !colorByCall
            setColorByCall(next)
            saveLS(`results_color_by_call_${widgetId}`, next)
          }}
        >
          <Palette className="h-3.5 w-3.5" />
        </Button>
        <Button
          type="button"
          size="sm"
          variant="outline"
          onClick={() => setColumnsModalOpen(true)}
        >
          <SlidersHorizontal className="mr-1 h-3.5 w-3.5" />
          Columns
        </Button>
      </div>

      {status && (
        <div className="px-2 pt-2">
          <Alert
            variant={statusType === 'error' ? 'destructive' : 'default'}
            className="py-1.5"
          >
            <AlertDescription className="text-xs">
              <div className="flex flex-wrap items-center gap-x-3 gap-y-1">
                <span>{status}</span>
                {parserMeta && (
                  <ParserBadge meta={parserMeta} />
                )}
              </div>
            </AlertDescription>
          </Alert>
        </div>
      )}
      {generatedSql && (
        <div className="px-2 pt-2">
          <Alert className="py-1.5">
            <AlertDescription className="whitespace-pre-wrap font-mono text-[11px]">
              {generatedSql}
            </AlertDescription>
          </Alert>
        </div>
      )}

      {!loading && rows.length === 0 && !status && (
        <div className="flex flex-1 items-center justify-center text-xs text-muted-foreground">
          Waiting for search...
        </div>
      )}
      {(loading || rows.length > 0) && (
        <div
          className={cn(
            'relative flex min-h-0 flex-1',
            searchProtoNum === OTLP_METRICS_PROTO && 'flex-col',
          )}
        >
          {loading && (
            <div className="absolute inset-0 z-20 flex flex-col items-center justify-center gap-4 bg-background/85 text-xs text-muted-foreground backdrop-blur-sm">
              <div className="relative h-20 w-20">
                <div className="absolute inset-0 animate-ping rounded-full bg-primary/10" />
                <div className="absolute inset-2 animate-pulse rounded-full bg-primary/5" />
                <img
                  src="/logo.png"
                  alt="Homer"
                  className="relative h-full w-full animate-[spin_3s_linear_infinite] object-contain drop-shadow"
                />
              </div>
              <div className="flex items-center gap-1 font-medium tracking-wide">
                <span>Searching transactions</span>
                <span className="inline-flex gap-0.5">
                  <span className="animate-bounce [animation-delay:-0.3s]">.</span>
                  <span className="animate-bounce [animation-delay:-0.15s]">.</span>
                  <span className="animate-bounce">.</span>
                </span>
              </div>
            </div>
          )}
          {searchProtoNum === OTLP_METRICS_PROTO && (
            <div className="flex shrink-0 items-center gap-1 border-b border-border bg-muted/25 px-2 py-1">
              <Button
                type="button"
                size="sm"
                variant={otlpMetricsTab === 'chart' ? 'secondary' : 'ghost'}
                className="h-7 text-xs"
                onClick={() => {
                  setOtlpMetricsTab('chart')
                  saveLS(LS_KEY_OTLP_METRICS_TAB(widgetId), 'chart')
                }}
              >
                Chart
              </Button>
              <Button
                type="button"
                size="sm"
                variant={otlpMetricsTab === 'table' ? 'secondary' : 'ghost'}
                className="h-7 text-xs"
                onClick={() => {
                  setOtlpMetricsTab('table')
                  saveLS(LS_KEY_OTLP_METRICS_TAB(widgetId), 'table')
                }}
              >
                Table
              </Button>
            </div>
          )}
          {searchProtoNum === OTLP_METRICS_PROTO && otlpMetricsTab === 'chart' && (
            <div className="shrink-0 border-b border-border px-2 py-2">
              <OtlpMetricsInlineChart rows={filteredRows} widgetId={widgetId} />
            </div>
          )}
          <div
            ref={containerRef}
            onScroll={(e) => setScrollTop(e.currentTarget.scrollTop)}
            className={cn(
              'flex-1 overflow-auto',
              searchProtoNum === OTLP_METRICS_PROTO && otlpMetricsTab === 'chart' && 'hidden',
            )}
          >
          <table className="w-full border-separate border-spacing-0 text-[11px] text-foreground">
            <thead className="sticky top-0 z-10 bg-card">
              <tr>
                {tableColumns.map((col) => (
                  <th
                    key={col}
                    onClick={() => handleSort(col)}
                    className={cn(
                      'whitespace-nowrap border-b border-border px-2 py-1.5 text-left font-medium text-slate-600 dark:text-slate-300',
                      col !== '_actions' && col !== '_select' && 'cursor-pointer select-none hover:text-slate-900 dark:hover:text-sky-100',
                    )}
                    title={col !== '_actions' && col !== '_select' ? `Sort by ${col}` : undefined}
                  >
                    {col === '_select' ? (
                      <span className="inline-flex items-center" onClick={(e) => e.stopPropagation()}>
                        <Checkbox
                          checked={
                            allFilteredSelected
                              ? true
                              : someFilteredSelected
                                ? 'indeterminate'
                                : false
                          }
                          onCheckedChange={(c) => toggleSelectAllFiltered(c === true)}
                          aria-label="Select all filtered rows"
                        />
                      </span>
                    ) : col === '_actions' ? (
                      <span className="text-[10px] uppercase tracking-wider">Actions</span>
                    ) : (
                      <span className="inline-flex items-center gap-1">
                        {col}
                        {sortCol === col ? (
                          sortDir === 'asc' ? (
                            <ArrowUp className="h-3 w-3" />
                          ) : (
                            <ArrowDown className="h-3 w-3" />
                          )
                        ) : (
                          <ArrowUpDown className="h-3 w-3 opacity-45 dark:opacity-50" />
                        )}
                      </span>
                    )}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {topSpacer > 0 && (
                <tr>
                  <td colSpan={tableColumns.length} style={{ height: topSpacer, padding: 0, border: 'none' }} />
                </tr>
              )}
              {visibleRows.map((row, idx) => {
                const globalIdx = startIndex + idx
                const callId =
                  searchProtoNum === OTLP_METRICS_PROTO
                    ? (row?.name ?? row?.NAME ?? '')
                    : row?.session_id || row?.cid || row?.trace_id || row?.TRACE_ID
                const rowBg = colorByCall ? callColor(callId, dark) : undefined
                return (
                  <tr
                    key={pickRowUuid(row) || globalIdx}
                    className="border-b border-border/50 hover:bg-sky-50/80 dark:hover:bg-sky-950/35"
                    style={rowBg ? { background: rowBg } : undefined}
                  >
                    {tableColumns.map((col) => {
                      if (col === '_select') {
                        const k = rowSelectionKey(row, globalIdx)
                        return (
                          <td
                            key={col}
                            className="w-10 border-b border-border/30 px-2 py-1 align-middle"
                            onClick={(e) => e.stopPropagation()}
                          >
                            <Checkbox
                              checked={selectedKeys.has(k)}
                              onCheckedChange={(c) => toggleRowSelected(row, globalIdx, c === true)}
                              aria-label="Select row"
                            />
                          </td>
                        )
                      }
                      if (col === '_actions') {
                        const hideMsg =
                          searchProtoNum === OTLP_TRACES_PROTO ||
                          searchProtoNum === OTLP_METRICS_PROTO
                        const txLabel =
                          searchProtoNum === OTLP_TRACES_PROTO
                            ? 'Trace'
                            : searchProtoNum === OTLP_LOGS_PROTO
                              ? 'Logs'
                              : searchProtoNum === OTLP_METRICS_PROTO
                                ? 'Series'
                                : 'Tx'
                        return (
                          <td key={col} className="border-b border-border/30 px-2 py-1">
                            {!hideMsg ? (
                            <div className="flex items-center gap-1">
                              <button
                                type="button"
                                onClick={() => openMessage(row)}
                                className="font-medium text-sky-700 underline-offset-2 hover:text-sky-900 hover:underline dark:text-sky-400 dark:hover:text-sky-300 dark:hover:underline"
                              >
                                Msg
                              </button>
                              <span className="text-slate-400 dark:text-slate-500">·</span>
                              <button
                                type="button"
                                onClick={() =>
                                  openTransaction(
                                    pickSessionForTx(row, searchProtoNum) || row?.session_id || row?.cid || row?.uuid,
                                    row,
                                  )
                                }
                                className="font-medium text-sky-700 underline-offset-2 hover:text-sky-900 hover:underline dark:text-sky-400 dark:hover:text-sky-300 dark:hover:underline"
                              >
                                {txLabel}
                              </button>
                            </div>
                            ) : (
                              <button
                                type="button"
                                onClick={() =>
                                  openTransaction(
                                    pickSessionForTx(row, searchProtoNum) || row?.session_id || row?.cid || row?.uuid,
                                    row,
                                  )
                                }
                                className="font-medium text-sky-700 underline-offset-2 hover:text-sky-900 hover:underline dark:text-sky-400 dark:hover:text-sky-300 dark:hover:underline"
                              >
                                {txLabel}
                              </button>
                            )}
                          </td>
                        )
                      }
                      let v = row[col]
                      if (col === 'timestamp') v = formatTs(v)
                      else if (col === 'src_ip') v = displaySrcIp(row)
                      else if (col === 'dst_ip') v = displayDstIp(row)
                      const text = v == null ? '' : typeof v === 'object' ? JSON.stringify(v) : String(v)
                      const rawIp =
                        col === 'src_ip'
                          ? row.src_ip
                          : col === 'dst_ip'
                            ? row.dst_ip
                            : undefined
                      const cellTitle =
                        (col === 'src_ip' || col === 'dst_ip') &&
                        rawIp != null &&
                        String(text) !== String(rawIp)
                          ? `${text}\n(IP: ${rawIp})`
                          : text
                      const tableLinkCls =
                        'font-medium text-sky-700 underline-offset-2 hover:text-sky-900 hover:underline dark:text-sky-400 dark:hover:text-sky-300 dark:hover:underline'

                      if (col === 'response_code') {
                        const code = Number(text)
                        let codeCls = 'text-slate-800 dark:text-zinc-200'
                        if (!Number.isNaN(code)) {
                          if (code >= 200 && code < 300) codeCls = 'text-emerald-700 dark:text-emerald-400'
                          else if (code >= 300 && code < 400) codeCls = 'text-violet-700 dark:text-violet-300'
                          else if (code >= 400) codeCls = 'text-rose-600 dark:text-rose-400'
                        }
                        return (
                          <td key={col} title={text} className="border-b border-border/30 px-2 py-1 align-top">
                            <button
                              type="button"
                              onClick={() => openMessage(row)}
                              className={cn(
                                'font-semibold underline-offset-2 hover:underline',
                                codeCls,
                              )}
                            >
                              {text || '—'}
                            </button>
                          </td>
                        )
                      }
                      if (['method', 'cseq_method'].includes(col)) {
                        return (
                          <td key={col} title={text} className="border-b border-border/30 px-2 py-1 align-top">
                            <button
                              type="button"
                              onClick={() => openMessage(row)}
                              className={tableLinkCls}
                            >
                              {text || '—'}
                            </button>
                          </td>
                        )
                      }
                      if (col === 'uuid' || col === 'UUID') {
                        return (
                          <td key={col} title={text} className="max-w-[220px] truncate border-b border-border/30 px-2 py-1 align-top">
                            <button
                              type="button"
                              onClick={() => openMessage(row)}
                              className={tableLinkCls}
                            >
                              {text || '—'}
                            </button>
                          </td>
                        )
                      }
                      if ((col === 'body' || col === 'BODY') && searchProtoNum === OTLP_LOGS_PROTO) {
                        return (
                          <td key={col} title={text} className="max-w-[280px] truncate border-b border-border/30 px-2 py-1 align-top">
                            <button
                              type="button"
                              onClick={() => openMessage(row)}
                              className={tableLinkCls}
                            >
                              {text || '—'}
                            </button>
                          </td>
                        )
                      }
                      if (
                        ['cid', 'session_id', 'trace_id', 'TRACE_ID'].includes(col) ||
                        ((col === 'name' || col === 'NAME') && searchProtoNum === OTLP_METRICS_PROTO)
                      ) {
                        return (
                          <td key={col} title={text} className="max-w-[220px] truncate border-b border-border/30 px-2 py-1 align-top">
                            <button
                              type="button"
                              onClick={() => openTransaction(text, row)}
                              className={tableLinkCls}
                            >
                              {text || '—'}
                            </button>
                          </td>
                        )
                      }
                      const style = {}
                      let extraCls = ''
                      if (col === 'country' || col === 'src_country' || col === 'dst_country') {
                        extraCls = cn(extraCls, 'font-semibold text-slate-800 dark:text-slate-100')
                      }
                      return (
                        <td
                          key={col}
                          title={cellTitle}
                          style={style}
                          className={cn('max-w-[260px] truncate border-b border-border/30 px-2 py-1 align-top', extraCls)}
                        >
                          {text}
                        </td>
                      )
                    })}
                  </tr>
                )
              })}
              {bottomSpacer > 0 && (
                <tr>
                  <td colSpan={tableColumns.length} style={{ height: bottomSpacer, padding: 0, border: 'none' }} />
                </tr>
              )}
            </tbody>
          </table>
          </div>
        </div>
      )}

      <Dialog open={columnsModalOpen} onOpenChange={setColumnsModalOpen}>
        <DialogContent className="!max-w-lg gap-3">
          <DialogHeader>
            <DialogTitle>Columns</DialogTitle>
          </DialogHeader>
          <ScrollArea className="h-[360px] pr-2">
            {allCols.length === 0 && (
              <div className="py-4 text-center text-xs text-muted-foreground">No columns yet</div>
            )}
            <div className="flex flex-col">
              {allCols.map((col, idx) => (
                <div
                  key={col}
                  className="flex items-center gap-2 border-b border-border/50 py-1.5 text-xs"
                >
                  <Checkbox
                    id={`col-${col}`}
                    checked={!hiddenColumns.includes(col)}
                    onCheckedChange={(checked) => {
                      setHiddenColumns((prev) => {
                        const next = checked
                          ? prev.filter((c) => c !== col)
                          : [...prev, col]
                        saveLS(LS_KEY_HIDDEN(widgetId, currentProto, currentEvent), next)
                        return next
                      })
                    }}
                  />
                  <label htmlFor={`col-${col}`} className="flex-1 cursor-pointer truncate">
                    {col}
                  </label>
                  <Button
                    type="button"
                    variant="ghost"
                    size="icon-xs"
                    disabled={idx === 0}
                    onClick={() => {
                      setColumnOrder((prev) => {
                        const arr = [...prev]
                        const i = arr.indexOf(col)
                        if (i <= 0) return prev
                        ;[arr[i - 1], arr[i]] = [arr[i], arr[i - 1]]
                        saveLS(LS_KEY_ORDER(widgetId, currentProto, currentEvent), arr)
                        return arr
                      })
                    }}
                  >
                    <ArrowUp className="h-3 w-3" />
                  </Button>
                  <Button
                    type="button"
                    variant="ghost"
                    size="icon-xs"
                    disabled={idx === allCols.length - 1}
                    onClick={() => {
                      setColumnOrder((prev) => {
                        const arr = [...prev]
                        const i = arr.indexOf(col)
                        if (i < 0 || i >= arr.length - 1) return prev
                        ;[arr[i], arr[i + 1]] = [arr[i + 1], arr[i]]
                        saveLS(LS_KEY_ORDER(widgetId, currentProto, currentEvent), arr)
                        return arr
                      })
                    }}
                  >
                    <ArrowDown className="h-3 w-3" />
                  </Button>
                </div>
              ))}
            </div>
          </ScrollArea>
          <DialogFooter className="sm:justify-start">
            <Button
              type="button"
              variant="outline"
              size="sm"
              onClick={() => {
                setHiddenColumns([])
                saveLS(LS_KEY_HIDDEN(widgetId, currentProto, currentEvent), [])
              }}
            >
              Show all
            </Button>
            <Button
              type="button"
              variant="outline"
              size="sm"
              onClick={() => {
                setColumnOrder([])
                saveLS(LS_KEY_ORDER(widgetId, currentProto, currentEvent), [])
              }}
            >
              Reset order
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {messageModals.map(m => (
        <MessageModal
          key={m.modalKey}
          modal={m}
          timeZone={timeZone}
          onClose={() => setMessageModals(prev => prev.filter(x => x.modalKey !== m.modalKey))}
        />
      ))}
      {transactionModals.map(m => (
        <TransactionModal
          key={m.modalKey}
          modal={m}
          timeZone={timeZone}
          onClose={() => setTransactionModals(prev => prev.filter(x => x.modalKey !== m.modalKey))}
        />
      ))}
      {otlpTraceModals.map((m) => (
        <OTLPTraceModal
          key={m.modalKey}
          modal={m}
          timeZone={timeZone}
          onClose={() => setOtplTraceModals((prev) => prev.filter((x) => x.modalKey !== m.modalKey))}
        />
      ))}
      {otlpLogModals.map((m) => (
        <OTLPLogsTraceModal
          key={m.modalKey}
          modal={m}
          timeZone={timeZone}
          onClose={() => setOtplLogModals((prev) => prev.filter((x) => x.modalKey !== m.modalKey))}
        />
      ))}
      {otlpLogRowModals.map((m) => (
        <OTLPLogRowModal
          key={m.modalKey}
          modal={m}
          timeZone={timeZone}
          onClose={() => setOtplLogRowModals((prev) => prev.filter((x) => x.modalKey !== m.modalKey))}
        />
      ))}
      {otlpMetricModals.map((m) => (
        <OTLPMetricsSeriesModal
          key={m.modalKey}
          modal={m}
          timeZone={timeZone}
          onClose={() => setOtplMetricModals((prev) => prev.filter((x) => x.modalKey !== m.modalKey))}
        />
      ))}
    </div>
  )
}
