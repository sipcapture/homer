import { useEffect, useRef } from 'react'
import { useDashboard } from './context/DashboardContext'
import {
  getDeepLinkSearchParams,
  parseSearchDeepLink,
  stripDeepLinkParamsFromURL,
} from './searchDeepLink'
import { useApplyDeepLinkSearch } from './useApplyDeepLinkSearch'

/**
 * On dashboard load, apply ?from_user=… (or homer-app legacy JSON) and run search
 * against the first results/chart widget.
 */
export default function SearchDeepLinkBootstrap() {
  const { loading } = useDashboard()
  const apply = useApplyDeepLinkSearch()
  const specRef = useRef(parseSearchDeepLink(getDeepLinkSearchParams()))
  const appliedRef = useRef(false)

  useEffect(() => {
    if (appliedRef.current || loading) return
    const spec = specRef.current
    if (!spec) return
    if (!apply(spec)) return

    appliedRef.current = true
    stripDeepLinkParamsFromURL()
  }, [loading, apply])

  return null
}
