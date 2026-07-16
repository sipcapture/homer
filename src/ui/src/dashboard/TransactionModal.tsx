// @ts-nocheck - TODO: rewrite with shadcn/ui + TanStack; typed when refactored
import React from 'react'
import { apiPost } from '../api'
import ExportModal from '../settings/ExportModal'
import CallFlow from './CallFlow'
import ExportTab from './ExportTab'
import MessageModal from './MessageModal'
import QosPanel from './QosPanel'
import CallInfoPanel from './CallInfoPanel'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { FloatingWindow } from '@/components/ui/floating-window'
import { Input } from '@/components/ui/input'
import { ScrollArea } from '@/components/ui/scroll-area'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { displayDstIp, displaySrcIp } from '@/lib/ipAliasDisplay'
import {
  formatJsonField,
  highlightJSON,
  isJsonDisplayable,
  serializeRowForDisplay,
} from '@/lib/jsonDisplay'
import { cn } from '@/lib/utils'
import { newModalKey } from '@/lib/modalKey'
import { useLocale } from '@/components/locale/locale-provider'
import { ArrowDown, ArrowUp, ArrowUpDown } from 'lucide-react'
import { getMethodColor } from './flow-utils'
import { resolveTimeRange } from './utils/resolveTimeRange'

/** SIP / transaction message row: DuckLake uses method + response_code, not `event`. */
function messageCallId(row) {
  const v = row?.session_id ?? row?.cid ?? row?.call_id ?? row?.callid
  if (v == null || v === '') return ''
  return String(v)
}

function messageEventLabel(row) {
  const rawRc = row?.response_code ?? row?.resp_code ?? row?.status
  if (rawRc != null && rawRc !== '') {
    let n
    if (typeof rawRc === 'bigint') n = Number(rawRc)
    else if (typeof rawRc === 'number') n = rawRc
    else n = parseInt(String(rawRc).trim(), 10)
    if (Number.isFinite(n) && n > 0) return String(n)
  }
  const m = row?.method ?? row?.sip_method ?? row?.cseq_method ?? row?.event
  if (m != null && String(m).trim() !== '') return String(m).trim()
  return ''
}

/** Compare displayed event labels: numeric SIP codes as integers, else case-insensitive string. */
function compareDisplayedEventLabels(a, b) {
  const sa = String(a ?? '').trim().toLowerCase()
  const sb = String(b ?? '').trim().toLowerCase()
  if (sa === sb) return 0
  const na = /^\d+$/.test(sa) ? parseInt(sa, 10) : NaN
  const nb = /^\d+$/.test(sb) ? parseInt(sb, 10) : NaN
  if (Number.isFinite(na) && Number.isFinite(nb)) return na - nb
  if (sa < sb) return -1
  if (sa > sb) return 1
  return 0
}

/** Synthetic key: original row index from the API (for “#” column sort / restore order). */
const MESSAGE_SORT_ORIG = '__orig__'

const MESSAGE_TABLE_COLUMNS = [
  { key: 'timestamp', header: 'Timestamp', format: 'datetime' },
  { key: 'call_id', header: 'Call-ID', accessor: messageCallId },
  { key: 'event', header: 'Event', accessor: messageEventLabel },
  { key: 'src_ip', header: 'src_ip' },
  { key: 'src_port', header: 'src_port' },
  { key: 'dst_ip', header: 'dst_ip' },
  { key: 'dst_port', header: 'dst_port' },
  { key: 'caller', header: 'caller' },
  { key: 'callee', header: 'callee' },
]

function messageSortScalar(colKey, row, origIndex) {
  if (colKey === MESSAGE_SORT_ORIG) return origIndex
  const col = MESSAGE_TABLE_COLUMNS.find((c) => c.key === colKey)
  if (!col) return ''
  if (col.accessor) return col.accessor(row)
  let value = row[col.key]
  if (col.key === 'src_ip') value = displaySrcIp(row)
  else if (col.key === 'dst_ip') value = displayDstIp(row)
  if (col.format === 'datetime') {
    const d = parseTimestampValue(value)
    return d && !Number.isNaN(d.getTime()) ? d.getTime() : null
  }
  if (col.key === 'src_port' || col.key === 'dst_port') {
    if (value == null || value === '') return null
    const n = typeof value === 'number' ? value : parseInt(String(value).trim(), 10)
    return Number.isFinite(n) ? n : null
  }
  if (value === undefined || value === null) return ''
  return String(value)
}

/**
 * Compare two message rows for table sorting. Null / empty numeric or missing time sort last in ASC
 * (same idea as SQL NULLS LAST).
 */
