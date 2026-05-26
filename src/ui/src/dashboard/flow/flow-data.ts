import type { CallIdColors } from '../flow-utils'
import { getColorByString, getHighContrastColor, getMethodColor } from '../flow-utils'
import {
  aliasDstEnrichment,
  aliasSrcEnrichment,
  displayDstIp,
  displaySrcIp,
  qosRouteArrow,
} from '@/lib/ipAliasDisplay'
import { computeSipFlowLabels } from './sip-flow-label'

export interface RawMessage {
  uuid?: string
  src_ip?: string
  dst_ip?: string
  src_port?: number | string
  dst_port?: number | string
  session_id?: string
  cid?: string
  sip_method?: string
  method?: string
  event?: string
  protocol?: string | number
  ruri_user?: string
  timestamp?: string | number | Date
  create_ts?: string | number | Date
  [key: string]: unknown
}

export interface Host {
  ip: string
  port: number | string
  key: string
  ports?: Array<number | string>
  /** Distinct IPs merged into this column (group-by-ip / group-by-alias). */
  ips?: string[]
  /** Friendly name from IP alias enrichment (aliasSrc / aliasDst) */
  displayLabel?: string
  /** Custom image URL from matched alias row (call flow header) */
  aliasImage?: string
  /** Tag values (tag1…tag4) from matched alias row */
  aliasTags?: string[]
}

export interface FlowItemData {
  id: string | number
  idx: number
  method: string
  description: string
  srcIp: string
  dstIp: string
  srcPort: number | string
  dstPort: number | string
  callid: string
  callidColors: CallIdColors
  methodColor: string
  timeStr: string
  fullDateStr: string
  diffStr: string
  protoLabel: string
  payloadType: string
  start: number
  middle: number
  rightEnd: number
  direction: boolean
  isRadial: boolean
  isLastHost: boolean
  arrowStyleSolid: boolean
  raw: RawMessage
}

export interface CallIdLegend {
  callid: string
  colors: CallIdColors
}

export type HostGrouping = 'ungrouped' | 'group-by-ip' | 'group-by-alias'

const ALIAS_KEY_PREFIX = 'alias:'

function rowOf(msg: RawMessage): Record<string, unknown> {
  return msg as Record<string, unknown>
}

/** Resolved alias label for an endpoint, or empty when enrichment has no alias. */
export function endpointAlias(row: Record<string, unknown>, side: 'src' | 'dst'): string {
  const ip = String(side === 'src' ? row.src_ip ?? '' : row.dst_ip ?? '')
  const lbl = side === 'src' ? displaySrcIp(row) : displayDstIp(row)
  if (lbl && lbl !== ip) return lbl
  return ''
}

function mergeHostEndpoint(
  host: Host,
  ip: string,
  port: number | string,
  grouping: HostGrouping,
  aliasLabel: string,
): void {
  if (grouping === 'group-by-ip' || grouping === 'group-by-alias') {
    if (!host.ports!.includes(port)) host.ports!.push(port)
    if (grouping === 'group-by-alias' && ip && !host.ips!.includes(ip)) host.ips!.push(ip)
  }
  if (grouping === 'group-by-alias' && aliasLabel && !host.displayLabel) {
    host.displayLabel = aliasLabel
  }
}

/**
 * Build the call-ID legend from a *raw* (un-filtered) message list so
 * the CallIdSelector can keep showing every call-ID — including the
 * ones the user has just toggled off — and remain a togglable filter
 * UI rather than a destructive click-once-and-it's-gone control.
 *
 * Coloring is stable: the first call-ID encountered (in capture-time
 * order) is treated as the "base" and influences the high-contrast
 * palette used for the rest, matching what buildFlow does internally
 * so the legend colors line up with the arrows on screen.
 */
export function buildCallIdLegend(items: RawMessage[] | null | undefined): CallIdLegend[] {
  if (!items || items.length === 0) return []
  const seen = new Set<string>()
  const ordered: string[] = []
  for (const m of items) {
    const cid = m.session_id || m.cid || ''
    if (!cid || seen.has(cid)) continue
    seen.add(cid)
    ordered.push(cid)
  }
  if (ordered.length === 0) return []
  const base = ordered[0]
  return ordered.map((cid) => ({ callid: cid, colors: getHighContrastColor(base, cid, ordered) }))
}

