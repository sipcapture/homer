# VoIP Games (Homer Next-Gen dashboard)

The dashboard includes five educational mini-games under the **Games** category. Widgets are added from the dashboard palette (`registry.ts`: `packet_defender`, `sip_dialog_master`, `jitter_buffer_hero`, `sipetris`, `netris`). They reinforce SIP/RTP terminology and typical traffic-handling patterns in a lightweight, interactive way.

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

## Source locations

| Game | File |
|------|------|
| Packet Defender | `src/ui/src/dashboard/widgets/PacketDefenderPanel.tsx` |
| SIP Dialog Master | `src/ui/src/dashboard/widgets/SIPDialogMasterPanel.tsx` |
| Jitter Buffer Hero | `src/ui/src/dashboard/widgets/JitterBufferHeroPanel.tsx` |
| SIPetris | `src/ui/src/dashboard/widgets/SIPetrisPanel.tsx` |
| Netris | `src/ui/src/dashboard/widgets/NetrisPanel.tsx` |
| Widget registration | `src/ui/src/dashboard/widgets/registry.ts` |

The single-player games (Packet Defender, SIP Dialog Master, Jitter Buffer Hero, SIPetris) are **not** wired to live Homer capture; they are **local dashboard UI** only. **Netris** is the one exception — it talks to homer-core's coordinator over a JWT-protected WebSocket (`/api/v4/games/netris`) but only for two-player relay; it does not consume captured traffic.
