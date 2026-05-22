import { useMemo } from 'react'
import { cn } from '@/lib/utils'

function initialsFromLabel(label: string): string {
  const parts = label.trim().split(/\s+/).filter(Boolean)
  if (parts.length >= 2) {
    return (parts[0][0] + parts[1][0]).toUpperCase()
  }
  const one = parts[0] || '?'
  return one.slice(0, 2).toUpperCase()
}

export interface UserAvatarProps {
  label: string
  imageUrl?: string | null
  className?: string
  size?: 'sm' | 'md'
}

export default function UserAvatar({ label, imageUrl, className, size = 'sm' }: UserAvatarProps) {
  const initials = useMemo(() => initialsFromLabel(label), [label])
  const dim = size === 'md' ? 'h-10 w-10 text-sm' : 'h-8 w-8 text-xs'

  if (imageUrl) {
    return (
      <img
        src={imageUrl}
        alt=""
        className={cn(dim, 'shrink-0 rounded-full object-cover ring-1 ring-border', className)}
      />
    )
  }

  return (
    <span
      className={cn(
        dim,
        'inline-flex shrink-0 items-center justify-center rounded-full bg-primary font-semibold text-primary-foreground ring-1 ring-border',
        className,
      )}
      aria-hidden
    >
      {initials}
    </span>
  )
}
