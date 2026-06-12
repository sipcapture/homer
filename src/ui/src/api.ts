import {
  AUTH_TOKEN_KEY,
  clearAuthToken,
  getAuthToken,
  isCookieSessionMarker,
} from './lib/authTokenStorage'

const apiCredentials: RequestCredentials = 'include'

const apiBase: string = import.meta.env.VITE_API_BASE || '/api/v4'
const tokenKey = AUTH_TOKEN_KEY

export type QueryParams = Record<string, string | number | boolean | null | undefined>

export interface ApiErrorBody {
  error?: {
    title?: string
    detail?: string
  }
}

function getToken(): string {
  return getAuthToken()
}

function authHeaders(): Record<string, string> {
  const token = getToken()
  if (!token || isCookieSessionMarker(token)) return {}
  return { Authorization: `Bearer ${token}` }
}

/** Clear token and notify App to show login. Call on 401 from any API. */
export function handleUnauthorized(): void {
  clearAuthToken()
  window.dispatchEvent(new CustomEvent('auth:unauthorized'))
}

async function parseError(res: Response): Promise<string> {
  let detail = `Request failed (${res.status})`
  try {
    const payload = (await res.json()) as ApiErrorBody
    if (payload?.error?.detail) {
      detail = payload.error.detail
    } else if (payload?.error?.title) {
      detail = payload.error.title
    }
  } catch {
    // ignore
  }
  return detail
}

export async function apiGet<T = any>(path: string, params?: QueryParams): Promise<T> {
  const url = new URL(`${apiBase}${path}`, window.location.origin)
  if (params) {
    for (const [key, value] of Object.entries(params)) {
      if (value !== undefined && value !== null && value !== '') {
        url.searchParams.append(key, String(value))
      }
    }
  }
  const res = await fetch(url.toString(), { headers: authHeaders(), credentials: apiCredentials })
  if (res.status === 401) {
    handleUnauthorized()
    throw new Error('Unauthorized')
  }
  if (!res.ok) {
    throw new Error(await parseError(res))
  }
  return res.json() as Promise<T>
}

/** Fetch SSO profile photo via coordinator proxy (requires Authorization). */
export async function fetchMeAvatarObjectUrl(): Promise<string | null> {
  const res = await fetch(`${apiBase}/me/avatar`, { headers: authHeaders(), credentials: apiCredentials })
  if (res.status === 401) {
    handleUnauthorized()
    throw new Error('Unauthorized')
  }
  if (res.status === 404) {
    return null
  }
  if (!res.ok) {
    throw new Error(await parseError(res))
  }
  const blob = await res.blob()
  if (!blob.size) {
    return null
  }
  return URL.createObjectURL(blob)
}

export async function apiPost<T = unknown>(path: string, body?: unknown): Promise<T | null> {
  const res = await fetch(`${apiBase}${path}`, {
    method: 'POST',
    headers: { ...authHeaders(), 'Content-Type': 'application/json' },
    credentials: apiCredentials,
    body: JSON.stringify(body),
  })
  if (res.status === 401) {
    handleUnauthorized()
    throw new Error('Unauthorized')
  }
  if (!res.ok) {
    throw new Error(await parseError(res))
  }
  if (res.status === 204) return null
  return res.json() as Promise<T>
}

export async function apiPut<T = unknown>(path: string, body?: unknown): Promise<T | null> {
  const res = await fetch(`${apiBase}${path}`, {
    method: 'PUT',
    headers: { ...authHeaders(), 'Content-Type': 'application/json' },
    credentials: apiCredentials,
    body: JSON.stringify(body),
  })
  if (res.status === 401) {
    handleUnauthorized()
    throw new Error('Unauthorized')
  }
  if (!res.ok) {
    throw new Error(await parseError(res))
  }
  if (res.status === 204) return null
  return res.json() as Promise<T>
}

export async function apiPatch<T = unknown>(path: string, body?: unknown): Promise<T | null> {
  const res = await fetch(`${apiBase}${path}`, {
    method: 'PATCH',
    headers: { ...authHeaders(), 'Content-Type': 'application/json' },
    credentials: apiCredentials,
    body: JSON.stringify(body),
  })
  if (res.status === 401) {
    handleUnauthorized()
    throw new Error('Unauthorized')
  }
  if (!res.ok) {
    throw new Error(await parseError(res))
  }
  if (res.status === 204) return null
  return res.json() as Promise<T>
}

