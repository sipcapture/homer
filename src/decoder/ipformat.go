// Copyright (C) 2026 Homer Server Contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package decoder


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

// ipv6BytesToString formats 16 bytes as IPv6 text (one heap string).
// IPv6 is uncommon on the SIP capture hot path; keep a small helper.
func ipv6BytesToString(b []byte) string {
	if len(b) != 16 {
		return ""
	}
	var buf [39]byte
	off := 0
	for i := 0; i < 16; i += 2 {
		if i > 0 {
			buf[off] = ':'
			off++
		}
		off += writeHexUint16(buf[off:], uint16(b[i])<<8|uint16(b[i+1]))
	}
	return string(buf[:off])
}

func writeHexUint16(buf []byte, v uint16) int {
	const hexdigits = "0123456789abcdef"
	buf[0] = hexdigits[v>>12&0xf]
	buf[1] = hexdigits[v>>8&0xf]
	buf[2] = hexdigits[v>>4&0xf]
	buf[3] = hexdigits[v&0xf]
	return 4
}
