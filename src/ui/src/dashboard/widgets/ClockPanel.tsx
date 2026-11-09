import { useCallback, useEffect, useMemo, useState } from 'react'
import { Globe, Plus, Search, Settings2, X } from 'lucide-react'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { ScrollArea } from '@/components/ui/scroll-area'
import { useLocale } from '@/components/locale/locale-provider'
import { canonicalTimeZone, suggestTimeZones, timeZoneLabel } from '@/lib/timezones'

const MAX_ZONES = 8

export interface ClockPanelConfig {
  /** @deprecated use timeZones */
  timeZone?: string
  /** IANA identifiers, or "local" for browser time */
  timeZones?: string[]
}

interface ClockPanelProps {
  config?: ClockPanelConfig
  onConfigChange?: (cfg: ClockPanelConfig) => void
}

function normalizeZones(config?: ClockPanelConfig): string[] {
  const raw = config?.timeZones
  if (Array.isArray(raw) && raw.length > 0) {
    const seen = new Set<string>()
    const out: string[] = []
    for (const z of raw) {
      // Saved configs may hold legacy or mis-cased names (Asia/Calcutta, europe/berlin).
      const t = typeof z === 'string' ? canonicalTimeZone(z) : null
      if (!t || seen.has(t)) continue
      seen.add(t)
      out.push(t)
      if (out.length >= MAX_ZONES) break
    }
    if (out.length) return out
  }
  const legacy = config?.timeZone ? canonicalTimeZone(config.timeZone) : null
  if (legacy) return [legacy]
  return ['local']
}

function formatClock(time: Date, timeZone: string, locale: string | undefined) {
  const opts: Intl.DateTimeFormatOptions = {
    hour: 'numeric',
    minute: 'numeric',
    second: 'numeric',
  }
  const dateOpts: Intl.DateTimeFormatOptions = {
    weekday: 'short',
    year: 'numeric',
    month: 'short',
    day: 'numeric',
  }
  if (timeZone !== 'local') {
    opts.timeZone = timeZone
    dateOpts.timeZone = timeZone
  }
  return {
    time: new Intl.DateTimeFormat(locale, opts).format(time),
    date: new Intl.DateTimeFormat(locale, dateOpts).format(time),
    label: timeZoneLabel(timeZone),
  }
}

