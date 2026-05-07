// Copyright (C) 2025 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package netris

import (
	"errors"
	"testing"
	"time"
)

// drainTypes pulls every frame currently waiting in the outbox and
// returns their Envelope.Type fields in order. Useful for asserting
// the sequence of server -> client messages without forcing tests to
// peer at field-by-field equality.
func drainTypes(t *testing.T, p *Player) []string {
	t.Helper()
	out := []string{}
	for {
		select {
		case buf, ok := <-p.Out:
			if !ok {
				return out
			}
			e, err := Decode(buf)
			if err != nil {
				t.Fatalf("decode: %v", err)
			}
			out = append(out, e.Type)
		case <-time.After(20 * time.Millisecond):
			return out
		}
	}
}

// drainOne reads exactly one frame with a small timeout so tests
// don't hang on misrouted messages.
func drainOne(t *testing.T, p *Player) Envelope {
	t.Helper()
	select {
	case buf, ok := <-p.Out:
		if !ok {
			t.Fatalf("outbox closed unexpectedly")
		}
		e, err := Decode(buf)
		if err != nil {
			t.Fatalf("decode: %v", err)
		}
		return e
	case <-time.After(200 * time.Millisecond):
		t.Fatalf("timed out waiting for a frame")
		return Envelope{}
	}
}

func TestGarbageLinesFor(t *testing.T) {
	cases := map[int]int{0: 0, 1: 0, 2: 1, 3: 2, 4: 4, 5: 4}
	for in, want := range cases {
		if got := GarbageLinesFor(in); got != want {
			t.Errorf("GarbageLinesFor(%d) = %d, want %d", in, got, want)
		}
	}
}

func TestSanitiseDisplayName(t *testing.T) {
	if SanitiseDisplayName("  alice  ") != "alice" {
		t.Errorf("trim failed")
	}
	if SanitiseDisplayName("\x01bad\x02") != "bad" {
		t.Errorf("control chars not stripped")
	}
	long := make([]byte, 64)
	for i := range long {
		long[i] = 'a'
	}
	if got := SanitiseDisplayName(string(long)); len(got) != 32 {
		t.Errorf("expected cap to 32, got %d", len(got))
	}
}

func TestRoomPairingByCode(t *testing.T) {
	hub := NewHub(Config{WaitingTimeout: time.Second})
	hub.SeedRNG(42)

	alice := NewPlayer("alice", "", 4)
	bob := NewPlayer("bob", "", 4)

	if _, err := hub.JoinRoom("abc", alice); err != nil {
		t.Fatalf("alice join: %v", err)
	}
	first := drainOne(t, alice)
	if first.Type != MsgMatched || first.Opponent != "" {
		t.Errorf("alice first frame should be matched-with-no-opponent, got %+v", first)
	}

	if _, err := hub.JoinRoom("abc", bob); err != nil {
		t.Fatalf("bob join: %v", err)
	}
	if e := drainOne(t, alice); e.Type != MsgMatched || e.Opponent != "bob" {
		t.Errorf("alice should see matched(bob), got %+v", e)
	}
	if e := drainOne(t, bob); e.Type != MsgMatched || e.Opponent != "alice" {
		t.Errorf("bob should see matched(alice), got %+v", e)
	}

	stats := hub.Stats()
	if stats.Rooms != 1 || stats.WaitingQuick {
		t.Errorf("unexpected stats: %+v", stats)
	}
}

func TestRoomFullRejectsThird(t *testing.T) {
	hub := NewHub(Config{WaitingTimeout: time.Second})
	a := NewPlayer("a", "", 4)
	b := NewPlayer("b", "", 4)
	c := NewPlayer("c", "", 4)
	if _, err := hub.JoinRoom("x", a); err != nil {
		t.Fatal(err)
	}
	if _, err := hub.JoinRoom("x", b); err != nil {
		t.Fatal(err)
	}
	if _, err := hub.JoinRoom("x", c); !errors.Is(err, ErrRoomFull) {
		t.Errorf("expected ErrRoomFull, got %v", err)
	}
}

