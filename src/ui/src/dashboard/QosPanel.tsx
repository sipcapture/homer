// @ts-nocheck - TODO: rewrite with shadcn/ui + TanStack; typed when refactored
import React, { useEffect, useRef, useMemo, useState, useCallback } from 'react'
import uPlot from 'uplot'
import 'uplot/dist/uPlot.min.css'
import { Button } from '@/components/ui/button'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { cn } from '@/lib/utils'
import { qosRouteArrow } from '@/lib/ipAliasDisplay'
import { useLocale } from '@/components/locale/locale-provider'

const METRIC_COLORS_RTCP = {
  packets:       { bg: 'rgba(244, 67, 54, 0.5)',  border: 'rgba(244, 67, 54, 1)' },
  octets:        { bg: 'rgba(233, 30, 99, 0.5)',   border: 'rgba(233, 30, 99, 1)' },
  highest_seq_no:{ bg: 'rgba(158, 158, 158, 0.5)', border: 'rgba(158, 158, 158, 1)' },
  ia_jitter:     { bg: 'rgba(0, 188, 212, 0.5)',   border: 'rgba(0, 188, 212, 1)' },
  lsr:           { bg: 'rgba(255, 255, 255, 0.3)',  border: 'rgba(200, 200, 200, 1)' },
  mos:           { bg: 'rgba(33, 150, 243, 0.5)',   border: 'rgba(33, 150, 243, 1)' },
  packets_lost:  { bg: 'rgba(244, 67, 54, 0.7)',   border: 'rgba(244, 67, 54, 1)' },
  fraction_lost: { bg: 'rgba(156, 39, 176, 0.5)',  border: 'rgba(156, 39, 176, 1)' },
}

const METRIC_COLORS_RTP = {
  TOTAL_PK:    { bg: 'rgba(244, 67, 54, 0.5)',  border: 'rgba(244, 67, 54, 1)' },
  EXPECTED_PK: { bg: 'rgba(76, 175, 80, 0.5)',   border: 'rgba(76, 175, 80, 1)' },
  JITTER:      { bg: 'rgba(0, 188, 212, 0.5)',   border: 'rgba(0, 188, 212, 1)' },
  MOS:         { bg: 'rgba(33, 150, 243, 0.5)',   border: 'rgba(33, 150, 243, 1)' },
  DELTA:       { bg: 'rgba(255, 152, 0, 0.5)',    border: 'rgba(255, 152, 0, 1)' },
  PACKET_LOSS: { bg: 'rgba(244, 67, 54, 0.7)',   border: 'rgba(244, 67, 54, 1)' },
}

const METRIC_COLORS_VQRTCP = {
  MOS_LQ:      { bg: 'rgba(33, 150, 243, 0.5)',   border: 'rgba(33, 150, 243, 1)' },
  MOS_CQ:      { bg: 'rgba(63, 81, 181, 0.5)',    border: 'rgba(63, 81, 181, 1)' },
  JITTER:      { bg: 'rgba(0, 188, 212, 0.5)',   border: 'rgba(0, 188, 212, 1)' },
  PACKET_LOSS: { bg: 'rgba(244, 67, 54, 0.7)',   border: 'rgba(244, 67, 54, 1)' },
}

const STAT_COLORS = [
  '#e53935', '#d81b60', '#8e24aa', '#5e35b1',
  '#1e88e5', '#00acc1', '#43a047', '#f4511e',
]

/** Fixed chart height avoids ResizeObserver ↔ flex feedback (layout jump until window resize). */
const QOS_CHART_HEIGHT = 300

function parsePayload(raw) {
  if (!raw) return null
  try {
    const s = typeof raw === 'string' ? raw : JSON.stringify(raw)
    return JSON.parse(s.replace(/\u0010/g, ''))
  } catch { return null }
}

/**
 * Normalize event time to Unix milliseconds (DuckDB/JSON may use string int64, μs, ns, or ISO strings).
 */
