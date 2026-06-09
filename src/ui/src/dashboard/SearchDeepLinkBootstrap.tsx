import { useEffect, useRef } from 'react'
import { useDashboard } from './context/DashboardContext'
import { useSearchStore, searchStoreKey } from './stores/search-store'
import {
  buildSearchPayload,
  deepLinkSpecToFormFields,
  getDeepLinkSearchParams,
  parseSearchDeepLink,
  resolveDeepLinkTimestamp,
  stripDeepLinkParamsFromURL,
} from './searchDeepLink'

/**
 * On dashboard load, apply ?from_user=… (or homer-app legacy JSON) and run search
 * against the first results/chart widget.
 */
export default function SearchDeepLinkBootstrap() {
  const {
    widgets,
    loading,
    publishSearch,
    requestTimeRange,
  } = useDashboard()
  const specRef = useRef(parseSearchDeepLink(getDeepLinkSearchParams()))
  const appliedRef = useRef(false)

  useEffect(() => {
    if (appliedRef.current || loading) return
    const spec = specRef.current
    if (!spec) return

    const resultWidget = widgets.find((w) => w.type === 'results' || w.type === 'chart')
    if (!resultWidget) return

    appliedRef.current = true

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
    stripDeepLinkParamsFromURL()
  }, [widgets, loading, publishSearch, requestTimeRange])

  return null
}
