// Copyright (C) 2025 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package netris

import (
	"errors"
	"fmt"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"
)

// Default knobs — exported so coordinator config can override.
const (
	DefaultPlayerOutbox    = 64
	DefaultRoomCapacity    = 2
	DefaultWaitingTimeout  = 60 * time.Second
	maxRoomCodeLen         = 64
)

// ErrRoomFull is returned by JoinRoom when the named room already has
// two players. The handler maps this to a one-shot "error" frame and
// closes the socket — no implicit kick of the existing pair.
var ErrRoomFull = errors.New("netris: room is full")

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

// Hub holds all live netris rooms and the quick-match queue. One Hub
// per coordinator process is plenty — rooms are tiny and matchmaking
// happens under a single mutex.
type Hub struct {
	mu        sync.Mutex
	rooms     map[string]*Room
	queue     *Player // single player waiting for quick-match (nil = empty queue)
	quickSeq  uint64
	rng       *rand.Rand
	rngMu     sync.Mutex
	cfg       Config
	timeNow   func() time.Time // tests inject a fake clock
}

// NewHub builds an empty hub with the supplied config. A separate
// rng with a stable seed is allocated so the same Hub gives
// deterministic garbage hole columns across two test runs when the
// caller seeds it explicitly.
func NewHub(cfg Config) *Hub {
	return &Hub{
		rooms:   make(map[string]*Room),
		rng:     rand.New(rand.NewSource(time.Now().UnixNano())),
		cfg:     cfg,
		timeNow: time.Now,
	}
}

// SeedRNG lets tests pin the random hole columns to a known sequence.
func (h *Hub) SeedRNG(seed int64) {
	h.rngMu.Lock()
	defer h.rngMu.Unlock()
	h.rng = rand.New(rand.NewSource(seed))
}

// rngHole grabs a hole column under the rng mutex (rand.Rand is not
// safe for concurrent use). Public for tests via RandomHole.
func (h *Hub) rngHole() int {
	h.rngMu.Lock()
	defer h.rngMu.Unlock()
	return RandomHoleColumn(h.rng)
}

// Player is a single live socket. The handler owns Out and reads
// from it in its write-pump goroutine; the hub writes to Out and to
// no other goroutine — once Out is closed, no further sends are
// allowed.
type Player struct {
	Username    string
	DisplayName string
	Out         chan []byte

	hub      *Hub
	room     *Room
	ready    bool
	doneOnce sync.Once
	done     chan struct{}
}

func (p *Player) markDone() {
	p.doneOnce.Do(func() { close(p.done) })
}

// Done returns a channel closed when the player has been removed
// from its room (either by Leave or because the room ended).
func (p *Player) Done() <-chan struct{} { return p.done }

// Room is a 2-player relay session. The room is created lazily by
// the first player to JoinRoom/JoinQuick and torn down when both
// players have left.
//
// Rooms have no goroutine of their own: all transitions (join, leave,
// frame routing) happen inline on the calling goroutine while
// holding hub.mu, then enqueueing pre-rendered envelopes to the
// recipient's Out channel. This keeps the locking shape simple:
//   - hub.mu is held while mutating rooms / queue;
//   - per-player Out channel writes are non-blocking (DropOldest)
//     so a slow client never wedges the hub.
type Room struct {
	Code      string
	Quick     bool
	createdAt time.Time

	p1 *Player
	p2 *Player

	// timer fires waiting_timeout if the second player never shows.
	// Cancelled on pair / on lone leave.
	waitTimer *time.Timer

	// in_play once both players have sent "ready". Garbage / opponent
	// relay frames are only forwarded after start.
	inPlay bool
	// ended once a player has topped_out or disconnected; further
	// frames are silently dropped.
	ended bool
}

// JoinRoom places a player into a named room (creating it if needed).
// Returns the room and a "matched" envelope to be sent back to the
// caller (and to the existing player, if any). When the second
// player joins, both sockets get an identical "matched" frame and
// the waiting timer is cancelled.
func (h *Hub) JoinRoom(code string, p *Player) (*Room, error) {
	if len(code) > maxRoomCodeLen {
		code = code[:maxRoomCodeLen]
	}
	h.mu.Lock()
	defer h.mu.Unlock()

	r, ok := h.rooms[code]
	if !ok {
		r = &Room{
			Code:      code,
			Quick:     false,
			createdAt: h.timeNow(),
			p1:        p,
		}
		h.rooms[code] = r
		p.room = r
		p.hub = h
		h.armWaitTimer(r)
		// Tell the lone player they're in but waiting.
		h.send(p, Envelope{Type: MsgMatched, Room: code, You: displayOf(p), Opponent: ""})
		return r, nil
	}

	if r.p1 != nil && r.p2 != nil {
		return nil, ErrRoomFull
	}
	r.p2 = p
	p.room = r
	p.hub = h
	h.cancelWaitTimer(r)
	h.notifyMatched(r)
	return r, nil
}