func TestQuickMatchPairs(t *testing.T) {
	hub := NewHub(Config{WaitingTimeout: time.Second})
	a := NewPlayer("a", "", 4)
	b := NewPlayer("b", "", 4)

	if _, err := hub.JoinQuick(a); err != nil {
		t.Fatal(err)
	}
	if e := drainOne(t, a); e.Type != MsgMatched || e.Opponent != "" {
		t.Errorf("a expected lone matched, got %+v", e)
	}
	if !hub.Stats().WaitingQuick {
		t.Errorf("queue should mark waiting after first joiner")
	}

	if _, err := hub.JoinQuick(b); err != nil {
		t.Fatal(err)
	}
	if e := drainOne(t, a); e.Type != MsgMatched || e.Opponent != "b" {
		t.Errorf("a should see matched(b), got %+v", e)
	}
	if e := drainOne(t, b); e.Type != MsgMatched || e.Opponent != "a" {
		t.Errorf("b should see matched(a), got %+v", e)
	}
	if hub.Stats().WaitingQuick {
		t.Errorf("queue should be empty after pair")
	}
}

func TestReadyAndStart(t *testing.T) {
	hub := NewHub(Config{WaitingTimeout: time.Second})
	a := NewPlayer("a", "", 8)
	b := NewPlayer("b", "", 8)
	if _, err := hub.JoinRoom("r", a); err != nil {
		t.Fatal(err)
	}
	if _, err := hub.JoinRoom("r", b); err != nil {
		t.Fatal(err)
	}
	drainTypes(t, a)
	drainTypes(t, b)

	hub.HandleFrame(a, Envelope{Type: MsgReady})
	// Only one ready -> nothing yet.
	if got := drainTypes(t, a); len(got) != 0 {
		t.Errorf("unexpected frames after one ready: %v", got)
	}
	hub.HandleFrame(b, Envelope{Type: MsgReady})

	if e := drainOne(t, a); e.Type != MsgStart {
		t.Errorf("a expected start, got %+v", e)
	}
	if e := drainOne(t, b); e.Type != MsgStart {
		t.Errorf("b expected start, got %+v", e)
	}
}

func TestLineClearRoutesGarbageWithStableHole(t *testing.T) {
	hub := NewHub(Config{WaitingTimeout: time.Second})
	hub.SeedRNG(1) // deterministic hole picks

	a := NewPlayer("a", "", 8)
	b := NewPlayer("b", "", 8)
	if _, err := hub.JoinRoom("r", a); err != nil {
		t.Fatal(err)
	}
	if _, err := hub.JoinRoom("r", b); err != nil {
		t.Fatal(err)
	}
	hub.HandleFrame(a, Envelope{Type: MsgReady})
	hub.HandleFrame(b, Envelope{Type: MsgReady})
	drainTypes(t, a)
	drainTypes(t, b)

	// 1-line clear -> no garbage.
	hub.HandleFrame(a, Envelope{Type: MsgLineClear, Cleared: 1})
	if got := drainTypes(t, b); len(got) != 0 {
		t.Errorf("1-line clear should not send garbage, got %v", got)
	}

	// 4-line clear -> 4 garbage rows with a hole in [0, BoardCols).
	hub.HandleFrame(a, Envelope{Type: MsgLineClear, Cleared: 4})
	e := drainOne(t, b)
	if e.Type != MsgGarbageIn || e.Lines != 4 {
		t.Errorf("expected garbage_in lines=4, got %+v", e)
	}
	if e.Hole < 0 || e.Hole >= BoardCols {
		t.Errorf("hole %d out of range", e.Hole)
	}

	// Sender should not get its own garbage.
	if got := drainTypes(t, a); len(got) != 0 {
		t.Errorf("sender should not receive garbage, got %v", got)
	}
}

func TestOpponentLeftBroadcasts(t *testing.T) {
	hub := NewHub(Config{WaitingTimeout: time.Second})
	a := NewPlayer("a", "", 4)
	b := NewPlayer("b", "", 4)
	if _, err := hub.JoinRoom("r", a); err != nil {
		t.Fatal(err)
	}
	if _, err := hub.JoinRoom("r", b); err != nil {
		t.Fatal(err)
	}
	hub.HandleFrame(a, Envelope{Type: MsgReady})
	hub.HandleFrame(b, Envelope{Type: MsgReady})
	drainTypes(t, a)
	drainTypes(t, b)

	hub.Leave(a)
	got := drainTypes(t, b)
	// Expect opponent_left and a win frame in some order.
	saw := map[string]bool{}
	for _, t := range got {
		saw[t] = true
	}
	if !saw[MsgOpponentLeft] {
		t.Errorf("b should see opponent_left, got %v", got)
	}
	if !saw[MsgWin] {
		t.Errorf("b should see win, got %v", got)
	}

	if hub.Stats().Rooms != 1 {
		t.Errorf("room should still exist while b is connected")
	}
	hub.Leave(b)
	if hub.Stats().Rooms != 0 {
		t.Errorf("room should be cleaned after both leave")
	}
}

