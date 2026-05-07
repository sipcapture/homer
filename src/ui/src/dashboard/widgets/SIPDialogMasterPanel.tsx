/**
 * SIP Dialog Master — build correct SIP dialog order; per-round timer, speed bonus, round history.
 */

import { useCallback, useEffect, useRef, useState } from 'react'
import { Button } from '@/components/ui/button'
import { Label } from '@/components/ui/label'
import { ScrollArea } from '@/components/ui/scroll-area'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { cn } from '@/lib/utils'
import { readBestScore, writeBestScoreIfHigher } from './gameScoreStorage'

interface SipSequence {
  name: string
  description: string
  steps: string[]
  difficulty: 'easy' | 'medium' | 'hard'
}

interface RoundResult {
  seqName: string
  correct: number
  total: number
  bonusScore: number
  timeMs: number
}

const SIP_SEQUENCES: SipSequence[] = [
  {
    name: 'Basic Call',
    description: 'Standard INVITE dialog with ringing',
    steps: ['INVITE', '100 Trying', '180 Ringing', '200 OK', 'ACK', 'BYE', '200 OK'],
    difficulty: 'easy',
  },
  {
    name: 'Fast Answer',
    description: 'Immediate answer, no ringing',
    steps: ['INVITE', '100 Trying', '200 OK', 'ACK', 'BYE', '200 OK'],
    difficulty: 'easy',
  },
  {
    name: 'Registration',
    description: 'REGISTER with digest challenge',
    steps: ['REGISTER', '401 Unauthorized', 'REGISTER', '200 OK'],
    difficulty: 'medium',
  },
  {
    name: 'Busy Rejection',
    description: 'Callee rejects — line busy',
    steps: ['INVITE', '100 Trying', '486 Busy Here', 'ACK'],
    difficulty: 'easy',
  },
  {
    name: 'No Answer (CANCEL)',
    description: 'Caller cancels while ringing',
    steps: ['INVITE', '100 Trying', '180 Ringing', 'CANCEL', '200 OK', '487 Request Terminated', 'ACK'],
    difficulty: 'hard',
  },
  {
    name: 'OPTIONS Ping',
    description: 'Keepalive OPTIONS',
    steps: ['OPTIONS', '200 OK'],
    difficulty: 'easy',
  },
  {
    name: 'Redirect 302',
    description: 'Temporary redirect, second INVITE',
    steps: ['INVITE', '100 Trying', '302 Moved Temporarily', 'ACK', 'INVITE', '200 OK', 'ACK', 'BYE', '200 OK'],
    difficulty: 'hard',
  },
  {
    name: 'Re-INVITE (Hold)',
    description: 'Mid-call hold via re-INVITE',
    steps: ['INVITE', '200 OK', 'ACK', 'INVITE', '200 OK', 'ACK', 'BYE', '200 OK'],
    difficulty: 'medium',
  },
  {
    name: 'Proxy Auth',
    description: '407 on INVITE, retry with credentials',
    steps: ['INVITE', '407 Proxy Auth Required', 'ACK', 'INVITE', '200 OK', 'ACK', 'BYE', '200 OK'],
    difficulty: 'hard',
  },
  {
    name: 'Not Found',
    description: 'Destination does not exist',
    steps: ['INVITE', '100 Trying', '404 Not Found', 'ACK'],
    difficulty: 'easy',
  },
]

const OPTION_POOLS: Record<string, string[]> = {
  INVITE: ['INVITE', 'REGISTER', 'SUBSCRIBE', 'OPTIONS'],
  '100 Trying': ['100 Trying', '180 Ringing', '200 OK', '101 Dialog Established'],
  '180 Ringing': ['180 Ringing', '183 Session Progress', '200 OK', '181 Call Is Being Forwarded'],
  '200 OK': ['200 OK', '202 Accepted', '183 Session Progress', '180 Ringing'],
  ACK: ['ACK', 'PRACK', 'BYE', 'CANCEL'],
  BYE: ['BYE', 'CANCEL', 'ACK', 'NOTIFY'],
  REGISTER: ['REGISTER', 'INVITE', 'SUBSCRIBE', 'PUBLISH'],
  '401 Unauthorized': ['401 Unauthorized', '403 Forbidden', '407 Proxy Auth Required', '200 OK'],
  '486 Busy Here': ['486 Busy Here', '480 Temporarily Unavailable', '600 Busy Everywhere', '200 OK'],
  CANCEL: ['CANCEL', 'BYE', 'ACK', 'OPTIONS'],
  '487 Request Terminated': ['487 Request Terminated', '488 Not Acceptable Here', '200 OK', '486 Busy Here'],
  OPTIONS: ['OPTIONS', 'INFO', 'MESSAGE', 'NOTIFY'],
  '302 Moved Temporarily': ['302 Moved Temporarily', '301 Moved Permanently', '305 Use Proxy', '200 OK'],
  '407 Proxy Auth Required': ['407 Proxy Auth Required', '401 Unauthorized', '403 Forbidden', '200 OK'],
  '404 Not Found': ['404 Not Found', '403 Forbidden', '480 Temporarily Unavailable', '200 OK'],
}