export async function apiDelete(path: string): Promise<null> {
  const res = await fetch(`${apiBase}${path}`, {
    method: 'DELETE',
    headers: authHeaders(),
    credentials: apiCredentials,
  })
  if (res.status === 401) {
    handleUnauthorized()
    throw new Error('Unauthorized')
  }
  if (!res.ok) {
    throw new Error(await parseError(res))
  }
  return null
}

export async function apiPostFile<T = unknown>(path: string, file: File): Promise<T> {
  const formData = new FormData()
  formData.append('file', file)
  const res = await fetch(`${apiBase}${path}`, {
    method: 'POST',
    headers: authHeaders(),
    credentials: apiCredentials,
    body: formData,
  })
  if (res.status === 401) {
    handleUnauthorized()
    throw new Error('Unauthorized')
  }
  if (!res.ok) {
    throw new Error(await parseError(res))
  }
  return res.json() as Promise<T>
}

export function apiDownloadUrl(path: string): string {
  return `${apiBase}${path}`
}

/**
 * Build a WebSocket URL rooted at the same origin as the REST API.
 * When a Bearer JWT is stored (Remember me), it is appended as
 * `?access_token=...` because browsers can't attach Authorization headers
 * to a WS handshake. With HttpOnly cookie auth the cookie is sent on the
 * handshake automatically. Extra query params
 * (e.g. `?proto=1`, `?method=INVITE`) are merged on top of the caller's.
 */
export function buildWsURL(path: string, params?: QueryParams): string {
  const base = new URL(apiBase, window.location.origin)
  const scheme = base.protocol === 'https:' ? 'wss:' : 'ws:'
  const u = new URL(`${apiBase}${path}`, window.location.origin)
  u.protocol = scheme
  const token = getToken()
  if (token && !isCookieSessionMarker(token)) u.searchParams.set('access_token', token)
  if (params) {
    for (const [k, v] of Object.entries(params)) {
      if (v === undefined || v === null || v === '') continue
      if (Array.isArray(v)) {
        for (const item of v) u.searchParams.append(k, String(item))
      } else {
        u.searchParams.append(k, String(v))
      }
    }
  }
  return u.toString()
}

export interface HepStreamEvent {
  ts: number
  proto: number
  src_ip?: string
  src_port?: number
  dst_ip?: string
  dst_port?: number
  node_id?: number
  sip?: {
    method?: string
    /** 3-digit SIP status code as string ("200"). Empty / missing for requests. */
    resp_code?: string
    resp_text?: string
    callid?: string
    cseq?: string
    from_user?: string
    to_user?: string
    ruri_user?: string
  }
  /** Present only when the subscriber is admin and requested include_payload=1. */
  payload?: string
}

export interface HepStreamOptions {
  /** Filter events by HEP protocol id (1 = SIP). Multiple accepted. */
  proto?: number | number[]
  /** Filter by SIP method (INVITE, BYE, REGISTER, ...). */
  method?: string | string[]
  /** Only deliver requests (drop responses). */
  onlyRequests?: boolean
  /** Request raw payload. Requires an admin JWT on the server side. */
  includePayload?: boolean
  /** Replay last N buffered events at connection time. 0 = none. */
  history?: number
}

export interface HepStreamClient {
  /** Stop retrying and close the underlying socket. */
  close: () => void
  /** Current socket state for UI indicators. */
  readonly state: () => 'connecting' | 'open' | 'closed'
}

/**
 * Thick WebSocket client for `/api/v4/stream/hep` with automatic
 * reconnect + exponential backoff (capped at 30s). Events are
 * JSON-decoded before reaching `onEvent`; decode errors and socket
 * errors are surfaced via `onError` but do not stop the client.
 *
 * Reconnection is intentional: nodes restart, coordinators restart,
 * JWTs expire. The caller controls lifetime via `close()`.
 */
