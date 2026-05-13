// Copyright (C) 2025 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

package sipparser

import (
	"errors"
	"strings"
	"unsafe"
)

// ZeroCopyOpts configures optional SIP header extraction for ParseMsgZeroCopy.
// Nil opts or both slices empty: no extra matching (same as legacy callers).
//
// AlegIDs lists SIP header names (case-insensitive). While scanning headers top
// to bottom, the first line whose name matches any entry sets SipMsg.XCallID;
// later lines do not overwrite a non-empty XCallID.
//
// CustomHeaders lists extra header names to copy into SipMsg.CustomHeader; map
// keys are the configured strings (stable for data_extra JSON).
type ZeroCopyOpts struct {
	AlegIDs       []string
	CustomHeaders []string
}

// btos converts []byte to string without copying.
func btos(b []byte) string {
	return unsafe.String(unsafe.SliceData(b), len(b))
}

var (
	errNoEOF    = errors.New("ParseMsgZeroCopy: no SIP headers end found")
	errTooShort = errors.New("ParseMsgZeroCopy: message too short")
)

// ParseMsgZeroCopy parses a SIP message from raw bytes with minimal allocations.
// String fields reference the original buffer via unsafe; keep data alive while
// using the returned SipMsg.
// If opts is non-nil and lists are non-empty, AlegIDs and CustomHeaders populate
// XCallID and CustomHeader respectively.
func ParseMsgZeroCopy(data []byte, opts *ZeroCopyOpts) *SipMsg {
	if len(data) < 16 {
		return &SipMsg{Error: errTooShort}
	}

	headersEnd := findCRLFCRLF(data)
	if headersEnd == -1 {
		headersEnd = zcFindLastCRLF(data)
	}
	if headersEnd == -1 {
		return &SipMsg{Error: errNoEOF}
	}

	s := &SipMsg{
		Msg:       btos(data),
		eof:       headersEnd,
		zcHdrOpts: opts,
	}

	if bodyStart := headersEnd + 4; bodyStart < len(data) {
		s.Body = btos(data[bodyStart:])
	}

	zcParseHeaders(data[:headersEnd+2], s)
	return s
}

// ParseMsgZeroCopyLegacy is equivalent to ParseMsgZeroCopy(data, nil).
func ParseMsgZeroCopyLegacy(data []byte) *SipMsg {
	return ParseMsgZeroCopy(data, nil)
}

// zcApplyAlegCustomHeaders applies ingest aleg_ids and custom_headers for one header line.
func zcApplyAlegCustomHeaders(name, val []byte, s *SipMsg) {
	o := s.zcHdrOpts
	if o == nil {
		return
	}
	if len(o.AlegIDs) == 0 && len(o.CustomHeaders) == 0 {
		return
	}
	nameStr := btos(name)
	valStr := btos(zcTrimWS(val))

	if len(o.AlegIDs) > 0 && s.XCallID == "" {
		for _, id := range o.AlegIDs {
			if id == "" {
				continue
			}
			if strings.EqualFold(nameStr, id) {
				s.XCallID = valStr
				break
			}
		}
	}
	if len(o.CustomHeaders) == 0 {
		return
	}
	for _, h := range o.CustomHeaders {
		if h == "" {
			continue
		}
		if strings.EqualFold(nameStr, h) {
			if s.CustomHeader == nil {
				s.CustomHeader = make(map[string]string)
			}
			s.CustomHeader[h] = valStr
			break
		}
	}
}

func zcFindLastCRLF(data []byte) int {
	n := len(data)
	if n < 2 {
		return -1
	}
	for i := n - 2; i >= 0; i-- {
		if data[i] == '\r' && data[i+1] == '\n' {
			return i
		}
	}
	return -1
}

// zcParseHeaders parses all header lines from raw bytes, zero-copy.
func zcParseHeaders(region []byte, s *SipMsg) {
	n := len(region)
	lineStart := 0
	firstLine := true

	for lineStart < n-1 {
		lineEnd := -1
		for i := lineStart; i < n-1; i++ {
			if region[i] == '\r' && region[i+1] == '\n' {
				lineEnd = i
				break
			}
		}
		if lineEnd == -1 || lineEnd == lineStart {
			break
		}

		line := region[lineStart:lineEnd]
		if firstLine {
			firstLine = false
			zcParseStartLine(line, s)
		} else {
			zcParseHeaderLine(line, s)
		}
		if s.Error != nil {
			return
		}
		lineStart = lineEnd + 2
	}
}

