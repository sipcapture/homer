/**
 * Shared time zone list for the dashboard time range picker and the clock widget.
 *
 * `local` is a pseudo-zone meaning "use the browser/runtime zone"; every other
 * value is an IANA identifier understood by `Intl.DateTimeFormat`.
 */

export const LOCAL_TIME_ZONE = 'local'

/**
 * Short list of commonly used zones. Offered first by both pickers, and used
 * as the picker list when `Intl.supportedValuesOf` is not available.
 * Use {@link getCommonTimeZones} to get the names valid on the current runtime.
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
 * Legacy IANA names (tzdata `backward` renames) mapped to their current name.
 *
 * V8/Chromium take the time zone IDs from CLDR, which keeps many of the old
 * spellings: `Intl.supportedValuesOf('timeZone')` lists `Asia/Calcutta`, and
 * `resolvedOptions().timeZone` turns `Asia/Kolkata` back into `Asia/Calcutta`.
 * Only renames are listed here. Zones that tzdata merged into another region
 * (e.g. `Europe/Oslo` → `Europe/Berlin`) keep their own name.
 */
export const LEGACY_TIME_ZONE_ALIASES: Readonly<Record<string, string>> = {
  'Africa/Asmera': 'Africa/Asmara',
  'America/Buenos_Aires': 'America/Argentina/Buenos_Aires',
  'America/Catamarca': 'America/Argentina/Catamarca',
  'America/Coral_Harbour': 'America/Atikokan',
  'America/Cordoba': 'America/Argentina/Cordoba',
  'America/Godthab': 'America/Nuuk',
  'America/Indianapolis': 'America/Indiana/Indianapolis',
  'America/Jujuy': 'America/Argentina/Jujuy',
  'America/Louisville': 'America/Kentucky/Louisville',
  'America/Mendoza': 'America/Argentina/Mendoza',
  'Asia/Ashkhabad': 'Asia/Ashgabat',
  'Asia/Calcutta': 'Asia/Kolkata',
  'Asia/Dacca': 'Asia/Dhaka',
  'Asia/Katmandu': 'Asia/Kathmandu',
  'Asia/Macao': 'Asia/Macau',
  'Asia/Rangoon': 'Asia/Yangon',
  'Asia/Saigon': 'Asia/Ho_Chi_Minh',
  'Asia/Thimbu': 'Asia/Thimphu',
  'Asia/Ujung_Pandang': 'Asia/Makassar',
  'Asia/Ulan_Bator': 'Asia/Ulaanbaatar',
  'Atlantic/Faeroe': 'Atlantic/Faroe',
  'Europe/Kiev': 'Europe/Kyiv',
  'Pacific/Enderbury': 'Pacific/Kanton',
  'Pacific/Ponape': 'Pacific/Pohnpei',
  'Pacific/Truk': 'Pacific/Chuuk',
}

const LEGACY_NAMES_BY_CURRENT: Readonly<Record<string, readonly string[]>> = (() => {
  const out: Record<string, string[]> = {}
  for (const [legacy, current] of Object.entries(LEGACY_TIME_ZONE_ALIASES)) {
    ;(out[current] ??= []).push(legacy)
  }
  return out
})()

type SupportedValuesOf = (key: 'timeZone') => string[]
type ZoneValidator = (id: string) => boolean

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

/**
 * Current IANA name for a legacy alias, but only when the runtime knows it.
 * On older ICU (< 72, e.g. Chrome < 110) `Europe/Kyiv` does not exist, so
 * `Europe/Kiev` is kept there.
 */
function preferCurrentName(id: string, isValid: ZoneValidator): string {
  const current = LEGACY_TIME_ZONE_ALIASES[id]
  return current && isValid(current) ? current : id
}

/**
 * The one spelling to store and compare a zone by: correct case (`Intl`
 * accepts `europe/berlin`), and the current name instead of a legacy alias
 * when the runtime supports it. Returns `null` for unknown zones.
 */
