import { describe, expect, it } from 'vitest'
import { formatAxisTime, withTimeZone } from './datetime'

describe('withTimeZone', () => {
  it('strips pre-existing option timezone when local is requested', () => {
    expect(withTimeZone({ hour: '2-digit', timeZone: 'UTC' }, 'local')).toEqual({
      hour: '2-digit',
    })
  })

  it('overrides pre-existing option timezone when a specific timezone is requested', () => {
    expect(withTimeZone({ hour: '2-digit', timeZone: 'UTC' }, 'America/New_York')).toEqual({
      hour: '2-digit',
      timeZone: 'America/New_York',
    })
  })
})

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
