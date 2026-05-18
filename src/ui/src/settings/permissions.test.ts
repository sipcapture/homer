import { describe, expect, it } from 'vitest'
import { ROLE, canServerReset, canViewSection } from './permissions'

describe('permissions reset', () => {
  it('allows server reset for admin only', () => {
    expect(canServerReset(ROLE.ADMIN)).toBe(true)
    expect(canServerReset(ROLE.COMMON)).toBe(false)
    expect(canServerReset(ROLE.EXTERNAL)).toBe(false)
  })

  it('keeps reset page visible for common users', () => {
    expect(canViewSection(ROLE.COMMON, 'reset')).toBe(true)
  })
})
