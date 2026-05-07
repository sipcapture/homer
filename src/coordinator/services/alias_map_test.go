package services

import (
	"net"
	"testing"
)

func TestIPAliasMap_Find_hostRoute(t *testing.T) {
	rows := []AliasItem{
		{Alias: "sip-gw", IP: "10.10.10.10", Mask: 32, Port: 5060, CaptureID: "", Status: true},
	}
	m := NewIPAliasMap(rows)
	ip := net.ParseIP("10.10.10.10")
	name, ok := m.Find(ip, 5060, nil)
	if !ok || name != "sip-gw" {
		t.Fatalf("want sip-gw, ok=%v got %q", ok, name)
	}
	if _, ok := m.Find(ip, 5070, nil); ok {
		t.Fatal("different port should not match")
	}
}

func TestIPAliasMap_Find_wildcardPort(t *testing.T) {
	rows := []AliasItem{
		{Alias: "any-port", IP: "192.168.1.1", Mask: 32, Port: 0, CaptureID: "", Status: true},
	}
	m := NewIPAliasMap(rows)
	ip := net.ParseIP("192.168.1.1")
	name, ok := m.Find(ip, 0, nil)
	if !ok || name != "any-port" {
		t.Fatalf("wildcard port row matches query port 0 only: ok=%v name=%q", ok, name)
	}
}

func TestResolveAliasName_captureID(t *testing.T) {
	rows := []AliasItem{
		{Alias: "cap-a", IP: "10.0.0.5", Mask: 32, Port: 5060, CaptureID: "7", Status: true},
	}
	m := NewIPAliasMap(rows)
	n, ok := ResolveAliasName(m, "10.0.0.5", 5060, "7")
	if !ok || n != "cap-a" {
		t.Fatalf("want cap-a, ok=%v got %q", ok, n)
	}
}

func TestResolveAliasName_wildcardPortRow(t *testing.T) {
	rows := []AliasItem{
		{Alias: "wildcard", IP: "10.1.1.1", Mask: 32, Port: 0, CaptureID: "", Status: true},
	}
	m := NewIPAliasMap(rows)
	n, ok := ResolveAliasName(m, "10.1.1.1", 5060, "")
	if !ok || n != "wildcard" {
		t.Fatalf("port-0 alias row matches via Find(port 0): ok=%v got %q", ok, n)
	}
}

// Regression: homer-app-style rows that store the CIDR inside the ip
// column ("10.1.68.0/24" + mask=0) used to be silently dropped because
// netip.ParseAddr rejects the embedded slash. The parser now strips
// the suffix and uses it as the mask when the row mask is unset.
func TestNewIPAliasMap_acceptsEmbeddedCIDR(t *testing.T) {
	rows := []AliasItem{
		{Alias: "lan", IP: "10.1.68.0/24", Mask: 0, Port: 5060, Status: true},
	}
	m := NewIPAliasMap(rows)
	if got := m.Size(); got != 1 {
		t.Fatalf("loaded_prefixes = %d, want 1 (embedded CIDR must be parsed)", got)
	}
	if n, ok := m.Find(net.ParseIP("10.1.68.219"), 5060, nil); !ok || n != "lan" {
		t.Fatalf("LPM lookup over /24 failed: ok=%v name=%q", ok, n)
	}
}

// Regression: a row with mask larger than the address family's bit
// width (e.g. mask=128 on an IPv4 address) used to crash netip.Prefix
// silently; now the parser rejects it with a clear error so the
// NewIPAliasMap log line points at the offending row.
func TestNewIPAliasMap_rejectsImpossibleMask(t *testing.T) {
	rows := []AliasItem{
		{Alias: "broken", IP: "10.0.0.1", Mask: 128, Status: true},
		{Alias: "good", IP: "10.0.0.2", Mask: 32, Status: true},
	}
	m := NewIPAliasMap(rows)
	if got := m.Size(); got != 1 {
		t.Fatalf("loaded_prefixes = %d, want 1 (only the valid row should load)", got)
	}
}

// Regression: an empty alias name should not silently drop a prefix —
// the row is still skipped, but the test asserts we don't load it
// either way.
func TestNewIPAliasMap_dropsEmptyName(t *testing.T) {
	rows := []AliasItem{
		{Alias: "  ", IP: "10.0.0.1", Mask: 32, Status: true},
	}
	if got := NewIPAliasMap(rows).Size(); got != 0 {
		t.Fatalf("empty-name row must not be loaded, got Size=%d", got)
	}
}

func TestEnrichRowIPAliases_setsImageAndTags(t *testing.T) {
	rows := []AliasItem{
		{
			Alias: "gw", IP: "10.0.0.1", Mask: 32, Port: 5060, Status: true,
			CustomImage: "https://example.com/a.png",
			Tag1:        "SBC", Tag2: "Linux", Tag3: "dc1", Tag4: "",
		},
	}
	m := NewIPAliasMap(rows)
	row := map[string]interface{}{
		"src_ip": "10.0.0.1", "src_port": 5060,
		"dst_ip": "10.0.0.2", "dst_port": 5060,
	}
	EnrichRowIPAliases(m, row)
	if row["aliasSrc"] != "gw" {
		t.Fatalf("aliasSrc: got %v", row["aliasSrc"])
	}
	if row["aliasSrc_image"] != "https://example.com/a.png" {
		t.Fatalf("aliasSrc_image: got %v", row["aliasSrc_image"])
	}
	if row["aliasSrc_tag1"] != "SBC" || row["aliasSrc_tag2"] != "Linux" || row["aliasSrc_tag3"] != "dc1" {
		t.Fatalf("tags: %#v", row)
	}
	if _, has := row["aliasSrc_tag4"]; has {
		t.Fatal("empty tag4 must not be set")
	}
	if _, has := row["aliasDst"]; has {
		t.Fatal("dst should not match")
	}
}
