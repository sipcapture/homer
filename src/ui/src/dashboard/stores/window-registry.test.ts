import { afterEach, describe, expect, it, vi } from 'vitest'
import { useWindowRegistry } from './window-registry'

function resetRegistry() {
  useWindowRegistry.setState({
    minimized: [],
    open: {},
    focusStack: [],
  })
}

describe('useWindowRegistry focus stack', () => {
  afterEach(() => {
    resetRegistry()
  })

  it('tracks focus order as windows register and focus', () => {
    const a = { id: 'a', onClose: vi.fn(), bringToFront: vi.fn() }
    const b = { id: 'b', onClose: vi.fn(), bringToFront: vi.fn() }
    useWindowRegistry.getState().register(a)
    useWindowRegistry.getState().register(b)
    expect(useWindowRegistry.getState().focusStack).toEqual(['a', 'b'])

    useWindowRegistry.getState().focus('a')
    expect(useWindowRegistry.getState().focusStack).toEqual(['b', 'a'])
  })

  it('closeFocused closes the top visible window and focuses the previous one', () => {
    const closeA = vi.fn()
    const closeB = vi.fn()
    const frontA = vi.fn()
    const frontB = vi.fn()
    useWindowRegistry.getState().register({ id: 'a', onClose: closeA, bringToFront: frontA })
    useWindowRegistry.getState().register({ id: 'b', onClose: closeB, bringToFront: frontB })

    useWindowRegistry.getState().closeFocused()

    expect(closeB).toHaveBeenCalledOnce()
    expect(frontA).toHaveBeenCalledOnce()
    expect(closeA).not.toHaveBeenCalled()
  })

  it('skips minimized windows when closing via closeFocused', () => {
    const closeA = vi.fn()
    const closeB = vi.fn()
    const frontA = vi.fn()
    useWindowRegistry.getState().register({ id: 'a', onClose: closeA, bringToFront: frontA })
    useWindowRegistry.getState().register({ id: 'b', onClose: closeB, bringToFront: vi.fn() })
    useWindowRegistry.getState().minimize({
      id: 'b',
      title: 'B',
      onRestore: vi.fn(),
      onClose: closeB,
    })

    useWindowRegistry.getState().closeFocused()

    expect(closeA).toHaveBeenCalledOnce()
    expect(closeB).not.toHaveBeenCalled()
  })

  it('updates callbacks on re-register without changing focus order', () => {
    const close1 = vi.fn()
    const close2 = vi.fn()
    useWindowRegistry.getState().register({ id: 'a', onClose: close1, bringToFront: vi.fn() })
    useWindowRegistry.getState().register({ id: 'b', onClose: vi.fn(), bringToFront: vi.fn() })
    useWindowRegistry.getState().register({ id: 'a', onClose: close2, bringToFront: vi.fn() })

    expect(useWindowRegistry.getState().focusStack).toEqual(['a', 'b'])
    useWindowRegistry.getState().focus('a')
    useWindowRegistry.getState().closeFocused()
    expect(close2).toHaveBeenCalledOnce()
    expect(close1).not.toHaveBeenCalled()
  })
})
