// @ts-nocheck
import React from 'react'
import { ChevronDown } from 'lucide-react'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { ScrollArea } from '@/components/ui/scroll-area'
import { cn } from '@/lib/utils'

const LABELS = {
  session_id: 'Call-ID',
  status: 'Status',
  status_code: 'Status code',
  ringing_seconds: 'Ringing time',
  call_duration_seconds: 'Call duration',
  session_duration_seconds: 'Session duration',
  session_request_delay_ms: 'Session request delay',
  session_setup_delay_ms: 'Session setup delay',
  failed_session_setup_delay_ms: 'Failed setup delay',
  session_disconnect_delay_ms: 'Session disconnect delay',
  codecs: 'Codecs',
  uac: 'UAC',
  uas: 'UAS',
  source: 'Source',
  destination: 'Destination',
  caller: 'Caller',
  callee: 'Callee',
  message_count: 'Messages',
  last_bad_reply: 'Last bad reply',
  first_seen: 'First seen',
  last_seen: 'Last seen',
  from_party: 'From',
  ruri_party: 'R-URI',
}

const PIE_COLORS = [
  '#e53935', '#1e88e5', '#43a047', '#fb8c00', '#8e24aa',
  '#00acc1', '#fdd835', '#6d4c41', '#78909c', '#d81b60',
]

const TILE_DEFS = [
  { key: 'ringing_seconds', label: 'Ringing', suffix: 's', isSec: true },
  { key: 'session_duration_seconds', label: 'Session duration', suffix: 's', isSec: true },
  { key: 'session_request_delay_ms', label: 'SRD', suffix: 'ms', isSec: false },
  { key: 'session_setup_delay_ms', label: 'Setup delay', suffix: 'ms', isSec: false },
  { key: 'failed_session_setup_delay_ms', label: 'Failed setup', suffix: 'ms', isSec: false },
  { key: 'session_disconnect_delay_ms', label: 'Disconnect delay', suffix: 'ms', isSec: false },
]

function formatValue(k, v) {
  if (v == null || v === '') return '—'
  if (typeof v === 'number') {
    if (k.endsWith('_seconds')) return `${v} s`
    if (k.endsWith('_ms')) return `${v} ms`
    return String(v)
  }
  return String(v)
}

// STATUS_COLOR classifies the text produced by callinfo_compute.go
// (homerStatusText) into four families and returns a Tailwind class set
// that paints a small tinted badge. Works in both light and dark mode.
const STATUS_COLOR = {
  // Success — green
  Connected: 'bg-green-500/15 text-green-700 dark:text-green-300 border border-green-500/30',
  Finished: 'bg-green-500/15 text-green-700 dark:text-green-300 border border-green-500/30',
  Forwarded: 'bg-green-500/15 text-green-700 dark:text-green-300 border border-green-500/30',

  // In-progress / auth challenge — yellow
  Invite: 'bg-yellow-500/15 text-yellow-700 dark:text-yellow-300 border border-yellow-500/30',
  '183 Early': 'bg-yellow-500/15 text-yellow-700 dark:text-yellow-300 border border-yellow-500/30',
  Ringing: 'bg-yellow-500/15 text-yellow-700 dark:text-yellow-300 border border-yellow-500/30',
  Unauth: 'bg-yellow-500/15 text-yellow-700 dark:text-yellow-300 border border-yellow-500/30',

  // Soft failures — orange
  Busy: 'bg-orange-500/15 text-orange-700 dark:text-orange-300 border border-orange-500/30',
  Cancelled: 'bg-orange-500/15 text-orange-700 dark:text-orange-300 border border-orange-500/30',

  // Hard failures — red
  Rejected: 'bg-red-500/15 text-red-700 dark:text-red-300 border border-red-500/30',
  'Server error': 'bg-red-500/15 text-red-700 dark:text-red-300 border border-red-500/30',
  Timeout: 'bg-red-500/15 text-red-700 dark:text-red-300 border border-red-500/30',
  'Global failure': 'bg-red-500/15 text-red-700 dark:text-red-300 border border-red-500/30',
}

