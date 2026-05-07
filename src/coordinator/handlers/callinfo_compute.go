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

var rtpmapCodecRe = regexp.MustCompile(`(?i)a=rtpmap:\d+\s+([A-Za-z0-9._+-]+)`)

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

func extractCodecsFromSDP(payload string) string {
	if payload == "" {
		return ""
	}
	lower := strings.ToLower(payload)
	if !strings.Contains(lower, "m=audio") && !strings.Contains(lower, "m=video") &&
		!strings.Contains(lower, "application/sdp") {
		return ""
	}
	seen := make(map[string]struct{})
	var out []string
	for _, m := range rtpmapCodecRe.FindAllStringSubmatch(payload, -1) {
		if len(m) < 2 {
			continue
		}
		name := strings.TrimSpace(m[1])
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, name)
	}
	return strings.Join(out, ", ")
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
	firstMs, lastMs int64

	RingingTime   int64
	Duration      int64 // answered leg: stop - connect (ms)
	SessionDur    int64 // last - first (ms)
	SRD           int64 // first response after INVITE (any 1xx-6xx)
	SSS           int64 // first 101-199 after INVITE
	FSSD          int64 // failed setup (INVITE error) delay
	SDD           int64 // 200 OK to BYE disconnect delay
	Status        int
	LastBadReply  int
	UAC, UAS      string
	SrcIP, SrcPort, DstIP, DstPort string
	Caller, Callee string
	MsgCount      int
	Codecs        string
	Methods       map[string]int
	FromParty     string
	RuriParty     string
}

func (m *sipCallLegMetrics) ingestSDP(payload string) {
	if c := extractCodecsFromSDP(payload); c != "" && m.Codecs == "" {
		m.Codecs = c
	}
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
	st := &sipCallLegMetrics{sessionID: sessionID, Methods: make(map[string]int)}
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
			st.ingestSDP(payload)
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
				st.ingestSDP(payload)
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

	out := map[string]interface{}{
		"session_id": st.sessionID,
		"caller":     nullIfEmpty(st.Caller),
		"callee":     nullIfEmpty(st.Callee),
		"uac":        nullIfEmpty(st.UAC),
		"uas":        nullIfEmpty(st.UAS),
		"codecs":     nullIfEmpty(st.Codecs),
		"status":     homerStatusText(st.Status),
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
