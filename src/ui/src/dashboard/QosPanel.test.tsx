import { describe, expect, it, vi } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import QosPanel from './QosPanel'

const uplot = vi.hoisted(() => ({ charts: [] as { opts: { series: { label: string }[] }; data: (number | null)[][] }[] }))

vi.mock('uplot', () => {
  class FakeUPlot {
    static paths = { linear: () => () => {}, bars: () => () => {} }
    constructor(opts: { series: { label: string }[] }, data: (number | null)[][]) {
      uplot.charts.push({ opts, data })
    }
    setSize() {}
    destroy() {}
  }
  return { default: FakeUPlot }
})

vi.mock('@/components/locale/locale-provider', () => ({
  useLocale: () => ({
    locale: 'en-001',
    setLocale: vi.fn(),
    resolved: 'en-001',
    auto: 'en-US',
  }),
}))

const item = (payload: unknown, ts = '2026-09-18T20:42:29.679777Z') => ({
  src_ip: '192.0.2.10',
  dst_ip: '192.0.2.20',
  src_port: 10185,
  dst_port: 10013,
  timestamp: ts,
  payload: JSON.stringify(payload),
})

const receiverReport = {
  ssrc: 846123712,
  type: 201,
  report_count: 1,
  sender_information: null,
  report_blocks: [
    { source_ssrc: 846123712, fraction_lost: 64, packets_lost: 47, highest_seq_no: 432, ia_jitter: 31, lsr: 0, dlsr: 0 },
  ],
}

const senderReport = {
  ssrc: 2461583827,
  type: 200,
  report_count: 1,
  sender_information: { packets: 198, octets: 31680, ntp_timestamp_sec: 3998723254, rtp_timestamp: 32000 },
  report_blocks: [
    { source_ssrc: 537159449, fraction_lost: 0, packets_lost: 0, highest_seq_no: 432, ia_jitter: 0, lsr: 0, dlsr: 60292 },
  ],
}

const qosData = (rtcp: unknown[]) => ({ rtcp: { data: rtcp }, rtp: { data: [] }, vqrtcp: { data: [] } })

const statValue = (label: string) => screen.getByText(label).parentElement?.lastElementChild?.textContent

const seriesFor = (metric: string) => {
  const chart = uplot.charts.at(-1)
  const index = chart?.opts.series.findIndex(serie => serie.label === metric) ?? -1
  return index < 0 ? undefined : chart?.data[index]
}

describe('QosPanel RTCP tab', () => {
  it('charts a session whose packets are all Receiver Reports', () => {
    render(<QosPanel qosData={qosData([item(receiverReport)])} timeZone="UTC" />)

    expect(screen.getByRole('tab', { name: /RTCP \(1\)/ })).toBeInTheDocument()
    expect(screen.queryByText(/No QoS data available/)).not.toBeInTheDocument()
    expect(statValue('MAX IA_JITTER')).toBe('31')
  })

  it('leaves sent packets and octets empty for a Receiver Report', () => {
    render(<QosPanel qosData={qosData([item(receiverReport)])} timeZone="UTC" />)

    expect(statValue('MAX PACKETS')).toBe('0')
    expect(statValue('MAX OCTETS')).toBe('0')
  })

  it('counts Receiver Report jitter in a session mixing both report types', () => {
    render(<QosPanel qosData={qosData([item(senderReport), item(receiverReport)])} timeZone="UTC" />)

    expect(screen.getByRole('tab', { name: /RTCP \(2\)/ })).toBeInTheDocument()
    expect(statValue('MAX IA_JITTER')).toBe('31')
    expect(statValue('MAX PACKETS_LOST')).toBe('47')
  })

  it('charts a session whose packets are all Sender Reports', () => {
    render(<QosPanel qosData={qosData([item(senderReport)])} timeZone="UTC" />)

    expect(screen.getByRole('tab', { name: /RTCP \(1\)/ })).toBeInTheDocument()
  })

  it('reports no data for a Sender Report that sent no packets', () => {
    const empty = { ...senderReport, sender_information: { packets: 0, octets: 0 } }
    render(<QosPanel qosData={qosData([item(empty)])} timeZone="UTC" />)

    expect(screen.getByText(/No QoS data available/)).toBeInTheDocument()
  })

  it('gaps the sent packet series on a Receiver Report sample', async () => {
    Object.defineProperty(HTMLElement.prototype, 'clientWidth', { configurable: true, value: 800 })
    uplot.charts.length = 0

    render(<QosPanel qosData={qosData([item(senderReport), item(receiverReport)])} timeZone="UTC" />)
    await waitFor(() => expect(uplot.charts.length).toBeGreaterThan(0))

    expect(seriesFor('packets')).toEqual([198, null])
    expect(seriesFor('octets')).toEqual([31680, null])
    expect(seriesFor('ia_jitter')).toEqual([0, 31])

    Object.defineProperty(HTMLElement.prototype, 'clientWidth', { configurable: true, value: 0 })
  })

  it('reports no data for a charted non-RR type that carries no sender information', () => {
    const sdes = { ssrc: 846123712, type: 202, report_count: 0, report_blocks: [] }
    render(<QosPanel qosData={qosData([item(sdes)])} timeZone="UTC" />)

    expect(screen.getByText(/No QoS data available/)).toBeInTheDocument()
  })

  it('reports no data for an RTCP type it does not chart', () => {
    render(<QosPanel qosData={qosData([item({ ...receiverReport, type: 203 })])} timeZone="UTC" />)

    expect(screen.getByText(/No QoS data available/)).toBeInTheDocument()
  })
})
