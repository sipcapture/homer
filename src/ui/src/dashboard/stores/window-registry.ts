import { create } from 'zustand'
import type { ReactNode } from 'react'

export interface MinimizedEntry {
  id: string
  title: ReactNode
  onRestore: () => void
  onClose: () => void
}

interface WindowRegistryState {
  minimized: MinimizedEntry[]
  minimize: (entry: MinimizedEntry) => void
  restore: (id: string) => void
  remove: (id: string) => void
}

export const useWindowRegistry = create<WindowRegistryState>((set) => ({
  minimized: [],
  minimize: (entry) =>
    set((s) => ({
      minimized: s.minimized.some((e) => e.id === entry.id)
        ? s.minimized.map((e) => (e.id === entry.id ? entry : e))
        : [...s.minimized, entry],
    })),
  restore: (id) =>
    set((s) => ({ minimized: s.minimized.filter((e) => e.id !== id) })),
  remove: (id) =>
    set((s) => ({ minimized: s.minimized.filter((e) => e.id !== id) })),
}))