function parseTimestamp(val) {
  if (val === null || val === undefined || val === '') return 0
  if (val instanceof Date) {
    const t = val.getTime()
    return Number.isFinite(t) ? t : 0
  }
  if (typeof val === 'bigint') {
    const n = Number(val)
    return Number.isFinite(n) ? parseTimestamp(n) : 0
  }
  if (typeof val === 'number') {
    if (!Number.isFinite(val)) return 0
    const ax = Math.abs(val)
    if (ax >= 1e16) return Math.round(val / 1e6) // nanoseconds → ms
    if (ax >= 1e14) return Math.round(val / 1e3) // microseconds → ms
    if (ax >= 1e11) return Math.round(val) // already ms
    if (ax >= 1e8) return Math.round(val * 1000) // seconds → ms
    return Math.round(val * 1000)
  }
  if (typeof val === 'string') {
    const t = val.trim()
    if (!t) return 0
    if (/^-?\d+(\.\d+)?([eE][+-]?\d+)?$/.test(t)) {
      return parseTimestamp(Number(t))
    }
    let s = t
    if (/^\d{4}-\d{2}-\d{2}$/.test(s)) s = `${s}T00:00:00Z`
    else if (s.includes(' ') && !s.includes('T')) s = s.replace(' ', 'T')
    if (!/[zZ]|[+\-]\d{2}:?\d{2}$/.test(s)) s = `${s}Z`
    const d = new Date(s)
    return Number.isNaN(d.getTime()) ? 0 : d.getTime()
  }
  if (typeof val === 'object' && val !== null && typeof val.seconds === 'number') {
    const nanos = typeof val.nanos === 'number' ? val.nanos : 0
    return val.seconds * 1000 + Math.floor(nanos / 1e6)
  }
  return 0
}

/** Call-ID / session id for splitting aggregated QoS rows by leg. */
function qosSessionKey(item) {
  const sid = item?.session_id ?? item?.cid
  if (sid == null || sid === '') return ''
  return String(sid).trim()
}

/** First usable time column on a QoS row (API / legacy field names). */
function eventTimeMs(row) {
  if (!row || typeof row !== 'object') return 0
  for (const k of ['timestamp', 'time', 'create_date', 'ts']) {
    const ms = parseTimestamp(row[k])
    if (ms > 0) return ms
  }
  return 0
}

function formatAxisTime(unixSec, timeZone, locale) {
  const ms = unixSec * 1000
  if (!Number.isFinite(ms)) return '—'
  const opts = {
    hour: 'numeric',
    minute: 'numeric',
    second: 'numeric',
  }
  try {
    if (timeZone && timeZone !== 'local') {
      return new Intl.DateTimeFormat(locale, { ...opts, timeZone }).format(new Date(ms))
    }
    const d = new Date(ms)
    return `${String(d.getUTCHours()).padStart(2, '0')}:${String(d.getUTCMinutes()).padStart(2, '0')}:${String(d.getUTCSeconds()).padStart(2, '0')}`
  } catch {
    return '—'
  }
}

function calculateMOS(jitter, numPL, rtt = 10) {
  if (rtt === 0) rtt = 10
  const eLat = rtt + (jitter * 2) + 10
  let r = eLat < 160 ? 93.2 - (eLat / 40) : 93.2 - (eLat - 120) / 10
  r -= (numPL * 2.5)
  r = Math.max(0, Math.min(100, r))
  let mos = 1 + 0.035 * r + 0.000007 * r * (r - 60) * (100 - r)
  return Math.min(4.7, Math.round(mos * 100) / 100)
}

