// @ts-nocheck — dashboard modals; typed when refactored with shared types
import { useMemo, useState } from 'react'
import { FloatingWindow } from '@/components/ui/floating-window'
import { ScrollArea } from '@/components/ui/scroll-area'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { cn } from '@/lib/utils'
import OTLPLogRowModal from './OTLPLogRowModal'
import { useLocale } from '@/components/locale/locale-provider'

/** OpenTelemetry SeverityNumber ranges (stable mapping when severity_text is empty). */
function severityFromNumber(n) {
  if (n == null || Number.isNaN(n)) return ''
  const v = Number(n)
  if (v >= 21) return 'FATAL'
  if (v >= 17) return 'ERROR'
  if (v >= 13) return 'WARN'
  if (v >= 9) return 'INFO'
  if (v >= 5) return 'DEBUG'
  if (v >= 1) return 'TRACE'
  return ''
}

function resolvedSeverityLabel(row) {
  const text = row?.severity_text ?? row?.SEVERITY_TEXT
  if (text != null && String(text).trim() !== '') return String(text).trim()
  const num = row?.severity_number ?? row?.SEVERITY_NUMBER
  if (num != null && num !== '') return severityFromNumber(Number(num)) || `n=${num}`
  return '—'
}

function severityBadgeClass(label) {
  const u = String(label || '').toUpperCase().trim()
  if (u === 'ERROR' || u === 'FATAL') {
    return 'border-rose-600/50 bg-rose-600/90 text-white dark:bg-rose-700/95'
  }
  if (u === 'WARN' || u === 'WARNING') {
    return 'border-amber-600/40 bg-amber-500/90 text-amber-950 dark:bg-amber-600/85 dark:text-amber-50'
  }
  if (u === 'INFO') {
    return 'border-sky-600/40 bg-sky-600/85 text-white dark:bg-sky-700/90'
  }
  if (u === 'DEBUG' || u === 'TRACE' || u === 'VERBOSE') {
    return 'border-border bg-muted text-muted-foreground'
  }
  return 'border-border bg-card text-foreground'
}

function rowTimestampMs(row) {
  const v = row?.timestamp ?? row?.TIMESTAMP
  if (v == null) return 0
  const d = v instanceof Date ? v : new Date(v)
  const t = d.getTime()
  return Number.isNaN(t) ? 0 : t
}

export default function OTLPLogsTraceModal({ modal, timeZone, onClose }) {
  const { resolved: locale } = useLocale()
  const { modalKey, traceId, loading, items, error } = modal
  const [detailModal, setDetailModal] = useState(null)

  const sorted = useMemo(() => {
    const list = Array.isArray(items) ? [...items] : []
    list.sort((a, b) => rowTimestampMs(a) - rowTimestampMs(b))
    return list
  }, [items])

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

  const shortTrace =
    traceId && String(traceId).length > 24
      ? `${String(traceId).slice(0, 12)}…${String(traceId).slice(-8)}`
      : traceId || 'trace'

  const baseModalKey = modalKey || traceId || 'otlplogs'

  const handleCloseTrace = () => {
    setDetailModal(null)
    onClose()
  }

  return (
    <>
      <FloatingWindow
      open
      onClose={handleCloseTrace}
      id={`otlplogs:${modalKey || traceId}`}
      title={
        <span className="truncate font-mono text-sm">
          OTLP logs · trace{' '}
          <span className="text-muted-foreground">{shortTrace}</span>
          {!loading && sorted.length != null ? (
            <span className="ml-2 font-sans text-xs text-muted-foreground">({sorted.length} entries)</span>
          ) : null}
        </span>
      }
      headerActions={null}
      defaultWidth={Math.min(920, typeof window !== 'undefined' ? window.innerWidth - 72 : 920)}
      defaultHeight={Math.min(Math.round((typeof window !== 'undefined' ? window.innerHeight : 800) * 0.75), 640)}
      minWidth={520}
      minHeight={320}
      className="flex flex-col"
    >
      <div className="flex min-h-0 flex-1 flex-col">
        {loading && (
          <div className="flex items-center justify-center py-6 text-xs text-muted-foreground">Loading logs…</div>
        )}
        {error && (
          <Alert variant="destructive" className="m-3 py-2">
            <AlertDescription className="text-xs">{error}</AlertDescription>
          </Alert>
        )}
        {!loading && !error && (
          <ScrollArea className="min-h-0 flex-1 border-t border-border">
            <table className="w-full border-separate border-spacing-0 text-[11px] text-foreground">
              <thead className="sticky top-0 z-10 bg-card">
                <tr>
                  <th className="border-b border-border px-2 py-1.5 text-left font-medium text-muted-foreground">Time</th>
                  <th className="border-b border-border px-2 py-1.5 text-left font-medium text-muted-foreground">Level</th>
                  <th className="border-b border-border px-2 py-1.5 text-left font-medium text-muted-foreground">Service</th>
                  <th className="border-b border-border px-2 py-1.5 text-left font-medium text-muted-foreground">Body</th>
                </tr>
              </thead>
              <tbody>
                {sorted.length === 0 ? (
                  <tr>
                    <td colSpan={4} className="px-2 py-6 text-center text-muted-foreground">
                      No log rows for this trace_id.
                    </td>
                  </tr>
                ) : (
                  sorted.map((row, idx) => {
                    const sev = resolvedSeverityLabel(row)
                    const badgeCls = severityBadgeClass(sev)
                    const svc = row?.service_name ?? row?.SERVICE_NAME ?? '—'
                    const body = row?.body ?? row?.BODY ?? ''
                    const ts = row?.timestamp ?? row?.TIMESTAMP
                    const entryKey = `${baseModalKey}:row:${rowTimestampMs(row)}:${idx}`
                    return (
                      <tr
                        key={entryKey}
                        role="button"
                        tabIndex={0}
                        title="Open full log record"
                        onClick={() =>
                          setDetailModal({
                            modalKey: entryKey,
                            row: { ...row },
                          })
                        }
                        onKeyDown={(e) => {
                          if (e.key === 'Enter' || e.key === ' ') {
                            e.preventDefault()
                            setDetailModal({
                              modalKey: entryKey,
                              row: { ...row },
                            })
                          }
                        }}
                        className="cursor-pointer border-b border-border/40 hover:bg-accent/40"
                      >
                        <td className="whitespace-nowrap px-2 py-1 align-top font-mono text-muted-foreground">
                          {formatTs(ts)}
                        </td>
                        <td className="px-2 py-1 align-top">
                          <span
                            className={cn(
                              'inline-block rounded-none border px-1.5 py-0.5 text-[10px] font-semibold uppercase tracking-wide',
                              badgeCls,
                            )}
                          >
                            {sev}
                          </span>
                        </td>
                        <td className="max-w-[140px] truncate px-2 py-1 align-top font-mono text-muted-foreground" title={String(svc)}>
                          {String(svc)}
                        </td>
                        <td className="max-w-0 px-2 py-1 align-top break-words text-foreground">{String(body)}</td>
                      </tr>
                    )
                  })
                )}
              </tbody>
            </table>
          </ScrollArea>
        )}
      </div>
    </FloatingWindow>
      {detailModal ? (
        <OTLPLogRowModal
          modal={detailModal}
          timeZone={timeZone}
          onClose={() => setDetailModal(null)}
        />
      ) : null}
    </>
  )
}
