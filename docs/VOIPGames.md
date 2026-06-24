# Games (Homer Next-Gen dashboard)

The dashboard includes eight mini-games under the **Games** category. Widgets are added from the dashboard palette (`registry.ts`: `packet_defender`, `sip_dialog_master`, `jitter_buffer_hero`, `sipetris`, `netris`, `chess`, `netchess`, `doom`). Five are educational (SIP/RTP-themed), two are general-purpose chess games, and one is the actual 1993 Doom compiled to WebAssembly. The chess pair shares one rules engine (`chess.js` on the UI, `notnil/chess` on the server) and one presentational board component (`ChessBoard.tsx`).

### Default dashboards

On first login (or after a `Reset` from `DashboardSettingsDialog`) the coordinator seeds four dashboards (`DashboardService.ResetDashboards`):

1. **Home** — SIP Call Search + Results + Clock.
2. **Smart Search** — Protocol Search + Results + Time Chart.
3. **Games** — Packet Defender, SIP Dialog Master, Jitter Buffer Hero, SIPetris, Chess. Single-player only — no coordinator hubs required.
4. **NetGames** — Netris, NetChess. Multiplayer over WebSocket; both widgets show a connection error until the corresponding hub is reachable, so we keep them on a separate tab.

Items 3–4 are omitted when `coordinator.widgets.control.games` is `false` (see below).

Users can rename, reorder, or delete any of these dashboards at runtime; the seed only runs when `ListDashboards` returns an empty set.

### Disabling games in production

To hide Games / NetGames from the widget picker and skip seeding those default dashboards, set:

```json
"coordinator": {
  "widgets": {
    "control": {
      "games": false
    }
  }
}
```

When `games` is `false`:

- The **Games** category is hidden in Add Widget.
- Default **Games** and **NetGames** tabs are not created on reset / first login.
- Multiplayer hubs (`/api/v4/games/netris`, `/api/v4/games/netchess`) are not started (endpoints return 503).
- The `/gamedata/` static route (Doom IWAD) is not registered.

Existing dashboards that already contain game widgets may remain until removed manually; those widgets show a short “category disabled” message instead of loading the game UI.

Other picker categories (`search`, `visualize`, `external`, `utility`) can be set to `false` the same way; today that only hides them in Add Widget (no extra backend gates yet).

---

## Shared controls

| Control | Behavior |
|--------|----------|
| **Start / Restart** | Starts or restarts a run. |
| **Pause / Resume** | Pauses timers, spawning, movement, and input; on resume there is no large accumulated-time jump (the clock baseline is updated correctly). |
| **“Paused” overlay** | Covers the play area so you do not accidentally click through while paused. |

Pause is cleared automatically when the game is not `running` (for example after game over).

### Score

- **Current score** is shown prominently in the header (highlighted “Score” pill) while you play.
- **Best / Record** (highest session score) is stored in **browser `localStorage`** per game (`homer_game_best_*` keys) and updated when a session ends (game over or SIP session complete). SIP Dialog Master uses **Record** for this value so it does not collide with **Best** (best answer streak).
- The pause overlay repeats the current score (and best/record when already set) so you can read it without leaving pause.

---

## 1. Packet Defender (`packet_defender`)

**Concept:** you defend an **SBC**. “Packets” (cards with a SIP/RTP label and short blurb) move downward. **Click only bad** packets before they reach the SBC; **good** packets should be allowed through.

### Good vs bad packets

- **Good** (green/neutral): typical legitimate messages — INVITE, 200 OK, 180 Ringing, ACK, BYE, REGISTER, OPTIONS, RTP, RTCP SR, etc.
- **Bad** (red/orange): anomalies and attacks — malformed INVITE, INVITE flood, suspicious RTP (SSRC, seq), fuzzed OPTIONS, brute-force REGISTER, SipVicious, bad CANCEL, 404 spoof, etc.

### Score and failure

- Blocking a bad packet awards points with a **combo** multiplier (up to ×10).
- Clicking a **good** packet is a **false positive**: **MOS drops by 0.3** and combo resets.
- If a **bad** packet reaches the SBC — MOS penalty; if a **good** one passes through — small MOS bonus.
- **Game over** when **MOS ≤ 1.0** while the game is active.

### Waves

**Waves** increase difficulty over time: higher share of bad packets and faster spawn rate.

### Power-ups

| Button | Effect |
|--------|--------|
| **Rate limit** | Freezes traffic briefly; long cooldown. |
| **HEP capture** | Highlights threats for a limited time. |
| **Firewall** | Auto-removes up to several bad packets (charges + cooldown). |

Power-ups are disabled while paused.

