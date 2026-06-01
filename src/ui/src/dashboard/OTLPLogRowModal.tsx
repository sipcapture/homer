// @ts-nocheck — dashboard modals; typed when refactored with shared types
import { useMemo, useState } from 'react'
import { FloatingWindow } from '@/components/ui/floating-window'
import { ScrollArea } from '@/components/ui/scroll-area'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { cn } from '@/lib/utils'
import { useLocale } from '@/components/locale/locale-provider'

function escapeHtml(str) {
  if (typeof str !== 'string') return str
  return str
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
}

function isJSON(str) {
  if (!str) return false
  const trimmed = String(str).trim()
  return (trimmed.startsWith('{') && trimmed.endsWith('}')) || (trimmed.startsWith('[') && trimmed.endsWith(']'))
}

function highlightJSON(payload) {
  try {
    const obj = JSON.parse(payload)
    const formatted = JSON.stringify(obj, null, 2)
    let html = escapeHtml(formatted)
    html = html.replace(/&quot;([^&]+)&quot;(\s*:)/g,
      '<span class="text-sky-500">&quot;$1&quot;</span><span class="text-muted-foreground">$2</span>')
    html = html.replace(/: &quot;([^&]*)&quot;/g,
      ': <span class="text-emerald-500">&quot;$1&quot;</span>')
    html = html.replace(/: (-?\d+\.?\d*)/g,
      ': <span class="text-amber-500">$1</span>')
    html = html.replace(/: (true|false)/g,
      ': <span class="text-violet-500">$1</span>')
    html = html.replace(/: (null)/g,
      ': <span class="text-muted-foreground">$1</span>')
    html = html.replace(/([{}\[\]])/g, '<span class="text-muted-foreground">$1</span>')
    return html
  } catch {
    return escapeHtml(payload)
  }
}

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

