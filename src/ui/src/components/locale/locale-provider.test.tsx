import { describe, expect, it } from 'vitest'
import { render } from '@testing-library/react'
import { useLocale } from './locale-provider'

function LocaleConsumer() {
  useLocale()
  return null
}

describe('useLocale', () => {
  it('throws when used outside LocaleProvider', () => {
    expect(() => render(<LocaleConsumer />)).toThrow('useLocale must be used within a LocaleProvider')
  })
})
