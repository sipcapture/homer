// Copyright (C) 2025 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package netchess

import (
	"errors"
	"fmt"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	notnil "github.com/notnil/chess"
)

// Default knobs — coordinator wiring can override via Config{}.
const (
	DefaultPlayerOutbox   = 64
	DefaultWaitingTimeout = 90 * time.Second
)

// ErrRoomFull signals that a named room already has two players.
// The handler maps this to a one-shot "error" frame and closes the
// socket — we never bump an existing pair.
var ErrRoomFull = errors.New("netchess: room is full")

// Config controls hub behaviour. Zero values fall back to the
// Default* constants above so coordinator wiring can pass an empty
// Config and still get a working hub.
type Config struct {
	PlayerOutbox   int
	WaitingTimeout time.Duration
}

func (c Config) outbox() int {
	if c.PlayerOutbox <= 0 {
		return DefaultPlayerOutbox
	}
	return c.PlayerOutbox
}

func (c Config) waiting() time.Duration {
	if c.WaitingTimeout <= 0 {
		return DefaultWaitingTimeout
	}
	return c.WaitingTimeout
}

// JoinOptions carry the per-handshake parameters the URL parser
// extracts from the WS upgrade request.
type JoinOptions struct {
	// ColorPref is "white" | "black" | "random" | "" (defaults to random).
	ColorPref string
	// InitialMS / IncrementMS configure the clocks for a new room.
	// Ignored when the room already exists (second joiner inherits
	// the first joiner's setup).
	InitialMS   int64
	IncrementMS int64
}

// Hub holds all live netchess rooms, the quick-match queue, and the
// shared RNG used for random colour assignment / quick-room codes.
type Hub struct {
	mu       sync.Mutex
	rooms    map[string]*Room
	queue    *Player // single waiting quick-match player
	quickSeq uint64
	rng      *rand.Rand
	rngMu    sync.Mutex
	cfg      Config
	timeNow  func() time.Time // injected for deterministic clock tests
}

// NewHub builds an empty hub with the supplied config. The RNG is
// seeded from the wall clock; tests should call SeedRNG.
func NewHub(cfg Config) *Hub {
	return &Hub{
		rooms:   make(map[string]*Room),
		rng:     rand.New(rand.NewSource(time.Now().UnixNano())),
		cfg:     cfg,
		timeNow: time.Now,
	}
}

// SeedRNG fixes the RNG sequence — used in tests for deterministic
// colour assignment.
func (h *Hub) SeedRNG(seed int64) {
	h.rngMu.Lock()
	defer h.rngMu.Unlock()
	h.rng = rand.New(rand.NewSource(seed))
}

// SetTimeNow replaces the hub's clock source. Test-only.
func (h *Hub) SetTimeNow(fn func() time.Time) { h.timeNow = fn }

// rngIntn grabs a random int under the rng mutex (rand.Rand is not
// safe for concurrent use).
func (h *Hub) rngIntn(n int) int {
	h.rngMu.Lock()
	defer h.rngMu.Unlock()
	if n <= 0 {
		return 0
	}
	return h.rng.Intn(n)
}

// Player is a single live socket. The WS handler owns Out and reads
// from it in its writer pump; the hub writes to Out and to no other
// goroutine. Once Out is closed, no further sends are made.
type Player struct {
	Username    string
	DisplayName string
	Out         chan []byte

	// Set when the player is a read-only spectator. Spectators never
	// own the `room.white`/`room.black` slot.
	Spectator bool

	hub      *Hub
	room     *Room
	color    string // "white" | "black" | "" for spectators
	ready    bool
	doneOnce sync.Once
	done     chan struct{}
}

// NewPlayer constructs a Player with an outbox of the requested size.
// The handler is responsible for filling Username (from the JWT) and
// optionally DisplayName.
func NewPlayer(username, displayName string, outboxSize int) *Player {
	if outboxSize <= 0 {
		outboxSize = DefaultPlayerOutbox
	}
	return &Player{
		Username:    username,
		DisplayName: SanitiseDisplayName(displayName),
		Out:         make(chan []byte, outboxSize),
		done:        make(chan struct{}),
	}
}

