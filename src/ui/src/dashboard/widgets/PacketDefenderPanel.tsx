/**
 * Packet Defender — mini-game: click bad packets, let good ones reach the SBC.
 */

import { useCallback, useEffect, useRef, useState } from 'react'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'
import { apiGet, openHepStream, type HepStreamClient, type HepStreamEvent } from '@/api'
import { buildExactAliasMapFromApiItems, resolveExactStreamIp } from '@/lib/ipAliasDisplay'
import { readBestScore, writeBestScoreIfHigher } from './gameScoreStorage'

interface PacketDef {
  label: string
  sub: string
  bad: boolean
  fill: string
  border: string
  textColor: string
  subColor: string
}

interface LivePacket {
  id: number
  def: PacketDef
  x: number
  y: number
  speed: number
  clicked: boolean
  state: 'alive' | 'blocked' | 'missed' | 'passed'
  flashLabel?: string
  flashSub?: string
}

interface GameState {
  running: boolean
  score: number
  mos: number
  wave: number
  combo: number
  blockedBad: number
  falsePos: number
  passedThru: number
  spawnInterval: number
  // pwCD holds the remaining cooldown in ms for each power-up. A
  // button is usable only when its entry is 0 (or missing). Single-use
  // semantics across the board: Rate Limit 18s, HEP Capture 20s,
  // Firewall 25s — the charges-based Firewall spam was removed in
  // favour of one press = one 25 s lockout.
  pwCD: Record<string, number>
  frozen: boolean
  freezeT: number
  hepActive: boolean
  hepT: number
}

const GOOD_PACKETS: PacketDef[] = [
  { label: 'INVITE', sub: 'sip:bob@pbx.local', bad: false, fill: '#e6f4ee', border: '#0f6e56', textColor: '#04342c', subColor: '#0f6e56' },
  { label: '200 OK', sub: 'to INVITE', bad: false, fill: '#eaf3de', border: '#3b6d11', textColor: '#173404', subColor: '#3b6d11' },
  { label: '180 Ringing', sub: 'provisional response', bad: false, fill: '#eaf3de', border: '#3b6d11', textColor: '#173404', subColor: '#3b6d11' },
  { label: 'ACK', sub: 'CSeq: 1 ACK', bad: false, fill: '#e6f4ee', border: '#0f6e56', textColor: '#04342c', subColor: '#0f6e56' },
  { label: 'BYE', sub: 'CSeq: 2 BYE', bad: false, fill: '#e6f4ee', border: '#0f6e56', textColor: '#04342c', subColor: '#0f6e56' },
  { label: '200 OK', sub: 'to BYE', bad: false, fill: '#eaf3de', border: '#3b6d11', textColor: '#173404', subColor: '#3b6d11' },
  { label: 'REGISTER', sub: 'Expires: 3600', bad: false, fill: '#eeedfe', border: '#534ab7', textColor: '#26215c', subColor: '#534ab7' },
  { label: 'OPTIONS', sub: 'keepalive ping', bad: false, fill: '#eeedfe', border: '#534ab7', textColor: '#26215c', subColor: '#534ab7' },
  { label: 'RTP', sub: 'G.711 PT=0 seq+1', bad: false, fill: '#e6f1fb', border: '#185fa5', textColor: '#042c53', subColor: '#185fa5' },
  { label: 'RTCP SR', sub: 'sender report', bad: false, fill: '#e6f1fb', border: '#185fa5', textColor: '#042c53', subColor: '#185fa5' },
]