---

## 2. SIP Dialog Master (`sip_dialog_master`)

**Concept:** build the **correct SIP message sequence** for the selected scenario (like a real dialog). Each step offers several choices; pick the next expected message.

### Scenarios (examples)

| Scenario | Difficulty | Flow idea |
|----------|------------|-----------|
| Basic Call | easy | INVITE → 100 → 180 → 200 → ACK → BYE → 200 |
| Fast Answer | easy | no 180, straight to 200 |
| Registration | medium | REGISTER → 401 → REGISTER → 200 |
| Busy Rejection | easy | through 486 Busy Here + ACK |
| No Answer (CANCEL) | hard | cancel while ringing |
| OPTIONS Ping | easy | keepalive |
| Redirect 302 | hard | redirect and second INVITE |
| Re-INVITE (Hold) | medium | mid-call hold |
| Proxy Auth | hard | 407 and retried INVITE |
| Not Found | easy | 404 + ACK |

Difficulty affects how distracting the wrong answers are at each step.

### Timer and score

- Each round has a **time limit**; finishing quickly grants a **score bonus**.
- Round history is shown in the UI.

While paused, the round timer does not tick and answer selection is blocked.

---

## 3. Jitter Buffer Hero (`jitter_buffer_hero`)

**Concept:** a simplified **RTP stream** with jitter, a deadline bar, and playout-style visualization. Packets arrive with timing variance; **click packets in ascending RTP sequence number** order before the deadline (jitter buffer / playout abstraction).

### Mechanics

- Each packet shows **seq**, **timestamp**, **PT** (payload type), **SSRC**, frame size, and **jitter** (displayed arrival spread).
- **Waves** ramp up: more packets, stronger jitter, shorter deadline, **duplicates** and simulated **loss**.
- Codecs in the pool: PCMU, PCMA, G722, G729, opus, G726-32 (with typical timestamp steps for a 20 ms frame).

### Mistakes

- Out-of-order seq click, late relative to deadline, duplicate — penalties to **MOS** and score (exact numbers appear in the UI).

### Pause

Game time, spawning, and deadline logic stop; clicks on RTP cards are ignored.

---

## 4. SIPetris (`sipetris`)

**Concept:** classic Tetris where each tetromino represents a SIP method. Stack falling pieces, clear full lines, and every cleared line is logged as a SIP "transaction" in the side panel.

### Method ↔ shape mapping

| Tetromino | SIP method | Why |
|-----------|------------|-----|
| **I** (line of 4) | INVITE | Long call setup, fills a row in one shot. |
| **O** (square) | ACK | Small, square, terminal confirm. |
| **T** | REGISTER | Branching binding hub. |
| **L** | BYE | Tearing down the dialog. |
| **J** | CANCEL | Mirror of BYE — early teardown. |
| **S** | OPTIONS | Sliding capability probe. |
| **Z** | PRACK | Mirror probe — provisional ack. |

### Controls

- **← / →** move piece left / right
- **↑** or **X** rotate clockwise (with small wall-kick)
- **↓** soft drop (+1 score per cell)
- **Space** hard drop (+2 score per cell)
- **P** pause / resume

Click the board first so it has keyboard focus.

### Scoring

Classic NES-Tetris formula:

| Lines cleared | Base × level |
|---------------|--------------|
| 1 | 100 |
| 2 | 300 |
| 3 | 500 |
| 4 (TETRIS) | 800 |

Soft drop adds **1 pt per row**, hard drop adds **2 pt per row**. Level rises every **10 cleared lines**, and the drop interval shortens with level. Game over when a freshly spawned piece can't fit ("503 Service Unavailable").

The side panel shows the **next 3 pieces**, the **last 8 cleared transactions** (with the dominant SIP method and points), and a **method legend**. Best score persists in `localStorage` (`homer_game_best_sipetris`).

### Live mode

The HUD has a **Live ○/●** toggle that opens the homer-core HEP WebSocket stream (`/api/v4/stream/hep?proto=1&history=20`, JWT-protected — same client as Packet Defender). Each incoming SIP event whose method maps to one of the seven tetrominoes is queued in a small ring (cap 16). When a new piece is needed, the queue takes priority over the 7-bag, so **the call going through your homer-core right now drives the next falling shape**.

- Methods without a tetromino mapping (UPDATE, NOTIFY, SUBSCRIBE, …) are ignored — the bag stays valid.
- Toggling Live off drains the ring and the game falls back to the regular 7-bag immediately.
- The button label shows the live stream state (`connecting` / `● on (N)` / `closed`); reconnection backoff lives inside the WebSocket client.

