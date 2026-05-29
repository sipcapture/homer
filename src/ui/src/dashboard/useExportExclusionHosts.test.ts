import { describe, expect, it } from 'vitest'
import type { ExportHostOption } from './useExportExclusionHosts'

// Replicate merge logic for unit tests without mounting hooks.
function mergeHostList(
  hosts: ExportHostOption[],
  aliases: Array<{ ip?: string; alias?: string; mask?: number }>,
  excludedCIDR: Array<{ ip?: string; alias?: string; disabled?: boolean }> | undefined,
): { options: ExportHostOption[]; defaultSelected: string[] } {
  const byIP = new Map<string, ExportHostOption>()
  for (const h of hosts) {
    byIP.set(h.ip, { ip: h.ip, label: h.ip })
  }
  const defaultSelected: string[] = []
  if (excludedCIDR) {
    for (const host of excludedCIDR) {
      if (host.disabled) continue
      if (host.ip && !host.alias) {
        defaultSelected.push(host.ip)
        if (!byIP.has(host.ip)) byIP.set(host.ip, { ip: host.ip, label: host.ip })
      } else if (host.alias) {
        const row = aliases.find((a) => a.alias === host.alias)
        if (!row?.ip) continue
        const mask = row.mask != null ? String(row.mask) : '32'
        const ip = `${row.ip}/${mask}`
        defaultSelected.push(ip)
        if (!byIP.has(ip)) byIP.set(ip, { ip, label: ip })
      }
    }
  }
  return { options: Array.from(byIP.values()), defaultSelected: [...new Set(defaultSelected)] }
}

describe('export exclusion host list', () => {
  it('preselects non-disabled excludedCIDR IPs', () => {
    const { defaultSelected } = mergeHostList(
      [{ ip: '10.0.0.2', label: '10.0.0.2' }],
      [],
      [{ ip: '10.0.0.1', disabled: false }, { ip: '10.0.0.9', disabled: true }],
    )
    expect(defaultSelected).toEqual(['10.0.0.1'])
  })

  it('resolves alias-based excludedCIDR to CIDR string', () => {
    const { defaultSelected, options } = mergeHostList(
      [],
      [{ ip: '192.168.1.1', alias: 'sbc', mask: 24 }],
      [{ alias: 'sbc', disabled: false }],
    )
    expect(defaultSelected).toEqual(['192.168.1.1/24'])
    expect(options.some((o) => o.ip === '192.168.1.1/24')).toBe(true)
  })
})
