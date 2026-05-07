/**
 * Jitter Buffer Hero — RTP with PT/SSRC/timestamp, jittered arrival times,
 * deadline bar, duplicate drop, loss simulation, playout buffer visualization.
 */

import { useCallback, useEffect, useRef, useState } from 'react'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'
import { readBestScore, writeBestScoreIfHigher } from './gameScoreStorage'

interface RtpPacket {
  id: number
  seq: number
  ts: number
  pt: number
  codec: string
  ssrc: string
  size: number
  arrivalMs: number
  deadlineMs: number
  windowMs: number
  state: 'waiting' | 'selected' | 'played' | 'late' | 'duplicate'
  jitterMs: number
}

interface WaveConfig {
  seqCount: number
  jitterRange: number
  speedMs: number
  deadlineMs: number
  dupProb: number
  lossProb: number
}

interface CodecRow {
  name: string
  pt: number
  size: number
  /** RTP clock Hz */
  clockHz: number
}

const CODECS: CodecRow[] = [
  { name: 'PCMU', pt: 0, size: 160, clockHz: 8000 },
  { name: 'PCMA', pt: 8, size: 160, clockHz: 8000 },
  { name: 'G722', pt: 9, size: 160, clockHz: 8000 },
  { name: 'G729', pt: 18, size: 20, clockHz: 8000 },
  { name: 'opus', pt: 111, size: 80, clockHz: 48000 },
  { name: 'G726-32', pt: 2, size: 80, clockHz: 8000 },
]

/** RTP timestamp increment for one 20 ms frame (RFC 3551 / typical VoIP). */
function tsStepForCodec(c: CodecRow): number {
  return Math.round(c.clockHz * 0.02)
}

function randomSSRC(): string {
  const n = (Math.random() * 0xffffffff) >>> 0
  return `0x${n.toString(16).toUpperCase().padStart(8, '0')}`
}

const JITTER_GAME_ID = 'jitter_buffer_hero'

const WAVE_CONFIGS: WaveConfig[] = [
  { seqCount: 5, jitterRange: 60, speedMs: 1800, deadlineMs: 6000, dupProb: 0, lossProb: 0 },
  { seqCount: 6, jitterRange: 90, speedMs: 1500, deadlineMs: 5000, dupProb: 0.05, lossProb: 0 },
  { seqCount: 7, jitterRange: 120, speedMs: 1300, deadlineMs: 4500, dupProb: 0.08, lossProb: 0.05 },
  { seqCount: 8, jitterRange: 150, speedMs: 1100, deadlineMs: 4000, dupProb: 0.1, lossProb: 0.08 },
  { seqCount: 9, jitterRange: 180, speedMs: 950, deadlineMs: 3500, dupProb: 0.12, lossProb: 0.1 },
  { seqCount: 10, jitterRange: 200, speedMs: 800, deadlineMs: 3000, dupProb: 0.15, lossProb: 0.12 },
]

let _uid = 0
const uid = () => ++_uid

function waveConfig(wave: number): WaveConfig {
  return WAVE_CONFIGS[Math.min(wave - 1, WAVE_CONFIGS.length - 1)]!
}

interface WaveQueued {
  seq: number
  arrivalMs: number
  isDup: boolean
  isLost: boolean
  /** |actual relative arrival − nominal slot|, for UI display. */
  jitterDisplayMs: number
}

interface WaveBuilt {
  arrivals: WaveQueued[]
  codec: CodecRow
  ssrc: string
  cfg: WaveConfig
  baseSeq: number
  baseRtpTs: number
  tsStep: number
}

