import { describe, expect, it } from 'vitest'
import { formatISUP } from './isupDisplay'

describe('formatISUP', () => {
  it('joins message summaries', () => {
    const text = formatISUP({
      messages: [
        { summary: 'IAM — Initial address\nCalled party number: 632107608' },
        { summary: 'REL — Release\nCause: 16 Normal call clearing' },
      ],
    })
    expect(text).toContain('632107608')
    expect(text).toContain('Normal call clearing')
    expect(text).toContain('\n\nREL')
  })

  it('returns a top-level error when there are no messages', () => {
    expect(formatISUP({ error: 'ANSI ISUP is not decoded' })).toBe('ANSI ISUP is not decoded')
  })

  it('returns empty text for a non-ISUP decode', () => {
    expect(formatISUP(null)).toBe('')
    expect(formatISUP({})).toBe('')
  })
})
