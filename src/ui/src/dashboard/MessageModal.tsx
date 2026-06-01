// @ts-nocheck - TODO: rewrite with shadcn/ui + TanStack; typed when refactored
import React from 'react'
import { apiPost } from '../api'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { FloatingWindow } from '@/components/ui/floating-window'
import { ScrollArea } from '@/components/ui/scroll-area'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { displayDstIp, displaySrcIp } from '@/lib/ipAliasDisplay'
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
  const trimmed = str.trim()
  return (trimmed.startsWith('{') && trimmed.endsWith('}')) ||
    (trimmed.startsWith('[') && trimmed.endsWith(']'))
}

// Tailwind v4 scans source files for class strings — keeping literal classnames
// here so the compiler picks them up:
// text-sky-500 text-violet-500 text-amber-500 text-emerald-500
// text-destructive text-muted-foreground font-semibold font-medium

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

function highlightSIP(payload) {
  if (!payload) return '(no payload)'
  const lines = payload.replace(/\r\n/g, '\n').replace(/\r/g, '\n').split('\n')
  const out = []
  for (const lineRaw of lines) {
    const line = lineRaw.trim()
    let escaped = escapeHtml(line)
    if (/^SIP\/2\.0\s+\d{3}/i.test(line)) {
      const respMatch = line.match(/^(SIP\/2\.0)\s+(\d{3})\s*(.*)$/i)
      if (respMatch) {
        const code = parseInt(respMatch[2], 10)
        let cls = 'text-foreground font-semibold'
        if (code >= 200 && code < 300) cls = 'text-emerald-500 font-semibold'
        else if (code >= 300 && code < 400) cls = 'text-amber-500 font-semibold'
        else if (code >= 400) cls = 'text-destructive font-semibold'
        escaped = `<span class="${cls}">${escapeHtml(respMatch[1])} ${escapeHtml(respMatch[2])} ${escapeHtml(respMatch[3])}</span>`
        out.push(escaped)
        continue
      }
    }
    if (/^[A-Z]+\s+sip:/i.test(line)) {
      const reqMatch = line.match(/^([A-Z]+)\s+(sip:[^\s]+)\s+(SIP\/2\.0)$/i)
      if (reqMatch) {
        const userMatch = reqMatch[2].match(/sip:([^@;>]+)/)
        const user = userMatch ? userMatch[1] : ''
        let uri = escapeHtml(reqMatch[2])
        if (user) {
          uri = uri.replace(escapeHtml(user), `<span class="text-amber-500">${escapeHtml(user)}</span>`)
        }
        escaped = `<span class="text-violet-500 font-semibold">${escapeHtml(reqMatch[1])}</span> <span class="text-amber-500">${uri}</span> ${escapeHtml(reqMatch[3])}`
        out.push(escaped)
        continue
      }
    }
    if (/^Call-ID\s*:/i.test(line)) {
      const m = line.match(/^(Call-ID\s*:)\s*(.*)$/i)
      if (m) {
        escaped = `<span class="text-sky-500 font-medium">${escapeHtml(m[1])}</span> <span class="text-emerald-500">${escapeHtml(m[2])}</span>`
      }
      out.push(escaped)
      continue
    }
    if (/^From\s*:/i.test(line)) {
      const fromUserMatch = line.match(/sip:([^@;>\s]+)/i)
      escaped = escapeHtml(line)
      if (fromUserMatch) {
        escaped = escaped.replace(escapeHtml(fromUserMatch[1]), `<span class="text-emerald-500">${escapeHtml(fromUserMatch[1])}</span>`)
      }
      escaped = escaped.replace(/^(From\s*:)/i, '<span class="text-sky-500 font-medium">$1</span>')
      out.push(escaped)
      continue
    }
    if (/^To\s*:/i.test(line)) {
      const toUserMatch = line.match(/sip:([^@;>\s]+)/i)
      escaped = escapeHtml(line)
      if (toUserMatch) {
        escaped = escaped.replace(escapeHtml(toUserMatch[1]), `<span class="text-emerald-500">${escapeHtml(toUserMatch[1])}</span>`)
      }
      escaped = escaped.replace(/^(To\s*:)/i, '<span class="text-sky-500 font-medium">$1</span>')
      out.push(escaped)
      continue
    }
    if (/^CSeq\s*:/i.test(line)) {
      const m = line.match(/^(CSeq\s*:)\s*(\d+)\s+([A-Z]+)/i)
      if (m) {
        escaped = `<span class="text-sky-500 font-medium">${escapeHtml(m[1])}</span> ${escapeHtml(m[2])} <span class="text-violet-500 font-semibold">${escapeHtml(m[3])}</span>`
      }
      out.push(escaped)
      continue
    }
    if (/^Via\s*:/i.test(line)) {
      escaped = escapeHtml(line).replace(/^(Via\s*:)/i, '<span class="text-sky-500 font-medium">$1</span>')
      out.push(escaped)
      continue
    }
    const headerMatch = line.match(/^([A-Za-z][A-Za-z0-9-]*\s*:)/)
    if (headerMatch) {
      escaped = escapeHtml(line).replace(escapeHtml(headerMatch[1]), `<span class="text-sky-500 font-medium">${escapeHtml(headerMatch[1])}</span>`)
    }
    out.push(escaped)
  }
  return out.join('\n')
}

