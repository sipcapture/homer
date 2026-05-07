import { useMemo, useState } from 'react'
import type { HostGrouping, RawMessage } from './flow-data'
import { payloadTypeOf } from './flow-data'

export interface FilterToken {
  value: string
  selected: boolean
}

export interface FlowFilters {
  isSimplify: boolean
  isAbsoluteTime: boolean
  isHighContrast: boolean
  hostGrouping: HostGrouping
  ipExcluded: Set<string>
  methodExcluded: Set<string>
  payloadTypeExcluded: Set<string>
  callIdExcluded: Set<string>
}

export const DEFAULT_FILTERS: FlowFilters = {
  isSimplify: false,
  isAbsoluteTime: false,
  isHighContrast: false,
  hostGrouping: 'ungrouped',
  ipExcluded: new Set(),
  methodExcluded: new Set(),
  payloadTypeExcluded: new Set(),
  callIdExcluded: new Set(),
}

function collectUnique(items: RawMessage[], picker: (m: RawMessage) => string[]): string[] {
  const set = new Set<string>()
  items.forEach((m) => {
    picker(m).forEach((v) => {
      if (v) set.add(v)
    })
  })
  return Array.from(set)
}

function toTokens(values: string[], excluded: Set<string>): FilterToken[] {
  return values.map((v) => ({ value: v, selected: !excluded.has(v) }))
}

export function tokensToExcluded(tokens: FilterToken[]): Set<string> {
  return new Set(tokens.filter((t) => !t.selected).map((t) => t.value))
}

export function toggleInSet(set: Set<string>, value: string): Set<string> {
  const next = new Set(set)
  if (next.has(value)) next.delete(value)
  else next.add(value)
  return next
}

export interface UseFlowFiltersResult {
  filters: FlowFilters
  setFilters: React.Dispatch<React.SetStateAction<FlowFilters>>
  filterIP: FilterToken[]
  filterMethod: FilterToken[]
  filterPayloadType: FilterToken[]
  filterCallId: FilterToken[]
  filteredItems: RawMessage[]
}

export function useFlowFilters(items: RawMessage[] | null | undefined): UseFlowFiltersResult {
  const [filters, setFilters] = useState<FlowFilters>(DEFAULT_FILTERS)

  const { filterIP, filterMethod, filterPayloadType, filterCallId, filteredItems } = useMemo(() => {
    const safe = items ?? []
    const ips = collectUnique(safe, (m) => [m.src_ip || '', m.dst_ip || ''])
    const methods = collectUnique(safe, (m) => [
      (m.sip_method as string) || (m.method as string) || (m.event as string) || '',
    ])
    const payloadTypes = collectUnique(safe, (m) => [payloadTypeOf(m)])
    const callIds = collectUnique(safe, (m) => [
      (m.session_id as string) || (m.cid as string) || '',
    ])

    const filteredItems = safe.filter((m) => {
      const src = m.src_ip || ''
      const dst = m.dst_ip || ''
      if (filters.ipExcluded.size > 0 && filters.ipExcluded.has(src) && filters.ipExcluded.has(dst))
        return false
      if (
        filters.ipExcluded.size > 0 &&
        (filters.ipExcluded.has(src) || filters.ipExcluded.has(dst))
      )
        return false

      const method =
        (m.sip_method as string) || (m.method as string) || (m.event as string) || ''
      if (filters.methodExcluded.has(method)) return false

      if (filters.payloadTypeExcluded.has(payloadTypeOf(m))) return false

      const callid = (m.session_id as string) || (m.cid as string) || ''
      if (filters.callIdExcluded.has(callid)) return false

      return true
    })

    return {
      filterIP: toTokens(ips, filters.ipExcluded),
      filterMethod: toTokens(methods, filters.methodExcluded),
      filterPayloadType: toTokens(payloadTypes, filters.payloadTypeExcluded),
      filterCallId: toTokens(callIds, filters.callIdExcluded),
      filteredItems,
    }
  }, [items, filters])

  return { filters, setFilters, filterIP, filterMethod, filterPayloadType, filterCallId, filteredItems }
}
