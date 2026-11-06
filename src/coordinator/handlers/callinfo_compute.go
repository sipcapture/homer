// Copyright (C) 2025 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// SIP call-info metrics per Call-ID (session_id), aligned with legacy Homer
// tab-callinfo timing (ringing, answered duration, setup delays, codecs).

package handlers

import (
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

const callInfoMaxRows = 50000

// rfc3551Static maps static RTP payload types to their encoding names.
// Dynamic types (96–127) stay unnamed until an a=rtpmap line provides one.
// Reserved and unassigned types are omitted; an rtpmap still overrides these.
var rfc3551Static = map[int]string{
	0:  "PCMU",
	3:  "GSM",
	4:  "G723",
	5:  "DVI4",
	6:  "DVI4",
	7:  "LPC",
	8:  "PCMA",
	9:  "G722",
	10: "L16",
	11: "L16",
	12: "QCELP",
	13: "CN",
	14: "MPA",
	15: "G728",
	16: "DVI4",
	17: "DVI4",
	18: "G729",
	25: "CelB",
	26: "JPEG",
	28: "nv",
	31: "H261",
	32: "MPV",
	33: "MP2T",
	34: "H263",
}

func stringFrom(v interface{}) string {
	if v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return t
	case []byte:
		return string(t)
	case json.Number:
		return t.String()
	case float64:
		if t == math.Trunc(t) {
			return strconv.FormatInt(int64(t), 10)
		}
		return strconv.FormatFloat(t, 'f', -1, 64)
	case int64:
		return strconv.FormatInt(t, 10)
	case int:
		return strconv.Itoa(t)
	default:
		return fmt.Sprint(t)
	}
}

func intFrom(v interface{}) int {
	s := strings.TrimSpace(stringFrom(v))
	if s == "" {
		return 0
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0
	}
	return n
}

// rowTimestampMs returns Unix milliseconds for a SIP row timestamp (JSON / DuckDB shapes).
func rowTimestampMs(v interface{}) int64 {
	if v == nil {
		return 0
	}
	switch t := v.(type) {
	case time.Time:
		return t.UnixMilli()
	case json.Number:
		f, err := t.Float64()
		if err != nil {
			return 0
		}
		ax := math.Abs(f)
		if ax >= 1e16 {
			return int64(math.Round(f / 1e6))
		}
		if ax >= 1e14 {
			return int64(math.Round(f / 1e3))
		}
		if ax >= 1e11 {
			return int64(math.Round(f))
		}
		if ax >= 1e8 {
			return int64(math.Round(f * 1000))
		}
		return int64(math.Round(f * 1000))
	case float64:
		return rowTimestampMs(json.Number(strconv.FormatFloat(t, 'f', -1, 64)))
	case string:
		s := strings.TrimSpace(t)
		if s == "" {
			return 0
		}
		if n, err := strconv.ParseInt(s, 10, 64); err == nil {
			ax := math.Abs(float64(n))
			if ax >= 1e16 {
				return n / 1e6
			}
			if ax >= 1e14 {
				return n / 1e3
			}
			if ax >= 1e11 {
				return n
			}
			if ax >= 1e8 {
				return n * 1000
			}
			return n * 1000
		}
		layouts := []string{
			time.RFC3339Nano,
			"2006-01-02 15:04:05.999999999",
			"2006-01-02 15:04:05.999999",
			"2006-01-02 15:04:05.999",
			"2006-01-02 15:04:05",
			"2006-01-02T15:04:05.999999999Z07:00",
		}
		for _, layout := range layouts {
			if d, err := time.Parse(layout, s); err == nil {
				return d.UnixMilli()
			}
		}
		return 0
	default:
		return rowTimestampMs(fmt.Sprint(t))
	}
}

