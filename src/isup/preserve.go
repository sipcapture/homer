// Copyright (C) 2026 Homer Server Contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

// Package isup preserves binary SIP bodies (SIP-I application/isup) across
// UTF-8 text storage and decodes ITU-T Q.763 messages.
package isup

import (
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"mime"
	"strconv"
	"strings"
	"unicode/utf8"
)

// binaryBodyHeader marks a SIP entity whose body was rewritten so it can live
// in a UTF-8 VARCHAR / Parquet string. The original octets come back via
// RestoreBinarySIPPayload (PCAP) and DecodeSIP.
const binaryBodyHeader = "X-Homer-Binary-Body"

// multiBodyMagic prefixes isup_body when one SIP message carries more than
// one application/isup part. A single part is stored as the raw body.
const multiBodyMagic = "ISI1"

// DetachISUPBody removes application/isup bodies that cannot live in a UTF-8
// VARCHAR column. The returned payload keeps the SIP headers and any text
// parts; body is the original octets for isup_body, or nil when there is
// nothing to store. Messages already rewritten with X-Homer-Binary-Body
// (11.0.376) are left unchanged.
func DetachISUPBody(payload string) (stored string, body []byte) {
	if !unsafeUTF8(payload) {
		return payload, nil
	}
	headers, nl, rawBody, ok := splitSIP(payload)
	if !ok {
		return payload, nil
	}
	if _, has := headerValue(headers, nl, binaryBodyHeader); has {
		return payload, nil
	}
	ct, _ := headerValue(headers, nl, "Content-Type")
	if boundary, multipart := multipartBoundary(ct); multipart {
		newBody, parts := detachMultipartISUP(rawBody, boundary, nl)
		if len(parts) == 0 {
			return payload, nil
		}
		headers, _ = replaceHeaderValue(headers, nl, "Content-Length", strconv.Itoa(len(newBody)))
		return headers + nl + nl + newBody, packISUPBodies(parts)
	}
	if _, isup := isISUPType(ct); !isup || !unsafeUTF8(rawBody) {
		return payload, nil
	}
	return setSIPBody(headers, nl, ""), []byte(rawBody)
}

// AttachISUPBody puts a stored isup_body back into the SIP message and
// updates Content-Length. A nil body returns payload unchanged.
func AttachISUPBody(payload string, body []byte) string {
	if len(body) == 0 {
		return payload
	}
	headers, nl, rawBody, ok := splitSIP(payload)
	if !ok {
		return payload
	}
	ct, _ := headerValue(headers, nl, "Content-Type")
	if boundary, multipart := multipartBoundary(ct); multipart {
		newBody := attachMultipartISUP(rawBody, boundary, nl, body)
		headers, _ = replaceHeaderValue(headers, nl, "Content-Length", strconv.Itoa(len(newBody)))
		return headers + nl + nl + newBody
	}
	if _, isup := isISUPType(ct); !isup {
		return payload
	}
	parts := unpackISUPBodies(body, 1)
	return setSIPBody(headers, nl, string(parts[0]))
}

// MessageBytes restores a SIP message for PCAP and Decode. 11.0.376 rows
// carry the body as hex inside the payload. Later rows keep it in isup_body.
func MessageBytes(payload string, isupBody interface{}) string {
	payload = RestoreBinarySIPPayload(payload)
	body := BodyFromCell(isupBody)
	if len(body) == 0 {
		return payload
	}
	return AttachISUPBody(payload, body)
}

// BodyFromCell reads an isup_body value from a search row. DuckDB returns
// []byte. A JSON API returns the same bytes as standard base64.
func BodyFromCell(v interface{}) []byte {
	switch t := v.(type) {
	case nil:
		return nil
	case []byte:
		if len(t) == 0 {
			return nil
		}
		return t
	case string:
		if t == "" {
			return nil
		}
		raw, err := base64.StdEncoding.DecodeString(t)
		if err != nil || len(raw) == 0 {
			return nil
		}
		return raw
	default:
		return nil
	}
}

func packISUPBodies(parts [][]byte) []byte {
	if len(parts) == 0 {
		return nil
	}
	if len(parts) == 1 {
		return parts[0]
	}
	out := make([]byte, 6, 6+len(parts)*8)
	copy(out, multiBodyMagic)
	binary.BigEndian.PutUint16(out[4:6], uint16(len(parts)))
	var lenb [4]byte
	for _, part := range parts {
		binary.BigEndian.PutUint32(lenb[:], uint32(len(part)))
		out = append(out, lenb[:]...)
		out = append(out, part...)
	}
	return out
}

func unpackISUPBodies(body []byte, nparts int) [][]byte {
	if nparts <= 1 {
		return [][]byte{body}
	}
	if decoded, ok := unpackISUPEnvelope(body); ok && len(decoded) == nparts {
		return decoded
	}
	out := make([][]byte, nparts)
	out[0] = body
	return out
}

