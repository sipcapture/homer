import { describe, expect, it } from 'vitest'
import { DEFAULT_TRANSACTION_RANGE, parseTransactionRange, transactionWindow } from './transactionRange'

describe('parseTransactionRange', () => {
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