const BAD_PACKETS: PacketDef[] = [
  { label: 'INVITE', sub: 'malformed Via header', bad: true, fill: '#fcebeb', border: '#a32d2d', textColor: '#501313', subColor: '#a32d2d' },
  { label: 'INVITE FLOOD', sub: '10k/sec · 1 src IP', bad: true, fill: '#fcebeb', border: '#a32d2d', textColor: '#501313', subColor: '#a32d2d' },
  { label: 'RTP', sub: 'wrong SSRC spoof', bad: true, fill: '#faeeda', border: '#854f0b', textColor: '#412402', subColor: '#854f0b' },
  { label: 'RTP', sub: 'seq jump +5000', bad: true, fill: '#faeeda', border: '#854f0b', textColor: '#412402', subColor: '#854f0b' },
  { label: 'OPTIONS', sub: 'fuzzed headers', bad: true, fill: '#fcebeb', border: '#a32d2d', textColor: '#501313', subColor: '#a32d2d' },
  { label: 'REGISTER', sub: 'brute-force auth', bad: true, fill: '#fcebeb', border: '#a32d2d', textColor: '#501313', subColor: '#a32d2d' },
  { label: 'SipVicious', sub: 'scanner UA string', bad: true, fill: '#fcebeb', border: '#a32d2d', textColor: '#501313', subColor: '#a32d2d' },
  { label: 'CANCEL', sub: 'wrong branch tag', bad: true, fill: '#faeeda', border: '#854f0b', textColor: '#412402', subColor: '#854f0b' },
  { label: '404 spoof', sub: 'no matching tx', bad: true, fill: '#faeeda', border: '#854f0b', textColor: '#412402', subColor: '#854f0b' },
  { label: 'RTP', sub: 'payload=gibberish', bad: true, fill: '#faeeda', border: '#854f0b', textColor: '#412402', subColor: '#854f0b' },
]

const SBC_H = 48
const PKT_W = 140
const GAME_ID = 'packet_defender'
const LIVE_RING_CAP = 40

let _uid = 0
const uid = () => ++_uid

// Map a real HEP event coming over the stream into a "good" game
// packet. Non-SIP events fall back to a plain RTP/RTCP label. We
// never mark a live event as bad -- the "bad" pool stays synthetic so
// the game keeps a predictable difficulty curve.
function livePacketFromEvent(evt: HepStreamEvent, aliasMap: Map<string, string> | null): PacketDef {
  if (evt.sip) {
    // Server marks responses with a non-empty resp_code ("200", "100",
    // …); requests leave it blank. Falling back to "SIP" keeps the
    // label from ever rendering undefined.
    const isResponse = !!evt.sip.resp_code
    const label = isResponse ? `${evt.sip.resp_code} ${evt.sip.resp_text ?? ''}`.trim() : evt.sip.method || 'SIP'
    const sub = [evt.sip.from_user, evt.sip.to_user].filter(Boolean).join(' → ') || evt.sip.callid || 'live'
    return {
      label,
      sub: sub.slice(0, 48),
      bad: false,
      fill: '#e6f4ee',
      border: '#0f6e56',
      textColor: '#04342c',
      subColor: '#0f6e56',
    }
  }
  const proto = evt.proto === 5 ? 'RTCP' : evt.proto === 6 ? 'RTP' : `HEP ${evt.proto}`
  return {
    label: proto,
    sub: `${resolveExactStreamIp(evt.src_ip, evt.src_port, aliasMap)} → ${resolveExactStreamIp(evt.dst_ip, evt.dst_port, aliasMap)}`.slice(0, 48),
    bad: false,
    fill: '#e6f1fb',
    border: '#185fa5',
    textColor: '#042c53',
    subColor: '#185fa5',
  }
}

