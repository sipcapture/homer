import { useState, useRef, useEffect, useCallback } from 'react'
import { Check, ChevronDown, Clock } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { ScrollArea } from '@/components/ui/scroll-area'
import { cn } from '@/lib/utils'
import type { CalendarPreset } from '../utils/resolveTimeRange'
import { useLocale } from '@/components/locale/locale-provider'

const QUICK_RANGES = [
  { label: 'Last 5 minutes', minutes: 5 },
  { label: 'Last 10 minutes', minutes: 10 },
  { label: 'Last 15 minutes', minutes: 15 },
  { label: 'Last 30 minutes', minutes: 30 },
  { label: 'Last 1 hour', minutes: 60 },
  { label: 'Last 3 hours', minutes: 180 },
  { label: 'Last 6 hours', minutes: 360 },
  { label: 'Last 12 hours', minutes: 720 },
  { label: 'Last 24 hours', minutes: 1440 },
  { label: 'Last 2 days', minutes: 2880 },
  { label: 'Last 7 days', minutes: 10080 },
  { label: 'Last 14 days', minutes: 20160 },
  { label: 'Last 30 days', minutes: 43200 },
]

const CALENDAR_PRESETS: { label: string; value: CalendarPreset }[] = [
  { label: 'Today', value: 'today' },
  { label: 'Yesterday', value: 'yesterday' },
  { label: 'This month', value: 'this_month' },
  { label: 'Last month', value: 'last_month' },
]

const TIMEZONES = [
  { value: 'local', label: 'Browser (Local)' },
  { value: 'UTC', label: 'UTC' },
  { value: 'Europe/London', label: 'Europe/London' },
  { value: 'Europe/Berlin', label: 'Europe/Berlin' },
  { value: 'Europe/Moscow', label: 'Europe/Moscow' },
  { value: 'Europe/Kiev', label: 'Europe/Kiev' },
  { value: 'America/New_York', label: 'America/New_York' },
  { value: 'America/Chicago', label: 'America/Chicago' },
  { value: 'America/Los_Angeles', label: 'America/Los_Angeles' },
  { value: 'Asia/Tokyo', label: 'Asia/Tokyo' },
  { value: 'Asia/Shanghai', label: 'Asia/Shanghai' },
  { value: 'Asia/Kolkata', label: 'Asia/Kolkata' },
  { value: 'Australia/Sydney', label: 'Australia/Sydney' },
]

function pad(n) {
  return n.toString().padStart(2, '0')
}

