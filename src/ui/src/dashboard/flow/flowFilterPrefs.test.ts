import { describe, expect, it, beforeEach } from 'vitest'
import {
  FLOW_FILTER_PREFS_LS_KEY,
  initialFlowFilters,
  loadStoredFlowPrefs,
  saveStoredFlowPrefs,
} from './flowFilterPrefs'
import { DEFAULT_FILTERS } from './flowFilterPrefs'

describe('flowFilterPrefs', () => {
  beforeEach(() => {
    localStorage.clear()
  })

  it('round-trips host grouping and toggles', () => {
    saveStoredFlowPrefs({
      ...DEFAULT_FILTERS,
      hostGrouping: 'group-by-alias',
      isSimplify: true,
      isAbsoluteTime: true,
      isHighContrast: true,
    })
    expect(loadStoredFlowPrefs()).toEqual({
      hostGrouping: 'group-by-alias',
      isSimplify: true,
      isAbsoluteTime: true,
      isHighContrast: true,
      isConsolidateCaptureIds: false,
      consolidationTimeThresholdMs: 500,
      showRtcp: false,
    })
  })

  it('ignores invalid stored host grouping', () => {
    localStorage.setItem(
      FLOW_FILTER_PREFS_LS_KEY,
      JSON.stringify({ hostGrouping: 'invalid', isSimplify: true }),
    )
    expect(loadStoredFlowPrefs()).toEqual({ isSimplify: true })
  })

  it('initialFlowFilters merges stored prefs with empty exclusion sets', () => {
    localStorage.setItem(
      FLOW_FILTER_PREFS_LS_KEY,
      JSON.stringify({ hostGrouping: 'group-by-ip' }),
    )
    const f = initialFlowFilters()
    expect(f.hostGrouping).toBe('group-by-ip')
    expect(f.ipExcluded.size).toBe(0)
    expect(f.methodExcluded.size).toBe(0)
  })

  it('round-trips consolidation prefs', () => {
    saveStoredFlowPrefs({ ...DEFAULT_FILTERS, isConsolidateCaptureIds: true, consolidationTimeThresholdMs: 250 })
    expect(loadStoredFlowPrefs()).toMatchObject({ isConsolidateCaptureIds: true, consolidationTimeThresholdMs: 250 })
  })

  it('ignores non-boolean isConsolidateCaptureIds', () => {
    localStorage.setItem(FLOW_FILTER_PREFS_LS_KEY, JSON.stringify({ isConsolidateCaptureIds: 'yes' }))
    expect(loadStoredFlowPrefs().isConsolidateCaptureIds).toBeUndefined()
  })

  it('clamps negative consolidationTimeThresholdMs to 0', () => {
    localStorage.setItem(FLOW_FILTER_PREFS_LS_KEY, JSON.stringify({ consolidationTimeThresholdMs: -100 }))
    expect(loadStoredFlowPrefs().consolidationTimeThresholdMs).toBe(0)
  })

  it('ignores non-finite consolidationTimeThresholdMs', () => {
    localStorage.setItem(FLOW_FILTER_PREFS_LS_KEY, JSON.stringify({ consolidationTimeThresholdMs: null }))
    expect(loadStoredFlowPrefs().consolidationTimeThresholdMs).toBeUndefined()
  })

  it('round-trips showRtcp', () => {
    saveStoredFlowPrefs({ ...DEFAULT_FILTERS, showRtcp: true })
    expect(loadStoredFlowPrefs()).toMatchObject({ showRtcp: true })
    expect(initialFlowFilters().showRtcp).toBe(true)
  })

  it('defaults showRtcp to false', () => {
    expect(DEFAULT_FILTERS.showRtcp).toBe(false)
    expect(initialFlowFilters().showRtcp).toBe(false)
  })

  it('ignores non-boolean showRtcp', () => {
    localStorage.setItem(FLOW_FILTER_PREFS_LS_KEY, JSON.stringify({ showRtcp: 'yes' }))
    expect(loadStoredFlowPrefs().showRtcp).toBeUndefined()
  })
})