export default function PacketDefenderPanel() {
  const [gs, setGs] = useState<GameState>({
    running: false,
    score: 0,
    mos: 4.4,
    wave: 1,
    combo: 1,
    blockedBad: 0,
    falsePos: 0,
    passedThru: 0,
    spawnInterval: 900,
    pwCD: { rl: 0, hep: 0, fw: 0 },
    frozen: false,
    freezeT: 0,
    hepActive: false,
    hepT: 0,
  })
  const [packets, setPackets] = useState<LivePacket[]>([])
  const [sbcFlash, setSbcFlash] = useState<'idle' | 'bad' | 'good'>('idle')
  const [banner, setBanner] = useState('')
  const [gameOver, setGameOver] = useState(false)
  const [paused, setPaused] = useState(false)
  const [arenaPx, setArenaPx] = useState({ w: 400, h: 320 })
  const [bestScore, setBestScore] = useState(0)
  const [liveTraffic, setLiveTraffic] = useState(false)
  const [liveState, setLiveState] = useState<'off' | 'connecting' | 'open' | 'closed'>('off')

  const liveRingRef = useRef<PacketDef[]>([])
  const liveClientRef = useRef<HepStreamClient | null>(null)
  /** Exact ip:port alias map for live stream labels (refreshed when Live traffic is on). */
  const streamAliasMapRef = useRef<Map<string, string> | null>(null)

  const gsRef = useRef(gs)
  const prevGameOverRef = useRef(false)
  const pausedRef = useRef(false)
  const packetsRef = useRef(packets)
  const arenaPxRef = useRef(arenaPx)
  const rafRef = useRef(0)
  const lastFRef = useRef(0)
  const spawnTRef = useRef(0)
  const waveTRef = useRef(0)
  const arenaElRef = useRef<HTMLDivElement | null>(null)

  gsRef.current = gs
  packetsRef.current = packets
  arenaPxRef.current = arenaPx
  pausedRef.current = paused

  useEffect(() => {
    if (!gs.running) setPaused(false)
  }, [gs.running])

  useEffect(() => {
    setBestScore(readBestScore(GAME_ID))
  }, [])

  useEffect(() => {
    if (gameOver && !prevGameOverRef.current) {
      setBestScore(writeBestScoreIfHigher(GAME_ID, gs.score))
    }
    prevGameOverRef.current = gameOver
  }, [gameOver, gs.score])

  useEffect(() => {
    const el = arenaElRef.current
    if (!el) return
    const ro = new ResizeObserver(() => {
      const r = el.getBoundingClientRect()
      const h = Math.max(200, Math.floor(r.height))
      const w = Math.max(200, Math.floor(r.width))
      setArenaPx({ w, h })
    })
    ro.observe(el)
    return () => ro.disconnect()
  }, [])

  useEffect(() => {
    if (gs.mos <= 1.0 && gs.running) {
      setGameOver(true)
      setGs((s) => ({ ...s, running: false }))
    }
  }, [gs.mos, gs.running])

  const showBanner = useCallback((msg: string) => {
    setBanner(msg)
    setTimeout(() => setBanner(''), 1800)
  }, [])

  const flashSBC = useCallback((bad: boolean) => {
    setSbcFlash(bad ? 'bad' : 'good')
    setTimeout(() => setSbcFlash('idle'), 420)
  }, [])

  useEffect(() => {
    if (!liveTraffic) {
      streamAliasMapRef.current = null
      return
    }
    let cancelled = false
    ;(async () => {
      try {
        const data = await apiGet('/aliases', { 'page[limit]': 5000 })
        const items = data?.data?.items ?? []
        if (cancelled) return
        streamAliasMapRef.current = buildExactAliasMapFromApiItems(items)
      } catch {
        streamAliasMapRef.current = null
      }
    })()
    return () => {
      cancelled = true
    }
  }, [liveTraffic])

  const spawnPacket = useCallback(() => {
    const g = gsRef.current
    const badRatio = Math.min(0.22 + g.wave * 0.045, 0.58)
    const isBad = Math.random() < badRatio
    let def: PacketDef
    if (isBad) {
      def = BAD_PACKETS[Math.floor(Math.random() * BAD_PACKETS.length)]!
    } else if (liveRingRef.current.length > 0) {
      // Prefer live traffic for "good" packets when the stream is
      // connected. shift() gives us FIFO so repeated spawns don't
      // keep drawing the same INVITE and we stay close to real-time.
      def = liveRingRef.current.shift()!
    } else {
      def = GOOD_PACKETS[Math.floor(Math.random() * GOOD_PACKETS.length)]!
    }
    const maxX = Math.max(8, arenaPxRef.current.w - PKT_W - 16)
    const x = Math.random() * maxX + 8
    const speed = 42 + g.wave * 9 + Math.random() * 28
    const pkt: LivePacket = { id: uid(), def, x, y: -62, speed, clicked: false, state: 'alive' }
    setPackets((prev) => [...prev, pkt])
  }, [])

  // Open / tear down the HEP stream subscription when the user flips
  // the Live traffic toggle. We subscribe to SIP only (proto=1) since
  // the game is built around INVITE/REGISTER-shaped packets. No
  // payload is requested -- metadata is enough and keeps the JWT
  // admin-gating requirement off the player.
  useEffect(() => {
    if (!liveTraffic) {
      liveClientRef.current?.close()
      liveClientRef.current = null
      liveRingRef.current = []
      setLiveState('off')
      return
    }
    setLiveState('connecting')
    const client = openHepStream(
      { proto: 1, history: 20 },
      {
        onOpen: () => setLiveState('open'),
        onClose: () => setLiveState('closed'),
        onError: () => {
          // surfaced via state; backoff is handled inside the client
        },
        onEvent: (evt) => {
          const def = livePacketFromEvent(evt, streamAliasMapRef.current)
          const ring = liveRingRef.current
          ring.push(def)
          if (ring.length > LIVE_RING_CAP) ring.splice(0, ring.length - LIVE_RING_CAP)
        },
      },
    )
    liveClientRef.current = client
    return () => {
      client.close()
      liveClientRef.current = null
      liveRingRef.current = []
    }
  }, [liveTraffic])

  const handleClick = useCallback((id: number) => {
    if (pausedRef.current) return
    setPackets((prev) => {
      const idx = prev.findIndex((p) => p.id === id)
      if (idx === -1 || prev[idx]!.clicked) return prev
      const pkt = prev[idx]!
      const next = [...prev]

      if (pkt.def.bad) {
        setGs((g) => {
          const combo = Math.min(g.combo + 1, 10)
          return { ...g, score: g.score + 10 * g.combo, combo, blockedBad: g.blockedBad + 1 }
        })
        next[idx] = {
          ...pkt,
          clicked: true,
          state: 'blocked',
          flashLabel: 'BLOCKED ✓',
          flashSub: `+${10 * gsRef.current.combo}`,
        }
      } else {
        setGs((g) => {
          const mos = Math.max(1.0, g.mos - 0.3)
          return { ...g, mos, combo: 1, falsePos: g.falsePos + 1 }
        })
        next[idx] = {
          ...pkt,
          clicked: true,
          state: 'missed',
          flashLabel: 'FALSE POS.',
          flashSub: 'MOS −0.3',
        }
      }

      setTimeout(() => setPackets((pp) => pp.filter((p) => p.id !== id)), 340)
      return next
    })
  }, [])

  const usePower = useCallback(
    (p: string) => {
      const g = gsRef.current
      if (!g.running) return
      if (pausedRef.current) return

      if (p === 'rl') {
        if (g.pwCD.rl > 0) return
        setGs((prev) => ({ ...prev, frozen: true, freezeT: 3000, pwCD: { ...prev.pwCD, rl: 18000 } }))
        showBanner('Rate Limit ON — traffic frozen 3s · 18s cooldown')
      } else if (p === 'hep') {
        if (g.pwCD.hep > 0) return
        setGs((prev) => ({ ...prev, hepActive: true, hepT: 5000, pwCD: { ...prev.pwCD, hep: 20000 } }))
        showBanner('HEP Capture — threats highlighted 5s · 20s cooldown')
      } else if (p === 'fw') {
        // Single-use cooldown: one click, one 25 s lockout. The
        // "charges" mechanic was removed so Firewall has the same
        // rhythm as Rate Limit and HEP Capture — a clear 25-second
        // sweep button instead of a five-tap burst that then vanished.
        if (g.pwCD.fw > 0) return
        let killed = 0
        setPackets((prev) => {
          const next = [...prev]
          for (let i = next.length - 1; i >= 0 && killed < 5; i--) {
            const row = next[i]!
            if (row.def.bad && !row.clicked) {
              next[i] = {
                ...row,
                clicked: true,
                state: 'blocked',
                flashLabel: 'FW BLOCK',
                flashSub: 'auto',
              }
              killed++
            }
          }
          return next
        })
        setGs((prev) => {
          let combo = prev.combo
          let score = prev.score
          for (let k = 0; k < killed; k++) {
            score += 10 * combo
            combo = Math.min(combo + 1, 10)
          }
          return {
            ...prev,
            score,
            combo,
            blockedBad: prev.blockedBad + killed,
            pwCD: { ...prev.pwCD, fw: 25000 },
          }
        })
        showBanner(`Firewall: ${killed} threats nuked · 25s cooldown`)
        setTimeout(
          () =>
            setPackets((pp) =>
              pp.filter((p2) => {
                if (!p2.clicked) return true
                if (p2.flashSub === 'auto' && p2.flashLabel === 'FW BLOCK') return false
                return true
              }),
            ),
          340,
        )
      }
    },
    [showBanner],
  )

  const tick = useCallback(
    (ts: number) => {
      if (!gsRef.current.running) return
      if (!lastFRef.current) lastFRef.current = ts
      if (pausedRef.current) {
        lastFRef.current = ts
        rafRef.current = requestAnimationFrame(tick)
        return
      }
      const dt = Math.min(ts - lastFRef.current, 50)
      lastFRef.current = ts

      setGs((g) => {
        const pwCD = {
          rl: Math.max(0, g.pwCD.rl - dt),
          hep: Math.max(0, g.pwCD.hep - dt),
          fw: Math.max(0, g.pwCD.fw - dt),
        }
        let frozen = g.frozen
        let freezeT = Math.max(0, g.freezeT - dt)
        if (freezeT === 0) frozen = false
        let hepActive = g.hepActive
        let hepT = Math.max(0, g.hepT - dt)
        if (hepT === 0) hepActive = false
        return { ...g, pwCD, frozen, freezeT, hepActive, hepT }
      })

      const g = gsRef.current
      if (!g.running) return

      if (!g.frozen) {
        spawnTRef.current += dt
        waveTRef.current += dt

        if (spawnTRef.current >= g.spawnInterval) {
          spawnTRef.current = 0
          spawnPacket()
        }
        if (waveTRef.current >= 15000) {
          waveTRef.current = 0
          setGs((prev) => ({
            ...prev,
            wave: prev.wave + 1,
            spawnInterval: Math.max(280, prev.spawnInterval - 75),
          }))
        }
      }

      const floor = arenaPxRef.current.h - SBC_H - 36

      setPackets((prev) => {
        let mosHit = false
        let goodPass = false
        const next = prev.map((pk) => {
          if (pk.clicked) return pk
          const y = g.frozen ? pk.y : pk.y + (pk.speed * dt) / 1000
          if (y >= floor) {
            if (pk.def.bad) mosHit = true
            else goodPass = true
            return { ...pk, y, clicked: true, state: 'passed' as const }
          }
          return { ...pk, y }
        })

        if (mosHit) {
          flashSBC(true)
          setGs((g2) => {
            const mos = Math.max(1.0, g2.mos - 0.5)
            const dead = mos <= 1.0
            if (dead) setGameOver(true)
            return { ...g2, mos, combo: 1, passedThru: g2.passedThru + 1, running: dead ? false : g2.running }
          })
        }
        if (goodPass) {
          flashSBC(false)
          setGs((g2) => ({ ...g2, score: g2.score + 2 }))
        }
        return next.filter((pk) => {
          if (!pk.clicked) return true
          if (pk.y >= floor) return false
          return true
        })
      })

      rafRef.current = requestAnimationFrame(tick)
    },
    [spawnPacket, flashSBC],
  )

  const startGame = useCallback(() => {
    setPaused(false)
    setPackets([])
    setGameOver(false)
    setBanner('')
    spawnTRef.current = 0
    waveTRef.current = 0
    lastFRef.current = 0
    setGs({
      running: true,
      score: 0,
      mos: 4.4,
      wave: 1,
      combo: 1,
      blockedBad: 0,
      falsePos: 0,
      passedThru: 0,
      spawnInterval: 900,
      pwCD: { rl: 0, hep: 0, fw: 0 },
      frozen: false,
      freezeT: 0,
      hepActive: false,
      hepT: 0,
    })
    cancelAnimationFrame(rafRef.current)
    rafRef.current = requestAnimationFrame(tick)
  }, [tick])

  useEffect(() => () => cancelAnimationFrame(rafRef.current), [])

  const mosColor = gs.mos >= 4 ? '#22863a' : gs.mos >= 3 ? '#b08800' : '#cb2431'

  return (
    <div className="flex h-full min-h-0 flex-col select-none font-mono text-xs text-foreground">
      <div className="mb-1 flex flex-wrap items-center gap-3 gap-y-1 border-b border-border/60 pb-1">
        <div className="flex items-end gap-2">
          <div className="rounded-md border border-primary/40 bg-primary/5 px-2.5 py-1">
            <div className="text-[10px] font-medium uppercase tracking-wide text-muted-foreground">Score</div>
            <div className="text-lg font-bold tabular-nums leading-tight text-primary">{gs.score}</div>
          </div>
          <HudStat label="Best" value={String(bestScore)} />
        </div>
        <HudStat label="MOS" value={gs.mos.toFixed(1)} color={mosColor} />
        <HudStat label="Wave" value={String(gs.wave)} />
        <HudStat label="Combo" value={`x${gs.combo}`} />
        <div className="min-w-[4px] flex-1" />
        <PowerBtn
          icon="🛡"
          name="Rate Limit"
          cd={gs.pwCD.rl}
          cdMax={18000}
          running={gs.running && !paused}
          onClick={() => usePower('rl')}
        />
        <PowerBtn
          icon="🔍"
          name="HEP Capture"
          cd={gs.pwCD.hep}
          cdMax={20000}
          running={gs.running && !paused}
          onClick={() => usePower('hep')}
        />
        <PowerBtn
          icon="🔥"
          name="Firewall"
          cd={gs.pwCD.fw}
          cdMax={25000}
          running={gs.running && !paused}
          onClick={() => usePower('fw')}
        />
        <Button
          type="button"
          variant={liveTraffic ? 'default' : 'outline'}
          size="sm"
          className="h-8 shrink-0 text-xs"
          onClick={() => setLiveTraffic((v) => !v)}
          title={
            liveTraffic
              ? `Real SIP traffic mixed in (stream ${liveState})`
              : 'Stream real SIP traffic from homer-core and mix it into the game'
          }
        >
          {liveTraffic
            ? liveState === 'open'
              ? 'Live ● on'
              : `Live ${liveState}`
            : 'Live ○ off'}
        </Button>
        <Button
          type="button"
          variant="outline"
          size="sm"
          className="h-8 shrink-0 text-xs"
          disabled={!gs.running || gameOver}
          onClick={() => setPaused((p) => !p)}
        >
          {paused ? 'Resume' : 'Pause'}
        </Button>
        <Button type="button" variant="secondary" size="sm" className="h-8 shrink-0 text-xs" onClick={startGame}>
          {gs.running ? 'Restart' : 'Start'}
        </Button>
      </div>

      <div
        ref={arenaElRef}
        className="relative min-h-0 flex-1 cursor-crosshair overflow-hidden rounded-lg border border-border bg-muted/40"
      >
        {gs.frozen && (
          <div
            className="pointer-events-none absolute inset-0 z-[15] rounded-lg border-2 border-primary/60 bg-primary/10"
            aria-hidden
          />
        )}

        {paused && gs.running && (
          <div
            className="absolute inset-0 z-[25] flex items-center justify-center rounded-lg bg-background/55 backdrop-blur-[1px]"
            aria-live="polite"
          >
            <span className="rounded-md border border-border bg-card px-4 py-2 text-center text-sm font-semibold shadow-sm">
              <span className="block">Paused</span>
              <span className="mt-1 block text-xs font-normal tabular-nums text-muted-foreground">
                Score {gs.score}
                {bestScore > 0 ? ` · Best ${bestScore}` : ''}
              </span>
            </span>
          </div>
        )}

        {!gs.running && !gameOver && (
          <div className="pointer-events-none absolute left-1/2 top-[38%] z-[5] w-[90%] max-w-md -translate-x-1/2 -translate-y-1/2 text-center text-muted-foreground">
            <p>
              Click <b className="text-destructive">bad</b> packets (red/orange) to drop them.
            </p>
            <p className="mt-1">
              Let <b className="text-emerald-600 dark:text-emerald-400">good</b> ones (green/blue/teal) reach the SBC.
            </p>
            <p className="mt-2 text-[10px]">
              Power-ups unlock after start · every 15s a new wave — faster spawn and more bad traffic
            </p>
          </div>
        )}

        {banner ? (
          <div className="pointer-events-none absolute left-1/2 top-2 z-20 -translate-x-1/2 whitespace-nowrap rounded-md border border-border bg-card px-3 py-1 text-[11px] font-medium shadow-sm">
            {banner}
          </div>
        ) : null}

        {packets.map((pk) => (
          <button
            type="button"
            key={pk.id}
            onClick={() => handleClick(pk.id)}
            className={cn(
              'absolute rounded-lg border p-2 text-left transition-colors',
              gs.hepActive && pk.def.bad && !pk.clicked && 'outline outline-2 outline-offset-1 outline-destructive',
            )}
            style={{
              left: pk.x,
              top: pk.y,
              width: PKT_W,
              background:
                pk.state === 'blocked' ? '#ddfbe7' : pk.state === 'missed' ? '#ffebe9' : pk.def.fill,
              borderColor:
                pk.state === 'blocked' ? '#2da44e' : pk.state === 'missed' ? '#cf222e' : pk.def.border,
            }}
          >
            <div
              className="text-[12px] font-semibold"
              style={{
                color:
                  pk.state !== 'alive'
                    ? pk.state === 'blocked'
                      ? '#2da44e'
                      : '#cf222e'
                    : pk.def.textColor,
              }}
            >
              {pk.flashLabel ?? pk.def.label}
            </div>
            <div
              className="mt-0.5 truncate text-[10px]"
              style={{
                color:
                  pk.state !== 'alive'
                    ? pk.state === 'blocked'
                      ? '#2da44e'
                      : '#cf222e'
                    : pk.def.subColor,
              }}
            >
              {pk.flashSub ?? pk.def.sub}
            </div>
          </button>
        ))}

        <div
          className={cn(
            'absolute bottom-0 left-0 right-0 flex items-center justify-center border-t border-border text-[12px] font-medium transition-colors',
            sbcFlash === 'bad' && 'bg-destructive/15 text-destructive',
            sbcFlash === 'good' && 'bg-emerald-500/15 text-emerald-700 dark:text-emerald-400',
            sbcFlash === 'idle' && 'bg-card/90 text-muted-foreground',
          )}
          style={{ height: SBC_H }}
        >
          {sbcFlash === 'bad' ? '⚠ BAD PACKET REACHED SBC ⚠' : '━━━━━━━━ SBC / MEDIA PROXY ━━━━━━━━'}
        </div>

        {gameOver ? (
          <div className="absolute inset-0 z-30 flex items-center justify-center bg-background/70 backdrop-blur-[2px]">
            <div className="max-w-sm rounded-lg border border-border bg-card p-6 text-center shadow-lg">
              <div className="text-base font-semibold">Call quality collapsed</div>
              <div className="mt-1 text-muted-foreground">MOS ≤ 1.0</div>
              <div className="mt-3 text-sm">
                Score: <b className="tabular-nums">{gs.score}</b>
                {bestScore > 0 ? (
                  <span className="text-muted-foreground">
                    {' '}
                    · Best: <b className="tabular-nums text-foreground">{bestScore}</b>
                  </span>
                ) : null}
              </div>
              <div className="mt-1 text-muted-foreground">
                Wave {gs.wave} · Blocked {gs.blockedBad} threats
              </div>
              <Button type="button" className="mt-4" size="sm" onClick={startGame}>
                Play again
              </Button>
            </div>
          </div>
        ) : null}
      </div>

      <div className="mt-1 flex justify-between text-[10px] text-muted-foreground">
        <span>
          Bad blocked: {gs.blockedBad} · False positives: {gs.falsePos} · Bad passed: {gs.passedThru}
        </span>
        {gs.frozen ? <span className="font-medium text-primary">FROZEN</span> : null}
      </div>
    </div>
  )
}

