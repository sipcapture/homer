import { useEffect, useMemo, useState } from 'react'
import type { RawMessage } from './flow-data'
import { payloadTypeOf } from './flow-data'
import {
  initialFlowFilters,
  saveStoredFlowPrefs,
  type FlowFilters,
} from './flowFilterPrefs'

export type { FlowFilters } from './flowFilterPrefs'
export { DEFAULT_FILTERS } from './flowFilterPrefs'

export interface FilterToken {
  value: string
  selected: boolean
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

export function applyFlowFilters(items: RawMessage[], filters: FlowFilters): RawMessage[] {
  return items.filter((m) => {
    const payloadType = payloadTypeOf(m)
    if (!filters.showRtcp && payloadType === 'RTCP') return false

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

    if (filters.payloadTypeExcluded.has(payloadType)) return false

    const callid = (m.session_id as string) || (m.cid as string) || ''
    if (filters.callIdExcluded.has(callid)) return false

    return true
  })
}

export function useFlowFilters(items: RawMessage[] | null | undefined): UseFlowFiltersResult {
  const [filters, setFilters] = useState<FlowFilters>(initialFlowFilters)

  useEffect(() => {
    saveStoredFlowPrefs(filters)
  }, [
    filters.hostGrouping,
    filters.isSimplify,
    filters.isAbsoluteTime,
    filters.isHighContrast,
    filters.isConsolidateCaptureIds,
    filters.consolidationTimeThresholdMs,
    filters.showRtcp,
  ])

  const { filterIP, filterMethod, filterPayloadType, filterCallId, filteredItems } = useMemo(() => {
    const safe = items ?? []
    const filteredItems = applyFlowFilters(safe, filters)
    const ips = collectUnique(filteredItems, (m) => [m.src_ip || '', m.dst_ip || ''])
    const methods = collectUnique(filteredItems, (m) => [
      (m.sip_method as string) || (m.method as string) || (m.event as string) || '',
    ])
    const payloadTypes = collectUnique(filteredItems, (m) => [payloadTypeOf(m)])
    const callIds = collectUnique(filteredItems, (m) => [
      (m.session_id as string) || (m.cid as string) || '',
    ])

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