export function openHepStream(
  opts: HepStreamOptions,
  handlers: {
    onEvent: (e: HepStreamEvent) => void
    onOpen?: () => void
    onClose?: (ev: CloseEvent) => void
    onError?: (err: unknown) => void
  },
): HepStreamClient {
  let ws: WebSocket | null = null
  let closedByUser = false
  let attempt = 0
  let reconnectTimer: number | null = null
  let currentState: 'connecting' | 'open' | 'closed' = 'closed'

  const params: QueryParams = {}
  if (opts.proto !== undefined) params.proto = Array.isArray(opts.proto) ? (opts.proto as any) : opts.proto
  if (opts.method !== undefined) params.method = Array.isArray(opts.method) ? (opts.method as any) : opts.method
  if (opts.onlyRequests) params.only_requests = 1
  if (opts.includePayload) params.include_payload = 1
  if (opts.history && opts.history > 0) params.history = opts.history

  const connect = () => {
    if (closedByUser) return
    currentState = 'connecting'
    const url = buildWsURL('/stream/hep', params)
    let sock: WebSocket
    try {
      sock = new WebSocket(url)
    } catch (err) {
      handlers.onError?.(err)
      scheduleReconnect()
      return
    }
    ws = sock
    sock.onopen = () => {
      attempt = 0
      currentState = 'open'
      handlers.onOpen?.()
    }
    sock.onmessage = (ev) => {
      try {
        const parsed = JSON.parse(String(ev.data)) as HepStreamEvent
        handlers.onEvent(parsed)
      } catch (err) {
        handlers.onError?.(err)
      }
    }
    sock.onerror = (err) => {
      handlers.onError?.(err)
    }
    sock.onclose = (ev) => {
      currentState = 'closed'
      handlers.onClose?.(ev)
      if (!closedByUser) scheduleReconnect()
    }
  }

  const scheduleReconnect = () => {
    if (closedByUser) return
    attempt = Math.min(attempt + 1, 8)
    const backoffMs = Math.min(30000, 500 * 2 ** (attempt - 1))
    const jitter = Math.random() * 250
    reconnectTimer = window.setTimeout(connect, backoffMs + jitter)
  }

  connect()

  return {
    close: () => {
      closedByUser = true
      if (reconnectTimer !== null) {
        window.clearTimeout(reconnectTimer)
        reconnectTimer = null
      }
      if (ws) {
        try {
          ws.close(1000, 'client closed')
        } catch {
          // ignore
        }
        ws = null
      }
      currentState = 'closed'
    },
    state: () => currentState,
  }
}

/**
 * Single envelope used by the Netris (PvP SIPetris) WebSocket. The
 * server side mirror lives in `src/coordinator/games/netris/protocol.go`
 * — keep field names in sync. Unknown fields on either side are
 * tolerated, so adding new optional fields does not require a
 * lock-step rollout.
 */
export interface NetrisMessage {
  type: string
  display_name?: string
  room?: string
  you?: string
  opponent?: string
  from?: string
  text?: string
  cleared?: number
  lines?: number
  hole?: number
  cells?: string
  score?: number
  level?: number
  reason?: string
  message?: string
}

export type NetrisSocketState = 'connecting' | 'open' | 'closed'

export interface NetrisSocketOptions {
  /** Named room code. Mutually exclusive with `quick`. */
  room?: string
  /** Auto-pair with the next unmatched player. */
  quick?: boolean
  /** Optional display label override (≤ 32 chars after sanitisation). */
  display?: string
}

export interface NetrisSocket {
  /** Send one envelope to the server. Drops silently while disconnected. */
  send: (msg: NetrisMessage) => void
  close: () => void
  readonly state: () => NetrisSocketState
}

/**
 * Thick WebSocket client for `/api/v4/games/netris` with automatic
 * reconnect + exponential backoff (capped at 30s). Mirrors
 * `openHepStream`'s shape; messages are JSON-encoded envelopes
 * matching `netris.Envelope` on the server.
 *
 * Reconnection is intentional: coordinators restart, JWTs expire.
 * The caller controls lifetime via `close()`. After a reconnect the
 * client must re-handshake (server has no cross-socket session
 * memory beyond an in-flight room), so the consumer should re-send
 * `hello`/`ready` from `onOpen`.
 */