function formatForInput(date, tz) {
  if (!date) return ''
  if (tz === 'local') {
    return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}T${pad(date.getHours())}:${pad(date.getMinutes())}:${pad(date.getSeconds())}`
  }
  const f = new Intl.DateTimeFormat('en-US', {
    timeZone: tz,
    hour12: false,
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
  })
  const v = Object.fromEntries(f.formatToParts(date).map((p) => [p.type, p.value]))
  const hr = v.hour === '24' ? '00' : v.hour
  return `${v.year}-${v.month}-${v.day}T${hr}:${v.minute}:${v.second}`
}

function formatDisplay(date: Date | null, tz: string, locale: string | undefined) {
  if (!date) return '—'
  const opts: Intl.DateTimeFormatOptions = {
    month: 'short',
    day: 'numeric',
    hour: 'numeric',
    minute: 'numeric',
    second: 'numeric',
  }
  if (tz && tz !== 'local') opts.timeZone = tz
  return new Intl.DateTimeFormat(locale, opts).format(date)
}

function parseInputToMs(value, tz) {
  if (!value) return 0
  if (tz === 'local') return new Date(value).getTime()
  const parts = value.match(/(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2}):?(\d{2})?/)
  if (!parts) return new Date(value).getTime()
  const [, y, mo, d, h, mi, s] = parts.map(Number)
  const utcGuess = Date.UTC(y, mo - 1, d, h, mi, s || 0)
  const f = new Intl.DateTimeFormat('en-US', {
    timeZone: tz,
    hour12: false,
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
  })
  const v = Object.fromEntries(f.formatToParts(new Date(utcGuess)).map((p) => [p.type, p.value]))
  const asUTC = Date.UTC(
    Number(v.year),
    Number(v.month) - 1,
    Number(v.day),
    Number(v.hour === '24' ? 0 : v.hour),
    Number(v.minute),
    Number(v.second),
  )
  const offset = asUTC - new Date(utcGuess).getTime()
  return utcGuess - offset
}

interface TimeRangePickerProps {
  timeFrom: number | string
  timeTo: number | string
  onTimeChange: (
    minutes?: number | null,
    fromMs?: number,
    toMs?: number,
    opts?: { calendar?: CalendarPreset },
  ) => void
  activePreset: number | null
  calendarPreset?: CalendarPreset | null
  timeZone: string
  onTimeZoneChange: (tz: string) => void
}

export default function TimeRangePicker({
  timeFrom,
  timeTo,
  onTimeChange,
  activePreset,
  calendarPreset = null,
  timeZone,
  onTimeZoneChange,
}: TimeRangePickerProps) {
  const { resolved: locale } = useLocale()
  const [open, setOpen] = useState(false)
  const [tab, setTab] = useState('quick')
  const [absFrom, setAbsFrom] = useState('')
  const [absTo, setAbsTo] = useState('')
  const ref = useRef<HTMLDivElement | null>(null)

  useEffect(() => {
    function handleClickOutside(e: MouseEvent) {
      if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false)
    }
    if (open) document.addEventListener('mousedown', handleClickOutside)
    return () => document.removeEventListener('mousedown', handleClickOutside)
  }, [open])

  const handleOpen = useCallback(() => {
    const fromDate = timeFrom ? new Date(timeFrom) : new Date()
    const toDate = timeTo ? new Date(timeTo) : new Date()
    setAbsFrom(formatForInput(fromDate, timeZone))
    setAbsTo(formatForInput(toDate, timeZone))
    setOpen((prev) => !prev)
  }, [timeFrom, timeTo, timeZone])

  const handleQuickRange = useCallback(
    (minutes) => {
      onTimeChange(minutes)
      setOpen(false)
    },
    [onTimeChange],
  )

  const handleCalendarPreset = useCallback(
    (cal: CalendarPreset) => {
      onTimeChange(undefined, undefined, undefined, { calendar: cal })
      setOpen(false)
    },
    [onTimeChange],
  )

  const handleApplyAbsolute = useCallback(() => {
    const fromMs = parseInputToMs(absFrom, timeZone)
    const toMs = parseInputToMs(absTo, timeZone)
    if (fromMs && toMs && toMs > fromMs) {
      onTimeChange(null, fromMs, toMs)
      setOpen(false)
    }
  }, [absFrom, absTo, timeZone, onTimeChange])

  const presetLabel =
    typeof activePreset === 'number' && activePreset > 0
      ? QUICK_RANGES.find((r) => r.minutes === activePreset)?.label || `Last ${activePreset}m`
      : null
  const calendarLabel =
    calendarPreset === 'today'
      ? 'Today'
      : calendarPreset === 'yesterday'
        ? 'Yesterday'
        : calendarPreset === 'this_month'
          ? 'This month'
          : calendarPreset === 'last_month'
            ? 'Last month'
            : null

  const fromDate = timeFrom ? new Date(timeFrom) : null
  const toDate = timeTo ? new Date(timeTo) : null
  const displayLabel =
    presetLabel ||
    calendarLabel ||
    (fromDate && toDate
      ? `${formatDisplay(fromDate, timeZone, locale)} — ${formatDisplay(toDate, timeZone, locale)}`
      : 'Select time range')

  return (
    <div className="relative" ref={ref}>
      <Button
        type="button"
        variant="outline"
        size="sm"
        onClick={handleOpen}
        aria-expanded={open}
        className="min-w-0 justify-between gap-2 font-medium"
      >
        <Clock className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
        <span className="max-w-[320px] truncate">{displayLabel}</span>
        <ChevronDown
          className={cn(
            'h-3.5 w-3.5 shrink-0 text-muted-foreground transition-transform',
            open && 'rotate-180',
          )}
        />
      </Button>

      {open && (
        <div className="absolute right-0 top-full z-50 mt-1 w-[460px] border border-border bg-popover p-3 text-popover-foreground shadow-lg ring-1 ring-foreground/10">
          <Tabs value={tab} onValueChange={setTab}>
            <TabsList className="w-full">
              <TabsTrigger value="quick">Quick ranges</TabsTrigger>
              <TabsTrigger value="absolute">Absolute time</TabsTrigger>
              <TabsTrigger value="timezone">Time zone</TabsTrigger>
            </TabsList>

            <TabsContent value="quick" className="mt-3">
              <div className="flex flex-col gap-3">
                <div>
                  <div className="mb-1.5 text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">
                    Relative time ranges
                  </div>
                  <div className="grid grid-cols-3 gap-1.5">
                    {QUICK_RANGES.map((r) => (
                      <Button
                        key={r.minutes}
                        type="button"
                        variant={
                          activePreset === r.minutes && !calendarPreset ? 'secondary' : 'outline'
                        }
                        size="sm"
                        onClick={() => handleQuickRange(r.minutes)}
                        className="justify-start text-xs"
                      >
                        {r.label}
                      </Button>
                    ))}
                  </div>
                </div>
                <div>
                  <div className="mb-1.5 text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">
                    Other ranges
                  </div>
                  <div className="grid grid-cols-2 gap-1.5">
                    {CALENDAR_PRESETS.map((r) => {
                      const active = calendarPreset === r.value
                      return (
                        <Button
                          key={r.value}
                          type="button"
                          variant={active ? 'secondary' : 'outline'}
                          size="sm"
                          onClick={() => handleCalendarPreset(r.value)}
                          className="justify-start text-xs"
                        >
                          {r.label}
                        </Button>
                      )
                    })}
                  </div>
                </div>
              </div>
            </TabsContent>

            <TabsContent value="absolute" className="mt-3">
              <div className="flex flex-col gap-3">
                <div className="grid gap-1.5">
                  <Label htmlFor="trp-from">From</Label>
                  <Input
                    id="trp-from"
                    type="datetime-local"
                    step="1"
                    value={absFrom}
                    onChange={(e) => setAbsFrom(e.target.value)}
                  />
                </div>
                <div className="grid gap-1.5">
                  <Label htmlFor="trp-to">To</Label>
                  <div className="flex items-center gap-2">
                    <Input
                      id="trp-to"
                      type="datetime-local"
                      step="1"
                      value={absTo}
                      onChange={(e) => setAbsTo(e.target.value)}
                    />
                    <Button
                      type="button"
                      variant="outline"
                      size="sm"
                      onClick={() => setAbsTo(formatForInput(new Date(), timeZone))}
                    >
                      Now
                    </Button>
                  </div>
                </div>
                <Button type="button" size="sm" onClick={handleApplyAbsolute}>
                  Apply time range
                </Button>
              </div>
            </TabsContent>

            <TabsContent value="timezone" className="mt-3">
              <div className="flex flex-col gap-2">
                <ScrollArea className="h-[260px] pr-2">
                  <div className="flex flex-col gap-0.5">
                    {TIMEZONES.map((tz) => {
                      const active = timeZone === tz.value
                      return (
                        <Button
                          key={tz.value}
                          type="button"
                          variant="ghost"
                          size="sm"
                          onClick={() => onTimeZoneChange(tz.value)}
                          className={cn(
                            'w-full justify-between gap-2 text-xs',
                            active && 'bg-muted text-foreground',
                          )}
                        >
                          <span className="truncate">{tz.label}</span>
                          {active && <Check className="h-3.5 w-3.5" />}
                        </Button>
                      )
                    })}
                  </div>
                </ScrollArea>
                <div className="border-t border-border pt-2 text-[11px] text-muted-foreground">
                  Current: <strong className="text-foreground">{timeZone === 'local' ? 'Browser' : timeZone}</strong>
                </div>
              </div>
            </TabsContent>
          </Tabs>
        </div>
      )}
    </div>
  )
}
