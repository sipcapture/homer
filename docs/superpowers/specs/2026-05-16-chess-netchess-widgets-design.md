# Chess + NetChess dashboard widgets — design

Date: 2026-05-16
Status: approved
Owner: Alexandr Dubovikov

## Goal

Add two new dashboard widgets to Homer under the existing **Games** category:

1. **`chess`** — single-player chess. Play against a built-in minimax bot, or — when the operator has enabled the MCP LLM backend — against an LLM opponent served through the existing `src/mcp/llm.go` client.
2. **`netchess`** — two-player network chess. Architectural mirror of `netris`: lobby + WebSocket relay through the coordinator, but with the **server as the authoritative game state** (move validation, mate/draw detection, clocks).

Both widgets share one chess core (`chess.js` on the UI, `notnil/chess` on the server) and one presentational board component (`ChessBoard.tsx`).

## Non-goals (this iteration)

- Stockfish or other native engine integration. The built-in minimax is good enough for a dashboard widget; LLM-mode covers the "stronger / more interesting opponent" use case.
- Engine SVG asset sets (Cburnett etc). Unicode glyphs are good enough at widget cell sizes and avoid an asset pipeline.
- Tournament / Elo / persistent rankings. Local best-score per browser is enough.
- Variant chess (Chess960, Atomic, Antichess).

## Architecture

```
src/ui/src/dashboard/widgets/
  chessCore.ts                # thin chess.js wrapper: typed Move/Square, FEN/PGN helpers, status()
  chessEngine.ts              # synchronous minimax + alpha-beta, depth 1..4
  chessEngine.worker.ts       # Web Worker around chessEngine
  ChessBoard.tsx              # presentational 8x8 board (Tailwind), click/drag-to-move,
                              # promotion menu, flip, check/last-move highlights
  ChessPanel.tsx              # single-player widget (Bot | LLM toggle)
  NetChessPanel.tsx           # PvP widget over WebSocket
  registry.ts                 # + chess + netchess entries

src/ui/src/api.ts
  openNetChessSocket(opts)    # WS client mirroring openNetrisSocket
  postChessLLMMove(body)      # POST /api/v4/games/chess/llm-move
  fetchChessLLMStatus()       # GET  /api/v4/games/chess/llm-status

src/coordinator/games/netchess/
  protocol.go                 # Envelope: hello/ready/move/resign/draw_*/takeback/chat +
                              # matched/start/opponent_move/clock_sync/game_over/draw_offered/...
  hub.go                      # Hub + Room with notnil/chess game, clocks, spectator list
  hub_test.go                 # matchmaking, legal/illegal moves, mate/stalemate, resign,
                              # draw offer/accept, takeback, clock decrement, flag timeout,
                              # spectator broadcast

src/coordinator/handlers/games_v4.go
  V4NetChess          (WS)    # /api/v4/games/netchess
  V4ChessLLMMove      (POST)  # /api/v4/games/chess/llm-move
  V4ChessLLMStatus    (GET)   # /api/v4/games/chess/llm-status (returns {enabled})

src/coordinator/coordinator.go
  + games/netchess import
  + gamesHandler.SetNetChessHub(netchess.NewHub(netchess.Config{}))
  + gamesHandler.SetMCPClient(mcpClient)        // for chess LLM mode
  + routes: GET  /games/netchess
            POST /games/chess/llm-move
            GET  /games/chess/llm-status
```

### Key choice: server is authoritative for NetChess

Netris is a thin relay: each client crunches its own board and the server only translates `line_clear` → `garbage_in`. For NetChess we flip the model — the server holds the canonical `*chess.Game` (from `github.com/notnil/chess`) and validates every move. Clients render and submit UCI strings. This:

- Eliminates the entire class of "we drifted out of sync" bugs.
- Blocks the trivial cheat of clients sending illegal moves.
- Lets the server own clocks (decrement on its own goroutine, flag with `time.AfterFunc`) so a paused/throttled tab can't gain time.

The trade-off — one extra round-trip of latency per move — is invisible at the resolution chess is played in a browser widget.

### Shared logic, no duplication

`chessCore.ts` is the only place that knows about chess rules on the UI side. Both widgets and the engine worker import from it. The Go side uses `notnil/chess` directly; the wire format between them is FEN + UCI, both of which are standards, so there is no schema duplication.

## Data flow

### Single-player Chess (Bot mode)

