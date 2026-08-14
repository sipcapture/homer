import { useCallback } from 'react'
import { useDashboard } from './context/DashboardContext'
import { useSearchStore, searchStoreKey } from './stores/search-store'
import {
  buildSearchPayload,
  deepLinkSpecToFormFields,
  resolveDeepLinkTimestamp,
  type SearchDeepLinkSpec,
} from './searchDeepLink'

/** Apply a search deep-link spec to the first results/chart widget on the dashboard. */
export function useApplyDeepLinkSearch() {
  const { widgets, publishSearch, requestTimeRange } = useDashboard()

  return useCallback(
    (spec: SearchDeepLinkSpec): boolean => {
      const resultWidget = widgets.find((w) => w.type === 'results' || w.type === 'chart')
      if (!resultWidget) return false

      const searchWidget = widgets.find((w) => w.type === 'search')
      const storeKey = searchWidget ? searchStoreKey(searchWidget.config) : null
      const ts = resolveDeepLinkTimestamp(spec)
      requestTimeRange(null, ts.from, ts.to)

      const formFields = deepLinkSpecToFormFields(spec)
      if (storeKey) {
        const { setField, setLimit } = useSearchStore.getState()
        for (const [k, v] of Object.entries(formFields)) {
          setField(storeKey, k, v)
        }
        if (spec.limit != null) {
          setLimit(storeKey, spec.limit)
        }
      }

      const payload = buildSearchPayload(spec, { from: ts.from, to: ts.to }, spec.limit ?? 50)
      publishSearch(resultWidget.id, payload)
      return true
    },
    [widgets, publishSearch, requestTimeRange],
  )
}