This means SIPetris doubles as a live SIP method visualiser: a heavy `INVITE` storm produces I-pieces, a `REGISTER` burst produces T-pieces, and so on.

---

## 5. Netris (`netris`)

**Concept:** two-player SIPetris over the dashboard's authenticated WebSocket. Same shapes, same SIP-method theming as single-player SIPetris, but every line you clear sends **garbage rows** to your opponent's stack — and theirs lift up into yours. Last stack standing wins.

The widget is built on top of the same `tetrisCore.ts` as SIPetris, so movement, scoring, level progression and rotation behaviour are identical.

### Matchmaking

Two ways to find an opponent (both on the same WS endpoint, just different query params):

| Mode | What it does |
|------|--------------|
| **Quick** | Joins the global FIFO queue; the next player to also pick Quick is your opponent. One-click match-making. |
| **Room** | You and your friend agree on a code, both type it in, and the first room with that code matches you up. Capacity is 2 — a third player gets a polite `error` frame and is dropped. |

A lone player in either mode times out after **60 seconds** with a `waiting_timeout` frame (the room is then deleted server-side).

### Garbage rule

The server is the authority on garbage so the two clients never disagree on how a clear translates into pain for the opponent:

| Lines you clear | Garbage rows sent to opponent |
|-----------------|--------------------------------|
| 1               | 0 |
| 2               | 1 |
| 3               | 2 |
| 4 (TETRIS)      | 4 |

Each garbage row is a fully filled grey strip with **one empty cell** (the "hole") in the same column across all rows of that burst. The server picks the hole column once per burst and sends it as `{"type":"garbage_in","lines":k,"hole":c}` so both clients render the same incoming row. If the lifted stack collides with your active piece, you immediately top out — the other side wins.

### Lifecycle

1. Pick **Quick** or **Room** (with a code) and press the action button → status pill shows `Waiting…`.
2. Once the second player connects, both sides see `Matched · vs <name>` and a **Ready** button appears.
3. After both players hit Ready, the server emits `start` and the boards begin to fall.
4. First to top out (own stack overflow or buried by garbage) loses. The other player gets a `200 OK` win banner; the loser gets `503 Service Unavailable`.
5. Either side can press **Cancel** / **Leave** at any time — the other player is told `opponent_left` and (if in-play) wins the round.

### Authentication & accounts

