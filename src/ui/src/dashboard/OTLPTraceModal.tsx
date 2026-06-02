// @ts-nocheck — Grafana-style OTLP trace waterfall; typed when refactored
import { useCallback, useEffect, useMemo, useState } from 'react'
import { AlertCircle, ChevronDown, ChevronRight } from 'lucide-react'
import { FloatingWindow } from '@/components/ui/floating-window'
import { ScrollArea } from '@/components/ui/scroll-area'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Input } from '@/components/ui/input'
import { cn } from '@/lib/utils'
import { useLocale } from '@/components/locale/locale-provider'

function isEmptyParentSpanId(v) {
  if (v == null) return true
  const s = String(v).trim().toLowerCase()
  if (!s) return true
  if (/^0+$/.test(s)) return true
  return false
}

function spanKey(row) {
  return String(row?.span_id ?? row?.SPAN_ID ?? row?.spanId ?? '').trim()
}

function parentKey(row) {
  return String(row?.parent_span_id ?? row?.PARENT_SPAN_ID ?? '').trim()
}

function rowTimeMs(val) {
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

function rowEndMs(row) {
  const end = rowTimeMs(row?.end_timestamp ?? row?.END_TIMESTAMP)
  if (end != null) return end
  const start = rowTimeMs(row?.timestamp ?? row?.TIMESTAMP)
  const durNs = row?.duration_ns ?? row?.DURATION_NS
  if (start != null && durNs != null && Number.isFinite(Number(durNs))) {
    return start + Number(durNs) / 1e6
  }
  return start
}

/** Build forest of span rows keyed by parent_span_id (empty / zeros → root). */
function buildSpanForest(items) {
  const list = Array.isArray(items) ? items : []
  const idSet = new Set()
  for (const row of list) {
    const id = spanKey(row)
    if (id) idSet.add(id)
  }
  const byParent = new Map()
  for (const row of list) {
    const rawParent = row?.parent_span_id ?? row?.PARENT_SPAN_ID
    let pk = ''
    if (!isEmptyParentSpanId(rawParent)) {
      const p = String(rawParent).trim()
      pk = idSet.has(p) ? p : ''
    }
    if (!byParent.has(pk)) byParent.set(pk, [])
    byParent.get(pk).push(row)
  }
  for (const arr of byParent.values()) {
    arr.sort((a, b) => {
      const ta = rowTimeMs(a?.timestamp ?? a?.TIMESTAMP) ?? 0
      const tb = rowTimeMs(b?.timestamp ?? b?.TIMESTAMP) ?? 0
      return ta - tb
    })
  }
  return byParent
}

function strHue(s) {
  if (!s) return 200
  let h = 5381
  const t = String(s)
  for (let i = 0; i < t.length; i++) h = ((h << 5) + h) ^ t.charCodeAt(i)
  return Math.abs(h) % 360
}

function serviceBarHsl(serviceName, dark) {
  const hue = strHue(serviceName || 'svc')
  return dark
    ? `hsl(${hue} 72% 42%)`
    : `hsl(${hue} 70% 46%)`
}

function serviceStripeHsl(serviceName, dark) {
  const hue = strHue(serviceName || 'svc')
  return dark ? `hsl(${hue} 55% 32%)` : `hsl(${hue} 65% 38%)`
}

function maxDepth(byParent, parentKeyId) {
  const ch = byParent.get(parentKeyId) || []
  if (!ch.length) return 0
  let m = 0
  for (const c of ch) {
    const id = spanKey(c) || '_'
    m = Math.max(m, 1 + maxDepth(byParent, id))
  }
  return m
}

function spanStatusError(row) {
  const sc = row?.status_code ?? row?.STATUS_CODE
  if (sc == null || sc === '') return false
  const n = Number(sc)
  return Number.isFinite(n) && n !== 0 && n !== 1
}

function formatDurationMs(ms) {
  if (ms == null || Number.isNaN(ms)) return '—'
  if (Math.abs(ms) < 0.01) return `${(ms * 1000).toFixed(0)} μs`
  if (ms < 1000) return `${ms.toFixed(2)} ms`
  return `${(ms / 1000).toFixed(2)} s`
}

function formatJsonField(val) {
  if (val == null) return '—'
  if (typeof val === 'string') {
    const t = val.trim()
    if ((t.startsWith('{') && t.endsWith('}')) || (t.startsWith('[') && t.endsWith(']'))) {
      try {
        return JSON.stringify(JSON.parse(t), null, 2)
      } catch {
        return val
      }
    }
    return val
  }
  if (typeof val === 'object') {
    try {
      return JSON.stringify(val, null, 2)
    } catch {
      return String(val)
    }
  }
  return String(val)
}

/** Match + ancestors for tree filter (Grafana-style). */
function visibleSpanIdsForFilter(items, filterTrim) {
  if (!filterTrim?.trim()) return null
  const f = filterTrim.toLowerCase()
  const idToRow = new Map()
  for (const row of items) {
    const id = spanKey(row)
    if (id) idToRow.set(id, row)
  }
  const need = new Set()
  for (const row of items) {
    const id = spanKey(row)
    const nm = `${row?.name ?? row?.NAME ?? ''} ${row?.service_name ?? row?.SERVICE_NAME ?? ''}`.toLowerCase()
    if (!nm.includes(f)) continue
    let cur = id
    for (let guard = 0; guard < 128 && cur; guard++) {
      need.add(cur)
      const r = idToRow.get(cur)
      if (!r) break
      const p = String(r?.parent_span_id ?? r?.PARENT_SPAN_ID ?? '').trim()
      if (!p || /^0+$/i.test(p)) break
      if (!idToRow.has(p)) break
      cur = p
    }
    if (!id && row) need.add(`__row_${items.indexOf(row)}`)
  }
  return need
}

function useIsDark() {
  const [dark, setDark] = useState(
    () => typeof document !== 'undefined' && document.documentElement.classList.contains('dark'),
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

function WaterfallRows({
  byParent,
  parentKeyId,
  depth,
  traceMinMs,
  rangeMs,
  collapsed,
  toggleCollapsed,
  selected,
  onSelect,
  visibleIds,
  dark,
}) {
  if (parentKeyId && collapsed.has(parentKeyId)) return null
  const children = byParent.get(parentKeyId) || []
  if (!children.length) return null

  return (
    <>
      {children.map((row, idx) => {
        const sid = spanKey(row) || `orphan-${depth}-${idx}`
        if (visibleIds && sid && !visibleIds.has(sid)) return null

        const svc = String(row?.service_name ?? row?.SERVICE_NAME ?? '—')
        const name = String(row?.name ?? row?.NAME ?? '—')
        const startMs = rowTimeMs(row?.timestamp ?? row?.TIMESTAMP)
        const endMs = rowEndMs(row)
        const durMs =
          startMs != null && endMs != null ? Math.max(0, endMs - startMs) : null
        const leftPct = startMs != null && rangeMs > 0 ? ((startMs - traceMinMs) / rangeMs) * 100 : 0
        const widthPct =
          startMs != null && endMs != null && rangeMs > 0
            ? Math.max(0.35, ((endMs - startMs) / rangeMs) * 100)
            : 2
        const bar = serviceBarHsl(svc, dark)
        const stripe = serviceStripeHsl(svc, dark)
        const err = spanStatusError(row)
        const childCount = (byParent.get(sid) || []).length
        const isCollapsed = collapsed.has(sid)
        const active = selected && spanKey(selected) === spanKey(row) && spanKey(row)

        return (
          <div key={sid} className="border-b border-border/50">
            <div
              className={cn(
                'grid grid-cols-1 gap-0 sm:grid-cols-[minmax(0,1fr)_minmax(140px,38%)]',
                active && 'bg-accent/40',
              )}
            >
              <div
                className={cn(
                  'flex min-h-[30px] min-w-0 items-center gap-1 py-0.5 pr-1 sm:border-r sm:border-border/60',
                )}
                style={{ paddingLeft: 6 + depth * 14 }}
              >
                {childCount > 0 ? (
                  <button
                    type="button"
                    className="inline-flex shrink-0 cursor-pointer rounded p-0.5 text-muted-foreground hover:bg-muted hover:text-foreground"
                    onClick={() => toggleCollapsed(sid)}
                    aria-expanded={!isCollapsed}
                  >
                    {isCollapsed ? (
                      <ChevronRight className="h-3.5 w-3.5" />
                    ) : (
                      <ChevronDown className="h-3.5 w-3.5" />
                    )}
                  </button>
                ) : (
                  <span className="inline-block w-5 shrink-0" />
                )}
                <span
                  className="h-4 w-1 shrink-0 rounded-sm"
                  style={{ background: stripe }}
                  title={svc}
                />
                <button
                  type="button"
                  onClick={() => onSelect(row)}
                  className="min-w-0 flex-1 truncate text-left text-[11px] hover:underline"
                >
                  <span className="font-semibold text-foreground">{svc}</span>
                  <span className="text-muted-foreground"> {name}</span>
                </button>
                {err ? <AlertCircle className="h-3.5 w-3.5 shrink-0 text-rose-600 dark:text-rose-400" /> : null}
              </div>
              <div className="relative min-h-[30px] border-t border-border/40 bg-muted/15 sm:border-t-0">
                <div className="absolute inset-y-0 left-0 right-0">
                  <div
                    className="absolute top-1/2 h-[10px] -translate-y-1/2 rounded-sm opacity-95 shadow-sm"
                    style={{
                      left: `${Math.min(100, Math.max(0, leftPct))}%`,
                      width: `${Math.min(100 - leftPct, widthPct)}%`,
                      minWidth: 3,
                      background: bar,
                    }}
                  />
                </div>
                <span className="absolute right-1 top-1/2 z-[1] -translate-y-1/2 rounded bg-card/90 px-0.5 font-mono text-[10px] tabular-nums text-muted-foreground">
                  {durMs != null ? formatDurationMs(durMs) : '—'}
                </span>
              </div>
            </div>
            <WaterfallRows
              byParent={byParent}
              parentKeyId={sid}
              depth={depth + 1}
              traceMinMs={traceMinMs}
              rangeMs={rangeMs}
              collapsed={collapsed}
              toggleCollapsed={toggleCollapsed}
              selected={selected}
              onSelect={onSelect}
              visibleIds={visibleIds}
              dark={dark}
            />
          </div>
        )
      })}
    </>
  )
}

function TraceMinimap({ items, traceMinMs, rangeMs, dark }) {
  const list = Array.isArray(items) ? items : []
  if (!list.length || rangeMs <= 0) {
    return <div className="h-6 rounded-sm border border-border bg-muted/40" />
  }
  return (
    <div className="relative h-6 overflow-hidden rounded-sm border border-border bg-muted/30">
      {list.map((row, i) => {
        const sid = spanKey(row) || `m-${i}`
        const svc = String(row?.service_name ?? row?.SERVICE_NAME ?? '')
        const startMs = rowTimeMs(row?.timestamp ?? row?.TIMESTAMP)
        const endMs = rowEndMs(row)
        if (startMs == null || endMs == null) return null
        const left = ((startMs - traceMinMs) / rangeMs) * 100
        const w = Math.max(0.2, ((endMs - startMs) / rangeMs) * 100)
        return (
          <div
            key={sid}
            className="absolute top-1 bottom-1 rounded-sm opacity-80"
            style={{
              left: `${left}%`,
              width: `${w}%`,
              minWidth: 2,
              background: serviceBarHsl(svc, dark),
            }}
          />
        )
      })}
    </div>
  )
}

export default function OTLPTraceModal({ modal, timeZone, onClose }) {
  const { resolved: locale } = useLocale()
  const { modalKey, traceId, loading, items, error } = modal
  const byParent = useMemo(() => buildSpanForest(items), [items])
  const [selected, setSelected] = useState(null)
  const [collapsed, setCollapsed] = useState(() => new Set())
  const [findText, setFindText] = useState('')
  const dark = useIsDark()

  const visibleIds = useMemo(() => visibleSpanIdsForFilter(items || [], findText), [items, findText])

  useEffect(() => {
    if (!Array.isArray(items) || items.length === 0) {
      setSelected(null)
      return
    }
    const bp = buildSpanForest(items)
    const r = bp.get('') || []
    setSelected(r[0] || items[0] || null)
  }, [items, modalKey])

  useEffect(() => {
    setCollapsed(new Set())
    setFindText('')
  }, [modalKey])

  const stats = useMemo(() => {
    const list = Array.isArray(items) ? items : []
    if (!list.length) return null
    let minT = Infinity
    let maxT = -Infinity
    const services = new Set()
    for (const row of list) {
      const t0 = rowTimeMs(row?.timestamp ?? row?.TIMESTAMP)
      const t1 = rowEndMs(row)
      if (t0 != null) {
        minT = Math.min(minT, t0)
        maxT = Math.max(maxT, t1 ?? t0)
      }
      const svc = row?.service_name ?? row?.SERVICE_NAME
      if (svc != null && String(svc).trim() !== '') services.add(String(svc))
    }
    if (!Number.isFinite(minT)) minT = 0
    if (!Number.isFinite(maxT)) maxT = minT + 1
    const rangeMs = Math.max(1, maxT - minT)
    const depth = maxDepth(byParent, '')
    return {
      minMs: minT,
      maxMs: maxT,
      rangeMs,
      services: services.size,
      depth,
      spanCount: list.length,
    }
  }, [items, byParent])

  const toggleCollapsed = useCallback((sid) => {
    setCollapsed((prev) => {
      const next = new Set(prev)
      if (next.has(sid)) next.delete(sid)
      else next.add(sid)
      return next
    })
  }, [])

  const onSelect = useCallback((row) => {
    setSelected(row)
  }, [])

  const first = (byParent.get('') || [])[0] || (Array.isArray(items) && items[0]) || null
  const effectiveSelected = selected || first

  const shortTrace =
    traceId && String(traceId).length > 24
      ? `${String(traceId).slice(0, 12)}…${String(traceId).slice(-8)}`
      : traceId || 'trace'

  const formatTs = (ms) => {
    if (ms == null || !Number.isFinite(ms)) return '—'
    try {
      const d = new Date(ms)
      const opts = {
        year: 'numeric',
        month: 'numeric',
        day: 'numeric',
        hour: 'numeric',
        minute: 'numeric',
        second: 'numeric',
        fractionalSecondDigits: 3,
      }
      if (timeZone && timeZone !== 'local') opts.timeZone = timeZone
      return new Intl.DateTimeFormat(locale, opts).format(d)
    } catch {
      return String(ms)
    }
  }

  const ticks = [0, 0.25, 0.5, 0.75, 1]

  return (
    <FloatingWindow
      open
      onClose={onClose}
      id={`otlp:${modalKey || traceId}`}
      title={
        <span className="truncate font-mono text-sm">
          Trace <span className="text-muted-foreground">{shortTrace}</span>
          {!loading && items?.length != null ? (
            <span className="ml-2 font-sans text-xs text-muted-foreground">({items.length} spans)</span>
          ) : null}
        </span>
      }
      headerActions={null}
      defaultWidth={Math.min(1100, typeof window !== 'undefined' ? window.innerWidth - 48 : 1100)}
      defaultHeight={Math.min(Math.round((typeof window !== 'undefined' ? window.innerHeight : 800) * 0.82), 780)}
      minWidth={640}
      minHeight={400}
      className="flex flex-col"
    >
      <div className="flex min-h-0 flex-1 flex-col">
        {loading && (
          <div className="flex items-center justify-center py-6 text-xs text-muted-foreground">Loading trace…</div>
        )}
        {error && (
          <Alert variant="destructive" className="m-3 py-2">
            <AlertDescription className="text-xs">{error}</AlertDescription>
          </Alert>
        )}
        {!loading && !error && stats && (
          <div className="flex min-h-0 min-w-0 flex-1 flex-col gap-2 px-3 pb-2 pt-1">
            <div className="grid grid-cols-2 gap-x-3 gap-y-1.5 rounded-sm border border-border bg-muted/25 px-3 py-2 text-[11px] sm:grid-cols-3 lg:grid-cols-6">
              <div>
                <div className="text-[10px] font-medium uppercase tracking-wide text-muted-foreground">Trace start</div>
                <div className="font-mono text-foreground">{formatTs(stats.minMs)}</div>
              </div>
              <div>
                <div className="text-[10px] font-medium uppercase tracking-wide text-muted-foreground">Duration</div>
                <div className="font-mono text-foreground">{formatDurationMs(stats.rangeMs)}</div>
              </div>
              <div>
                <div className="text-[10px] font-medium uppercase tracking-wide text-muted-foreground">Services</div>
                <div className="font-mono text-foreground">{stats.services}</div>
              </div>
              <div>
                <div className="text-[10px] font-medium uppercase tracking-wide text-muted-foreground">Depth</div>
                <div className="font-mono text-foreground">{stats.depth}</div>
              </div>
              <div>
                <div className="text-[10px] font-medium uppercase tracking-wide text-muted-foreground">Spans</div>
                <div className="font-mono text-foreground">{stats.spanCount}</div>
              </div>
              <div className="min-w-0 sm:col-span-1">
                <div className="text-[10px] font-medium uppercase tracking-wide text-muted-foreground">Trace ID</div>
                <div className="truncate font-mono text-[10px] text-foreground" title={String(traceId)}>
                  {String(traceId || '—')}
                </div>
              </div>
            </div>

            <Input
              value={findText}
              onChange={(e) => setFindText(e.target.value)}
              placeholder="Find spans (name, service)…"
              className="h-8 max-w-md text-xs"
            />

            <div className="space-y-1">
              <div className="text-[10px] font-medium uppercase tracking-wide text-muted-foreground">Minimap</div>
              <TraceMinimap items={items} traceMinMs={stats.minMs} rangeMs={stats.rangeMs} dark={dark} />
            </div>

            <div className="grid min-h-0 flex-1 grid-cols-1 flex-col overflow-hidden rounded-sm border border-border bg-card sm:grid-cols-[minmax(0,1fr)_minmax(140px,38%)]">
              <div className="hidden border-b border-border bg-muted/40 px-2 py-1 text-[10px] font-medium uppercase tracking-wide text-muted-foreground sm:col-span-2 sm:grid sm:grid-cols-[minmax(0,1fr)_minmax(140px,38%)]">
                <span>Service & operation</span>
                <span className="border-l border-border/60 pl-2">Timeline</span>
              </div>
              <div className="relative hidden h-6 border-b border-border/80 bg-muted/20 sm:col-span-2 sm:block">
                {ticks.map((t) => (
                  <span
                    key={t}
                    className="absolute top-1 font-mono text-[9px] text-muted-foreground"
                    style={{ left: `calc(${t * 100}% + 4px)` }}
                  >
                    {formatDurationMs(t * stats.rangeMs)}
                  </span>
                ))}
              </div>
              <ScrollArea className="min-h-[220px] sm:col-span-2">
                <div className="min-w-[520px] pr-2">
                  {visibleIds && visibleIds.size === 0 ? (
                    <p className="p-4 text-center text-xs text-muted-foreground">No spans match the filter.</p>
                  ) : (
                    <WaterfallRows
                      byParent={byParent}
                      parentKeyId=""
                      depth={0}
                      traceMinMs={stats.minMs}
                      rangeMs={stats.rangeMs}
                      collapsed={collapsed}
                      toggleCollapsed={toggleCollapsed}
                      selected={effectiveSelected}
                      onSelect={onSelect}
                      visibleIds={visibleIds}
                      dark={dark}
                    />
                  )}
                </div>
              </ScrollArea>
            </div>

            <Tabs defaultValue="details" className="flex min-h-0 shrink-0 flex-col border-t border-border pt-1">
              <TabsList variant="line" className="h-8 w-full justify-start">
                <TabsTrigger value="details" className="text-[11px]">
                  Span details
                </TabsTrigger>
                <TabsTrigger value="json" className="text-[11px]">
                  JSON
                </TabsTrigger>
              </TabsList>
              <TabsContent value="details" className="mt-0 max-h-[200px]">
                <ScrollArea className="h-[200px]">
                  {!effectiveSelected ? (
                    <p className="p-2 text-xs text-muted-foreground">Select a span.</p>
                  ) : (
                    <dl className="grid gap-1 p-2 font-mono text-[11px]">
                      {[
                        ['trace_id', effectiveSelected.trace_id ?? effectiveSelected.TRACE_ID],
                        ['span_id', effectiveSelected.span_id ?? effectiveSelected.SPAN_ID],
                        ['parent_span_id', effectiveSelected.parent_span_id ?? effectiveSelected.PARENT_SPAN_ID],
                        ['name', effectiveSelected.name ?? effectiveSelected.NAME],
                        ['service_name', effectiveSelected.service_name ?? effectiveSelected.SERVICE_NAME],
                        ['kind', effectiveSelected.kind ?? effectiveSelected.KIND],
                        ['status_code', effectiveSelected.status_code ?? effectiveSelected.STATUS_CODE],
                        ['status_message', effectiveSelected.status_message ?? effectiveSelected.STATUS_MESSAGE],
                        ['duration_ns', effectiveSelected.duration_ns ?? effectiveSelected.DURATION_NS],
                        ['timestamp', String(effectiveSelected.timestamp ?? effectiveSelected.TIMESTAMP ?? '')],
                        ['end_timestamp', String(effectiveSelected.end_timestamp ?? effectiveSelected.END_TIMESTAMP ?? '')],
                      ].map(([k, v]) => (
                        <div key={k} className="grid grid-cols-[8rem_1fr] gap-x-2 border-b border-border/30 py-0.5">
                          <dt className="text-muted-foreground">{k}</dt>
                          <dd className="min-w-0 break-all text-foreground">{v == null || v === '' ? '—' : String(v)}</dd>
                        </div>
                      ))}
                    </dl>
                  )}
                </ScrollArea>
              </TabsContent>
              <TabsContent value="json" className="mt-0 max-h-[200px]">
                <ScrollArea className="h-[200px]">
                  <pre className="whitespace-pre-wrap break-all p-2 font-mono text-[10px] text-foreground">
                    {effectiveSelected
                      ? [
                          '--- span_attrs ---',
                          formatJsonField(effectiveSelected.span_attrs ?? effectiveSelected.SPAN_ATTRS),
                          '\n--- resource_attrs ---',
                          formatJsonField(effectiveSelected.resource_attrs ?? effectiveSelected.RESOURCE_ATTRS),
                          '\n--- raw ---',
                          formatJsonField(effectiveSelected.raw ?? effectiveSelected.RAW),
                        ].join('\n')
                      : '—'}
                  </pre>
                </ScrollArea>
              </TabsContent>
            </Tabs>
          </div>
        )}
      </div>
    </FloatingWindow>
  )
}
