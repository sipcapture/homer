import { describe, expect, it } from 'vitest'
import { sipViewParts } from './sipIDisplay'

const detached = [
  'INVITE sip:a@b SIP/2.0',
  'Content-Type: application/isup;version=itu-t92+',
  'Content-Length: 0',
  '',
  '',
].join('\r\n')

describe('sipViewParts', () => {
  it('replaces an empty application/isup body with a click marker', () => {
    const parts = sipViewParts(detached)
    expect(parts.map((part) => part.kind)).toEqual(['text', 'binary'])
    expect(parts[0]).toMatchObject({ kind: 'text' })
    if (parts[0].kind === 'text') {
      expect(parts[0].text).toContain('Content-Type: application/isup;version=itu-t92+')
      expect(parts[0].text.endsWith('\r\n\r\n')).toBe(true)
    }
  })

  it('hides a legacy hex body and the homer marker', () => {
    const legacy = [
      'INVITE sip:a@b SIP/2.0',
      'Content-Type: application/isup;version=itu-t92+',
      'X-Homer-Binary-Body: hex',
      'Content-Length: 8',
      '',
      '01006000',
    ].join('\r\n')
    const parts = sipViewParts(legacy)
    const text = parts.filter((part) => part.kind === 'text').map((part) => (part.kind === 'text' ? part.text : '')).join('')
    expect(parts.some((part) => part.kind === 'binary')).toBe(true)
    expect(text).not.toContain('01006000')
    expect(text).not.toContain('X-Homer-Binary-Body')
    expect(text).toContain('application/isup')
  })

  it('keeps SDP and marks only the empty ISUP part', () => {
    const multipart = [
      'INVITE sip:a@b SIP/2.0',
      'Content-Type: multipart/mixed;boundary=sipb',
      'Content-Length: 10',
      '',
      '--sipb',
      'Content-Type: application/sdp',
      '',
      'v=0',
      '',
      '--sipb',
      'Content-Type: application/isup;version=itu-t92+',
      'Content-Length: 0',
      '',
      '',
      '--sipb--',
    ].join('\r\n')
    const parts = sipViewParts(multipart)
    const text = parts.filter((part) => part.kind === 'text').map((part) => (part.kind === 'text' ? part.text : '')).join('')
    expect(parts.filter((part) => part.kind === 'binary')).toHaveLength(1)
    expect(text).toContain('v=0')
    expect(text).toContain('application/sdp')
    expect(text).toContain('--sipb--')
  })

  it('leaves ordinary SIP as one text part', () => {
    const sip = 'OPTIONS sip:a@b SIP/2.0\r\nContent-Length: 0\r\n\r\n'
    expect(sipViewParts(sip)).toEqual([{ kind: 'text', text: sip }])
  })
})