// Done is closed when the player has been removed from its room
// (by Leave or because the room ended). The handler waits on this to
// tear down its writer pump.
func (p *Player) Done() <-chan struct{} { return p.done }

func (p *Player) markDone() { p.doneOnce.Do(func() { close(p.done) }) }

// Room is a single chess game between two players (plus zero or more
// spectators). All transitions happen under hub.mu; per-player sends
// are non-blocking so a slow client never wedges the hub.
type Room struct {
	Code      string
	Quick     bool
	createdAt time.Time

	white *Player
	black *Player

	spectators []*Player

	// Authoritative game state. Nil until both players have sent
	// "ready" and the game has started.
	game *notnil.Game

	// Time control.
	initialMS   int64
	incrementMS int64
	whiteMS     int64
	blackMS     int64
	// lastMoveAt is the wall-clock moment the side-to-move's clock
	// started ticking — i.e. just after the previous half-move
	// (or the start of the game).
	lastMoveAt time.Time
	flagTimer  *time.Timer

	// waitTimer fires waiting_timeout if a quick-match queued
	// player or a lone-room joiner never gets paired.
	waitTimer *time.Timer

	// pendingDrawFrom / pendingTakebackFrom track outstanding
	// proposals so the responder's accept/decline is matched to a
	// real offer (and stale frames after game-end are ignored).
	pendingDrawFrom     *Player
	pendingTakebackFrom *Player

	started bool
	ended   bool
}

// JoinRoom places a player into a named room (creating it if needed).
// Returns the room. The handler should call this exactly once per
// player. The lobby `matched` envelope is enqueued onto the player's
// outbox before this returns so the writer pump can flush it.
func (h *Hub) JoinRoom(code string, p *Player, opts JoinOptions) (*Room, error) {
	if len(code) > MaxRoomCodeLen {
		code = code[:MaxRoomCodeLen]
	}
	h.mu.Lock()
	defer h.mu.Unlock()

	r, ok := h.rooms[code]
	if !ok {
		r = h.createRoom(code, false, opts)
		h.rooms[code] = r
		h.seatFirst(r, p, opts)
		h.armWaitTimer(r)
		h.send(p, Envelope{Type: MsgMatched, Room: code, You: displayOf(p), Color: p.color,
			InitialMS: r.initialMS, IncrementMS: r.incrementMS})
		return r, nil
	}

	if r.white != nil && r.black != nil {
		return nil, ErrRoomFull
	}
	h.seatSecond(r, p)
	h.cancelWaitTimer(r)
	h.notifyMatched(r)
	return r, nil
}

// JoinQuick places a player into the quick-match queue. If a player
// is already waiting, the two are paired into a fresh `quick-N`
// room; otherwise the caller becomes the new queued player.
func (h *Hub) JoinQuick(p *Player, opts JoinOptions) (*Room, error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.queue == nil {
		code := fmt.Sprintf("quick-pending-%d", atomic.AddUint64(&h.quickSeq, 1))
		r := h.createRoom(code, true, opts)
		h.rooms[code] = r
		h.seatFirst(r, p, opts)
		h.queue = p
		h.armWaitTimer(r)
		h.send(p, Envelope{Type: MsgMatched, Room: code, You: displayOf(p), Color: p.color,
			InitialMS: r.initialMS, IncrementMS: r.incrementMS})
		return r, nil
	}

	first := h.queue
	h.queue = nil
	r := first.room
	if r == nil {
		// Defensive: shouldn't happen because we always create the
		// pending room above.
		code := fmt.Sprintf("quick-%d", atomic.AddUint64(&h.quickSeq, 1))
		r = h.createRoom(code, true, opts)
		h.rooms[code] = r
	}
	delete(h.rooms, r.Code)
	r.Code = fmt.Sprintf("quick-%d", atomic.AddUint64(&h.quickSeq, 1))
	h.rooms[r.Code] = r
	h.seatSecond(r, p)
	h.cancelWaitTimer(r)
	h.notifyMatched(r)
	return r, nil
}