function compareMessageRowsForSort(colKey, dir, rowA, origA, rowB, origB) {
  const va = messageSortScalar(colKey, rowA, origA)
  const vb = messageSortScalar(colKey, rowB, origB)

  const nullsLastBody = (cmpNums) => {
    const aNull = va == null || va === ''
    const bNull = vb == null || vb === ''
    if (aNull && bNull) return 0
    if (aNull) return 1
    if (bNull) return -1
    if (cmpNums) {
      const c = va - vb
      if (c !== 0) return c
    }
    const sa = String(va).toLowerCase()
    const sb = String(vb).toLowerCase()
    if (sa < sb) return -1
    if (sa > sb) return 1
    return origA - origB
  }

  let cmp
  if (colKey === MESSAGE_SORT_ORIG) {
    cmp = va - vb
  } else if (colKey === 'timestamp' || colKey === 'src_port' || colKey === 'dst_port') {
    cmp = nullsLastBody(true)
  } else if (colKey === 'event') {
    const aNull = va == null || va === ''
    const bNull = vb == null || vb === ''
    if (aNull && bNull) cmp = 0
    else if (aNull) cmp = 1
    else if (bNull) cmp = -1
    else {
      cmp = compareDisplayedEventLabels(va, vb)
      if (cmp === 0) cmp = origA - origB
    }
  } else {
    cmp = nullsLastBody(false)
  }
  if (cmp !== 0) return dir === 'asc' ? cmp : -cmp
  return origA - origB
}

function formatMessageTableCell(col, row, timeZone, locale) {
  let text
  if (col.accessor) {
    text = col.accessor(row)
  } else {
    let value = row[col.key]
    if (col.key === 'src_ip') {
      value = displaySrcIp(row)
    } else if (col.key === 'dst_ip') {
      value = displayDstIp(row)
    }
    if (col.format === 'datetime') value = formatDateTime(value, locale, timeZone)
    text = value === undefined || value === null ? '' : String(value)
  }
  if (text === '' || text == null) return '—'
  // Colour the Event column the same way the Call Flow arrows do
  // — getMethodColor handles both SIP method names (INVITE/BYE/…)
  // and numeric response codes (1xx grey, 2xx green, 3xx purple,
  // 4xx orange, 5xx/6xx red), so the Messages table reads at a
  // glance instead of a wall of monochrome text.
  if (col.key === 'event') {
    return (
      <span style={{ color: getMethodColor(text), fontWeight: 600 }}>
        {text}
      </span>
    )
  }
  return text
}

function parseTimestampValue(value) {
  if (!value && value !== 0) return null
  if (value instanceof Date) return value
  if (typeof value === 'number') {
    let ms = value
    if (value > 1e15) ms = Math.round(value / 1e6)
    else if (value > 1e12) ms = Math.round(value)
    else if (value > 1e9) ms = Math.round(value * 1000)
    return new Date(ms)
  }
  if (typeof value === 'string') {
    let s = value.trim()
    if (/^\d{4}-\d{2}-\d{2}$/.test(s)) return new Date(`${s}T00:00:00Z`)
    if (s.includes(' ') && !s.includes('T')) s = s.replace(' ', 'T')
    if (!/[zZ]|[+\-]\d{2}:?\d{2}$/.test(s)) s = `${s}Z`
    const d = new Date(s)
    return Number.isNaN(d.getTime()) ? null : d
  }
  return null
}

/**
 * Time window for QoS/events correlation: span all loaded SIP messages ± padding (ms),
 * like classic Homer using the full call/search range — not ±5m around a single grid row.
 */
function timeRangeFromMessageItems(messageRows, fallback, timeZone, padMs = 5 * 60 * 1000) {
  const resolvedFallback = fallback ? resolveTimeRange(fallback, timeZone) : null
  if (!messageRows?.length) {
    if (resolvedFallback?.from != null && resolvedFallback?.to != null) {
      return { from: resolvedFallback.from, to: resolvedFallback.to }
    }
    return null
  }
  let min = Infinity
  let max = -Infinity
  for (const row of messageRows) {
    const d = parseTimestampValue(row?.timestamp)
    if (!d) continue
    const ms = d.getTime()
    if (Number.isNaN(ms)) continue
    min = Math.min(min, ms)
    max = Math.max(max, ms)
  }
  if (!Number.isFinite(min) || !Number.isFinite(max)) {
    if (resolvedFallback?.from != null && resolvedFallback?.to != null) {
      return { from: resolvedFallback.from, to: resolvedFallback.to }
    }
    return null
  }
  return { from: min - padMs, to: max + padMs }
}

/** Body for transaction sub-APIs: multi Call-ID uses session_ids + SIP time span from loaded messages. */
function buildTransactionTabBody(sessionIdsForApi, items, timeRange, timeZone, extra = {}) {
  const tr = timeRangeFromMessageItems(items, timeRange, timeZone)
  const body = { ...extra }
  if (sessionIdsForApi.length > 1) {
    body.session_ids = sessionIdsForApi
  } else if (sessionIdsForApi.length === 1) {
    body.session_id = sessionIdsForApi[0]
  }
  if (tr) body.timestamp = { from: tr.from, to: tr.to }
  return body
}