function processRTCPItems(items) {
  const streams = []
  const streamMap = {}
  const allPoints = []

  for (const item of items) {
    const payload = parsePayload(item.payload || item.raw || item.data_extra)
    if (!payload) continue

    const si = payload.sender_information
    if (!si || !si.packets || !si.octets) continue

    const type = payload.type ? Number(payload.type) : 0
    if (type && ![200, 201, 202, 207].includes(type)) continue

    const srcIp = item.src_ip || 'unknown'
    const dstIp = item.dst_ip || 'unknown'
    const srcPort = item.src_port || 0
    const dstPort = item.dst_port || 0
    const sid = qosSessionKey(item)
    const route = qosRouteArrow(item)
    const key = sid ? `${sid}|${srcIp}:${srcPort}` : `${srcIp}:${srcPort}`
    const ts = eventTimeMs(item)
    if (!ts) continue

    if (!streamMap[key]) {
      streamMap[key] = {
        key, sid, srcIp, dstIp, srcPort, dstPort,
        label: sid ? `${sid} · ${route}` : route,
        enabled: { packets: true, octets: true, highest_seq_no: true, ia_jitter: true, lsr: true, mos: true, packets_lost: true, fraction_lost: true },
      }
      streams.push(streamMap[key])
    }

    const block = (payload.report_blocks || [])[0] || {}
    const jitter = block.ia_jitter || 0
    let numPL = block.packets_lost || 0
    if (block.fraction_lost != null) {
      if (block.fraction_lost <= 0) numPL = 0
      else if (block.fraction_lost > 256) numPL = 100
      else numPL = (block.fraction_lost / 256) * 100
    }
    const mos = calculateMOS(jitter, numPL, block.dlsr < 1000 ? block.dlsr : 0)

    allPoints.push({
      ts, streamKey: key,
      packets: si.packets || 0,
      octets: si.octets || 0,
      highest_seq_no: block.highest_seq_no || 0,
      ia_jitter: jitter,
      lsr: (block.lsr || 0) * 1,
      mos,
      packets_lost: block.packets_lost || 0,
      fraction_lost: block.fraction_lost ? (block.fraction_lost <= 0 ? 0 : block.fraction_lost / 256) : 0,
    })
  }

  allPoints.sort((a, b) => a.ts - b.ts)
  return { streams, allPoints, metricKeys: ['packets', 'octets', 'highest_seq_no', 'ia_jitter', 'lsr', 'mos', 'packets_lost', 'fraction_lost'], colors: METRIC_COLORS_RTCP }
}

function processRTPItems(items) {
  const streams = []
  const streamMap = {}
  const allPoints = []

  for (const item of items) {
    const payload = parsePayload(item.payload || item.raw || item.data_extra)
    const srcIp = item.src_ip || 'unknown'
    const dstIp = item.dst_ip || 'unknown'
    const srcPort = item.src_port || 0
    const dstPort = item.dst_port || 0
    const sid = qosSessionKey(item)
    const route = qosRouteArrow(item)
    const key = sid ? `${sid}|${srcIp}:${srcPort}` : `${srcIp}:${srcPort}`
    const ts = eventTimeMs(item)
    if (!ts) continue

    if (!streamMap[key]) {
      streamMap[key] = {
        key, sid, srcIp, dstIp, srcPort, dstPort,
        label: sid ? `${sid} · ${route}` : route,
        enabled: { TOTAL_PK: true, EXPECTED_PK: true, JITTER: true, MOS: true, DELTA: true, PACKET_LOSS: true },
      }
      streams.push(streamMap[key])
    }

    if (!payload) continue
    const jitter = payload.JITTER || payload.jitter || 0
    const pLoss = payload.PACKET_LOSS || payload.packet_loss || 0

    allPoints.push({
      ts, streamKey: key,
      TOTAL_PK: payload.TOTAL_PK || payload.total_pk || 0,
      EXPECTED_PK: payload.EXPECTED_PK || payload.expected_pk || 0,
      JITTER: jitter,
      MOS: payload.MOS || payload.mos || calculateMOS(jitter, pLoss),
      DELTA: payload.DELTA || payload.delta || 0,
      PACKET_LOSS: pLoss,
    })
  }

  allPoints.sort((a, b) => a.ts - b.ts)
  return { streams, allPoints, metricKeys: ['TOTAL_PK', 'EXPECTED_PK', 'JITTER', 'MOS', 'DELTA', 'PACKET_LOSS'], colors: METRIC_COLORS_RTP }
}