// zcParseStartLine parses the first line (request or response) zero-copy.
func zcParseStartLine(line []byte, s *SipMsg) {
	if len(line) < 3 {
		return
	}

	// Response: SIP/2.0 200 OK
	if line[0] == 'S' && line[1] == 'I' && line[2] == 'P' {
		sp1 := findByte(line, ' ')
		if sp1 == -1 {
			return
		}
		rest := line[sp1+1:]
		sp2 := findByte(rest, ' ')
		if sp2 == -1 {
			return
		}
		s.FirstResp = btos(rest[:sp2])
		s.FirstRespText = btos(rest[sp2+1:])
		return
	}

	// Request: INVITE sip:user@host SIP/2.0
	sp1 := findByte(line, ' ')
	if sp1 == -1 {
		return
	}
	rest := line[sp1+1:]
	sp2 := findByte(rest, ' ')
	if sp2 == -1 {
		return
	}

	s.FirstMethod = btos(line[:sp1])
	zcParseRequestURI(rest[:sp2], s)
}

// zcParseRequestURI extracts User and Host from a SIP URI.
func zcParseRequestURI(uri []byte, s *SipMsg) {
	s.URIRaw = btos(uri)
	s.URIUser, s.URIHost = zcExtractUserHost(uri)
}

// zcParseHeaderLine dispatches a single header line.
func zcParseHeaderLine(line []byte, s *SipMsg) {
	colonPos := findByte(line, ':')
	if colonPos == -1 {
		return
	}

	name := zcTrimWS(line[:colonPos])
	nameLen := len(name)
	if nameLen == 0 {
		return
	}

	valStart := colonPos + 1
	for valStart < len(line) && (line[valStart] == ' ' || line[valStart] == '\t') {
		valStart++
	}
	val := line[valStart:]

	// Single-char compact headers
	if nameLen == 1 {
		switch name[0] {
		case 'i', 'I':
			s.CallID = btos(zcTrimWS(val))
		case 'f', 'F':
			zcParseFromTo(val, s, 'f')
		case 't', 'T':
			zcParseFromTo(val, s, 't')
		case 'm', 'M':
			s.ContactVal = btos(zcTrimWS(val))
			zcParseContact(val, s)
		case 'v', 'V':
			zcParseVia(val, s)
		case 'c', 'C':
			s.ContentType = btos(zcTrimWS(val))
		case 'l', 'L':
			s.ContentLength = btos(zcTrimWS(val))
		}
		return
	}

	if nameLen == 2 {
		zcParseFromTo(val, s, 't')
		return
	}

	first := name[0]
	if first >= 'A' && first <= 'Z' {
		first += 32
	}

	switch first {
	case 'v':
		if nameLen == 3 && zcEqCI3(name, 'v', 'i', 'a') {
			zcParseVia(val, s)
		}
	case 'f':
		if nameLen == 4 && zcEqCI4(name, 'f', 'r', 'o', 'm') {
			zcParseFromTo(val, s, 'f')
		}
	case 'c':
		switch nameLen {
		case 4:
			if zcEqCI4(name, 'c', 's', 'e', 'q') {
				s.CseqVal = btos(zcTrimWS(val))
				zcParseCSeq(val, s)
			}
		case 7:
			if zcEqCI(name, []byte("call-id")) {
				s.CallID = btos(zcTrimWS(val))
			} else if zcEqCI(name, []byte("contact")) {
				s.ContactVal = btos(zcTrimWS(val))
				zcParseContact(val, s)
			}
		case 12:
			if zcEqCI(name, []byte("content-type")) {
				s.ContentType = btos(zcTrimWS(val))
			}
		case 14:
			if zcEqCI(name, []byte("content-length")) {
				s.ContentLength = btos(zcTrimWS(val))
			}
		}
	case 'u':
		if nameLen == 10 && zcEqCI(name, []byte("user-agent")) {
			s.UserAgent = btos(zcTrimWS(val))
		}
	case 's':
		if nameLen == 6 && zcEqCI(name, []byte("server")) {
			s.Server = btos(zcTrimWS(val))
		}
	case 't':
		if nameLen == 2 {
			zcParseFromTo(val, s, 't')
		}
	case 'm':
		if nameLen == 12 && zcEqCI(name, []byte("max-forwards")) {
			s.MaxForwards = btos(zcTrimWS(val))
		}
	case 'o':
		if nameLen == 12 && zcEqCI(name, []byte("organization")) {
			s.Organization = btos(zcTrimWS(val))
		}
	case 'a':
		if nameLen == 13 && zcEqCI(name, []byte("authorization")) {
			s.AuthVal = btos(zcTrimWS(val))
			zcParseAuthUser(val, s)
		}
	case 'p':
		if nameLen == 19 && zcEqCI(name, []byte("p-asserted-identity")) {
			s.PAssertedIdVal = btos(zcTrimWS(val))
			zcParsePAIUser(val, s)
		} else if nameLen == 7 && zcEqCI(name, []byte("privacy")) {
			s.Privacy = btos(zcTrimWS(val))
		} else if nameLen == 19 && zcEqCI(name, []byte("proxy-authorization")) {
			s.AuthVal = btos(zcTrimWS(val))
			zcParseAuthUser(val, s)
		}
	case 'r':
		if nameLen == 6 && zcEqCI(name, []byte("reason")) {
			s.ReasonVal = btos(zcTrimWS(val))
		} else if nameLen == 15 && zcEqCI(name, []byte("remote-party-id")) {
			s.RemotePartyIdVal = btos(zcTrimWS(val))
		}
	case 'd':
		if nameLen == 9 && zcEqCI(name, []byte("diversion")) {
			s.DiversionVal = btos(zcTrimWS(val))
		}
	case 'e':
		if nameLen == 7 && zcEqCI(name, []byte("expires")) {
			s.Expires = btos(zcTrimWS(val))
		}
	case 'x':
		if nameLen == 10 && zcEqCI(name, []byte("x-rtp-stat")) {
			s.RTPStatVal = btos(zcTrimWS(val))
		}
	}
	zcApplyAlegCustomHeaders(name, val, s)
}

