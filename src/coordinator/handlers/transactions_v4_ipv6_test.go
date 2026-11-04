package handlers

import (
	"strings"
	"testing"

	"github.com/sipcapture/homer-core/src/coordinator/services"
	"github.com/sipcapture/homer-core/src/coordinator/sqlvalidator"
)

const (
	ipv6SrcClause = "src_ip IN ('2001:db8::10', '2001:0db8:0000:0000:0000:0000:0000:0010')"
	ipv4DstClause = "dst_ip = '10.0.0.2'"
)

func ipv6SearchRequest() *SearchObjectV4 {
	req := &SearchObjectV4{}
	req.Filter.ProtoType = 1
	req.Filter.EventType = "call"
	req.Filter.SrcIP = "2001:db8::10"
	req.Filter.DstIP = "10.0.0.2"
	req.Timestamp.From = 1_000_000
	req.Timestamp.To = 2_000_000
	return req
}

func TestBuildSearchSQLV4_IPv6FilterMatchesBothForms(t *testing.T) {
	sql, err := buildSearchSQLV4WithOpts("homer_lake", ipv6SearchRequest(), nil, searchSQLOpts{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sql, ipv6SrcClause) || !strings.Contains(sql, ipv4DstClause) {
		t.Fatalf("expected %q and %q, got:\n%s", ipv6SrcClause, ipv4DstClause, sql)
	}
}

func TestBuildMCPRawSQL_IPv6FilterMatchesBothForms(t *testing.T) {
	sql := buildMCPRawSQL("homer_lake", ipv6SearchRequest())
	if err := sqlvalidator.ValidateRawSQL(sql); err != nil {
		t.Fatalf("generated SQL rejected: %v\n%s", err, sql)
	}
	if !strings.Contains(sql, ipv6SrcClause) || !strings.Contains(sql, ipv4DstClause) {
		t.Fatalf("expected %q and %q, got:\n%s", ipv6SrcClause, ipv4DstClause, sql)
	}
}

func TestBuildSimpleSearchSQL_IPv6FilterMatchesBothForms(t *testing.T) {
	h := &SearchHandler{flightService: services.NewFlightService(nil, 0, false)}
	sql, err := h.buildSimpleSearchSQL(&SimpleSearchRequest{SrcIP: "2001:0db8:0000:0000:0000:0000:0000:0010", DstIP: "10.0.0.2"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sql, ipv6SrcClause) || !strings.Contains(sql, ipv4DstClause) {
		t.Fatalf("expected %q and %q, got:\n%s", ipv6SrcClause, ipv4DstClause, sql)
	}
}

func TestBuildExportExcludeIPClause_IPv6(t *testing.T) {
	got := buildExportExcludeIPClause([]string{"2001:db8::1"})
	want := " AND src_ip NOT IN ('2001:db8::1', '2001:0db8:0000:0000:0000:0000:0000:0001')" +
		" AND dst_ip NOT IN ('2001:db8::1', '2001:0db8:0000:0000:0000:0000:0000:0001')"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestExtractIPAfter(t *testing.T) {
	for _, tc := range []struct{ text, keyword, want string }{
		{"calls from src ip 192.0.2.1 today", `(?:src|source)`, "192.0.2.1"},
		{"source ip: 198.51.100.5", `(?:src|source)`, "198.51.100.5"},
		{"src ip=192.0.2.1", `(?:src|source)`, "192.0.2.1"},
		{"src ip 2001:db8::10", `(?:src|source)`, "2001:db8::10"},
		{"SRC IP 2001:DB8::10", `(?:src|source)`, "2001:DB8::10"},
		{"src ip ::1", `(?:src|source)`, "::1"},
		{"src ip: ::1", `(?:src|source)`, "::1"},
		{"src ip: 2001:db8::10, method INVITE", `(?:src|source)`, "2001:db8::10"},
		{"dst ip=2001:db8::1 and src ip 192.0.2.1", `(?:dst|destination)`, "2001:db8::1"},
		{"destination ip ::ffff:192.0.2.1", `(?:dst|destination)`, "::ffff:192.0.2.1"},
		{"src ip abc", `(?:src|source)`, ""},
		{"no address here", `(?:src|source)`, ""},
	} {
		if got := extractIPAfter(tc.text, tc.keyword); got != tc.want {
			t.Errorf("extractIPAfter(%q) = %q, want %q", tc.text, got, tc.want)
		}
	}
}
