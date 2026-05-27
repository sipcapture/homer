import type { HostGrouping } from './flow-data'

export const FLOW_FILTER_PREFS_LS_KEY = 'homer_callflow_prefs'

const HOST_GROUPINGS: HostGrouping[] = ['ungrouped', 'group-by-ip', 'group-by-alias']

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

export interface StoredFlowPrefs {
  hostGrouping?: HostGrouping
  isSimplify?: boolean
  isAbsoluteTime?: boolean
  isHighContrast?: boolean
}

export function isHostGrouping(value: unknown): value is HostGrouping {
  return typeof value === 'string' && (HOST_GROUPINGS as string[]).includes(value)
}

export function loadStoredFlowPrefs(): StoredFlowPrefs {
  if (typeof localStorage === 'undefined') return {}
  try {
    const raw = localStorage.getItem(FLOW_FILTER_PREFS_LS_KEY)
    if (!raw) return {}
    const parsed = JSON.parse(raw) as StoredFlowPrefs
    if (!parsed || typeof parsed !== 'object') return {}
    const out: StoredFlowPrefs = {}
    if (isHostGrouping(parsed.hostGrouping)) out.hostGrouping = parsed.hostGrouping
    if (typeof parsed.isSimplify === 'boolean') out.isSimplify = parsed.isSimplify
    if (typeof parsed.isAbsoluteTime === 'boolean') out.isAbsoluteTime = parsed.isAbsoluteTime
    if (typeof parsed.isHighContrast === 'boolean') out.isHighContrast = parsed.isHighContrast
    return out
  } catch {
    return {}
  }
}

export function saveStoredFlowPrefs(filters: FlowFilters): void {
  if (typeof localStorage === 'undefined') return
  const payload: StoredFlowPrefs = {
    hostGrouping: filters.hostGrouping,
    isSimplify: filters.isSimplify,
    isAbsoluteTime: filters.isAbsoluteTime,
    isHighContrast: filters.isHighContrast,
  }
  try {
    localStorage.setItem(FLOW_FILTER_PREFS_LS_KEY, JSON.stringify(payload))
  } catch {
    /* ignore quota / private mode */
  }
}

/** UI prefs from localStorage + fresh per-call exclusion sets. */
export function initialFlowFilters(): FlowFilters {
  const stored = loadStoredFlowPrefs()
  return {
    ...DEFAULT_FILTERS,
    ...stored,
    ipExcluded: new Set(),
    methodExcluded: new Set(),
    payloadTypeExcluded: new Set(),
    callIdExcluded: new Set(),
  }
}
