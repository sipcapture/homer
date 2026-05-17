// Copyright (C) 2025 Homer Server Contributors

package netchess

import (
	"encoding/json"
	"testing"
	"time"
)

// drain pulls every queued envelope off the player's outbox into a
// slice — destructive read, useful for asserting frames after a
// hub action. Test-only helper.
func drain(p *Player) []Envelope {
	var out []Envelope
	for {
		select {
		case buf, ok := <-p.Out:
			if !ok {
				return out
			}
			var e Envelope
			if err := json.Unmarshal(buf, &e); err == nil {
				out = append(out, e)
			}
		default:
			return out
		}
	}
}

func newTestPlayer(name string) *Player { return NewPlayer(name, "", 32) }

func TestQuickMatchPairsTwoPlayers(t *testing.T) {
	h := NewHub(Config{WaitingTimeout: 10 * time.Second})
	h.SeedRNG(1)

	alice := newTestPlayer("alice")
	bob := newTestPlayer("bob")

	if _, err := h.JoinQuick(alice, JoinOptions{}); err != nil {
		t.Fatalf("alice join: %v", err)
	}
	if _, err := h.JoinQuick(bob, JoinOptions{}); err != nil {
		t.Fatalf("bob join: %v", err)
	}

	aliceMsgs := drain(alice)
	bobMsgs := drain(bob)

	if len(aliceMsgs) < 1 || aliceMsgs[len(aliceMsgs)-1].Type != MsgMatched ||
		aliceMsgs[len(aliceMsgs)-1].Opponent == "" {
		t.Fatalf("alice didn't receive matched-with-opponent: %+v", aliceMsgs)
	}
	if len(bobMsgs) < 1 || bobMsgs[len(bobMsgs)-1].Type != MsgMatched ||
		bobMsgs[len(bobMsgs)-1].Opponent == "" {
		t.Fatalf("bob didn't receive matched-with-opponent: %+v", bobMsgs)
	}
	// Colours must differ.
	if aliceMsgs[len(aliceMsgs)-1].Color == bobMsgs[len(bobMsgs)-1].Color {
		t.Fatalf("both players got same colour: alice=%s bob=%s",
			aliceMsgs[len(aliceMsgs)-1].Color, bobMsgs[len(bobMsgs)-1].Color)
	}
}

func TestRoomMatchAndStartOnBothReady(t *testing.T) {
	h := NewHub(Config{WaitingTimeout: 10 * time.Second})
	a := newTestPlayer("a")
	b := newTestPlayer("b")
	if _, err := h.JoinRoom("private", a, JoinOptions{ColorPref: ColorWhite}); err != nil {
		t.Fatalf("a join: %v", err)
	}
	if _, err := h.JoinRoom("private", b, JoinOptions{}); err != nil {
		t.Fatalf("b join: %v", err)
	}
	_ = drain(a)
	_ = drain(b)

	h.HandleFrame(a, Envelope{Type: MsgReady})
	h.HandleFrame(b, Envelope{Type: MsgReady})

	aMsgs := drain(a)
	bMsgs := drain(b)
	if len(aMsgs) == 0 || aMsgs[0].Type != MsgStart {
		t.Fatalf("a didn't receive start frame: %+v", aMsgs)
	}
	if len(bMsgs) == 0 || bMsgs[0].Type != MsgStart {
		t.Fatalf("b didn't receive start frame: %+v", bMsgs)
	}
	if aMsgs[0].FEN == "" || aMsgs[0].WhiteMS <= 0 {
		t.Fatalf("start frame missing FEN/clock: %+v", aMsgs[0])
	}
}

func TestLegalMoveBroadcastsToOpponent(t *testing.T) {
	h := NewHub(Config{WaitingTimeout: 10 * time.Second})
	a := newTestPlayer("a")
	b := newTestPlayer("b")
	_, _ = h.JoinRoom("r1", a, JoinOptions{ColorPref: ColorWhite})
	_, _ = h.JoinRoom("r1", b, JoinOptions{})
	h.HandleFrame(a, Envelope{Type: MsgReady})
	h.HandleFrame(b, Envelope{Type: MsgReady})
	_ = drain(a)
	_ = drain(b)

	// White plays e2e4.
	h.HandleFrame(a, Envelope{Type: MsgMove, UCI: "e2e4"})

	for _, p := range []*Player{a, b} {
		msgs := drain(p)
		if len(msgs) == 0 {
			t.Fatalf("expected opponent_move broadcast, %s got nothing", p.Username)
		}
		last := msgs[len(msgs)-1]
		if last.Type != MsgOpponentMove || last.UCI != "e2e4" || last.FEN == "" {
			t.Fatalf("%s wrong frame: %+v", p.Username, last)
		}
	}
}

