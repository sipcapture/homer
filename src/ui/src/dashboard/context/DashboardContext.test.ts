import { describe, expect, it, vi } from 'vitest'
import { EventBus } from './DashboardContext'

describe('EventBus pending/replay', () => {
  it('replays an emit with no listener to the next on()', () => {
    const bus = new EventBus()
    const fn = vi.fn()

    bus.emit('search', 'w1', { foo: 'bar' })
    bus.on('search', 'w1', fn)

    expect(fn).toHaveBeenCalledTimes(1)
    expect(fn).toHaveBeenCalledWith({ foo: 'bar' })
  })

  it('does not replay to a second on() once already delivered', () => {
    const bus = new EventBus()
    const first = vi.fn()
    const second = vi.fn()

    bus.emit('search', 'w1', { foo: 'bar' })
    bus.on('search', 'w1', first)
    bus.on('search', 'w1', second)

    expect(first).toHaveBeenCalledTimes(1)
    expect(second).not.toHaveBeenCalled()
  })

  it('delivers immediately, without buffering, when a listener is already live', () => {
    const bus = new EventBus()
    const fn = vi.fn()

    bus.on('search', 'w1', fn)
    bus.emit('search', 'w1', { foo: 'bar' })
    bus.emit('search', 'w1', { foo: 'baz' })

    expect(fn).toHaveBeenCalledTimes(2)
    expect(fn).toHaveBeenNthCalledWith(1, { foo: 'bar' })
    expect(fn).toHaveBeenNthCalledWith(2, { foo: 'baz' })

    // A later on() for the same key must not replay anything - both
    // earlier emits were already delivered live, nothing was buffered.
    const later = vi.fn()
    bus.on('search', 'w1', later)
    expect(later).not.toHaveBeenCalled()
  })

  it('keeps pending emits scoped per event:id key', () => {
    const bus = new EventBus()
    const fnOther = vi.fn()

    bus.emit('search', 'w1', { foo: 'bar' })
    bus.on('search', 'w2', fnOther)

    expect(fnOther).not.toHaveBeenCalled()
  })

  it('unsubscribing removes the listener so a later emit is buffered again', () => {
    const bus = new EventBus()
    const fn = vi.fn()

    const unsub = bus.on('search', 'w1', fn)
    unsub()
    bus.emit('search', 'w1', { foo: 'bar' })

    expect(fn).not.toHaveBeenCalled()

    const later = vi.fn()
    bus.on('search', 'w1', later)
    expect(later).toHaveBeenCalledTimes(1)
  })
})
