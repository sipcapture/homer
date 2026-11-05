// Copyright (C) 2026 Homer Server Contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package isup

import (
	"encoding/hex"
	"fmt"
	"mime"
	"strings"
)

// Result is the ITU-T Q.763 decode of one or more application/isup bodies
// carried in a SIP message (SIP-I, RFC 3204). The SIP body starts at the
// ISUP message type; the circuit identification code is not included.
type Result struct {
	ContentType string    `json:"content_type,omitempty"`
	Version     string    `json:"version,omitempty"`
	Messages    []Message `json:"messages,omitempty"`
	Error       string    `json:"error,omitempty"`
}

// Message is one ISUP message.
type Message struct {
	Type                 int          `json:"type"`
	Acronym              string       `json:"acronym"`
	Name                 string       `json:"name"`
	CalledPartyNumber    *PartyNumber `json:"called_party_number,omitempty"`
	CallingPartyNumber   *PartyNumber `json:"calling_party_number,omitempty"`
	OriginalCalledNumber *PartyNumber `json:"original_called_number,omitempty"`
	RedirectingNumber    *PartyNumber `json:"redirecting_number,omitempty"`
	SubsequentNumber     string       `json:"subsequent_number,omitempty"`
	Cause                *Cause       `json:"cause,omitempty"`
	CallingPartyCategory string       `json:"calling_party_category,omitempty"`
	TransmissionMedium   string       `json:"transmission_medium,omitempty"`
	Event                string       `json:"event,omitempty"`
	Parameters           []Parameter  `json:"parameters,omitempty"`
	Hex                  string       `json:"hex,omitempty"`
	Summary              string       `json:"summary,omitempty"`
	Error                string       `json:"error,omitempty"`
}

// PartyNumber is an ISUP address (called, calling, redirecting, …).
type PartyNumber struct {
	Digits          string `json:"digits"`
	NatureOfAddress string `json:"nature_of_address,omitempty"`
	NumberingPlan   string `json:"numbering_plan,omitempty"`
	Presentation    string `json:"presentation,omitempty"`
	Screening       string `json:"screening,omitempty"`
	Incomplete      bool   `json:"incomplete,omitempty"`
}

// Cause is a Q.850 cause indicator.
type Cause struct {
	Value    int    `json:"value"`
	Text     string `json:"text,omitempty"`
	Location string `json:"location,omitempty"`
}

// Parameter is an ISUP parameter that is not promoted to a top-level field.
type Parameter struct {
	Code   int    `json:"code"`
	Name   string `json:"name"`
	Digits string `json:"digits,omitempty"`
	Text   string `json:"text,omitempty"`
	Hex    string `json:"hex,omitempty"`
}

// DecodeSIP finds application/isup bodies in a SIP message and decodes them.
// It understands a raw binary body and the hex form written into the SIP
// message by 11.0.376. A message that is not SIP-I returns nil.
func DecodeSIP(payload string) *Result {
	headers, nl, body, ok := splitSIP(payload)
	if !ok {
		return nil
	}
	ct, _ := headerValue(headers, nl, "Content-Type")
	if boundary, multipart := multipartBoundary(ct); multipart {
		return decodeMultipart(body, boundary, nl, ct)
	}
	version, isup := isISUPType(ct)
	if !isup {
		return nil
	}
	raw, err := entityBody(headers, nl, body)
	if err != nil {
		return &Result{ContentType: ct, Version: version, Error: "ISUP body could not be decoded"}
	}
	return &Result{
		ContentType: ct,
		Version:     version,
		Messages:    []Message{decodeOrReject(version, raw)},
	}
}