// zcParseFromTo extracts user, host, tag from From/To header value.
func zcParseFromTo(val []byte, s *SipMsg, target byte) {
	val = zcTrimWS(val)

	tag := zcExtractParam(val, []byte("tag="))

	lbrack := findByte(val, '<')
	var uriBytes []byte
	if lbrack != -1 {
		rbrack := findByte(val[lbrack:], '>')
		if rbrack != -1 {
			uriBytes = val[lbrack+1 : lbrack+rbrack]
		}
	}
	if uriBytes == nil {
		semi := findByte(val, ';')
		if semi != -1 {
			uriBytes = zcTrimWS(val[:semi])
		} else {
			uriBytes = val
		}
	}

	user, host := zcExtractUserHost(uriBytes)

	switch target {
	case 'f':
		s.FromUser = user
		s.FromHost = host
		s.FromTag = btos(tag)
	case 't':
		s.ToUser = user
		s.ToHost = host
		s.ToTag = btos(tag)
	}
}

// zcParseContact extracts user, host from Contact header.
func zcParseContact(val []byte, s *SipMsg) {
	val = zcTrimWS(val)
	lbrack := findByte(val, '<')
	var uriBytes []byte
	if lbrack != -1 {
		rbrack := findByte(val[lbrack:], '>')
		if rbrack != -1 {
			uriBytes = val[lbrack+1 : lbrack+rbrack]
		}
	}
	if uriBytes == nil {
		semi := findByte(val, ';')
		if semi != -1 {
			uriBytes = zcTrimWS(val[:semi])
		} else {
			uriBytes = val
		}
	}
	s.ContactUser, s.ContactHost = zcExtractUserHost(uriBytes)
}

// zcParseCSeq extracts CSeq method.
func zcParseCSeq(val []byte, s *SipMsg) {
	val = zcTrimWS(val)
	sp := findByte(val, ' ')
	if sp == -1 {
		return
	}
	s.CseqMethod = btos(zcTrimWS(val[sp+1:]))
}

// zcParseVia extracts Via header and branch.
func zcParseVia(val []byte, s *SipMsg) {
	s.ViaOne = btos(zcTrimWS(val))
	branch := zcExtractParam(val, []byte("branch="))
	if len(branch) > 0 {
		s.ViaOneBranch = btos(branch)
	}
}

// zcParseAuthUser extracts username from Authorization header.
func zcParseAuthUser(val []byte, s *SipMsg) {
	idx := zcFindBytesCI(val, []byte("username=\""))
	if idx != -1 {
		start := idx + 10
		end := findByte(val[start:], '"')
		if end != -1 {
			s.AuthUser = btos(val[start : start+end])
		} else {
			s.AuthUser = btos(zcTrimWS(val[start:]))
		}
		return
	}
	idx = zcFindBytesCI(val, []byte("username="))
	if idx != -1 {
		start := idx + 9
		end := findByte(val[start:], ',')
		if end != -1 {
			s.AuthUser = btos(zcTrimWS(val[start : start+end]))
		} else {
			s.AuthUser = btos(zcTrimWS(val[start:]))
		}
	}
}

// zcParsePAIUser extracts user from P-Asserted-Identity.
func zcParsePAIUser(val []byte, s *SipMsg) {
	val = zcTrimWS(val)
	lbrack := findByte(val, '<')
	var uriBytes []byte
	if lbrack != -1 {
		rbrack := findByte(val[lbrack:], '>')
		if rbrack != -1 {
			uriBytes = val[lbrack+1 : lbrack+rbrack]
		}
	}
	if uriBytes == nil {
		uriBytes = val
	}
	s.PaiUser, s.PaiHost = zcExtractUserHost(uriBytes)
}

