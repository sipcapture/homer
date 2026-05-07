// Copyright (C) 2025 Homer Server Contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package services

import (
	"fmt"
	"net"
	"net/netip"
	"strconv"
	"strings"

	"github.com/gaissmai/cidrtree"
	logger "github.com/sipcapture/homer-core/src/utils/logging"
)

// aliasEntry mirrors homer-app's Alias — one row under a CIDR prefix.
type aliasEntry struct {
	Name         string
	Port         int
	CaptureID    string
	CustomImage  string
	Tag1         string
	Tag2         string
	Tag3         string
	Tag4         string
}

// IPAliasMap resolves IP (+ port + optional capture_id) to a friendly alias
// using longest-prefix match (same strategy as homer-app/data/service/alias.go).
type IPAliasMap struct {
	table *cidrtree.Table[*[]aliasEntry]
}

// NewIPAliasMap builds a lookup table from active alias rows (status should be true).
//
// Rows that fail to convert into a CIDR prefix or carry an empty alias
// name are skipped — but the failure is logged at Warn so silent
// "active_rows=N, loaded_prefixes=0" cases (the canonical "I created
// an alias but enrichment doesn't fire" symptom) are diagnosable from
// the server log alone.
func NewIPAliasMap(rows []AliasItem) *IPAliasMap {
	var t cidrtree.Table[*[]aliasEntry]
	byPrefix := make(map[netip.Prefix]*[]aliasEntry)

	for _, row := range rows {
		if !row.Status {
			// Defence in depth: ListActive already filters
			// `WHERE status = 1` server-side, so reaching this
			// branch means the row.Status mapping is broken
			// upstream (toBoolValue / mapRowToAlias). The
			// "active_rows=N loaded_prefixes=0" symptom that
			// dogged 11.0.122 was exactly this — a missing
			// int32 case in toBoolValue made every active alias
			// look inactive. Log so the next regression is loud.
			logger.Warn("IPAliasMap: skipping alias row marked Status=false despite ListActive filter",
				"alias", row.Alias, "ip", row.IP, "guid", row.GUID)
			continue
		}
		pfx, err := prefixFromAliasRow(row.IP, row.Mask)
		if err != nil {
			logger.Warn("IPAliasMap: skipping alias row, prefix parse failed",
				"alias", row.Alias, "ip", row.IP, "mask", row.Mask, "error", err.Error())
			continue
		}
		e := aliasEntry{
			Name:        strings.TrimSpace(row.Alias),
			Port:        row.Port,
			CaptureID:   strings.TrimSpace(row.CaptureID),
			CustomImage: strings.TrimSpace(row.CustomImage),
			Tag1:        strings.TrimSpace(row.Tag1),
			Tag2:        strings.TrimSpace(row.Tag2),
			Tag3:        strings.TrimSpace(row.Tag3),
			Tag4:        strings.TrimSpace(row.Tag4),
		}
		if e.Name == "" {
			logger.Warn("IPAliasMap: skipping alias row with empty alias name",
				"ip", row.IP, "mask", row.Mask, "guid", row.GUID)
			continue
		}
		if as, ok := byPrefix[pfx]; ok {
			*as = append(*as, e)
		} else {
			as := &[]aliasEntry{e}
			byPrefix[pfx] = as
			t.Insert(pfx, as)
		}
	}
	return &IPAliasMap{table: &t}
}

// prefixFromAliasRow turns (ip, mask) from an alias row into a CIDR
// prefix. It is permissive: ip may carry an embedded "/<mask>" (some
// homer-app installs store "10.0.0.0/24" in the ip column with mask
// left at 0) and the mask is clamped to the address family's max bit
// width.
func prefixFromAliasRow(ip string, mask int) (netip.Prefix, error) {
	ip = strings.TrimSpace(ip)
	if ip == "" {
		return netip.Prefix{}, fmt.Errorf("empty ip")
	}

	// Strip an embedded CIDR suffix if present and use it when no
	// explicit mask was set on the row.
	if idx := strings.Index(ip, "/"); idx >= 0 {
		if mask <= 0 {
			if n, err := strconv.Atoi(ip[idx+1:]); err == nil {
				mask = n
			}
		}
		ip = ip[:idx]
	}

	if mask <= 0 {
		addr, err := netip.ParseAddr(ip)
		if err != nil {
			return netip.Prefix{}, err
		}
		bits := 32
		if addr.Is6() {
			bits = 128
		}
		return addr.Unmap().Prefix(bits)
	}

	addr, err := netip.ParseAddr(ip)
	if err != nil {
		return netip.Prefix{}, err
	}
	addr = addr.Unmap()
	maxBits := 32
	if addr.Is6() {
		maxBits = 128
	}
	if mask > maxBits {
		return netip.Prefix{}, fmt.Errorf("mask %d exceeds %d-bit address family", mask, maxBits)
	}
	return addr.Prefix(mask)
}

// Size returns the number of CIDR prefixes loaded into the LPM table.
// Each prefix may carry multiple alias entries (port- or capture-scoped);
// this only counts the unique CIDRs. Used for diagnostics ("are my
// aliases loaded?") and tests.
func (m *IPAliasMap) Size() int {
	if m == nil || m.table == nil {
		return 0
	}
	n := 0
	m.table.Walk(func(_ netip.Prefix, _ *[]aliasEntry) bool {
		n++
		return true
	})
	return n
}

