/**
 * DoomPanel — the real Doom (Chocolate Doom compiled to WebAssembly,
 * cloudflare/doom-wasm build) as a dashboard widget.
 *
 * Architecture notes:
 *   - The engine runs inside an <iframe src="/game/index.html">, never in
 *     the React tree: Emscripten/SDL2 builds pollute window globals and
 *     cannot be torn down — removing the iframe is the only reliable
 *     cleanup. The iframe also keeps keyboard capture scoped to the game.
 *   - The engine assets (websockets-doom.{js,wasm}) ship with the UI
 *     bundle, but the IWAD does not: it is served from disk via the
 *     coordinator's /gamedata/ route (`gamedata_dir` config key) so it is
 *     never embedded into the homer-core binary. When the WAD is absent
 *     the widget shows provisioning instructions instead of starting.
 *   - Widget <-> iframe communication is same-origin postMessage; frames
 *     are validated by parseDoomFrameMessage (doomWad.ts).
 */

import { useCallback, useEffect, useRef, useState } from 'react'
import { Button } from '@/components/ui/button'
import {
  DOOM_FRAME_URL,
  checkWadAvailable,
  parseDoomFrameMessage,
  type WadAvailability,
} from './doomWad'

type Phase = 'idle' | 'checking' | 'wad-missing' | 'wad-error' | 'booting' | 'running' | 'crashed'

export default function DoomPanel() {
  const [phase, setPhase] = useState<Phase>('idle')
  const [errorDetail, setErrorDetail] = useState('')
  const iframeRef = useRef<HTMLIFrameElement | null>(null)

  // Listen for state frames from the game iframe.
  useEffect(() => {
    const onMessage = (ev: MessageEvent) => {
      if (ev.origin !== window.location.origin) return
      if (iframeRef.current && ev.source !== iframeRef.current.contentWindow) return
      const msg = parseDoomFrameMessage(ev.data)
      if (!msg) return
      if (msg.state === 'running') setPhase('running')
      if (msg.state === 'error') {
        if (msg.code === 'wad-missing') {
          setPhase('wad-missing')
        } else {
          setErrorDetail(msg.code ? `${msg.code}${msg.detail ? `: ${msg.detail}` : ''}` : '')
          setPhase('crashed')
        }
      }
    }
    window.addEventListener('message', onMessage)
    return () => window.removeEventListener('message', onMessage)
  }, [])

  const start = useCallback(async () => {
    setPhase('checking')
    setErrorDetail('')
    const avail: WadAvailability = await checkWadAvailable()
    if (avail === 'ok') {
      setPhase('booting') // mounts the iframe; host page takes over
    } else if (avail === 'missing') {
      setPhase('wad-missing')
    } else if (avail === 'bad-wad') {
      setErrorDetail('doom1.wad exists but is not a valid IWAD/PWAD file')
      setPhase('crashed')
    } else {
      setPhase('wad-error')
    }
  }, [])

  const stop = useCallback(() => {
    // Unmounting the iframe is the teardown: audio, RAF loop, wasm memory
    // all go away with the browsing context.
    setPhase('idle')
    setErrorDetail('')
  }, [])

  const sendCmd = useCallback((cmd: 'fullscreen' | 'focus') => {
    iframeRef.current?.contentWindow?.postMessage(
      { type: 'doom-cmd', cmd },
      window.location.origin,
    )
  }, [])

  const gameMounted = phase === 'booting' || phase === 'running'

  return (
    <div className="flex h-full flex-col">
      <div className="flex items-center gap-2 border-b px-2 py-1">
        <span className="text-xs font-semibold tracking-wide text-muted-foreground">DOOM</span>
        <div className="flex-1" />
        {gameMounted ? (
          <>
            <Button size="sm" variant="outline" className="h-6 px-2 text-xs" onClick={() => sendCmd('fullscreen')}>
              Fullscreen
            </Button>
            <Button size="sm" variant="outline" className="h-6 px-2 text-xs" onClick={stop}>
              Stop
            </Button>
          </>
        ) : (
          <Button
            size="sm"
            className="h-6 px-2 text-xs"
            onClick={start}
            disabled={phase === 'checking'}
          >
            {phase === 'checking' ? 'Checking…' : 'Start'}
          </Button>
        )}
      </div>

      <div className="relative min-h-0 flex-1 bg-black">
        {gameMounted && (
          <iframe
            ref={iframeRef}
            src={DOOM_FRAME_URL}
            title="Doom"
            className="h-full w-full border-0"
            // Same-origin so the host page can fetch /gamedata; scripts for
            // the engine itself. No top-navigation, no popups.
            sandbox="allow-scripts allow-same-origin allow-pointer-lock"
            allow="fullscreen; autoplay"
            onMouseEnter={() => sendCmd('focus')}
          />
        )}

        {phase === 'idle' && (
          <Overlay>
            <p className="text-sm text-red-500">RIP AND TEAR</p>
            <p className="max-w-72 text-center text-xs text-muted-foreground">
              Chocolate Doom compiled to WebAssembly. Press Start, click the
              screen for keyboard focus, ESC opens the in-game menu.
            </p>
          </Overlay>
        )}

        {phase === 'wad-missing' && (
          <Overlay>
            <p className="text-sm text-red-500">WAD NOT FOUND</p>
            <p className="max-w-80 text-center text-xs text-muted-foreground">
              The IWAD is served from disk and is not part of homer-core. On
              the coordinator host run:
            </p>
            <code className="rounded bg-muted px-2 py-1 text-[11px]">
              ./scripts/fetch-doom-wad.sh /path/to/gamedata
            </code>
            <p className="max-w-80 text-center text-xs text-muted-foreground">
              and set <code className="text-[11px]">coordinator.http_server.gamedata_dir</code> to
              that directory, then restart homer-core.
            </p>
            <Button size="sm" variant="outline" className="h-6 px-2 text-xs" onClick={start}>
              Retry
            </Button>
          </Overlay>
        )}

        {phase === 'wad-error' && (
          <Overlay>
            <p className="text-sm text-red-500">GAMEDATA UNREACHABLE</p>
            <p className="max-w-72 text-center text-xs text-muted-foreground">
              The /gamedata/ route did not answer. Is the coordinator up?
            </p>
            <Button size="sm" variant="outline" className="h-6 px-2 text-xs" onClick={start}>
              Retry
            </Button>
          </Overlay>
        )}

        {phase === 'crashed' && (
          <Overlay>
            <p className="text-sm text-red-500">ENGINE ERROR</p>
            {errorDetail && (
              <code className="max-w-80 rounded bg-muted px-2 py-1 text-center text-[11px]">
                {errorDetail}
              </code>
            )}
            <Button size="sm" variant="outline" className="h-6 px-2 text-xs" onClick={stop}>
              Back
            </Button>
          </Overlay>
        )}
      </div>
    </div>
  )
}

function Overlay({ children }: { children: React.ReactNode }) {
  return (
    <div className="absolute inset-0 z-10 flex flex-col items-center justify-center gap-3 bg-black p-4">
      {children}
    </div>
  )
}