function parseVqrtcpMessage(item) {
  const raw = item?.message ?? item?.data_extra
  if (!raw) return null
  if (typeof raw === 'object') return raw
  return parsePayload(raw)
}

function parseMetricFromKV(str, key) {
  if (!str || typeof str !== 'string') return 0
  const re = new RegExp(`${key}=([0-9.]+)`, 'i')
  const m = str.match(re)
  return m ? Number(m[1]) : 0
}

function processVQRTCPItems(items) {
  const streams = []
  const streamMap = {}
  const allPoints = []

  for (const item of items) {
    const msg = parseVqrtcpMessage(item) || {}
    const srcIp = item.source_ip || item.src_ip || 'unknown'
    const dstIp = item.destination_ip || item.dst_ip || 'unknown'
    const srcPort = item.source_port || item.src_port || 0
    const dstPort = item.destination_port || item.dst_port || 0
    const sid = qosSessionKey({ session_id: item.callid, cid: item.callid })
    const route = qosRouteArrow({ src_ip: srcIp, dst_ip: dstIp, src_port: srcPort, dst_port: dstPort })
    const key = sid ? `${sid}|${srcIp}:${srcPort}` : `${srcIp}:${srcPort}`
    const ts = eventTimeMs(item)
    if (!ts) continue

    if (!streamMap[key]) {
      streamMap[key] = {
        key, sid, srcIp, dstIp, srcPort, dstPort,
        label: sid ? `${sid} · ${route}` : route,
        enabled: { MOS_LQ: true, MOS_CQ: true, JITTER: true, PACKET_LOSS: true },
      }
      streams.push(streamMap[key])
    }

    const mosLQ = Number(msg.moslq ?? item.mos ?? 0)
    const mosCQ = Number(msg.moscq ?? 0)
    const jitter = parseMetricFromKV(msg.jitter_buffer, 'JBN')
    const packetLoss = parseMetricFromKV(msg.packet_loss, 'NLR')

    allPoints.push({
      ts, streamKey: key,
      MOS_LQ: mosLQ,
      MOS_CQ: mosCQ,
      JITTER: jitter,
      PACKET_LOSS: packetLoss,
      event: item.event || '',
    })
  }

  allPoints.sort((a, b) => a.ts - b.ts)
  return { streams, allPoints, metricKeys: ['MOS_LQ', 'MOS_CQ', 'JITTER', 'PACKET_LOSS'], colors: METRIC_COLORS_VQRTCP }
}

function computeStats(allPoints, metricKeys) {
  const stats = {}
  for (const mk of metricKeys) {
    const vals = allPoints.map(p => p[mk]).filter(v => v != null && !isNaN(v) && v !== 0)
    if (vals.length === 0) {
      stats[mk] = { min: 0, avg: 0, max: 0 }
    } else {
      stats[mk] = {
        min: Math.min(...vals),
        avg: Math.round(vals.reduce((a, b) => a + b, 0) / vals.length * 100) / 100,
        max: Math.max(...vals),
      }
    }
  }
  return stats
}