// FindEntry returns the full alias row when IP matches a prefix and port/capture rules match.
// Port 0 on an alias row matches only when querying with port 0 (wildcard lookup).
func (m *IPAliasMap) FindEntry(ip net.IP, port int, captureID *string) (aliasEntry, bool) {
	if m == nil || m.table == nil {
		return aliasEntry{}, false
	}
	addr, ok := netip.AddrFromSlice(ip)
	if !ok {
		return aliasEntry{}, false
	}
	_, entries, ok := m.table.Lookup(addr.Unmap())
	if !ok || entries == nil || len(*entries) == 0 {
		return aliasEntry{}, false
	}
	for _, a := range *entries {
		if a.Port != port {
			continue
		}
		if captureID != nil && a.CaptureID != *captureID {
			continue
		}
		return a, true
	}
	return aliasEntry{}, false
}

// Find returns the alias name when IP matches a prefix and port/capture rules match.
// Port 0 on an alias row matches only when querying with port 0 (wildcard lookup).
func (m *IPAliasMap) Find(ip net.IP, port int, captureID *string) (string, bool) {
	e, ok := m.FindEntry(ip, port, captureID)
	return e.Name, ok
}

func parseIPString(s string) net.IP {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	if strings.HasPrefix(s, "[") {
		if host, _, err := net.SplitHostPort(s); err == nil {
			return net.ParseIP(host)
		}
		if h, err := netip.ParseAddr(strings.Trim(s, "[]")); err == nil {
			return net.IP(h.AsSlice())
		}
	}
	if ip := net.ParseIP(s); ip != nil {
		return ip
	}
	host, _, err := net.SplitHostPort(s)
	if err == nil {
		return net.ParseIP(host)
	}
	return nil
}

func rowIntCI(row map[string]interface{}, want string) int {
	var v interface{}
	var ok bool
	lw := strings.ToLower(want)
	for k, val := range row {
		if strings.ToLower(k) == lw {
			v, ok = val, true
			break
		}
	}
	if !ok {
		return 0
	}
	switch x := v.(type) {
	case int:
		return x
	case int32:
		return int(x)
	case int64:
		return int(x)
	case float64:
		return int(x)
	case float32:
		return int(x)
	case string:
		n, err := strconv.Atoi(strings.TrimSpace(x))
		if err != nil {
			return 0
		}
		return n
	default:
		return 0
	}
}

func rowStringCI(row map[string]interface{}, want string) string {
	lw := strings.ToLower(want)
	for k, val := range row {
		if strings.ToLower(k) != lw {
			continue
		}
		if val == nil {
			return ""
		}
		switch s := val.(type) {
		case string:
			return strings.TrimSpace(s)
		default:
			return strings.TrimSpace(fmt.Sprint(s))
		}
	}
	return ""
}

// ResolveAliasEntry returns the matched alias row (name + optional display fields).
func ResolveAliasEntry(m *IPAliasMap, ipStr string, port int, captureStr string) (aliasEntry, bool) {
	ipStr = strings.TrimSpace(ipStr)
	ip := parseIPString(ipStr)
	if m == nil || ip == nil {
		return aliasEntry{}, false
	}
	var capPtr *string
	if captureStr != "" {
		capPtr = &captureStr
	}
	tries := []func() (aliasEntry, bool){
		func() (aliasEntry, bool) {
			if capPtr == nil {
				return aliasEntry{}, false
			}
			return m.FindEntry(ip, port, capPtr)
		},
		func() (aliasEntry, bool) {
			if capPtr == nil {
				return aliasEntry{}, false
			}
			return m.FindEntry(ip, 0, capPtr)
		},
		func() (aliasEntry, bool) { return m.FindEntry(ip, port, nil) },
		func() (aliasEntry, bool) { return m.FindEntry(ip, 0, nil) },
	}
	for _, t := range tries {
		if e, ok := t(); ok && e.Name != "" {
			return e, true
		}
	}
	return aliasEntry{}, false
}

// ResolveAliasName returns a friendly alias when the IP/port/capture_id matches
// an active alias row; otherwise ok is false (caller keeps raw columns).
func ResolveAliasName(m *IPAliasMap, ipStr string, port int, captureStr string) (string, bool) {
	e, ok := ResolveAliasEntry(m, ipStr, port, captureStr)
	return e.Name, ok
}

func enrichAliasSide(row map[string]interface{}, keyPrefix string, e aliasEntry) {
	row[keyPrefix] = e.Name
	if e.CustomImage != "" {
		row[keyPrefix+"_image"] = e.CustomImage
	}
	if e.Tag1 != "" {
		row[keyPrefix+"_tag1"] = e.Tag1
	}
	if e.Tag2 != "" {
		row[keyPrefix+"_tag2"] = e.Tag2
	}
	if e.Tag3 != "" {
		row[keyPrefix+"_tag3"] = e.Tag3
	}
	if e.Tag4 != "" {
		row[keyPrefix+"_tag4"] = e.Tag4
	}
}

// EnrichRowIPAliases sets aliasSrc and aliasDst only when a matching alias exists
// (same contract as homer-app: raw src_ip/dst_ip stay unchanged).
// When present on the row, custom_image and tag1–tag4 are copied as aliasSrc_* / aliasDst_*.
func EnrichRowIPAliases(m *IPAliasMap, row map[string]interface{}) {
	if row == nil || m == nil {
		return
	}
	src := rowStringCI(row, "src_ip")
	dst := rowStringCI(row, "dst_ip")
	capture := rowStringCI(row, "capture_id")

	srcPort := rowIntCI(row, "src_port")
	dstPort := rowIntCI(row, "dst_port")

	if e, ok := ResolveAliasEntry(m, src, srcPort, capture); ok {
		enrichAliasSide(row, "aliasSrc", e)
	}
	if e, ok := ResolveAliasEntry(m, dst, dstPort, capture); ok {
		enrichAliasSide(row, "aliasDst", e)
	}
}
