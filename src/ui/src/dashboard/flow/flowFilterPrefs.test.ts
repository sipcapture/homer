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
})