// JoinSpectator attaches a read-only listener to an existing room.
// Returns ErrRoomFull (overloaded — spec-cap reached) when the
// spectator quota is saturated.
func (h *Hub) JoinSpectator(code string, p *Player) (*Room, error) {
	if len(code) > MaxRoomCodeLen {
		code = code[:MaxRoomCodeLen]
	}
	h.mu.Lock()
	defer h.mu.Unlock()

	r, ok := h.rooms[code]
	if !ok {
		return nil, fmt.Errorf("no such room: %s", code)
	}
	if len(r.spectators) >= MaxSpectators {
		return nil, ErrRoomFull
	}
	p.Spectator = true
	p.hub = h
	p.room = r
	r.spectators = append(r.spectators, p)
	// Brief on arrival: room metadata + current FEN (if game running).
	fen := ""
	whiteMS, blackMS := r.initialMS, r.initialMS
	if r.game != nil {
		fen = r.game.FEN()
		whiteMS, blackMS = r.whiteMS, r.blackMS
	}
	h.send(p, Envelope{
		Type: MsgMatched, Room: code, Spectator: true,
		You:         displayOf(p),
		Opponent:    fmt.Sprintf("%s vs %s", displayOf(r.white), displayOf(r.black)),
		InitialMS:   r.initialMS,
		IncrementMS: r.incrementMS,
		FEN:         fen,
		WhiteMS:     whiteMS,
		BlackMS:     blackMS,
	})
	return r, nil
}

// createRoom builds a Room shell. Caller holds hub.mu.
func (h *Hub) createRoom(code string, quick bool, opts JoinOptions) *Room {
	init := opts.InitialMS
	if init <= 0 {
		init = DefaultInitialMS
	}
	inc := opts.IncrementMS
	if inc < 0 {
		inc = 0
	}
	if opts.InitialMS == 0 && opts.IncrementMS == 0 {
		inc = DefaultIncMS
	}
	return &Room{
		Code:        code,
		Quick:       quick,
		createdAt:   h.timeNow(),
		initialMS:   init,
		incrementMS: inc,
		whiteMS:     init,
		blackMS:     init,
	}
}

// seatFirst assigns a colour to the lone joiner. Caller holds hub.mu.
func (h *Hub) seatFirst(r *Room, p *Player, opts JoinOptions) {
	switch opts.ColorPref {
	case ColorWhite:
		r.white = p
		p.color = ColorWhite
	case ColorBlack:
		r.black = p
		p.color = ColorBlack
	default:
		// random — pick now; the other slot is fixed by exclusion.
		if h.rngIntn(2) == 0 {
			r.white = p
			p.color = ColorWhite
		} else {
			r.black = p
			p.color = ColorBlack
		}
	}
	p.hub = h
	p.room = r
}

// seatSecond places the second arrival into whichever slot is empty.
// Caller holds hub.mu.
func (h *Hub) seatSecond(r *Room, p *Player) {
	if r.white == nil {
		r.white = p
		p.color = ColorWhite
	} else {
		r.black = p
		p.color = ColorBlack
	}
	p.hub = h
	p.room = r
}

// notifyMatched sends both players the full matched frame. Caller
// holds hub.mu.
func (h *Hub) notifyMatched(r *Room) {
	if r.white == nil || r.black == nil {
		return
	}
	h.send(r.white, Envelope{
		Type: MsgMatched, Room: r.Code,
		You: displayOf(r.white), Opponent: displayOf(r.black),
		Color: ColorWhite, InitialMS: r.initialMS, IncrementMS: r.incrementMS,
	})
	h.send(r.black, Envelope{
		Type: MsgMatched, Room: r.Code,
		You: displayOf(r.black), Opponent: displayOf(r.white),
		Color: ColorBlack, InitialMS: r.initialMS, IncrementMS: r.incrementMS,
	})
}

func (h *Hub) armWaitTimer(r *Room) {
	d := h.cfg.waiting()
	if d <= 0 {
		return
	}
	r.waitTimer = time.AfterFunc(d, func() { h.handleWaitingTimeout(r) })
}

func (h *Hub) cancelWaitTimer(r *Room) {
	if r.waitTimer != nil {
		r.waitTimer.Stop()
		r.waitTimer = nil
	}
}

func (h *Hub) handleWaitingTimeout(r *Room) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if (r.white != nil && r.black != nil) || r.ended {
		return
	}
	r.ended = true
	if r.white != nil {
		h.send(r.white, Envelope{Type: MsgWaitingTimeout, Reason: "no_opponent"})
		h.closePlayer(r.white)
	}
	if r.black != nil {
		h.send(r.black, Envelope{Type: MsgWaitingTimeout, Reason: "no_opponent"})
		h.closePlayer(r.black)
	}
	delete(h.rooms, r.Code)
}