```
ChessPanel  --tryMove(uci)-->  chessCore  --ok-->  setState(fen, history)
ChessPanel  --postMessage({fen,depth})-->  chessEngine.worker
                                    <--{uci,evalCp}-- worker
ChessPanel  --tryMove(uci, by:bot)-->  chessCore  -->  setState
```

### Single-player Chess (LLM mode)

```
ChessPanel  --tryMove(uci)-->  setState
ChessPanel  --POST /games/chess/llm-move {fen, history_pgn, level}-->  coordinator
coordinator  --mcp/llm.go: chat-completion w/ FEN+PGN+system_prompt-->  LLM
LLM  --"e2e4"--> coordinator  --validate via notnil/chess--> 
   ok: respond {uci, source:"llm", latency_ms}
   bad: run server-side minimax fallback, respond {uci, source:"fallback", latency_ms}
```

### PvP NetChess

```
Player A         coordinator hub               Player B           Spectators
   |  hello, ready  |                              |                  |
   |--------------->|  matched, start              |                  |
   |                |----------------------------->|                  |
   |  move e2e4     |                              |                  |
   |--------------->|  validate; update game,clocks|                  |
   |                |  opponent_move{san,fen,clks} |                  |
   |                |----------------------------->|----------------->|
   |                |                              |                  |
   ...
   |                |  game_over{checkmate, 1-0}   |                  |
   |<---------------|----------------------------->|----------------->|
```

## Wire protocol — NetChess Envelope

Single JSON envelope, unknown fields tolerated on both sides (same convention as Netris).

| Field | Type | Sent in | Notes |
|---|---|---|---|
| `type` | string | every | see message table |
| `display_name` | string | hello | optional override |
| `room` | string | hello, matched | empty → quick-match |
| `you`, `opponent` | string | matched | usernames |
| `color` | "white"\|"black" | matched | assigned to you |
| `time_control` | `{initial_ms, increment_ms}` | matched | from URL query (default 600000+5000) |
| `uci` | string | move, opponent_move | "e2e4" / "e7e8q" |
| `san` | string | opponent_move | for history rendering on the receiver |
| `fen` | string | opponent_move, clock_sync | post-move position |
| `white_ms`, `black_ms` | int | opponent_move, clock_sync | remaining clock |
| `result` | "1-0"\|"0-1"\|"1/2-1/2" | game_over | |
| `reason` | string | game_over | checkmate/stalemate/resignation/flag/agreement/insufficient/50_move/3fold |
| `from` | string | chat | username |
| `text` | string | chat | sanitised to printable, ≤ 200 runes |
| `message` | string | error | human-readable |

Client→server: `hello`, `ready`, `move`, `resign`, `draw_offer`, `draw_accept`, `draw_decline`, `takeback_request`, `takeback_accept`, `takeback_decline`, `chat`.

Server→client: `matched`, `start`, `opponent_move`, `clock_sync`, `game_over`, `draw_offered`, `takeback_offered`, `opponent_left`, `waiting_timeout`, `error`.

## URL & auth

- `GET /api/v4/games/netchess` — same JWT middleware as `/games/netris`.
- Query params: `?quick=true` OR `?room=CODE`, optional `?color=white|black|random` (default `random`), optional `?time_control=600000+5000` (initial_ms+increment_ms), optional `?spectate=true&room=CODE`.

## Authoritative state on the server

Per `Room`:
- `game *chess.Game` — notnil/chess. Source of truth.
- `players [2]*Player` — index 0 = white, index 1 = black.
- `spectators []*Spectator` — read-only listeners, cap 8.
- `clocks { whiteMs, blackMs int64; lastMoveAt time.Time; increment time.Duration }`.
- `flagTimer *time.Timer` — restarted on every move.
- `mu sync.Mutex` — protects everything above; moves serialise through it.

`timeNow func() time.Time` is injected into the hub so `hub_test.go` can drive clocks deterministically.

## Single-player widget specifics

### Layout (top bar → mirrors Netris/SIPetris):
- Eval bar (replacing the Score card): mapped from engine cp to [-3..+3] for the visual.
- `New Game`, `Resign`, `Takeback`, `Export PGN`.
- Side selector: `White / Black / Random`.
- Time control: `Untimed / Bullet 1+0 / Blitz 3+2 / Rapid 10+5`.
- Mode toggle: `Bot` / `LLM (MCP)` — the latter shown only if `fetchChessLLMStatus()` reports `{enabled: true}`.
- Level slider (1..4) — only meaningful for `Bot` and `fallback` paths.