function HudStat({ label, value, color }: { label: string; value: string; color?: string }) {
  return (
    <div>
      <div className="text-[10px] text-muted-foreground">{label}</div>
      <div className="text-base font-semibold" style={color ? { color } : undefined}>
        {value}
      </div>
    </div>
  )
}

function PowerBtn({
  icon,
  name,
  cd,
  cdMax,
  running,
  onClick,
}: {
  icon: string
  name: string
  /** Remaining cooldown in ms. 0 means ready. */
  cd: number
  /** Max cooldown in ms for this power; used for tooltip + progress. */
  cdMax: number
  running: boolean
  onClick: () => void
}) {
  // Single rule now that charges are gone: you can click when the
  // game is running and the cooldown timer is zero. Pause does NOT
  // disable the button itself (usePower already guards paused game),
  // but we keep running-only to match the rest of the HUD.
  const onCooldown = cd > 0
  const disabled = !running || onCooldown
  const secsLeft = onCooldown ? Math.ceil(cd / 1000) : 0
  return (
    <Button
      type="button"
      variant="outline"
      size="sm"
      disabled={disabled}
      onClick={onClick}
      title={onCooldown ? `${name}: ${secsLeft}s cooldown` : `${name}: ready (${Math.round(cdMax / 1000)}s cooldown per use)`}
      className="flex h-auto min-w-[72px] flex-col gap-0.5 py-1.5 text-[10px] font-medium"
    >
      <span className="text-base leading-none">{icon}</span>
      <span>{name}</span>
      <span className="font-normal text-muted-foreground tabular-nums">
        {onCooldown ? `${secsLeft}s` : 'ready'}
      </span>
    </Button>
  )
}