function formatDateTime(value, locale, timeZone) {
  const date = parseTimestampValue(value)
  if (!date) return value
  const options = {
    year: 'numeric',
    month: 'numeric',
    day: 'numeric',
    hour: 'numeric',
    minute: 'numeric',
    second: 'numeric',
    fractionalSecondDigits: 3,
  }
  if (timeZone && timeZone !== 'local') options.timeZone = timeZone
  return new Intl.DateTimeFormat(locale, options).format(date)
}

/** Transaction → Events (HEP LOG / proto 100): fixed columns; table-fixed + col widths keep cells from overflowing. */
const EVENTS_TABLE_COLUMNS = [
  { header: 'Date', keys: ['timestamp', 'date', 'record_datetime'], isDate: true, colClass: 'w-[14%] min-w-0' },
  { header: 'Node_ID', keys: ['node_id', 'node'], colClass: 'w-[10%] min-w-0' },
  { header: 'Payload', keys: ['payload', 'message', 'data'], colClass: 'w-[26%] min-w-0', cellBreak: 'break-all' },
  { header: 'Session_ID', keys: ['session_id', 'call_id', 'callid', 'cid'], colClass: 'w-[20%] min-w-0' },
  { header: 'Storage Value', keys: ['storage_value', 'storage', 'lake_name', 'lake'], colClass: 'w-[12%] min-w-0' },
  { header: 'UUID', keys: ['uuid', 'guid'], colClass: 'w-[18%] min-w-0' },
]

/** OTLP Log search result table (subset of otlp_logs columns). */
const OTLP_LOG_TABLE_COLUMNS = [
  { header: 'Timestamp', keys: ['timestamp'], isDate: true, colClass: 'w-[13%] min-w-0' },
  { header: 'Scope name', keys: ['scope_name'], colClass: 'w-[11%] min-w-0' },
  { header: 'Service name', keys: ['service_name'], colClass: 'w-[10%] min-w-0' },
  { header: 'Body', keys: ['body'], colClass: 'w-[26%] min-w-0', cellBreak: 'break-all' },
  { header: 'Span ID', keys: ['span_id'], colClass: 'w-[12%] min-w-0', cellBreak: 'break-all' },
  { header: 'Trace ID', keys: ['trace_id'], colClass: 'w-[14%] min-w-0', cellBreak: 'break-all' },
  { header: 'Severity', keys: ['severity_text'], colClass: 'w-[14%] min-w-0' },
]

function pickFirstField(row, keys) {
  for (const k of keys) {
    if (row[k] != null && row[k] !== '') return row[k]
  }
  return null
}

function formatEventsCell(value, col, timeZone, locale) {
  if (value == null || value === '') return '—'
  if (col.isDate) {
    const s = formatDateTime(value, locale, timeZone)
    return s || '—'
  }
  const payloadLike = col.keys?.some((k) => k === 'payload' || k === 'message' || k === 'data' || k === 'body')
  if (payloadLike && isJsonDisplayable(value)) {
    const pretty = formatJsonField(value).replace(/\s+/g, ' ')
    return pretty.length > 200 ? `${pretty.slice(0, 197)}…` : pretty
  }
  if (typeof value === 'object') {
    try {
      const j = JSON.stringify(value)
      return j.length > 140 ? `${j.slice(0, 137)}…` : j
    } catch {
      return '—'
    }
  }
  const s = String(value)
  return s.length > 200 ? `${s.slice(0, 197)}…` : s
}

function EventRecordDetail({ row }) {
  const [recordTab, setRecordTab] = React.useState('pretty')
  const recordText = serializeRowForDisplay(row)
  const recordPrettyHtml = highlightJSON(row)

  return (
    <div className="flex h-full min-h-0 flex-1 flex-col overflow-hidden">
      <Tabs
        value={recordTab}
        onValueChange={setRecordTab}
        className="flex h-full min-h-0 flex-1 flex-col gap-1 overflow-hidden"
      >
        <TabsList variant="line" className="h-8 w-fit shrink-0 justify-start">
          <TabsTrigger value="pretty" className="text-[11px]">
            Pretty
          </TabsTrigger>
          <TabsTrigger value="raw" className="text-[11px]">
            Raw
          </TabsTrigger>
        </TabsList>
        <TabsContent
          value="pretty"
          className="mt-0 flex min-h-0 flex-1 flex-col overflow-hidden data-[state=inactive]:hidden"
        >
          <div className="min-h-0 flex-1 overflow-auto rounded-md border border-border bg-muted/40">
            <pre
              className="w-max min-w-full whitespace-pre p-2 font-mono text-[11px] leading-relaxed"
              dangerouslySetInnerHTML={{ __html: recordPrettyHtml || '(empty)' }}
            />
          </div>
        </TabsContent>
        <TabsContent
          value="raw"
          className="mt-0 flex min-h-0 flex-1 flex-col overflow-hidden data-[state=inactive]:hidden"
        >
          <div className="min-h-0 flex-1 overflow-auto rounded-md border border-border bg-muted/40">
            <pre className="min-w-full whitespace-pre-wrap break-words p-2 font-mono text-[11px] leading-relaxed text-foreground">
              {recordText}
            </pre>
          </div>
        </TabsContent>
      </Tabs>
    </div>
  )
}

