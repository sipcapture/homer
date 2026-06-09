import { describe, expect, it } from 'vitest'
import { searchStoreKey, useSearchStore } from './search-store'

describe('searchStoreKey', () => {
  it('keys configured widgets by protocol and profile', () => {
    expect(
      searchStoreKey({
        protocol_id: { value: 1 },
        protocol_profile: 'call',
        fields: [{ id: 'call_id' }],
      }),
    ).toBe('proto:1:call')
  })

  it('is stable across dashboards/widgets for the same protocol', () => {
    const a = searchStoreKey({
      protocol_id: { value: 1 },
      protocol_profile: 'registration',
      fields: [{ id: 'aor' }],
    })
    const b = searchStoreKey({
      protocol_id: { value: 1 },
      protocol_profile: 'registration',
      fields: [{ id: 'call_id' }, { id: 'aor' }],
    })
    expect(a).toBe(b)
  })

  it('separates different protocols and profiles', () => {
    const sipCall = searchStoreKey({
      protocol_id: { value: 1 },
      protocol_profile: 'call',
      fields: [{ id: 'call_id' }],
    })
    const sipReg = searchStoreKey({
      protocol_id: { value: 1 },
      protocol_profile: 'registration',
      fields: [{ id: 'aor' }],
    })
    const otlp = searchStoreKey({
      protocol_id: { value: 200 },
      protocol_profile: 'default',
      fields: [{ id: 'trace_id' }],
    })
    expect(new Set([sipCall, sipReg, otlp]).size).toBe(3)
  })

  it('falls back to default profile when profile is empty', () => {
    expect(
      searchStoreKey({ protocol_id: { value: 5 }, fields: [{ id: 'x' }] }),
    ).toBe('proto:5:default')
  })

  it('uses the shared legacy key for unconfigured widgets', () => {
    expect(searchStoreKey(undefined)).toBe('legacy:sip')
    expect(searchStoreKey({})).toBe('legacy:sip')
    expect(searchStoreKey({ fields: [] })).toBe('legacy:sip')
    // protocol set but no fields yet (bootstrap pending) — still legacy
    expect(searchStoreKey({ protocol_id: { value: 1 } })).toBe('legacy:sip')
  })
})

describe('useSearchStore persistence behaviour', () => {
  it('persists fields under a protocol key and survives unrelated keys', () => {
    const { setField, getForm, clearForm } = useSearchStore.getState()
    const key = 'proto:1:call'

    setField(key, 'call_id', 'abc-123')
    setField(key, 'from_user', 'alice')
    setField('proto:1:registration', 'aor', 'bob')

    expect(getForm(key).form.call_id).toBe('abc-123')
    expect(getForm(key).form.from_user).toBe('alice')
    expect(getForm('proto:1:registration').form.aor).toBe('bob')

    clearForm(key)
    expect(getForm(key).form.call_id).toBe('')
    // other protocol untouched
    expect(getForm('proto:1:registration').form.aor).toBe('bob')
    clearForm('proto:1:registration')
  })
})