const DIFFICULTY_COLOR: Record<string, string> = {
  easy: '#2da44e',
  medium: '#b08800',
  hard: '#cf222e',
}

/** Base points per dialog step (depends on scenario difficulty). */
const DIFFICULTY_SCORE: Record<string, number> = {
  easy: 10,
  medium: 18,
  hard: 28,
}

type SessionDifficulty = 'easy' | 'medium' | 'hard'

/** Round score multiplier and which scenarios appear in the pool. */
const SESSION: Record<
  SessionDifficulty,
  { scoreMult: number; allow: (s: SipSequence) => boolean }
> = {
  easy: { scoreMult: 0.85, allow: (s) => s.difficulty === 'easy' },
  medium: { scoreMult: 1, allow: (s) => s.difficulty === 'easy' || s.difficulty === 'medium' },
  hard: { scoreMult: 1.2, allow: () => true },
}

function shuffle<T>(arr: T[]): T[] {
  const a = [...arr]
  for (let i = a.length - 1; i > 0; i--) {
    const j = Math.floor(Math.random() * (i + 1))
    ;[a[i], a[j]] = [a[j]!, a[i]!]
  }
  return a
}

function getOptions(step: string): string[] {
  const pool = OPTION_POOLS[step]
  if (!pool) return shuffle([step, 'BYE', 'ACK', 'CANCEL'])
  return shuffle(pool)
}

const SIP_GAME_ID = 'sip_dialog_master'

