import { createContext, useCallback, useContext, useEffect, useMemo, useState } from "react"

/** Only these two tabs make sense as an initial landing tab; conditional
 * tabs (QoS, Events, ...) aren't valid choices here. */
export type DefaultDetailsTab = "flow" | "messages"

type DetailsTabProviderProps = {
  children: React.ReactNode
  defaultTab?: DefaultDetailsTab
  storageKey?: string
}

type DetailsTabProviderState = {
  defaultDetailsTab: DefaultDetailsTab
  setDefaultDetailsTab: (tab: DefaultDetailsTab) => void
}

function isValidTab(value: string): value is DefaultDetailsTab {
  return value === "flow" || value === "messages"
}

/** Read+validate the stored pref. Falls back to `fallback` on missing/invalid/inaccessible storage. */
function readStoredPref(storageKey: string, fallback: DefaultDetailsTab): DefaultDetailsTab {
  try {
    const stored = localStorage.getItem(storageKey)
    if (stored && isValidTab(stored)) return stored
  } catch {
    // localStorage can throw when disabled (e.g. private mode, security policy).
  }
  return fallback
}

/** Best-effort persist. Storage failures are silently ignored so the in-memory state still works. */
function writeStoredPref(storageKey: string, value: DefaultDetailsTab): void {
  try {
    localStorage.setItem(storageKey, value)
  } catch {
    // Quota exceeded / storage disabled — preference will be session-only.
  }
}

const DetailsTabProviderContext = createContext<DetailsTabProviderState | undefined>(undefined)

export function DetailsTabProvider({
  children,
  defaultTab = "messages",
  storageKey = "vite-ui-default-details-tab",
  ...props
}: DetailsTabProviderProps) {
  const [defaultDetailsTab, setDefaultDetailsTabState] = useState<DefaultDetailsTab>(() =>
    readStoredPref(storageKey, defaultTab),
  )

  useEffect(() => {
    writeStoredPref(storageKey, defaultDetailsTab)
  }, [defaultDetailsTab, storageKey])

  const setDefaultDetailsTab = useCallback(
    (next: DefaultDetailsTab) => {
      setDefaultDetailsTabState(isValidTab(next) ? next : defaultTab)
    },
    [defaultTab],
  )

  const value = useMemo<DetailsTabProviderState>(
    () => ({ defaultDetailsTab, setDefaultDetailsTab }),
    [defaultDetailsTab, setDefaultDetailsTab],
  )

  return (
    <DetailsTabProviderContext.Provider {...props} value={value}>
      {children}
    </DetailsTabProviderContext.Provider>
  )
}

// eslint-disable-next-line react-refresh/only-export-components
export const useDefaultDetailsTab = () => {
  const context = useContext(DetailsTabProviderContext)
  if (context === undefined)
    throw new Error("useDefaultDetailsTab must be used within a DetailsTabProvider")
  return context
}
