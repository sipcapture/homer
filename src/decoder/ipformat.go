// Copyright (C) 2026 Homer Server Contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package decoder

import "net/netip"

// ipv4BytesToString formats four octets as a dotted-quad without net.IP
// intermediate allocations (To4/String allocate on the parse hot path).
func ipv4BytesToString(b []byte) string {
	if len(b) != 4 {
		return ""
	}
	var buf [15]byte
	n := 0
	for i, v := range b {
		if i > 0 {
			buf[n] = '.'
			n++
		}
		n += writeDecimalUint8(buf[n:], v)
	}
	return string(buf[:n])
}

func writeDecimalUint8(buf []byte, v byte) int {
	if v >= 100 {
		buf[0] = byte('0' + v/100)
		buf[1] = byte('0' + (v/10)%10)
		buf[2] = byte('0' + v%10)
		return 3
	}
	if v >= 10 {
		buf[0] = byte('0' + v/10)
		buf[1] = byte('0' + v%10)
		return 2
	}
	buf[0] = byte('0' + v)
	return 1
}

// ipv6BytesToString formats 16 bytes as RFC 5952 text ("2001:db8::10"), the
// same text net.IP.String gives: an IPv4-mapped address prints as a
// dotted-quad. Search filters compare this text, so it must stay canonical.
func ipv6BytesToString(b []byte) string {
	if len(b) != 16 {
		return ""
	}
	return netip.AddrFrom16([16]byte(b)).Unmap().String()
}