export default function JitterBufferHeroPanel() {
  const [running, setRunning] = useState(false)
  const [gameOver, setGameOver] = useState(false)
  const [score, setScore] = useState(0)
  const [mos, setMos] = useState(4.4)
  const [wave, setWave] = useState(1)
  const [combo, setCombo] = useState(1)
  const [packets, setPackets] = useState<RtpPacket[]>([])
  const [nextExpected, setNextExpected] = useState(1)
  const [playedSeq, setPlayedSeq] = useState<number[]>([])
  const [stats, setStats] = useState({ played: 0, late: 0, dup: 0, lost: 0 })
  const [flash, setFlash] = useState('')
  const [paused, setPaused] = useState(false)
  const [bestScore, setBestScore] = useState(0)
  const rafRef = useRef(0)
  const pausedRef = useRef(false)
  const prevGameOverRef = useRef(false)
  const lastFRef = useRef(0)
  const gameTimeMs = useRef(0)
  const nextSeqRef = useRef(1)
  const spawnRef = useRef(0)
  const spawnIdxRef = useRef(0)
  const waveRef = useRef(1)
  const runRef = useRef(false)
  const wavePacketsRef = useRef<WaveQueued[]>([])
  const waveMetaRef = useRef<{
    codec: CodecRow
    ssrc: string
    baseSeq: number
    baseRtpTs: number
    tsStep: number
    deadlineMs: number
  } | null>(null)
  const comboRef = useRef(1)
  const waveTransitionRef = useRef(false)

  comboRef.current = combo
  pausedRef.current = paused

  useEffect(() => {
    if (!running) setPaused(false)
  }, [running])

  useEffect(() => {
    setBestScore(readBestScore(JITTER_GAME_ID))
  }, [])

  useEffect(() => {
    if (gameOver && !prevGameOverRef.current) {
      setBestScore(writeBestScoreIfHigher(JITTER_GAME_ID, score))
    }
    prevGameOverRef.current = gameOver
  }, [gameOver, score])

  const showFlash = useCallback((msg: string) => {
    setFlash(msg)
    setTimeout(() => setFlash(''), 1200)
  }, [])

  const endGame = useCallback(() => {
    runRef.current = false
    setRunning(false)
    setGameOver(true)
    cancelAnimationFrame(rafRef.current)
  }, [])

  const buildWavePackets = useCallback((w: number, base: number): WaveBuilt => {
    const cfg = waveConfig(w)
    const codec = CODECS[Math.floor(Math.random() * CODECS.length)]!
    const ssrc = randomSSRC()
    const tsStep = tsStepForCodec(codec)
    const baseRtpTs = Math.floor(Math.random() * 0xfffffff)
    const seqs = Array.from({ length: cfg.seqCount }, (_, i) => base + i)

    const arrivals: WaveQueued[] = seqs.map((seq, i) => {
      const nominalMs = i * cfg.speedMs
      const jitter = (Math.random() - 0.5) * cfg.jitterRange
      const arrivalMs = Math.max(0, nominalMs + jitter)
      return {
        seq,
        arrivalMs,
        isDup: false,
        isLost: false,
        jitterDisplayMs: Math.round(Math.abs(arrivalMs - nominalMs)),
      }
    })
    arrivals.sort((a, b) => a.arrivalMs - b.arrivalMs)

    const withDups: WaveQueued[] = [...arrivals]
    arrivals.forEach((p) => {
      if (Math.random() < cfg.dupProb) {
        const dupArrival = p.arrivalMs + cfg.speedMs * 0.5
        withDups.push({
          seq: p.seq,
          arrivalMs: dupArrival,
          isDup: true,
          isLost: false,
          jitterDisplayMs: p.jitterDisplayMs + Math.round(cfg.speedMs * 0.25),
        })
      }
    })

    const withLoss: WaveQueued[] = withDups.map((p) => {
      if (p.isDup) return p
      let lost = Math.random() < cfg.lossProb
      if (lost && p.seq === base) lost = false
      return { ...p, isLost: lost }
    })

    withLoss.sort((a, b) => a.arrivalMs - b.arrivalMs)
    return { arrivals: withLoss, codec, ssrc, cfg, baseSeq: base, baseRtpTs, tsStep }
  }, [])

  const startWave = useCallback(
    (w: number) => {
      waveTransitionRef.current = false
      const base = Math.floor(Math.random() * 25000) + 5000
      const built = buildWavePackets(w, base)
      wavePacketsRef.current = built.arrivals
      waveMetaRef.current = {
        codec: built.codec,
        ssrc: built.ssrc,
        baseSeq: built.baseSeq,
        baseRtpTs: built.baseRtpTs,
        tsStep: built.tsStep,
        deadlineMs: built.cfg.deadlineMs,
      }
      spawnIdxRef.current = 0
      spawnRef.current = gameTimeMs.current
      nextSeqRef.current = base
      setNextExpected(base)
      setPackets([])
      setPlayedSeq([])
    },
    [buildWavePackets],
  )

  const tick = useCallback(
    (ts: number) => {
      if (!runRef.current) return
      if (!lastFRef.current) lastFRef.current = ts
      if (pausedRef.current) {
        lastFRef.current = ts
        rafRef.current = requestAnimationFrame(tick)
        return
      }
      const dt = Math.min(ts - lastFRef.current, 50)
      lastFRef.current = ts
      gameTimeMs.current += dt
      const nowMs = gameTimeMs.current

      const meta = waveMetaRef.current
      const waveQ = wavePacketsRef.current

      while (spawnIdxRef.current < waveQ.length) {
        const wp = waveQ[spawnIdxRef.current]!
        if (nowMs < wp.arrivalMs + spawnRef.current) break

        if (!wp.isLost) {
          if (!meta) break
          const rtpTs = meta.baseRtpTs + (wp.seq - meta.baseSeq) * meta.tsStep
          const windowMs = meta.deadlineMs
          const pkt: RtpPacket = {
            id: uid(),
            seq: wp.seq,
            ts: rtpTs,
            pt: meta.codec.pt,
            codec: meta.codec.name,
            ssrc: meta.ssrc,
            size: meta.codec.size,
            arrivalMs: nowMs,
            deadlineMs: nowMs + windowMs,
            windowMs,
            state: wp.isDup ? 'duplicate' : 'waiting',
            jitterMs: wp.jitterDisplayMs,
          }
          setPackets((prev) => [...prev, pkt])
          if (wp.isDup) {
            setStats((s) => ({ ...s, dup: s.dup + 1 }))
            showFlash(`Duplicate seq=${wp.seq} — drop it!`)
          }
        } else {
          setStats((s) => ({ ...s, lost: s.lost + 1 }))
          setMos((m) => Math.max(1.0, m - 0.15))
          showFlash(`Lost seq=${wp.seq} — MOS −0.15`)
          if (wp.seq === nextSeqRef.current) {
            nextSeqRef.current += 1
            setNextExpected(nextSeqRef.current)
          }
        }

        spawnIdxRef.current += 1
      }

      setPackets((prev) => {
        let mosHit = false
        const next = prev.map((p) => {
          if (p.state !== 'waiting') return p
          if (nowMs > p.deadlineMs) {
            mosHit = true
            setStats((s) => ({ ...s, late: s.late + 1 }))
            return { ...p, state: 'late' as const }
          }
          return p
        })
        if (mosHit) setMos((m) => Math.max(1.0, m - 0.3))
        return next
      })

      const allSpawned = spawnIdxRef.current >= waveQ.length
      if (allSpawned && !waveTransitionRef.current) {
        setPackets((prev) => {
          const active = prev.filter((p) => p.state === 'waiting' || p.state === 'duplicate')
          if (active.length === 0) {
            waveTransitionRef.current = true
            setTimeout(() => {
              const nextW = waveRef.current + 1
              waveRef.current = nextW
              setWave(nextW)
              showFlash(`Wave ${nextW} — stronger jitter`)
              startWave(nextW)
            }, 600)
          }
          return prev
        })
      }

      setMos((m) => {
        if (m <= 1.0 && runRef.current) endGame()
        return m
      })

      rafRef.current = requestAnimationFrame(tick)
    },
    [startWave, showFlash, endGame],
  )

  const startGame = useCallback(() => {
    setPaused(false)
    cancelAnimationFrame(rafRef.current)
    gameTimeMs.current = 0
    lastFRef.current = 0
    waveRef.current = 1
    waveTransitionRef.current = false
    runRef.current = true
    setRunning(true)
    setGameOver(false)
    setScore(0)
    setMos(4.4)
    setWave(1)
    setCombo(1)
    setStats({ played: 0, late: 0, dup: 0, lost: 0 })
    setFlash('')
    startWave(1)
    rafRef.current = requestAnimationFrame(tick)
  }, [startWave, tick])

  useEffect(() => () => cancelAnimationFrame(rafRef.current), [])

  const handlePacketClick = useCallback(
    (pkt: RtpPacket) => {
      if (!running) return
      if (pausedRef.current) return

      if (pkt.state === 'duplicate') {
        setPackets((prev) => prev.filter((p) => p.id !== pkt.id))
        setScore((s) => s + 5)
        showFlash('Duplicate dropped ✓ +5')
        return
      }

      if (pkt.state !== 'waiting') return

      const expected = nextSeqRef.current

      if (pkt.seq === expected) {
        setPackets((prev) => prev.map((p) => (p.id === pkt.id ? { ...p, state: 'played' as const } : p)))
        const c = comboRef.current
        setScore((s) => s + 15 * c)
        setCombo((x) => {
          const n = Math.min(x + 1, 8)
          comboRef.current = n
          return n
        })
        setStats((s) => ({ ...s, played: s.played + 1 }))
        nextSeqRef.current = expected + 1
        setNextExpected(expected + 1)
        setPlayedSeq((ps) => [...ps, pkt.seq])
        setTimeout(() => setPackets((prev) => prev.filter((p) => p.id !== pkt.id)), 400)
      } else if (pkt.seq < expected) {
        showFlash(`seq=${pkt.seq} already played`)
      } else {
        setCombo(1)
        comboRef.current = 1
        setMos((m) => Math.max(1.0, m - 0.2))
        showFlash(`Out of order! Need seq=${expected}, got ${pkt.seq} — MOS −0.2`)
      }
    },
    [running, showFlash],
  )

  const mosColor = mos >= 4 ? '#22863a' : mos >= 3 ? '#b08800' : '#cf222e'
  const cfg = waveConfig(wave)
  const nowMs = gameTimeMs.current

  const bufferSlots = 24
  const playedTail = playedSeq.slice(-bufferSlots)
  const fillCount = Math.min(playedTail.length, bufferSlots)

  return (
    <div className="flex h-full min-h-0 flex-col font-mono text-xs text-foreground">
      <div className="mb-2 flex flex-wrap items-center gap-3 gap-y-1 border-b border-border/60 pb-2">
        <div className="flex items-end gap-2">
          <div className="rounded-md border border-primary/40 bg-primary/5 px-2.5 py-1">
            <div className="text-[10px] font-medium uppercase tracking-wide text-muted-foreground">Score</div>
            <div className="text-lg font-bold tabular-nums leading-tight text-primary">{score}</div>
          </div>
          <HudV label="Best" val={String(bestScore)} />
        </div>
        <HudV label="MOS" val={mos.toFixed(1)} color={mosColor} />
        <HudV label="Wave" val={String(wave)} />
        <HudV label="Combo" val={`x${combo}`} />
        <div className="min-w-[8px] flex-1" />
        <span className="text-[10px] text-muted-foreground">
          played {stats.played} · late {stats.late} · dup {stats.dup} · lost {stats.lost}
        </span>
        <Button
          type="button"
          variant="outline"
          size="sm"
          className="h-8 text-xs"
          disabled={!running || gameOver}
          onClick={() => setPaused((p) => !p)}
        >
          {paused ? 'Resume' : 'Pause'}
        </Button>
        <Button type="button" variant="secondary" size="sm" className="h-8 text-xs" onClick={startGame}>
          {running ? 'Restart' : 'Start'}
        </Button>
      </div>

      {running ? (
        <div className="mb-2 flex flex-wrap gap-x-4 gap-y-1 text-[11px] text-muted-foreground">
          <span>
            Next: <b className="text-foreground">seq={nextExpected}</b>
          </span>
          <span>jitter ±{Math.round(cfg.jitterRange / 2)} ms (spawn)</span>
          <span>deadline {cfg.deadlineMs / 1000}s / pkt</span>
          <LegendDot className="bg-primary" label="waiting" />
          <LegendDot className="bg-emerald-500" label="played" />
          <LegendDot className="bg-destructive" label="duplicate → drop" />
          <LegendDot className="bg-muted-foreground" label="late" />
        </div>
      ) : null}

      <div className="relative flex min-h-0 flex-1 flex-col rounded-lg border border-border bg-muted/30 p-3">
        {paused && running && (
          <div className="absolute inset-0 z-10 flex items-center justify-center rounded-lg bg-background/50 backdrop-blur-[1px]">
            <span className="rounded-md border border-border bg-card px-4 py-2 text-center text-sm font-semibold shadow-sm">
              <span className="block">Paused</span>
              <span className="mt-1 block text-xs font-normal tabular-nums text-muted-foreground">
                Score {score}
                {bestScore > 0 ? ` · Best ${bestScore}` : ''}
              </span>
            </span>
          </div>
        )}
        {flash ? (
          <div className="pointer-events-none mb-2 rounded-md border border-border bg-card px-3 py-1 text-center text-[11px] font-medium shadow-sm">
            {flash}
          </div>
        ) : null}

        {!running && !gameOver && (
          <div className="flex flex-1 flex-col items-center justify-center gap-2 p-4 text-center text-muted-foreground">
            <p className="text-sm">RTP arrives with jitter — out of order.</p>
            <p>
              Click packets in <b className="text-foreground">ascending seq</b> order before the deadline.
            </p>
            <p className="max-w-md text-[11px]">
              Real PT / SSRC / RTP TS (20 ms clock step), drop duplicates, loss and late arrivals hurt MOS.
              The bar is remaining time until playout.
            </p>
          </div>
        )}

        <div className={cn('flex flex-wrap gap-2', flash ? 'pt-1' : '')}>
          {packets.map((pkt) => (
            <RtpPacketCard key={pkt.id} pkt={pkt} now={nowMs} onClick={() => handlePacketClick(pkt)} />
          ))}
        </div>

        {running && playedSeq.length > 0 ? (
          <div className="mt-auto border-t border-border/80 pt-3">
            <div className="mb-1 text-[10px] font-semibold uppercase tracking-wide text-muted-foreground">
              Playout buffer (last {bufferSlots} seq)
            </div>
            <div className="mb-2 flex h-8 gap-px overflow-hidden rounded border border-border bg-background/80 p-px">
              {Array.from({ length: bufferSlots }).map((_, i) => {
                const filled = i < fillCount
                return (
                  <div
                    key={i}
                    className={cn(
                      'min-w-0 flex-1 rounded-sm transition-colors',
                      filled ? 'bg-emerald-500/70' : 'bg-muted/40',
                    )}
                    title={filled ? `seq ${playedTail[i]}` : 'empty'}
                  />
                )
              })}
            </div>
            <div className="flex flex-wrap gap-1">
              {playedTail.map((s) => (
                <span
                  key={s}
                  className="rounded border border-emerald-600 bg-emerald-500/15 px-1.5 py-0.5 text-[10px] text-emerald-800 dark:text-emerald-300"
                >
                  {s}
                </span>
              ))}
              <span className="animate-pulse rounded border border-primary bg-primary/10 px-2 py-0.5 text-[10px] text-primary">
                ▶ playout
              </span>
            </div>
          </div>
        ) : null}
      </div>

      {gameOver ? (
        <div className="mt-2 rounded-lg border border-border bg-muted/40 p-5 text-center">
          <div className="text-base font-semibold">Jitter buffer / MOS collapsed</div>
          <div className="mt-1 text-sm text-muted-foreground">MOS ≤ 1.0</div>
          <div className="mt-3 flex flex-wrap justify-center gap-6 text-sm">
            <StatBox label="Score" val={String(score)} />
            {bestScore > 0 ? <StatBox label="Best" val={String(bestScore)} /> : null}
            <StatBox label="Wave" val={String(wave)} />
            <StatBox label="Played" val={String(stats.played)} />
            <StatBox label="Late" val={String(stats.late)} warn />
            <StatBox label="Lost" val={String(stats.lost)} warn />
          </div>
          <Button type="button" className="mt-4" size="sm" onClick={startGame}>
            Play again
          </Button>
        </div>
      ) : null}
    </div>
  )
}

