import { useEffect, useState } from 'react'
import { apiGet } from '@/api'
import { DEFAULT_WIDGET_CONTROL } from '@/dashboard/widgets/registry'

type ModulesResponse = {
  data?: {
    widgets?: {
      control?: Record<string, boolean>
    }
  }
}

export function useModules() {
  const [widgetControl, setWidgetControl] =
    useState<Record<string, boolean>>(DEFAULT_WIDGET_CONTROL)
  const [loaded, setLoaded] = useState(false)

  useEffect(() => {
    let cancelled = false
    apiGet<ModulesResponse>('/modules')
      .then((res) => {
        if (cancelled) return
        const control = res?.data?.widgets?.control
        if (control) {
          setWidgetControl({ ...DEFAULT_WIDGET_CONTROL, ...control })
        }
        setLoaded(true)
      })
      .catch(() => {
        if (!cancelled) setLoaded(true)
      })
    return () => {
      cancelled = true
    }
  }, [])

  return { widgetControl, loaded }
}
