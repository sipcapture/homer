/**
 * Shared time zone list for the dashboard time range picker and the clock widget.
 *
 * `local` is a pseudo-zone meaning "use the browser/runtime zone"; every other
 * value is an IANA identifier understood by `Intl.DateTimeFormat`.
 */

export const LOCAL_TIME_ZONE = 'local'

/**
 * Short list of commonly used zones. Offered as presets by the clock widget,
 * and used as the picker list when `Intl.supportedValuesOf` is not available.
 */
export const COMMON_TIME_ZONES: readonly string[] = [
  LOCAL_TIME_ZONE,
  'UTC',
  'Europe/London',
  'Europe/Berlin',
  'Europe/Paris',
  'Europe/Moscow',
  'Europe/Kyiv',
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
]

/**
 * Some ICU versions still report deprecated names. Map them to the current
 * IANA name so the list shows the modern spelling.
 */
const RENAMED_TIME_ZONES: Record<string, string> = {
  'Europe/Kiev': 'Europe/Kyiv',
}

type SupportedValuesOf = (key: 'timeZone') => string[]

/**
 * Builds the selectable list: `local`, `UTC`, then every zone the runtime
 * supports, sorted. Falls back to {@link COMMON_TIME_ZONES} when
 * `Intl.supportedValuesOf` is missing or fails.
 */
export function buildTimeZoneList(supportedValuesOf: SupportedValuesOf | undefined): string[] {
  let zones: string[]
  try {
    if (typeof supportedValuesOf !== 'function') return [...COMMON_TIME_ZONES]
    zones = supportedValuesOf('timeZone')
  } catch {
    return [...COMMON_TIME_ZONES]
  }
  if (!Array.isArray(zones) || zones.length === 0) return [...COMMON_TIME_ZONES]

  const unique = new Set<string>()
  for (const z of zones) {
    const name = RENAMED_TIME_ZONES[z] ?? z
    if (name && name !== 'UTC' && isValidTimeZone(name)) unique.add(name)
  }
  return [LOCAL_TIME_ZONE, 'UTC', ...[...unique].sort((a, b) => a.localeCompare(b))]
}

let cachedTimeZones: string[] | null = null

/** Memoized {@link buildTimeZoneList} for the current runtime. */
export function getTimeZones(): string[] {
  if (!cachedTimeZones) {
    const intl = Intl as { supportedValuesOf?: SupportedValuesOf }
    cachedTimeZones = buildTimeZoneList(intl.supportedValuesOf?.bind(Intl))
  }
  return cachedTimeZones
}

/** True for `local` or any IANA name accepted by `Intl.DateTimeFormat`. */
export function isValidTimeZone(id: string): boolean {
  const z = id.trim()
  if (!z) return false
  if (z === LOCAL_TIME_ZONE) return true
  try {
    new Intl.DateTimeFormat(undefined, { timeZone: z })
    return true
  } catch {
    return false
  }
}

export function timeZoneLabel(id: string): string {
  return id === LOCAL_TIME_ZONE ? 'Browser (Local)' : id
}

function normalizeForSearch(s: string): string {
  return s.toLowerCase().replace(/[_/]+/g, ' ').replace(/\s+/g, ' ').trim()
}

/**
 * Case-insensitive filter that treats `_` and `/` as spaces, so
 * "sao paulo", "Sao_Paulo" and "america/sao" all match `America/Sao_Paulo`.
 */
export function filterTimeZones(zones: readonly string[], query: string): string[] {
  const q = normalizeForSearch(query)
  if (!q) return [...zones]
  return zones.filter((z) => normalizeForSearch(`${z} ${timeZoneLabel(z)}`).includes(q))
}