function RtpPacketCard({ pkt, onClick, now }: { pkt: RtpPacket; onClick: () => void; now: number }) {
  const remaining = Math.max(0, pkt.deadlineMs - now)
  const denom = Math.max(1, pkt.windowMs)
  const pct = Math.min(100, (remaining / denom) * 100)

  const bg =
    pkt.state === 'duplicate'
      ? 'bg-destructive/10'
      : pkt.state === 'played'
        ? 'bg-emerald-500/15'
        : pkt.state === 'late'
          ? 'bg-muted/50'
          : 'bg-card'
  const border =
    pkt.state === 'duplicate'
      ? 'border-destructive'
      : pkt.state === 'played'
        ? 'border-emerald-600'
        : pkt.state === 'late'
          ? 'border-muted-foreground/50'
          : 'border-primary/60'

  return (
    <button
      type="button"
      onClick={onClick}
      disabled={pkt.state !== 'waiting' && pkt.state !== 'duplicate'}
      className={cn(
        'w-[148px] rounded-lg border p-2 text-left transition-colors',
        bg,
        border,
        pkt.state === 'late' && 'opacity-50',
        (pkt.state === 'waiting' || pkt.state === 'duplicate') && 'cursor-pointer hover:bg-accent/40',
        pkt.state !== 'waiting' && pkt.state !== 'duplicate' && 'cursor-default',
      )}
    >
      <div className="mb-1 flex justify-between gap-1">
        <span
          className={cn(
            'font-mono text-[11px] font-semibold',
            pkt.state === 'duplicate' ? 'text-destructive' : 'text-primary',
          )}
        >
          {pkt.state === 'duplicate' ? 'DUP ' : ''}seq={pkt.seq}
        </span>
        {pkt.state === 'played' ? <span className="text-[10px] text-emerald-600">✓</span> : null}
        {pkt.state === 'late' ? <span className="text-[10px] text-muted-foreground">LATE</span> : null}
      </div>
      <div className="text-[10px] text-muted-foreground">
        {pkt.codec} PT={pkt.pt} · {pkt.size} B
      </div>
      <div className="truncate font-mono text-[10px] text-muted-foreground/90">{pkt.ssrc}</div>
      <div className="mb-1 font-mono text-[10px] text-muted-foreground">TS={pkt.ts}</div>
      <div className="mb-1 text-[10px] text-muted-foreground">sched jitter ~{pkt.jitterMs} ms</div>
      {pkt.state === 'waiting' ? (
        <div className="h-1 overflow-hidden rounded-full bg-border">
          <div
            className={cn(
              'h-full rounded-full transition-[width] duration-100',
              pct > 50 ? 'bg-emerald-600' : pct > 25 ? 'bg-amber-500' : 'bg-destructive',
            )}
            style={{ width: `${pct}%` }}
          />
        </div>
      ) : null}
    </button>
  )
}

function HudV({ label, val, color }: { label: string; val: string; color?: string }) {
  return (
    <div>
      <div className="text-[10px] text-muted-foreground">{label}</div>
      <div className="text-base font-semibold" style={color ? { color } : undefined}>
        {val}
      </div>
    </div>
  )
}

function LegendDot({ className, label }: { className: string; label: string }) {
  return (
    <span className="inline-flex items-center gap-1">
      <span className={cn('size-2 shrink-0 rounded-full', className)} />
      <span>{label}</span>
    </span>
  )
}

function StatBox({ label, val, warn }: { label: string; val: string; warn?: boolean }) {
  return (
    <div>
      <div className="text-[11px] text-muted-foreground">{label}</div>
      <div className={cn('text-lg font-semibold', warn && 'text-destructive')}>{val}</div>
    </div>
  )
}
