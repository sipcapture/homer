// Copyright (C) 2025 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package ducklake

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math/rand"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/sipcapture/homer-core/src/decoder"
)

// fastUUID generates UUID v4-like strings without syscalls.
// Uses a per-goroutine atomic counter combined with a random prefix
// seeded once at startup. Output format: 8-4-4-4-12 hex.
var (
	uuidCounter uint64
	uuidPrefix  [8]byte
	uuidOnce    sync.Once
)

func initUUIDPrefix() {
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	for i := range uuidPrefix {
		uuidPrefix[i] = byte(r.Intn(256))
	}
}

func fastUUID() string {
	uuidOnce.Do(initUUIDPrefix)
	seq := atomic.AddUint64(&uuidCounter, 1)

	var raw [16]byte
	copy(raw[:8], uuidPrefix[:8])
	binary.BigEndian.PutUint64(raw[8:], seq)
	// Set version 4 and variant bits for RFC 4122 compliance
	raw[6] = (raw[6] & 0x0f) | 0x40 // version 4
	raw[8] = (raw[8] & 0x3f) | 0x80 // variant 10

	var buf [36]byte
	hex.Encode(buf[0:8], raw[0:4])
	buf[8] = '-'
	hex.Encode(buf[9:13], raw[4:6])
	buf[13] = '-'
	hex.Encode(buf[14:18], raw[6:8])
	buf[18] = '-'
	hex.Encode(buf[19:23], raw[8:10])
	buf[23] = '-'
	hex.Encode(buf[24:36], raw[10:16])
	return string(buf[:])
}

// cachedDate caches the formatted date string, refreshing only when the day changes.
var (
	cachedDateStr  atomic.Value // stores string
	cachedDateDay  atomic.Int32 // day-of-year * 1000 + year%1000
)

func fastDate(ts time.Time) string {
	dayKey := int32(ts.YearDay()*1000 + ts.Year()%1000)
	if dayKey == cachedDateDay.Load() {
		if v := cachedDateStr.Load(); v != nil {
			return v.(string)
		}
	}
	s := ts.Format("2006-01-02")
	cachedDateStr.Store(s)
	cachedDateDay.Store(dayKey)
	return s
}

// nodeIDCache caches strconv.FormatUint results for NodeID values.
// Typically there are only 1-5 distinct NodeIDs.
var nodeIDCache sync.Map // uint32 -> string

func fastNodeID(id uint32) string {
	if v, ok := nodeIDCache.Load(id); ok {
		return v.(string)
	}
	s := strconv.FormatUint(uint64(id), 10)
	nodeIDCache.Store(id, s)
	return s
}

// HEPAdapter converts HEP packets to DuckLake records (legacy single-table)
type HEPAdapter struct {
	writer *Writer
}

// NewHEPAdapter creates a new HEP adapter (legacy single-table)
func NewHEPAdapter(writer *Writer) *HEPAdapter {
	return &HEPAdapter{writer: writer}
}

// WriteHEP converts and writes a HEP packet (legacy single-table)
func (a *HEPAdapter) WriteHEP(hep *decoder.HEP) error {
	record := a.convertHEP(hep)
	return a.writer.Write(record)
}

// MultiTableAdapter converts HEP packets to appropriate tables based on proto_type and SIP method
type MultiTableAdapter struct {
	writer *MultiTableWriter
}

// NewMultiTableAdapter creates a new multi-table HEP adapter
func NewMultiTableAdapter(writer *MultiTableWriter) *MultiTableAdapter {
	return &MultiTableAdapter{writer: writer}
}

// WriteHEP converts and writes a HEP packet to the appropriate table
func (a *MultiTableAdapter) WriteHEP(hep *decoder.HEP) error {
	key, values := a.convertHEPToValues(hep)
	return a.writer.WriteRecord(key, values)
}

// ShardedAdapter converts HEP packets and distributes writes across shards.
// It embeds a MultiTableAdapter for conversion logic and overrides WriteHEP
// to route through the ShardedWriter's round-robin distribution.
type ShardedAdapter struct {
	sw        *ShardedWriter
	converter *MultiTableAdapter // reuse conversion logic from primary shard
}

