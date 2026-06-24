import { Suspense, useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useConfirm } from '@/components/ui/confirm-dialog'
import { Lock, Plus, Settings, Unlock } from 'lucide-react'
import { ResponsiveGridLayout } from 'react-grid-layout'
import 'react-grid-layout/css/styles.css'
import 'react-resizable/css/styles.css'
import DashboardHeader from './DashboardHeader'
import { DashboardProvider, useDashboard } from './context/DashboardContext'
import PanelChrome from './components/PanelChrome'
import AddWidgetDialog from './components/AddWidgetDialog'
import DashboardSettingsDialog from './components/DashboardSettingsDialog'
import SearchWidgetSettings from './components/SearchWidgetSettings'
import { getWidgetMeta, isWidgetCategoryEnabled } from './widgets/registry'
import { useModules } from '@/hooks/useModules'
import { GRID_ROW_HEIGHT, GRID_MARGIN, computeAvailableRows, fitWidgetHeight } from './utils/grid-utils'
import { resolveTimeRange, type CalendarPreset } from './utils/resolveTimeRange'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Separator } from '@/components/ui/separator'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'
import { cn } from '@/lib/utils'
import SearchDeepLinkBootstrap from './SearchDeepLinkBootstrap'

function DashboardGrid() {
  const confirm = useConfirm()
  const ctx = useDashboard()
  const {
    dashboards, activeDashboardId, widgets, locked, loading,
    loadDashboardList, loadDashboard, createDashboard, deleteDashboard,
    updateWidgets, setLocked,
  } = ctx

  const [addOpen, setAddOpen] = useState(false)
  const [settingsOpen, setSettingsOpen] = useState(false)
  const [settingsWidgetId, setSettingsWidgetId] = useState<string | null>(null)
  const [createInput, setCreateInput] = useState('')
  const [creating, setCreating] = useState(false)
  const [editingTabId, setEditingTabId] = useState<string | null>(null)
  const [editTabName, setEditTabName] = useState('')
  const containerRef = useRef<HTMLDivElement | null>(null)
  const [containerWidth, setContainerWidth] = useState(0)
  const [containerHeight, setContainerHeight] = useState(0)
  const widgetsRef = useRef(widgets)
  const skipLayoutChangeRef = useRef(false)
  const { widgetControl } = useModules()

  useEffect(() => { widgetsRef.current = widgets }, [widgets])

  useEffect(() => {
    const el = containerRef.current
    if (!el) return
    setContainerWidth(el.offsetWidth)
    setContainerHeight(el.offsetHeight)
    const ro = new ResizeObserver((entries) => {
      for (const entry of entries) {
        setContainerWidth(entry.contentRect.width)
        setContainerHeight(entry.contentRect.height)
      }
    })
    ro.observe(el)
    return () => ro.disconnect()
  }, [])

  useEffect(() => {
    loadDashboardList().then((items) => {
      if (items.length > 0 && !activeDashboardId) {
        const savedId = localStorage.getItem('homer_active_dashboard')
        const target = savedId && items.find(d => (d.id || d.param) === savedId)
        const fallback = items[0]
        const chosenId = (target ? target.id || target.param : fallback.id || fallback.param)
        if (chosenId) loadDashboard(chosenId)
      }
    })
  }, [])

  const layout = useMemo(() =>
    widgets.map((w) => {
      const meta = getWidgetMeta(w.type)
      return {
        i: w.id,
        x: w.x ?? 0, y: w.y ?? 0,
        w: w.w ?? meta?.defaultW ?? 4,
        h: w.h ?? meta?.defaultH ?? 3,
        minW: meta?.minW ?? 2, minH: meta?.minH ?? 2,
        static: locked,
      }
    }), [widgets, locked])

  const handleLayoutChange = useCallback((newLayout) => {
    if (skipLayoutChangeRef.current) {
      skipLayoutChangeRef.current = false
      return
    }
    const current = widgetsRef.current
    let changed = false
    const updated = current.map((w) => {
      const item = newLayout.find((l) => l.i === w.id)
      if (item && (w.x !== item.x || w.y !== item.y || w.w !== item.w || w.h !== item.h)) {
        changed = true
        return { ...w, x: item.x, y: item.y, w: item.w, h: item.h }
      }
      return w
    })
    if (changed) updateWidgets(updated)
  }, [updateWidgets])

  const availableRows = computeAvailableRows(containerHeight)

  const handleAddWidget = (widget) => {
    skipLayoutChangeRef.current = true
    if (locked) setLocked(false)
    const meta = getWidgetMeta(widget.type)
    const clampedH = fitWidgetHeight(widget.h ?? meta?.defaultH ?? 3, meta?.minH ?? 2, availableRows)
    updateWidgets([...widgets, { ...widget, h: clampedH }])
  }

  const handleRemoveWidget = (widgetId) => {
    skipLayoutChangeRef.current = true
    updateWidgets(widgets.filter((w) => w.id !== widgetId))
  }

  const handleDuplicateWidget = (widget) => {
    skipLayoutChangeRef.current = true
    const meta = getWidgetMeta(widget.type)
    const clampedH = fitWidgetHeight(widget.h ?? meta?.defaultH ?? 3, meta?.minH ?? 2, availableRows)
    const copy = { ...widget, id: `${widget.type}-${Date.now()}`, x: 0, y: Infinity, h: clampedH }
    updateWidgets([...widgets, copy])
  }

  const handleWidgetConfigChange = (widgetId, config) => {
    updateWidgets(widgets.map((w) => w.id === widgetId ? { ...w, config: { ...w.config, ...config } } : w))
  }

  const handleWidgetTitleChange = (widgetId, title) => {
    updateWidgets(widgets.map((w) => w.id === widgetId ? { ...w, title } : w))
  }

  const handleCreateDashboard = async () => {
    if (!createInput.trim()) return
    setCreating(true)
    try {
      const id = await createDashboard(createInput.trim())
      setCreateInput('')
      await loadDashboard(id)
    } finally {
      setCreating(false)
    }
  }

  const handleDeleteDashboard = async () => {
    if (!activeDashboardId) return
    if (!(await confirm({ message: 'Delete this dashboard?', variant: 'destructive' }))) return
    await deleteDashboard(activeDashboardId)
    const remaining = dashboards.filter((d) => (d.id || d.param) !== activeDashboardId)
    if (remaining.length > 0) {
      const nextId = remaining[0].id || remaining[0].param
      if (nextId) await loadDashboard(nextId)
    }
  }

  const commitTabRename = async () => {
    const id = editingTabId
    setEditingTabId(null)
    if (!id || !editTabName.trim()) return
    const dash = dashboards.find((d) => (d.id || d.param) === id)
    if (!dash || dash.name === editTabName.trim()) return
    try {
      await ctx.apiCall('PUT', `/dashboards/${encodeURIComponent(id)}`, {
        ...dash,
        name: editTabName.trim(),
      })
      await loadDashboardList()
    } catch { /* ignore */ }
  }

  const activeDash = dashboards.find((d) => (d.id || d.param) === activeDashboardId)

  const handleSettingsSave = async (updated) => {
    if (!activeDashboardId) return
    const dash = dashboards.find((d) => (d.id || d.param) === activeDashboardId)
    await ctx.apiCall('PUT', `/dashboards/${encodeURIComponent(activeDashboardId)}`, {
      ...dash,
      name: updated.name ?? dash?.name,
      shared: updated.shared ?? dash?.shared,
      widgets,
      config: updated.config || dash?.config,
    })
    await loadDashboardList()
  }

  return (
    <div className="flex h-full min-h-0 flex-col" ref={containerRef}>
      {/* Dashboard tab bar */}
      <div className="flex shrink-0 items-center gap-2 border-b border-border bg-card/60 px-3 py-1.5 backdrop-blur-sm">
        <div className="flex min-w-0 flex-1 items-center gap-1 overflow-x-auto">
          {dashboards.map((d) => {
            const id = d.id || d.param
            const active = id === activeDashboardId
            if (editingTabId === id) {
              return (
                <Input
                  key={id}
                  value={editTabName}
                  onChange={(e) => setEditTabName(e.target.value)}
                  onBlur={commitTabRename}
                  onKeyDown={(e) => {
                    if (e.key === 'Enter') commitTabRename()
                    if (e.key === 'Escape') setEditingTabId(null)
                  }}
                  autoFocus
                  className="h-7 w-32 shrink-0 text-xs"
                />
              )
            }
            return (
              <button
                key={id}
                type="button"
                onClick={() => id && loadDashboard(id)}
                onDoubleClick={() => {
                  if (!id) return
                  setEditingTabId(id)
                  setEditTabName(d.name || id)
                }}
                className={cn(
                  'inline-flex h-8 shrink-0 items-center gap-1.5 border-b-2 px-3 text-xs font-medium transition-colors',
                  active
                    ? 'border-primary text-foreground'
                    : 'border-transparent text-muted-foreground hover:text-foreground',
                )}
              >
                {d.name || id}
              </button>
            )
          })}
        </div>
        <div className="flex items-center gap-1.5">
          <Input
            value={createInput}
            onChange={(e) => setCreateInput(e.target.value)}
            placeholder="New dashboard..."
            onKeyDown={(e) => { if (e.key === 'Enter') handleCreateDashboard() }}
            className="h-8 w-44 text-xs"
          />
          <Tooltip>
            <TooltipTrigger asChild>
              <Button
                variant="outline"
                size="icon-sm"
                onClick={handleCreateDashboard}
                disabled={creating || !createInput.trim()}
                aria-label="Create dashboard"
              >
                <Plus className="h-4 w-4" />
              </Button>
            </TooltipTrigger>
            <TooltipContent>Create dashboard</TooltipContent>
          </Tooltip>
          {activeDashboardId && (
            <>
              <Separator orientation="vertical" className="mx-1 h-5" />
              <Tooltip>
                <TooltipTrigger asChild>
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={() => setAddOpen(true)}
                    aria-label="Add widget"
                  >
                    <Plus className="mr-1 h-4 w-4" />
                    Widget
                  </Button>
                </TooltipTrigger>
                <TooltipContent>Add widget</TooltipContent>
              </Tooltip>
              <Tooltip>
                <TooltipTrigger asChild>
                  <Button
                    variant={locked ? 'outline' : 'secondary'}
                    size="sm"
                    onClick={() => setLocked(!locked)}
                    aria-label={locked ? 'Unlock layout' : 'Lock layout'}
                  >
                    {locked ? (
                      <Lock className="mr-1 h-4 w-4" />
                    ) : (
                      <Unlock className="mr-1 h-4 w-4" />
                    )}
                    {locked ? 'Locked' : 'Editing'}
                  </Button>
                </TooltipTrigger>
                <TooltipContent>{locked ? 'Unlock layout for editing' : 'Lock layout'}</TooltipContent>
              </Tooltip>
              <Tooltip>
                <TooltipTrigger asChild>
                  <Button
                    variant="outline"
                    size="icon-sm"
                    onClick={() => setSettingsOpen(true)}
                    aria-label="Dashboard settings"
                  >
                    <Settings className="h-4 w-4" />
                  </Button>
                </TooltipTrigger>
                <TooltipContent>Dashboard settings</TooltipContent>
              </Tooltip>
            </>
          )}
        </div>
      </div>

      {/* Widget grid */}
      <div className="relative flex-1 overflow-auto p-3">
        {loading && (
          <div className="flex h-full items-center justify-center text-sm text-muted-foreground">
            Loading dashboard...
          </div>
        )}
        {!loading && !activeDashboardId && (
          <div className="flex h-full flex-col items-center justify-center gap-2 text-sm text-muted-foreground">
            <p>No dashboards available. Create one above or reset to defaults.</p>
          </div>
        )}
        {!loading && activeDashboardId && widgets.length === 0 && (
          <div className="flex h-full flex-col items-center justify-center gap-3 text-sm text-muted-foreground">
            <p>Dashboard is empty.</p>
            <Button onClick={() => setAddOpen(true)}>
              <Plus className="mr-1 h-4 w-4" />
              Add Widget
            </Button>
          </div>
        )}
        {!loading && activeDashboardId && widgets.length > 0 && containerWidth > 0 && (
          <ResponsiveGridLayout
            className="layout"
            layouts={{ lg: layout }}
            breakpoints={{ lg: 1200, md: 996, sm: 768, xs: 480 }}
            cols={{ lg: 12, md: 10, sm: 6, xs: 4 }}
            rowHeight={GRID_ROW_HEIGHT}
            width={containerWidth - 24}
            dragConfig={{ enabled: !locked, handle: '.widget-drag-handle' }}
            resizeConfig={{ enabled: !locked }}
            onLayoutChange={handleLayoutChange}
          >
            {widgets.map((widget) => {
              const meta = getWidgetMeta(widget.type)
              const WidgetComponent = meta?.component
              const categoryDisabled =
                meta?.category != null &&
                !isWidgetCategoryEnabled(widgetControl, meta.category)
              return (
                <div key={widget.id}>
                  <PanelChrome
                    title={widget.title || meta?.label || widget.type}
                    icon={meta?.icon}
                    locked={locked}
                    onRemove={() => handleRemoveWidget(widget.id)}
                    onDuplicate={() => handleDuplicateWidget(widget)}
                    onTitleChange={(t) => handleWidgetTitleChange(widget.id, t)}
                    onSettings={
                      widget.type === 'search' || widget.type === 'protosearch'
                        ? () => setSettingsWidgetId(widget.id)
                        : undefined
                    }
                  >
                    <Suspense
                      fallback={
                        <div className="flex h-full items-center justify-center text-sm text-muted-foreground">
                          Loading...
                        </div>
                      }
                    >
                      {categoryDisabled ? (
                        <div className="flex h-full items-center justify-center px-4 text-center text-sm text-muted-foreground">
                          This widget category is disabled on this server ({meta?.category}).
                        </div>
                      ) : WidgetComponent ? (
                        <WidgetComponent
                          widgetId={widget.id}
                          config={widget.config || {}}
                          onConfigChange={(cfg) => handleWidgetConfigChange(widget.id, cfg)}
                        />
                      ) : (
                        <div className="flex h-full items-center justify-center text-sm text-muted-foreground">
                          Unknown widget: {widget.type}
                        </div>
                      )}
                    </Suspense>
                  </PanelChrome>
                </div>
              )
            })}
          </ResponsiveGridLayout>
        )}
      </div>

      {addOpen && (
        <AddWidgetDialog
          onAdd={handleAddWidget}
          onClose={() => setAddOpen(false)}
          widgetControl={widgetControl}
        />
      )}
      {settingsOpen && activeDash && (
        <DashboardSettingsDialog
          dashboard={activeDash}
          onSave={handleSettingsSave}
          onClose={() => setSettingsOpen(false)}
          onDelete={handleDeleteDashboard}
        />
      )}
      {settingsWidgetId && (() => {
        const sw = widgets.find((w) => w.id === settingsWidgetId)
        if (!sw) return null
        return (
          <SearchWidgetSettings
            open={true}
            onClose={() => setSettingsWidgetId(null)}
            config={sw.config || {}}
            onSave={(newConfig) => {
              handleWidgetConfigChange(settingsWidgetId, newConfig)
              setSettingsWidgetId(null)
            }}
          />
        )
      })()}
    </div>
  )
}

