import { useMemo, useState } from 'react'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { ScrollArea } from '@/components/ui/scroll-area'
import { cn } from '@/lib/utils'
import {
  isWidgetCategoryEnabled,
  widgetCategories,
  widgetRegistry,
} from '../widgets/registry'
import { PanelIcon } from './PanelChrome'

/**
 * Picker entry; can be either a base widget type or a preset (extra)
 * variant of that type with a pre-populated `config`.
 */
interface PickerEntry {
  /** React key — for base entries this is the registry type, for extras it
   * is the extra.id (so multiple cards over the same base type don't collide). */
  key: string
  /** Widget type to instantiate — same for base and its extras. */
  type: string
  label: string
  icon: string
  defaultW: number
  defaultH: number
  defaultConfig: Record<string, unknown>
}

interface AddWidgetDialogProps {
  onAdd: (widget: {
    id: string
    type: string
    x: number
    y: number
    w: number
    h: number
    title: string
    config: Record<string, unknown>
  }) => void
  onClose: () => void
  widgetControl?: Record<string, boolean>
}

export default function AddWidgetDialog({ onAdd, onClose, widgetControl }: AddWidgetDialogProps) {
  const visibleCategories = useMemo(
    () =>
      widgetCategories.filter((cat) => isWidgetCategoryEnabled(widgetControl, cat)),
    [widgetControl],
  )
  const [activeCategory, setActiveCategory] = useState(visibleCategories[0] ?? 'Search')

  const handleAdd = (entry: PickerEntry) => {
    onAdd({
      id: `${entry.type}-${Date.now()}`,
      type: entry.type,
      x: 0,
      y: Infinity,
      w: entry.defaultW,
      h: entry.defaultH,
      title: entry.label,
      config: { ...entry.defaultConfig },
    })
    onClose()
  }

  return (
    <Dialog open onOpenChange={(open) => { if (!open) onClose() }}>
      <DialogContent className="!max-w-2xl gap-3">
        <DialogHeader>
          <DialogTitle>Add Widget</DialogTitle>
        </DialogHeader>
        <Tabs value={activeCategory} onValueChange={setActiveCategory}>
          <TabsList className="w-full">
            {visibleCategories.map((cat) => (
              <TabsTrigger key={cat} value={cat}>
                {cat}
              </TabsTrigger>
            ))}
          </TabsList>
          {visibleCategories.map((cat) => {
            const entries: PickerEntry[] = []
            Object.entries(widgetRegistry).forEach(([type, meta]) => {
              if (meta.category !== cat || meta.hidden) return
              entries.push({
                key: type,
                type,
                label: meta.label,
                icon: meta.icon,
                defaultW: meta.defaultW,
                defaultH: meta.defaultH,
                defaultConfig: {},
              })
              meta.extras?.forEach((extra) => {
                entries.push({
                  key: extra.id,
                  type,
                  label: extra.label,
                  icon: extra.icon,
                  defaultW: extra.defaultW ?? meta.defaultW,
                  defaultH: extra.defaultH ?? meta.defaultH,
                  defaultConfig: extra.defaultConfig,
                })
              })
            })
            return (
              <TabsContent key={cat} value={cat} className="mt-2">
                <ScrollArea className="h-[360px] pr-2">
                  {entries.length === 0 ? (
                    <div className="flex h-32 items-center justify-center text-xs text-muted-foreground">
                      No widgets in this category
                    </div>
                  ) : (
                    <div className="grid grid-cols-2 gap-2">
                      {entries.map((entry) => (
                        <button
                          key={entry.key}
                          type="button"
                          onClick={() => handleAdd(entry)}
                          className={cn(
                            'group flex items-center gap-3 border border-border bg-card p-3 text-left transition-colors hover:bg-accent hover:text-accent-foreground focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring',
                          )}
                        >
                          <div className="flex h-9 w-9 shrink-0 items-center justify-center border border-border bg-muted text-muted-foreground group-hover:text-accent-foreground">
                            <PanelIcon name={entry.icon} size={18} />
                          </div>
                          <div className="flex min-w-0 flex-1 flex-col">
                            <span className="truncate text-xs font-medium text-foreground group-hover:text-accent-foreground">
                              {entry.label}
                            </span>
                            <span className="text-[10px] text-muted-foreground">
                              {entry.defaultW}×{entry.defaultH}
                            </span>
                          </div>
                        </button>
                      ))}
                    </div>
                  )}
                </ScrollArea>
              </TabsContent>
            )
          })}
        </Tabs>
      </DialogContent>
    </Dialog>
  )
}