Auth is the same JWT used everywhere else in the dashboard — the WebSocket carries the token via `?access_token=…` (browsers can't put it on the handshake header). The player's display name is taken from the `username` JWT claim, so two different Homer accounts always see each other under their real Homer logins.

### Endpoint

- `GET /api/v4/games/netris` — protected (JWTMiddlewareV4). Query params:
  - `room=<code>` — join (or create) a named room (capacity 2, code ≤ 64 chars).
  - `mode=quick` — auto-pair via the FIFO queue.
  - `display=<name>` — optional label override (≤ 32 chars after sanitisation; the JWT username is still used for matchmaking).

The relay is intentionally thin: server enforces matchmaking, garbage translation, and disconnect detection — both clients are still trusted to crunch their own boards. A nefarious client could in theory send a `line_clear` it didn't earn; for an in-house dashboard game that's an acceptable trade-off (and the loser will notice the other side never visibly clears a line).

### Source

| Component | File |
|-----------|------|
| UI widget | `src/ui/src/dashboard/widgets/NetrisPanel.tsx` |
| Shared Tetris core | `src/ui/src/dashboard/widgets/tetrisCore.ts` |
| WS client | `src/ui/src/api.ts` (`openNetrisSocket`) |
| Server hub | `src/coordinator/games/netris/hub.go`, `protocol.go` |
| HTTP handler | `src/coordinator/handlers/games_v4.go` |

---

## 6. Chess (`chess`)

**Concept:** single-player chess against a built-in bot or — when the operator enables the MCP LLM bridge — against an LLM opponent. The minimax bot runs in a Web Worker so the UI never blocks. The widget is general-purpose entertainment; it does not consume captured traffic.

### Controls

| Control | Behavior |
|---------|----------|
| **Time control** | `Untimed` / `Bullet 1+0` / `Blitz 3+2` / `Rapid 10+5` (Fischer increment). Changes only apply on the next `New game`. |
| **Side** | `White` / `Black` / `Random` — applied on the next new game. |
| **Level** | 1–4 search depth slider for the bot (also passed as a hint to the LLM). |
| **Bot / LLM** | Toggle between the local minimax and the MCP LLM. Only shown if `GET /api/v4/games/chess/llm-status` reports `{ enabled: true }`. |
| **New game** | Resets the position, applies the current side/time-control. |
| **Takeback** | Pops two plies (your move + bot's reply) so it's your move again. |
| **Resign** | Ends the game in your loss. |
| **Export PGN** | Copies the full PGN of the played game to the clipboard. |

### LLM mode

When LLM mode is active, every bot move is requested from the coordinator endpoint `POST /api/v4/games/chess/llm-move`. The coordinator:

1. Calls the configured `mcp.llm` provider via the shared `src/mcp/llm.go` client with a fixed chess system prompt.
2. Validates the model's reply via `github.com/notnil/chess` (`ValidateUCI`).
3. If the model returns an illegal or unparseable move, falls back to a server-side greedy picker so the widget always receives a legal UCI for non-terminal positions. The response carries `source: "llm" | "fallback"` and the widget shows an **`fallback`** pill when the fallback path was used.

The LLM client is shared with MCP search, so there's no separate configuration. To enable: set `mcp.llm.enable = true` plus the usual `base_url` / `model` / `api_key` in your homer config.

### Persistence

A game in progress is saved to `localStorage` (`homer_chess_state_v1`) including PGN, side, time control, clocks, mode, and level. It restores across page reloads. Local play state is per-browser; not synced.

---

## 7. NetChess (`netchess`)

**Concept:** two-player chess over the coordinator. Lobby + WebSocket relay, but unlike Netris **the server is the authoritative game state**: it holds the `*notnil/chess.Game`, validates every move, manages clocks, and emits game-over frames. Clients render whatever FEN the server hands them.

### Controls

| Control | Behavior |
|---------|----------|
| **Quick / Room / Spectate** | `Quick` auto-pairs you with the next waiting player. `Room` joins/creates a named room (or click `Random` for a 6-letter code). `Spectate` joins an existing room read-only (up to 8 spectators). |
| **Time control** | `Bullet`, `Blitz`, `Rapid`, `Classical` — first joiner of a room sets the time control, second joiner inherits it. |
| **Colour** | `White` / `Black` / `Random` — applied at the join handshake. |
| **Ready** | Sent by both players to start the game (mirrors Netris). |
| **Offer draw / Takeback / Resign** | Standard chess game-management buttons. Each generates a server-validated transition. |

### Authoritative state

Per room the server tracks: white/black slot, spectator list (cap 8), `*chess.Game`, white/black clocks in ms, a `time.AfterFunc` flag timer, and pending draw / takeback offers. Every client `move` frame is checked against `game.ValidMoves()` and the side-to-move; illegal frames receive an `error` envelope and the game state is unchanged. Clocks tick on the server (`time.AfterFunc` for the flag, recomputed at each move from wall-clock deltas) so a backgrounded / throttled tab cannot gain time.

### Wire protocol

JSON envelope shared with the client; constants live in `src/coordinator/games/netchess/protocol.go`:

| Frame | Direction | Notes |
|-------|-----------|-------|
| `hello`, `ready`, `move{uci}`, `resign`, `draw_{offer,accept,decline}`, `takeback_{request,accept,decline}`, `chat{text}` | client → server | one-way commands |
| `matched{you, opponent, color, room, initial_ms, increment_ms, spectator?}` | server → client | issued on join + on opponent arrival |
| `start{fen, white_ms, black_ms}` | server → client | both players have sent `ready` |
| `opponent_move{uci, san, fen, white_ms, black_ms}` | server → both players + spectators | post-validation broadcast |
| `clock_sync{fen, white_ms, black_ms}` | server → all | after takeback or periodic re-sync |
| `game_over{result, reason, white_ms, black_ms}` | server → all | `result ∈ {1-0, 0-1, 1/2-1/2}`, `reason ∈ {checkmate, stalemate, resignation, agreement, flag, insufficient_material, fifty_move, threefold_repetition, opponent_disconnect}` |
| `draw_offered{from}`, `takeback_offered{from}`, `opponent_left`, `waiting_timeout`, `error{message}` | server → recipient | one-shot notifications |

### Source

| Component | File |
|-----------|------|
| UI widget | `src/ui/src/dashboard/widgets/NetChessPanel.tsx` |
| Reusable board | `src/ui/src/dashboard/widgets/ChessBoard.tsx` |
| Shared chess core | `src/ui/src/dashboard/widgets/chessCore.ts` (wraps `chess.js`) |
| Client engine (worker) | `src/ui/src/dashboard/widgets/chessEngine.ts`, `chessEngine.worker.ts` (used by `chess` only) |
| WS client | `src/ui/src/api.ts` (`openNetChessSocket`) |
| LLM endpoints | `src/ui/src/api.ts` (`fetchChessLLMStatus`, `postChessLLMMove`) |
| Server-side rules + LLM bridge | `src/coordinator/games/chess/engine.go`, `llm.go` |
| Server hub | `src/coordinator/games/netchess/hub.go`, `protocol.go` |
| HTTP / WS handlers | `src/coordinator/handlers/games_v4.go` (`V4NetChess`, `V4ChessLLMMove`, `V4ChessLLMStatus`) |

---

## 8. Doom (`doom`)

**Concept:** the real thing — Chocolate Doom compiled to WebAssembly (the
[cloudflare/doom-wasm](https://github.com/cloudflare/doom-wasm) build, GPL-2)
running inside the widget. No SIP theming, no excuses.

### Architecture

| Piece | Where | Shipped how |
|-------|-------|-------------|
| Engine (`websockets-doom.js` + `.wasm`, ~2.5 MB) | `src/ui/public/game/` | Committed + embedded with the UI bundle (provenance and sha256 in `public/game/README.md`) |
| Host page | `src/ui/public/game/index.html` | Embedded; loaded by the widget in an `<iframe>` |
| IWAD (`doom1.wad`, ~4 MB) | On-disk `gamedata_dir`, served at `/gamedata/` | **Never embedded** into the homer-core binary; provisioned manually |

The engine runs in an iframe because Emscripten/SDL2 builds pollute window
globals and cannot be torn down cleanly — removing the iframe (the Stop
button or deleting the widget) is the teardown. Widget and host page talk
over same-origin `postMessage` (`doomWad.ts` validates the frames).

### WAD provisioning

The shareware `doom1.wad` is freely distributable but deliberately kept out
of the repo, the UI bundle, and the `go:embed` binary. On the coordinator
host (the script ships with the deb/rpm under
`/usr/local/homer-core/scripts/`):

```bash
sudo /usr/local/homer-core/scripts/fetch-doom-wad.sh /usr/local/homer-core/gamedata
```

`gamedata_dir` defaults to `/usr/local/homer-core/gamedata`, so with the
package layout no config change is needed — just download the WAD. To serve
from another directory set `coordinator.http_server.gamedata_dir`; an empty
string disables the `/gamedata/` route entirely.

Any IWAD/PWAD named `doom1.wad` in that directory works (full Doom,
Freedoom Phase 1 renamed — note vanilla visplane limits apply). If the WAD
or the route is missing, the widget shows provisioning instructions instead
of starting. The download happens once per page load with a progress bar;
afterwards the browser HTTP cache applies.

### Controls

Standard Chocolate Doom bindings: arrows move, **Ctrl** fire, **Space**
use, **ESC** in-game menu (which is also the pause). Click the game screen
first so the iframe has keyboard focus. The header has **Fullscreen** and
**Stop** buttons; music is disabled (`-nomusic`), sound effects work after
the first user gesture (the Start click satisfies autoplay policy).

### Dev mode

`vite.config.ts` proxies `/gamedata` to the coordinator, mirroring `/api` —
run a coordinator with `gamedata_dir` set, or expect the WAD-missing hint.

---

## Source locations

| Game | File |
|------|------|
| Packet Defender | `src/ui/src/dashboard/widgets/PacketDefenderPanel.tsx` |
| SIP Dialog Master | `src/ui/src/dashboard/widgets/SIPDialogMasterPanel.tsx` |
| Jitter Buffer Hero | `src/ui/src/dashboard/widgets/JitterBufferHeroPanel.tsx` |
| SIPetris | `src/ui/src/dashboard/widgets/SIPetrisPanel.tsx` |
| Netris | `src/ui/src/dashboard/widgets/NetrisPanel.tsx` |
| Chess | `src/ui/src/dashboard/widgets/ChessPanel.tsx` |
| NetChess | `src/ui/src/dashboard/widgets/NetChessPanel.tsx` |
| Doom | `src/ui/src/dashboard/widgets/DoomPanel.tsx`, `doomWad.ts`, `src/ui/public/game/` |
| Widget registration | `src/ui/src/dashboard/widgets/registry.ts` |

The single-player widgets (Packet Defender, SIP Dialog Master, Jitter Buffer Hero, SIPetris, Chess in bot mode, Doom) are **not** wired to live Homer capture; they are **local dashboard UI** only. **Netris** and **NetChess** talk to homer-core's coordinator over JWT-protected WebSockets (`/api/v4/games/netris`, `/api/v4/games/netchess`); the **Chess LLM mode** also relies on the coordinator (`/api/v4/games/chess/llm-{status,move}`); **Doom** only fetches its IWAD from the static `/gamedata/` route. None of these endpoints consume captured traffic.