func decodeMultipart(body, boundary, nl, topCT string) *Result {
	marker := "--" + boundary
	if !strings.Contains(body, marker) {
		return nil
	}
	parts := strings.Split(body, marker)
	var messages []Message
	var version string
	var partCT string
	found := false
	for i := 1; i < len(parts); i++ {
		seg := parts[i]
		if strings.HasPrefix(seg, "--") {
			continue
		}
		rest := seg
		if strings.HasPrefix(rest, nl) {
			rest = rest[len(nl):]
		}
		if strings.HasSuffix(rest, nl) {
			rest = rest[:len(rest)-len(nl)]
		}
		ph, pnl, pb, ok := splitSIP(rest)
		if !ok {
			continue
		}
		pct, _ := headerValue(ph, pnl, "Content-Type")
		ver, isup := isISUPType(pct)
		if !isup {
			continue
		}
		found = true
		if partCT == "" {
			partCT = pct
			version = ver
		}
		raw, err := entityBody(ph, pnl, pb)
		if err != nil {
			messages = append(messages, Message{Name: "ISUP", Error: "ISUP body could not be decoded", Summary: "ISUP body could not be decoded"})
			continue
		}
		messages = append(messages, decodeOrReject(ver, raw))
	}
	if !found {
		return nil
	}
	ct := partCT
	if ct == "" {
		ct = topCT
	}
	return &Result{ContentType: ct, Version: version, Messages: messages}
}

func entityBody(headers, nl, body string) ([]byte, error) {
	enc, has := headerValue(headers, nl, binaryBodyHeader)
	if !has {
		return []byte(body), nil
	}
	return decodeEncodedBody(enc, body)
}

func decodeOrReject(version string, raw []byte) Message {
	if versionLooksANSI(version) {
		return Message{
			Name:    "ISUP",
			Hex:     hex.EncodeToString(raw),
			Error:   "ANSI ISUP is not decoded",
			Summary: "ANSI ISUP is not decoded",
		}
	}
	return Decode(raw)
}

func versionLooksANSI(version string) bool {
	return strings.Contains(strings.ToLower(version), "ansi")
}

func isISUPType(contentType string) (version string, ok bool) {
	media, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		if strings.Contains(strings.ToLower(contentType), "application/isup") {
			return "", true
		}
		return "", false
	}
	if !strings.EqualFold(media, "application/isup") {
		return "", false
	}
	return params["version"], true
}

type varKind uint8

const (
	varCalled varKind = iota + 1
	varCause
	varSubsequent
)

type msgLayout struct {
	acronym string
	name    string
	fixed   int
	vars    []varKind
	opt     bool
}

// Layouts follow ITU-T Q.763 for the message types SIP-I actually carries.
var msgLayouts = map[byte]msgLayout{
	1:  {"IAM", "Initial address", 5, []varKind{varCalled}, true},
	2:  {"SAM", "Subsequent address", 0, []varKind{varSubsequent}, true},
	3:  {"INR", "Information request", 2, nil, true},
	4:  {"INF", "Information", 2, nil, true},
	5:  {"COT", "Continuity", 1, nil, false},
	6:  {"ACM", "Address complete", 2, nil, true},
	7:  {"CON", "Connect", 2, nil, true},
	8:  {"FOT", "Forward transfer", 0, nil, true},
	9:  {"ANM", "Answer", 0, nil, true},
	12: {"REL", "Release", 0, []varKind{varCause}, true},
	13: {"SUS", "Suspend", 1, nil, true},
	14: {"RES", "Resume", 1, nil, true},
	16: {"RLC", "Release complete", 0, nil, true},
	17: {"CCR", "Continuity check request", 0, nil, false},
	18: {"RSC", "Reset circuit", 0, nil, false},
	19: {"BLO", "Blocking", 0, nil, false},
	20: {"UBL", "Unblocking", 0, nil, false},
	21: {"BLA", "Blocking acknowledgement", 0, nil, false},
	22: {"UBA", "Unblocking acknowledgement", 0, nil, false},
	44: {"CPG", "Call progress", 1, nil, true},
	47: {"CFN", "Confusion", 0, []varKind{varCause}, true},
}

// Decode parses one ITU-T ISUP message (no CIC).
func Decode(b []byte) Message {
	msg := Message{Hex: hex.EncodeToString(b)}
	if len(b) == 0 {
		msg.Error = "Empty ISUP body"
		msg.Summary = msg.Error
		return msg
	}
	msg.Type = int(b[0])
	layout, ok := msgLayouts[b[0]]
	if !ok {
		msg.Acronym = fmt.Sprintf("0x%02X", b[0])
		msg.Name = "Unknown message"
		msg.Error = "Unsupported ISUP message type"
		msg.Summary = msg.summary()
		return msg
	}
	msg.Acronym = layout.acronym
	msg.Name = layout.name
	if err := applyLayout(&msg, b, layout); err != nil {
		msg.Error = err.Error()
	}
	msg.Summary = msg.summary()
	return msg
}

