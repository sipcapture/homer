import { useState, useRef, useEffect } from 'react'
import { X, ChevronDown, Plus } from 'lucide-react'
import { cn } from '@/lib/utils'

interface Option {
  value: string
  name: string
}

interface MultiSelectInputProps {
  id?: string
  options: Option[]
  value: string[]
  onChange: (values: string[]) => void
  placeholder?: string
  className?: string
}

export default function MultiSelectInput({
  id,
  options,
  value,
  onChange,
  placeholder = 'Any',
  className,
}: MultiSelectInputProps) {
  const [open, setOpen] = useState(false)
  const [customInput, setCustomInput] = useState('')
  const containerRef = useRef<HTMLDivElement>(null)

  // Close dropdown on outside click
  useEffect(() => {
    const handler = (e: MouseEvent) => {
      if (containerRef.current && !containerRef.current.contains(e.target as Node)) {
        setOpen(false)
      }
    }
    document.addEventListener('mousedown', handler)
    return () => document.removeEventListener('mousedown', handler)
  }, [])

  const toggle = (v: string) => {
    if (value.includes(v)) {
      onChange(value.filter((x) => x !== v))
    } else {
      onChange([...value, v])
    }
  }

  const remove = (v: string, e: React.MouseEvent) => {
    e.stopPropagation()
    onChange(value.filter((x) => x !== v))
  }

  const addCustom = () => {
    const trimmed = customInput.trim()
    if (trimmed && !value.includes(trimmed)) {
      onChange([...value, trimmed])
    }
    setCustomInput('')
  }

  const handleCustomKey = (e: React.KeyboardEvent<HTMLInputElement>) => {
    if (e.key === 'Enter') {
      e.preventDefault()
      addCustom()
    }
  }

  return (
    <div ref={containerRef} className={cn('relative', className)}>
      {/* Trigger */}
      <div
        id={id}
        className={cn(
          'flex min-h-7 w-full cursor-pointer flex-wrap items-center gap-1 rounded-md border border-input bg-background px-2 py-0.5 text-xs ring-offset-background',
          'hover:border-ring focus-within:border-ring focus-within:ring-1 focus-within:ring-ring',
          open && 'border-ring ring-1 ring-ring',
        )}
        onClick={() => setOpen((o) => !o)}
      >
        {value.length === 0 ? (
          <span className="text-muted-foreground">{placeholder}</span>
        ) : (
          value.map((v) => (
            <span
              key={v}
              className="inline-flex items-center gap-0.5 rounded bg-primary/15 px-1.5 py-0.5 text-[10px] font-medium text-primary"
            >
              {v}
              <X
                className="h-2.5 w-2.5 cursor-pointer opacity-60 hover:opacity-100"
                onClick={(e) => remove(v, e)}
              />
            </span>
          ))
        )}
        <ChevronDown
          className={cn(
            'ml-auto h-3 w-3 shrink-0 text-muted-foreground transition-transform',
            open && 'rotate-180',
          )}
        />
      </div>

      {/* Dropdown */}
      {open && (
        <div className="absolute z-50 mt-1 w-full rounded-md border border-border bg-popover shadow-md">
          {/* Predefined options with checkboxes */}
          <div className="max-h-48 overflow-y-auto py-1">
            {options.map((opt) => (
              <div
                key={opt.value}
                className="flex cursor-pointer items-center gap-2 px-2 py-1 text-xs hover:bg-accent"
                onClick={() => toggle(opt.value)}
              >
                <input
                  type="checkbox"
                  readOnly
                  checked={value.includes(opt.value)}
                  className="h-3 w-3 cursor-pointer accent-primary"
                />
                <span>{opt.name}</span>
              </div>
            ))}
          </div>

          {/* Custom value input */}
          <div className="border-t border-border px-2 py-1.5">
            <div className="flex items-center gap-1">
              <input
                type="text"
                value={customInput}
                onChange={(e) => setCustomInput(e.target.value)}
                onKeyDown={handleCustomKey}
                placeholder="Custom value + Enter"
                className="h-6 flex-1 rounded border border-input bg-background px-1.5 text-[10px] outline-none focus:border-ring"
                onClick={(e) => e.stopPropagation()}
              />
              <button
                type="button"
                onClick={(e) => { e.stopPropagation(); addCustom() }}
                disabled={!customInput.trim()}
                className="flex h-6 w-6 items-center justify-center rounded border border-input bg-background text-muted-foreground hover:bg-accent disabled:opacity-40"
              >
                <Plus className="h-3 w-3" />
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}