// armFlagTimer (re)starts the flag timer for the side currently on
// move. Caller holds hub.mu.
func (h *Hub) armFlagTimer(r *Room) {
	if r.flagTimer != nil {
		r.flagTimer.Stop()
		r.flagTimer = nil
	}
	if r.game == nil || r.ended {
		return
	}
	side := r.game.Position().Turn()
	var remaining int64
	if side == notnil.White {
		remaining = r.whiteMS
	} else {
		remaining = r.blackMS
	}
	if remaining <= 0 {
		// Already flagged — handle inline.
		h.handleFlag(r, side)
		return
	}
	d := time.Duration(remaining) * time.Millisecond
	r.flagTimer = time.AfterFunc(d, func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		if r.ended || r.game == nil {
			return
		}
		// Re-evaluate the elapsed time at fire instead of trusting
		// the original `remaining`: setTimeNow / pauses in tests can
		// drift.
		h.tickClock(r)
		if r.whiteMS <= 0 {
			h.handleFlag(r, notnil.White)
		} else if r.blackMS <= 0 {
			h.handleFlag(r, notnil.Black)
		} else {
			// Spurious — re-arm.
			h.armFlagTimer(r)
		}
	})
}

// tickClock subtracts elapsed time from the side-to-move's clock and
// updates lastMoveAt. Caller holds hub.mu.
func (h *Hub) tickClock(r *Room) {
	if r.game == nil || !r.started || r.ended {
		return
	}
	now := h.timeNow()
	elapsed := now.Sub(r.lastMoveAt).Milliseconds()
	if elapsed <= 0 {
		return
	}
	r.lastMoveAt = now
	if r.game.Position().Turn() == notnil.White {
		r.whiteMS -= elapsed
		if r.whiteMS < 0 {
			r.whiteMS = 0
		}
	} else {
		r.blackMS -= elapsed
		if r.blackMS < 0 {
			r.blackMS = 0
		}
	}
}

func (h *Hub) handleFlag(r *Room, flaggedSide notnil.Color) {
	r.ended = true
	if r.flagTimer != nil {
		r.flagTimer.Stop()
		r.flagTimer = nil
	}
	result := "0-1"
	if flaggedSide == notnil.Black {
		result = "1-0"
	}
	h.broadcast(r, Envelope{
		Type: MsgGameOver, Result: result, Reason: ReasonFlag,
		WhiteMS: r.whiteMS, BlackMS: r.blackMS,
	})
}

// HandleFrame routes a decoded client frame through the hub. Errors
// are surfaced as `error` envelopes back to the sender; the
// connection is left open so the client can recover.
func (h *Hub) HandleFrame(p *Player, e Envelope) {
	h.mu.Lock()
	defer h.mu.Unlock()
	r := p.room
	if r == nil || r.ended {
		return
	}
	if p.Spectator {
		// Spectators are mute. Silently drop anything they send.
		return
	}
	switch e.Type {
	case MsgReady:
		h.handleReady(r, p)
	case MsgMove:
		h.handleMove(r, p, e.UCI)
	case MsgResign:
		h.handleResign(r, p)
	case MsgDrawOffer:
		h.handleDrawOffer(r, p)
	case MsgDrawAccept:
		h.handleDrawAccept(r, p)
	case MsgDrawDecline:
		r.pendingDrawFrom = nil
	case MsgTakebackReq:
		h.handleTakebackRequest(r, p)
	case MsgTakebackAccept:
		h.handleTakebackAccept(r, p)
	case MsgTakebackDecl:
		r.pendingTakebackFrom = nil
	case MsgChat:
		text := SanitiseChat(e.Text)
		if text == "" {
			return
		}
		other := opponentOf(r, p)
		if other != nil {
			h.send(other, Envelope{Type: MsgChat, From: displayOf(p), Text: text})
		}
		for _, sp := range r.spectators {
			h.send(sp, Envelope{Type: MsgChat, From: displayOf(p), Text: text})
		}
	default:
		h.send(p, Envelope{Type: MsgError, Message: "unknown frame type"})
	}
}