func applyLayout(msg *Message, b []byte, layout msgLayout) error {
	need := 1 + layout.fixed + len(layout.vars)
	if layout.opt {
		need++
	}
	if len(b) < need {
		return fmt.Errorf("truncated ISUP message")
	}
	switch layout.acronym {
	case "IAM":
		msg.CallingPartyCategory = cpcName(int(b[4]))
		msg.TransmissionMedium = tmrName(int(b[5]))
	case "CPG":
		msg.Event = cpgEvent(b[1] & 0x7f)
	}
	base := 1 + layout.fixed
	for i, kind := range layout.vars {
		ptrPos := base + i
		start := ptrPos + int(b[ptrPos])
		val, err := varValue(b, start)
		if err != nil {
			return err
		}
		switch kind {
		case varCalled:
			if n, ok := decodeCalled(val); ok {
				msg.CalledPartyNumber = &n
			}
		case varCause:
			if c, ok := decodeCause(val); ok {
				msg.Cause = &c
			}
		case varSubsequent:
			if d, ok := decodeSubsequent(val); ok {
				msg.SubsequentNumber = d
			}
		}
	}
	if !layout.opt {
		return nil
	}
	ptrPos := base + len(layout.vars)
	if b[ptrPos] == 0 {
		return nil
	}
	start := ptrPos + int(b[ptrPos])
	if start > len(b) {
		return fmt.Errorf("truncated ISUP message")
	}
	return parseOptional(msg, b[start:])
}

func varValue(b []byte, start int) ([]byte, error) {
	if start < 0 || start >= len(b) {
		return nil, fmt.Errorf("truncated ISUP message")
	}
	ln := int(b[start])
	if start+1+ln > len(b) {
		return nil, fmt.Errorf("truncated ISUP message")
	}
	return b[start+1 : start+1+ln], nil
}

func parseOptional(msg *Message, b []byte) error {
	for len(b) > 0 {
		code := b[0]
		if code == 0 {
			return nil
		}
		if len(b) < 2 {
			return fmt.Errorf("truncated ISUP message")
		}
		ln := int(b[1])
		if len(b) < 2+ln {
			return fmt.Errorf("truncated ISUP message")
		}
		val := b[2 : 2+ln]
		b = b[2+ln:]
		msg.addOptional(code, val)
	}
	return nil
}

func (msg *Message) addOptional(code byte, val []byte) {
	switch code {
	case 4:
		if n, ok := decodeCalled(val); ok && msg.CalledPartyNumber == nil {
			msg.CalledPartyNumber = &n
		}
	case 5:
		if d, ok := decodeSubsequent(val); ok && msg.SubsequentNumber == "" {
			msg.SubsequentNumber = d
		}
	case 10:
		if n, ok := decodeCalling(val); ok {
			msg.CallingPartyNumber = &n
		}
	case 11:
		if n, ok := decodeCalling(val); ok {
			msg.RedirectingNumber = &n
		}
	case 12:
		if n, ok := decodeCalled(val); ok {
			msg.Parameters = append(msg.Parameters, Parameter{Code: int(code), Name: "Redirection number", Digits: n.Digits, Text: partyTail(&n)})
		}
	case 18:
		if c, ok := decodeCause(val); ok && msg.Cause == nil {
			msg.Cause = &c
		}
	case 33:
		if n, ok := decodeCalling(val); ok {
			msg.Parameters = append(msg.Parameters, Parameter{Code: int(code), Name: "Connected number", Digits: n.Digits, Text: partyTail(&n)})
		}
	case 40:
		if n, ok := decodeCalling(val); ok {
			msg.OriginalCalledNumber = &n
		}
	case 63:
		if n, ok := decodeCalling(val); ok {
			msg.Parameters = append(msg.Parameters, Parameter{Code: int(code), Name: "Location number", Digits: n.Digits, Text: partyTail(&n)})
		}
	case 192:
		qual, n, ok := decodeGeneric(val)
		if ok {
			msg.Parameters = append(msg.Parameters, Parameter{Code: int(code), Name: "Generic number", Digits: n.Digits, Text: qual})
		}
	default:
		msg.Parameters = append(msg.Parameters, Parameter{
			Code: int(code),
			Name: paramName(code),
			Hex:  hex.EncodeToString(val),
		})
	}
}