function CombinedChart({ allPoints, metricKeys, colors, streams, height, chartType, timeZone, locale }) {
  const containerRef = useRef(null)
  const chartRef = useRef(null)
  const rafRef = useRef(null)
  const narrowWidthRafAttempts = useRef(0)

  const buildChart = useCallback(() => {
    const el = containerRef.current
    if (!el || allPoints.length === 0) return

    const w = el.clientWidth
    const h = height || QOS_CHART_HEIGHT
    if (w < 50) {
      if (narrowWidthRafAttempts.current > 120) return
      narrowWidthRafAttempts.current += 1
      rafRef.current = requestAnimationFrame(buildChart)
      return
    }
    narrowWidthRafAttempts.current = 0

    const cs = getComputedStyle(el)
    const axisStroke = cs.getPropertyValue('--muted-foreground').trim() || '#8b949e'
    const gridStroke = cs.getPropertyValue('--border').trim() || 'rgba(255,255,255,0.1)'

    const enabledMetrics = metricKeys.filter(mk =>
      streams.some(s => s.enabled[mk])
    )
    if (enabledMetrics.length === 0) return

    const tsArr = allPoints.map(p => p.ts / 1000)
    const seriesData = [tsArr]
    const seriesOpts = [{ label: 'Time' }]

    for (const mk of enabledMetrics) {
      const data = allPoints.map(p => {
        const stream = streams.find(s => s.key === p.streamKey)
        if (!stream || !stream.enabled[mk]) return 0
        const v = p[mk]
        return v != null && !isNaN(v) ? v : 0
      })
      seriesData.push(data)
      const c = colors[mk] || { bg: 'rgba(100,100,100,0.5)', border: 'rgba(100,100,100,1)' }
      seriesOpts.push({
        label: mk,
        stroke: c.border,
        fill: c.bg,
        width: 2,
        paths: chartType === 'line' ? uPlot.paths.linear() : uPlot.paths.bars({ size: [0.7, 100] }),
        points: { show: false },
      })
    }

    const opts = {
      width: w,
      height: h,
      cursor: { show: true, drag: { x: false, y: false } },
      select: { show: false },
      legend: { show: false },
      axes: [
        {
          stroke: axisStroke,
          grid: { stroke: gridStroke, width: 1 },
          ticks: { stroke: gridStroke, width: 1 },
          font: '10px Inter, sans-serif',
          values: (u, vals) => vals.map(v => formatAxisTime(v, timeZone, locale)),
          gap: 4,
        },
        {
          stroke: axisStroke,
          grid: { stroke: gridStroke, width: 1 },
          ticks: { stroke: gridStroke, width: 1 },
          font: '10px Inter, sans-serif',
          size: 60,
          values: (u, vals) => vals.map(v => {
            if (v >= 1000000) return (v / 1000000).toFixed(1) + 'M'
            if (v >= 1000) return (v / 1000).toFixed(0) + 'k'
            return v.toFixed(v < 10 ? 2 : 0)
          }),
        },
      ],
      series: seriesOpts,
    }

    if (chartRef.current) chartRef.current.destroy()
    chartRef.current = new uPlot(opts, seriesData, el)
  }, [allPoints, metricKeys, colors, streams, height, chartType, timeZone, locale])

  useEffect(() => {
    narrowWidthRafAttempts.current = 0
    rafRef.current = requestAnimationFrame(buildChart)

    const el = containerRef.current
    if (!el) return

    const ro = new ResizeObserver(() => {
      if (chartRef.current && el.clientWidth > 0) {
        chartRef.current.setSize({ width: el.clientWidth, height: height || QOS_CHART_HEIGHT })
      }
    })
    ro.observe(el)

    return () => {
      if (rafRef.current) cancelAnimationFrame(rafRef.current)
      ro.disconnect()
      if (chartRef.current) { chartRef.current.destroy(); chartRef.current = null }
    }
  }, [buildChart, height])

  return (
    <div className="w-full" style={{ height: (height || QOS_CHART_HEIGHT) + 'px' }}>
      <div ref={containerRef} className="h-full w-full" />
    </div>
  )
}

function formatNum(v) {
  if (v === 0) return '0'
  if (Math.abs(v) >= 1e6) return (v / 1e6).toFixed(2) + 'M'
  if (Math.abs(v) >= 1e3) return (v / 1e3).toFixed(2) + 'k'
  return v % 1 === 0 ? v.toString() : v.toFixed(2)
}