function statusClass(status) {
  if (status == null) return 'bg-muted text-muted-foreground border border-border'
  return (
    STATUS_COLOR[String(status).trim()] ||
    'bg-muted text-muted-foreground border border-border'
  )
}

function formatDurHeader(v) {
  // Duration is always numeric in nature: treat missing/empty as 0 rather
  // than "—" so the Call Info card doesn't read as "unknown" for flows
  // that simply never completed (e.g. in-dialog INVITEs without BYE).
  if (v == null || v === '') return '0 s'
  if (typeof v === 'number') return `${v} s`
  return String(v)
}

// formatText renders optional string columns in the Call Info table.
// We intentionally fall back to an empty cell (not "—") so a row full of
// unknown text values stays visually clean instead of showing five dashes.
function formatText(v) {
  if (v == null) return ''
  const s = String(v)
  return s
}

/** Right column: session / SRD / setup delays (same keys as TILE_DEFS). */
function CallTimingStats({ row }) {
  const tiles = TILE_DEFS.map((t) => {
    const v = row[t.key]
    if (v == null || v === '' || (typeof v === 'number' && v <= 0)) return null
    const body = t.isSec ? `${v} ${t.suffix}` : `${v} ${t.suffix}`
    return (
      <div
        key={t.key}
        className="rounded border border-border bg-muted/40 px-2 py-1.5 text-[10px] leading-tight"
      >
        <div className="uppercase tracking-wide text-muted-foreground">{t.label}</div>
        <div className="font-mono text-[11px] text-foreground">{body}</div>
      </div>
    )
  }).filter(Boolean)

  if (tiles.length === 0) {
    return (
      <div className="flex min-h-[4.5rem] items-center rounded border border-dashed border-border/80 bg-muted/20 px-2 py-2 text-[10px] text-muted-foreground">
        No timing stats
      </div>
    )
  }
  return (
    <div className="grid grid-cols-2 gap-2 gap-x-3">
      {tiles}
    </div>
  )
}