export default function SIPDialogMasterPanel() {
  const [running, setRunning] = useState(false)
  const [score, setScore] = useState(0)
  const [round, setRound] = useState(0)
  const [totalRounds, setTotalRounds] = useState(10)
  const [sessionDifficulty, setSessionDifficulty] = useState<SessionDifficulty>('medium')
  const [streak, setStreak] = useState(0)
  const [bestStreak, setBestStreak] = useState(0)
  const [seq, setSeq] = useState<SipSequence | null>(null)
  const [stepIdx, setStepIdx] = useState(0)
  const [options, setOptions] = useState<string[]>([])
  const [wrongFlash, setWrongFlash] = useState<string | null>(null)
  const [correctFlash, setCorrectFlash] = useState<string | null>(null)
  const [history, setHistory] = useState<RoundResult[]>([])
  const [gameOver, setGameOver] = useState(false)
  const [timeLeft, setTimeLeft] = useState(0)
  const [stepBonus, setStepBonus] = useState(0)
  const [betweenRounds, setBetweenRounds] = useState(false)
  const [paused, setPaused] = useState(false)
  const [highScore, setHighScore] = useState(0)

  const timerRef = useRef<ReturnType<typeof setInterval> | null>(null)
  const pausedRef = useRef(false)
  const prevGameOverRef = useRef(false)
  const roundStartRef = useRef<number>(0)
  const usedSeqs = useRef<Set<number>>(new Set())
  const stepBonusRef = useRef(0)
  const sessionDifficultyRef = useRef<SessionDifficulty>('medium')

  sessionDifficultyRef.current = sessionDifficulty
  stepBonusRef.current = stepBonus
  pausedRef.current = paused

  useEffect(() => {
    if (!running) setPaused(false)
  }, [running])

  useEffect(() => {
    setHighScore(readBestScore(SIP_GAME_ID))
  }, [])

  useEffect(() => {
    if (gameOver && !prevGameOverRef.current) {
      setHighScore(writeBestScoreIfHigher(SIP_GAME_ID, score))
    }
    prevGameOverRef.current = gameOver
  }, [gameOver, score])

  const pickNextSeq = useCallback(() => {
    const allow = SESSION[sessionDifficultyRef.current].allow
    const allowedIdx = SIP_SEQUENCES.map((s, i) => (allow(s) ? i : -1)).filter((i) => i >= 0)
    if (allowedIdx.length === 0) return SIP_SEQUENCES[0]!
    if (usedSeqs.current.size >= allowedIdx.length) usedSeqs.current.clear()
    let idx: number
    do {
      idx = allowedIdx[Math.floor(Math.random() * allowedIdx.length)]!
    } while (usedSeqs.current.has(idx))
    usedSeqs.current.add(idx)
    return SIP_SEQUENCES[idx]!
  }, [])

  const startRound = useCallback(
    (newSeq: SipSequence, roundN: number) => {
      setBetweenRounds(false)
      setSeq(newSeq)
      setStepIdx(0)
      setOptions(getOptions(newSeq.steps[0]!))
      const initialBonus = DIFFICULTY_SCORE[newSeq.difficulty]! * newSeq.steps.length
      setStepBonus(initialBonus)
      stepBonusRef.current = initialBonus
      setTimeLeft(newSeq.steps.length * 6)
      setWrongFlash(null)
      setCorrectFlash(null)
      setRound(roundN)
      roundStartRef.current = Date.now()

      if (timerRef.current) clearInterval(timerRef.current)
      timerRef.current = setInterval(() => {
        if (pausedRef.current) return
        setTimeLeft((t) => {
          if (t <= 1) {
            if (timerRef.current) clearInterval(timerRef.current)
            setHistory((h) => [
              ...h,
              {
                seqName: newSeq.name,
                correct: 0,
                total: newSeq.steps.length,
                bonusScore: 0,
                timeMs: Date.now() - roundStartRef.current,
              },
            ])
            setStreak(0)
            if (roundN >= totalRounds) {
              setGameOver(true)
              setRunning(false)
            } else {
              const ns = pickNextSeq()
              startRound(ns, roundN + 1)
            }
            return 0
          }
          return t - 1
        })
      }, 1000)
    },
    [totalRounds, pickNextSeq],
  )

  const startGame = useCallback(() => {
    setPaused(false)
    if (timerRef.current) clearInterval(timerRef.current)
    usedSeqs.current.clear()
    setScore(0)
    setRound(0)
    setStreak(0)
    setBestStreak(0)
    setHistory([])
    setGameOver(false)
    setBetweenRounds(false)
    setRunning(true)
    const ns = pickNextSeq()
    startRound(ns, 1)
  }, [pickNextSeq, startRound])

  useEffect(
    () => () => {
      if (timerRef.current) clearInterval(timerRef.current)
    },
    [],
  )

  const handleChoice = useCallback(
    (chosen: string) => {
      if (!seq || !running || betweenRounds || pausedRef.current) return
      const expected = seq.steps[stepIdx]!

      if (chosen === expected) {
        setCorrectFlash(chosen)
        setTimeout(() => setCorrectFlash(null), 300)
        setStepBonus((b) => {
          const n = Math.max(0, b - 2)
          stepBonusRef.current = n
          return n
        })
        const nextStep = stepIdx + 1

        if (nextStep >= seq.steps.length) {
          if (timerRef.current) clearInterval(timerRef.current)
          const elapsed = Date.now() - roundStartRef.current
          const timeBudgetSec = seq.steps.length * 6
          const timeBonusRaw = Math.max(0, (timeBudgetSec - elapsed / 1000) * 3)
          const timeBonus = Math.round(timeBonusRaw)
          const mult = SESSION[sessionDifficultyRef.current].scoreMult
          const earned = Math.round((stepBonusRef.current + timeBonus) * mult)

          setScore((s) => s + earned)
          setStreak((st) => {
            const n = st + 1
            setBestStreak((b) => Math.max(b, n))
            return n
          })
          setHistory((h) => [
            ...h,
            {
              seqName: seq.name,
              correct: seq.steps.length,
              total: seq.steps.length,
              bonusScore: earned,
              timeMs: elapsed,
            },
          ])

          if (round >= totalRounds) {
            setGameOver(true)
            setRunning(false)
            setSeq(null)
            setStepIdx(0)
          } else {
            setBetweenRounds(true)
            setTimeout(() => {
              const ns = pickNextSeq()
              startRound(ns, round + 1)
            }, 800)
          }
        } else {
          setStepIdx(nextStep)
          setOptions(getOptions(seq.steps[nextStep]!))
        }
      } else {
        setWrongFlash(chosen)
        setStepBonus((b) => {
          const n = Math.max(0, b - 8)
          stepBonusRef.current = n
          return n
        })
        setStreak(0)
        setTimeout(() => setWrongFlash(null), 400)
      }
    },
    [seq, stepIdx, running, betweenRounds, round, totalRounds, pickNextSeq, startRound],
  )

  const completed = seq ? stepIdx : 0
  const total = seq ? seq.steps.length : 0

  return (
    <div className="flex h-full min-h-0 flex-col font-mono text-xs text-foreground">
      <div className="mb-2 flex flex-wrap items-center justify-between gap-2 border-b border-border/60 pb-2">
        <div className="flex flex-wrap gap-4">
          <div className="flex items-end gap-2">
            <div className="rounded-md border border-primary/40 bg-primary/5 px-2.5 py-1">
              <div className="text-[10px] font-medium uppercase tracking-wide text-muted-foreground">Score</div>
              <div className="text-lg font-bold tabular-nums leading-tight text-primary">{score}</div>
            </div>
            <HudVal label="Record" val={String(highScore)} />
          </div>
          <HudVal label="Round" val={running ? `${round}/${totalRounds}` : '—'} />
          <HudVal label="Streak" val={streak > 0 ? `🔥 ${streak}` : '0'} />
          <HudVal label="Best" val={String(bestStreak)} />
          {running ? (
            <HudVal
              label="Time"
              val={`${timeLeft}s`}
              className={timeLeft <= 5 ? 'text-destructive' : undefined}
            />
          ) : null}
        </div>
        <div className="flex flex-wrap items-end gap-2">
          <div className="grid gap-0.5">
            <Label className="text-[10px] text-muted-foreground">Difficulty</Label>
            <Select
              value={sessionDifficulty}
              onValueChange={(v) => setSessionDifficulty(v as SessionDifficulty)}
              disabled={running}
            >
              <SelectTrigger className="h-8 w-[120px] text-xs">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="easy">Easy ×0.85</SelectItem>
                <SelectItem value="medium">Medium ×1</SelectItem>
                <SelectItem value="hard">Hard ×1.2</SelectItem>
              </SelectContent>
            </Select>
          </div>
          <div className="grid gap-0.5">
            <Label className="text-[10px] text-muted-foreground">Rounds</Label>
            <Select
              value={String(totalRounds)}
              onValueChange={(v) => setTotalRounds(Number(v))}
              disabled={running}
            >
              <SelectTrigger className="h-8 w-[72px] text-xs">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {[5, 10, 15, 20].map((n) => (
                  <SelectItem key={n} value={String(n)}>
                    {n}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
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
      </div>

      <div className="min-h-0 flex-1 overflow-auto">
        {!running && !gameOver && (
          <div className="rounded-lg border border-border bg-muted/40 p-6 text-center text-muted-foreground">
            <p className="text-sm">
              Build the correct SIP dialog by tapping messages in order.
            </p>
            <p className="mt-2 text-[11px]">
              Per-round timer · speed bonus for fast clears · penalties for wrong picks · 10 scenarios
              (Register+401, Re-INVITE hold, Redirect 302, CANCEL flow, and more).
            </p>
          </div>
        )}

        {running && seq && (
          <div className="relative space-y-3">
            {paused && (
              <div className="absolute inset-0 z-10 flex items-center justify-center rounded-lg bg-background/50 backdrop-blur-[1px]">
                <span className="rounded-md border border-border bg-card px-4 py-2 text-center text-sm font-semibold shadow-sm">
                  <span className="block">Paused</span>
                  <span className="mt-1 block text-xs font-normal tabular-nums text-muted-foreground">
                    Score {score}
                    {highScore > 0 ? ` · Record ${highScore}` : ''}
                  </span>
                </span>
              </div>
            )}
            <div className="rounded-lg border border-border bg-muted/30 p-3">
              <div className="mb-1 flex items-center justify-between gap-2">
                <span className="text-sm font-semibold">{seq.name}</span>
                <span
                  className="rounded px-2 py-0.5 text-[10px] font-semibold"
                  style={{
                    background: `${DIFFICULTY_COLOR[seq.difficulty]}22`,
                    color: DIFFICULTY_COLOR[seq.difficulty],
                  }}
                >
                  {seq.difficulty}
                </span>
              </div>
              <p className="mb-2 text-[11px] text-muted-foreground">{seq.description}</p>
              <div className="mb-2 h-1 overflow-hidden rounded-full bg-border">
                <div
                  className="h-full rounded-full bg-emerald-600 transition-[width] duration-200 dark:bg-emerald-500"
                  style={{ width: `${total ? (completed / total) * 100 : 0}%` }}
                />
              </div>
              <div className="flex flex-wrap gap-1">
                {seq.steps.map((s, i) => (
                  <SipStep
                    key={`${s}-${i}`}
                    label={s}
                    state={
                      i < stepIdx ? 'done' : i === stepIdx && !betweenRounds ? 'active' : 'pending'
                    }
                  />
                ))}
              </div>
            </div>

            {!betweenRounds ? (
              <>
                <p className="text-center text-[11px] text-muted-foreground">
                  Step {stepIdx + 1} of {total} — what&apos;s next? · round pool bonus:{' '}
                  <b className="text-foreground">{stepBonus}</b> + speed
                </p>
                <div className="grid grid-cols-2 gap-2">
                  {options.map((opt) => {
                    const isWrong = wrongFlash === opt
                    const isCorrect = correctFlash === opt
                    return (
                      <Button
                        key={opt}
                        type="button"
                        variant="outline"
                        disabled={paused}
                        className={cn(
                          'h-auto min-h-[44px] justify-start whitespace-normal py-2 text-left font-mono text-xs font-medium',
                          isWrong && 'border-destructive bg-destructive/10 text-destructive',
                          isCorrect && 'border-emerald-600 bg-emerald-500/10 text-emerald-800 dark:text-emerald-300',
                        )}
                        onClick={() => handleChoice(opt)}
                      >
                        {opt}
                      </Button>
                    )
                  })}
                </div>
              </>
            ) : (
              <p className="py-4 text-center text-sm text-muted-foreground">Round complete — next…</p>
            )}
          </div>
        )}

        {history.length > 0 && (
          <div className="mt-3 border-t border-border/60 pt-2">
            <div className="mb-1 text-[10px] font-semibold uppercase tracking-wide text-muted-foreground">
              Round history ({history.length})
            </div>
            <ScrollArea className="max-h-[200px] pr-2">
              <div className="flex flex-col gap-1">
                {[...history].reverse().map((r, i) => (
                  <div
                    key={`${history.length - 1 - i}-${r.seqName}-${r.timeMs}`}
                    className="flex flex-wrap items-center justify-between gap-2 rounded border border-border bg-card/60 px-2 py-1.5 text-[11px]"
                  >
                    <span className="font-medium">{r.seqName}</span>
                    <span className="text-muted-foreground">
                      {r.correct}/{r.total} ·{' '}
                      <b className={r.bonusScore > 0 ? 'text-emerald-600 dark:text-emerald-400' : ''}>
                        +{r.bonusScore}
                      </b>{' '}
                      · {(r.timeMs / 1000).toFixed(1)}s
                    </span>
                  </div>
                ))}
              </div>
            </ScrollArea>
          </div>
        )}

        {gameOver && (
          <div className="mt-3 rounded-lg border border-border bg-muted/40 p-5 text-center">
            <div className="text-base font-semibold">Session complete</div>
            <div className="mt-1 text-sm text-muted-foreground">
              Score: <b className="text-lg tabular-nums text-foreground">{score}</b>
              {highScore > 0 ? (
                <span>
                  {' '}
                  · Record: <b className="tabular-nums text-foreground">{highScore}</b>
                </span>
              ) : null}
            </div>
            <div className="mt-1 text-[11px] text-muted-foreground">
              Best streak: {bestStreak} · rounds: {history.length}
            </div>
            <div className="mt-1 text-[11px] text-muted-foreground">
              Avg {Math.round(score / Math.max(history.length, 1))} pts / round (from history)
            </div>
            <Button type="button" className="mt-3" size="sm" onClick={startGame}>
              Play again
            </Button>
          </div>
        )}
      </div>
    </div>
  )
}

function HudVal({ label, val, className }: { label: string; val: string; className?: string }) {
  return (
    <div>
      <div className="text-[10px] text-muted-foreground">{label}</div>
      <div className={cn('text-base font-semibold', className)}>{val}</div>
    </div>
  )
}

function SipStep({ label, state }: { label: string; state: 'done' | 'active' | 'pending' }) {
  return (
    <span
      className={cn(
        'rounded border px-2 py-0.5 text-[10px] font-medium',
        state === 'done' && 'border-emerald-600 bg-emerald-500/15 text-emerald-800 dark:text-emerald-300',
        state === 'active' &&
          'border-primary bg-primary/10 text-primary animate-pulse dark:text-primary',
        state === 'pending' && 'border-border bg-muted/50 text-muted-foreground',
      )}
    >
      {label}
    </span>
  )
}
