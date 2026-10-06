import { useEffect, useState } from 'react'
import { apiGet } from '@/api'

/**
 * Offsets in ms around the clicked/selected rows when opening a transaction or a
 * single message. Configured as Settings → Advanced, category `transaction`,
 * param `range` (Homer 7's `transaction:range`).
 */
export type TransactionRange = {
  from: number
  to: number
  message_from: number
  message_to: number
}

export const DEFAULT_TRANSACTION_RANGE: TransactionRange = {
  from: -300000,
  to: 300000,
  message_from: -300000,
  message_to: 300000,
}

// Each open scans this whole span of Parquet, so a typo must not widen it unbounded.
export const MAX_TRANSACTION_OFFSET_MS = 24 * 3600 * 1000

function offset(d: Record<string, unknown>, key: keyof TransactionRange, sign: -1 | 1): number {
  const value = d[key]
  const fallback = DEFAULT_TRANSACTION_RANGE[key]
  if (value === undefined) return fallback
  if (
    typeof value === 'number' &&
    Number.isFinite(value) &&
    value * sign >= 0 &&
    Math.abs(value) <= MAX_TRANSACTION_OFFSET_MS
  ) {
    return value
  }
  console.warn(
    `transaction.range.${key} ${JSON.stringify(value)} must be a number in ${sign < 0 ? `[-${MAX_TRANSACTION_OFFSET_MS}, 0]` : `[0, ${MAX_TRANSACTION_OFFSET_MS}]`} ms, using ${fallback}`,
  )
  return fallback
}

export function parseTransactionRange(data: unknown): TransactionRange {
  if (data == null || typeof data !== 'object' || Array.isArray(data)) return DEFAULT_TRANSACTION_RANGE
  const d = data as Record<string, unknown>
  return {
    from: offset(d, 'from', -1),
    to: offset(d, 'to', 1),
    message_from: offset(d, 'message_from', -1),
    message_to: offset(d, 'message_to', 1),
  }
}

export function transactionWindow(range: TransactionRange, firstMs: number, lastMs: number) {
  return { from: firstMs + range.from, to: lastMs + range.to }
}

let rangePromise: Promise<TransactionRange> | null = null

/**
 * ResultsPanels mounting together share one in-flight `/advanced` request. It is not kept after it settles:
 * closing Settings remounts the dashboard without a page load, and must pick up an edited range.
 */
export function loadTransactionRange(): Promise<TransactionRange> {
  if (rangePromise) return rangePromise
  const request: Promise<TransactionRange> = apiGet('/advanced', {
    'filter[category]': 'transaction',
    'filter[param]': 'range',
    'page[limit]': 10,
  })
    .then((res) => {
      const row = (res?.data?.items || []).find(
        (i: { category?: string; param?: string }) => i?.category === 'transaction' && i?.param === 'range',
      )
      return row ? parseTransactionRange(row.data) : DEFAULT_TRANSACTION_RANGE
    })
    .catch((err) => {
      console.warn('transaction.range: failed to load advanced setting, using ±300s', err)
      return DEFAULT_TRANSACTION_RANGE
    })
    .finally(() => {
      if (rangePromise === request) rangePromise = null
    })
  rangePromise = request
  return request
}

export function resetTransactionRangeCache() {
  rangePromise = null
}

export function useTransactionRange(): TransactionRange {
  const [range, setRange] = useState(DEFAULT_TRANSACTION_RANGE)
  useEffect(() => {
    let cancelled = false
    void loadTransactionRange().then((r) => {
      if (!cancelled) setRange(r)
    })
    return () => {
      cancelled = true
    }
  }, [])
  return range
}
