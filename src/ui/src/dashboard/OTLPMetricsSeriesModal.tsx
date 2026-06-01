// @ts-nocheck — dashboard modals; typed when refactored with shared types
import { useEffect, useMemo, useRef, useState } from 'react'
import { FloatingWindow } from '@/components/ui/floating-window'
import { ScrollArea } from '@/components/ui/scroll-area'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Alert, AlertDescription } from '@/components/ui/alert'
import 'uplot/dist/uPlot.min.css'
import {
  buildMetricChartPayload,
  chartKindCaption,
  dominantMetricKind,
  normalizeKind,
  mountOtlpMetricSeriesChart,
  normalizeOtlpMetricChartType,
  rowTimestampMs,
} from './otlpMetricsSeriesChart'
import { useLocale } from '@/components/locale/locale-provider'

function formatValue(row) {
  const vd = row?.value_double ?? row?.VALUE_DOUBLE
  const vi = row?.value_int ?? row?.VALUE_INT
  if (vd != null && vd !== '') return String(vd)
  if (vi != null && vi !== '') return String(vi)
  return '—'
}

export default function OTLPMetricsSeriesModal({ modal, timeZone, onClose }) {
  const { resolved: locale } = useLocale()
  const { modalKey, metricName, loading, items, error } = modal
  const chartElRef = useRef(null)
  const disposeRef = useRef(null)
  const [chartType, setChartType] = useState(() => normalizeOtlpMetricChartType('area'))

  const sorted = useMemo(() => {
    const list = Array.isArray(items) ? [...items] : []
    list.sort((a, b) => rowTimestampMs(a) - rowTimestampMs(b))
    return list
  }, [items])

  const kind = useMemo(() => dominantMetricKind(sorted), [sorted])
  const mixedTypes = useMemo(() => {
    const s = new Set(sorted.map((r) => normalizeKind(r?.type ?? r?.TYPE)))
    return s.size > 1
  }, [sorted])

  const chartPayload = useMemo(() => buildMetricChartPayload(sorted, kind), [sorted, kind])

  useEffect(() => {
    if (disposeRef.current) {
      disposeRef.current()
      disposeRef.current = null
    }
    if (loading || error || !chartPayload || !chartElRef.current) return

    const el = chartElRef.current
    disposeRef.current = mountOtlpMetricSeriesChart(el, chartPayload, {
      chartType,
    })
    return () => {
      if (disposeRef.current) {
        disposeRef.current()
        disposeRef.current = null
      }
    }
  }, [loading, error, chartPayload, chartType])

  const formatTs = (val) => {
    if (!val) return '—'
    try {
      const d = val instanceof Date ? val : new Date(val)
      if (Number.isNaN(d.getTime())) return String(val)
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
      return String(val)
    }
  }

  const titleName = metricName && String(metricName).length > 48 ? `${String(metricName).slice(0, 46)}…` : metricName || 'metric'

  return (
    <FloatingWindow
      open
      onClose={onClose}
      id={`otlpmetric:${modalKey || metricName}`}
      title={
        <span className="truncate font-mono text-sm">
          OTLP metrics ·{' '}
          <span className="text-muted-foreground">{titleName}</span>
          {!loading && sorted.length != null ? (
            <span className="ml-2 font-sans text-xs text-muted-foreground">({sorted.length} points)</span>
          ) : null}
        </span>
      }
      headerActions={null}
      defaultWidth={Math.min(900, typeof window !== 'undefined' ? window.innerWidth - 72 : 900)}
      defaultHeight={Math.min(Math.round((typeof window !== 'undefined' ? window.innerHeight : 800) * 0.78), 680)}
      minWidth={520}
      minHeight={320}
      className="flex flex-col"
    >
      <div className="flex min-h-0 flex-1 flex-col">
        {loading && (
          <div className="flex items-center justify-center py-6 text-xs text-muted-foreground">Loading series…</div>
        )}
        {error && (
          <Alert variant="destructive" className="m-3 py-2">
            <AlertDescription className="text-xs">{error}</AlertDescription>
          </Alert>
        )}
        {!loading && !error && (
          <>
            <div className="shrink-0 border-b border-border px-3 py-2">
              <div className="flex flex-wrap items-center gap-2 text-[11px] text-muted-foreground">
                <Select
                  value={chartType}
                  onValueChange={(v) => setChartType(normalizeOtlpMetricChartType(v))}
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
                <span className="rounded-sm border border-border bg-muted/40 px-1.5 py-0.5 font-mono text-[10px] uppercase text-foreground">
                  {kind}
                </span>
                <span>{chartKindCaption(kind)}</span>
                {mixedTypes ? (
                  <span className="text-amber-600 dark:text-amber-400">· Mixed type column values — chart uses majority kind</span>
                ) : null}
              </div>
              {chartPayload ? (
                <div ref={chartElRef} className="mt-2 h-[240px] w-full min-h-[200px]" />
              ) : (
                <div className="mt-2 rounded-sm border border-dashed border-border py-6 text-center text-[11px] text-muted-foreground">
                  No numeric points to chart (check value_double / value_int and timestamp).
                </div>
              )}
            </div>
            <ScrollArea className="min-h-0 flex-1 border-t border-border">
              <table className="w-full border-separate border-spacing-0 text-[11px] text-foreground">
                <thead className="sticky top-0 z-10 bg-card">
                  <tr>
                    <th className="border-b border-border px-2 py-1.5 text-left font-medium text-muted-foreground">Time</th>
                    <th className="border-b border-border px-2 py-1.5 text-left font-medium text-muted-foreground">Type</th>
                    <th className="border-b border-border px-2 py-1.5 text-right font-medium text-muted-foreground">Value</th>
                    <th className="border-b border-border px-2 py-1.5 text-left font-medium text-muted-foreground">Unit</th>
                    <th className="border-b border-border px-2 py-1.5 text-left font-medium text-muted-foreground">Service</th>
                  </tr>
                </thead>
                <tbody>
                  {sorted.length === 0 ? (
                    <tr>
                      <td colSpan={5} className="px-2 py-6 text-center text-muted-foreground">
                        No metric points for this name in the selected time range.
                      </td>
                    </tr>
                  ) : (
                    sorted.map((row, idx) => {
                      const typ = row?.type ?? row?.TYPE ?? '—'
                      const unit = row?.unit ?? row?.UNIT ?? ''
                      const svc = row?.service_name ?? row?.SERVICE_NAME ?? '—'
                      const ts = row?.timestamp ?? row?.TIMESTAMP
                      return (
                        <tr key={idx} className="border-b border-border/40 hover:bg-accent/25">
                          <td className="whitespace-nowrap px-2 py-1 align-top font-mono text-muted-foreground">
                            {formatTs(ts)}
                          </td>
                          <td className="px-2 py-1 align-top font-mono text-foreground">{String(typ)}</td>
                          <td className="px-2 py-1 text-right align-top font-mono tabular-nums text-foreground">
                            {formatValue(row)}
                          </td>
                          <td className="max-w-[100px] truncate px-2 py-1 align-top text-muted-foreground" title={String(unit)}>
                            {String(unit) || '—'}
                          </td>
                          <td className="max-w-[180px] truncate px-2 py-1 align-top text-muted-foreground" title={String(svc)}>
                            {String(svc)}
                          </td>
                        </tr>
                      )
                    })
                  )}
                </tbody>
              </table>
            </ScrollArea>
          </>
        )}
      </div>
    </FloatingWindow>
  )
}