func (h *Hub) handleReady(r *Room, p *Player) {
	if r.white == nil || r.black == nil {
		// Lone room — readiness latches but doesn't start anything.
		p.ready = true
		return
	}
	p.ready = true
	if !r.white.ready || !r.black.ready {
		return
	}
	if r.started {
		// Idempotent: server already broadcast start; treat extra
		// readies as no-ops.
		return
	}
	r.game = notnil.NewGame()
	r.started = true
	r.lastMoveAt = h.timeNow()
	r.whiteMS = r.initialMS
	r.blackMS = r.initialMS
	startMsg := Envelope{
		Type: MsgStart, FEN: r.game.FEN(),
		WhiteMS: r.whiteMS, BlackMS: r.blackMS,
	}
	h.broadcast(r, startMsg)
	if r.initialMS > 0 {
		h.armFlagTimer(r)
	}
}

func (h *Hub) handleMove(r *Room, p *Player, uci string) {
	if r.game == nil || !r.started {
		h.send(p, Envelope{Type: MsgError, Message: "game not started"})
		return
	}
	// Wrong side? Reject without disturbing the game state.
	side := r.game.Position().Turn()
	if (side == notnil.White && p != r.white) || (side == notnil.Black && p != r.black) {
		h.send(p, Envelope{Type: MsgError, Message: "not your turn"})
		return
	}
	// Validate via notnil/chess.
	mv, err := validateUCI(r.game, uci)
	if err != nil {
		h.send(p, Envelope{Type: MsgError, Message: err.Error()})
		return
	}
	// Tick the clock before applying — so the time spent on this
	// move counts against the mover, not the opponent.
	h.tickClock(r)
	// Apply.
	if err := r.game.Move(mv); err != nil {
		h.send(p, Envelope{Type: MsgError, Message: "engine rejected move"})
		return
	}
	// Increment after the move (Fischer).
	if r.incrementMS > 0 {
		if side == notnil.White {
			r.whiteMS += r.incrementMS
		} else {
			r.blackMS += r.incrementMS
		}
	}
	// Reset lastMoveAt so the next side's clock starts now.
	r.lastMoveAt = h.timeNow()

	// Resolve SAN from the move list (notnil stores SAN on the
	// completed move via game.Moves()).
	moves := r.game.Moves()
	san := ""
	if len(moves) > 0 {
		san = notnil.AlgebraicNotation{}.Encode(r.game.Positions()[len(r.game.Positions())-2], moves[len(moves)-1])
	}
	// Broadcast.
	out := Envelope{
		Type:    MsgOpponentMove,
		UCI:     uci,
		SAN:     san,
		FEN:     r.game.FEN(),
		WhiteMS: r.whiteMS,
		BlackMS: r.blackMS,
	}
	h.broadcast(r, out)

	// Game-over check.
	if outcome := r.game.Outcome(); outcome != notnil.NoOutcome {
		reason := reasonFromMethod(r.game.Method())
		h.broadcast(r, Envelope{
			Type:    MsgGameOver,
			Result:  string(outcome),
			Reason:  reason,
			WhiteMS: r.whiteMS,
			BlackMS: r.blackMS,
		})
		r.ended = true
		if r.flagTimer != nil {
			r.flagTimer.Stop()
			r.flagTimer = nil
		}
		return
	}

	// Re-arm flag timer for the new side-to-move.
	if r.initialMS > 0 {
		h.armFlagTimer(r)
	}
}

func (h *Hub) handleResign(r *Room, p *Player) {
	if r.game == nil || !r.started || r.ended {
		return
	}
	r.ended = true
	if r.flagTimer != nil {
		r.flagTimer.Stop()
		r.flagTimer = nil
	}
	result := "0-1"
	if p == r.black {
		result = "1-0"
	}
	h.broadcast(r, Envelope{
		Type: MsgGameOver, Result: result, Reason: ReasonResignation,
		WhiteMS: r.whiteMS, BlackMS: r.blackMS,
	})
}

func (h *Hub) handleDrawOffer(r *Room, p *Player) {
	if !r.started || r.ended {
		return
	}
	r.pendingDrawFrom = p
	other := opponentOf(r, p)
	if other != nil {
		h.send(other, Envelope{Type: MsgDrawOffered, From: displayOf(p)})
	}
}

