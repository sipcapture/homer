import { CheckCircle2, XCircle } from 'lucide-react'
import { cn } from '@/lib/utils'

export type BoolIndicatorProps = {
  value: boolean | null | undefined
  /** Tooltip / `aria-label` when true */
  trueLabel?: string
  /** Tooltip / `aria-label` when false */
  falseLabel?: string
  className?: string
}

/** Consistent on/off display (green check vs muted X) across settings tables. */
export function BoolIndicator({
  value,
  trueLabel = 'Enabled',
  falseLabel = 'Disabled',
  className,
}: BoolIndicatorProps) {
  if (value === null || value === undefined) {
    return <span className={cn('text-muted-foreground', className)}>—</span>
  }
  const on = !!value
  const label = on ? trueLabel : falseLabel
  return (
    <span
      className={cn('inline-flex items-center justify-center', className)}
      aria-label={label}
      title={label}
    >
      {on ? (
        <CheckCircle2 className="size-[1.125rem] shrink-0 text-emerald-500" aria-hidden />
      ) : (
        <XCircle className="size-[1.125rem] shrink-0 text-muted-foreground" aria-hidden />
      )}
    </span>
  )
}
