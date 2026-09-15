package handlers

import (
	"strings"
	"testing"
)

func TestMsToNs(t *testing.T) {
	if msToNs(0) != 0 || msToNs(-5) != 0 {
		t.Fatal("non-positive ms must map to 0")
	}
	if msToNs(1737504000000) != 1737504000000000000 {
		t.Fatalf("got %d", msToNs(1737504000000))
	}
}

func TestExtractResponseCodesRejectedWith(t *testing.T) {
	got := extractResponseCodes("find all calls in the last 15 minutes that were rejected with 608 response codes")
	if got != "608" {
		t.Fatalf("expected response_code 608, got %q", got)
	}
}

func TestExtractResponseCodesMultiple(t *testing.T) {
	got := extractResponseCodes("show 608 or 486 responses")
	if got != "608,486" {
		t.Fatalf("expected response_code 608,486, got %q", got)
	}
}

func TestExtractResponseCodesIgnoresPortAndMinutes(t *testing.T) {
	got := extractResponseCodes("find INVITE on port 5060 in the last 15 minutes")
	if got != "" {
		t.Fatalf("expected empty response_code, got %q", got)
	}
}

func TestExtractCIDIsSeparateFromCallID(t *testing.T) {
	got := extractCID("find cid abc-123-xyz")
	if got != "abc-123-xyz" {
		t.Fatalf("expected cid abc-123-xyz, got %q", got)
	}
	if callID := extractCallID("find cid abc-123-xyz"); callID != "" {
		t.Fatalf("expected empty call_id (cid must not alias into call_id), got %q", callID)
	}
}

func TestExtractCallIDDoesNotMatchBareCID(t *testing.T) {
	// Regression guard: extractCallID used to treat bare "cid" as a
	// call_id/session_id synonym, so a "find cid X" query silently
	// searched the wrong column (Call-ID instead of the distinct cid
	// field). cid must only ever be picked up by extractCID.
	if got := extractCallID("find cid abc-123-xyz"); got != "" {
		t.Fatalf("expected extractCallID to ignore bare cid, got %q", got)
	}
}

func TestBuildMCPRawSQL_CIDMatchesOnlyCIDColumn(t *testing.T) {
	req := SearchObjectV4{}
	req.Filter.CID = "abc-123-xyz"

	sql := buildMCPRawSQL("homer_lake", &req)
	if !strings.Contains(sql, "cid") {
		t.Fatalf("expected a cid clause, got:\n%s", sql)
	}
	if strings.Contains(sql, "session_id") {
		t.Fatalf("expected cid to NOT also match session_id (that's call_id's job), got:\n%s", sql)
	}
}

func TestExtractUserAgentInvolving(t *testing.T) {
	got := extractUserAgent("show all calls involving 'Linphone'")
	if got != "Linphone" {
		t.Fatalf("expected user_agent 'Linphone', got %q", got)
	}
}

func TestExtractUserAgentKeyword(t *testing.T) {
	got := extractUserAgent(`find calls with user agent "Asterisk PBX"`)
	if got != "Asterisk PBX" {
		t.Fatalf("expected user_agent 'Asterisk PBX', got %q", got)
	}
}

func TestExtractUserAgentWhereIs(t *testing.T) {
	got := extractUserAgent(`where user agent is "Linphone"`)
	if got != "Linphone" {
		t.Fatalf("expected user_agent 'Linphone', got %q", got)
	}
}

func TestBuildMCPRawSQL_UserAgentUsesSubstringMatch(t *testing.T) {
	req := SearchObjectV4{}
	req.Filter.UserAgent = "Linphone"

	sql := buildMCPRawSQL("homer_lake", &req)
	// Regression guard: sqlFormMatchOne only emits LIKE when the value
	// already contains a literal '%' — without explicit wrapping this
	// degenerates into an exact '=' match that would miss any header
	// string that isn't identical to the search term.
	if !strings.Contains(sql, "json_extract_string(data_extra, '$.user_agent') LIKE '%Linphone%'") {
		t.Fatalf("expected substring LIKE match for user_agent, got:\n%s", sql)
	}
}