func decodeCalled(b []byte) (PartyNumber, bool) {
	if len(b) < 2 {
		return PartyNumber{}, false
	}
	return PartyNumber{
		Digits:          decodeDigits(b[2:], b[0]&0x80 != 0),
		NatureOfAddress: noaName(int(b[0] & 0x7f)),
		NumberingPlan:   npiName(int((b[1] >> 4) & 0x07)),
	}, true
}

func decodeCalling(b []byte) (PartyNumber, bool) {
	if len(b) < 2 {
		return PartyNumber{}, false
	}
	return PartyNumber{
		Digits:          decodeDigits(b[2:], b[0]&0x80 != 0),
		NatureOfAddress: noaName(int(b[0] & 0x7f)),
		NumberingPlan:   npiName(int((b[1] >> 4) & 0x07)),
		Presentation:    presentationName(int((b[1] >> 2) & 0x03)),
		Screening:       screeningName(int(b[1] & 0x03)),
		Incomplete:      b[1]&0x80 != 0,
	}, true
}

func decodeSubsequent(b []byte) (string, bool) {
	if len(b) < 1 {
		return "", false
	}
	return decodeDigits(b[1:], b[0]&0x80 != 0), true
}

func decodeGeneric(b []byte) (string, PartyNumber, bool) {
	if len(b) < 3 {
		return "", PartyNumber{}, false
	}
	n, ok := decodeCalling(b[1:])
	if !ok {
		return "", PartyNumber{}, false
	}
	return genericQualifier(int(b[0])), n, true
}

func decodeCause(b []byte) (Cause, bool) {
	if len(b) < 2 {
		return Cause{}, false
	}
	i := 1
	if b[0]&0x80 == 0 {
		if len(b) < 3 {
			return Cause{}, false
		}
		i = 2
	}
	v := int(b[i] & 0x7f)
	return Cause{Value: v, Text: causeText[v], Location: causeLocation(int(b[0] & 0x0f))}, true
}

func decodeDigits(raw []byte, odd bool) string {
	var b strings.Builder
	for i, c := range raw {
		b.WriteByte(isupDigit(c & 0x0f))
		if i == len(raw)-1 && odd {
			break
		}
		hi := c >> 4
		if hi == 0x0f {
			break
		}
		b.WriteByte(isupDigit(hi))
	}
	return b.String()
}

func isupDigit(n byte) byte {
	switch n {
	case 0, 1, 2, 3, 4, 5, 6, 7, 8, 9:
		return '0' + n
	case 10:
		return 'A'
	case 11:
		return 'B'
	case 12:
		return 'C'
	case 13:
		return '*'
	case 14:
		return '#'
	default:
		return 'F'
	}
}

