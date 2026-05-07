import { type ChangeEvent, type ReactNode, useId, useState } from 'react'
import { BoolIndicator } from '@/components/ui/bool-indicator'
import { Plus, RefreshCw } from 'lucide-react'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
} from '@/components/ui/card'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { cn } from '@/lib/utils'
import { SettingsPageHeader } from './SettingsPageHeader'

export interface CrudColumn {
  key: string
  label: string
  width?: string
  type?: 'bool' | 'json'
  /** When set, replaces default text/bool/json formatting for this column */
  render?: (value: unknown, row: Record<string, unknown>) => ReactNode
}

interface CrudTableProps {
  title: string
  description?: string
  columns: CrudColumn[]
  items: any[]
  loading?: boolean
  onLoad: () => void
  onCreateOpen?: () => void
  filters?: ReactNode
  editForm?: ReactNode
  createForm?: ReactNode
  idField?: string
  showActions?: boolean
  children?: (item: any) => ReactNode
  error?: string
}

export default function CrudTable({
  title,
  description,
  columns,
  items,
  loading,
  onLoad,
  onCreateOpen,
  filters,
  editForm,
  createForm,
  idField = 'guid',
  showActions = true,
  children,
  error,
}: CrudTableProps) {
  return (
    <div className="space-y-6">
      <SettingsPageHeader
        title={title}
        description={description}
        actions={
          <>
            <Button variant="outline" size="sm" onClick={onLoad} disabled={loading}>
              <RefreshCw className={cn('mr-1.5 size-4', loading && 'animate-spin')} />
              Refresh
            </Button>
            {onCreateOpen && (
              <Button size="sm" onClick={onCreateOpen}>
                <Plus className="mr-1.5 size-4" />
                Create
              </Button>
            )}
          </>
        }
      />

      {error && (
        <div className="rounded-none border border-destructive/40 bg-destructive/10 px-3 py-2 text-sm text-destructive">
          {error}
        </div>
      )}

      {filters && (
        <Card>
          <CardContent>
            <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 md:grid-cols-3 lg:grid-cols-4">
              {filters}
            </div>
          </CardContent>
        </Card>
      )}

      {editForm}

      <Card>
        <CardContent className="p-0">
          {items.length === 0 ? (
            <p className="p-6 text-sm text-muted-foreground">
              {loading ? 'Loading...' : 'No items loaded.'}
            </p>
          ) : (
            <div className="overflow-x-auto">
              <Table>
                <TableHeader>
                  <TableRow>
                    {columns.map((col) => (
                      <TableHead
                        key={col.key}
                        style={col.width ? { width: col.width } : undefined}
                      >
                        {col.label}
                      </TableHead>
                    ))}
                    {showActions && (
                      <TableHead className="w-[140px] text-right">Actions</TableHead>
                    )}
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {items.map((item) => (
                    <TableRow key={item[idField] || JSON.stringify(item)}>
                      {columns.map((col) => (
                        <TableCell
                          key={col.key}
                          title={cellTitle(item[col.key], col)}
                          className={cn(
                            'max-w-[280px]',
                            col.type !== 'bool' && !col.render && 'truncate',
                            col.type === 'json' && 'font-mono text-[11px]',
                            col.type === 'bool' && 'w-[80px]',
                          )}
                        >
                          {col.render
                            ? col.render(item[col.key], item)
                            : renderCell(item[col.key], col)}
                        </TableCell>
                      ))}
                      {showActions && (
                        <TableCell>
                          <div className="flex flex-wrap items-center justify-end gap-1.5">
                            {children?.(item)}
                          </div>
                        </TableCell>
                      )}
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </div>
          )}
        </CardContent>
      </Card>

      {createForm}
    </div>
  )
}

function cellTitle(value: unknown, col: CrudColumn): string {
  if (col.type === 'bool') {
    if (value === null || value === undefined) return ''
    return value ? 'Enabled' : 'Disabled'
  }
  return formatCell(value, col)
}

function renderCell(value: unknown, col: CrudColumn): ReactNode {
  if (col.type === 'bool') {
    return <BoolIndicator value={value as boolean | null | undefined} />
  }
  return formatCell(value, col)
}

function formatCell(value: unknown, col: CrudColumn): string {
  if (value === null || value === undefined) return '—'
  if (col.type === 'json') {
    if (typeof value === 'object') {
      const str = JSON.stringify(value)
      return str.length > 60 ? str.slice(0, 57) + '...' : str
    }
    return String(value)
  }
  return String(value)
}

/**
 * Edit-dialog title: `action · category · name` so the header shows what is edited.
 */
export function settingsEditTitle(
  category: string | number | undefined | null,
  name: string | number | undefined | null,
  action = 'Edit',
) {
  const c =
    category === undefined || category === null || String(category).trim() === ''
      ? '—'
      : String(category).trim()
  const n =
    name === undefined || name === null || String(name).trim() === ''
      ? '—'
      : String(name).trim()
  return `${action} · ${c} · ${n}`
}

export function CrudModal({
  title,
  description,
  open,
  onClose,
  children,
}: {
  title: string
  description?: string
  open: boolean
  onClose: () => void
  children: ReactNode
}) {
  return (
    <Dialog open={open} onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="max-w-2xl">
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
          {description && <DialogDescription>{description}</DialogDescription>}
        </DialogHeader>
        {children}
      </DialogContent>
    </Dialog>
  )
}

/** Edit form in a modal (same shell as Scripts: header bar, scroll body, footer actions). */
export function CrudEditDialog({
  open,
  title,
  onCancel,
  onSave,
  saving,
  saveLabel,
  savingLabel,
  children,
  className,
}: {
  open: boolean
  title: string
  onCancel: () => void
  onSave: () => void
  saving?: boolean
  /** Primary button label when not saving */
  saveLabel?: string
  savingLabel?: string
  children: ReactNode
  /** Passed to `DialogContent` (e.g. wider layout for large forms). */
  className?: string
}) {
  return (
    <Dialog open={open} onOpenChange={(o) => !o && onCancel()}>
      <DialogContent
        showCloseButton
        className={cn(
          'flex max-h-[92vh] min-h-0 flex-col gap-0 overflow-hidden border p-0 pb-0 pt-10 sm:max-w-2xl',
          className,
        )}
      >
        <DialogHeader className="border-b border-border px-4 pb-3">
          <DialogTitle>{title}</DialogTitle>
        </DialogHeader>
        <div className="min-h-0 flex-1 overflow-y-auto px-4 py-4">
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">{children}</div>
        </div>
        <DialogFooter className="border-t border-border px-4 py-3 sm:justify-end">
          <Button variant="outline" type="button" onClick={onCancel}>
            Cancel
          </Button>
          <Button type="button" onClick={onSave} disabled={saving}>
            {saving ? savingLabel ?? 'Saving...' : saveLabel ?? 'Save'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

export function Field({
  label,
  children,
  className,
  full,
}: {
  label: string
  children: ReactNode
  className?: string
  full?: boolean
}) {
  return (
    <label className={cn('grid gap-1.5', full && 'col-span-full', className)}>
      <span className="text-xs font-medium text-foreground">{label}</span>
      {children}
    </label>
  )
}

export function JsonEditorField({
  label,
  value,
  onChange,
}: {
  label: string
  value: unknown
  onChange: (parsed: unknown) => void
}) {
  const id = useId()
  const [text, setText] = useState<string>(() =>
    typeof value === 'object' && value !== null
      ? JSON.stringify(value, null, 2)
      : typeof value === 'string'
        ? value
        : '',
  )
  const [error, setError] = useState('')

  const handleChange = (e: ChangeEvent<HTMLTextAreaElement>) => {
    const newText = e.target.value
    setText(newText)
    try {
      const parsed = JSON.parse(newText)
      setError('')
      onChange(parsed)
    } catch {
      setError('Invalid JSON')
    }
  }

  return (
    <div className="col-span-full grid gap-2">
      <Label htmlFor={id}>{label}</Label>
      <Textarea
        id={id}
        value={text}
        onChange={handleChange}
        rows={6}
        className={cn(
          'font-mono text-xs',
          error && 'border-destructive focus-visible:ring-destructive/30',
        )}
      />
      {error && <span className="text-xs text-destructive">{error}</span>}
    </div>
  )
}
