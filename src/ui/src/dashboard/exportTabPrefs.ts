export const EXPORT_CORRELATE_LS_KEY = 'homer_export_correlate'

export function loadExportCorrelate(): boolean {
  if (typeof localStorage === 'undefined') return false
  try {
    return localStorage.getItem(EXPORT_CORRELATE_LS_KEY) === '1'
  } catch {
    return false
  }
}

export function saveExportCorrelate(value: boolean): void {
  if (typeof localStorage === 'undefined') return
  try {
    localStorage.setItem(EXPORT_CORRELATE_LS_KEY, value ? '1' : '0')
  } catch {
    /* ignore quota / private mode */
  }
}
