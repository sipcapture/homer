import { useState, useRef, useCallback, useEffect } from 'react'
import { GripVertical } from 'lucide-react'
import { cn } from '@/lib/utils'
import type { FieldItem } from '@/hooks/useMappings'

interface DragDropFieldListProps {
  /** Full list of fields for the selected protocol */
  fields: FieldItem[]
  /** IDs of currently active (selected) fields, in display order */
  activeFieldIds?: string[]
  onChange: (updated: FieldItem[]) => void
}

type ListId = 'active' | 'inactive'

interface DragState {
  fieldId: string
  sourceList: ListId
}

export default function DragDropFieldList({
  fields,
  activeFieldIds,
  onChange,
}: DragDropFieldListProps) {
  const buildLists = useCallback(
    (allFields: FieldItem[], activeIds?: string[]) => {
      const visible = allFields.filter((f) => !f.skip)
      if (activeIds && activeIds.length > 0) {
        const activeSet = new Set(activeIds)
        const active: FieldItem[] = []
        // preserve order from activeIds
        activeIds.forEach((id) => {
          const f = visible.find((f) => f.id === id)
          if (f) active.push({ ...f, selected: true })
        })
        const inactive = visible
          .filter((f) => !activeSet.has(f.id))
          .map((f) => ({ ...f, selected: false }))
        return { active, inactive }
      }
      // fall back to field's own selected flag
      const active = visible.filter((f) => f.selected).map((f) => ({ ...f, selected: true }))
      const inactive = visible
        .filter((f) => !f.selected)
        .map((f) => ({ ...f, selected: false }))
      return { active, inactive }
    },
    [],
  )

  const [activeList, setActiveList] = useState<FieldItem[]>(() => buildLists(fields, activeFieldIds).active)
  const [inactiveList, setInactiveList] = useState<FieldItem[]>(() => buildLists(fields, activeFieldIds).inactive)

  // Re-build lists when the underlying fields prop changes (protocol switch)
  useEffect(() => {
    const { active, inactive } = buildLists(fields, activeFieldIds)
    setActiveList(active)
    setInactiveList(inactive)
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [fields, buildLists])

  const dragState = useRef<DragState | null>(null)
  const [dragOver, setDragOver] = useState<{ listId: ListId; index: number } | null>(null)

  const emitChange = useCallback(
    (newActive: FieldItem[], newInactive: FieldItem[]) => {
      onChange([
        ...newActive.map((f) => ({ ...f, selected: true })),
        ...newInactive.map((f) => ({ ...f, selected: false })),
      ])
    },
    [onChange],
  )

  const handleDragStart = (fieldId: string, sourceList: ListId) => {
    dragState.current = { fieldId, sourceList }
  }

  const handleDragOver = (e: React.DragEvent, listId: ListId, index: number) => {
    e.preventDefault()
    setDragOver({ listId, index })
  }

  const handleDrop = (e: React.DragEvent, targetList: ListId, targetIndex: number) => {
    e.preventDefault()
    setDragOver(null)
    const ds = dragState.current
    if (!ds) return

    const srcItems = ds.sourceList === 'active' ? [...activeList] : [...inactiveList]
    const dstItems = targetList === 'active' ? [...activeList] : [...inactiveList]
    const srcIndex = srcItems.findIndex((f) => f.id === ds.fieldId)
    if (srcIndex === -1) return

    const [moved] = srcItems.splice(srcIndex, 1)

    if (ds.sourceList === targetList) {
      srcItems.splice(targetIndex, 0, moved)
      if (targetList === 'active') {
        setActiveList(srcItems)
        emitChange(srcItems, inactiveList)
      } else {
        setInactiveList(srcItems)
        emitChange(activeList, srcItems)
      }
    } else {
      // cross-list transfer
      const actualDst = ds.sourceList === 'active' ? dstItems : dstItems
      actualDst.splice(targetIndex, 0, moved)
      const newActive = ds.sourceList === 'active' ? srcItems : actualDst
      const newInactive = ds.sourceList === 'inactive' ? srcItems : actualDst
      setActiveList(newActive)
      setInactiveList(newInactive)
      emitChange(newActive, newInactive)
    }

    dragState.current = null
  }

  const handleDragEnd = () => {
    dragState.current = null
    setDragOver(null)
  }

  const renderList = (list: FieldItem[], listId: ListId) => (
    <div
      className={cn(
        'flex min-h-64 flex-col gap-1 rounded-md border border-dashed border-border/80 bg-muted/20 p-2',
        dragOver?.listId === listId && dragOver.index === list.length && 'ring-1 ring-primary',
      )}
      onDragOver={(e) => {
        e.preventDefault()
        e.dataTransfer.dropEffect = 'move'
        if (list.length === 0) setDragOver({ listId, index: 0 })
      }}
      onDrop={(e) => {
        if (list.length === 0) handleDrop(e, listId, 0)
      }}
    >
      {list.map((field, idx) => {
        const isOver = dragOver?.listId === listId && dragOver.index === idx
        return (
          <div
            key={field.id}
            draggable
            onDragStart={() => handleDragStart(field.id, listId)}
            onDragEnd={handleDragEnd}
            onDragOver={(e) => {
              e.preventDefault()
              e.stopPropagation()
              handleDragOver(e, listId, idx)
            }}
            onDrop={(e) => {
              e.stopPropagation()
              handleDrop(e, listId, idx)
            }}
            className={cn(
              'flex cursor-grab items-center gap-2 rounded border border-border bg-card px-2 py-1.5 text-xs select-none',
              'hover:border-primary/50 hover:bg-accent',
              isOver && 'border-primary ring-1 ring-primary',
            )}
          >
            <GripVertical className="h-3 w-3 shrink-0 text-muted-foreground" />
            <span className="truncate">{field.name || field.id}</span>
          </div>
        )
      })}
      {/* Tall tail drop target — end of list (and easier hit than a 1px gap between rows) */}
      {list.length > 0 && (
        <div
          className={cn(
            'mt-1 flex min-h-14 flex-1 shrink-0 items-center justify-center rounded border border-dashed border-transparent px-2 text-[11px] text-muted-foreground/80',
            dragOver?.listId === listId && dragOver.index === list.length && 'border-primary/60 bg-accent/25 text-foreground',
          )}
          onDragOver={(e) => {
            e.preventDefault()
            e.stopPropagation()
            setDragOver({ listId, index: list.length })
          }}
          onDrop={(e) => {
            e.stopPropagation()
            handleDrop(e, listId, list.length)
          }}
        >
          Drop at end
        </div>
      )}
      {list.length === 0 && (
        <div
          className={cn(
            'flex min-h-44 flex-1 flex-col items-center justify-center rounded border border-dashed border-border/80 px-3 py-8 text-xs text-muted-foreground',
            dragOver?.listId === listId && 'border-primary bg-accent/30 text-foreground',
          )}
          onDragOver={(e) => {
            e.preventDefault()
            e.stopPropagation()
            setDragOver({ listId, index: 0 })
          }}
          onDrop={(e) => {
            e.stopPropagation()
            handleDrop(e, listId, 0)
          }}
        >
          Drop fields here
        </div>
      )}
    </div>
  )

  return (
    <div className="grid grid-cols-2 gap-3">
      <div>
        <p className="mb-1.5 text-xs font-medium text-muted-foreground">Inactive</p>
        {renderList(inactiveList, 'inactive')}
      </div>
      <div>
        <p className="mb-1.5 text-xs font-medium text-muted-foreground">Active</p>
        {renderList(activeList, 'active')}
      </div>
    </div>
  )
}
