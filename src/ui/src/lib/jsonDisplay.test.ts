import { describe, expect, it } from 'vitest'
import {
  formatJsonField,
  highlightJSON,
  isJsonDisplayable,
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

  it('emits syntax-highlight markup with stable CSS classes', () => {
    const html = highlightJSON('{"level":"INFO","count":3,"ok":true,"x":null}')
    expect(html).toContain('json-hl-key')
    expect(html).toContain('json-hl-str')
    expect(html).toContain('json-hl-num')
    expect(html).toContain('json-hl-bool')
    expect(html).toContain('json-hl-null')
  })
})
