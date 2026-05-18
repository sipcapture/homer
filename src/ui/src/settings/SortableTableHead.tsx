import { type CSSProperties, type ReactNode } from 'react'
import { ArrowDown, ArrowUp, ArrowUpDown } from 'lucide-react'
import { TableHead } from '@/components/ui/table'
import { cn } from '@/lib/utils'
import type { SortDirection } from './tableSort'

export function SortableTableHead({
  columnKey,
  label,
  sortCol,
  sortDir,
  onSort,
  className,
  style,
}: {
  columnKey: string
  label: ReactNode
  sortCol: string | null
  sortDir: SortDirection
  onSort: (key: string) => void
  className?: string
  style?: CSSProperties
}) {
  const active = sortCol === columnKey
  return (
    <TableHead className={className} style={style}>
      <button
        type="button"
        className={cn(
          'inline-flex cursor-pointer select-none items-center gap-1 border-0 bg-transparent p-0 font-inherit text-inherit',
          'hover:text-foreground',
        )}
        title={`Sort by ${typeof label === 'string' ? label : columnKey}`}
        onClick={() => onSort(columnKey)}
      >
        {label}
        {active ? (
          sortDir === 'asc' ? (
            <ArrowUp className="size-3 shrink-0" />
          ) : (
            <ArrowDown className="size-3 shrink-0" />
          )
        ) : (
          <ArrowUpDown className="size-3 shrink-0 opacity-45 dark:opacity-50" />
        )}
      </button>
    </TableHead>
  )
}