func unpackISUPEnvelope(body []byte) ([][]byte, bool) {
	if len(body) < 6 || string(body[:4]) != multiBodyMagic {
		return nil, false
	}
	n := int(binary.BigEndian.Uint16(body[4:6]))
	if n <= 1 {
		return nil, false
	}
	rest := body[6:]
	out := make([][]byte, 0, n)
	for i := 0; i < n; i++ {
		if len(rest) < 4 {
			return nil, false
		}
		l := int(binary.BigEndian.Uint32(rest[:4]))
		rest = rest[4:]
		if l < 0 || len(rest) < l {
			return nil, false
		}
		out = append(out, rest[:l])
		rest = rest[l:]
	}
	if len(rest) != 0 {
		return nil, false
	}
	return out, true
}

func setSIPBody(headers, nl, body string) string {
	if headers != "" {
		headers, _ = replaceHeaderValue(headers, nl, "Content-Length", strconv.Itoa(len(body)))
	}
	return headers + nl + nl + body
}

func detachMultipartISUP(body, boundary, nl string) (string, [][]byte) {
	var parts [][]byte
	newBody, changed := mapMultipart(body, boundary, nl, func(part, pnl string) (string, bool) {
		headers, hnl, pbody, ok := splitSIP(part)
		if !ok {
			return part, false
		}
		ct, _ := headerValue(headers, hnl, "Content-Type")
		if _, isup := isISUPType(ct); !isup || !unsafeUTF8(pbody) {
			return part, false
		}
		parts = append(parts, []byte(pbody))
		return setSIPBody(headers, hnl, ""), true
	})
	if !changed {
		return body, nil
	}
	return newBody, parts
}

func attachMultipartISUP(body, boundary, nl string, blob []byte) string {
	nparts := 0
	mapMultipart(body, boundary, nl, func(part, pnl string) (string, bool) {
		headers, hnl, _, ok := splitSIP(part)
		if !ok {
			return part, false
		}
		ct, _ := headerValue(headers, hnl, "Content-Type")
		if _, isup := isISUPType(ct); isup {
			nparts++
		}
		return part, false
	})
	bodies := unpackISUPBodies(blob, nparts)
	idx := 0
	newBody, _ := mapMultipart(body, boundary, nl, func(part, pnl string) (string, bool) {
		headers, hnl, _, ok := splitSIP(part)
		if !ok {
			return part, false
		}
		ct, _ := headerValue(headers, hnl, "Content-Type")
		if _, isup := isISUPType(ct); !isup {
			return part, false
		}
		var raw []byte
		if idx < len(bodies) {
			raw = bodies[idx]
		}
		idx++
		return setSIPBody(headers, hnl, string(raw)), true
	})
	return newBody
}

// PreserveBinarySIPPayload rewrites a SIP message so a binary body survives
// toUTF8 and a UTF-8 text column. NUL and non-UTF-8 octets are the bytes
// toUTF8 drops; SIP-I ISUP is full of them.
//
// 11.0.376 stored that body as hex inside the SIP message. New messages use
// DetachISUPBody and the isup_body column instead. This function remains so
// already stored rows can be restored.
func PreserveBinarySIPPayload(payload string) string {
	if !unsafeUTF8(payload) {
		return payload
	}
	headers, nl, _, ok := splitSIP(payload)
	if !ok {
		return payload
	}
	if _, has := headerValue(headers, nl, binaryBodyHeader); has {
		return payload
	}
	ct, _ := headerValue(headers, nl, "Content-Type")
	if boundary, multipart := multipartBoundary(ct); multipart {
		newBody, changed := mapMultipart(payloadBody(payload, nl), boundary, nl, preservePart)
		if !changed {
			return payload
		}
		headers, _ = replaceHeaderValue(headers, nl, "Content-Length", strconv.Itoa(len(newBody)))
		return headers + nl + nl + newBody
	}
	rewritten, changed := preservePart(payload, nl)
	if !changed {
		return payload
	}
	return rewritten
}

// RestoreBinarySIPPayload undoes PreserveBinarySIPPayload. Messages without
// the marker are returned unchanged, so a PCAP of an ordinary SIP call stays
// byte-for-byte what was stored.
func RestoreBinarySIPPayload(payload string) string {
	headers, nl, _, ok := splitSIP(payload)
	if !ok {
		return payload
	}
	if _, has := headerValue(headers, nl, binaryBodyHeader); has {
		restored, changed := restorePart(payload, nl)
		if !changed {
			return payload
		}
		return restored
	}
	ct, _ := headerValue(headers, nl, "Content-Type")
	boundary, multipart := multipartBoundary(ct)
	if !multipart {
		return payload
	}
	newBody, changed := mapMultipart(payloadBody(payload, nl), boundary, nl, restorePart)
	if !changed {
		return payload
	}
	headers, _ = replaceHeaderValue(headers, nl, "Content-Length", strconv.Itoa(len(newBody)))
	return headers + nl + nl + newBody
}

func preservePart(part, nl string) (string, bool) {
	headers, pnl, body, ok := splitSIP(part)
	if !ok || !unsafeUTF8(body) {
		return part, false
	}
	if _, has := headerValue(headers, pnl, binaryBodyHeader); has {
		return part, false
	}
	encoded := hex.EncodeToString([]byte(body))
	if headers != "" {
		headers += pnl
	}
	headers += binaryBodyHeader + ": hex"
	headers, _ = replaceHeaderValue(headers, pnl, "Content-Length", strconv.Itoa(len(encoded)))
	return headers + pnl + pnl + encoded, true
}

