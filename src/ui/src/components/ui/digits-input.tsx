import type { ComponentProps } from 'react'
import { Input } from '@/components/ui/input'

export type DigitsInputProps = Omit<
  ComponentProps<typeof Input>,
  'type' | 'value' | 'onChange'
> & {
  value: string
  onValueChange: (next: string) => void
}

/** Integer digits only (empty allowed). Avoid `type="number"` + controlled number — it forces 0 on clear and breaks typing. */
export function DigitsInput({ value, onValueChange, onBlur, ...rest }: DigitsInputProps) {
  return (
    <Input
      {...rest}
      type="text"
      inputMode="numeric"
      autoComplete="off"
      value={value}
      onChange={(e) => {
        const next = e.target.value
        if (!/^\d*$/.test(next)) return
        onValueChange(next)
      }}
      onBlur={(e) => {
        onBlur?.(e)
      }}
    />
  )
}

/** Parse optional unsigned int; empty → defaultValue */
export function parseUint(str: string, defaultValue = 0): number {
  const n = parseInt(str, 10)
  return Number.isFinite(n) ? n : defaultValue
}