export function openNetrisSocket(
  opts: NetrisSocketOptions,
  handlers: {
    onMessage: (msg: NetrisMessage) => void
    onOpen?: () => void
    onClose?: (ev: CloseEvent) => void
    onError?: (err: unknown) => void
  },
): NetrisSocket {
  let ws: WebSocket | null = null
  let closedByUser = false
  let attempt = 0
  let reconnectTimer: number | null = null
  let currentState: NetrisSocketState = 'closed'

  const params: QueryParams = {}
  if (opts.room) params.room = opts.room
  if (opts.quick) params.mode = 'quick'
  if (opts.display) params.display = opts.display

  const connect = () => {
    if (closedByUser) return
    currentState = 'connecting'
    const url = buildWsURL('/games/netris', params)
    let sock: WebSocket
    try {
      sock = new WebSocket(url)
    } catch (err) {
      handlers.onError?.(err)
      scheduleReconnect()
      return
    }
    ws = sock
    sock.onopen = () => {
      attempt = 0
      currentState = 'open'
      handlers.onOpen?.()
    }
    sock.onmessage = (ev) => {
      try {
        const parsed = JSON.parse(String(ev.data)) as NetrisMessage
        handlers.onMessage(parsed)
      } catch (err) {
        handlers.onError?.(err)
      }
    }
    sock.onerror = (err) => {
      handlers.onError?.(err)
    }
    sock.onclose = (ev) => {
      currentState = 'closed'
      handlers.onClose?.(ev)
      if (!closedByUser) scheduleReconnect()
    }
  }

  const scheduleReconnect = () => {
    if (closedByUser) return
    attempt = Math.min(attempt + 1, 8)
    const backoffMs = Math.min(30000, 500 * 2 ** (attempt - 1))
    const jitter = Math.random() * 250
    reconnectTimer = window.setTimeout(connect, backoffMs + jitter)
  }

  connect()

  return {
    send: (msg) => {
      if (!ws || ws.readyState !== WebSocket.OPEN) return
      try {
        ws.send(JSON.stringify(msg))
      } catch (err) {
        handlers.onError?.(err)
      }
    },
    close: () => {
      closedByUser = true
      if (reconnectTimer !== null) {
        window.clearTimeout(reconnectTimer)
        reconnectTimer = null
      }
      if (ws) {
        try {
          ws.close(1000, 'client closed')
        } catch {
          // ignore
        }
        ws = null
      }
      currentState = 'closed'
    },
    state: () => currentState,
  }
}

// ---------------------------------------------------------------------------
// NetChess (PvP) — WebSocket envelope + thin client
// ---------------------------------------------------------------------------

/**
 * Single envelope for the NetChess (PvP Chess) WebSocket. The server
 * mirror lives in `src/coordinator/games/netchess/protocol.go` — keep
 * field names in sync. Unknown fields are tolerated on both sides.
 */
export interface NetChessMessage {
  type: string
  display_name?: string
  room?: string
  you?: string
  opponent?: string
  /** Your colour (only set on `matched`). */
  color?: 'white' | 'black'
  spectator?: boolean

  /** Time control (set on `matched`). */
  initial_ms?: number
  increment_ms?: number

  /** Move / opponent_move / clock_sync */
  uci?: string
  san?: string
  fen?: string
  clock_ms?: number
  white_ms?: number
  black_ms?: number

  /** game_over */
  result?: '1-0' | '0-1' | '1/2-1/2'
  reason?: string

  /** chat */
  from?: string
  text?: string

  /** error */
  message?: string
}

export type NetChessSocketState = 'connecting' | 'open' | 'closed'

export interface NetChessSocketOptions {
  /** Named room code. Mutually exclusive with `quick`. */
  room?: string
  /** Auto-pair with the next unmatched player. */
  quick?: boolean
  /** Join `room` read-only as a spectator. */
  spectate?: boolean
  /** Colour preference for new rooms. Default random. */
  color?: 'white' | 'black' | 'random'
  /** Time control overrides (milliseconds). */
  initialMs?: number
  incrementMs?: number
  /** Optional display label override. */
  display?: string
}

export interface NetChessSocket {
  send: (msg: NetChessMessage) => void
  close: () => void
  readonly state: () => NetChessSocketState
}

/**
 * Thin WebSocket client for `/api/v4/games/netchess`. Same reconnect
 * / backoff strategy as `openNetrisSocket`; after reconnects the
 * caller should re-emit `hello` / `ready` since the server keeps no
 * cross-socket session state beyond an in-flight room.
 */
