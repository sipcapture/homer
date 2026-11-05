import { render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { apiPost } from '../api'
import MessageModal from './MessageModal'

vi.mock('../api', () => ({
  apiPost: vi.fn(),
}))

vi.mock('@/components/locale/locale-provider', () => ({
  useLocale: () => ({ resolved: 'en' }),
}))

describe('MessageModal ISUP decode', () => {
  beforeEach(() => {
    vi.mocked(apiPost).mockReset()
  })

  it('shows the ISUP summary for an application/isup payload', async () => {
    vi.mocked(apiPost).mockResolvedValue({
      data: {
        isup: {
          messages: [{ summary: 'IAM — Initial address\nCalled party number: 632107608' }],
        },
      },
    })

    render(
      <MessageModal
        modal={{
          uuid: 'u1',
          loading: false,
          data: {
            payload:
              'INVITE sip:a@b SIP/2.0\r\nContent-Type: application/isup;version=itu-t92+\r\n\r\n00',
            method: 'INVITE',
          },
          messageContext: { proto_type: 1, event_type: 'call' },
        }}
        onClose={() => {}}
        timeZone="UTC"
      />,
    )

    expect(await screen.findByText(/632107608/)).toBeTruthy()
    expect(apiPost).toHaveBeenCalledWith(
      '/messages/decoded',
      expect.objectContaining({ uuid: 'u1', proto_type: 1, event_type: 'call' }),
    )
  })
})
