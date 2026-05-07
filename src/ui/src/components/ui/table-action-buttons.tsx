import { Loader2, Pencil, Trash2 } from 'lucide-react'
import type { ComponentProps } from 'react'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'

type IconActionProps = Omit<ComponentProps<typeof Button>, 'children'>

/** Compact outline edit control for table rows (icon only + tooltip / aria). */
export function EditIconButton({
  className,
  size = 'icon-xs',
  'aria-label': ariaLabel = 'Edit',
  title,
  ...props
}: IconActionProps) {
  return (
    <Button
      type="button"
      variant="outline"
      size={size}
      aria-label={ariaLabel}
      title={title ?? ariaLabel}
      className={cn(className)}
      {...props}
    >
      <Pencil className="size-3" aria-hidden />
    </Button>
  )
}

/** Compact destructive delete control for table rows. */
export function DeleteIconButton({
  className,
  size = 'icon-xs',
  loading,
  disabled,
  'aria-label': ariaLabel = 'Delete',
  title,
  ...props
}: IconActionProps & { loading?: boolean }) {
  const label =
    loading && ariaLabel === 'Delete' ? 'Deleting…' : ariaLabel
  return (
    <Button
      type="button"
      variant="destructive"
      size={size}
      aria-label={label}
      title={title ?? label}
      className={cn(className)}
      {...props}
      disabled={loading || disabled}
    >
      {loading ? (
        <Loader2 className="size-3 animate-spin" aria-hidden />
      ) : (
        <Trash2 className="size-3" aria-hidden />
      )}
    </Button>
  )
}
