import { create } from 'zustand'
import type { ReactNode } from 'react'

export interface MinimizedEntry {
  id: string
  title: ReactNode
  onRestore: () => void
  onClose: () => void
}

export interface OpenWindowEntry {
  id: string
  onClose: () => void
  bringToFront: () => void
  minimized: boolean
}

interface WindowRegistryState {
  minimized: MinimizedEntry[]
  open: Record<string, OpenWindowEntry>
  /** Window ids in focus order (oldest → newest). */
  focusStack: string[]
  minimize: (entry: MinimizedEntry) => void
  restore: (id: string) => void
  remove: (id: string) => void
  register: (entry: Omit<OpenWindowEntry, 'minimized'>) => void
  focus: (id: string) => void
  closeFocused: () => void
}

let escapeListenerAttached = false

function visibleFocusStack(open: Record<string, OpenWindowEntry>, focusStack: string[]): string[] {
  return focusStack.filter((id) => open[id] && !open[id].minimized)
}

function shouldDeferEscapeToOverlay(): boolean {
  return !!document.querySelector(
    '[data-slot="dialog-content"][data-state="open"], [data-slot="select-content"][data-state="open"], [data-slot="dropdown-menu-content"][data-state="open"]',
  )
}

function attachEscapeListener() {
  if (escapeListenerAttached || typeof document === 'undefined') return
  escapeListenerAttached = true
  document.addEventListener('keydown', (e) => {
    if (e.key !== 'Escape' || e.defaultPrevented) return
    if (shouldDeferEscapeToOverlay()) return
    const { open, focusStack, closeFocused } = useWindowRegistry.getState()
    const visible = visibleFocusStack(open, focusStack)
    if (visible.length === 0) return
    e.preventDefault()
    closeFocused()
  })
}

export const useWindowRegistry = create<WindowRegistryState>((set, get) => ({
  minimized: [],
  open: {},
  focusStack: [],

  minimize: (entry) =>
    set((s) => {
      const open = { ...s.open }
      if (open[entry.id]) {
        open[entry.id] = { ...open[entry.id], minimized: true }
      }
      return {
        minimized: s.minimized.some((e) => e.id === entry.id)
          ? s.minimized.map((e) => (e.id === entry.id ? entry : e))
          : [...s.minimized, entry],
        open,
        focusStack: s.focusStack.filter((id) => id !== entry.id),
      }
    }),

  restore: (id) =>
    set((s) => {
      const open = { ...s.open }
      if (open[id]) {
        open[id] = { ...open[id], minimized: false }
      }
      const focusStack = s.focusStack.filter((fid) => fid !== id)
      focusStack.push(id)
      return {
        minimized: s.minimized.filter((e) => e.id !== id),
        open,
        focusStack,
      }
    }),

  remove: (id) =>
    set((s) => ({
      minimized: s.minimized.filter((e) => e.id !== id),
      open: Object.fromEntries(Object.entries(s.open).filter(([k]) => k !== id)),
      focusStack: s.focusStack.filter((fid) => fid !== id),
    })),

  register: (entry) => {
    attachEscapeListener()
    set((s) => {
      const existing = s.open[entry.id]
      const open = {
        ...s.open,
        [entry.id]: { ...entry, minimized: existing?.minimized ?? false },
      }
      if (existing) return { open }
      const focusStack = [...s.focusStack, entry.id]
      return { open, focusStack }
    })
  },

  focus: (id) =>
    set((s) => {
      if (!s.open[id] || s.open[id].minimized) return s
      const focusStack = s.focusStack.filter((fid) => fid !== id)
      focusStack.push(id)
      return { focusStack }
    }),

  closeFocused: () => {
    const { open, focusStack } = get()
    const visible = visibleFocusStack(open, focusStack)
    if (visible.length === 0) return
    const focusedId = visible[visible.length - 1]
    const previousId = visible.length > 1 ? visible[visible.length - 2] : null
    const focused = open[focusedId]
    if (!focused) return
    focused.onClose()
    if (previousId && open[previousId]) {
      open[previousId].bringToFront()
      get().focus(previousId)
    }
  },
}))
