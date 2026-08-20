import { describe, expect, it } from 'vitest'
import { highlightSIP } from './sipDisplay'

function assertEscaped(html: string) {
  expect(html).not.toMatch(/<script[\s>/]/i)
  expect(html).not.toMatch(/<img[\s>]/i)
  expect(html).not.toMatch(/<svg[\s>/]/i)
  expect(html).toContain('&lt;')
}

describe('highlightSIP', () => {
  it('escapes XSS in request-line and headers before wrapping spans', () => {
    const sip = [
      'INVITE sip:<script>alert(1)</script>@host SIP/2.0',
      'From: <sip:alice@host>;tag=<img src=x onerror=alert(1)>',
      'To: <sip:bob@host>',
      'Call-ID: <svg/onload=alert(1)>',
      'CSeq: 1 INVITE',
      'Via: SIP/2.0/UDP host',
      'X-Custom: <script>alert(1)</script>',
      '',
      '<script>alert(1)</script>',
    ].join('\n')
    const html = highlightSIP(sip)
    assertEscaped(html)
    expect(html).toContain('text-sky-500')
  })

  it('escapes XSS in SIP status line reason phrase', () => {
    const html = highlightSIP('SIP/2.0 403 <script>alert(1)</script>')
    assertEscaped(html)
    expect(html).toContain('text-destructive')
  })

  it('returns a placeholder for empty payload', () => {
    expect(highlightSIP('')).toBe('(no payload)')
    expect(highlightSIP(null)).toBe('(no payload)')
  })
})
