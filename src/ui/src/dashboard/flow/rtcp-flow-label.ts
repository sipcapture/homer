import { displayDstIp, displaySrcIp, qosRouteArrow } from '@/lib/ipAliasDisplay'
import type { RawMessage } from './flow-data'

const RTCP_TYPE_NAMES: Record<number, string> = {
  200: 'SR',
  201: 'RR',
  202: 'SDES',
  204: 'APP',
  205: 'FIR',
  206: 'NACK',
  207: 'XR',
}

function parsePayload(raw: unknown): Record<string, unknown> | null {
  if (!raw) return null
  if (typeof raw === 'object' && !Array.isArray(raw)) return raw as Record<string, unknown>
  if (typeof raw !== 'string') return null
  try {
    const parsed = JSON.parse(raw.replaceAll('\u0010', ''))
    return parsed && typeof parsed === 'object' && !Array.isArray(parsed)
      ? (parsed as Record<string, unknown>)
      : null
  } catch {
    return null
  }
}

function routeDescription(msg: RawMessage): string {
  const row = msg as Record<string, unknown>
  return qosRouteArrow(row) || `${displaySrcIp(row)} \u2192 ${displayDstIp(row)}`
}

function fractionLostPercent(fractionLost: unknown): number | null {
  if (fractionLost == null || fractionLost === '') return null
  const fl = Number(fractionLost)
  if (!Number.isFinite(fl)) return null
  if (fl <= 0) return 0
  if (fl > 256) return 100
  return (fl / 256) * 100
}

function formatLoss(block: Record<string, unknown>): string {
  const pct = fractionLostPercent(block.fraction_lost)
  if (pct != null) {
    const digits = Number.isInteger(pct) || Math.abs(pct - Math.round(pct)) < 0.05 ? 0 : 1
    return `loss=${pct.toFixed(digits)}%`
  }
  if (block.packets_lost != null && block.packets_lost !== '') {
    return `loss=${block.packets_lost}`
  }
  return ''
}

/** Compact Call Flow label for an RTCP JSON report (HEP proto 5). */
export function computeRtcpFlowLabels(msg: RawMessage): { method: string; description: string } {
  const payload = parsePayload(msg.payload ?? msg.raw ?? msg.data_extra)
  if (!payload) {
    return { method: 'RTCP', description: routeDescription(msg) }
  }

  const type = Number(payload.type) || 0
  const typeName = RTCP_TYPE_NAMES[type]
  const method = typeName ? `RTCP ${typeName}` : 'RTCP'

  const blocks = payload.report_blocks
  const block =
    Array.isArray(blocks) && blocks[0] && typeof blocks[0] === 'object'
      ? (blocks[0] as Record<string, unknown>)
      : {}

  const parts: string[] = []
  if (block.ia_jitter != null && block.ia_jitter !== '') {
    parts.push(`jitter=${block.ia_jitter}`)
  }
  const loss = formatLoss(block)
  if (loss) parts.push(loss)

  return {
    method,
    description: parts.length > 0 ? parts.join(' ') : routeDescription(msg),
  }
}