export function canonicalTimeZone(id: string, isValid: ZoneValidator = isValidTimeZone): string | null {
  const z = id.trim()
  if (!z) return null
  if (z.toLowerCase() === LOCAL_TIME_ZONE) return LOCAL_TIME_ZONE
  let resolved: string
  try {
    resolved = new Intl.DateTimeFormat(undefined, { timeZone: z }).resolvedOptions().timeZone
  } catch {
    return null
  }
  return preferCurrentName(resolved, isValid)
}

/**
 * {@link COMMON_TIME_ZONES} with every entry usable on this runtime: a zone
 * the runtime does not know is swapped for a legacy alias it does know, or
 * dropped.
 */
export function buildCommonTimeZones(isValid: ZoneValidator = isValidTimeZone): string[] {
  const out: string[] = []
  for (const z of COMMON_TIME_ZONES) {
    const usable = isValid(z) ? z : LEGACY_NAMES_BY_CURRENT[z]?.find(isValid)
    if (usable && !out.includes(usable)) out.push(usable)
  }
  return out
}

/**
 * Builds the selectable list: `local`, `UTC`, then every zone the runtime
 * supports under its current name, sorted and without duplicates. Falls back
 * to the common zones when `Intl.supportedValuesOf` is missing or fails.
 */
export function buildTimeZoneList(
  supportedValuesOf: SupportedValuesOf | undefined,
  isValid: ZoneValidator = isValidTimeZone,
): string[] {
  let zones: string[]
  try {
    if (typeof supportedValuesOf !== 'function') return buildCommonTimeZones(isValid)
    zones = supportedValuesOf('timeZone')
  } catch {
    return buildCommonTimeZones(isValid)
  }
  if (!Array.isArray(zones) || zones.length === 0) return buildCommonTimeZones(isValid)

  const unique = new Set<string>()
  for (const z of zones) {
    if (!z || !isValid(z)) continue
    const name = preferCurrentName(z, isValid)
    if (name !== 'UTC') unique.add(name)
  }
  return [LOCAL_TIME_ZONE, 'UTC', ...[...unique].sort((a, b) => a.localeCompare(b))]
}

let cachedTimeZones: string[] | null = null
let cachedCommonTimeZones: string[] | null = null

/** Memoized {@link buildTimeZoneList} for the current runtime. */
export function getTimeZones(): string[] {
  if (!cachedTimeZones) {
    const intl = Intl as { supportedValuesOf?: SupportedValuesOf }
    cachedTimeZones = buildTimeZoneList(intl.supportedValuesOf?.bind(Intl))
  }
  return cachedTimeZones
}

/** Memoized {@link buildCommonTimeZones} for the current runtime. */
export function getCommonTimeZones(): string[] {
  if (!cachedCommonTimeZones) cachedCommonTimeZones = buildCommonTimeZones()
  return cachedCommonTimeZones
}

export function timeZoneLabel(id: string): string {
  return id === LOCAL_TIME_ZONE ? 'Browser (Local)' : id
}

function normalizeForSearch(s: string): string {
  return s.toLowerCase().replace(/[_/]+/g, ' ').replace(/\s+/g, ' ').trim()
}

/** Every name a zone can be found by: itself, its label, and its legacy/current aliases. */
function searchText(z: string): string {
  const aliases = [...(LEGACY_NAMES_BY_CURRENT[z] ?? []), LEGACY_TIME_ZONE_ALIASES[z] ?? '']
  return normalizeForSearch([z, timeZoneLabel(z), ...aliases].join(' '))
}

/**
 * Case-insensitive filter that treats `_` and `/` as spaces, so
 * "sao paulo", "Sao_Paulo" and "america/sao" all match `America/Sao_Paulo`.
 * Legacy names match too: "calcutta" finds `Asia/Kolkata`.
 */
export function filterTimeZones(zones: readonly string[], query: string): string[] {
  const q = normalizeForSearch(query)
  if (!q) return [...zones]
  return zones.filter((z) => searchText(z).includes(q))
}

/**
 * What a time zone picker should list for a search box value: the short
 * common list while the box is empty, and matches from the full list once
 * the user types.
 */
export function suggestTimeZones(
  query: string,
  allZones: readonly string[] = getTimeZones(),
  commonZones: readonly string[] = getCommonTimeZones(),
): string[] {
  if (!query.trim()) return [...commonZones]
  return filterTimeZones(allZones, query)
}
