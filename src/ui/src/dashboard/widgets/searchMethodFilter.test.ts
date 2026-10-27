import { describe, expect, it } from 'vitest'
import { assignLegacyMethodFilter } from './searchMethodFilter'

function apply(method: unknown): Record<string, unknown> {
  const filter: Record<string, unknown> = { proto_type: 1, event_type: 'call' }
  assignLegacyMethodFilter(filter, method)
  return filter
}

describe('assignLegacyMethodFilter', () => {
  it('omits an empty method array so the API does not receive "method": []', () => {
    expect(apply([])).toEqual({ proto_type: 1, event_type: 'call' })
  })

  it('sends a single selected method as method', () => {
    expect(apply(['INVITE'])).toEqual({
      proto_type: 1,
      event_type: 'call',
      method: 'INVITE',
    })
  })

  it('sends several methods as methods', () => {
    expect(apply(['INVITE', 'BYE'])).toEqual({
      proto_type: 1,
      event_type: 'call',
      methods: ['INVITE', 'BYE'],
    })
  })

  it('keeps a plain string method', () => {
    expect(apply('REGISTER')).toEqual({
      proto_type: 1,
      event_type: 'call',
      method: 'REGISTER',
    })
  })

  it('omits an empty string', () => {
    expect(apply('')).toEqual({ proto_type: 1, event_type: 'call' })
  })

  it('omits null and undefined', () => {
    expect(apply(null)).toEqual({ proto_type: 1, event_type: 'call' })
    expect(apply(undefined)).toEqual({ proto_type: 1, event_type: 'call' })
  })
})