func TestIllegalMoveIsRejectedWithError(t *testing.T) {
	h := NewHub(Config{WaitingTimeout: 10 * time.Second})
	a := newTestPlayer("a")
	b := newTestPlayer("b")
	_, _ = h.JoinRoom("r2", a, JoinOptions{ColorPref: ColorWhite})
	_, _ = h.JoinRoom("r2", b, JoinOptions{})
	h.HandleFrame(a, Envelope{Type: MsgReady})
	h.HandleFrame(b, Envelope{Type: MsgReady})
	_ = drain(a)
	_ = drain(b)

	// White tries a 3-square pawn move.
	h.HandleFrame(a, Envelope{Type: MsgMove, UCI: "e2e5"})

	aMsgs := drain(a)
	if len(aMsgs) == 0 || aMsgs[len(aMsgs)-1].Type != MsgError {
		t.Fatalf("expected error frame, got %+v", aMsgs)
	}
	// Opponent must not have been told anything.
	if bMsgs := drain(b); len(bMsgs) != 0 {
		t.Fatalf("opponent saw something for illegal move: %+v", bMsgs)
	}
}

func TestWrongTurnIsRejected(t *testing.T) {
	h := NewHub(Config{WaitingTimeout: 10 * time.Second})
	a := newTestPlayer("a")
	b := newTestPlayer("b")
	_, _ = h.JoinRoom("r3", a, JoinOptions{ColorPref: ColorWhite})
	_, _ = h.JoinRoom("r3", b, JoinOptions{})
	h.HandleFrame(a, Envelope{Type: MsgReady})
	h.HandleFrame(b, Envelope{Type: MsgReady})
	_ = drain(a)
	_ = drain(b)

	// Black tries to move first.
	h.HandleFrame(b, Envelope{Type: MsgMove, UCI: "e7e5"})

	bMsgs := drain(b)
	if len(bMsgs) == 0 || bMsgs[len(bMsgs)-1].Type != MsgError {
		t.Fatalf("expected wrong-turn error, got %+v", bMsgs)
	}
}

func TestResignEndsGame(t *testing.T) {
	h := NewHub(Config{WaitingTimeout: 10 * time.Second})
	a := newTestPlayer("a")
	b := newTestPlayer("b")
	_, _ = h.JoinRoom("r4", a, JoinOptions{ColorPref: ColorWhite})
	_, _ = h.JoinRoom("r4", b, JoinOptions{})
	h.HandleFrame(a, Envelope{Type: MsgReady})
	h.HandleFrame(b, Envelope{Type: MsgReady})
	_ = drain(a)
	_ = drain(b)

	h.HandleFrame(a, Envelope{Type: MsgResign})
	for _, p := range []*Player{a, b} {
		msgs := drain(p)
		if len(msgs) == 0 {
			t.Fatalf("%s no game_over after resign", p.Username)
		}
		last := msgs[len(msgs)-1]
		if last.Type != MsgGameOver || last.Reason != ReasonResignation || last.Result != "0-1" {
			t.Fatalf("%s wrong game_over after white resign: %+v", p.Username, last)
		}
	}
}

func TestDrawOfferAndAccept(t *testing.T) {
	h := NewHub(Config{WaitingTimeout: 10 * time.Second})
	a := newTestPlayer("a")
	b := newTestPlayer("b")
	_, _ = h.JoinRoom("r5", a, JoinOptions{ColorPref: ColorWhite})
	_, _ = h.JoinRoom("r5", b, JoinOptions{})
	h.HandleFrame(a, Envelope{Type: MsgReady})
	h.HandleFrame(b, Envelope{Type: MsgReady})
	_ = drain(a)
	_ = drain(b)

	h.HandleFrame(a, Envelope{Type: MsgDrawOffer})
	bMsgs := drain(b)
	if len(bMsgs) == 0 || bMsgs[len(bMsgs)-1].Type != MsgDrawOffered {
		t.Fatalf("expected draw_offered to opponent, got %+v", bMsgs)
	}

	h.HandleFrame(b, Envelope{Type: MsgDrawAccept})
	for _, p := range []*Player{a, b} {
		msgs := drain(p)
		if len(msgs) == 0 || msgs[len(msgs)-1].Type != MsgGameOver || msgs[len(msgs)-1].Result != "1/2-1/2" {
			t.Fatalf("%s missing 1/2-1/2 after draw accept: %+v", p.Username, msgs)
		}
	}
}