export function parseTs(value: unknown): Date | null {
  if (value === undefined || value === null || value === '') return null
  if (value instanceof Date) return value
  if (typeof value === 'number') {
    let ms = value
    if (value > 1e15) ms = Math.round(value / 1e6)
    else if (value > 1e12) ms = Math.round(value)
    else if (value > 1e9) ms = Math.round(value * 1000)
    return new Date(ms)
  }
  if (typeof value === 'string') {
    let s = value.trim()
    if (/^\d{4}-\d{2}-\d{2}$/.test(s)) return new Date(`${s}T00:00:00Z`)
    if (s.includes(' ') && !s.includes('T')) s = s.replace(' ', 'T')
    if (!/[zZ]|[+-]\d{2}:?\d{2}$/.test(s)) s = `${s}Z`
    const d = new Date(s)
    return Number.isNaN(d.getTime()) ? null : d
  }
  return null
}

export function protoLabelOf(proto: string | number | undefined): string {
  const p = String(proto ?? '')
  if (p === '17' || p === '1' || p === 'sip' || p === 'SIP') return 'UDP'
  if (p === '6' || p === '2') return 'TCP'
  if (p === '3') return 'WSS'
  if (p === '22') return 'TLS'
  return ''
}

export function payloadTypeOf(msg: RawMessage): string {
  const p = String(msg.protocol ?? '')
  if (p === '1' || p === 'sip' || p === 'SIP' || p === '17') return 'SIP'
  if (p === '6' || p === '2') return 'TCP'
  if (p === '3') return 'WSS'
  if (p === '22') return 'TLS'
  if (p === '5') return 'RTP'
  if (p === '34' || p === '35') return 'RTCP'
  if (p === '38') return 'DTMF'
  if (p === '100') return 'LOG'
  if (msg.sip_method || msg.method) return 'SIP'
  return 'OTHER'
}

export function shortcutIPv6(str: string): string {
  if (!str) return ''
  const m = str.match(/^\[?([\da-fA-F]+):.*:([\da-fA-F]+)]?$/)
  if (m) return `${m[1]}:...:${m[2]}`
  return str
}

export function hostKey(
  ip: string,
  port: number | string,
  grouping: HostGrouping,
  row?: Record<string, unknown>,
  side?: 'src' | 'dst',
): string {
  if (grouping === 'group-by-ip') return ip
  if (grouping === 'group-by-alias' && row && side) {
    const alias = endpointAlias(row, side)
    if (alias) return `${ALIAS_KEY_PREFIX}${alias}`
    return `${ip}:${port}`
  }
  return `${ip}:${port}`
}

export function hostKeyFromMessage(
  msg: RawMessage,
  side: 'src' | 'dst',
  grouping: HostGrouping,
): string {
  const row = rowOf(msg)
  const ip = String(side === 'src' ? row.src_ip ?? 'unknown' : row.dst_ip ?? 'unknown')
  const port = side === 'src' ? (row.src_port ?? 0) : (row.dst_port ?? 0)
  return hostKey(ip, port as number | string, grouping, row, side)
}

export function buildHosts(items: RawMessage[], grouping: HostGrouping): Host[] {
  const order: string[] = []
  const map = new Map<string, Host>()
  const merges = grouping === 'group-by-ip' || grouping === 'group-by-alias'

  items.forEach((msg) => {
    const row = rowOf(msg)
    const srcIp = msg.src_ip || 'unknown'
    const dstIp = msg.dst_ip || 'unknown'
    const srcPort = msg.src_port ?? 0
    const dstPort = msg.dst_port ?? 0
    const srcAlias = grouping === 'group-by-alias' ? endpointAlias(row, 'src') : ''
    const dstAlias = grouping === 'group-by-alias' ? endpointAlias(row, 'dst') : ''
    const srcKey = hostKey(srcIp, srcPort, grouping, row, 'src')
    const dstKey = hostKey(dstIp, dstPort, grouping, row, 'dst')

    if (!map.has(srcKey)) {
      const h: Host = {
        ip: srcIp,
        port: srcPort,
        key: srcKey,
        ports: [srcPort],
        ips: merges ? [srcIp] : undefined,
      }
      if (srcAlias) h.displayLabel = srcAlias
      map.set(srcKey, h)
      order.push(srcKey)
    } else if (merges) {
      mergeHostEndpoint(map.get(srcKey)!, srcIp, srcPort, grouping, srcAlias)
    }

    if (!map.has(dstKey)) {
      const h: Host = {
        ip: dstIp,
        port: dstPort,
        key: dstKey,
        ports: [dstPort],
        ips: merges ? [dstIp] : undefined,
      }
      if (dstAlias) h.displayLabel = dstAlias
      map.set(dstKey, h)
      order.push(dstKey)
    } else if (merges) {
      mergeHostEndpoint(map.get(dstKey)!, dstIp, dstPort, grouping, dstAlias)
    }
  })
  return order.map((k) => map.get(k) as Host)
}