func parseDataExtraMap(v interface{}) map[string]interface{} {
	if v == nil {
		return nil
	}
	if m, ok := v.(map[string]interface{}); ok {
		return m
	}
	s := stringFrom(v)
	if s == "" || s == "{}" {
		return nil
	}
	var m map[string]interface{}
	if json.Unmarshal([]byte(s), &m) == nil {
		return m
	}
	return nil
}

func headerValue(payload, name string) string {
	if payload == "" {
		return ""
	}
	// name e.g. "User-Agent"
	re := regexp.MustCompile(`(?i)^` + regexp.QuoteMeta(name) + `:\s*(.+)$`)
	for _, line := range strings.Split(payload, "\n") {
		line = strings.TrimRight(line, "\r")
		if m := re.FindStringSubmatch(line); len(m) > 1 {
			return strings.TrimSpace(m[1])
		}
	}
	return ""
}

func pickUA(extra map[string]interface{}, payload string) string {
	if extra != nil {
		for _, k := range []string{"user_agent", "User-Agent"} {
			if v, ok := extra[k]; ok {
				if s := strings.TrimSpace(stringFrom(v)); s != "" {
					return s
				}
			}
		}
		for _, k := range []string{"server", "Server"} {
			if v, ok := extra[k]; ok {
				if s := strings.TrimSpace(stringFrom(v)); s != "" {
					return s
				}
			}
		}
	}
	if ua := headerValue(payload, "User-Agent"); ua != "" {
		return ua
	}
	if sv := headerValue(payload, "Server"); sv != "" {
		return sv
	}
	return ""
}

// extractCodecsFromSDP lists codecs from every active media section.
// Payload types on the m= line are resolved with a=rtpmap when present,
// otherwise with the RFC 3551 static table. Sections with port 0 are
// rejected streams and contribute nothing. Order follows the m= lines.
func extractCodecsFromSDP(payload string) string {
	if payload == "" {
		return ""
	}
	if !strings.Contains(strings.ToLower(payload), "m=") {
		return ""
	}
	payload = strings.ReplaceAll(payload, "\r\n", "\n")
	payload = strings.ReplaceAll(payload, "\r", "\n")

	seen := make(map[string]struct{})
	var out []string
	var cur *sdpMediaSection
	finish := func() {
		if cur == nil || cur.port == 0 {
			return
		}
		for _, pt := range cur.pts {
			name := cur.rtpmap[pt]
			if name == "" {
				name = rfc3551Static[pt]
			}
			if name == "" {
				continue
			}
			if _, ok := seen[name]; ok {
				continue
			}
			seen[name] = struct{}{}
			out = append(out, name)
		}
	}
	for _, line := range strings.Split(payload, "\n") {
		line = strings.TrimSpace(line)
		if len(line) < 2 || line[1] != '=' {
			continue
		}
		switch line[0] {
		case 'm', 'M':
			finish()
			cur = parseSDPMedia(line[2:])
		case 'a', 'A':
			if cur == nil {
				continue
			}
			if pt, name, ok := parseSDPRtpmap(line); ok {
				cur.rtpmap[pt] = name
			}
		}
	}
	finish()
	return strings.Join(out, ", ")
}

type sdpMediaSection struct {
	port   int
	pts    []int
	rtpmap map[int]string
}

func parseSDPMedia(val string) *sdpMediaSection {
	fields := strings.Fields(val)
	if len(fields) < 3 {
		return nil
	}
	port, err := strconv.Atoi(fields[1])
	if err != nil || port < 0 {
		return nil
	}
	sec := &sdpMediaSection{port: port, rtpmap: make(map[int]string)}
	for _, f := range fields[3:] {
		n, err := strconv.Atoi(f)
		if err != nil || n < 0 || n > 127 {
			continue
		}
		sec.pts = append(sec.pts, n)
	}
	return sec
}

