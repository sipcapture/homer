import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from 'react'
import { handleUnauthorized } from '../../api'
import { sortDashboardsByWeight } from '../utils/dashboard-sort'
import type { CalendarPreset } from '../utils/resolveTimeRange'

export interface DashboardConfig {
  columns?: number
  grid_type?: string
  locked?: boolean
}

export interface DashboardSummary {
  id?: string
  param?: string
  name?: string
  type?: number
  weight?: number
  shared?: boolean
  config?: DashboardConfig
}

export interface WidgetModel {
  id: string
  type: string
  title?: string
  x?: number
  y?: number
  w?: number
  h?: number
  config?: Record<string, unknown>
}

export interface TimeRange {
  from: number
  to: number
  activePreset?: number | null
  /** Rolling calendar window; mutually exclusive with numeric activePreset for API resolution */
  calendarPreset?: CalendarPreset | null
}

export type RequestTimeRange = (
  minutes?: number | null,
  fromMs?: number,
  toMs?: number,
  opts?: { calendar?: CalendarPreset },
) => void

type EventPayload = unknown
type EventListener = (payload: EventPayload) => void

class EventBus {
  private listeners = new Map<string, Set<EventListener>>()

  on(event: string, id: string, fn: EventListener): () => void {
    const key = `${event}:${id}`
    if (!this.listeners.has(key)) this.listeners.set(key, new Set())
    this.listeners.get(key)!.add(fn)
    return () => {
      this.listeners.get(key)?.delete(fn)
    }
  }

  emit(event: string, id: string, payload: EventPayload): void {
    const key = `${event}:${id}`
    this.listeners.get(key)?.forEach((fn) => fn(payload))
  }

  broadcast(event: string, payload: EventPayload): void {
    for (const [key, fns] of this.listeners) {
      if (key.startsWith(`${event}:`)) {
        fns.forEach((fn) => fn(payload))
      }
    }
  }

  clear(): void {
    this.listeners.clear()
  }
}

type HttpMethod = 'GET' | 'POST' | 'PUT' | 'PATCH' | 'DELETE'

export interface DashboardContextValue {
  bus: EventBus
  apiBase: string
  token: string
  authHeader: Record<string, string>
  apiCall: <T = unknown>(method: HttpMethod, path: string, body?: unknown) => Promise<T | null>
  timeRange: TimeRange
  timeZone: string
  requestTimeRange: RequestTimeRange
  dashboards: DashboardSummary[]
  activeDashboardId: string | null
  widgets: WidgetModel[]
  locked: boolean
  loading: boolean
  setLocked: (locked: boolean) => void
  loadDashboardList: () => Promise<DashboardSummary[]>
  loadDashboard: (dashboardId: string) => Promise<void>
  saveDashboard: (
    dashboardId: string,
    widgetList: WidgetModel[],
    config?: DashboardConfig,
  ) => Promise<void>
  createDashboard: (name: string, id?: string) => Promise<string>
  deleteDashboard: (dashboardId: string) => Promise<void>
  updateWidgets: (widgets: WidgetModel[]) => void
  publishSearch: (targetWidgetId: string, searchData: EventPayload) => void
  broadcastTimeRange: (range: TimeRange) => void
  subscribeToSearch: (widgetId: string, callback: EventListener) => () => void
  subscribeToTimeRange: (widgetId: string, callback: EventListener) => () => void
}

const DashboardContext = createContext<DashboardContextValue | null>(null)

interface DashboardProviderProps {
  children: ReactNode
  apiBase: string
  token: string
  timeRange: TimeRange
  timeZone: string
  requestTimeRange: RequestTimeRange
}

