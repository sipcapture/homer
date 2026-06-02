import { describe, expect, it } from 'vitest'
import { withTimeZone } from './datetime'

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