func parseSDPRtpmap(line string) (pt int, name string, ok bool) {
	val := line[2:]
	const key = "rtpmap:"
	if len(val) < len(key) || !strings.EqualFold(val[:len(key)], key) {
		return 0, "", false
	}
	fields := strings.Fields(val[len(key):])
	if len(fields) < 2 {
		return 0, "", false
	}
	n, err := strconv.Atoi(fields[0])
	if err != nil || n < 0 || n > 127 {
		return 0, "", false
	}
	name = fields[1]
	if i := strings.IndexByte(name, '/'); i >= 0 {
		name = name[:i]
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return 0, "", false
	}
	return n, name, true
}

// cseqNumber reads the sequence from the CSeq header, stopping at the body.
func cseqNumber(payload string) int {
	if payload == "" {
		return 0
	}
	payload = strings.ReplaceAll(payload, "\r\n", "\n")
	payload = strings.ReplaceAll(payload, "\r", "\n")
	started := false
	for _, line := range strings.Split(payload, "\n") {
		if line == "" {
			if !started {
				continue
			}
			break
		}
		started = true
		if len(line) < 5 || !strings.EqualFold(line[:5], "cseq:") {
			continue
		}
		rest := strings.TrimSpace(line[5:])
		field, _, _ := strings.Cut(rest, " ")
		n, err := strconv.Atoi(field)
		if err != nil || n < 0 {
			return 0
		}
		return n
	}
	return 0
}

func sipReplyAndCSeqMethod(methodCol, respCol, cseqCol string) (reply int, cseqMethod string) {
	cseqMethod = strings.TrimSpace(strings.ToUpper(cseqCol))
	rc := strings.TrimSpace(respCol)
	if rc != "" && rc != "0" {
		if v, err := strconv.Atoi(rc); err == nil && v > 0 {
			return v, cseqMethod
		}
	}
	mc := strings.TrimSpace(methodCol)
	if v, err := strconv.Atoi(mc); err == nil && v >= 100 && v < 700 {
		return v, cseqMethod
	}
	return 0, cseqMethod
}

// eventLabelForMetrics labels each SIP row for method histogram (legacy Homer chart).
func eventLabelForMetrics(methodCol string, reply int, reqMethod string) string {
	if reqMethod != "" {
		return reqMethod
	}
	if reply >= 100 && reply < 700 {
		return strconv.Itoa(reply)
	}
	s := strings.TrimSpace(methodCol)
	if s != "" {
		return s
	}
	return ""
}

func sipRequestMethod(methodCol, respCol string) string {
	rc := strings.TrimSpace(respCol)
	if rc != "" && rc != "0" {
		if _, err := strconv.Atoi(rc); err == nil {
			return ""
		}
	}
	mc := strings.TrimSpace(methodCol)
	if mc == "" {
		return ""
	}
	if v, err := strconv.Atoi(mc); err == nil && v >= 100 && v < 700 {
		return ""
	}
	return strings.ToUpper(mc)
}

// homerStatusText maps internal Homer UI status codes (legacy tab-callinfo).
func homerStatusText(code int) string {
	switch code {
	case 0:
		return "Unknown"
	case 1:
		return "Invite"
	case 2:
		return "Unauth"
	case 3:
		return "183 Early"
	case 4:
		return "Ringing"
	case 5:
		return "Connected"
	case 6:
		return "Forwarded"
	case 7:
		return "Busy"
	case 8:
		return "Rejected"
	case 9:
		return "Server error"
	case 10:
		return "Finished"
	case 11:
		return "Cancelled"
	case 12:
		return "Timeout"
	case 14:
		return "Global failure"
	default:
		return fmt.Sprintf("Status %d", code)
	}
}