export function DashboardProvider({
  children,
  apiBase,
  token,
  timeRange,
  timeZone,
  requestTimeRange,
}: DashboardProviderProps) {
  const busRef = useRef(new EventBus())
  const [activeDashboardId, setActiveDashboardId] = useState<string | null>(null)
  const [dashboards, setDashboards] = useState<DashboardSummary[]>([])
  const [widgets, setWidgets] = useState<WidgetModel[]>([])
  const [locked, setLocked] = useState(true)
  const [loading, setLoading] = useState(false)
  const saveTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null)

  const authHeader = useMemo<Record<string, string>>(() => {
    const headers: Record<string, string> = {}
    if (token) headers.Authorization = `Bearer ${token}`
    return headers
  }, [token])

  const apiCall = useCallback(
    async <T = unknown>(method: HttpMethod, path: string, body?: unknown): Promise<T | null> => {
      const opts: RequestInit = {
        method,
        headers: { ...authHeader, 'Content-Type': 'application/json' },
      }
      if (body !== undefined) opts.body = JSON.stringify(body)
      const res = await fetch(`${apiBase}${path}`, opts)
      if (res.status === 401) {
        handleUnauthorized()
        throw new Error('Unauthorized')
      }
      if (!res.ok) throw new Error(`API ${method} ${path}: ${res.status}`)
      if (res.status === 204) return null
      return (await res.json()) as T
    },
    [apiBase, authHeader],
  )

  const loadDashboardList = useCallback(async (): Promise<DashboardSummary[]> => {
    try {
      const data = await apiCall<{ data?: { items?: DashboardSummary[] } }>('GET', '/dashboards')
      const items = sortDashboardsByWeight(data?.data?.items || [])
      setDashboards(items)
      return items
    } catch {
      setDashboards([])
      return []
    }
  }, [apiCall])

  const loadDashboard = useCallback(
    async (dashboardId: string): Promise<void> => {
      if (!dashboardId) return
      setLoading(true)
      try {
        const data = await apiCall<{
          data?: { widgets?: WidgetModel[]; config?: DashboardConfig }
        }>('GET', `/dashboards/${encodeURIComponent(dashboardId)}`)
        const el = data?.data || {}
        setWidgets(el.widgets || [])
        setLocked(el.config?.locked === true)
        setActiveDashboardId(dashboardId)
        localStorage.setItem('homer_active_dashboard', dashboardId)
      } catch {
        setWidgets([])
      } finally {
        setLoading(false)
      }
    },
    [apiCall],
  )

  const saveDashboard = useCallback(
    async (
      dashboardId: string,
      widgetList: WidgetModel[],
      config?: DashboardConfig,
    ): Promise<void> => {
      if (!dashboardId) return
      const current = dashboards.find(
        (d) => d.id === dashboardId || d.param === dashboardId,
      )
      const payload = {
        ...(current || {}),
        id: dashboardId,
        param: dashboardId,
        widgets: widgetList,
        config: config || { columns: 12, grid_type: 'fit', locked },
      }
      try {
        await apiCall('PUT', `/dashboards/${encodeURIComponent(dashboardId)}`, payload)
      } catch {
        // silent fail on auto-save
      }
    },
    [apiCall, dashboards, locked],
  )

  const debouncedSave = useCallback(
    (dashboardId: string, widgetList: WidgetModel[]) => {
      if (saveTimerRef.current) clearTimeout(saveTimerRef.current)
      saveTimerRef.current = setTimeout(() => {
        saveDashboard(dashboardId, widgetList)
      }, 1500)
    },
    [saveDashboard],
  )

  const createDashboard = useCallback(
    async (name: string, id?: string): Promise<string> => {
      const dashId = id || name.toLowerCase().replace(/\s+/g, '_')
      const payload = {
        id: dashId,
        param: dashId,
        name,
        type: 1,
        weight: 10,
        shared: false,
        widgets: [],
        config: { columns: 12, grid_type: 'fit', locked: false },
      }
      await apiCall('POST', '/dashboards', payload)
      await loadDashboardList()
      return dashId
    },
    [apiCall, loadDashboardList],
  )

  const deleteDashboard = useCallback(
    async (dashboardId: string): Promise<void> => {
      await apiCall('DELETE', `/dashboards/${encodeURIComponent(dashboardId)}`)
      await loadDashboardList()
      if (activeDashboardId === dashboardId) {
        setActiveDashboardId(null)
        setWidgets([])
        localStorage.removeItem('homer_active_dashboard')
      }
    },
    [apiCall, loadDashboardList, activeDashboardId],
  )

  const updateWidgets = useCallback(
    (newWidgets: WidgetModel[]) => {
      setWidgets(newWidgets)
      if (activeDashboardId) {
        debouncedSave(activeDashboardId, newWidgets)
      }
    },
    [activeDashboardId, debouncedSave],
  )

  const publishSearch = useCallback((targetWidgetId: string, searchData: EventPayload) => {
    busRef.current.emit('search', targetWidgetId, searchData)
  }, [])

  const broadcastTimeRange = useCallback((range: TimeRange) => {
    busRef.current.broadcast('timerange', range)
  }, [])

  // Broadcast time range whenever it changes so widgets can react (e.g. re-run search)
  useEffect(() => {
    if (timeRange?.from != null && timeRange?.to != null) {
      busRef.current.broadcast('timerange', timeRange)
    }
  }, [timeRange?.from, timeRange?.to, timeRange?.activePreset, timeRange?.calendarPreset])

  const subscribeToSearch = useCallback((widgetId: string, callback: EventListener) => {
    return busRef.current.on('search', widgetId, callback)
  }, [])

  const subscribeToTimeRange = useCallback((widgetId: string, callback: EventListener) => {
    return busRef.current.on('timerange', widgetId, callback)
  }, [])

  const value = useMemo<DashboardContextValue>(
    () => ({
      bus: busRef.current,
      apiBase,
      token,
      authHeader,
      apiCall,
      timeRange,
      timeZone,
      requestTimeRange,
      dashboards,
      activeDashboardId,
      widgets,
      locked,
      loading,
      setLocked,
      loadDashboardList,
      loadDashboard,
      saveDashboard,
      createDashboard,
      deleteDashboard,
      updateWidgets,
      publishSearch,
      broadcastTimeRange,
      subscribeToSearch,
      subscribeToTimeRange,
    }),
    [
      apiBase,
      token,
      authHeader,
      apiCall,
      timeRange,
      timeZone,
      requestTimeRange,
      dashboards,
      activeDashboardId,
      widgets,
      locked,
      loading,
      loadDashboardList,
      loadDashboard,
      saveDashboard,
      createDashboard,
      deleteDashboard,
      updateWidgets,
      publishSearch,
      broadcastTimeRange,
      subscribeToSearch,
      subscribeToTimeRange,
    ],
  )

  return <DashboardContext.Provider value={value}>{children}</DashboardContext.Provider>
}

// eslint-disable-next-line react-refresh/only-export-components
export function useDashboard(): DashboardContextValue {
  const ctx = useContext(DashboardContext)
  if (!ctx) throw new Error('useDashboard must be used inside DashboardProvider')
  return ctx
}

// eslint-disable-next-line react-refresh/only-export-components
export function useWidgetSearch<T = EventPayload>(widgetId: string): T | null {
  const { subscribeToSearch } = useDashboard()
  const [searchData, setSearchData] = useState<T | null>(null)

  useEffect(() => {
    if (!widgetId) return
    const unsub = subscribeToSearch(widgetId, (data) => {
      setSearchData({ ...(data as object), _ts: Date.now() } as T)
    })
    return unsub
  }, [widgetId, subscribeToSearch])

  return searchData
}