func TestWaitingTimeoutFiresOnLoneRoom(t *testing.T) {
	hub := NewHub(Config{WaitingTimeout: 30 * time.Millisecond})
	a := NewPlayer("a", "", 4)
	if _, err := hub.JoinRoom("solo", a); err != nil {
		t.Fatal(err)
	}
	// Drain the initial matched-no-opponent frame.
	drainOne(t, a)

	select {
	case buf, ok := <-a.Out:
		if !ok {
			t.Fatalf("outbox closed before timeout")
		}
		e, err := Decode(buf)
		if err != nil {
			t.Fatalf("decode: %v", err)
		}
		if e.Type != MsgWaitingTimeout {
			t.Errorf("expected waiting_timeout, got %s", e.Type)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("waiting_timeout never fired")
	}
}

func TestToppedOutSendsWinAndLose(t *testing.T) {
	hub := NewHub(Config{WaitingTimeout: time.Second})
	a := NewPlayer("a", "", 4)
	b := NewPlayer("b", "", 4)
	if _, err := hub.JoinRoom("r", a); err != nil {
		t.Fatal(err)
	}
	if _, err := hub.JoinRoom("r", b); err != nil {
		t.Fatal(err)
	}
	hub.HandleFrame(a, Envelope{Type: MsgReady})
	hub.HandleFrame(b, Envelope{Type: MsgReady})
	drainTypes(t, a)
	drainTypes(t, b)

	hub.HandleFrame(a, Envelope{Type: MsgToppedOut})
	if e := drainOne(t, a); e.Type != MsgLose {
		t.Errorf("a expected lose, got %+v", e)
	}
	if e := drainOne(t, b); e.Type != MsgWin {
		t.Errorf("b expected win, got %+v", e)
	}
	// Subsequent frames after end-of-game should be ignored.
	hub.HandleFrame(b, Envelope{Type: MsgLineClear, Cleared: 4})
	if got := drainTypes(t, a); len(got) != 0 {
		t.Errorf("post-end frames should be ignored, got %v", got)
	}
}

func TestBoardAndScoreRelay(t *testing.T) {
	hub := NewHub(Config{WaitingTimeout: time.Second})
	a := NewPlayer("a", "", 8)
	b := NewPlayer("b", "", 8)
	if _, err := hub.JoinRoom("r", a); err != nil {
		t.Fatal(err)
	}
	if _, err := hub.JoinRoom("r", b); err != nil {
		t.Fatal(err)
	}
	hub.HandleFrame(a, Envelope{Type: MsgReady})
	hub.HandleFrame(b, Envelope{Type: MsgReady})
	drainTypes(t, a)
	drainTypes(t, b)

	hub.HandleFrame(a, Envelope{Type: MsgBoard, Cells: "abc"})
	e := drainOne(t, b)
	if e.Type != MsgOpponentBoard || e.Cells != "abc" {
		t.Errorf("opponent_board mismatch: %+v", e)
	}

	hub.HandleFrame(a, Envelope{Type: MsgScore, Score: 1200, Lines: 5, Level: 2})
	e = drainOne(t, b)
	if e.Type != MsgOpponentScore || e.Score != 1200 || e.Lines != 5 || e.Level != 2 {
		t.Errorf("opponent_score mismatch: %+v", e)
	}
}

func TestHelloUpdatesDisplayName(t *testing.T) {
	hub := NewHub(Config{WaitingTimeout: time.Second})
	a := NewPlayer("alice", "", 8)
	b := NewPlayer("bob", "", 8)
	if _, err := hub.JoinRoom("r", a); err != nil {
		t.Fatal(err)
	}
	if _, err := hub.JoinRoom("r", b); err != nil {
		t.Fatal(err)
	}
	drainTypes(t, a)
	drainTypes(t, b)

	hub.HandleFrame(a, Envelope{Type: MsgHello, DisplayName: "AceA"})
	// Expect both clients to receive a refreshed matched frame so b
	// can update the opponent label.
	if e := drainOne(t, b); e.Type != MsgMatched || e.Opponent != "AceA" {
		t.Errorf("b should see refreshed matched(AceA), got %+v", e)
	}
}
