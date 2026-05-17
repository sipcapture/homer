import { lazy } from 'react'
import type React from 'react'

const SearchPanel = lazy(() => import('./SearchPanel.jsx'))
const SmartInputPanel = lazy(() => import('./SmartInputPanel.jsx'))
const ResultsPanel = lazy(() => import('./ResultsPanel.jsx'))
const ChartPanel = lazy(() => import('./ChartPanel.jsx'))
const DataChartPanel = lazy(() => import('./DataChartPanel.jsx'))
const ClockPanel = lazy(() => import('./ClockPanel.jsx'))
const IFramePanel = lazy(() => import('./IFramePanel.jsx'))
const GrafanaPanel = lazy(() => import('./GrafanaPanel.jsx'))
const NotePanel = lazy(() => import('./NotePanel.jsx'))
const AlertPanel = lazy(() => import('./AlertPanel.jsx'))
const PcapUploaderPanel = lazy(() => import('./PcapUploaderPanel.jsx'))
const CodeEditorPanel = lazy(() => import('./CodeEditorPanel.jsx'))
const PacketDefenderPanel = lazy(() => import('./PacketDefenderPanel.jsx'))
const SIPDialogMasterPanel = lazy(() => import('./SIPDialogMasterPanel.jsx'))
const JitterBufferHeroPanel = lazy(() => import('./JitterBufferHeroPanel.jsx'))
const SIPetrisPanel = lazy(() => import('./SIPetrisPanel'))
const NetrisPanel = lazy(() => import('./NetrisPanel'))
const ChessPanel = lazy(() => import('./ChessPanel'))
const NetChessPanel = lazy(() => import('./NetChessPanel'))

/**
 * Preset variant of a base widget type. Lets the AddWidgetDialog show a
 * dedicated card (label/icon/size) that drops in a widget of the SAME
 * `type` but with `defaultConfig` already populated. SIP and OTLP presets
 * for Protocol Search (see SearchPanel.tsx).
 */
export interface WidgetExtraEntry {
  /** Stable id used as React key in the picker grid. */
  id: string
  label: string
  icon: string
  /** Optional grid size override (defaults to the base WidgetMeta). */
  defaultW?: number
  defaultH?: number
  /** Seed merged into the new widget's `config` field on creation. */
  defaultConfig: Record<string, unknown>
}

interface WidgetMeta {
  component: React.LazyExoticComponent<React.ComponentType<any>>
  label: string
  icon: string
  category: string
  hidden?: boolean
  minW: number
  minH: number
  defaultW: number
  defaultH: number
  /** Optional preset variants surfaced as separate entries in the picker. */
  extras?: WidgetExtraEntry[]
}