// NewShardedAdapter creates a new sharded HEP adapter
func NewShardedAdapter(sw *ShardedWriter) *ShardedAdapter {
	return &ShardedAdapter{
		sw:        sw,
		converter: NewMultiTableAdapter(sw.Primary()),
	}
}

// WriteHEP converts and writes a HEP packet via the sharded writer
func (a *ShardedAdapter) WriteHEP(hep *decoder.HEP) error {
	key, values := a.converter.convertHEPToValues(hep)
	return a.sw.WriteRecord(key, values)
}

// GetReader returns a sharded multi-table reader
func (a *ShardedAdapter) GetReader() *ShardedMultiTableReader {
	return NewShardedMultiTableReader(a.sw)
}

// getTableKey determines the TableKey for a HEP packet
func (a *MultiTableAdapter) getTableKey(hep *decoder.HEP) TableKey {
	key := TableKey{ProtoType: hep.ProtoType}

	// For SIP packets, determine sub-type based on method
	if hep.ProtoType == ProtoTypeSIP && hep.SIP != nil {
		// Get effective method: for responses use CSeq method, for requests use FirstMethod
		method := GetSIPMethod(hep.SIP.FirstMethod, hep.SIP.CseqMethod, hep.SIP.FirstResp)
		key.SubType = GetSIPType(method)
	}

	return key
}

// convertHEPToValues converts a HEP packet to TableKey and values array
func (a *MultiTableAdapter) convertHEPToValues(hep *decoder.HEP) (TableKey, []interface{}) {
	return a.convertHEPToValuesWithSIPSubtype(hep, "")
}

// convertHEPToValuesWithSIPSubtype converts a HEP packet to TableKey and values.
// If forcedSIPSubtype is non-empty and hep is SIP (proto 1), that subtype selects the
// DuckLake table and row shape (call / registration / default) instead of inferring from the method.
func (a *MultiTableAdapter) convertHEPToValuesWithSIPSubtype(hep *decoder.HEP, forcedSIPSubtype string) (TableKey, []interface{}) {
	// Calculate timestamp as time.Time for DuckDB TIMESTAMP type
	ts := time.Unix(int64(hep.Tsec), int64(hep.Tmsec)*1000)
	date := fastDate(ts)
	uid := fastUUID()
	nodeID := fastNodeID(hep.NodeID)

	var key TableKey
	if hep.ProtoType == ProtoTypeSIP && forcedSIPSubtype != "" {
		key = TableKey{ProtoType: ProtoTypeSIP, SubType: forcedSIPSubtype}
	} else {
		key = a.getTableKey(hep)
	}

	switch hep.ProtoType {
	case ProtoTypeSIP:
		return key, a.buildSIPValues(hep, uid, date, ts, nodeID, key.SubType)
	case ProtoTypeRTCPJSON, ProtoTypeRTCP, ProtoTypeRTP:
		return key, a.buildRTCPValues(hep, uid, date, ts, nodeID)
	case ProtoTypeDNS:
		return key, a.buildDNSValues(hep, uid, date, ts, nodeID)
	case ProtoTypeLOG:
		return key, a.buildLOGValues(hep, uid, date, ts, nodeID)
	default:
		return key, a.buildDefaultValues(hep, uid, date, ts, nodeID)
	}
}

// buildSIPValues builds values for SIP packets based on sub-type
func (a *MultiTableAdapter) buildSIPValues(hep *decoder.HEP, uid string, date string, ts time.Time, nodeID string, sipType string) []interface{} {
	switch sipType {
	case SIPTypeCall:
		return a.buildSIPCallValues(hep, uid, date, ts, nodeID)
	case SIPTypeRegistration:
		return a.buildSIPRegistrationValues(hep, uid, date, ts, nodeID)
	default:
		return a.buildSIPDefaultValues(hep, uid, date, ts, nodeID)
	}
}

