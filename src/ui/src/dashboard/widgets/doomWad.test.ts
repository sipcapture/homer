import { describe, expect, it, vi } from 'vitest'
import {
  DOOM_WAD_URL,
  checkWadAvailable,
  isWadMagic,
  parseDoomFrameMessage,
} from './doomWad'

const enc = (s: string) => new TextEncoder().encode(s)

describe('isWadMagic', () => {
  it('accepts IWAD and PWAD headers', () => {
    expect(isWadMagic(enc('IWAD....'))).toBe(true)
    expect(isWadMagic(enc('PWAD....'))).toBe(true)
  })

  it('rejects everything else', () => {
    expect(isWadMagic(enc('WAD!'))).toBe(false)
    expect(isWadMagic(enc('<htm'))).toBe(false) // SPA fallback page, not a WAD
    expect(isWadMagic(enc('IW'))).toBe(false)
    expect(isWadMagic(new Uint8Array(0))).toBe(false)
  })
})

describe('checkWadAvailable', () => {
  const respond = (status: number, body = '') =>
    vi.fn(async () =>
      ({
        ok: status >= 200 && status < 300,
        status,
        arrayBuffer: async () => enc(body).buffer,
      }) as unknown as Response,
    )

  it('returns ok for a ranged response with WAD magic', async () => {
    const f = respond(206, 'IWAD')
    await expect(checkWadAvailable(f)).resolves.toBe('ok')
    expect(f).toHaveBeenCalledWith(DOOM_WAD_URL, { headers: { Range: 'bytes=0-3' } })
    await expect(checkWadAvailable(respond(200, 'PWAD....full-body'))).resolves.toBe('ok')
  })

  it('returns bad-wad when the payload is not a WAD (e.g. SPA fallback)', async () => {
    await expect(checkWadAvailable(respond(200, '<!do'))).resolves.toBe('bad-wad')
  })

  it('returns missing for 404', async () => {
    await expect(checkWadAvailable(respond(404))).resolves.toBe('missing')
  })

  it('returns error for 500 and network failures', async () => {
    await expect(checkWadAvailable(respond(500))).resolves.toBe('error')
    const boom = vi.fn(async () => {
      throw new Error('offline')
    })
    await expect(checkWadAvailable(boom as unknown as typeof fetch)).resolves.toBe('error')
  })
})

describe('parseDoomFrameMessage', () => {
  it('parses valid frames', () => {
    expect(parseDoomFrameMessage({ type: 'doom', state: 'running' })).toEqual({
      type: 'doom',
      state: 'running',
      code: undefined,
      detail: undefined,
      message: undefined,
    })
    expect(
      parseDoomFrameMessage({ type: 'doom', state: 'error', code: 'wad-missing' })?.code,
    ).toBe('wad-missing')
    expect(
      parseDoomFrameMessage({ type: 'doom', state: 'engine', message: 'doom: 10, game started' })
        ?.message,
    ).toBe('doom: 10, game started')
  })

  it('rejects foreign or malformed payloads', () => {
    expect(parseDoomFrameMessage(null)).toBeNull()
    expect(parseDoomFrameMessage('doom')).toBeNull()
    expect(parseDoomFrameMessage({ type: 'chess', state: 'running' })).toBeNull()
    expect(parseDoomFrameMessage({ type: 'doom', state: 'exploded' })).toBeNull()
    expect(parseDoomFrameMessage({ type: 'doom' })).toBeNull()
  })
})