function formatTs(val, locale, timeZone) {
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

/** Preferred detail keys first; remaining keys appended alphabetically. */
const DETAIL_PRIORITY = [
  'timestamp',
  'TIMESTAMP',
  'trace_id',
  'TRACE_ID',
  'span_id',
  'SPAN_ID',
  'service_name',
  'SERVICE_NAME',
  'severity_text',
  'SEVERITY_TEXT',
  'severity_number',
  'SEVERITY_NUMBER',
  'body',
  'BODY',
]

export default function OTLPLogRowModal({ modal, timeZone, onClose }) {
  const { resolved: locale } = useLocale()
  const { modalKey, row } = modal
  const [jsonTab, setJsonTab] = useState('details')

  const { detailEntries, jsonText, bodyText } = useMemo(() => {
    if (!row || typeof row !== 'object') {
      return { detailEntries: [], jsonText: '{}', bodyText: '' }
    }
    const seen = new Set()
    const entries = []
    for (const k of DETAIL_PRIORITY) {
      if (!Object.prototype.hasOwnProperty.call(row, k)) continue
      seen.add(k)
      const v = row[k]
      let display = v
      if (k === 'timestamp' || k === 'TIMESTAMP') display = formatTs(v, locale, timeZone)
      else if (v != null && typeof v === 'object') display = JSON.stringify(v)
      else if (v != null) display = String(v)
      else display = '—'
      entries.push({ key: k, display })
    }
    const rest = Object.keys(row)
      .filter((k) => !seen.has(k))
      .sort((a, b) => a.localeCompare(b))
    for (const k of rest) {
      const v = row[k]
      let display = v
      if (v != null && typeof v === 'object') display = JSON.stringify(v)
      else if (v != null) display = String(v)
      else display = '—'
      entries.push({ key: k, display })
    }
    let text
    try {
      text = JSON.stringify(row, null, 2)
    } catch {
      text = String(row)
    }
    const body = row?.body ?? row?.BODY ?? ''
    return { detailEntries: entries, jsonText: text, bodyText: String(body ?? '') }
  }, [row, timeZone, locale])

  const traceId = row?.trace_id ?? row?.TRACE_ID ?? ''
  const shortTrace =
    traceId && String(traceId).length > 24
      ? `${String(traceId).slice(0, 12)}…${String(traceId).slice(-8)}`
      : traceId || 'log'

  const sev = resolvedSeverityLabel(row)
  const badgeCls = severityBadgeClass(sev)
  const prettyHtml = isJSON(jsonText) ? highlightJSON(jsonText) : escapeHtml(jsonText)

  return (
    <FloatingWindow
      open
      onClose={onClose}
      id={`otlplogrow:${modalKey || shortTrace}`}
      title={
        <span className="truncate font-mono text-sm">
          OTLP log · <span className="text-muted-foreground">{shortTrace}</span>
        </span>
      }
      headerActions={null}
      defaultWidth={Math.min(820, typeof window !== 'undefined' ? window.innerWidth - 64 : 820)}
      defaultHeight={Math.min(Math.round((typeof window !== 'undefined' ? window.innerHeight : 800) * 0.72), 640)}
      minWidth={480}
      minHeight={320}
      className="flex min-h-0 flex-col overflow-hidden"
    >
      <div className="flex min-h-0 flex-1 flex-col overflow-hidden">
        <div className="shrink-0 space-y-2 border-b border-border px-3 py-2">
          <div className="flex flex-wrap items-center gap-2 text-[11px]">
            <span
              className={cn(
                'inline-block rounded-none border px-1.5 py-0.5 text-[10px] font-semibold uppercase tracking-wide',
                badgeCls,
              )}
            >
              {sev}
            </span>
            <span className="font-mono text-muted-foreground">
              {formatTs(row?.timestamp ?? row?.TIMESTAMP, locale, timeZone)}
            </span>
          </div>
          {bodyText ? (
            <div>
              <span className="text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">Body</span>
              <ScrollArea className="mt-1 max-h-[140px] rounded-sm border border-border bg-muted/20">
                <pre className="whitespace-pre-wrap break-words p-2 font-mono text-[11px] text-foreground">
                  {bodyText}
                </pre>
              </ScrollArea>
            </div>
          ) : null}
        </div>

        <Tabs
          value={jsonTab}
          onValueChange={setJsonTab}
          className="flex min-h-0 flex-1 flex-col overflow-hidden px-3 pb-2 pt-1"
        >
          <TabsList variant="line" className="h-8 w-fit shrink-0 justify-start">
            <TabsTrigger value="details" className="text-[11px]">
              Details
            </TabsTrigger>
            <TabsTrigger value="pretty" className="text-[11px]">
              JSON (pretty)
            </TabsTrigger>
            <TabsTrigger value="raw" className="text-[11px]">
              JSON (raw)
            </TabsTrigger>
          </TabsList>
          <TabsContent
            value="details"
            className="mt-0 flex min-h-0 flex-1 flex-col overflow-hidden data-[state=inactive]:hidden"
          >
            <ScrollArea className="min-h-0 flex-1 rounded-sm border border-border">
              <dl className="grid grid-cols-1 gap-x-4 gap-y-2 p-2 text-[11px] sm:grid-cols-2">
                {detailEntries.length === 0 ? (
                  <div className="col-span-full py-4 text-center text-muted-foreground">No row data</div>
                ) : (
                  detailEntries.map(({ key, display }) => (
                    <div key={key} className="min-w-0">
                      <dt className="text-[10px] uppercase tracking-wider text-muted-foreground">{key}</dt>
                      <dd className="break-words font-mono text-foreground">{display}</dd>
                    </div>
                  ))
                )}
              </dl>
            </ScrollArea>
          </TabsContent>
          <TabsContent
            value="pretty"
            className="mt-0 flex min-h-0 flex-1 flex-col overflow-hidden data-[state=inactive]:hidden"
          >
            <ScrollArea className="min-h-0 flex-1 rounded-sm border border-border bg-muted/30">
              <pre
                className="whitespace-pre p-2 font-mono text-[11px] leading-relaxed"
                dangerouslySetInnerHTML={{ __html: prettyHtml || '(empty)' }}
              />
            </ScrollArea>
          </TabsContent>
          <TabsContent
            value="raw"
            className="mt-0 flex min-h-0 flex-1 flex-col overflow-hidden data-[state=inactive]:hidden"
          >
            <ScrollArea className="min-h-0 flex-1 rounded-sm border border-border bg-muted/30">
              <pre className="whitespace-pre-wrap break-all p-2 font-mono text-[11px] leading-relaxed text-foreground">
                {jsonText}
              </pre>
            </ScrollArea>
          </TabsContent>
        </Tabs>
      </div>
    </FloatingWindow>
  )
}