/** Host column: alias label, optional image and tags from first matching enriched message. */
export function resolveHostFlowMeta(
  host: Host,
  items: RawMessage[],
  grouping: HostGrouping,
): { displayLabel: string; aliasImage: string; aliasTags: string[] } {
  const empty = { displayLabel: '', aliasImage: '', aliasTags: [] as string[] }
  for (const msg of items) {
    const srcIp = msg.src_ip || 'unknown'
    const dstIp = msg.dst_ip || 'unknown'
    const sp = msg.src_port ?? 0
    const dp = msg.dst_port ?? 0
    const row = rowOf(msg)
    if (grouping === 'group-by-ip') {
      if (srcIp === host.ip) {
        const lbl = displaySrcIp(row)
        const en = aliasSrcEnrichment(row)
        const hasAlias = lbl !== '' && lbl !== srcIp
        if (hasAlias || en.image || en.tags.length > 0) {
          return { displayLabel: hasAlias ? lbl : '', aliasImage: en.image, aliasTags: en.tags }
        }
      }
      if (dstIp === host.ip) {
        const lbl = displayDstIp(row)
        const en = aliasDstEnrichment(row)
        const hasAlias = lbl !== '' && lbl !== dstIp
        if (hasAlias || en.image || en.tags.length > 0) {
          return { displayLabel: hasAlias ? lbl : '', aliasImage: en.image, aliasTags: en.tags }
        }
      }
    } else {
      if (hostKey(srcIp, sp, grouping, row, 'src') === host.key) {
        const lbl = displaySrcIp(row)
        const en = aliasSrcEnrichment(row)
        const hasAlias = lbl !== '' && lbl !== srcIp
        if (hasAlias || en.image || en.tags.length > 0) {
          return { displayLabel: hasAlias ? lbl : '', aliasImage: en.image, aliasTags: en.tags }
        }
      }
      if (hostKey(dstIp, dp, grouping, row, 'dst') === host.key) {
        const lbl = displayDstIp(row)
        const en = aliasDstEnrichment(row)
        const hasAlias = lbl !== '' && lbl !== dstIp
        if (hasAlias || en.image || en.tags.length > 0) {
          return { displayLabel: hasAlias ? lbl : '', aliasImage: en.image, aliasTags: en.tags }
        }
      }
    }
  }
  if (grouping === 'group-by-alias' && host.displayLabel) {
    return { ...empty, displayLabel: host.displayLabel }
  }
  return empty
}

/** @deprecated Use resolveHostFlowMeta; kept for callers that only need the label string. */
export function resolveHostDisplayLabel(
  host: Host,
  items: RawMessage[],
  grouping: HostGrouping,
): string {
  return resolveHostFlowMeta(host, items, grouping).displayLabel
}

function indexOfHost(
  hosts: Host[],
  msg: RawMessage,
  side: 'src' | 'dst',
  grouping: HostGrouping,
): number {
  const key = hostKeyFromMessage(msg, side, grouping)
  return hosts.findIndex((h) => h.key === key)
}

interface BuildOpts {
  timeZone?: string
  grouping: HostGrouping
}

export interface BuildResult {
  hosts: Host[]
  flowItems: FlowItemData[]
  callIds: CallIdLegend[]
}