func (m Message) summary() string {
	var b strings.Builder
	if m.Acronym != "" && m.Name != "" {
		fmt.Fprintf(&b, "%s — %s", m.Acronym, m.Name)
	} else if m.Name != "" {
		b.WriteString(m.Name)
	}
	if s := formatParty(m.CalledPartyNumber); s != "" {
		fmt.Fprintf(&b, "\nCalled party number: %s", s)
	}
	if s := formatParty(m.CallingPartyNumber); s != "" {
		fmt.Fprintf(&b, "\nCalling party number: %s", s)
	}
	if s := formatParty(m.OriginalCalledNumber); s != "" {
		fmt.Fprintf(&b, "\nOriginal called number: %s", s)
	}
	if s := formatParty(m.RedirectingNumber); s != "" {
		fmt.Fprintf(&b, "\nRedirecting number: %s", s)
	}
	if m.SubsequentNumber != "" {
		fmt.Fprintf(&b, "\nSubsequent number: %s", m.SubsequentNumber)
	}
	if m.Cause != nil {
		if m.Cause.Text != "" {
			fmt.Fprintf(&b, "\nCause: %d %s", m.Cause.Value, m.Cause.Text)
		} else {
			fmt.Fprintf(&b, "\nCause: %d", m.Cause.Value)
		}
		if m.Cause.Location != "" {
			fmt.Fprintf(&b, " (%s)", m.Cause.Location)
		}
	}
	if m.CallingPartyCategory != "" {
		fmt.Fprintf(&b, "\nCalling party's category: %s", m.CallingPartyCategory)
	}
	if m.TransmissionMedium != "" {
		fmt.Fprintf(&b, "\nTransmission medium: %s", m.TransmissionMedium)
	}
	if m.Event != "" {
		fmt.Fprintf(&b, "\nEvent: %s", m.Event)
	}
	for _, p := range m.Parameters {
		switch {
		case p.Digits != "" && p.Text != "":
			fmt.Fprintf(&b, "\n%s: %s (%s)", p.Name, p.Digits, p.Text)
		case p.Digits != "":
			fmt.Fprintf(&b, "\n%s: %s", p.Name, p.Digits)
		case p.Text != "":
			fmt.Fprintf(&b, "\n%s: %s", p.Name, p.Text)
		case p.Hex != "":
			fmt.Fprintf(&b, "\n%s: %s", p.Name, p.Hex)
		}
	}
	if m.Error != "" {
		if b.Len() > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(m.Error)
	}
	return b.String()
}

func formatParty(n *PartyNumber) string {
	if n == nil || n.Digits == "" {
		return ""
	}
	tail := partyTail(n)
	if tail == "" {
		return n.Digits
	}
	return n.Digits + " (" + tail + ")"
}

func partyTail(n *PartyNumber) string {
	if n == nil {
		return ""
	}
	var extra []string
	if n.NatureOfAddress != "" {
		extra = append(extra, n.NatureOfAddress)
	}
	if n.NumberingPlan != "" {
		extra = append(extra, n.NumberingPlan)
	}
	if n.Presentation != "" {
		extra = append(extra, n.Presentation)
	}
	if n.Screening != "" {
		extra = append(extra, n.Screening)
	}
	if n.Incomplete {
		extra = append(extra, "incomplete")
	}
	return strings.Join(extra, ", ")
}

func noaName(v int) string {
	switch v {
	case 1:
		return "subscriber number"
	case 2:
		return "unknown"
	case 3:
		return "national (significant) number"
	case 4:
		return "international number"
	case 5:
		return "network-specific number"
	case 6:
		return "network routing number, national format"
	case 7:
		return "network routing number, network-specific format"
	default:
		return fmt.Sprintf("nature of address %d", v)
	}
}

func npiName(v int) string {
	switch v {
	case 1:
		return "ISDN (E.164)"
	case 3:
		return "data (X.121)"
	case 4:
		return "telex"
	case 5:
		return "private"
	case 6:
		return "reserved for national use"
	default:
		return fmt.Sprintf("numbering plan %d", v)
	}
}

func presentationName(v int) string {
	switch v {
	case 0:
		return "presentation allowed"
	case 1:
		return "presentation restricted"
	case 2:
		return "address not available"
	default:
		return fmt.Sprintf("presentation %d", v)
	}
}

func screeningName(v int) string {
	switch v {
	case 0:
		return "user provided, not verified"
	case 1:
		return "user provided, verified and passed"
	case 2:
		return "user provided, verified and failed"
	case 3:
		return "network provided"
	default:
		return ""
	}
}

func cpcName(v int) string {
	switch v {
	case 0:
		return "unknown"
	case 10:
		return "ordinary calling subscriber"
	case 11:
		return "calling subscriber with priority"
	case 12:
		return "data call"
	case 13:
		return "test call"
	case 15:
		return "payphone"
	default:
		return fmt.Sprintf("category %d", v)
	}
}

