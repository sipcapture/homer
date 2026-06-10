/**
 * doomWad — small testable core for the Doom widget (DoomPanel.tsx).
 *
 * The IWAD is intentionally NOT part of the UI bundle: everything under
 * `public/` is embedded into the homer-core binary via `go:embed`, so the
 * WAD lives on disk and is served by the coordinator's `/gamedata/` route
 * (config key `gamedata_dir`, see scripts/fetch-doom-wad.sh). This module
 * only checks availability and parses iframe postMessage frames; the
 * actual download into MEMFS happens inside `public/game/index.html`.
 */

export const DOOM_WAD_URL = '/gamedata/doom1.wad'
export const DOOM_FRAME_URL = '/game/index.html'

export type WadAvailability = 'ok' | 'missing' | 'bad-wad' | 'error'

/**
 * Probe the WAD before mounting the engine iframe. Uses a ranged GET for
 * the first 4 bytes (echo.Static rejects HEAD with 405, but supports
 * Range), which doubles as a magic-bytes sanity check — an SPA fallback
 * page or a stray file in gamedata_dir is caught here, not by the engine.
 */
export async function checkWadAvailable(
  fetchImpl: typeof fetch = fetch,
): Promise<WadAvailability> {
  try {
    const resp = await fetchImpl(DOOM_WAD_URL, {
      headers: { Range: 'bytes=0-3' },
    })
    if (resp.status === 404) return 'missing'
    if (!resp.ok && resp.status !== 206) return 'error'
    const head = new Uint8Array(await resp.arrayBuffer())
    return isWadMagic(head.subarray(0, 4)) ? 'ok' : 'bad-wad'
  } catch {
    return 'error'
  }
}

/** Doom WAD files start with "IWAD" (game data) or "PWAD" (patch). */
export function isWadMagic(bytes: Uint8Array): boolean {
  if (bytes.length < 4) return false
  const magic = String.fromCharCode(bytes[0], bytes[1], bytes[2], bytes[3])
  return magic === 'IWAD' || magic === 'PWAD'
}

/** State frames posted by public/game/index.html to the parent widget. */
export interface DoomFrameMessage {
  type: 'doom'
  state: 'loading' | 'running' | 'error' | 'engine'
  /** Error code when state === 'error' (e.g. 'wad-missing', 'bad-wad'). */
  code?: string
  detail?: string
  /** Raw engine stdout line when state === 'engine' ("doom: 10, game started"). */
  message?: string
}

const FRAME_STATES = new Set(['loading', 'running', 'error', 'engine'])

/** Validate an untrusted postMessage payload from the game iframe. */
export function parseDoomFrameMessage(data: unknown): DoomFrameMessage | null {
  if (typeof data !== 'object' || data === null) return null
  const msg = data as Record<string, unknown>
  if (msg.type !== 'doom') return null
  if (typeof msg.state !== 'string' || !FRAME_STATES.has(msg.state)) return null
  return {
    type: 'doom',
    state: msg.state as DoomFrameMessage['state'],
    code: typeof msg.code === 'string' ? msg.code : undefined,
    detail: typeof msg.detail === 'string' ? msg.detail : undefined,
    message: typeof msg.message === 'string' ? msg.message : undefined,
  }
}
