import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import MessageModal from './MessageModal'
import { apiPost } from '../api'

vi.mock('../api', () => ({ apiPost: vi.fn() }))
vi.mock('@/components/locale/locale-provider', () => ({
  useLocale: () => ({ resolved: 'en-US' }),
}))

const mockedApiPost = vi.mocked(apiPost)

const detached = [
  'INVITE sip:a@b SIP/2.0',
  'Content-Type: application/isup;version=itu-t92+',
  'Content-Length: 0',
  '',
  '',
].join('\r\n')

describe('MessageModal SIP-I body', () => {
  beforeEach(() => {
    mockedApiPost.mockReset()
    mockedApiPost.mockResolvedValue({ data: { isup: { messages: [{ summary: 'IAM — Initial address' }] } } })
  })

  it('shows a clickable binary body instead of the missing octets', async () => {
    render(
      <MessageModal
        modal={{ uuid: 'u1', loading: false, data: { payload: detached }, error: null }}
        onClose={() => {}}
        timeZone="UTC"
      />,
    )

    const body = await screen.findByRole('button', { name: 'binary body - click to decode' })
    expect(screen.getByText(/application\/isup/)).toBeTruthy()
    expect(document.body.textContent).not.toContain('01006000')

    fireEvent.click(body)
    await waitFor(() => {
      expect(mockedApiPost).toHaveBeenCalledWith(
        '/messages/decoded',
        expect.objectContaining({ uuid: 'u1' }),
      )
    })
    expect(await screen.findByText(/IAM — Initial address/)).toBeTruthy()
  })
})
