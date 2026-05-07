import { useCallback, useEffect, useMemo, useState } from 'react'
import { Globe, Plus, Settings2, X } from 'lucide-react'
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
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'

const MAX_ZONES = 8

/** Popular IANA zones + browser local pseudo-zone */
const PRESET_TIMEZONES = [
  'local',
  'UTC',
  'Europe/London',
  'Europe/Berlin',
  'Europe/Paris',
  'Europe/Moscow',
  'America/New_York',
  'America/Chicago',
  'America/Denver',
  'America/Los_Angeles',
  'America/Sao_Paulo',
  'Asia/Dubai',
  'Asia/Kolkata',
  'Asia/Shanghai',
  'Asia/Tokyo',
  'Asia/Seoul',
  'Australia/Sydney',
  'Pacific/Auckland',
] as const

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

function isValidTimeZone(id: string): boolean {
  const z = id.trim()
  if (!z || z === 'local') return true
  try {
    Intl.DateTimeFormat(undefined, { timeZone: z }).format()
    return true
  } catch {
    return false
  }
}

function normalizeZones(config?: ClockPanelConfig): string[] {
  const raw = config?.timeZones
  if (Array.isArray(raw) && raw.length > 0) {
    const seen = new Set<string>()
    const out: string[] = []
    for (const z of raw) {
      const t = typeof z === 'string' ? z.trim() : ''
      if (!t || seen.has(t) || !isValidTimeZone(t)) continue
      seen.add(t)
      out.push(t)
      if (out.length >= MAX_ZONES) break
    }
    if (out.length) return out
  }
  const legacy = config?.timeZone?.trim()
  if (legacy && isValidTimeZone(legacy)) return [legacy === '' ? 'local' : legacy]
  return ['local']
}

function formatClock(time: Date, timeZone: string) {
  const opts: Intl.DateTimeFormatOptions = {
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
    hour12: false,
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
    time: new Intl.DateTimeFormat('en-GB', opts).format(time),
    date: new Intl.DateTimeFormat('en-GB', dateOpts).format(time),
    label: timeZone === 'local' ? 'Local' : timeZone,
  }
}

export default function ClockPanel({ config, onConfigChange }: ClockPanelProps) {
  const [now, setNow] = useState(() => new Date())
  const [settingsOpen, setSettingsOpen] = useState(false)
  const [customTz, setCustomTz] = useState('')

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

  const addPreset = (z: string) => {
    if (zones.includes(z) || zones.length >= MAX_ZONES) return
    handleZonesChange([...zones, z])
  }

  const addCustom = () => {
    const z = customTz.trim()
    if (!z || zones.includes(z) || zones.length >= MAX_ZONES) return
    if (!isValidTimeZone(z)) return
    handleZonesChange([...zones, z])
    setCustomTz('')
  }

  const count = zones.length
  const gridClass =
    count === 1
      ? 'grid-cols-1'
      : count === 2
        ? 'grid-cols-2'
        : 'grid-cols-2 sm:grid-cols-3'

  const presetsToAdd = PRESET_TIMEZONES.filter(z => !zones.includes(z))

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
                  Up to {MAX_ZONES} clocks. Use IANA names (e.g. Europe/Berlin) or Local for this browser.
                </DialogDescription>
              </DialogHeader>
              <div className="grid gap-3 py-2">
                <div className="flex flex-wrap gap-1.5">
                  {zones.map(z => (
                    <span
                      key={z}
                      className="inline-flex items-center gap-1 rounded-md border border-border bg-muted/40 px-2 py-0.5 font-mono text-[11px]"
                    >
                      {z === 'local' ? 'Local' : z}
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
                {presetsToAdd.length > 0 && (
                  <div className="grid gap-1">
                    <Label className="text-[11px] text-muted-foreground">Add preset</Label>
                    <Select onValueChange={v => { addPreset(v) }}>
                      <SelectTrigger className="h-8 text-xs">
                        <SelectValue placeholder="Choose timezone…" />
                      </SelectTrigger>
                      <SelectContent className="max-h-64">
                        {presetsToAdd.map(z => (
                          <SelectItem key={z} value={z} className="text-xs font-mono">
                            {z === 'local' ? 'Local (browser)' : z}
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                  </div>
                )}
                <div className="grid gap-1">
                  <Label htmlFor="clock-custom-tz" className="text-[11px] text-muted-foreground">
                    Custom IANA
                  </Label>
                  <div className="flex gap-1">
                    <Input
                      id="clock-custom-tz"
                      value={customTz}
                      onChange={e => setCustomTz(e.target.value)}
                      placeholder="e.g. Africa/Cairo"
                      className="h-8 font-mono text-xs"
                      onKeyDown={e => {
                        if (e.key === 'Enter') {
                          e.preventDefault()
                          addCustom()
                        }
                      }}
                    />
                    <Button type="button" size="sm" className="h-8 shrink-0 px-2" onClick={addCustom} disabled={zones.length >= MAX_ZONES}>
                      <Plus className="h-3.5 w-3.5" />
                    </Button>
                  </div>
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
          const { time: timeStr, date, label } = formatClock(now, z)
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