type sipCallLegMetrics struct {
	sessionID string

	timeInvite, timeBye, timeCancel int64
	cdrRinging, cdrConnect, cdrStop int64
	firstMs, lastMs                 int64

	RingingTime                    int64
	Duration                       int64 // answered leg: stop - connect (ms)
	SessionDur                     int64 // last - first (ms)
	SRD                            int64 // first response after INVITE (any 1xx-6xx)
	SSS                            int64 // first 101-199 after INVITE
	FSSD                           int64 // failed setup (INVITE error) delay
	SDD                            int64 // 200 OK to BYE disconnect delay
	Status                         int
	LastBadReply                   int
	UAC, UAS                       string
	SrcIP, SrcPort, DstIP, DstPort string
	Caller, Callee                 string
	MsgCount                       int
	// CodecsOffered is the first SDP offer (INVITE, or the 200 when the
	// INVITE had none). CodecNegotiated is the latest answer: 200 OK to
	// INVITE/UPDATE, or the ACK when the offer was in the 200.
	CodecsOffered   string
	CodecNegotiated string
	// sdpOfferTxn records INVITE/UPDATE transactions whose request carried
	// an offer. sdpAnsweredTxn records transactions whose 200 was already
	// consumed, so a retransmission is not treated as a new offer.
	sdpOfferTxn      map[string]struct{}
	sdpAnsweredTxn   map[string]struct{}
	hasDelayedOffer  bool
	delayedOfferCSeq int
	Methods          map[string]int
	FromParty        string
	RuriParty        string
}

func sdpTxnKey(method string, cseq int) string {
	return method + ":" + strconv.Itoa(cseq)
}

// takeOffer finds the request that this 200 answers. The CSeq number is
// preferred. A single outstanding offer is used only when one side omitted
// the CSeq header, so a different transaction number is not paired with it.
func (m *sipCallLegMetrics) takeOffer(method string, cseq int) (string, bool) {
	key := sdpTxnKey(method, cseq)
	if _, ok := m.sdpOfferTxn[key]; ok {
		return key, true
	}
	prefix := method + ":"
	found := ""
	count := 0
	for k := range m.sdpOfferTxn {
		if strings.HasPrefix(k, prefix) {
			found = k
			count++
		}
	}
	if count == 1 && (cseq == 0 || strings.HasSuffix(found, ":0")) {
		return found, true
	}
	return "", false
}

func (m *sipCallLegMetrics) noteOffer(codecs string) {
	if codecs != "" && m.CodecsOffered == "" {
		m.CodecsOffered = codecs
	}
}

func (m *sipCallLegMetrics) noteAnswer(codecs string) {
	if codecs != "" {
		m.CodecNegotiated = codecs
	}
}

// observeSDP pairs SDP offers and answers on a single call leg.
// A request with SDP is the offer. The matching 200 is the answer, except
// when the request had no SDP: then the 200 is a delayed offer and the ACK
// carries the answer. A later re-INVITE or UPDATE answer replaces the
// negotiated list; the original offer is kept.
func (m *sipCallLegMetrics) observeSDP(reqMethod string, reply int, cseqMethod, payload string) {
	n := cseqNumber(payload)
	switch reqMethod {
	case "INVITE", "UPDATE":
		key := sdpTxnKey(reqMethod, n)
		// A new request reuses the CSeq key when the header is missing (0).
		// Drop the previous answer mark so this transaction's 200 is applied.
		delete(m.sdpAnsweredTxn, key)
		if c := extractCodecsFromSDP(payload); c != "" {
			m.noteOffer(c)
			m.sdpOfferTxn[key] = struct{}{}
		} else {
			delete(m.sdpOfferTxn, key)
		}
		return
	case "ACK":
		if m.hasDelayedOffer && (n == m.delayedOfferCSeq || n == 0 || m.delayedOfferCSeq == 0) {
			if c := extractCodecsFromSDP(payload); c != "" {
				m.noteAnswer(c)
			}
			m.hasDelayedOffer = false
		}
		return
	}
	if reply != 200 || (cseqMethod != "INVITE" && cseqMethod != "UPDATE") {
		return
	}
	if _, answered := m.sdpAnsweredTxn[sdpTxnKey(cseqMethod, n)]; answered {
		return
	}
	c := extractCodecsFromSDP(payload)
	if key, ok := m.takeOffer(cseqMethod, n); ok {
		m.noteAnswer(c)
		delete(m.sdpOfferTxn, key)
		m.sdpAnsweredTxn[key] = struct{}{}
		m.sdpAnsweredTxn[sdpTxnKey(cseqMethod, n)] = struct{}{}
		return
	}
	if c == "" {
		return
	}
	m.noteOffer(c)
	m.hasDelayedOffer = true
	m.delayedOfferCSeq = n
	m.sdpAnsweredTxn[sdpTxnKey(cseqMethod, n)] = struct{}{}
}

