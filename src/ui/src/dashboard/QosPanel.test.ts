import { describe, expect, it } from 'vitest'
import { formatAxisTime } from './QosPanel'

const axisOptions = {
  hour: 'numeric',
  minute: 'numeric',
  second: 'numeric',
} satisfies Intl.DateTimeFormatOptions

describe('formatAxisTime', () => {
  it('uses the browser/runtime timezone when local is selected', () => {
    const date = new Date('2026-08-17T11:30:04Z')
    const unixSec = date.getTime() / 1000

    expect(formatAxisTime(unixSec, 'local', 'fr-FR')).toBe(
      new Intl.DateTimeFormat('fr-FR', axisOptions).format(date),
    )
  })

  it('uses an explicit timezone when one is selected', () => {
    const date = new Date('2026-08-17T11:30:04Z')
    const unixSec = date.getTime() / 1000

    expect(formatAxisTime(unixSec, 'UTC', 'fr-FR')).toBe(
      new Intl.DateTimeFormat('fr-FR', { ...axisOptions, timeZone: 'UTC' }).format(date),
    )
  })
})
