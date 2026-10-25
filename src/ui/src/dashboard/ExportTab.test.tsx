import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import ExportTab from './ExportTab'
import { EXPORT_CORRELATE_LS_KEY } from './exportTabPrefs'

vi.mock('./context/DashboardContext', () => ({
  useDashboard: () => ({ timeZone: 'UTC' }),
}))

vi.mock('./useExportExclusionHosts', () => ({
  useExportExclusionHosts: () => ({
    options: [],
    selectedIPs: [],
    setSelectedIPs: vi.fn(),
    loading: false,
  }),
}))

const fetchMock = vi.fn()

function renderTab() {
  return render(<ExportTab sessionId="b1@host" items={[]} protoType={1} eventType="call" />)
}

/** Click a card's Download button and return the JSON body sent to the export endpoint. */
async function downloadAndGetBody(cardIndex: number): Promise<Record<string, unknown>> {
  fetchMock.mockClear()
  fireEvent.click(screen.getAllByRole('button', { name: /download/i })[cardIndex])
  await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1))
  return JSON.parse(fetchMock.mock.calls[0][1].body as string)
}

describe('ExportTab correlated legs option', () => {
  beforeEach(() => {
    localStorage.clear()
    fetchMock.mockResolvedValue({ ok: true, blob: async () => new Blob(['x']) })
    vi.stubGlobal('fetch', fetchMock)
    URL.createObjectURL = vi.fn(() => 'blob:test')
    URL.revokeObjectURL = vi.fn()
    vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => {})
  })

  afterEach(() => {
    vi.unstubAllGlobals()
    vi.restoreAllMocks()
  })

  it('is unchecked by default and does not send correlate', async () => {
    renderTab()
    expect(screen.getByRole('checkbox', { name: /include correlated legs/i })).not.toBeChecked()

    const textBody = await downloadAndGetBody(0)
    expect(textBody.session_id).toBe('b1@host')
    expect(textBody).not.toHaveProperty('correlate')
  })

  it('sends correlate=true to text and pcap export once checked', async () => {
    renderTab()
    fireEvent.click(screen.getByRole('checkbox', { name: /include correlated legs/i }))

    expect(fetchMock).not.toHaveBeenCalled()
    const textBody = await downloadAndGetBody(0)
    expect(fetchMock.mock.calls[0][0]).toBe('/api/v4/transactions/export/text')
    expect(textBody.correlate).toBe(true)

    const pcapBody = await downloadAndGetBody(1)
    expect(fetchMock.mock.calls[0][0]).toBe('/api/v4/transactions/export/pcap')
    expect(pcapBody.correlate).toBe(true)
  })

  it('persists the choice in local storage', () => {
    renderTab()
    const box = screen.getByRole('checkbox', { name: /include correlated legs/i })

    fireEvent.click(box)
    expect(localStorage.getItem(EXPORT_CORRELATE_LS_KEY)).toBe('1')

    fireEvent.click(box)
    expect(localStorage.getItem(EXPORT_CORRELATE_LS_KEY)).toBe('0')
  })

  it('restores a stored choice on mount', async () => {
    localStorage.setItem(EXPORT_CORRELATE_LS_KEY, '1')
    renderTab()

    expect(screen.getByRole('checkbox', { name: /include correlated legs/i })).toBeChecked()
    expect((await downloadAndGetBody(0)).correlate).toBe(true)
  })
})