// buildSIPCallValues builds values for SIP call messages (INVITE, BYE, etc.)
func (a *MultiTableAdapter) buildSIPCallValues(hep *decoder.HEP, uid string, date string, ts time.Time, nodeID string) []interface{} {
	var sessionID, caller, callee, method, responseCode, cseqMethod string

	if hep.SIP != nil {
		sessionID = hep.SIP.CallID
		caller = hep.SIP.FromUser
		callee = hep.SIP.ToUser
		method = hep.SIP.FirstMethod
		responseCode = hep.SIP.FirstResp
		cseqMethod = hep.SIP.CseqMethod
	}

	extraJSON := buildExtraJSON(hep)

	// Columns: uuid, date, timestamp, session_id, caller, callee, src_ip, dst_ip,
	//          src_port, dst_port, method, response_code, cseq_method,
	//          protocol, node_id, cid, payload, data_extra
	return []interface{}{
		uid, date, ts, sessionID, caller, callee,
		hep.SrcIP, hep.DstIP, hep.SrcPort, hep.DstPort,
		method, responseCode, cseqMethod,
		hep.Protocol, nodeID, hep.CID, hep.Payload, extraJSON,
	}
}

// buildSIPRegistrationValues builds values for REGISTER messages
func (a *MultiTableAdapter) buildSIPRegistrationValues(hep *decoder.HEP, uid string, date string, ts time.Time, nodeID string) []interface{} {
	var sessionID, aor, contact, expires, userAgent, method, responseCode string

	if hep.SIP != nil {
		sessionID = hep.SIP.CallID
		// AOR is typically the To URI for REGISTER
		aor = hep.SIP.ToUser + "@" + hep.SIP.ToHost
		contact = hep.SIP.ContactVal
		expires = hep.SIP.Expires
		userAgent = hep.SIP.UserAgent
		method = hep.SIP.FirstMethod
		responseCode = hep.SIP.FirstResp
	}

	extraJSON := buildExtraJSON(hep)

	// Columns: uuid, date, timestamp, session_id, aor, contact, expires, user_agent,
	//          src_ip, dst_ip, src_port, dst_port, method, response_code,
	//          protocol, node_id, payload, data_extra
	return []interface{}{
		uid, date, ts, sessionID, aor, contact, expires, userAgent,
		hep.SrcIP, hep.DstIP, hep.SrcPort, hep.DstPort,
		method, responseCode,
		hep.Protocol, nodeID, hep.Payload, extraJSON,
	}
}

// buildSIPDefaultValues builds values for other SIP messages (OPTIONS, NOTIFY, etc.)
func (a *MultiTableAdapter) buildSIPDefaultValues(hep *decoder.HEP, uid string, date string, ts time.Time, nodeID string) []interface{} {
	var sessionID, method, responseCode string

	if hep.SIP != nil {
		sessionID = hep.SIP.CallID
		method = hep.SIP.FirstMethod
		responseCode = hep.SIP.FirstResp
	}

	extraJSON := buildExtraJSON(hep)

	// Columns: uuid, date, timestamp, session_id, src_ip, dst_ip,
	//          src_port, dst_port, method, response_code,
	//          protocol, node_id, cid, payload, data_extra
	return []interface{}{
		uid, date, ts, sessionID,
		hep.SrcIP, hep.DstIP, hep.SrcPort, hep.DstPort,
		method, responseCode,
		hep.Protocol, nodeID, hep.CID, hep.Payload, extraJSON,
	}
}

// buildRTCPValues builds values for RTCP/RTP packets
func (a *MultiTableAdapter) buildRTCPValues(hep *decoder.HEP, uid string, date string, ts time.Time, nodeID string) []interface{} {
	sessionID := hep.CID

	// Columns: uuid, date, timestamp, session_id, src_ip, dst_ip,
	//          src_port, dst_port, protocol, node_id, cid, payload, data_extra
	return []interface{}{
		uid, date, ts, sessionID,
		hep.SrcIP, hep.DstIP, hep.SrcPort, hep.DstPort,
		hep.Protocol, nodeID, hep.CID, hep.Payload, buildSimpleExtraJSON(hep),
	}
}