export function buildFlow(items: RawMessage[] | null | undefined, opts: BuildOpts): BuildResult {
  if (!items || items.length === 0) return { hosts: [], flowItems: [], callIds: [] }

  // Items arrive already ORDER BY timestamp ASC from
  // /transactions/messages — capture-time order is the only one we
  // support, no client-side re-sort.
  const sorted = items
  const grouping = opts.grouping
  const hostsRaw = buildHosts(sorted, grouping)
  const hosts: Host[] = hostsRaw.map((h) => {
    const meta = resolveHostFlowMeta(h, sorted, grouping)
    const out: Host = { ...h }
    if (meta.displayLabel) out.displayLabel = meta.displayLabel
    if (meta.aliasImage) out.aliasImage = meta.aliasImage
    if (meta.aliasTags.length > 0) out.aliasTags = meta.aliasTags
    return out
  })

  const callIdSet = new Set<string>()
  sorted.forEach((m) => {
    const cid = m.session_id || m.cid || ''
    if (cid) callIdSet.add(cid)
  })
  const allCallIds = Array.from(callIdSet)
  const baseCallId = allCallIds[0] || ''

  const fmt: Intl.DateTimeFormatOptions = {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
    fractionalSecondDigits: 3,
    hour12: false,
  }
  const formatter =
    opts.timeZone && opts.timeZone !== 'local'
      ? new Intl.DateTimeFormat('en-GB', { ...fmt, timeZone: opts.timeZone })
      : new Intl.DateTimeFormat('en-GB', fmt)

  let prevTs = 0
  const flowItems: FlowItemData[] = sorted.map((msg, idx) => {
    const srcIp = msg.src_ip || 'unknown'
    const dstIp = msg.dst_ip || 'unknown'
    const srcPort = msg.src_port ?? 0
    const dstPort = msg.dst_port ?? 0

    const callid = msg.session_id || msg.cid || ''
    const callidColors = callid
      ? getHighContrastColor(baseCallId, callid, allCallIds)
      : {
          color: getColorByString(`item-${idx}`, 60, 70, 0.2),
          tabColor: getColorByString(`item-${idx}`, 60, 55, 1),
          arrowColor: getColorByString(`item-${idx}`, 60, 55, 0.55),
        }

    let method = msg.sip_method || msg.method || msg.event || ''
    const proto = msg.protocol

    const srcIdx = indexOfHost(hosts, msg, 'src', grouping)
    const dstIdx = indexOfHost(hosts, msg, 'dst', grouping)

    const isRadial = srcIdx === dstIdx
    const isLastHost = isRadial && hosts.length > 1 && srcIdx === hosts.length - 1

    const a = srcIdx
    const b = dstIdx
    const singleHost = hosts.length === 1
    const start = singleHost ? 0 : Math.min(a, b)
    const middle = Math.abs(a - b) || 1
    const direction = isLastHost || a > b
    const rightEnd = singleHost ? 0 : hosts.length - 1 - Math.max(a, b)

    const protoStr = String(proto ?? '')
    const isSIP =
      !protoStr || protoStr === '1' || protoStr === '17' || protoStr === 'sip' || protoStr === 'SIP'
    const arrowStyleSolid = isSIP

    const date = parseTs(msg.timestamp ?? msg.create_ts)
    const ts = date ? date.getTime() : 0
    if (prevTs === 0) prevTs = ts
    const diffMs = ts - prevTs
    prevTs = ts

    const fullDateStr = date ? formatter.format(date).replace(',', '') : ''
    const diffStr = `+${diffMs.toFixed(1)}ms`

    let description =
      (msg.ruri_user as string | undefined) ||
      qosRouteArrow(msg as Record<string, unknown>) ||
      `${displaySrcIp(msg as Record<string, unknown>)} \u2192 ${displayDstIp(msg as Record<string, unknown>)}`

    if (payloadTypeOf(msg) === 'SIP') {
      const sipLabels = computeSipFlowLabels(msg as Record<string, unknown>)
      if (sipLabels) {
        method = sipLabels.method
        description = sipLabels.description
      }
    }

    return {
      id: msg.uuid || idx,
      idx,
      method,
      description,
      srcIp,
      dstIp,
      srcPort,
      dstPort,
      callid,
      callidColors,
      methodColor: getMethodColor(method),
      timeStr: fullDateStr,
      fullDateStr,
      diffStr,
      protoLabel: protoLabelOf(proto),
      payloadType: payloadTypeOf(msg),
      start,
      middle,
      rightEnd,
      direction,
      isRadial,
      isLastHost,
      arrowStyleSolid,
      raw: msg,
    }
  })

  const callIds: CallIdLegend[] = allCallIds.map((cid) => ({
    callid: cid,
    colors: getHighContrastColor(baseCallId, cid, allCallIds),
  }))

  return { hosts, flowItems, callIds }
}