function StatBoxes({ stats, metricKeys }) {
  return (
    <div className="grid grid-cols-2 gap-1.5 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-6">
      {metricKeys.map((mk, i) => {
        const s = stats[mk]
        if (!s) return null
        const color = STAT_COLORS[i % STAT_COLORS.length]
        return (
          <React.Fragment key={mk}>
            <div
              className="flex flex-col gap-0.5 border border-l-4 border-border bg-card px-2 py-1.5"
              style={{ borderLeftColor: color }}
            >
              <span className="text-[9px] uppercase tracking-wider text-muted-foreground">
                MIN {mk.toUpperCase()}
              </span>
              <span className="font-mono text-xs text-foreground">{formatNum(s.min)}</span>
            </div>
            <div
              className="flex flex-col gap-0.5 border border-l-4 border-border bg-card px-2 py-1.5"
              style={{ borderLeftColor: color }}
            >
              <span className="text-[9px] uppercase tracking-wider text-muted-foreground">
                AVG {mk.toUpperCase()}
              </span>
              <span className="font-mono text-xs text-foreground">{formatNum(s.avg)}</span>
            </div>
            <div
              className="flex flex-col gap-0.5 border border-l-4 border-border bg-card px-2 py-1.5"
              style={{ borderLeftColor: color }}
            >
              <span className="text-[9px] uppercase tracking-wider text-muted-foreground">
                MAX {mk.toUpperCase()}
              </span>
              <span className="font-mono text-xs text-foreground">{formatNum(s.max)}</span>
            </div>
          </React.Fragment>
        )
      })}
    </div>
  )
}

function StreamCheckboxes({ streams, metricKeys, colors, onChange }) {
  return (
    <div className="flex flex-col gap-2">
      {streams.map(stream => (
        <div key={stream.key} className="border border-border bg-card px-2 py-2">
          <div className="flex flex-col gap-0.5 border-b border-border pb-1.5 font-mono text-[10px] text-foreground">
            {stream.sid && (
              <span className="break-all text-muted-foreground">{stream.sid}</span>
            )}
            <div className="flex flex-wrap items-center gap-2">
              <span>{stream.srcIp}:{stream.srcPort}</span>
              <span className="text-muted-foreground">→</span>
              <span>{stream.dstIp}:{stream.dstPort}</span>
            </div>
          </div>
          <div className="mt-1.5 flex flex-wrap gap-x-3 gap-y-1">
            {metricKeys.map(mk => {
              const c = colors[mk]
              return (
                <label key={mk} className="flex cursor-pointer items-center gap-1 font-mono text-[10px] text-foreground">
                  <input
                    type="checkbox"
                    checked={stream.enabled[mk]}
                    onChange={() => onChange(stream.key, mk)}
                    className="h-3 w-3"
                  />
                  <span
                    className="inline-block h-2 w-2 rounded-full"
                    style={{ background: c?.border || '#888' }}
                  />
                  {mk}
                </label>
              )
            })}
          </div>
        </div>
      ))}
    </div>
  )
}