func restorePart(part, nl string) (string, bool) {
	headers, pnl, body, ok := splitSIP(part)
	if !ok {
		return part, false
	}
	enc, has := headerValue(headers, pnl, binaryBodyHeader)
	if !has {
		return part, false
	}
	raw, err := decodeEncodedBody(enc, body)
	if err != nil {
		return part, false
	}
	headers = removeHeader(headers, pnl, binaryBodyHeader)
	headers, _ = replaceHeaderValue(headers, pnl, "Content-Length", strconv.Itoa(len(raw)))
	return headers + pnl + pnl + string(raw), true
}

func decodeEncodedBody(encoding, body string) ([]byte, error) {
	s := strings.TrimSpace(body)
	switch strings.ToLower(strings.TrimSpace(encoding)) {
	case "hex":
		return hex.DecodeString(s)
	case "base64":
		return base64.StdEncoding.DecodeString(s)
	default:
		return nil, hex.ErrLength
	}
}

func unsafeUTF8(s string) bool {
	return strings.IndexByte(s, 0) >= 0 || !utf8.ValidString(s)
}

func splitSIP(payload string) (headers, nl, body string, ok bool) {
	if i := strings.Index(payload, "\r\n\r\n"); i >= 0 {
		return payload[:i], "\r\n", payload[i+4:], true
	}
	if i := strings.Index(payload, "\n\n"); i >= 0 {
		return payload[:i], "\n", payload[i+2:], true
	}
	return "", "", "", false
}

func payloadBody(payload, nl string) string {
	_, _, body, ok := splitSIP(payload)
	if !ok {
		return ""
	}
	return body
}

func multipartBoundary(contentType string) (string, bool) {
	media, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		return "", false
	}
	if !strings.HasPrefix(strings.ToLower(media), "multipart/") {
		return "", false
	}
	boundary := params["boundary"]
	if boundary == "" {
		return "", false
	}
	return boundary, true
}

// mapMultipart applies fn to each MIME part. The CRLF that introduces the
// next boundary stays part of the delimiter, not the part body.
func mapMultipart(body, boundary, nl string, fn func(part, nl string) (string, bool)) (string, bool) {
	marker := "--" + boundary
	if !strings.Contains(body, marker) {
		return body, false
	}
	parts := strings.Split(body, marker)
	changed := false
	var b strings.Builder
	b.Grow(len(body) + 32)
	b.WriteString(parts[0])
	for i := 1; i < len(parts); i++ {
		seg := parts[i]
		b.WriteString(marker)
		if strings.HasPrefix(seg, "--") {
			b.WriteString(seg)
			continue
		}
		lead, rest := "", seg
		if strings.HasPrefix(rest, nl) {
			lead = nl
			rest = rest[len(nl):]
		}
		trail := ""
		if strings.HasSuffix(rest, nl) {
			trail = nl
			rest = rest[:len(rest)-len(nl)]
		}
		next, ok := fn(rest, nl)
		if ok && next != rest {
			changed = true
			rest = next
		}
		b.WriteString(lead)
		b.WriteString(rest)
		b.WriteString(trail)
	}
	if !changed {
		return body, false
	}
	return b.String(), true
}

func headerValue(headers, nl, name string) (string, bool) {
	for _, line := range strings.Split(headers, nl) {
		hname, value, ok := splitHeaderLine(line)
		if ok && headerNamesMatch(hname, name) {
			return value, true
		}
	}
	return "", false
}

func replaceHeaderValue(headers, nl, name, value string) (string, bool) {
	lines := strings.Split(headers, nl)
	found := false
	for i, line := range lines {
		hname, _, ok := splitHeaderLine(line)
		if !ok || !headerNamesMatch(hname, name) {
			continue
		}
		lines[i] = hname + ": " + value
		found = true
	}
	if !found {
		return headers, false
	}
	return strings.Join(lines, nl), true
}

func removeHeader(headers, nl, name string) string {
	lines := strings.Split(headers, nl)
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		hname, _, ok := splitHeaderLine(line)
		if ok && headerNamesMatch(hname, name) {
			continue
		}
		out = append(out, line)
	}
	return strings.Join(out, nl)
}

func splitHeaderLine(line string) (name, value string, ok bool) {
	if line == "" || line[0] == ' ' || line[0] == '\t' {
		return "", "", false
	}
	i := strings.IndexByte(line, ':')
	if i <= 0 {
		return "", "", false
	}
	return strings.TrimSpace(line[:i]), strings.TrimSpace(line[i+1:]), true
}

func headerNamesMatch(got, want string) bool {
	if strings.EqualFold(got, want) {
		return true
	}
	switch strings.ToLower(want) {
	case "content-type":
		return strings.EqualFold(got, "c")
	case "content-length":
		return strings.EqualFold(got, "l")
	default:
		return false
	}
}