// buildDNSValues builds values for DNS packets
func (a *MultiTableAdapter) buildDNSValues(hep *decoder.HEP, uid string, date string, ts time.Time, nodeID string) []interface{} {
	// Columns: uuid, date, timestamp, src_ip, dst_ip,
	//          src_port, dst_port, protocol, node_id, payload, data_extra
	return []interface{}{
		uid, date, ts,
		hep.SrcIP, hep.DstIP, hep.SrcPort, hep.DstPort,
		hep.Protocol, nodeID, hep.Payload, buildSimpleExtraJSON(hep),
	}
}

// buildLOGValues builds values for LOG packets
func (a *MultiTableAdapter) buildLOGValues(hep *decoder.HEP, uid string, date string, ts time.Time, nodeID string) []interface{} {
	sessionID := hep.CID

	// Columns: uuid, date, timestamp, session_id, src_ip, dst_ip, node_id, payload, data_extra
	return []interface{}{
		uid, date, ts, sessionID,
		hep.SrcIP, hep.DstIP, nodeID, hep.Payload, buildSimpleExtraJSON(hep),
	}
}

// buildDefaultValues builds values for unknown proto_types
func (a *MultiTableAdapter) buildDefaultValues(hep *decoder.HEP, uid string, date string, ts time.Time, nodeID string) []interface{} {
	sessionID := hep.CID

	// Columns: uuid, date, timestamp, session_id, src_ip, dst_ip,
	//          src_port, dst_port, protocol, node_id, cid, payload, data_extra
	return []interface{}{
		uid, date, ts, sessionID,
		hep.SrcIP, hep.DstIP, hep.SrcPort, hep.DstPort,
		hep.Protocol, nodeID, hep.CID, hep.Payload, buildSimpleExtraJSON(hep),
	}
}

// GetWriter returns the underlying writer
func (a *MultiTableAdapter) GetWriter() *MultiTableWriter {
	return a.writer
}

// GetReader returns a multi-table reader
func (a *MultiTableAdapter) GetReader() *MultiTableReader {
	return NewMultiTableReader(a.writer)
}

// convertHEP converts a HEP packet to a DuckLake record (legacy)
func (a *HEPAdapter) convertHEP(hep *decoder.HEP) HEPRecord {
	// Calculate timestamp as time.Time for DuckDB TIMESTAMP type
	ts := time.Unix(int64(hep.Tsec), int64(hep.Tmsec)*1000)

	record := HEPRecord{
		UUID:      fastUUID(),
		Timestamp: ts,
		SrcIP:     hep.SrcIP,
		DstIP:     hep.DstIP,
		SrcPort:   hep.SrcPort,
		DstPort:   hep.DstPort,
		ProtoType: hep.ProtoType,
		Protocol:  hep.Protocol,
		NodeID:    fastNodeID(hep.NodeID),
		CID:       hep.CID,
		Payload:   hep.Payload,
		DataExtra: "{}", // Default empty JSON object
	}

	// Extract SIP-specific fields
	if hep.SIP != nil {
		record.SessionID = hep.SIP.CallID
		record.Caller = hep.SIP.FromUser
		record.Callee = hep.SIP.ToUser
		record.Event = hep.SIP.FirstMethod
		record.DataExtra = buildExtraJSON(hep)
	} else {
		record.DataExtra = buildSimpleExtraJSON(hep)
	}

	return record
}

// jsonEscape escapes a string for safe embedding in JSON values.
func jsonEscape(s string) string {
	// Fast path: no special chars
	needsEscape := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '"' || c == '\\' || c < 0x20 {
			needsEscape = true
			break
		}
	}
	if !needsEscape {
		return s
	}
	var b strings.Builder
	b.Grow(len(s) + 8)
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch c {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			if c < 0x20 {
				fmt.Fprintf(&b, `\u%04x`, c)
			} else {
				b.WriteByte(c)
			}
		}
	}
	return b.String()
}

