import { useEffect, useMemo, useState } from 'react'
import { apiGet } from '@/api'

export type ExportHostOption = {
  ip: string
  label: string
}

type AliasRow = {
  ip?: string
  alias?: string
  mask?: number | string
}

type ExcludedCIDRRow = {
  ip?: string
  alias?: string
  disabled?: boolean
}

function aliasSuffix(alias: string | undefined): string {
  return alias ? ` : ${alias}` : ''
}

function aliasLabelForIP(ip: string, aliases: AliasRow[]): string {
  const row = aliases.find((a) => a.ip === ip)
  return `${ip}${aliasSuffix(row?.alias)}`
}

function hostsFromItems(items: unknown[]): ExportHostOption[] {
  const seen = new Set<string>()
  const out: ExportHostOption[] = []
  for (const raw of items || []) {
    const item = raw as Record<string, unknown>
    for (const key of ['src_ip', 'dst_ip']) {
      const ip = item[key]
      if (typeof ip !== 'string' || !ip || seen.has(ip)) continue
      seen.add(ip)
      out.push({ ip, label: ip })
    }
  }
  return out
}

function mergeHostList(
  hosts: ExportHostOption[],
  aliases: AliasRow[],
  excludedCIDR: ExcludedCIDRRow[] | undefined,
): { options: ExportHostOption[]; defaultSelected: string[] } {
  const byIP = new Map<string, ExportHostOption>()
  for (const h of hosts) {
    byIP.set(h.ip, { ip: h.ip, label: aliasLabelForIP(h.ip, aliases) })
  }

  const defaultSelected: string[] = []

  if (excludedCIDR) {
    for (const host of excludedCIDR) {
      if (host.disabled) continue
      if (host.ip && !host.alias) {
        const label = aliasLabelForIP(host.ip, aliases)
        if (!byIP.has(host.ip)) {
          byIP.set(host.ip, { ip: host.ip, label })
        }
        defaultSelected.push(host.ip)
      } else if (host.alias) {
        const row = aliases.find((a) => a.alias === host.alias)
        if (!row?.ip) continue
        const mask = row.mask != null && String(row.mask) !== '' ? String(row.mask) : '32'
        const ip = `${row.ip}/${mask}`
        const label = `${ip}${aliasSuffix(row.alias)}`
        if (!byIP.has(ip)) {
          byIP.set(ip, { ip, label })
        }
        defaultSelected.push(ip)
      }
    }
  }

  for (const h of hosts) {
    const label = aliasLabelForIP(h.ip, aliases)
    byIP.set(h.ip, { ip: h.ip, label })
  }

  const options = Array.from(byIP.values()).sort((a, b) => a.ip.localeCompare(b.ip))
  const uniqueDefaults = [...new Set(defaultSelected.filter((ip) => byIP.has(ip)))]
  return { options, defaultSelected: uniqueDefaults }
}

export function useExportExclusionHosts(items: unknown[]) {
  const [options, setOptions] = useState<ExportHostOption[]>([])
  const [selectedIPs, setSelectedIPs] = useState<string[]>([])
  const [loading, setLoading] = useState(true)

  const itemHosts = useMemo(() => hostsFromItems(items), [items])

  useEffect(() => {
    let cancelled = false
    const run = async () => {
      setLoading(true)
      try {
        const [advRes, aliasRes] = await Promise.all([
          apiGet('/advanced', {
            'filter[category]': 'export',
            'filter[param]': 'transaction',
            'page[limit]': 10,
          }),
          apiGet('/aliases', { 'page[limit]': 5000 }),
        ])
        if (cancelled) return
        const advItems = (advRes?.data?.items || []) as Array<{ data?: { excludedCIDR?: ExcludedCIDRRow[] } }>
        const exportRow = advItems.find(() => true)
        const excludedCIDR = exportRow?.data?.excludedCIDR
        const aliases = (aliasRes?.data?.items || []) as AliasRow[]
        const { options: merged, defaultSelected } = mergeHostList(itemHosts, aliases, excludedCIDR)
        setOptions(merged)
        setSelectedIPs(defaultSelected)
      } catch {
        if (!cancelled) {
          setOptions(itemHosts)
          setSelectedIPs([])
        }
      } finally {
        if (!cancelled) setLoading(false)
      }
    }
    void run()
    return () => {
      cancelled = true
    }
  }, [itemHosts])

  return { options, selectedIPs, setSelectedIPs, loading }
}