/** Donut + methods (compact left cluster) + timing stats fills the rest on desktop. */
function MethodsAndTimingBlock({ row, distribution }) {
  const hasDist = Array.isArray(distribution) && distribution.length > 0
  const total = hasDist ? distribution.reduce((s, x) => s + (Number(x.count) || 0), 0) : 0
  const showDonut = hasDist && total > 0

  const cx = 50
  const cy = 50
  const R = 40
  const rIn = 22
  let angle = -Math.PI / 2
  const paths = showDonut
    ? distribution.map((seg, i) => {
        const v = Number(seg.count) || 0
        const frac = v / total
        const a0 = angle
        const a1 = angle + frac * 2 * Math.PI
        angle = a1
        const large = a1 - a0 > Math.PI ? 1 : 0
        const x0 = cx + R * Math.cos(a0)
        const y0 = cy + R * Math.sin(a0)
        const x1 = cx + R * Math.cos(a1)
        const y1 = cy + R * Math.sin(a1)
        const ix0 = cx + rIn * Math.cos(a1)
        const iy0 = cy + rIn * Math.sin(a1)
        const ix1 = cx + rIn * Math.cos(a0)
        const iy1 = cy + rIn * Math.sin(a0)
        const d = `M ${x0} ${y0} A ${R} ${R} 0 ${large} 1 ${x1} ${y1} L ${ix0} ${iy0} A ${rIn} ${rIn} 0 ${large} 0 ${ix1} ${iy1} Z`
        const color = PIE_COLORS[i % PIE_COLORS.length]
        return (
          <path
            key={`${seg.method}-${i}`}
            d={d}
            fill={color}
            stroke="var(--border)"
            strokeWidth="0.6"
          />
        )
      })
    : []

  return (
    <div className="flex flex-col gap-3 border-b border-border/60 pb-3 lg:flex-row lg:items-start lg:gap-4">
      <div className="flex w-full min-w-0 flex-col flex-wrap gap-2 sm:flex-row sm:items-start sm:gap-3 lg:inline-flex lg:w-auto lg:max-w-[min(17rem,20vw)] lg:shrink-0">
        {showDonut ? (
          <svg viewBox="0 0 100 100" className="h-20 w-20 shrink-0 sm:h-24 sm:w-24" aria-hidden>
            {paths}
          </svg>
        ) : null}
        <div className="min-w-0 space-y-1.5">
          <div className="text-[10px] font-medium uppercase tracking-wide text-muted-foreground">
            Methods
          </div>
          {hasDist ? (
            <div className="grid max-h-40 grid-cols-2 gap-x-2 gap-y-0.5 overflow-y-auto pr-1">
              {distribution.map((seg, i) => (
                <div key={seg.method} className="flex min-w-0 items-center gap-1.5 font-mono text-[11px]">
                  <span
                    className="inline-block h-2 w-2 shrink-0 rounded-sm"
                    style={{ background: PIE_COLORS[i % PIE_COLORS.length] }}
                  />
                  <span className="min-w-0 truncate text-foreground">{seg.method}</span>
                  <span className="shrink-0 text-muted-foreground">×{seg.count}</span>
                </div>
              ))}
            </div>
          ) : (
            <div className="text-[10px] text-muted-foreground">No method distribution</div>
          )}
        </div>
      </div>

      <div className="w-full min-w-0 flex-1 space-y-1.5 border-t border-border/60 pt-3 lg:border-l lg:border-t-0 lg:pl-4 lg:pt-0">
        <div className="text-[10px] font-medium uppercase tracking-wide text-muted-foreground">
          {'Session & timing'}
        </div>
        <CallTimingStats row={row} />
      </div>
    </div>
  )
}

function layoutHiddenKeys(row) {
  const h = new Set([
    'session_id',
    'methods_distribution',
    'status',
    'call_duration_seconds',
    'from_party',
    'ruri_party',
    'source',
    'destination',
    'uac',
    'uas',
    'ringing_seconds',
    'session_request_delay_ms',
    'session_setup_delay_ms',
    'failed_session_setup_delay_ms',
    'session_disconnect_delay_ms',
    'session_duration_seconds',
  ])
  if (row.from_party) {
    h.add('caller')
    h.add('callee')
  }
  return h
}