export default function QosPanel({ qosData, timeZone }) {
  const { resolved: locale } = useLocale()
  const [subTab, setSubTab] = useState('rtcp')
  const [chartType, setChartType] = useState('bar')
  const [rtcpStreams, setRtcpStreams] = useState([])
  const [rtpStreams, setRtpStreams] = useState([])
  const [vqrtcpStreams, setVqrtcpStreams] = useState([])

  const rtcpParsed = useMemo(() => processRTCPItems(qosData?.rtcp?.data || []), [qosData])
  const rtpParsed = useMemo(() => processRTPItems(qosData?.rtp?.data || []), [qosData])
  const vqrtcpParsed = useMemo(() => processVQRTCPItems(qosData?.vqrtcp?.data || []), [qosData])

  useEffect(() => { setRtcpStreams(rtcpParsed.streams) }, [rtcpParsed])
  useEffect(() => { setRtpStreams(rtpParsed.streams) }, [rtpParsed])
  useEffect(() => { setVqrtcpStreams(vqrtcpParsed.streams) }, [vqrtcpParsed])

  const hasRTCP = rtcpParsed.allPoints.length > 0
  const hasRTP = rtpParsed.allPoints.length > 0
  const hasVQRTCP = vqrtcpParsed.allPoints.length > 0

  const toggleMetric = useCallback((streamKey, metricKey) => {
    const setter = subTab === 'rtcp' ? setRtcpStreams : subTab === 'rtp' ? setRtpStreams : setVqrtcpStreams
    setter(prev => prev.map(s =>
      s.key === streamKey ? { ...s, enabled: { ...s.enabled, [metricKey]: !s.enabled[metricKey] } } : s
    ))
  }, [subTab])

  if (!hasRTCP && !hasRTP && !hasVQRTCP) {
    return (
      <div className="flex h-full items-center justify-center p-6 text-xs text-muted-foreground">
        No QoS data available for this transaction.
      </div>
    )
  }

  const parsed = subTab === 'rtcp' ? rtcpParsed : subTab === 'rtp' ? rtpParsed : vqrtcpParsed
  const activeStreams = subTab === 'rtcp' ? rtcpStreams : subTab === 'rtp' ? rtpStreams : vqrtcpStreams
  const stats = computeStats(parsed.allPoints, parsed.metricKeys)

  return (
    <div className="flex h-full min-h-0 flex-col gap-2 p-2">
      {(hasRTCP || hasRTP || hasVQRTCP) && (
        <Tabs value={subTab} onValueChange={setSubTab}>
          <TabsList variant="line" className="h-8 border-b border-border">
            {hasRTCP && (
              <TabsTrigger value="rtcp">
                RTCP ({(qosData?.rtcp?.data || []).length})
              </TabsTrigger>
            )}
            {hasRTP && (
              <TabsTrigger value="rtp">
                RTP ({(qosData?.rtp?.data || []).length})
              </TabsTrigger>
            )}
            {hasVQRTCP && (
              <TabsTrigger value="vqrtcp">
                VQRTCP ({(qosData?.vqrtcp?.data || []).length})
              </TabsTrigger>
            )}
          </TabsList>
        </Tabs>
      )}

      <div className="flex items-center justify-between gap-2">
        <div className="flex flex-wrap gap-2">
          {activeStreams.map((s, i) => (
            <span
              key={s.key}
              className="flex items-center gap-1 font-mono text-[10px]"
              style={{ color: STAT_COLORS[i % STAT_COLORS.length] }}
            >
              <span
                className="inline-block h-2 w-2 rounded-full"
                style={{ background: STAT_COLORS[i % STAT_COLORS.length] }}
              />
              {s.label}
            </span>
          ))}
        </div>
        <div className="flex gap-1">
          <Button
            type="button"
            variant={chartType === 'bar' ? 'default' : 'outline'}
            size="sm"
            className="h-6 text-[10px]"
            onClick={() => setChartType('bar')}
          >
            Bar
          </Button>
          <Button
            type="button"
            variant={chartType === 'line' ? 'default' : 'outline'}
            size="sm"
            className="h-6 text-[10px]"
            onClick={() => setChartType('line')}
          >
            Line
          </Button>
        </div>
      </div>

      <div className="w-full shrink-0">
        <CombinedChart
          allPoints={parsed.allPoints}
          metricKeys={parsed.metricKeys}
          colors={parsed.colors}
          streams={activeStreams}
          height={QOS_CHART_HEIGHT}
          chartType={chartType}
          timeZone={timeZone}
          locale={locale}
        />
      </div>

      <StreamCheckboxes
        streams={activeStreams}
        metricKeys={parsed.metricKeys}
        colors={parsed.colors}
        onChange={toggleMetric}
      />

      <div className="flex min-h-0 flex-1 flex-col gap-2 overflow-y-auto">
        <StatBoxes stats={stats} metricKeys={parsed.metricKeys} />
      </div>
    </div>
  )
}