// JoinQuick puts the player into the quick-match queue. If a player
// is already waiting, the two are paired into an anonymous room
// "quick-<n>" and matched frames go out to both. Otherwise the
// caller becomes the new queued player and a waiting timer starts.
func (h *Hub) JoinQuick(p *Player) (*Room, error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.queue == nil {
		h.queue = p
		p.hub = h
		// Build a placeholder room so a lone leaver can be cleaned up
		// with the same code path as a named-room leaver. The room
		// code is rewritten to its quick-<n> form when the second
		// player arrives.
		code := fmt.Sprintf("quick-pending-%d", atomic.AddUint64(&h.quickSeq, 1))
		r := &Room{
			Code:      code,
			Quick:     true,
			createdAt: h.timeNow(),
			p1:        p,
		}
		h.rooms[code] = r
		p.room = r
		h.armWaitTimer(r)
		h.send(p, Envelope{Type: MsgMatched, Room: code, You: displayOf(p), Opponent: ""})
		return r, nil
	}

	// Pair with the queued player.
	first := h.queue
	h.queue = nil

	// Promote the queued player's pending room to a real quick room
	// rather than building a brand-new one — keeps the player's
	// Player.room pointer valid throughout.
	r := first.room
	if r == nil {
		// Defensive: should never happen because JoinQuick always
		// installs a pending room. Fall through to a fresh room.
		code := fmt.Sprintf("quick-%d", atomic.AddUint64(&h.quickSeq, 1))
		r = &Room{Code: code, Quick: true, createdAt: h.timeNow(), p1: first}
		h.rooms[code] = r
		first.room = r
	}
	delete(h.rooms, r.Code)
	r.Code = fmt.Sprintf("quick-%d", atomic.AddUint64(&h.quickSeq, 1))
	h.rooms[r.Code] = r

	r.p2 = p
	p.room = r
	p.hub = h
	h.cancelWaitTimer(r)
	h.notifyMatched(r)
	return r, nil
}

// notifyMatched sends both players the matched frame with the actual
// opponent name filled in. Caller holds hub.mu.
func (h *Hub) notifyMatched(r *Room) {
	if r.p1 == nil || r.p2 == nil {
		return
	}
	h.send(r.p1, Envelope{Type: MsgMatched, Room: r.Code, You: displayOf(r.p1), Opponent: displayOf(r.p2)})
	h.send(r.p2, Envelope{Type: MsgMatched, Room: r.Code, You: displayOf(r.p2), Opponent: displayOf(r.p1)})
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
	if r.p2 != nil || r.ended {
		return
	}
	r.ended = true
	if r.p1 != nil {
		h.send(r.p1, Envelope{Type: MsgWaitingTimeout})
		// Pop from quick queue if this was a lone quick waiter.
		if h.queue == r.p1 {
			h.queue = nil
		}
	}
	delete(h.rooms, r.Code)
}

// Leave detaches a player from its room. If their opponent is still
// connected they receive an "opponent_left" frame and (if the game
// had started) a "win" frame. Caller-safe to invoke on disconnect
// or on graceful close.
func (h *Hub) Leave(p *Player) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.leaveLocked(p)
}

func (h *Hub) leaveLocked(p *Player) {
	r := p.room
	if r == nil {
		// Player never joined (e.g. early read error).
		p.markDone()
		closeOutbox(p)
		return
	}
	p.room = nil

	// Lone-quick-queue case: the queued player aborts before pair.
	if h.queue == p {
		h.queue = nil
	}

	other := r.opponentOf(p)
	switch p {
	case r.p1:
		r.p1 = nil
	case r.p2:
		r.p2 = nil
	}

	if other != nil {
		h.send(other, Envelope{Type: MsgOpponentLeft})
		if r.inPlay && !r.ended {
			h.send(other, Envelope{Type: MsgWin, Reason: "opponent_left"})
			r.ended = true
		}
	}

	if r.p1 == nil && r.p2 == nil {
		h.cancelWaitTimer(r)
		delete(h.rooms, r.Code)
	}

	p.markDone()
	closeOutbox(p)
}

