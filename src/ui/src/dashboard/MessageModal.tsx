// @ts-nocheck - TODO: rewrite with shadcn/ui + TanStack; typed when refactored
import React from 'react'
import { apiPost } from '../api'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { FloatingWindow } from '@/components/ui/floating-window'
import { ScrollArea } from '@/components/ui/scroll-area'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { displayDstIp, displaySrcIp } from '@/lib/ipAliasDisplay'
import {
  highlightJSON,
  isJsonDisplayable,
  payloadAsString,
} from '@/lib/jsonDisplay'
import { highlightSIP } from '@/lib/sipDisplay'
import { useLocale } from '@/components/locale/locale-provider'
import { captureIdOf } from './flow/flow-data'

const META_FIELDS = ['uuid', 'date', 'timestamp', 'event', 'method', 'src_ip', 'dst_ip', 'src_port', 'dst_port', 'session_id', 'cid', 'node_id', 'node_name']

const META_FIELD_LABELS: Record<string, string> = {
  node_id: 'Capture ID',
  node_name: 'Node Name',
}

function resolveCaptureIdDisplay(data) {
  if (!data) return undefined
  if (data.node_id != null && String(data.node_id).trim() !== '') return String(data.node_id).trim()
  const resolved = captureIdOf(data)
  return resolved || undefined
}

function resolveNodeNameDisplay(data) {
  if (!data) return undefined
  if (data.node_name != null && String(data.node_name).trim() !== '') {
    return String(data.node_name).trim()
  }
  const extra = data.data_extra
  if (!extra) return undefined
  let obj = extra
  if (typeof extra === 'string') {
    try {
      obj = JSON.parse(extra)
    } catch {
      return undefined
    }
  }
  if (obj && typeof obj === 'object' && obj.node_name != null && String(obj.node_name).trim() !== '') {
    return String(obj.node_name).trim()
  }
  return undefined
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

function MetaGrid({ data, timeZone, locale, overrides = {} }) {
  if (!data) return null
  return (
    <dl className="grid grid-cols-2 gap-x-4 gap-y-1 text-[11px] sm:grid-cols-3">
      {META_FIELDS.map((field) => {
        if (field in overrides) {
          return (
            <div key={field} className="flex min-w-0 flex-col">
              <dt className="text-[10px] uppercase tracking-wider text-muted-foreground">{META_FIELD_LABELS[field] ?? field}</dt>
              <dd className="truncate font-mono text-muted-foreground" title={String(overrides[field])}>
                {String(overrides[field])}
              </dd>
            </div>
          )
        }
        const raw = field === 'node_id'
          ? resolveCaptureIdDisplay(data)
          : field === 'node_name'
            ? resolveNodeNameDisplay(data)
            : data[field]
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
            <dt className="text-[10px] uppercase tracking-wider text-muted-foreground">{META_FIELD_LABELS[field] ?? field}</dt>
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
  const payload = payloadAsString(data?.payload)
  const payloadIsJson = isJsonDisplayable(data?.payload ?? payload)
  const payloadHtml = payloadIsJson ? highlightJSON(data?.payload ?? payload) : highlightSIP(payload)

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
            <MetaGrid data={data} timeZone={timeZone} locale={locale} overrides={messageContext?.metaOverrides ?? {}} />

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
