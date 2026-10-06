import { renderHook, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { apiGet } from '@/api'
import {
  DEFAULT_TRANSACTION_RANGE,
  loadTransactionRange,
  MAX_TRANSACTION_OFFSET_MS,
  parseTransactionRange,
  resetTransactionRangeCache,
  transactionWindow,
  useTransactionRange,
} from './transactionRange'

vi.mock('@/api', () => ({ apiGet: vi.fn() }))

const mockedApiGet = vi.mocked(apiGet)

describe('parseTransactionRange', () => {
  beforeEach(() => {
    vi.spyOn(console, 'warn').mockImplementation(() => {})
  })
  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('keeps the historical ±300s window when no setting exists', () => {
    expect(parseTransactionRange(undefined)).toEqual(DEFAULT_TRANSACTION_RANGE)
    expect(DEFAULT_TRANSACTION_RANGE).toEqual({
      from: -300000,
      to: 300000,
      message_from: -300000,
      message_to: 300000,
    })
  })

  it('accepts an asymmetric Homer 7 style transaction:range', () => {
    expect(parseTransactionRange({ from: -600000, to: 10800000 })).toEqual({
      ...DEFAULT_TRANSACTION_RANGE,
      from: -600000,
      to: 10800000,
    })
  })

  it('reads message_from / message_to independently', () => {
    expect(parseTransactionRange({ message_from: -1000, message_to: 1000 })).toEqual({
      ...DEFAULT_TRANSACTION_RANGE,
      message_from: -1000,
      message_to: 1000,
    })
  })

  it('falls back per field on wrong sign, non-numbers and non-finite values', () => {
    expect(
      parseTransactionRange({ from: 600000, to: -1, message_from: '-1000', message_to: Number.POSITIVE_INFINITY }),
    ).toEqual(DEFAULT_TRANSACTION_RANGE)
  })

  it('accepts offsets up to 24h and falls back above it', () => {
    expect(MAX_TRANSACTION_OFFSET_MS).toBe(24 * 3600 * 1000)
    expect(parseTransactionRange({ from: -MAX_TRANSACTION_OFFSET_MS, to: MAX_TRANSACTION_OFFSET_MS })).toEqual({
      ...DEFAULT_TRANSACTION_RANGE,
      from: -MAX_TRANSACTION_OFFSET_MS,
      to: MAX_TRANSACTION_OFFSET_MS,
    })
    // 30h typo for 3h
    expect(parseTransactionRange({ from: -600000, to: 108000000 })).toEqual({
      ...DEFAULT_TRANSACTION_RANGE,
      from: -600000,
    })
  })

  it('warns about each rejected field but not about absent ones', () => {
    parseTransactionRange({ from: -600000, to: 108000000 })
    expect(console.warn).toHaveBeenCalledTimes(1)
    expect(vi.mocked(console.warn).mock.calls[0][0]).toContain('transaction.range.to')
  })

  it('ignores non-object data', () => {
    expect(parseTransactionRange('[]')).toEqual(DEFAULT_TRANSACTION_RANGE)
    expect(parseTransactionRange(null)).toEqual(DEFAULT_TRANSACTION_RANGE)
  })
})

describe('transactionWindow', () => {
  it('applies from to the earliest and to to the latest timestamp', () => {
    const range = parseTransactionRange({ from: -600000, to: 10800000 })
    expect(transactionWindow(range, 1_000_000_000, 1_000_060_000)).toEqual({
      from: 1_000_000_000 - 600000,
      to: 1_000_060_000 + 10800000,
    })
  })
})

describe('loadTransactionRange', () => {
  const row = { category: 'transaction', param: 'range', data: { from: -600000, to: 10800000 } }

  beforeEach(() => {
    resetTransactionRangeCache()
    mockedApiGet.mockReset()
    vi.spyOn(console, 'warn').mockImplementation(() => {})
  })
  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('shares one /advanced request across concurrent callers', async () => {
    mockedApiGet.mockResolvedValue({ data: { items: [row] } })
    const [a, b] = await Promise.all([loadTransactionRange(), loadTransactionRange()])
    expect(mockedApiGet).toHaveBeenCalledTimes(1)
    expect(a).toEqual({ ...DEFAULT_TRANSACTION_RANGE, from: -600000, to: 10800000 })
    expect(b).toBe(a)
  })

  it('picks up an edited setting on the next mount without a page reload', async () => {
    mockedApiGet.mockResolvedValueOnce({ data: { items: [{ ...row, data: { to: 3600000 } }] } })
    const first = renderHook(() => useTransactionRange())
    await waitFor(() => expect(first.result.current.to).toBe(3600000))
    first.unmount()

    mockedApiGet.mockResolvedValueOnce({ data: { items: [{ ...row, data: { to: 10800000 } }] } })
    const second = renderHook(() => useTransactionRange())
    await waitFor(() => expect(second.result.current.to).toBe(10800000))
  })

  it('matches category/param exactly, since the server filter is a substring match', async () => {
    mockedApiGet.mockResolvedValue({
      data: { items: [{ category: 'transaction', param: 'ranges', data: { to: 7200000 } }] },
    })
    expect(await loadTransactionRange()).toEqual(DEFAULT_TRANSACTION_RANGE)
  })

  it('warns, returns the default, and retries on the next call after a failure', async () => {
    mockedApiGet.mockRejectedValueOnce(new Error('Error 500'))
    expect(await loadTransactionRange()).toEqual(DEFAULT_TRANSACTION_RANGE)
    expect(console.warn).toHaveBeenCalledTimes(1)

    mockedApiGet.mockResolvedValueOnce({ data: { items: [row] } })
    expect(await loadTransactionRange()).toEqual({ ...DEFAULT_TRANSACTION_RANGE, from: -600000, to: 10800000 })
    expect(mockedApiGet).toHaveBeenCalledTimes(2)
  })
})