// appendJSONField appends ,"key":"value" to the builder (comma-separated).
// Returns true if a field was written (for comma tracking).
func appendJSONField(sb *strings.Builder, first *bool, key, value string) {
	if value == "" {
		return
	}
	if !*first {
		sb.WriteByte(',')
	}
	*first = false
	sb.WriteByte('"')
	sb.WriteString(key)
	sb.WriteString(`":"`)
	sb.WriteString(jsonEscape(value))
	sb.WriteByte('"')
}

var sbPool = sync.Pool{New: func() interface{} {
	b := new(strings.Builder)
	b.Grow(256)
	return b
}}

// buildExtraJSON builds the data_extra JSON string directly without
// map allocation or reflection-based encoding/json.Marshal.
func buildExtraJSON(hep *decoder.HEP) string {
	sb := sbPool.Get().(*strings.Builder)
	sb.Reset()
	sb.WriteByte('{')

	first := true
	sb.WriteString(`"version":`)
	sb.WriteString(strconv.FormatUint(uint64(hep.Version), 10))
	first = false

	if hep.SIP != nil {
		appendJSONField(sb, &first, "from_host", hep.SIP.FromHost)
		appendJSONField(sb, &first, "to_host", hep.SIP.ToHost)
		appendJSONField(sb, &first, "user_agent", hep.SIP.UserAgent)
		appendJSONField(sb, &first, "server", hep.SIP.Server)
		appendJSONField(sb, &first, "via", hep.SIP.ViaOne)
		appendJSONField(sb, &first, "contact", hep.SIP.ContactVal)
		appendJSONField(sb, &first, "authorization", hep.SIP.AuthVal)
		appendJSONField(sb, &first, "content_type", hep.SIP.ContentType)
		appendJSONField(sb, &first, "content_length", hep.SIP.ContentLength)
		appendJSONField(sb, &first, "cseq", hep.SIP.CseqVal)
		appendJSONField(sb, &first, "expires", hep.SIP.Expires)
		appendJSONField(sb, &first, "max_forwards", hep.SIP.MaxForwards)
		appendJSONField(sb, &first, "response_code", hep.SIP.FirstResp)
		appendJSONField(sb, &first, "response_reason", hep.SIP.FirstRespText)
		appendJSONField(sb, &first, "request_uri", hep.SIP.URIRaw)
		appendJSONField(sb, &first, "from_tag", hep.SIP.FromTag)
		appendJSONField(sb, &first, "to_tag", hep.SIP.ToTag)
		appendJSONField(sb, &first, "branch", hep.SIP.ViaOneBranch)
		appendJSONField(sb, &first, "x_call_id", hep.SIP.XCallID)

		if len(hep.SIP.CustomHeader) > 0 {
			if !first {
				sb.WriteByte(',')
			}
			first = false
			sb.WriteString(`"custom_headers":{`)
			cfirst := true
			for k, v := range hep.SIP.CustomHeader {
				if !cfirst {
					sb.WriteByte(',')
				}
				cfirst = false
				sb.WriteByte('"')
				sb.WriteString(jsonEscape(k))
				sb.WriteString(`":"`)
				sb.WriteString(jsonEscape(v))
				sb.WriteByte('"')
			}
			sb.WriteByte('}')
		}
	}

	sb.WriteByte('}')
	s := sb.String()
	sbPool.Put(sb)
	return s
}

// buildSimpleExtraJSON builds a minimal extra JSON for non-SIP packets.
func buildSimpleExtraJSON(hep *decoder.HEP) string {
	sb := sbPool.Get().(*strings.Builder)
	sb.Reset()
	sb.WriteString(`{"version":`)
	sb.WriteString(strconv.FormatUint(uint64(hep.Version), 10))
	sb.WriteString(`,"proto_type":`)
	sb.WriteString(strconv.FormatUint(uint64(hep.ProtoType), 10))
	sb.WriteByte('}')
	s := sb.String()
	sbPool.Put(sb)
	return s
}

// GetWriter returns the underlying writer
func (a *HEPAdapter) GetWriter() *Writer {
	return a.writer
}

// GetReader returns a reader for the writer
func (a *HEPAdapter) GetReader() *Reader {
	return NewReader(a.writer)
}