function EventsTab({ data, isLoading, err, emptyLabel, timeZone, locale }) {
  const [detailWindows, setDetailWindows] = React.useState([])

  const items = Array.isArray(data?.items) ? data.items : null
  React.useEffect(() => {
    setDetailWindows([])
  }, [data, isLoading, err])

  const openDetailWindow = (row) => {
    setDetailWindows((prev) => [...prev, { windowKey: newModalKey(), row }])
  }

  const closeDetailWindow = (windowKey) => {
    setDetailWindows((prev) => prev.filter((w) => w.windowKey !== windowKey))
  }

  if (isLoading) {
    return <div className="p-4 text-xs text-muted-foreground">Loading...</div>
  }
  if (err) {
    return (
      <Alert variant="destructive" className="m-2 py-2">
        <AlertDescription className="text-xs">{err}</AlertDescription>
      </Alert>
    )
  }
  if (!data || (typeof data === 'object' && Object.keys(data).length === 0)) {
    return <div className="p-4 text-xs text-muted-foreground">{emptyLabel}</div>
  }
  if (!items || items.length === 0) {
    return <div className="p-4 text-xs text-muted-foreground">{emptyLabel}</div>
  }
  return (
    <>
      <div className="flex h-full min-h-0 flex-col gap-1">
        <p className="shrink-0 px-1 text-[10px] text-muted-foreground">
          Click a row to open the full log record in a resizable window. Each click opens a new window.
        </p>
        <ScrollArea className="min-h-0 flex-1 border border-border">
          <Table className="table-fixed">
            <TableHeader className="sticky top-0 z-10 bg-card">
              <TableRow>
                {EVENTS_TABLE_COLUMNS.map((col) => (
                  <TableHead
                    key={col.header}
                    className={cn('text-[10px] uppercase tracking-wider', col.colClass)}
                  >
                    {col.header}
                  </TableHead>
                ))}
              </TableRow>
            </TableHeader>
            <TableBody>
              {items.map((row, idx) => (
                <TableRow
                  key={row.uuid || row.guid || idx}
                  className={`cursor-pointer hover:brightness-95 ${idx % 2 === 0 ? 'bg-white dark:bg-slate-800' : 'bg-sky-50 dark:bg-sky-900/60'}`}
                  title="View full event"
                  onClick={() => openDetailWindow(row)}
                >
                  {EVENTS_TABLE_COLUMNS.map((col) => (
                    <TableCell
                      key={col.header}
                      className={cn(
                        'min-w-0 align-top whitespace-normal font-mono text-[11px] break-words',
                        col.colClass,
                        col.cellBreak,
                      )}
                    >
                      {formatEventsCell(pickFirstField(row, col.keys), col, timeZone, locale)}
                    </TableCell>
                  ))}
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </ScrollArea>
      </div>

      {detailWindows.map(({ windowKey, row }) => (
        <FloatingWindow
          key={windowKey}
          open
          onClose={() => closeDetailWindow(windowKey)}
          id={`evlog:${windowKey}`}
          title={
            <span className="truncate font-mono text-xs">
              Event log
              {(row.uuid || row.guid) && (
                <span className="text-muted-foreground">
                  {' '}
                  — {String(row.uuid || row.guid)}
                </span>
              )}
            </span>
          }
          defaultWidth={Math.min(760, window.innerWidth - 80)}
          defaultHeight={Math.min(Math.round(window.innerHeight * 0.72), 640)}
          minWidth={420}
          minHeight={280}
          className="flex min-h-0 flex-col overflow-hidden"
        >
          <div className="flex min-h-0 flex-1 flex-col overflow-hidden px-3 pb-3 pt-1">
            <EventRecordDetail row={row} />
          </div>
        </FloatingWindow>
      ))}
    </>
  )
}

function OtlpLogsTab({
  callIdInput,
  onCallIdChange,
  onSearch,
  searchLoading,
  searchErr,
  resultData,
  hasSearched,
  timeZone,
  locale,
}) {
  const [detailRow, setDetailRow] = React.useState(null)
  const items = Array.isArray(resultData?.items) ? resultData.items : null

  React.useEffect(() => {
    setDetailRow(null)
  }, [resultData, searchLoading, searchErr])

  return (
    <>
      <div className="flex h-full min-h-0 flex-col gap-3">
        <div className="flex shrink-0 flex-col gap-2 sm:flex-row sm:items-end">
          <div className="min-w-0 flex-1 space-y-1">
            <label htmlFor="otlp-log-callid" className="text-[10px] font-medium uppercase tracking-wide text-muted-foreground">
              Call-ID (search in otlp_logs)
            </label>
            <Input
              id="otlp-log-callid"
              value={callIdInput}
              onChange={(e) => onCallIdChange(e.target.value)}
              className="font-mono text-xs"
              placeholder="Call-ID"
              autoComplete="off"
            />
          </div>
          <Button type="button" size="sm" className="shrink-0" onClick={onSearch} disabled={searchLoading}>
            {searchLoading ? 'Searching…' : 'Search'}
          </Button>
        </div>
        <p className="text-[10px] text-muted-foreground">
          Same time range as this transaction. Click a row to open the full OTLP log record (all fields, JSON).
        </p>
        {searchErr && (
          <Alert variant="destructive" className="py-2">
            <AlertDescription className="text-xs">{searchErr}</AlertDescription>
          </Alert>
        )}
        {searchLoading && (
          <div className="text-xs text-muted-foreground">Searching otlp_logs…</div>
        )}
        {!searchLoading && !searchErr && hasSearched && (!items || items.length === 0) && (
          <div className="text-xs text-muted-foreground">No OTLP log rows matched this Call-ID in the current time window.</div>
        )}
        {items && items.length > 0 && (
          <ScrollArea className="min-h-0 flex-1 border border-border">
            <Table className="table-fixed">
              <TableHeader className="sticky top-0 z-10 bg-card">
                <TableRow>
                  {OTLP_LOG_TABLE_COLUMNS.map((col) => (
                    <TableHead
                      key={col.header}
                      className={cn('text-[10px] uppercase tracking-wider', col.colClass)}
                    >
                      {col.header}
                    </TableHead>
                  ))}
                </TableRow>
              </TableHeader>
              <TableBody>
                {items.map((row, idx) => (
                  <TableRow
                    key={row.trace_id ? `${String(row.trace_id)}-${idx}` : idx}
                    className={`cursor-pointer hover:brightness-95 ${idx % 2 === 0 ? 'bg-white dark:bg-slate-800' : 'bg-sky-50 dark:bg-sky-900/60'}`}
                    title="View full OTLP log record"
                    onClick={() => setDetailRow(row)}
                  >
                    {OTLP_LOG_TABLE_COLUMNS.map((col) => (
                      <TableCell
                        key={col.header}
                        className={cn(
                          'min-w-0 align-top whitespace-normal font-mono text-[11px] break-words',
                          col.colClass,
                          col.cellBreak,
                        )}
                      >
                        {formatEventsCell(pickFirstField(row, col.keys), col, timeZone, locale)}
                      </TableCell>
                    ))}
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </ScrollArea>
        )}
      </div>

      {detailRow ? (
        <FloatingWindow
          open
          onClose={() => setDetailRow(null)}
          id={`otlp-row:${[detailRow.trace_id, detailRow.timestamp, detailRow.span_id].filter(Boolean).join('|') || 'row'}`}
          title={
            <span className="truncate font-mono text-xs">
              OTLP log row
              {detailRow.trace_id != null && String(detailRow.trace_id).trim() !== '' && (
                <span className="text-muted-foreground"> — {String(detailRow.trace_id).trim()}</span>
              )}
            </span>
          }
          defaultWidth={Math.min(900, window.innerWidth - 64)}
          defaultHeight={Math.min(Math.round(window.innerHeight * 0.78), 720)}
          minWidth={480}
          minHeight={320}
          className="flex min-h-0 flex-col overflow-hidden"
        >
          <div className="flex min-h-0 flex-1 flex-col overflow-hidden px-3 pb-3 pt-1">
            <EventRecordDetail row={detailRow} />
          </div>
        </FloatingWindow>
      ) : null}
    </>
  )
}



export default function TransactionModal({ modal, onClose, timeZone }) {
  const { resolved: locale } = useLocale()
  const [activeTab, setActiveTab] = React.useState('messages')
  const [qosData, setQosData] = React.useState(null)
  const [qosLoading, setQosLoading] = React.useState(false)
  const [qosError, setQosError] = React.useState('')
  const [callInfoData, setCallInfoData] = React.useState(null)
  const [callInfoLoading, setCallInfoLoading] = React.useState(false)
  const [callInfoError, setCallInfoError] = React.useState('')
  const [eventsData, setEventsData] = React.useState(null)
  const [eventsLoading, setEventsLoading] = React.useState(false)
  const [eventsError, setEventsError] = React.useState('')
  const [otlpLogCallId, setOtlpLogCallId] = React.useState('')
  const [otlpLogData, setOtlpLogData] = React.useState(null)
  const [otlpLogLoading, setOtlpLogLoading] = React.useState(false)
  const [otlpLogErr, setOtlpLogErr] = React.useState('')
  const [otlpLogSearched, setOtlpLogSearched] = React.useState(false)
  const [exportOpen, setExportOpen] = React.useState(false)
  const [messageModals, setMessageModals] = React.useState([])
  const [msgSortCol, setMsgSortCol] = React.useState(null)
  const [msgSortDir, setMsgSortDir] = React.useState('asc')

  // reset per-tab state whenever a new transaction is opened
  React.useEffect(() => {
    setActiveTab('messages')
    setQosData(null); setQosError(''); setQosLoading(false)
    setCallInfoData(null); setCallInfoError(''); setCallInfoLoading(false)
    setEventsData(null); setEventsError(''); setEventsLoading(false)
    setOtlpLogData(null); setOtlpLogErr(''); setOtlpLogLoading(false); setOtlpLogSearched(false)
    setExportOpen(false)
    setMessageModals([])
    setMsgSortCol(null)
    setMsgSortDir('asc')
  }, [modal?.modalKey])

  React.useEffect(() => {
    if (!modal?.modalKey) return
    const pid = modal.primarySessionId ?? (modal.sessionIds && modal.sessionIds[0]) ?? modal.id ?? ''
    setOtlpLogCallId(String(pid).trim())
  }, [modal?.modalKey, modal?.primarySessionId, modal?.id, modal?.sessionIds])

  if (!modal) return null
  const { id, loading, items, error, timeRange, modalKey, sessionIds, primarySessionId } = modal
  const primaryId = primarySessionId ?? (sessionIds && sessionIds[0]) ?? id
  const isMultiSession = Array.isArray(sessionIds) && sessionIds.length > 1
  const sessionIdsForApi = (Array.isArray(sessionIds) && sessionIds.length)
    ? sessionIds.map((s) => String(s).trim()).filter(Boolean)
    : (primaryId ? [String(primaryId).trim()] : [])

  const messageRowsIndexed = React.useMemo(
    () => (items || []).map((row, orig) => ({ row, orig })),
    [items],
  )

  const messageRowsSorted = React.useMemo(() => {
    if (!msgSortCol) return messageRowsIndexed
    const copy = [...messageRowsIndexed]
    copy.sort((a, b) =>
      compareMessageRowsForSort(msgSortCol, msgSortDir, a.row, a.orig, b.row, b.orig),
    )
    return copy
  }, [messageRowsIndexed, msgSortCol, msgSortDir])

  const handleMessageSort = (colKey) => {
    setMsgSortCol((prev) => {
      if (prev === colKey) {
        setMsgSortDir((d) => (d === 'asc' ? 'desc' : 'asc'))
        return colKey
      }
      setMsgSortDir('asc')
      return colKey
    })
  }

  const loadQos = async () => {
    if (qosData || qosLoading) return
    setQosLoading(true)
    setQosError('')
    try {
      const body = buildTransactionTabBody(sessionIdsForApi, items, timeRange, timeZone)
      const data = await apiPost('/transactions/qos', body)
      setQosData(data?.data || {})
    } catch (err) {
      setQosError(err.message)
    } finally {
      setQosLoading(false)
    }
  }

  const loadCallInfo = async () => {
    if (callInfoData || callInfoLoading) return
    setCallInfoLoading(true)
    setCallInfoError('')
    try {
      const body = buildTransactionTabBody(sessionIdsForApi, items, timeRange, timeZone, {
        proto_type: modal?.protoType || 1,
        event_type: modal?.eventType || 'all',
      })
      const data = await apiPost('/transactions/callinfo', body)
      setCallInfoData(data?.data || {})
    } catch (err) {
      setCallInfoError(err.message)
    } finally {
      setCallInfoLoading(false)
    }
  }

  const loadEvents = async () => {
    if (eventsData || eventsLoading) return
    setEventsLoading(true)
    setEventsError('')
    try {
      const body = buildTransactionTabBody(sessionIdsForApi, items, timeRange, timeZone)
      const data = await apiPost('/transactions/events', body)
      setEventsData(data?.data || {})
    } catch (err) {
      setEventsError(err.message)
    } finally {
      setEventsLoading(false)
    }
  }

  const runOtlpLogSearch = async () => {
    setOtlpLogErr('')
    setOtlpLogLoading(true)
    setOtlpLogSearched(true)
    try {
      const body = {
        ...buildTransactionTabBody(sessionIdsForApi, items, timeRange, timeZone),
        call_id: otlpLogCallId.trim(),
      }
      const data = await apiPost('/transactions/otlp-logs', body)
      setOtlpLogData(data?.data || {})
    } catch (err) {
      setOtlpLogErr(err.message)
      setOtlpLogData(null)
    } finally {
      setOtlpLogLoading(false)
    }
  }

  const handleTabChange = (tab) => {
    setActiveTab(tab)
    if (tab === 'qos') loadQos()
    if (tab === 'callinfo') loadCallInfo()
    if (tab === 'events') loadEvents()
  }

  const handleFlowMessageClick = async (flowItem) => {
    const raw = flowItem.raw || flowItem
    const uuid = raw.uuid || raw.id || flowItem.id
    const messageContext = {
      proto_type: 1,
      event_type: 'all',
      timestamp: (() => {
        const ts = resolveTimeRange(timeRange, timeZone)
        return ts ? { from: ts.from, to: ts.to } : undefined
      })(),
    }
    const nestedKey = newModalKey()
    const update = (modal) => setMessageModals(prev =>
      prev.map(m => m.modalKey === nestedKey ? modal : m)
    )
    if (raw.payload) {
      setMessageModals(prev => [...prev, { modalKey: nestedKey, uuid, loading: false, data: raw, error: null, messageContext }])
      return
    }
    setMessageModals(prev => [...prev, { modalKey: nestedKey, uuid, loading: true, data: null, error: null, messageContext }])
    try {
      const body = { uuid, proto_type: 1, event_type: 'all' }
      if (messageContext.timestamp) body.timestamp = messageContext.timestamp
      const result = await apiPost('/messages', body)
      update({ modalKey: nestedKey, uuid, loading: false, data: result?.data || raw, error: null, messageContext })
    } catch {
      update({ modalKey: nestedKey, uuid, loading: false, data: raw, error: null, messageContext })
    }
  }

  return (
    <>
      <FloatingWindow
        open
        onClose={onClose}
        id={`tx:${modalKey || id}`}
        title={
          isMultiSession ? (
            <span className="truncate text-sm">
              Transactions:{' '}
              <span className="font-mono text-muted-foreground">
                {sessionIds.length} {sessionIds.length === 1 ? 'session' : 'sessions'}
              </span>
            </span>
          ) : (
            <span className="truncate font-mono text-sm">
              Transaction: <span className="text-muted-foreground">{id}</span>
            </span>
          )
        }
        headerActions={null}
        defaultWidth={Math.min(900, window.innerWidth - 120)}
        defaultHeight={Math.min(Math.round(window.innerHeight * 0.65), 620)}
        minWidth={640}
        minHeight={400}
        className="flex flex-col"
      >
        <div className="flex min-h-0 flex-1 flex-col">
          {loading && (
            <div className="flex items-center justify-center py-6 text-xs text-muted-foreground">
              Loading...
            </div>
          )}

          {error && (
            <Alert variant="destructive" className="m-4 py-2">
              <AlertDescription className="text-xs">{error}</AlertDescription>
            </Alert>
          )}

          {!loading && !error && (
            <Tabs
              value={activeTab}
              onValueChange={handleTabChange}
              className="flex min-h-0 flex-1 flex-col gap-0 px-0"
            >
              <TabsList
                variant="line"
                className="mx-4 mt-2 h-9 w-[calc(100%-2rem)] shrink-0 justify-start overflow-x-auto overflow-y-clip border-b border-border"
              >
                <TabsTrigger value="messages">Messages</TabsTrigger>
                <TabsTrigger value="flow">Call Flow</TabsTrigger>
                <TabsTrigger value="qos">QoS</TabsTrigger>
                <TabsTrigger value="callinfo">Call Info</TabsTrigger>
                <TabsTrigger value="events">Events</TabsTrigger>
                <TabsTrigger value="otlplogs">OTLP Log</TabsTrigger>
                <TabsTrigger value="export">Export</TabsTrigger>
              </TabsList>

              <TabsContent value="messages" className="min-h-0 flex-1 overflow-hidden px-4 pb-4">
                <ScrollArea className="h-full border border-border">
                  <Table>
                    <TableHeader className="sticky top-0 z-10 bg-card">
                      <TableRow>
                        <TableHead className="w-10 text-[10px] uppercase tracking-wider">
                          <button
                            type="button"
                            className="inline-flex cursor-pointer select-none items-center gap-1 border-0 bg-transparent p-0 font-inherit text-inherit hover:text-foreground"
                            title="Sort by row # (API order)"
                            onClick={() => handleMessageSort(MESSAGE_SORT_ORIG)}
                          >
                            #
                            {msgSortCol === MESSAGE_SORT_ORIG ? (
                              msgSortDir === 'asc' ? (
                                <ArrowUp className="h-3 w-3 shrink-0" />
                              ) : (
                                <ArrowDown className="h-3 w-3 shrink-0" />
                              )
                            ) : (
                              <ArrowUpDown className="h-3 w-3 shrink-0 opacity-45 dark:opacity-50" />
                            )}
                          </button>
                        </TableHead>
                        {MESSAGE_TABLE_COLUMNS.map((col) => (
                          <TableHead key={col.key} className="text-[10px] uppercase tracking-wider">
                            <button
                              type="button"
                              className="inline-flex cursor-pointer select-none items-center gap-1 border-0 bg-transparent p-0 font-inherit text-inherit hover:text-foreground"
                              title={`Sort by ${col.header}`}
                              onClick={() => handleMessageSort(col.key)}
                            >
                              {col.header}
                              {msgSortCol === col.key ? (
                                msgSortDir === 'asc' ? (
                                  <ArrowUp className="h-3 w-3 shrink-0" />
                                ) : (
                                  <ArrowDown className="h-3 w-3 shrink-0" />
                                )
                              ) : (
                                <ArrowUpDown className="h-3 w-3 shrink-0 opacity-45 dark:opacity-50" />
                              )}
                            </button>
                          </TableHead>
                        ))}
                      </TableRow>
                    </TableHeader>
                    <TableBody>
                      {messageRowsSorted.map(({ row, orig }, idx) => (
                        <TableRow
                          key={row.uuid != null && String(row.uuid) !== '' ? String(row.uuid) : `tx-msg-${orig}`}
                          className={`cursor-pointer hover:brightness-95 ${idx % 2 === 0 ? 'bg-white dark:bg-slate-800' : 'bg-sky-50 dark:bg-sky-900/60'}`}
                          onClick={() => handleFlowMessageClick({ raw: row, id: row.uuid ?? orig })}
                        >
                          <TableCell className="font-mono text-[11px] text-muted-foreground">
                            {idx + 1}
                          </TableCell>
                          {MESSAGE_TABLE_COLUMNS.map((col) => (
                            <TableCell key={col.key} className="font-mono text-[11px]">
                              {formatMessageTableCell(col, row, timeZone, locale)}
                            </TableCell>
                          ))}
                        </TableRow>
                      ))}
                    </TableBody>
                  </Table>
                </ScrollArea>
              </TabsContent>

              <TabsContent value="flow" className="min-h-0 min-w-0 flex-1 overflow-hidden px-4 pb-4">
                <div className="h-full w-full min-w-0 overflow-auto border border-border bg-card/40">
                  <CallFlow items={items} timeZone={timeZone} onClickMessage={handleFlowMessageClick} />
                </div>
              </TabsContent>

              <TabsContent value="qos" className="flex min-h-0 flex-1 flex-col overflow-hidden px-4 pb-4">
                <div className="flex min-h-0 flex-1 flex-col overflow-hidden">
                  {qosLoading && (
                    <div className="p-4 text-xs text-muted-foreground">Loading QoS data...</div>
                  )}
                  {qosError && (
                    <Alert variant="destructive" className="m-2 py-2">
                      <AlertDescription className="text-xs">{qosError}</AlertDescription>
                    </Alert>
                  )}
                  {qosData && !qosLoading && (
                    <QosPanel qosData={qosData} timeZone={timeZone} />
                  )}
                  {!qosData && !qosLoading && !qosError && (
                    <div className="p-4 text-xs text-muted-foreground">No QoS data.</div>
                  )}
                </div>
              </TabsContent>

              <TabsContent value="callinfo" className="min-h-0 flex-1 overflow-hidden px-4 pb-4">
                <CallInfoPanel
                  data={callInfoData}
                  isLoading={callInfoLoading}
                  err={callInfoError}
                  emptyLabel="No call info available for this transaction."
                />
              </TabsContent>

              <TabsContent value="events" className="min-h-0 flex-1 overflow-hidden px-4 pb-4">
                <EventsTab
                  data={eventsData}
                  isLoading={eventsLoading}
                  err={eventsError}
                  emptyLabel="No events available for this transaction."
                  timeZone={timeZone}
                  locale={locale}
                />
              </TabsContent>

              <TabsContent value="otlplogs" className="min-h-0 flex-1 overflow-hidden px-4 pb-4">
                <OtlpLogsTab
                  callIdInput={otlpLogCallId}
                  onCallIdChange={setOtlpLogCallId}
                  onSearch={runOtlpLogSearch}
                  searchLoading={otlpLogLoading}
                  searchErr={otlpLogErr}
                  resultData={otlpLogData}
                  hasSearched={otlpLogSearched}
                  timeZone={timeZone}
                  locale={locale}
                />
              </TabsContent>

              <TabsContent value="export" className="min-h-0 flex-1 overflow-hidden">
                <ExportTab
                  sessionId={primaryId}
                  sessionIds={sessionIdsForApi.length > 1 ? sessionIdsForApi : undefined}
                  items={items}
                  timeRange={timeRange}
                  protoType={modal?.protoType || 1}
                  eventType={modal?.eventType || 'all'}
                />
              </TabsContent>
            </Tabs>
          )}
        </div>
      </FloatingWindow>

      {exportOpen && (
        <ExportModal
          searchPayload={
            sessionIdsForApi.length > 1
              ? { session_ids: sessionIdsForApi }
              : { session_id: primaryId }
          }
          onClose={() => setExportOpen(false)}
        />
      )}

      {messageModals.map(m => (
        <MessageModal
          key={m.modalKey}
          modal={m}
          onClose={() => setMessageModals(prev => prev.filter(x => x.modalKey !== m.modalKey))}
          timeZone={timeZone}
        />
      ))}
    </>
  )
}