export default function ClockPanel({ config, onConfigChange }: ClockPanelProps) {
  const { resolved: locale } = useLocale()
  const [now, setNow] = useState(() => new Date())
  const [settingsOpen, setSettingsOpen] = useState(false)
  const [tzQuery, setTzQuery] = useState('')

  const zones = useMemo(() => normalizeZones(config), [config])

  useEffect(() => {
    const id = setInterval(() => setNow(new Date()), 1000)
    return () => clearInterval(id)
  }, [])

  const handleZonesChange = useCallback(
    (next: string[]) => {
      const cleaned = normalizeZones({ timeZones: next })
      onConfigChange?.({ ...config, timeZones: cleaned, timeZone: undefined })
    },
    [config, onConfigChange],
  )

  const removeZone = (z: string) => {
    if (zones.length <= 1) return
    handleZonesChange(zones.filter(x => x !== z))
  }

  const atLimit = zones.length >= MAX_ZONES
  const query = tzQuery.trim()

  // Empty query: the short list of common zones. Typing: search every zone.
  const suggestions = useMemo(() => {
    return suggestTimeZones(query).filter(z => !zones.includes(z))
  }, [query, zones])

  // A valid IANA name that is not in the list (e.g. Etc/GMT+3) can still be added.
  // Compare by canonical name so "europe/berlin" or "Asia/Calcutta" never duplicate a clock.
  const typedZone = query ? canonicalTimeZone(query) : null
  const exactTyped =
    typedZone && !zones.includes(typedZone) && !suggestions.includes(typedZone) ? typedZone : null

  const addZone = (z: string) => {
    const zone = canonicalTimeZone(z)
    if (!zone || zones.includes(zone) || atLimit) return
    handleZonesChange([...zones, zone])
    setTzQuery('')
  }

  const addFromQuery = () => {
    if (exactTyped) addZone(exactTyped)
    else if (suggestions.length > 0) addZone(suggestions[0])
  }

  const count = zones.length
  const gridClass =
    count === 1
      ? 'grid-cols-1'
      : count === 2
        ? 'grid-cols-2'
        : 'grid-cols-2 sm:grid-cols-3'

  return (
    <div className="relative flex h-full min-h-0 flex-col p-2 text-foreground">
      {onConfigChange && (
        <div className="absolute right-1 top-1 z-10">
          <Dialog open={settingsOpen} onOpenChange={setSettingsOpen}>
            <DialogTrigger asChild>
              <Button variant="ghost" size="icon" className="h-7 w-7 shrink-0 text-muted-foreground" title="Time zones">
                <Settings2 className="h-3.5 w-3.5" />
              </Button>
            </DialogTrigger>
            <DialogContent className="max-w-md">
              <DialogHeader>
                <DialogTitle className="flex items-center gap-2 text-sm">
                  <Globe className="h-4 w-4" />
                  Time zones
                </DialogTitle>
                <DialogDescription className="text-xs">
                  Up to {MAX_ZONES} clocks. Pick a common zone, or type to search all IANA time zones.
                </DialogDescription>
              </DialogHeader>
              <div className="grid gap-3 py-2">
                <div className="flex flex-wrap gap-1.5">
                  {zones.map(z => (
                    <span
                      key={z}
                      className="inline-flex items-center gap-1 rounded-md border border-border bg-muted/40 px-2 py-0.5 font-mono text-[11px]"
                    >
                      {timeZoneLabel(z)}
                      <button
                        type="button"
                        className="rounded p-0.5 hover:bg-muted"
                        disabled={zones.length <= 1}
                        onClick={() => removeZone(z)}
                        aria-label={`Remove ${z}`}
                      >
                        <X className="h-3 w-3" />
                      </button>
                    </span>
                  ))}
                </div>
                <div className="grid gap-1">
                  <Label htmlFor="clock-tz-search" className="text-[11px] text-muted-foreground">
                    Add time zone
                  </Label>
                  <div className="relative">
                    <Search className="pointer-events-none absolute left-2 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-muted-foreground" />
                    <Input
                      id="clock-tz-search"
                      type="search"
                      value={tzQuery}
                      onChange={e => setTzQuery(e.target.value)}
                      placeholder="Search, e.g. sao paulo"
                      className="h-8 pl-7 font-mono text-xs"
                      disabled={atLimit}
                      onKeyDown={e => {
                        if (e.key === 'Enter') {
                          e.preventDefault()
                          addFromQuery()
                        }
                      }}
                    />
                  </div>
                  <div className="text-[10px] text-muted-foreground">
                    {atLimit
                      ? `Limit of ${MAX_ZONES} clocks reached. Remove one to add another.`
                      : typedZone && zones.includes(typedZone)
                        ? `${timeZoneLabel(typedZone)} is already added`
                        : exactTyped
                        ? 'Not in the list, but a valid IANA name · Enter adds it as typed'
                        : query
                        ? `${suggestions.length} match${suggestions.length === 1 ? '' : 'es'} · Enter adds the first`
                        : 'Common zones · type to search all'}
                  </div>
                  {!atLimit && (
                    <ScrollArea className="h-40 rounded-md border border-border">
                      <div className="flex flex-col gap-0.5 p-1">
                        {exactTyped && (
                          <Button
                            type="button"
                            variant="ghost"
                            size="sm"
                            className="h-7 justify-start gap-1.5 font-mono text-xs"
                            onClick={() => addZone(exactTyped)}
                          >
                            <Plus className="h-3 w-3" />
                            Add "{exactTyped}"
                          </Button>
                        )}
                        {suggestions.map(z => (
                          <Button
                            key={z}
                            type="button"
                            variant="ghost"
                            size="sm"
                            className="h-7 justify-start font-mono text-xs"
                            onClick={() => addZone(z)}
                          >
                            {timeZoneLabel(z)}
                          </Button>
                        ))}
                        {!exactTyped && suggestions.length === 0 && (
                          <div className="px-2 py-3 text-center text-xs text-muted-foreground">
                            {query ? `No time zones match "${query}"` : 'All common zones added. Type to search.'}
                          </div>
                        )}
                      </div>
                    </ScrollArea>
                  )}
                </div>
              </div>
              <DialogFooter>
                <Button type="button" variant="secondary" size="sm" onClick={() => setSettingsOpen(false)}>
                  Done
                </Button>
              </DialogFooter>
            </DialogContent>
          </Dialog>
        </div>
      )}

      <div className={`grid min-h-0 flex-1 gap-2 overflow-auto ${gridClass} place-content-center`}>
        {zones.map(z => {
          const { time: timeStr, date, label } = formatClock(now, z, locale)
          return (
            <div
              key={z}
              className="flex min-w-0 flex-col items-center justify-center gap-0.5 rounded-md border border-border/50 bg-muted/10 px-2 py-2"
            >
              <div className="truncate font-mono text-lg font-semibold tabular-nums tracking-tight sm:text-2xl md:text-3xl">
                {timeStr}
              </div>
              <div className="truncate text-center text-[10px] text-muted-foreground sm:text-xs">{date}</div>
              <div className="mt-0.5 truncate text-center text-[9px] uppercase tracking-wider text-muted-foreground">
                {label}
              </div>
            </div>
          )
        })}
      </div>
    </div>
  )
}
