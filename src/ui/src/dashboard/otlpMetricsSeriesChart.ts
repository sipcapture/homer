// @ts-nocheck — shared OTLP metrics uPlot helpers (modal + Results inline chart)
import uPlot from 'uplot'

export function rowTimestampMs(row) {
  const v = row?.timestamp ?? row?.TIMESTAMP
  if (v == null) return 0
  const d = v instanceof Date ? v : new Date(v)
  const t = d.getTime()
  return Number.isNaN(t) ? 0 : t
}

export function normalizeKind(typ) {
  return String(typ ?? '')
    .toLowerCase()
    .trim() || 'gauge'
}

export function dominantMetricKind(rows) {
  if (!Array.isArray(rows) || rows.length === 0) return 'gauge'
  const counts = new Map()
  for (const row of rows) {
    const k = normalizeKind(row?.type ?? row?.TYPE)
    counts.set(k, (counts.get(k) || 0) + 1)
  }
  let best = 'gauge'
  let n = -1
  for (const [k, c] of counts) {
    if (c > n) {
      n = c
      best = k
    }
  }
  return best
}

export function isHistogramKind(kind) {
  return kind === 'histogram' || kind === 'exponential_histogram' || kind === 'summary'
}

export function chartKindCaption(kind) {
  switch (kind) {
    case 'gauge':
      return 'Gauge — value over time'
    case 'sum':
      return 'Sum — cumulative / monotonic value over time'
    case 'histogram':
      return 'Histogram — aggregate sum and sample count (per-bucket detail is in raw JSON)'
    case 'exponential_histogram':
      return 'Exponential histogram — aggregate sum and sample count'
    case 'summary':
      return 'Summary — aggregate sum and sample count'
    default:
      return `Type “${kind}” — numeric value over time`
  }
}

/**
 * Build uPlot-aligned series from OTLP metric rows. Histogram-like kinds use
 * value_double as sum and value_int as count (see otlp_storage.go).
 */
export function buildMetricChartPayload(rows, kind) {
  const histo = isHistogramKind(kind)
  const xs = []
  const yPrimary = []
  const ySecondary = []
  for (const row of rows) {
    const tMs = rowTimestampMs(row)
    if (!tMs) continue
    const sec = tMs / 1000
    const vd = row?.value_double ?? row?.VALUE_DOUBLE
    const vi = row?.value_int ?? row?.VALUE_INT
    if (histo) {
      let sum = NaN
      let cnt = NaN
      if (vd != null && vd !== '') sum = Number(vd)
      if (vi != null && vi !== '') cnt = Number(vi)
      if (!Number.isNaN(sum) || !Number.isNaN(cnt)) {
        xs.push(sec)
        yPrimary.push(Number.isNaN(sum) ? 0 : sum)
        ySecondary.push(Number.isNaN(cnt) ? 0 : cnt)
      }
    } else {
      let v = NaN
      if (vd != null && vd !== '') v = Number(vd)
      else if (vi != null && vi !== '') v = Number(vi)
      if (!Number.isNaN(v)) {
        xs.push(sec)
        yPrimary.push(v)
      }
    }
  }
  if (xs.length === 0) return null
  if (histo) {
    return { dual: true, xs, yPrimary, ySecondary, aligned: [xs, yPrimary, ySecondary] }
  }
  return { dual: false, xs, yPrimary, ySecondary: null, aligned: [xs, yPrimary] }
}

function pad2(n) {
  return String(n).padStart(2, '0')
}

/** Visual style for OTLP metric time series: line, filled area, or bar (histogram) columns. */
export const OTLP_METRIC_CHART_TYPES = /** @type {const} */ (['line', 'area', 'histogram'])

/** @param {string} [v] */
export function normalizeOtlpMetricChartType(v) {
  return OTLP_METRIC_CHART_TYPES.includes(/** @type {any} */ (v)) ? v : 'area'
}

/**
 * Mount the same uPlot used in OTLPMetricsSeriesModal. Returns a dispose function.
 * @param options.chartType `'line'` | `'area'` | `'histogram'` — histogram uses uPlot bar paths.
 * @param options.showLegend defaults to true (series labels below the chart).
 */
