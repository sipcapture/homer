import { describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import ProfilePanel from './ProfilePanel'

vi.mock('@/components/locale/locale-provider', () => ({
  useLocale: () => ({
    locale: 'en',
    setLocale: vi.fn(),
    resolved: 'en',
    auto: 'en-US',
  }),
}))

describe('ProfilePanel locale selector', () => {
  it('keeps an unknown stored locale selectable', () => {
    render(<ProfilePanel me={null} />)
    expect(screen.getByRole('combobox')).toHaveTextContent('· en')
  })
})
