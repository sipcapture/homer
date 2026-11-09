import { describe, expect, it, vi } from 'vitest'
import { useState } from 'react'
import { fireEvent, render, screen, within } from '@testing-library/react'
import ClockPanel, { type ClockPanelConfig } from './ClockPanel'

vi.mock('@/components/locale/locale-provider', () => ({
  useLocale: () => ({ locale: 'en-US', setLocale: vi.fn(), resolved: 'en-US', auto: 'en-US' }),
}))

let saved: ClockPanelConfig = {}

function Harness({ initial }: { initial: string[] }) {
  const [config, setConfig] = useState<ClockPanelConfig>({ timeZones: initial })
  return (
    <ClockPanel
      config={config}
      onConfigChange={(next) => {
        saved = next
        setConfig(next)
      }}
    />
  )
}

function openSettings(initial: string[]) {
  saved = {}
  render(<Harness initial={initial} />)
  fireEvent.click(screen.getByTitle('Time zones'))
  const dialog = screen.getByRole('dialog')
  const search = within(dialog).getByLabelText('Add time zone')
  return { dialog, search }
}

describe('ClockPanel time zone settings', () => {
  it('does not add a second clock for a different-case name', () => {
    const { dialog, search } = openSettings(['Europe/Berlin'])
    fireEvent.change(search, { target: { value: 'europe/berlin' } })
    expect(within(dialog).queryByText(/Add "/)).toBeNull()
    expect(within(dialog).getByText('Europe/Berlin is already added')).toBeTruthy()
    fireEvent.keyDown(search, { key: 'Enter' })
    expect(saved.timeZones ?? ['Europe/Berlin']).toEqual(['Europe/Berlin'])
  })

  it('treats a legacy name as the zone already added', () => {
    const { dialog, search } = openSettings(['Asia/Kolkata'])
    fireEvent.change(search, { target: { value: 'Asia/Calcutta' } })
    expect(within(dialog).queryByText(/Add "/)).toBeNull()
    expect(within(dialog).queryByRole('button', { name: 'Asia/Kolkata' })).toBeNull()
    fireEvent.keyDown(search, { key: 'Enter' })
    expect(saved.timeZones ?? ['Asia/Kolkata']).toEqual(['Asia/Kolkata'])
  })

  it('stores the canonical name when adding a mis-cased or legacy zone', () => {
    const { search } = openSettings(['local'])
    fireEvent.change(search, { target: { value: 'asia/calcutta' } })
    fireEvent.keyDown(search, { key: 'Enter' })
    expect(saved.timeZones).toEqual(['local', 'Asia/Kolkata'])
  })

  it('collapses saved duplicates (legacy or mis-cased) into one clock', () => {
    openSettings(['Asia/Calcutta', 'Asia/Kolkata', 'europe/berlin', 'Europe/Berlin'])
    const dialog = screen.getByRole('dialog')
    expect(within(dialog).getAllByRole('button', { name: /^Remove / }).map((b) => b.getAttribute('aria-label'))).toEqual([
      'Remove Asia/Kolkata',
      'Remove Europe/Berlin',
    ])
  })
})