func TestFoolsMateGameOver(t *testing.T) {
	h := NewHub(Config{WaitingTimeout: 10 * time.Second})
	a := newTestPlayer("a")
	b := newTestPlayer("b")
	_, _ = h.JoinRoom("foolsmate", a, JoinOptions{ColorPref: ColorWhite})
	_, _ = h.JoinRoom("foolsmate", b, JoinOptions{})
	h.HandleFrame(a, Envelope{Type: MsgReady})
	h.HandleFrame(b, Envelope{Type: MsgReady})
	_ = drain(a)
	_ = drain(b)

	// 1. f3 e5 2. g4 Qh4#
	h.HandleFrame(a, Envelope{Type: MsgMove, UCI: "f2f3"})
	_ = drain(a)
	_ = drain(b)
	h.HandleFrame(b, Envelope{Type: MsgMove, UCI: "e7e5"})
	_ = drain(a)
	_ = drain(b)
	h.HandleFrame(a, Envelope{Type: MsgMove, UCI: "g2g4"})
	_ = drain(a)
	_ = drain(b)
	h.HandleFrame(b, Envelope{Type: MsgMove, UCI: "d8h4"})

	for _, p := range []*Player{a, b} {
		msgs := drain(p)
		if len(msgs) < 2 {
			t.Fatalf("%s expected move + game_over, got %+v", p.Username, msgs)
		}
		last := msgs[len(msgs)-1]
		if last.Type != MsgGameOver || last.Reason != ReasonCheckmate || last.Result != "0-1" {
			t.Fatalf("%s wrong fool's mate outcome: %+v", p.Username, last)
		}
	}
}

func TestRoomFullRejectsThirdPlayer(t *testing.T) {
	h := NewHub(Config{WaitingTimeout: 10 * time.Second})
	_, _ = h.JoinRoom("packed", newTestPlayer("a"), JoinOptions{})
	_, _ = h.JoinRoom("packed", newTestPlayer("b"), JoinOptions{})
	_, err := h.JoinRoom("packed", newTestPlayer("c"), JoinOptions{})
	if err == nil {
		t.Fatal("expected ErrRoomFull for third joiner")
	}
}

func TestLeaveBeforeStartLetsRoomerWait(t *testing.T) {
	h := NewHub(Config{WaitingTimeout: 10 * time.Second})
	a := newTestPlayer("a")
	b := newTestPlayer("b")
	_, _ = h.JoinRoom("waitroom", a, JoinOptions{})
	_, _ = h.JoinRoom("waitroom", b, JoinOptions{})
	_ = drain(a)
	_ = drain(b)

	h.Leave(b)

	aMsgs := drain(a)
	if len(aMsgs) == 0 || aMsgs[len(aMsgs)-1].Type != MsgOpponentLeft {
		t.Fatalf("expected opponent_left for a, got %+v", aMsgs)
	}
	if h.RoomCount() != 1 {
		t.Fatalf("room should still exist while a is in it, got %d rooms", h.RoomCount())
	}
}

func TestSpectatorJoinAndReceivesBroadcast(t *testing.T) {
	h := NewHub(Config{WaitingTimeout: 10 * time.Second})
	a := newTestPlayer("a")
	b := newTestPlayer("b")
	_, _ = h.JoinRoom("spec", a, JoinOptions{ColorPref: ColorWhite})
	_, _ = h.JoinRoom("spec", b, JoinOptions{})
	h.HandleFrame(a, Envelope{Type: MsgReady})
	h.HandleFrame(b, Envelope{Type: MsgReady})
	_ = drain(a)
	_ = drain(b)

	s := newTestPlayer("spec1")
	if _, err := h.JoinSpectator("spec", s); err != nil {
		t.Fatalf("spectator join: %v", err)
	}
	specInit := drain(s)
	if len(specInit) == 0 || !specInit[0].Spectator {
		t.Fatalf("spectator missing init frame: %+v", specInit)
	}

	h.HandleFrame(a, Envelope{Type: MsgMove, UCI: "e2e4"})
	msgs := drain(s)
	if len(msgs) == 0 || msgs[len(msgs)-1].Type != MsgOpponentMove {
		t.Fatalf("spectator didn't receive move broadcast: %+v", msgs)
	}

	// Spectator's own frames must be ignored.
	h.HandleFrame(s, Envelope{Type: MsgMove, UCI: "e7e5"})
	if msgs := drain(s); len(msgs) != 0 {
		t.Fatalf("spectator's send shouldn't echo: %+v", msgs)
	}
}

func TestProtocolEncodeDecodeRoundTrip(t *testing.T) {
	in := Envelope{Type: MsgMove, UCI: "e2e4", WhiteMS: 1234, BlackMS: 5678}
	buf, err := Encode(in)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	out, err := Decode(buf)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.Type != in.Type || out.UCI != in.UCI || out.WhiteMS != in.WhiteMS || out.BlackMS != in.BlackMS {
		t.Fatalf("round-trip mismatch: %+v vs %+v", in, out)
	}
}

func TestSanitiseDisplayNameTrimsAndClampsControlChars(t *testing.T) {
	if got := SanitiseDisplayName("  hello\x07world  "); got != "helloworld" {
		t.Fatalf("expected 'helloworld', got %q", got)
	}
	long := ""
	for i := 0; i < 50; i++ {
		long += "x"
	}
	if got := SanitiseDisplayName(long); len(got) != MaxDisplayNameLen {
		t.Fatalf("expected clamp to %d, got %d", MaxDisplayNameLen, len(got))
	}
}
