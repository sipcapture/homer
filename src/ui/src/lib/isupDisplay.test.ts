import { describe, expect, it } from 'vitest'
import { formatDecoded, formatISUP } from './isupDisplay'

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

  it('appends tshark JSON after the ISUP summary', () => {
    const text = formatDecoded({
      isup: { messages: [{ summary: 'IAM — Initial address' }] },
      shark: [{ _source: { layers: { 'isup.called': '632107608' } } }],
    })
    expect(text).toContain('IAM — Initial address')
    expect(text).toContain('"isup.called": "632107608"')
  })

  it('shows a tshark error without dropping the ISUP summary', () => {
    const text = formatDecoded({
      isup: { messages: [{ summary: 'REL — Release' }] },
      shark_error: 'tshark: not found',
    })
    expect(text).toContain('REL — Release')
    expect(text).toContain('tshark: not found')
  })

  it('returns empty text for a non-ISUP decode', () => {
    expect(formatISUP(null)).toBe('')
    expect(formatISUP({})).toBe('')
  })
})