export const widgetRegistry: Record<string, WidgetMeta> = {
  search: {
    component: SearchPanel,
    label: 'Protocol Search',
    icon: 'search',
    category: 'Search',
    minW: 2,
    minH: 4,
    defaultW: 3,
    defaultH: 8,
    extras: [
      {
        id: 'search-sip-call',
        label: 'SIP Call Search',
        icon: 'sip',
        defaultConfig: { preset: 'sip_call' },
      },
      {
        id: 'search-sip-registration',
        label: 'SIP Registration Search',
        icon: 'sip',
        defaultConfig: { preset: 'sip_registration' },
      },
      {
        id: 'search-otlp-traces',
        label: 'OTLP Trace Search',
        icon: 'otlp-trace',
        defaultConfig: { preset: 'otlp_traces' },
      },
      {
        id: 'search-otlp-metrics',
        label: 'OTLP Metric Search',
        icon: 'otlp-metric',
        defaultConfig: { preset: 'otlp_metrics' },
      },
      {
        id: 'search-otlp-logs',
        label: 'OTLP Log Search',
        icon: 'otlp-log',
        defaultConfig: { preset: 'otlp_logs' },
      },
    ],
  },
  /** Legacy type — same UI as `search`; hidden from Add Widget picker. */
  protosearch: {
    component: SearchPanel,
    label: 'Protocol Search',
    icon: 'search',
    category: 'Search',
    hidden: true,
    minW: 2,
    minH: 4,
    defaultW: 3,
    defaultH: 8,
  },
  smart_input: {
    component: SmartInputPanel,
    label: 'Smart Input',
    icon: 'code',
    category: 'Search',
    minW: 2,
    minH: 4,
    defaultW: 4,
    defaultH: 8,
  },
  results: {
    component: ResultsPanel,
    label: 'Results Table',
    icon: 'table',
    category: 'Visualize',
    minW: 4,
    minH: 4,
    defaultW: 9,
    defaultH: 14,
  },
  chart: {
    component: ChartPanel,
    label: 'Time Chart',
    icon: 'chart-line',
    category: 'Visualize',
    minW: 3,
    minH: 3,
    defaultW: 6,
    defaultH: 4,
  },
  data_chart: {
    component: DataChartPanel,
    label: 'Data Chart',
    icon: 'chart-line',
    category: 'Visualize',
    minW: 3,
    minH: 4,
    defaultW: 6,
    defaultH: 6,
  },
  // legacy alias — keep existing influx_chart widgets working; hidden from the Add Widget picker
  influx_chart: {
    component: DataChartPanel,
    label: 'Data Chart',
    icon: 'chart-line',
    category: 'Visualize',
    hidden: true,
    minW: 3,
    minH: 4,
    defaultW: 6,
    defaultH: 6,
  },
  clock: {
    component: ClockPanel,
    label: 'Clock',
    icon: 'clock',
    category: 'Utility',
    minW: 2,
    minH: 2,
    defaultW: 2,
    defaultH: 3,
  },
  iframe: {
    component: IFramePanel,
    label: 'IFrame / Grafana',
    icon: 'iframe',
    category: 'External',
    minW: 3,
    minH: 3,
    defaultW: 6,
    defaultH: 5,
  },
  grafana: {
    component: GrafanaPanel,
    label: 'Grafana',
    icon: 'iframe',
    category: 'External',
    minW: 4,
    minH: 3,
    defaultW: 6,
    defaultH: 5,
  },
  note: {
    component: NotePanel,
    label: 'Note',
    icon: 'note',
    category: 'Utility',
    minW: 2,
    minH: 2,
    defaultW: 3,
    defaultH: 3,
  },
  alert: {
    component: AlertPanel,
    label: 'Alert',
    icon: 'alert',
    category: 'Utility',
    minW: 2,
    minH: 2,
    defaultW: 2,
    defaultH: 3,
  },
  pcap_uploader: {
    component: PcapUploaderPanel,
    label: 'PCAP Uploader',
    icon: 'upload',
    category: 'Utility',
    minW: 3,
    minH: 3,
    defaultW: 4,
    defaultH: 4,
  },
  code_editor: {
    component: CodeEditorPanel,
    label: 'Code Editor',
    icon: 'code',
    category: 'Utility',
    minW: 3,
    minH: 3,
    defaultW: 6,
    defaultH: 5,
  },
  packet_defender: {
    component: PacketDefenderPanel,
    label: 'Packet Defender',
    icon: 'game',
    category: 'Games',
    minW: 4,
    minH: 6,
    defaultW: 6,
    defaultH: 10,
  },
  sip_dialog_master: {
    component: SIPDialogMasterPanel,
    label: 'SIP Dialog Master',
    icon: 'sip',
    category: 'Games',
    minW: 4,
    minH: 6,
    defaultW: 6,
    defaultH: 10,
  },
  jitter_buffer_hero: {
    component: JitterBufferHeroPanel,
    label: 'Jitter Buffer Hero',
    icon: 'buffer',
    category: 'Games',
    minW: 4,
    minH: 6,
    defaultW: 6,
    defaultH: 10,
  },
  sipetris: {
    component: SIPetrisPanel,
    label: 'SIPetris',
    icon: 'game',
    category: 'Games',
    minW: 5,
    minH: 8,
    defaultW: 7,
    defaultH: 12,
  },
  netris: {
    component: NetrisPanel,
    label: 'Netris',
    icon: 'game',
    category: 'Games',
    // The 10x20 arena + 200 px opponent sidebar fits comfortably in
    // ~8 grid columns. The previous 12-col default left a lot of
    // empty horizontal space inside the flex-1 wrapper because the
    // arena is `flex-shrink-0` and the cell clamp tops out at 48 px.
    minW: 5,
    minH: 8,
    defaultW: 8,
    defaultH: 12,
  },
  chess: {
    component: ChessPanel,
    label: 'Chess',
    icon: 'game',
    category: 'Games',
    minW: 6,
    minH: 8,
    defaultW: 8,
    defaultH: 12,
  },
  netchess: {
    component: NetChessPanel,
    label: 'NetChess',
    icon: 'game',
    category: 'Games',
    // Mirror the `chess` widget — the 220 px sidebar is identical and
    // the board itself is 8x8, so there is no good reason for the
    // NetChess board to render wider than the single-player one.
    minW: 6,
    minH: 8,
    defaultW: 8,
    defaultH: 12,
  },
}

export const widgetCategories = ['Search', 'Visualize', 'External', 'Utility', 'Games']

export function getWidgetMeta(type) {
  return widgetRegistry[type] || null
}