func buildFromParty(caller string, extra map[string]interface{}) string {
	c := strings.TrimSpace(caller)
	fh := strings.TrimSpace(stringFrom(extra["from_host"]))
	if c != "" && fh != "" {
		return c + "@" + fh
	}
	if c != "" {
		return c
	}
	return ""
}

func buildRURIParty(callee string, extra map[string]interface{}) string {
	ru := strings.TrimSpace(stringFrom(extra["request_uri"]))
	if ru != "" {
		return ru
	}
	c := strings.TrimSpace(callee)
	th := strings.TrimSpace(stringFrom(extra["to_host"]))
	if c != "" && th != "" {
		return c + "@" + th
	}
	if c != "" {
		return c
	}
	return ""
}

func computeSIPCallLeg(sessionID string, rows []map[string]interface{}) map[string]interface{} {
	st := &sipCallLegMetrics{
		sessionID:      sessionID,
		Methods:        make(map[string]int),
		sdpOfferTxn:    make(map[string]struct{}),
		sdpAnsweredTxn: make(map[string]struct{}),
	}
	for _, row := range rows {
		ts := rowTimestampMs(row["timestamp"])
		if ts <= 0 {
			continue
		}
		if st.firstMs == 0 || ts < st.firstMs {
			st.firstMs = ts
		}
		if ts > st.lastMs {
			st.lastMs = ts
		}
		st.MsgCount++

		methodCol := stringFrom(row["method"])
		respCol := stringFrom(row["response_code"])
		cseqM := stringFrom(row["cseq_method"])
		reply, cseqMethod := sipReplyAndCSeqMethod(methodCol, respCol, cseqM)
		reqMethod := sipRequestMethod(methodCol, respCol)
		payload := stringFrom(row["payload"])
		extra := parseDataExtraMap(row["data_extra"])
		st.observeSDP(reqMethod, reply, cseqMethod, payload)

		if lbl := eventLabelForMetrics(methodCol, reply, reqMethod); lbl != "" {
			st.Methods[lbl]++
		}

		if reqMethod == "INVITE" && st.timeInvite == 0 {
			st.timeInvite = ts
			st.UAC = pickUA(extra, payload)
			st.Caller = stringFrom(row["caller"])
			st.Callee = stringFrom(row["callee"])
			st.SrcIP = stringFrom(row["src_ip"])
			st.SrcPort = stringFrom(row["src_port"])
			st.DstIP = stringFrom(row["dst_ip"])
			st.DstPort = stringFrom(row["dst_port"])
			st.FromParty = buildFromParty(st.Caller, extra)
			st.RuriParty = buildRURIParty(st.Callee, extra)
			st.Status = 1
		} else if reqMethod == "BYE" && st.timeBye == 0 {
			st.timeBye = ts
			st.cdrStop = ts
			st.Status = 10
			if st.cdrConnect != 0 && st.cdrConnect < st.cdrStop {
				st.Duration = st.cdrStop - st.cdrConnect
			}
			if st.cdrRinging != 0 && st.RingingTime == 0 && st.cdrRinging < st.cdrStop {
				st.RingingTime = st.cdrStop - st.cdrRinging
			}
		} else if reqMethod == "CANCEL" && st.timeCancel == 0 {
			st.timeCancel = ts
			st.cdrStop = ts
			if st.cdrRinging != 0 && st.RingingTime == 0 && st.cdrRinging < st.cdrStop {
				st.RingingTime = st.cdrStop - st.cdrRinging
			}
			st.Status = 11
		} else if reply >= 100 && reply < 700 {
			if st.timeInvite != 0 && st.SRD == 0 {
				st.SRD = ts - st.timeInvite
			}
			if reply > 100 && reply < 200 && st.SSS == 0 && st.timeInvite != 0 {
				st.SSS = ts - st.timeInvite
				st.UAS = pickUA(extra, payload)
			}
			if reply == 183 && st.cdrRinging == 0 {
				st.Status = 3
				st.cdrRinging = ts
			} else if reply == 180 && (st.cdrRinging == 0 || st.Status == 3) {
				st.cdrRinging = ts
				st.Status = 4
			} else if reply == 200 && st.cdrConnect == 0 && cseqMethod == "INVITE" {
				st.cdrConnect = ts
				st.cdrStop = 0
				st.Status = 5
				if st.cdrRinging != 0 && st.RingingTime == 0 && st.cdrRinging < st.cdrConnect {
					st.RingingTime = st.cdrConnect - st.cdrRinging
				}
				st.UAS = pickUA(extra, payload)
			} else if reply > 400 && reply < 700 && reply != 401 && reply != 402 && reply != 407 && reply != 487 &&
				st.FSSD == 0 && cseqMethod == "INVITE" {
				switch {
				case reply == 486:
					st.Status = 7
				case reply == 480:
					st.Status = 12
				case reply >= 400 && reply < 500:
					st.Status = 8
				case reply >= 500 && reply < 600:
					st.Status = 9
				case reply >= 600:
					st.Status = 14
				}
				st.cdrStop = ts
				if ts > st.timeInvite && st.timeInvite != 0 {
					st.FSSD = ts - st.timeInvite
				}
				if st.cdrRinging != 0 && st.RingingTime == 0 && st.cdrRinging < st.cdrStop {
					st.RingingTime = st.cdrStop - st.cdrRinging
				}
			}
			if reply == 401 || (reply == 407 && cseqMethod == "INVITE") {
				st.Status = 2
			}
			if reply > 300 && reply < 400 && cseqMethod == "INVITE" {
				st.Status = 6
				st.cdrStop = ts
				if st.cdrRinging != 0 && st.RingingTime == 0 && st.cdrRinging < st.cdrStop {
					st.RingingTime = st.cdrStop - st.cdrRinging
				}
			}
			if reply > 400 && reply < 700 {
				st.LastBadReply = reply
			}
			if reply == 200 && st.SDD == 0 && cseqMethod == "BYE" {
				if st.timeBye != 0 && st.timeBye < ts {
					st.SDD = ts - st.timeBye
				}
				st.Status = 10
			}
		}
	}

	if st.firstMs > 0 && st.lastMs >= st.firstMs {
		st.SessionDur = st.lastMs - st.firstMs
	}

	type methodPair struct {
		k string
		v int
	}
	var mpairs []methodPair
	for k, v := range st.Methods {
		if v > 0 {
			mpairs = append(mpairs, methodPair{k, v})
		}
	}
	sort.Slice(mpairs, func(i, j int) bool {
		if mpairs[i].v != mpairs[j].v {
			return mpairs[i].v > mpairs[j].v
		}
		return mpairs[i].k < mpairs[j].k
	})
	var methodsDist []map[string]interface{}
	for _, p := range mpairs {
		methodsDist = append(methodsDist, map[string]interface{}{
			"method": p.k,
			"count":  p.v,
		})
	}

	codecs := st.CodecNegotiated
	if codecs == "" {
		codecs = st.CodecsOffered
	}

	out := map[string]interface{}{
		"session_id": st.sessionID,
		"caller":     nullIfEmpty(st.Caller),
		"callee":     nullIfEmpty(st.Callee),
		"uac":        nullIfEmpty(st.UAC),
		"uas":        nullIfEmpty(st.UAS),
		// codecs is the negotiated set when an answer exists, otherwise the offer.
		"codecs":           nullIfEmpty(codecs),
		"codecs_offered":   nullIfEmpty(st.CodecsOffered),
		"codec_negotiated": nullIfEmpty(st.CodecNegotiated),
		"status":           homerStatusText(st.Status),
		"status_code": func() interface{} {
			if st.Status == 0 {
				return nil
			}
			return st.Status
		}(),
		"message_count": st.MsgCount,
		"last_bad_reply": func() interface{} {
			if st.LastBadReply == 0 {
				return nil
			}
			return st.LastBadReply
		}(),
	}

	if st.SrcIP != "" {
		out["source"] = strings.TrimSpace(st.SrcIP + ":" + st.SrcPort)
	}
	if st.DstIP != "" {
		out["destination"] = strings.TrimSpace(st.DstIP + ":" + st.DstPort)
	}

	putFloatSec := func(key string, ms int64) {
		if ms > 0 {
			out[key] = math.Round(float64(ms)/10) / 100
		}
	}
	putFloatSec("ringing_seconds", st.RingingTime)
	putFloatSec("call_duration_seconds", st.Duration)
	putFloatSec("session_duration_seconds", st.SessionDur)

	putIntMs := func(key string, v int64) {
		if v > 0 {
			out[key] = v
		}
	}
	putIntMs("session_request_delay_ms", st.SRD)
	putIntMs("session_setup_delay_ms", st.SSS)
	putIntMs("failed_session_setup_delay_ms", st.FSSD)
	putIntMs("session_disconnect_delay_ms", st.SDD)

	if st.firstMs > 0 {
		out["first_seen"] = time.UnixMilli(st.firstMs).UTC().Format(time.RFC3339Nano)
	}
	if st.lastMs > 0 {
		out["last_seen"] = time.UnixMilli(st.lastMs).UTC().Format(time.RFC3339Nano)
	}

	if strings.TrimSpace(st.FromParty) != "" {
		out["from_party"] = st.FromParty
	}
	if strings.TrimSpace(st.RuriParty) != "" {
		out["ruri_party"] = st.RuriParty
	}
	if len(methodsDist) > 0 {
		out["methods_distribution"] = methodsDist
	}

	return out
}

