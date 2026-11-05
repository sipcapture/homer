// Copyright (C) 2026 Homer Server Contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

// Package isup preserves binary SIP bodies (SIP-I application/isup) across
// UTF-8 text storage and decodes ITU-T Q.763 messages.
package isup

import (
	"encoding/base64"
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

// PreserveBinarySIPPayload rewrites a SIP message so a binary body survives
// toUTF8 and a UTF-8 text column. NUL and non-UTF-8 octets are the bytes
// toUTF8 drops; SIP-I ISUP is full of them.
//
// Text messages are returned unchanged. A binary body is stored as hex and
// marked with X-Homer-Binary-Body. Multipart messages encode only the binary
// parts, so SDP stays readable. Hex cannot contain a MIME boundary ("--"),
// so the multipart framing stays unambiguous.
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