### Sidebar:
- PGN move list (click jumps to position, "Back to live" returns).
- "Last events" log (mate, check, capture, takeback, mode switches).

### Persistence:
- `localStorage['homer_chess_pgn']` — full PGN, restored on widget mount.
- `localStorage['homer_chess_best_*']` — per-mode best (longest mate-avoided, fastest mate-delivered) — secondary, optional.

## PvP widget specifics

### Lobby (when `conn === 'idle' | 'closed'`):
- `Quick` / `Room` mode (mirrors Netris).
- Time control dropdown (same presets as single-player).
- Color preference `White / Black / Random`.
- Spectate-by-room-code shortcut.

### In-game UI:
- Two clocks (top of own board, top of opponent board).
- PGN list (click = local review only — does not break the live stream).
- Action buttons: `Resign`, `Offer draw`, `Request takeback`.
- Status pill (`Waiting`, `Matched`, `Your move`, `Opponent thinking`, `Won`, `Lost`, `Draw`).

### Spectator mode:
- No interaction. Board flipped automatically based on `?as=white|black`, defaults to white-at-bottom.

## Error handling

| Error | UI response | Server response |
|---|---|---|
| Illegal client move | toast "illegal move", revert board to last server FEN | `error{message:"illegal move"}` and stay in current position |
| LLM returns invalid UCI | UI never sees raw error; receives `{source:"fallback"}` | logs `mcp_chess_invalid_move` warning, runs minimax |
| LLM timeout (≥ 8s) | spinner clears, fallback used | same |
| Opponent disconnects | banner "Opponent disconnected — game saved as draw" if mid-game (server-config), else "Opponent left" in lobby | `game_over{result:"1/2-1/2", reason:"opponent_disconnect"}` after a 30s reconnect window |
| Clock flag | `game_over{flag}` | server-side `time.AfterFunc` fires |
| Bad WS frame | client drops, server logs | server closes with `error{message:"protocol violation"}` |

## Testing

### UI (Vitest):
- `chessCore.test.ts` — legal vs illegal moves, FEN round-trip, PGN export/import, mate/stalemate/3fold detection.
- `chessEngine.test.ts` — mate-in-1 and mate-in-2 known positions, never loses material to a 1-ply tactic, prefers center early.
- `ChessBoard.test.tsx` — render, click-to-move, promotion menu, flipped orientation, last-move/check highlights.
- `ChessPanel.test.tsx` — bot answers, LLM call wired (mocked fetch), takeback rewinds 2 plies in bot mode, end-of-game overlay.
- `NetChessPanel.test.tsx` — reacts to server frames (mock WS): matched, start, opponent_move, draw_offered, game_over; resign sends correct frame.

### Server (Go):
- `netchess/hub_test.go` — legal/illegal move via hub, mate→game_over, resign, draw_offer + accept, takeback_request + accept, clock decrement, flag timeout (with injected `timeNow`), spectator broadcast (only receives, can't send).
- `netchess/protocol_test.go` — Encode/Decode round-trip, SanitiseDisplayName edge cases.
- `handlers/games_v4_chess_llm_test.go` — `/games/chess/llm-move` happy path (mocked LLM client) + invalid-UCI fallback.

## Risks & mitigations

| Risk | Mitigation |
|---|---|
| LLM returns garbage or illegal UCI | Server validates with notnil/chess; on failure, runs minimax fallback and tags response `source:"fallback"`. UI shows tiny indicator. |
| Web Worker not available | Detect at mount, fall back to synchronous engine at depth ≤ 2. |
| chess.js bundle size (~50KB gzipped) | Widget lazy-loaded via `lazy()` already; doesn't touch the initial bundle. |
| Server load from clock timers | `time.AfterFunc` per active room is trivial; limit 1000 simultaneous rooms (config). |
| Drift between chess.js (UI) and notnil/chess (server) | Server is authoritative. After every move the UI re-syncs to the FEN the server returned. Any divergence is treated as a UI bug. |
| Spectator firehose abuse | Cap 8 spectators per room. Spectators send no frames; their WS reader just drains. |
| LLM cost when chess widget is left open | Move requests only fire when it's the bot's turn — one request per actual move. No keep-alive. |

## Out of scope (future)

- Pre-moves, premove queue.
- Conditional premove ("if knight takes, recapture with bishop").
- Engine-assisted analysis after the game (built-in Stockfish.js could be added later).
- Spectator chat.
- Player profile / rating system.
- Time-control negotiation between paired players (today it's URL-driven and the first joiner's time control wins).