const META_FIELDS = ['uuid', 'date', 'timestamp', 'event', 'method', 'src_ip', 'dst_ip', 'src_port', 'dst_port', 'session_id', 'cid']

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

function formatDateTime(value, locale, timeZone, dateOnly = false) {
  const date = parseTimestampValue(value)
  if (!date) return value
  const options = {
    year: 'numeric',
    month: 'numeric',
    day: 'numeric',
    ...(dateOnly
      ? {}
      : {
          hour: 'numeric',
          minute: 'numeric',
          second: 'numeric',
          fractionalSecondDigits: 3,
        }),
  }
  if (timeZone && timeZone !== 'local') options.timeZone = timeZone
  return new Intl.DateTimeFormat(locale, options).format(date)
}

function MetaGrid({ data, timeZone, locale }) {
  if (!data) return null
  return (
    <dl className="grid grid-cols-2 gap-x-4 gap-y-1 text-[11px] sm:grid-cols-3">
      {META_FIELDS.map((field) => {
        const raw = data[field]
        const display = raw === undefined
          ? '—'
          : field === 'timestamp'
            ? formatDateTime(raw, locale, timeZone)
            : field === 'date'
              ? formatDateTime(raw, locale, timeZone, true)
              : field === 'src_ip'
                ? displaySrcIp(data)
                : field === 'dst_ip'
                  ? displayDstIp(data)
                  : String(raw)
        const tip =
          (field === 'src_ip' || field === 'dst_ip') &&
          raw !== undefined &&
          String(display) !== String(raw)
            ? `${display}\n(IP: ${raw})`
            : String(display)
        return (
          <div key={field} className="flex min-w-0 flex-col">
            <dt className="text-[10px] uppercase tracking-wider text-muted-foreground">{field}</dt>
            <dd className="truncate font-mono text-foreground" title={tip}>
              {String(display)}
            </dd>
          </div>
        )
      })}
    </dl>
  )
}