// HandleFrame is the one entry point the WS handler calls for every
// incoming frame. It applies relay rules (n-1 garbage), updates
// readiness state and broadcasts as needed. Unknown / bad frames
// are silently ignored — clients are advisory, not authoritative.
func (h *Hub) HandleFrame(p *Player, e Envelope) {
	h.mu.Lock()
	defer h.mu.Unlock()
	r := p.room
	if r == nil || r.ended {
		return
	}
	other := r.opponentOf(p)

	switch e.Type {
	case MsgHello:
		// Allow override only before play starts; afterwards the
		// label is sticky to keep both clients consistent.
		if !r.inPlay {
			if name := SanitiseDisplayName(e.DisplayName); name != "" {
				p.DisplayName = name
				if other != nil {
					h.notifyMatched(r) // refresh both labels
				}
			}
		}

	case MsgReady:
		p.ready = true
		if !r.inPlay && r.p1 != nil && r.p2 != nil && r.p1.ready && r.p2.ready {
			r.inPlay = true
			h.send(r.p1, Envelope{Type: MsgStart})
			h.send(r.p2, Envelope{Type: MsgStart})
		}

	case MsgLineClear:
		if !r.inPlay || other == nil {
			return
		}
		k := GarbageLinesFor(e.Cleared)
		if k <= 0 {
			return
		}
		hole := h.rngHole()
		h.send(other, Envelope{Type: MsgGarbageIn, Lines: k, Hole: hole})

	case MsgBoard:
		if other == nil {
			return
		}
		// Cap relay payload — opponent_board is a glance, not a
		// full board log. 4KiB ought to be plenty for a base64'd
		// 10×20 bitmap; anything larger is dropped.
		if len(e.Cells) > 4096 {
			return
		}
		h.send(other, Envelope{Type: MsgOpponentBoard, Cells: e.Cells})

	case MsgScore:
		if other == nil {
			return
		}
		h.send(other, Envelope{
			Type:  MsgOpponentScore,
			Score: e.Score,
			Lines: e.Lines,
			Level: e.Level,
		})

	case MsgChat:
		if other == nil {
			return
		}
		text := SanitiseDisplayName(e.Text) // same trimming/capping rules
		if text == "" {
			return
		}
		h.send(other, Envelope{Type: MsgChat, From: displayOf(p), Text: text})

	case MsgToppedOut:
		if r.ended {
			return
		}
		r.ended = true
		if other != nil {
			h.send(other, Envelope{Type: MsgWin, Reason: "opponent_topped_out"})
		}
		h.send(p, Envelope{Type: MsgLose, Reason: "topped_out"})

	default:
		// Unknown type — ignore silently.
	}
}

// send marshals an envelope and pushes it to the player's outbox,
// dropping the oldest frame if the outbox is full so a slow client
// can never wedge the hub. Caller holds hub.mu.
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
		// Drop-oldest backpressure — pop one and try again. Two
		// consecutive failures means the writer goroutine is dead;
		// give up and let Leave clean up on next disconnect.
		select {
		case <-p.Out:
		default:
		}
		select {
		case p.Out <- buf:
		default:
		}
	}
}

// closeOutbox is safe to call once per Player; double-close panics
// would only happen if Leave were invoked twice for the same player,
// but we still guard with sync.Once at the marker.
func closeOutbox(p *Player) {
	defer func() {
		// Recover from a double-close — handler may close on its
		// own end as a belt-and-braces; the second close is a no-op
		// from the hub's perspective.
		_ = recover()
	}()
	close(p.Out)
}

func (r *Room) opponentOf(p *Player) *Player {
	switch p {
	case r.p1:
		return r.p2
	case r.p2:
		return r.p1
	default:
		return nil
	}
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

// NewPlayer is a convenience constructor used by the WS handler.
// outbox > 0 lets the handler size the per-socket buffer to its
// observed write throughput; 0 falls back to DefaultPlayerOutbox.
func NewPlayer(username, display string, outbox int) *Player {
	if outbox <= 0 {
		outbox = DefaultPlayerOutbox
	}
	dn := SanitiseDisplayName(display)
	if dn == "" {
		dn = username
	}
	return &Player{
		Username:    username,
		DisplayName: dn,
		Out:         make(chan []byte, outbox),
		done:        make(chan struct{}),
	}
}

// Stats returns a point-in-time snapshot for /modules / observability.
type Stats struct {
	Rooms       int
	WaitingQuick bool
}

func (h *Hub) Stats() Stats {
	h.mu.Lock()
	defer h.mu.Unlock()
	return Stats{Rooms: len(h.rooms), WaitingQuick: h.queue != nil}
}
