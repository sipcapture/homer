import { useMemo } from 'react'
import { useLocale } from '@/components/locale/locale-provider'

export type DateTimeFormatOptions = Intl.DateTimeFormatOptions

export function withTimeZone(opts: DateTimeFormatOptions, timeZone?: string): DateTimeFormatOptions {
  const { timeZone: _ignored, ...rest } = opts
  if (!timeZone || timeZone === 'local') return rest
  return { ...rest, timeZone }
}

/**
 * Build an Intl.DateTimeFormat using the given locale (or browser default when undefined).
 * For React code prefer useDateTimeFormatter which reads the locale from context.
 */
export function makeDateTimeFormatter(
  locale: string | undefined,
  opts: DateTimeFormatOptions = {},
  timeZone?: string,
): Intl.DateTimeFormat {
  return new Intl.DateTimeFormat(locale, withTimeZone(opts, timeZone))
}

/** Hook returning a memoized Intl.DateTimeFormat that follows the user's locale preference. */
export function useDateTimeFormatter(
  opts: DateTimeFormatOptions = {},
  timeZone?: string,
): Intl.DateTimeFormat {
  const { resolved } = useLocale()
  const key = `${resolved ?? ''}|${timeZone ?? ''}|${JSON.stringify(opts)}`
  // eslint-disable-next-line react-hooks/exhaustive-deps
  return useMemo(() => makeDateTimeFormatter(resolved, opts, timeZone), [key])
}
