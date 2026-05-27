/** Unique key for stacked modals / floating windows (crypto RNG, not Math.random). */
export function newModalKey(): string {
  if (typeof crypto === 'undefined') {
    throw new Error('crypto is not available')
  }
  if (typeof crypto.randomUUID === 'function') {
    return `k${crypto.randomUUID()}`
  }
  const bytes = new Uint8Array(16)
  crypto.getRandomValues(bytes)
  return `k${Array.from(bytes, (b) => b.toString(16).padStart(2, '0')).join('')}`
}