export default function CallInfoPanel({ data, isLoading, err, emptyLabel }) {
  /** Call-ID → expanded; new legs default to open */
  const [expandedBySid, setExpandedBySid] = React.useState(() => ({}))
  const items = Array.isArray(data?.items) ? data.items : []

  React.useEffect(() => {
    if (items.length === 0) return
    setExpandedBySid((prev) => {
      const next = { ...prev }
      for (let i = 0; i < items.length; i++) {
        const row = items[i]
        const sid = row.session_id || `session-${i}`
        if (!(sid in next)) next[sid] = true
      }
      return next
    })
  }, [items])

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
  if (items.length === 0) {
    return <div className="p-4 text-xs text-muted-foreground">{emptyLabel}</div>
  }

  return (
    <ScrollArea className="h-full pr-2">
      <div className="flex flex-col gap-3 pb-4">
        {items.map((row, idx) => {
          const sid = row.session_id || `session-${idx}`
          const isOpen = expandedBySid[sid] !== false
          const hidden = layoutHiddenKeys(row)
          const gridKeys = Object.keys(row).filter((k) => !hidden.has(k) && row[k] != null && row[k] !== '')

          const dur = row.call_duration_seconds
          const fromDisp = row.from_party || (row.caller ? String(row.caller) : '')
          const ruriDisp = row.ruri_party || (row.callee ? String(row.callee) : '')

          return (
            <div
              key={sid}
              className="overflow-hidden rounded-md border border-border bg-card"
            >
              <button
                type="button"
                className="flex w-full cursor-pointer items-start gap-2 border-b border-border bg-muted/50 px-3 py-2 text-left transition-colors hover:bg-muted/80"
                onClick={() => setExpandedBySid((e) => ({ ...e, [sid]: !isOpen }))}
                aria-expanded={isOpen}
              >
                <ChevronDown
                  className={cn('mt-0.5 h-4 w-4 shrink-0 text-muted-foreground transition-transform', !isOpen && '-rotate-90')}
                  aria-hidden
                />
                <div className="min-w-0 flex-1">
                  <div className="flex flex-col gap-1 sm:flex-row sm:items-start sm:justify-between">
                    <div className="min-w-0 flex-1">
                      <div className="text-[10px] font-medium uppercase tracking-wide text-muted-foreground">
                        Call-ID
                      </div>
                      <div className="break-all font-mono text-[11px] text-foreground">{sid}</div>
                    </div>
                    <div className="flex shrink-0 flex-wrap gap-3 font-mono text-[11px]">
                      <div>
                        <span className="text-muted-foreground">Duration: </span>
                        <span className="text-foreground">{formatDurHeader(dur)}</span>
                      </div>
                      <div className="flex items-center gap-1.5">
                        <span className="text-muted-foreground">Status: </span>
                        <span
                          className={cn(
                            'inline-flex items-center rounded px-1.5 py-0.5 text-[10px] font-medium uppercase tracking-wide',
                            statusClass(row.status),
                          )}
                        >
                          {row.status ?? '—'}
                        </span>
                      </div>
                    </div>
                  </div>
                </div>
              </button>

              {isOpen && (
                <>
                  <div className="space-y-0 px-3 pt-3">
                    <MethodsAndTimingBlock row={row} distribution={row.methods_distribution} />

                    {(row.uac || row.uas) && (
                      <div className="border-b border-border/60 py-2 font-mono text-[11px] text-foreground">
                        <span className="text-muted-foreground">UAC → UAS: </span>
                        <strong>
                          {(row.uac && String(row.uac)) || '—'} → {(row.uas && String(row.uas)) || '—'}
                        </strong>
                      </div>
                    )}

                    <div className="overflow-x-auto border-b border-border/60 py-2">
                      <table className="w-full min-w-[480px] border-collapse text-left font-mono text-[11px]">
                        <thead>
                          <tr className="text-muted-foreground opacity-80">
                            <th className="pb-1 pr-2 font-medium">duration</th>
                            <th className="pb-1 pr-2 font-medium">from</th>
                            <th className="pb-1 pr-2 font-medium">ruri</th>
                            <th className="pb-1 pr-2 font-medium">src ip:port</th>
                            <th className="pb-1 font-medium">dst ip:port</th>
                          </tr>
                        </thead>
                        <tbody>
                          <tr className="text-foreground">
                            <td className="pr-2 align-top">{formatDurHeader(dur)}</td>
                            <td className="max-w-[140px] break-all pr-2 align-top">{formatText(fromDisp)}</td>
                            <td className="max-w-[160px] break-all pr-2 align-top">{formatText(ruriDisp)}</td>
                            <td className="pr-2 align-top">{formatText(row.source)}</td>
                            <td className="align-top">{formatText(row.destination)}</td>
                          </tr>
                        </tbody>
                      </table>
                    </div>
                  </div>

                  <div className="grid grid-cols-1 gap-x-4 gap-y-1.5 p-3 sm:grid-cols-2">
                    {gridKeys.map((k) => (
                      <div key={k} className="flex flex-col gap-0.5">
                        <span className="text-[10px] uppercase tracking-wide text-muted-foreground">
                          {LABELS[k] || k}
                        </span>
                        <span className="break-words font-mono text-[11px] text-foreground">
                          {formatValue(k, row[k])}
                        </span>
                      </div>
                    ))}
                  </div>
                </>
              )}
            </div>
          )
        })}
      </div>
    </ScrollArea>
  )
}