// zcExtractUserHost extracts user and host from a SIP URI.
func zcExtractUserHost(uri []byte) (user, host string) {
	raw := uri

	// Strip scheme: sip:, sips:, tel:
	if len(raw) > 4 {
		if raw[3] == ':' &&
			(raw[0]|0x20) == 's' && (raw[1]|0x20) == 'i' && (raw[2]|0x20) == 'p' {
			raw = raw[4:]
		} else if raw[3] == ':' &&
			(raw[0]|0x20) == 't' && (raw[1]|0x20) == 'e' && (raw[2]|0x20) == 'l' {
			raw = raw[4:]
		} else if len(raw) > 5 && raw[4] == ':' &&
			(raw[0]|0x20) == 's' && (raw[1]|0x20) == 'i' &&
			(raw[2]|0x20) == 'p' && (raw[3]|0x20) == 's' {
			raw = raw[5:]
		}
	}

	atPos := findByte(raw, '@')
	if atPos != -1 {
		userPart := raw[:atPos]
		semi := findByte(userPart, ';')
		if semi != -1 {
			user = btos(userPart[:semi])
		} else {
			user = btos(userPart)
		}

		hostPart := raw[atPos+1:]
		semi = findByte(hostPart, ';')
		if semi != -1 {
			hostPart = hostPart[:semi]
		}
		colon := findByte(hostPart, ':')
		if colon != -1 {
			host = btos(hostPart[:colon])
		} else {
			host = btos(hostPart)
		}
	} else {
		hostPart := raw
		semi := findByte(hostPart, ';')
		if semi != -1 {
			hostPart = hostPart[:semi]
		}
		colon := findByte(hostPart, ':')
		if colon != -1 {
			host = btos(hostPart[:colon])
		} else {
			host = btos(hostPart)
		}
	}
	return
}

// zcExtractParam finds ";key=value" and returns value bytes (zero-copy).
func zcExtractParam(data []byte, key []byte) []byte {
	kLen := len(key)
	n := len(data)
	if n < kLen+2 {
		return nil
	}
	for i := 0; i < n-kLen; i++ {
		if data[i] != ';' {
			continue
		}
		if i+1+kLen > n {
			break
		}
		match := true
		for j := 0; j < kLen; j++ {
			if data[i+1+j] != key[j] {
				match = false
				break
			}
		}
		if !match {
			continue
		}
		valStart := i + 1 + kLen
		valEnd := n
		for k := valStart; k < n; k++ {
			c := data[k]
			if c == ';' || c == ' ' || c == ',' || c == '\r' || c == '\n' {
				valEnd = k
				break
			}
		}
		return data[valStart:valEnd]
	}
	return nil
}

// --- Low-level helpers ---

func zcTrimWS(b []byte) []byte {
	for len(b) > 0 && (b[0] == ' ' || b[0] == '\t') {
		b = b[1:]
	}
	for len(b) > 0 && (b[len(b)-1] == ' ' || b[len(b)-1] == '\t') {
		b = b[:len(b)-1]
	}
	return b
}

// zcEqCI compares byte slices case-insensitively.
func zcEqCI(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		ca, cb := a[i], b[i]
		if ca >= 'A' && ca <= 'Z' {
			ca += 32
		}
		if cb >= 'A' && cb <= 'Z' {
			cb += 32
		}
		if ca != cb {
			return false
		}
	}
	return true
}

// Specialized 3-byte and 4-byte case-insensitive comparisons (inlineable).
func zcEqCI3(a []byte, b0, b1, b2 byte) bool {
	return len(a) == 3 &&
		(a[0]|0x20) == b0 &&
		(a[1]|0x20) == b1 &&
		(a[2]|0x20) == b2
}

func zcEqCI4(a []byte, b0, b1, b2, b3 byte) bool {
	return len(a) == 4 &&
		(a[0]|0x20) == b0 &&
		(a[1]|0x20) == b1 &&
		(a[2]|0x20) == b2 &&
		(a[3]|0x20) == b3
}

// zcFindBytesCI finds needle in data case-insensitively.
func zcFindBytesCI(data []byte, needle []byte) int {
	nLen := len(needle)
	if nLen == 0 || len(data) < nLen {
		return -1
	}
	end := len(data) - nLen + 1
	for i := 0; i < end; i++ {
		match := true
		for j := 0; j < nLen; j++ {
			a := data[i+j]
			b := needle[j]
			if a >= 'A' && a <= 'Z' {
				a += 32
			}
			if b >= 'A' && b <= 'Z' {
				b += 32
			}
			if a != b {
				match = false
				break
			}
		}
		if match {
			return i
		}
	}
	return -1
}
