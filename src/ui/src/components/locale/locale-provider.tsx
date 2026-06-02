import { createContext, useCallback, useContext, useEffect, useMemo, useState } from "react"

type LocalePref = "auto" | string

type LocaleProviderProps = {
  children: React.ReactNode
  defaultLocale?: LocalePref
  storageKey?: string
}

type LocaleProviderState = {
  locale: LocalePref
  setLocale: (locale: LocalePref) => void
  /** BCP-47 tag safe to pass to Intl APIs. When `locale` is 'auto', equals the browser default. */
  resolved: string
  /** What 'auto' would resolve to right now, independent of the current selection. */
  auto: string
}

/** Accept the 'auto' sentinel or any string Intl.Locale can parse. */
function isValidPref(value: string): boolean {
  if (value === "auto") return true
  try {
    new Intl.Locale(value)
    return true
  } catch {
    return false
  }
}

function browserLocale(): string {
  if (typeof navigator === "undefined") return "en-US"
  return navigator.languages?.[0] || navigator.language || "en-US"
}

/** Read+validate the stored pref. Falls back to `fallback` on missing/invalid/inaccessible storage. */
function readStoredPref(storageKey: string, fallback: LocalePref): LocalePref {
  try {
    const stored = localStorage.getItem(storageKey)
    if (stored && isValidPref(stored)) return stored
  } catch {
    // localStorage can throw when disabled (e.g. private mode, security policy).
  }
  return fallback
}

/** Best-effort persist. Storage failures are silently ignored so the in-memory state still works. */
function writeStoredPref(storageKey: string, value: LocalePref): void {
  try {
    localStorage.setItem(storageKey, value)
  } catch {
    // Quota exceeded / storage disabled — preference will be session-only.
  }
}

const initialState: LocaleProviderState = {
  locale: "auto",
  setLocale: () => null,
  resolved: "en-US",
  auto: "en-US",
}

const LocaleProviderContext = createContext<LocaleProviderState>(initialState)

export function LocaleProvider({
  children,
  defaultLocale = "auto",
  storageKey = "vite-ui-locale",
  ...props
}: LocaleProviderProps) {
  const [locale, setLocaleState] = useState<LocalePref>(() =>
    readStoredPref(storageKey, defaultLocale),
  )

  useEffect(() => {
    writeStoredPref(storageKey, locale)
  }, [locale, storageKey])

  const setLocale = useCallback(
    (next: LocalePref) => {
      setLocaleState(isValidPref(next) ? next : defaultLocale)
    },
    [defaultLocale],
  )

  const value = useMemo<LocaleProviderState>(
    () => {
      const auto = browserLocale()
      return {
        locale,
        setLocale,
        resolved: locale === "auto" ? auto : locale,
        auto,
      }
    },
    [locale, setLocale],
  )

  return (
    <LocaleProviderContext.Provider {...props} value={value}>
      {children}
    </LocaleProviderContext.Provider>
  )
}

// eslint-disable-next-line react-refresh/only-export-components
export const useLocale = () => {
  const context = useContext(LocaleProviderContext)
  if (context === undefined)
    throw new Error("useLocale must be used within a LocaleProvider")
  return context
}
