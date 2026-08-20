import { describe, expect, it } from 'vitest'
import {
  formatJsonField,
  highlightJSON,
  isJsonDisplayable,
  parseJsonDeep,
  rowWithoutEventPayload,
  serializeRowForDisplay,
} from './jsonDisplay'

describe('jsonDisplay', () => {
  it('expands JSON payload strings when serializing a row', () => {
    const row = {
      uuid: 'abc',
      payload: '{"level":"INFO","msg":"hello"}',
      session_id: 'x@host',
    }
    const out = serializeRowForDisplay(row)
    expect(out).toContain('"level": "INFO"')
    expect(out).not.toContain('\\"level\\"')
    expect(out).toContain('"payload": {')
  })

  it('pretty-prints a JSON payload string field', () => {
    const pretty = formatJsonField('{"a":1,"b":[2,3]}')
    expect(pretty).toBe(`{
  "a": 1,
  "b": [
    2,
    3
  ]
}`)
  })

  it('detects JSON objects and strings', () => {
    expect(isJsonDisplayable('{"x":1}')).toBe(true)
    expect(isJsonDisplayable({ x: 1 })).toBe(true)
    expect(isJsonDisplayable('plain text')).toBe(false)
  })

  it('HTML-escapes attacker-controlled JSON before highlight spans', () => {
    const html = highlightJSON({
      note: '<script>alert(1)</script>',
      img: '<img src=x onerror=alert(1)>',
    })
    expect(html).not.toMatch(/<script[\s>/]/i)
    expect(html).not.toMatch(/<img[\s>]/i)
    expect(html).toContain('&lt;script&gt;')
    expect(html).toContain('&lt;img')
    expect(html).toContain('json-hl-key')
  })

  it('emits syntax-highlight markup with stable CSS classes', () => {
    const html = highlightJSON('{"level":"INFO","count":3,"ok":true,"x":null}')
    expect(html).toContain('json-hl-key')
    expect(html).toContain('json-hl-str')
    expect(html).toContain('json-hl-num')
    expect(html).toContain('json-hl-bool')
    expect(html).toContain('json-hl-null')
  })

  it('parses double-encoded JSON strings from hlog()', () => {
    const inner = '{"level":"INFO","msg":"hello"}'
    const wrapped = JSON.stringify(inner)
    expect(parseJsonDeep(wrapped)).toEqual({ level: 'INFO', msg: 'hello' })
    expect(isJsonDisplayable(wrapped)).toBe(true)
    expect(formatJsonField(wrapped)).toContain('"level": "INFO"')
  })

  it('rowWithoutEventPayload omits only the populated payload column', () => {
    const row = { uuid: '1', payload: '{"x":1}', session_id: 'a@b' }
    const meta = rowWithoutEventPayload(row)
    expect(meta).toEqual({ uuid: '1', session_id: 'a@b' })
    expect(meta.payload).toBeUndefined()
  })

  it('rowWithoutEventPayload drops duplicate JSON payload columns', () => {
    const inner = { type: 'auth_request', authUser: '5000' }
    const row = {
      uuid: '1',
      payload: inner,
      message: JSON.stringify(inner),
      session_id: 'a@b',
    }
    const meta = rowWithoutEventPayload(row)
    expect(meta).toEqual({ uuid: '1', session_id: 'a@b' })
    expect(serializeRowForDisplay(meta)).not.toContain('authUser')
  })
})