export function openNetChessSocket(
  opts: NetChessSocketOptions,
  handlers: {
    onMessage: (msg: NetChessMessage) => void
    onOpen?: () => void
    onClose?: (ev: CloseEvent) => void
    onError?: (err: unknown) => void
  },
): NetChessSocket {
  let ws: WebSocket | null = null
  let closedByUser = false
  let attempt = 0
  let reconnectTimer: number | null = null
  let currentState: NetChessSocketState = 'closed'

  const params: QueryParams = {}
  if (opts.room) params.room = opts.room
  if (opts.quick) params.mode = 'quick'
  if (opts.spectate) params.spectate = 'true'
  if (opts.color) params.color = opts.color
  if (opts.initialMs !== undefined) params.initial_ms = String(opts.initialMs)
  if (opts.incrementMs !== undefined) params.increment_ms = String(opts.incrementMs)
  if (opts.display) params.display = opts.display

  const connect = () => {
    if (closedByUser) return
    currentState = 'connecting'
    const url = buildWsURL('/games/netchess', params)
    let sock: WebSocket
    try {
      sock = new WebSocket(url)
    } catch (err) {
      handlers.onError?.(err)
      scheduleReconnect()
      return
    }
    ws = sock
    sock.onopen = () => {
      attempt = 0
      currentState = 'open'
      handlers.onOpen?.()
    }
    sock.onmessage = (ev) => {
      try {
        const parsed = JSON.parse(String(ev.data)) as NetChessMessage
        handlers.onMessage(parsed)
      } catch (err) {
        handlers.onError?.(err)
      }
    }
    sock.onerror = (err) => {
      handlers.onError?.(err)
    }
    sock.onclose = (ev) => {
      currentState = 'closed'
      handlers.onClose?.(ev)
      if (!closedByUser) scheduleReconnect()
    }
  }

  const scheduleReconnect = () => {
    if (closedByUser) return
    attempt = Math.min(attempt + 1, 8)
    const backoffMs = Math.min(30000, 500 * 2 ** (attempt - 1))
    const jitter = Math.random() * 250
    reconnectTimer = window.setTimeout(connect, backoffMs + jitter)
  }

  connect()

  return {
    send: (msg) => {
      if (!ws || ws.readyState !== WebSocket.OPEN) return
      try {
        ws.send(JSON.stringify(msg))
      } catch (err) {
        handlers.onError?.(err)
      }
    },
    close: () => {
      closedByUser = true
      if (reconnectTimer !== null) {
        window.clearTimeout(reconnectTimer)
        reconnectTimer = null
      }
      if (ws) {
        try { ws.close(1000, 'client closed') } catch { /* ignore */ }
        ws = null
      }
      currentState = 'closed'
    },
    state: () => currentState,
  }
}

// ---------------------------------------------------------------------------
// Chess widget — LLM opponent endpoints
// ---------------------------------------------------------------------------

/** Status of the optional LLM-opponent mode for the single-player Chess
 *  widget. Mirrors the server-side `chessLLMStatusResponse`. The Chess
 *  widget polls this on mount to decide whether to render the LLM
 *  toggle. `enabled=false` keeps the widget in bot-only mode. */
export interface ChessLLMStatus {
  enabled: boolean
  model?: string
}

export async function fetchChessLLMStatus(): Promise<ChessLLMStatus> {
  try {
    return await apiGet<ChessLLMStatus>('/api/v4/games/chess/llm-status')
  } catch {
    // Endpoint absent or unauthorized — treat as disabled so the
    // widget gracefully falls back to bot-only mode.
    return { enabled: false }
  }
}

/** Request body for `/api/v4/games/chess/llm-move`. */
export interface ChessLLMMoveRequest {
  fen: string
  history_pgn?: string
  level?: number
}

/** Response shape for `/api/v4/games/chess/llm-move`. `uci` is empty
 *  for terminal positions; otherwise it is always a legal move. */
export interface ChessLLMMoveResponse {
  uci: string
  source: 'llm' | 'fallback'
  model?: string
  latency_ms: number
  reason?: string
}

export async function postChessLLMMove(req: ChessLLMMoveRequest): Promise<ChessLLMMoveResponse> {
  const res = await apiPost<ChessLLMMoveResponse>('/api/v4/games/chess/llm-move', req)
  if (!res) throw new Error('chess llm-move returned no body')
  return res
}

export { apiBase, tokenKey, getToken, authHeaders, parseError }