export interface DashboardLayoutProps {
  apiBase: string
  token: string
  me: { username?: string; display_name?: string } | null
  userAvatarUrl?: string | null
  onOpenSettings: () => void
  onOpenDashboard: () => void
  onLogout: () => void
  /** Opens Settings focused on the Reset section (browser + server actions). */
  onOpenResetSettings?: () => void
}

export default function DashboardLayout({
  apiBase,
  token,
  me,
  userAvatarUrl = null,
  onOpenSettings,
  onOpenDashboard,
  onLogout,
  onOpenResetSettings,
}: DashboardLayoutProps) {
  const [timeFromMs, setTimeFromMs] = useState(() => Date.now() - 60 * 60 * 1000)
  const [timeToMs, setTimeToMs] = useState(() => Date.now())
  const [activePreset, setActivePreset] = useState<number | null>(60)
  const [calendarPreset, setCalendarPreset] = useState<CalendarPreset | null>(null)
  const [timeZone, setTimeZone] = useState('local')

  const handleTimeChange = useCallback((
    minutes?: number | null,
    fromMs?: number,
    toMs?: number,
    opts?: { calendar?: CalendarPreset },
  ) => {
    const cal = opts?.calendar
    if (
      cal === 'today' ||
      cal === 'yesterday' ||
      cal === 'this_month' ||
      cal === 'last_month'
    ) {
      setCalendarPreset(cal)
      setActivePreset(null)
      const r = resolveTimeRange({ calendarPreset: cal }, timeZone)
      if (r) {
        setTimeFromMs(r.from)
        setTimeToMs(r.to)
      }
      return
    }
    if (typeof minutes === 'number' && Number.isFinite(minutes) && minutes > 0) {
      const now = Date.now()
      setTimeFromMs(now - minutes * 60 * 1000)
      setTimeToMs(now)
      setActivePreset(minutes)
      setCalendarPreset(null)
      return
    }
    if (fromMs != null && toMs != null && fromMs > 0 && toMs > 0 && toMs > fromMs) {
      setTimeFromMs(fromMs)
      setTimeToMs(toMs)
      setActivePreset(null)
      setCalendarPreset(null)
    }
  }, [timeZone])

  const timeRange = useMemo(() => ({
    from: timeFromMs,
    to: timeToMs,
    activePreset,
    calendarPreset,
  }), [timeFromMs, timeToMs, activePreset, calendarPreset])

  return (
    <div className="flex h-[calc(100vh-var(--dock-h,0px))] flex-col overflow-hidden bg-background text-foreground">
        <DashboardHeader
          timeFrom={timeFromMs}
          timeTo={timeToMs}
          onTimeChange={handleTimeChange}
          activePreset={activePreset}
          calendarPreset={calendarPreset}
          timeZone={timeZone}
        onTimeZoneChange={setTimeZone}
        userLabel={me?.display_name || me?.username || 'User'}
        userAvatarUrl={userAvatarUrl}
        onOpenSettings={onOpenSettings}
        onOpenDashboard={onOpenDashboard}
        onLogout={onLogout}
        onOpenResetSettings={onOpenResetSettings}
      />
      <main className="flex min-h-0 flex-1 flex-col">
        <DashboardProvider
          apiBase={apiBase}
          token={token}
          timeRange={timeRange}
          timeZone={timeZone}
          requestTimeRange={handleTimeChange}
        >
          <SearchDeepLinkBootstrap />
          <DashboardGrid />
        </DashboardProvider>
      </main>
    </div>
  )
}
