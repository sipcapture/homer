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

function offset(value: unknown, sign: -1 | 1, fallback: number): number {
  if (typeof value !== 'number' || !Number.isFinite(value)) return fallback
  return value * sign >= 0 ? value : fallback
}

export function parseTransactionRange(data: unknown): TransactionRange {
  if (data == null || typeof data !== 'object' || Array.isArray(data)) return DEFAULT_TRANSACTION_RANGE
  const d = data as Record<string, unknown>
  return {
    from: offset(d.from, -1, DEFAULT_TRANSACTION_RANGE.from),
    to: offset(d.to, 1, DEFAULT_TRANSACTION_RANGE.to),
    message_from: offset(d.message_from, -1, DEFAULT_TRANSACTION_RANGE.message_from),
    message_to: offset(d.message_to, 1, DEFAULT_TRANSACTION_RANGE.message_to),
  }
}

export function transactionWindow(range: TransactionRange, firstMs: number, lastMs: number) {
  return { from: firstMs + range.from, to: lastMs + range.to }
}

export function useTransactionRange(): TransactionRange {
  const [range, setRange] = useState(DEFAULT_TRANSACTION_RANGE)
  useEffect(() => {
    let cancelled = false
    apiGet('/advanced', {
      'filter[category]': 'transaction',
      'filter[param]': 'range',
      'page[limit]': 10,
    })
      .then((res) => {
        const row = (res?.data?.items || []).find(
          (i: { category?: string; param?: string }) => i?.category === 'transaction' && i?.param === 'range',
        )
        if (!cancelled && row) setRange(parseTransactionRange(row.data))
      })
      .catch(() => {})
    return () => {
      cancelled = true
    }
  }, [])
  return range
}
