import { describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import ProfilePanel from './ProfilePanel'
import { DetailsTabProvider } from '@/components/details-tab/details-tab-provider'

vi.mock('@/components/locale/locale-provider', () => ({
  useLocale: () => ({
    locale: 'en-001',
    setLocale: vi.fn(),
    resolved: 'en-001',
    auto: 'en-US',
  }),
}))

describe('ProfilePanel locale selector', () => {
  it('keeps an unknown stored locale selectable', () => {
    render(
      <DetailsTabProvider>
        <ProfilePanel me={null} />
      </DetailsTabProvider>,
    )
    expect(screen.getByRole('combobox', { name: 'Locale' })).toHaveTextContent('· en-001')
  })
})
