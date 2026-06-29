package ducklake

import "testing"

func TestGetSIPType_REFERGoesToCall(t *testing.T) {
	if got := GetSIPType("REFER"); got != SIPTypeCall {
		t.Fatalf("GetSIPType(REFER) = %q, want %q", got, SIPTypeCall)
	}
}

func TestGetSIPType_CallMethods(t *testing.T) {
	for _, m := range []string{"INVITE", "ACK", "BYE", "CANCEL", "UPDATE", "PRACK", "INFO", "REFER"} {
		if got := GetSIPType(m); got != SIPTypeCall {
			t.Errorf("GetSIPType(%q) = %q, want call", m, got)
		}
	}
}

func TestGetSIPType_DefaultMethods(t *testing.T) {
	for _, m := range []string{"OPTIONS", "NOTIFY", "SUBSCRIBE", "PUBLISH", "MESSAGE"} {
		if got := GetSIPType(m); got != SIPTypeDefault {
			t.Errorf("GetSIPType(%q) = %q, want default", m, got)
		}
	}
}
