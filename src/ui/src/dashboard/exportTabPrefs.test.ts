import { beforeEach, describe, expect, it } from 'vitest'
import { EXPORT_CORRELATE_LS_KEY, loadExportCorrelate, saveExportCorrelate } from './exportTabPrefs'

describe('exportTabPrefs', () => {
  beforeEach(() => {
    localStorage.clear()
  })

  it('defaults to off', () => {
    expect(loadExportCorrelate()).toBe(false)
  })

  it('round-trips the setting', () => {
    saveExportCorrelate(true)
    expect(loadExportCorrelate()).toBe(true)
    saveExportCorrelate(false)
    expect(loadExportCorrelate()).toBe(false)
  })

  it('treats unexpected stored values as off', () => {
    localStorage.setItem(EXPORT_CORRELATE_LS_KEY, 'true')
    expect(loadExportCorrelate()).toBe(false)
  })
})