func tmrName(v int) string {
	switch v {
	case 0:
		return "speech"
	case 2:
		return "64 kbit/s unrestricted"
	case 3:
		return "3.1 kHz audio"
	case 6:
		return "64 kbit/s preferred"
	default:
		return fmt.Sprintf("transmission medium %d", v)
	}
}

func cpgEvent(v byte) string {
	switch v {
	case 1:
		return "alerting"
	case 2:
		return "progress"
	case 3:
		return "in-band information available"
	case 4:
		return "call forwarded on busy"
	case 5:
		return "call forwarded on no reply"
	case 6:
		return "call forwarded unconditional"
	default:
		return fmt.Sprintf("event %d", v)
	}
}

func causeLocation(v int) string {
	switch v {
	case 0:
		return "user"
	case 1:
		return "private network serving the local user"
	case 2:
		return "public network serving the local user"
	case 3:
		return "transit network"
	case 4:
		return "public network serving the remote user"
	case 5:
		return "private network serving the remote user"
	case 7:
		return "international network"
	case 10:
		return "beyond an interworking point"
	default:
		return fmt.Sprintf("location %d", v)
	}
}

func genericQualifier(v int) string {
	switch v {
	case 1:
		return "additional called number"
	case 5:
		return "additional connected number"
	case 6:
		return "additional calling party number"
	case 7:
		return "additional original called number"
	case 8:
		return "additional redirecting number"
	case 9:
		return "additional redirection number"
	default:
		return fmt.Sprintf("qualifier %d", v)
	}
}

func paramName(code byte) string {
	if name, ok := paramNames[code]; ok {
		return name
	}
	return fmt.Sprintf("Parameter 0x%02X", code)
}

var paramNames = map[byte]string{
	1:   "Call reference",
	2:   "Transmission medium requirement",
	3:   "Access transport",
	4:   "Called party number",
	5:   "Subsequent number",
	6:   "Nature of connection indicators",
	7:   "Forward call indicators",
	8:   "Optional forward call indicators",
	9:   "Calling party's category",
	10:  "Calling party number",
	11:  "Redirecting number",
	12:  "Redirection number",
	13:  "Connection request",
	16:  "Continuity indicators",
	17:  "Backward call indicators",
	18:  "Cause indicators",
	19:  "Redirection information",
	29:  "User service information",
	32:  "User-to-user information",
	33:  "Connected number",
	36:  "Event information",
	40:  "Original called number",
	41:  "Optional backward call indicators",
	42:  "User-to-user indicators",
	44:  "Generic notification indicator",
	57:  "Parameter compatibility information",
	61:  "Hop counter",
	63:  "Location number",
	192: "Generic number",
	193: "Generic digits",
}

var causeText = map[int]string{
	1:   "Unallocated number",
	2:   "No route to specified transit network",
	3:   "No route to destination",
	16:  "Normal call clearing",
	17:  "User busy",
	18:  "No user responding",
	19:  "No answer from the user",
	20:  "Subscriber absent",
	21:  "Call rejected",
	22:  "Number changed",
	27:  "Destination out of order",
	28:  "Invalid number format",
	29:  "Facility rejected",
	31:  "Normal, unspecified",
	34:  "No circuit/channel available",
	38:  "Network out of order",
	41:  "Temporary failure",
	42:  "Switching equipment congestion",
	44:  "Requested circuit/channel not available",
	47:  "Resource unavailable, unspecified",
	50:  "Requested facility not subscribed",
	55:  "Incoming calls barred within CUG",
	57:  "Bearer capability not authorized",
	58:  "Bearer capability not presently available",
	63:  "Service or option not available, unspecified",
	65:  "Bearer capability not implemented",
	79:  "Service or option not implemented, unspecified",
	87:  "User not a member of CUG",
	88:  "Incompatible destination",
	95:  "Invalid message, unspecified",
	97:  "Message type non-existent or not implemented",
	99:  "Information element non-existent or not implemented",
	102: "Recovery on timer expiry",
	111: "Protocol error, unspecified",
	127: "Interworking, unspecified",
}