func (h *Hub) handleDrawAccept(r *Room, p *Player) {
	if r.pendingDrawFrom == nil || r.pendingDrawFrom == p {
		return
	}
	r.pendingDrawFrom = nil
	r.ended = true
	if r.flagTimer != nil {
		r.flagTimer.Stop()
		r.flagTimer = nil
	}
	h.broadcast(r, Envelope{
		Type: MsgGameOver, Result: "1/2-1/2", Reason: ReasonAgreement,
		WhiteMS: r.whiteMS, BlackMS: r.blackMS,
	})
}

func (h *Hub) handleTakebackRequest(r *Room, p *Player) {
	if r.game == nil || !r.started || r.ended {
		return
	}
	if len(r.game.Moves()) == 0 {
		return
	}
	r.pendingTakebackFrom = p
	other := opponentOf(r, p)
	if other != nil {
		h.send(other, Envelope{Type: MsgTakebackOffer, From: displayOf(p)})
	}
}

func (h *Hub) handleTakebackAccept(r *Room, p *Player) {
	if r.pendingTakebackFrom == nil || r.pendingTakebackFrom == p {
		return
	}
	requester := r.pendingTakebackFrom
	r.pendingTakebackFrom = nil
	if r.game == nil {
		return
	}
	// Pop half-moves until the requester is on move again. notnil's
	// Game doesn't expose Undo on the public surface (varies by
	// version), so reconstruct from the move list.
	all := r.game.Moves()
	if len(all) == 0 {
		return
	}
	// Determine how many half-moves to drop so it becomes requester's
	// turn. requester wants to redo their own move → drop their last
	// move AND any reply by the opponent. If the requester is the
	// side-to-move now (didn't get to play yet), one drop is enough.
	requesterColor := notnil.White
	if requester == r.black {
		requesterColor = notnil.Black
	}
	target := requesterColor // we want the side-to-move to be requesterColor
	keep := len(all)
	for keep > 0 {
		pos := startingThenReplay(all, keep)
		if pos.Turn() == target {
			break
		}
		keep--
	}
	// Rebuild the game.
	g := notnil.NewGame()
	for i := 0; i < keep; i++ {
		if err := g.Move(all[i]); err != nil {
			// Defensive — replay should always succeed.
			break
		}
	}
	r.game = g
	r.lastMoveAt = h.timeNow()
	// Broadcast a synthetic clock-sync with the new FEN. UCI is left
	// empty so clients know this is a takeback rather than a move.
	h.broadcast(r, Envelope{
		Type: MsgClockSync, FEN: r.game.FEN(),
		WhiteMS: r.whiteMS, BlackMS: r.blackMS,
	})
	if r.initialMS > 0 {
		h.armFlagTimer(r)
	}
}

// startingThenReplay re-plays `keep` half-moves from the starting
// position and returns the resulting position. Used by takeback to
// peek at whose turn it would be after dropping a tail.
func startingThenReplay(all []*notnil.Move, keep int) *notnil.Position {
	g := notnil.NewGame()
	for i := 0; i < keep; i++ {
		if err := g.Move(all[i]); err != nil {
			return g.Position()
		}
	}
	return g.Position()
}