export default function MessageModal({ modal, onClose, timeZone }) {
  const { resolved: locale } = useLocale()
  const [decoded, setDecoded] = React.useState(null)
  const [decoding, setDecoding] = React.useState(false)
  const [decodeError, setDecodeError] = React.useState('')
  const [jsonPayloadTab, setJsonPayloadTab] = React.useState('pretty')

  React.useEffect(() => {
    // reset decode state whenever the underlying message changes
    setDecoded(null)
    setDecodeError('')
    setDecoding(false)
    setJsonPayloadTab('pretty')
  }, [modal?.uuid])

  if (!modal) return null

  const { uuid, loading, data, error, messageContext, modalKey } = modal
  const payload = data?.payload || ''
  const payloadIsJson = isJSON(payload)
  const payloadHtml = payloadIsJson ? highlightJSON(payload) : highlightSIP(payload)

  const handleDecode = async () => {
    if (!uuid || decoding) return
    setDecoding(true)
    setDecodeError('')
    try {
      const body = {
        uuid,
        proto_type: messageContext?.proto_type ?? 1,
        event_type: messageContext?.event_type ?? 'call',
      }
      if (messageContext?.timestamp?.from != null && messageContext?.timestamp?.to != null) {
        body.timestamp = messageContext.timestamp
      }
      const res = await apiPost('/messages/decoded', body)
      setDecoded(res?.data || {})
    } catch (err) {
      setDecodeError(err.message)
    } finally {
      setDecoding(false)
    }
  }

  return (
    <FloatingWindow
      open
      onClose={onClose}
      id={`msg:${modalKey || uuid}`}
      title={<span className="truncate font-mono">Message: <span className="text-muted-foreground">{uuid}</span></span>}
      defaultWidth={Math.min(820, window.innerWidth - 64)}
      defaultHeight={Math.min(Math.round(window.innerHeight * 0.8), 720)}
      minWidth={480}
      minHeight={360}
    >
      <div className="flex flex-col gap-3 overflow-y-auto p-4">
        {loading && (
          <div className="flex items-center justify-center py-6 text-xs text-muted-foreground">
            Loading...
          </div>
        )}

        {error && (
          <Alert variant="destructive" className="py-2">
            <AlertDescription className="text-xs">{error}</AlertDescription>
          </Alert>
        )}

        {!loading && !error && (
          <>
            <MetaGrid data={data} timeZone={timeZone} locale={locale} />

            <div className="flex items-center justify-between">
              <span className="text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">
                Payload
              </span>
              <Button
                variant="outline"
                size="sm"
                className="h-6 text-[11px]"
                onClick={handleDecode}
                disabled={decoding}
              >
                {decoding ? 'Decoding...' : 'Decode'}
              </Button>
            </div>

            {payloadIsJson ? (
              <Tabs value={jsonPayloadTab} onValueChange={setJsonPayloadTab} className="flex flex-col gap-2">
                <TabsList variant="line" className="h-8 w-fit justify-start">
                  <TabsTrigger value="pretty" className="text-[11px]">
                    Pretty
                  </TabsTrigger>
                  <TabsTrigger value="raw" className="text-[11px]">
                    Raw
                  </TabsTrigger>
                </TabsList>
                <TabsContent value="pretty" className="mt-0">
                  <ScrollArea className="h-[320px] border border-border bg-muted/30">
                    <pre
                      className="whitespace-pre p-2 font-mono text-[11px] leading-relaxed"
                      dangerouslySetInnerHTML={{ __html: payloadHtml || '(no payload)' }}
                    />
                  </ScrollArea>
                </TabsContent>
                <TabsContent value="raw" className="mt-0">
                  <ScrollArea className="h-[320px] border border-border bg-muted/30">
                    <pre className="whitespace-pre-wrap break-all p-2 font-mono text-[11px] leading-relaxed text-foreground">
                      {payload || '(no payload)'}
                    </pre>
                  </ScrollArea>
                </TabsContent>
              </Tabs>
            ) : (
              <ScrollArea className="h-[320px] border border-border bg-muted/30">
                <pre
                  className="whitespace-pre p-2 font-mono text-[11px] leading-relaxed"
                  dangerouslySetInnerHTML={{ __html: payloadHtml || '(no payload)' }}
                />
              </ScrollArea>
            )}

            {decodeError && (
              <Alert variant="destructive" className="py-2">
                <AlertDescription className="text-xs">{decodeError}</AlertDescription>
              </Alert>
            )}

            {decoded && (
              <>
                <span className="text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">
                  Decoded
                </span>
                <ScrollArea className="h-[200px] border border-border bg-muted/30">
                  <pre className="whitespace-pre p-2 font-mono text-[11px] leading-relaxed text-foreground">
                    {typeof decoded === 'object' ? JSON.stringify(decoded, null, 2) : String(decoded)}
                  </pre>
                </ScrollArea>
              </>
            )}
          </>
        )}
      </div>
    </FloatingWindow>
  )
}