export function mountOtlpMetricSeriesChart(el, chartPayload, options) {
  const chartType = options?.chartType ?? 'area'
  const showLegend = options?.showLegend !== false

  const legendReserve = showLegend ? 34 : 0
  const width = Math.max(280, el.clientWidth || 400)
  const height = Math.max(120, (el.clientHeight || 200) - legendReserve)

  const isHistogram = chartType === 'histogram'
  const isArea = chartType === 'area'
  const barsPath = uPlot.paths?.bars?.({ size: [0.7, 100] })
  const linePath = uPlot.paths?.linear?.()
  const pathFn = isHistogram ? barsPath : linePath || undefined

  const cs = getComputedStyle(el)
  const axisStroke = cs.getPropertyValue('--muted-foreground').trim() || 'rgba(148,163,184,0.5)'
  const gridStroke = cs.getPropertyValue('--border').trim() || 'rgba(255,255,255,0.08)'
  const labelColor = cs.getPropertyValue('--muted-foreground').trim() || 'rgba(148,163,184,0.75)'

  const spanSec =
    chartPayload.xs.length >= 2
      ? chartPayload.xs[chartPayload.xs.length - 1] - chartPayload.xs[0]
      : 0
  const narrowTimeAxis = spanSec > 0 && spanSec < 120

  const xAxis = {
    stroke: axisStroke,
    grid: { stroke: gridStroke, width: 1 },
    ticks: { stroke: gridStroke, width: 1 },
    font: '10px Inter, system-ui, sans-serif',
    values: (_u, vals) =>
      vals.map((v) => {
        const d = new Date(v * 1000)
        if (narrowTimeAxis) {
          return `${pad2(d.getHours())}:${pad2(d.getMinutes())}:${pad2(d.getSeconds())}`
        }
        return `${pad2(d.getHours())}:${pad2(d.getMinutes())}`
      }),
    labelFont: '10px Inter, system-ui, sans-serif',
    labelColor,
    label: 'Time',
  }

  const yAxisLeft = {
    stroke: axisStroke,
    grid: { stroke: gridStroke, width: 1 },
    ticks: { stroke: gridStroke, width: 1 },
    font: '10px Inter, system-ui, sans-serif',
    labelFont: '10px Inter, system-ui, sans-serif',
    labelColor,
    size: 52,
    label: chartPayload.dual ? 'Sum (value_double)' : 'Value',
  }

  const few = chartPayload.xs.length < 10

  const fillSky = 'rgba(56, 189, 248, 0.14)'
  const fillOrange = 'rgba(249, 115, 22, 0.14)'

  let plot
  if (chartPayload.dual) {
    const yAxisRight = {
      scale: 'y1',
      stroke: axisStroke,
      side: 3,
      grid: { show: false },
      ticks: { stroke: gridStroke, width: 1 },
      font: '10px Inter, system-ui, sans-serif',
      labelFont: '10px Inter, system-ui, sans-serif',
      labelColor,
      size: 52,
      label: 'Count (value_int)',
    }
    const opts = {
      width,
      height,
      cursor: { show: true, drag: { x: false, y: false } },
      select: { show: false },
      legend: { show: showLegend, live: false },
      scales: {
        x: { time: true },
        y: { auto: true },
        y1: { auto: true, grid: { show: false } },
      },
      axes: [xAxis, yAxisLeft, yAxisRight],
      series: [
        {},
        {
          label: 'Sum',
          stroke: 'rgb(56, 189, 248)',
          fill: !isHistogram && isArea ? fillSky : undefined,
          width: 2,
          scale: 'y',
          paths: pathFn,
          points: { show: few && !isHistogram, size: 5 },
        },
        {
          label: 'Count',
          stroke: 'rgb(249, 115, 22)',
          fill: !isHistogram && isArea ? fillOrange : undefined,
          width: 2,
          scale: 'y1',
          paths: pathFn,
          points: { show: few && !isHistogram, size: 5 },
        },
      ],
    }
    plot = new uPlot(opts, chartPayload.aligned, el)
  } else {
    const opts = {
      width,
      height,
      cursor: { show: true, drag: { x: false, y: false } },
      select: { show: false },
      legend: { show: showLegend, live: false },
      scales: { x: { time: true }, y: { auto: true } },
      axes: [xAxis, yAxisLeft],
      series: [
        {},
        {
          label: 'Value',
          stroke: 'rgb(56, 189, 248)',
          fill: !isHistogram && isArea ? fillSky : undefined,
          width: 2,
          paths: pathFn,
          points: { show: few && !isHistogram, size: 5 },
        },
      ],
    }
    plot = new uPlot(opts, chartPayload.aligned, el)
  }

  const ro = new ResizeObserver(() => {
    if (plot && el.clientWidth > 0 && el.clientHeight > 0) {
      plot.setSize({
        width: el.clientWidth,
        height: Math.max(120, el.clientHeight - legendReserve),
      })
    }
  })
  ro.observe(el)

  return () => {
    ro.disconnect()
    plot.destroy()
  }
}
