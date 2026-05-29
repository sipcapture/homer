package handlers

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/sipcapture/homer-core/src/coordinator/services"
)

func TestBuildExportExcludeIPClause_empty(t *testing.T) {
	if got := buildExportExcludeIPClause(nil); got != "" {
		t.Fatalf("got %q want empty", got)
	}
	if got := buildExportExcludeIPClause([]string{"", "  "}); got != "" {
		t.Fatalf("got %q want empty", got)
	}
}

func TestBuildExportExcludeIPClause_single(t *testing.T) {
	got := buildExportExcludeIPClause([]string{"10.0.0.1"})
	want := " AND src_ip != '10.0.0.1' AND dst_ip != '10.0.0.1'"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestBuildExportExcludeIPClause_multiple(t *testing.T) {
	got := buildExportExcludeIPClause([]string{"10.0.0.1", "192.168.1.5"})
	if got != " AND src_ip != '10.0.0.1' AND dst_ip != '10.0.0.1' AND src_ip != '192.168.1.5' AND dst_ip != '192.168.1.5'" {
		t.Fatalf("unexpected clause: %q", got)
	}
}

func TestBuildExportExcludeIPClause_escapesQuotes(t *testing.T) {
	got := buildExportExcludeIPClause([]string{"10.0.0.1'; DROP TABLE--"})
	if got == "" || got == " AND src_ip != '10.0.0.1'; DROP TABLE--' AND dst_ip != '10.0.0.1'; DROP TABLE--'" {
		t.Fatalf("unsafe or empty clause: %q", got)
	}
}

func TestBuildTransactionExportSQL_includesWhitelist(t *testing.T) {
	h := &SearchHandler{flightService: services.NewFlightService(nil, 0)}
	sql, ids, err := h.buildTransactionExportSQL(&TransactionSessionRequestV4{
		SessionID: "abc@host",
		Whitelist: []string{"10.0.0.1", "192.168.1.5"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 1 || ids[0] != "abc@host" {
		t.Fatalf("ids: %v", ids)
	}
	if !strings.Contains(sql, "src_ip != '10.0.0.1'") || !strings.Contains(sql, "dst_ip != '192.168.1.5'") {
		t.Fatalf("sql missing whitelist filters: %s", sql)
	}
}

func TestShareExportPayload_whitelistRoundTrip(t *testing.T) {
	// Share links store the raw POST body; export handlers unmarshal TransactionSessionRequestV4.
	body := []byte(`{"session_id":"share@test","proto_type":1,"event_type":"call","whitelist":["10.0.0.2"]}`)
	var req TransactionSessionRequestV4
	if err := json.Unmarshal(body, &req); err != nil {
		t.Fatal(err)
	}
	h := &SearchHandler{flightService: services.NewFlightService(nil, 0)}
	sql, _, err := h.buildTransactionExportSQL(&req)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sql, "10.0.0.2") {
		t.Fatalf("share payload whitelist not in export SQL: %s", sql)
	}
}

func TestTransactionSessionRequestV4_whitelistJSON(t *testing.T) {
	raw := `{"session_id":"abc@host","whitelist":["10.0.0.1","192.168.1.5"]}`
	var req TransactionSessionRequestV4
	if err := json.Unmarshal([]byte(raw), &req); err != nil {
		t.Fatal(err)
	}
	if len(req.Whitelist) != 2 || req.Whitelist[0] != "10.0.0.1" {
		t.Fatalf("whitelist: %v", req.Whitelist)
	}
	clause := buildExportExcludeIPClause(req.Whitelist)
	if clause == "" {
		t.Fatal("expected SQL clause")
	}
}