func nullIfEmpty(s string) interface{} {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return s
}

// computeSIPCallInfoRows sorts SIP rows, groups by session_id, and returns one summary map per leg.
func computeSIPCallInfoRows(rows []map[string]interface{}) []map[string]interface{} {
	if len(rows) == 0 {
		return nil
	}
	sort.SliceStable(rows, func(i, j int) bool {
		si := stringFrom(rows[i]["session_id"])
		sj := stringFrom(rows[j]["session_id"])
		if si != sj {
			return si < sj
		}
		ti := rowTimestampMs(rows[i]["timestamp"])
		tj := rowTimestampMs(rows[j]["timestamp"])
		if ti != tj {
			return ti < tj
		}
		return stringFrom(rows[i]["uuid"]) < stringFrom(rows[j]["uuid"])
	})

	bySession := make([][]map[string]interface{}, 0, 8)
	var curSid string
	var curGroup []map[string]interface{}
	flush := func() {
		if len(curGroup) == 0 {
			return
		}
		bySession = append(bySession, curGroup)
		curGroup = nil
	}
	for _, row := range rows {
		sid := stringFrom(row["session_id"])
		if sid == "" {
			continue
		}
		if sid != curSid {
			flush()
			curSid = sid
		}
		curGroup = append(curGroup, row)
	}
	flush()

	out := make([]map[string]interface{}, 0, len(bySession))
	for _, grp := range bySession {
		if len(grp) == 0 {
			continue
		}
		sid := stringFrom(grp[0]["session_id"])
		out = append(out, computeSIPCallLeg(sid, grp))
	}
	return out
}