// Leave is invoked by the WS handler when the connection closes. The
// hub cleans up the player's slot and notifies the opponent.
func (h *Hub) Leave(p *Player) {
	h.mu.Lock()
	defer h.mu.Unlock()
	r := p.room
	if r == nil {
		// Quick-match queue waiter who never made it to a room.
		if h.queue == p {
			h.queue = nil
		}
		h.closePlayer(p)
		return
	}
	// Spectator?
	if p.Spectator {
		for i, sp := range r.spectators {
			if sp == p {
				r.spectators = append(r.spectators[:i], r.spectators[i+1:]...)
				break
			}
		}
		h.closePlayer(p)
		return
	}
	// One of the players.
	if r.white == p {
		r.white = nil
	}
	if r.black == p {
		r.black = nil
	}
	if r.started && !r.ended {
		// Forfeit by abandonment.
		r.ended = true
		if r.flagTimer != nil {
			r.flagTimer.Stop()
			r.flagTimer = nil
		}
		result := "0-1"
		if p == r.black || p.color == ColorBlack {
			result = "1-0"
		}
		other := r.white
		if other == nil {
			other = r.black
		}
		if other != nil {
			h.send(other, Envelope{
				Type: MsgGameOver, Result: result, Reason: ReasonOpponentLeft,
				WhiteMS: r.whiteMS, BlackMS: r.blackMS,
			})
		}
		for _, sp := range r.spectators {
			h.send(sp, Envelope{
				Type: MsgGameOver, Result: result, Reason: ReasonOpponentLeft,
				WhiteMS: r.whiteMS, BlackMS: r.blackMS,
			})
		}
	} else if !r.started {
		// Pre-game: tell the remaining player and reset the room
		// rather than ending it; they can wait for another joiner.
		other := r.white
		if other == nil {
			other = r.black
		}
		if other != nil {
			h.send(other, Envelope{Type: MsgOpponentLeft})
		}
	}
	h.closePlayer(p)
	// If both seats are empty (and no spectators), GC the room.
	if r.white == nil && r.black == nil && len(r.spectators) == 0 {
		delete(h.rooms, r.Code)
		if r.flagTimer != nil {
			r.flagTimer.Stop()
			r.flagTimer = nil
		}
	}
}

func (h *Hub) closePlayer(p *Player) {
	defer func() { _ = recover() }() // double-close on Out is benign
	close(p.Out)
	p.markDone()
}

// send tries to enqueue an envelope to a player. Non-blocking — if
// the outbox is full, we drop the frame. Caller holds hub.mu.
func (h *Hub) send(p *Player, e Envelope) {
	if p == nil {
		return
	}
	buf, err := Encode(e)
	if err != nil {
		return
	}
	select {
	case p.Out <- buf:
	default:
		// outbox full → drop. Clients reconnect when they notice.
	}
}

// broadcast sends an envelope to both players and all spectators in
// the room. Caller holds hub.mu.
func (h *Hub) broadcast(r *Room, e Envelope) {
	if r.white != nil {
		h.send(r.white, e)
	}
	if r.black != nil {
		h.send(r.black, e)
	}
	for _, sp := range r.spectators {
		h.send(sp, e)
	}
}

// opponentOf returns the other seated player (nil for spectators or
// solo rooms).
func opponentOf(r *Room, p *Player) *Player {
	if r.white == p {
		return r.black
	}
	if r.black == p {
		return r.white
	}
	return nil
}

func displayOf(p *Player) string {
	if p == nil {
		return ""
	}
	if p.DisplayName != "" {
		return p.DisplayName
	}
	return p.Username
}

// validateUCI mirrors `games/chess.ValidateUCI` but inline here so the
// netchess package doesn't depend on the LLM helper package.
func validateUCI(game *notnil.Game, uci string) (*notnil.Move, error) {
	for _, mv := range game.ValidMoves() {
		from := mv.S1().String()
		to := mv.S2().String()
		promo := ""
		switch mv.Promo() {
		case notnil.Queen:
			promo = "q"
		case notnil.Rook:
			promo = "r"
		case notnil.Bishop:
			promo = "b"
		case notnil.Knight:
			promo = "n"
		}
		candidate := from + to + promo
		if candidate == uci {
			return mv, nil
		}
	}
	return nil, fmt.Errorf("illegal move: %s", uci)
}

// reasonFromMethod maps notnil's draw / mate method to one of our
// stable wire constants.
func reasonFromMethod(m notnil.Method) string {
	switch m {
	case notnil.Checkmate:
		return ReasonCheckmate
	case notnil.Stalemate:
		return ReasonStalemate
	case notnil.InsufficientMaterial:
		return ReasonInsufficient
	case notnil.FiftyMoveRule, notnil.SeventyFiveMoveRule:
		return ReasonFiftyMove
	case notnil.ThreefoldRepetition, notnil.FivefoldRepetition:
		return ReasonThreefold
	case notnil.DrawOffer:
		return ReasonAgreement
	default:
		return ""
	}
}

// RoomCount returns the number of live rooms. Exposed for diagnostics
// / tests.
func (h *Hub) RoomCount() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.rooms)
}
