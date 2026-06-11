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
	"encoding/json"
	"math/rand"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

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
	return unsafe.String(&buf[0], 36)
}

// cachedDuckDate caches the calendar DATE value (UTC midnight), refreshing when the day changes.
var (
	cachedDuckDate atomic.Value // stores time.Time
	cachedDateDay  atomic.Int32 // day-of-year * 1000 + year%1000
)

// fastDuckDate returns a time.Time at UTC midnight for ts's calendar date, for DuckDB DATE columns
// (DuckDB Appender rejects string values for DATE).
func fastDuckDate(ts time.Time) time.Time {
	dayKey := int32(ts.YearDay()*1000 + ts.Year()%1000)
	if dayKey == cachedDateDay.Load() {
		if v := cachedDuckDate.Load(); v != nil {
			return v.(time.Time)
		}
	}
	u := ts.UTC()
	d := time.Date(u.Year(), u.Month(), u.Day(), 0, 0, 0, 0, time.UTC)
	cachedDuckDate.Store(d)
	cachedDateDay.Store(dayKey)
	return d
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
	date := fastDuckDate(ts)
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
func (a *MultiTableAdapter) buildSIPValues(hep *decoder.HEP, uid string, date time.Time, ts time.Time, nodeID string, sipType string) []interface{} {
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
func (a *MultiTableAdapter) buildSIPCallValues(hep *decoder.HEP, uid string, date time.Time, ts time.Time, nodeID string) []interface{} {
	var sessionID, caller, callee, method, responseCode, cseqMethod string

	if hep.SIP != nil {
		sessionID = hep.SIP.CallID
		caller = hep.SIP.FromUser
		callee = hep.SIP.ToUser
		method = hep.SIP.FirstMethod
		responseCode = hep.SIP.FirstResp
		cseqMethod = hep.SIP.CseqMethod
	}

	extraJSON := buildExtraJSONCell(hep)

	// Columns: uuid, date, timestamp, session_id, caller, callee, src_ip, dst_ip,
	//          src_port, dst_port, method, response_code, cseq_method,
	//          protocol, node_id, cid, payload, data_extra
	row := getRowSlice(18)
	row[0] = uid
	row[1] = date
	row[2] = ts
	row[3] = sessionID
	row[4] = caller
	row[5] = callee
	row[6] = hep.SrcIP
	row[7] = hep.DstIP
	row[8] = hep.SrcPort
	row[9] = hep.DstPort
	row[10] = method
	row[11] = responseCode
	row[12] = cseqMethod
	row[13] = hep.Protocol
	row[14] = nodeID
	row[15] = hep.CID
	row[16] = hep.Payload
	row[17] = extraJSON
	return row
}

// buildSIPRegistrationValues builds values for REGISTER messages
func (a *MultiTableAdapter) buildSIPRegistrationValues(hep *decoder.HEP, uid string, date time.Time, ts time.Time, nodeID string) []interface{} {
	var sessionID, aor, contact, expires, userAgent, method, responseCode string

	if hep.SIP != nil {
		sessionID = hep.SIP.CallID
		aor = hep.SIP.ToUser + "@" + hep.SIP.ToHost
		contact = hep.SIP.ContactVal
		expires = hep.SIP.Expires
		userAgent = hep.SIP.UserAgent
		method = hep.SIP.FirstMethod
		responseCode = hep.SIP.FirstResp
	}

	extraJSON := buildExtraJSONCell(hep)

	// Columns: uuid, date, timestamp, session_id, aor, contact, expires, user_agent,
	//          src_ip, dst_ip, src_port, dst_port, method, response_code,
	//          protocol, node_id, payload, data_extra
	row := getRowSlice(18)
	row[0] = uid
	row[1] = date
	row[2] = ts
	row[3] = sessionID
	row[4] = aor
	row[5] = contact
	row[6] = expires
	row[7] = userAgent
	row[8] = hep.SrcIP
	row[9] = hep.DstIP
	row[10] = hep.SrcPort
	row[11] = hep.DstPort
	row[12] = method
	row[13] = responseCode
	row[14] = hep.Protocol
	row[15] = nodeID
	row[16] = hep.Payload
	row[17] = extraJSON
	return row
}

// buildSIPDefaultValues builds values for other SIP messages (OPTIONS, NOTIFY, etc.)
func (a *MultiTableAdapter) buildSIPDefaultValues(hep *decoder.HEP, uid string, date time.Time, ts time.Time, nodeID string) []interface{} {
	var sessionID, method, responseCode string

	if hep.SIP != nil {
		sessionID = hep.SIP.CallID
		method = hep.SIP.FirstMethod
		responseCode = hep.SIP.FirstResp
	}

	extraJSON := buildExtraJSONCell(hep)

	// Columns: uuid, date, timestamp, session_id, src_ip, dst_ip,
	//          src_port, dst_port, method, response_code,
	//          protocol, node_id, cid, payload, data_extra
	row := getRowSlice(15)
	row[0] = uid
	row[1] = date
	row[2] = ts
	row[3] = sessionID
	row[4] = hep.SrcIP
	row[5] = hep.DstIP
	row[6] = hep.SrcPort
	row[7] = hep.DstPort
	row[8] = method
	row[9] = responseCode
	row[10] = hep.Protocol
	row[11] = nodeID
	row[12] = hep.CID
	row[13] = hep.Payload
	row[14] = extraJSON
	return row
}

// buildRTCPValues builds values for RTCP/RTP packets
func (a *MultiTableAdapter) buildRTCPValues(hep *decoder.HEP, uid string, date time.Time, ts time.Time, nodeID string) []interface{} {
	sessionID := hep.CID
	// Columns: uuid, date, timestamp, session_id, src_ip, dst_ip,
	//          src_port, dst_port, protocol, node_id, cid, payload, data_extra
	row := getRowSlice(13)
	row[0] = uid
	row[1] = date
	row[2] = ts
	row[3] = sessionID
	row[4] = hep.SrcIP
	row[5] = hep.DstIP
	row[6] = hep.SrcPort
	row[7] = hep.DstPort
	row[8] = hep.Protocol
	row[9] = nodeID
	row[10] = hep.CID
	row[11] = hep.Payload
	row[12] = buildSimpleExtraJSONCell(hep)
	return row
}

// buildDNSValues builds values for DNS packets
func (a *MultiTableAdapter) buildDNSValues(hep *decoder.HEP, uid string, date time.Time, ts time.Time, nodeID string) []interface{} {
	// Columns: uuid, date, timestamp, src_ip, dst_ip,
	//          src_port, dst_port, protocol, node_id, payload, data_extra
	row := getRowSlice(11)
	row[0] = uid
	row[1] = date
	row[2] = ts
	row[3] = hep.SrcIP
	row[4] = hep.DstIP
	row[5] = hep.SrcPort
	row[6] = hep.DstPort
	row[7] = hep.Protocol
	row[8] = nodeID
	row[9] = hep.Payload
	row[10] = buildSimpleExtraJSONCell(hep)
	return row
}

// buildLOGValues builds values for LOG packets
func (a *MultiTableAdapter) buildLOGValues(hep *decoder.HEP, uid string, date time.Time, ts time.Time, nodeID string) []interface{} {
	sessionID := hep.CID
	// Columns: uuid, date, timestamp, session_id, src_ip, dst_ip, node_id, payload, data_extra
	row := getRowSlice(9)
	row[0] = uid
	row[1] = date
	row[2] = ts
	row[3] = sessionID
	row[4] = hep.SrcIP
	row[5] = hep.DstIP
	row[6] = nodeID
	row[7] = hep.Payload
	row[8] = buildSimpleExtraJSONCell(hep)
	return row
}

// buildDefaultValues builds values for unknown proto_types
func (a *MultiTableAdapter) buildDefaultValues(hep *decoder.HEP, uid string, date time.Time, ts time.Time, nodeID string) []interface{} {
	sessionID := hep.CID
	// Columns: uuid, date, timestamp, session_id, src_ip, dst_ip,
	//          src_port, dst_port, protocol, node_id, cid, payload, data_extra
	row := getRowSlice(13)
	row[0] = uid
	row[1] = date
	row[2] = ts
	row[3] = sessionID
	row[4] = hep.SrcIP
	row[5] = hep.DstIP
	row[6] = hep.SrcPort
	row[7] = hep.DstPort
	row[8] = hep.Protocol
	row[9] = nodeID
	row[10] = hep.CID
	row[11] = hep.Payload
	row[12] = buildSimpleExtraJSONCell(hep)
	return row
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

// writeJSONStringEscaped appends s JSON-string-escaped (contents only, no quotes)
// to b without allocating an intermediate string.
func writeJSONStringEscaped(b *[]byte, s string) {
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch c {
		case '"':
			*b = append(*b, '\\', '"')
		case '\\':
			*b = append(*b, '\\', '\\')
		case '\n':
			*b = append(*b, '\\', 'n')
		case '\r':
			*b = append(*b, '\\', 'r')
		case '\t':
			*b = append(*b, '\\', 't')
		default:
			if c < 0x20 {
				*b = append(*b, '\\', 'u', '0', '0',
					"0123456789abcdef"[c>>4],
					"0123456789abcdef"[c&0xf])
			} else {
				*b = append(*b, c)
			}
		}
	}
}

// appendJSONField appends ,"key":"value" to the buffer (comma-separated).
func appendJSONField(b *[]byte, first *bool, key, value string) {
	if value == "" {
		return
	}
	if !*first {
		*b = append(*b, ',')
	}
	*first = false
	*b = append(*b, '"')
	writeJSONStringEscaped(b, key)
	*b = append(*b, '"', ':', '"')
	writeJSONStringEscaped(b, value)
	*b = append(*b, '"')
}

var sbPool = sync.Pool{New: func() interface{} {
	b := make([]byte, 0, 384)
	return &b
}}

// sipExtraFields drives data_extra JSON for SIP without repeating appendJSONField calls.
var sipExtraFields = []struct {
	key string
	get func(*decoder.HEP) string
}{
	{"from_host", func(h *decoder.HEP) string { return h.SIP.FromHost }},
	{"to_host", func(h *decoder.HEP) string { return h.SIP.ToHost }},
	{"user_agent", func(h *decoder.HEP) string { return h.SIP.UserAgent }},
	{"server", func(h *decoder.HEP) string { return h.SIP.Server }},
	{"via", func(h *decoder.HEP) string { return h.SIP.ViaOne }},
	{"contact", func(h *decoder.HEP) string { return h.SIP.ContactVal }},
	{"authorization", func(h *decoder.HEP) string { return h.SIP.AuthVal }},
	{"content_type", func(h *decoder.HEP) string { return h.SIP.ContentType }},
	{"content_length", func(h *decoder.HEP) string { return h.SIP.ContentLength }},
	{"cseq", func(h *decoder.HEP) string { return h.SIP.CseqVal }},
	{"expires", func(h *decoder.HEP) string { return h.SIP.Expires }},
	{"max_forwards", func(h *decoder.HEP) string { return h.SIP.MaxForwards }},
	{"response_code", func(h *decoder.HEP) string { return h.SIP.FirstResp }},
	{"response_reason", func(h *decoder.HEP) string { return h.SIP.FirstRespText }},
	{"request_uri", func(h *decoder.HEP) string { return h.SIP.URIRaw }},
	{"from_tag", func(h *decoder.HEP) string { return h.SIP.FromTag }},
	{"to_tag", func(h *decoder.HEP) string { return h.SIP.ToTag }},
	{"branch", func(h *decoder.HEP) string { return h.SIP.ViaOneBranch }},
	{"x_call_id", func(h *decoder.HEP) string { return h.SIP.XCallID }},
}

func appendSIPExtraFields(b *[]byte, first *bool, hep *decoder.HEP) {
	if hep.SIP == nil {
		return
	}
	for _, f := range sipExtraFields {
		appendJSONField(b, first, f.key, f.get(hep))
	}
	if len(hep.SIP.CustomHeader) > 0 {
		if !*first {
			*b = append(*b, ',')
		}
		*b = append(*b, `"custom_headers":{`...)
		cfirst := true
		for k, v := range hep.SIP.CustomHeader {
			if !cfirst {
				*b = append(*b, ',')
			}
			cfirst = false
			*b = append(*b, '"')
			writeJSONStringEscaped(b, k)
			*b = append(*b, '"', ':', '"')
			writeJSONStringEscaped(b, v)
			*b = append(*b, '"')
		}
		*b = append(*b, '}')
	}
}

// sipVersionOnlyCache caches {"version":N} for SIP when no optional fields are set.
var sipVersionOnlyCache sync.Map // uint32 version -> json.RawMessage

func cachedSIPVersionOnlyJSON(version uint32) json.RawMessage {
	if v, ok := sipVersionOnlyCache.Load(version); ok {
		return v.(json.RawMessage)
	}
	bp := sbPool.Get().(*[]byte)
	b := (*bp)[:0]
	b = append(b, `{"version":`...)
	b = strconv.AppendUint(b, uint64(version), 10)
	b = append(b, '}')
	// The cached value lives indefinitely, so copy out of the pooled buffer.
	m := json.RawMessage(string(b))
	*bp = b
	sbPool.Put(bp)
	sipVersionOnlyCache.Store(version, m)
	return m
}

func buildSIPExtraJSONInto(b []byte, hep *decoder.HEP) []byte {
	b = append(b, '{')
	b = append(b, `"version":`...)
	b = strconv.AppendUint(b, uint64(hep.Version), 10)
	first := false
	appendSIPExtraFields(&b, &first, hep)
	b = append(b, '}')
	return b
}

// buildExtraJSONCell returns a value suitable for row data_extra column.
// Either a cached json.RawMessage or *([]byte) (pooled; released in
// putRowSlice). data_extra columns are JSON-typed, and the duckdb-go
// Appender json.Marshal()s whatever it receives — a plain Go string would
// be stored as a double-encoded JSON string scalar instead of an object
// (breaking json_extract_string and Lua call correlation), so JSON cells
// must always be json.RawMessage or pooled raw bytes here.
func buildExtraJSONCell(hep *decoder.HEP) interface{} {
	if hep.SIP == nil {
		return buildSimpleExtraJSONCell(hep)
	}
	bp := sbPool.Get().(*[]byte)
	b := buildSIPExtraJSONInto((*bp)[:0], hep)
	// Version-only SIP extras hit the string cache (no batch retention).
	if len(b) <= 16 && !bytesContainsComma(b) {
		sbPool.Put(bp)
		return cachedSIPVersionOnlyJSON(hep.Version)
	}
	*bp = b
	return bp
}

func bytesContainsComma(b []byte) bool {
	for _, c := range b {
		if c == ',' {
			return true
		}
	}
	return false
}

// releaseExtraJSONCell returns pooled data_extra buffers to sbPool.
func releaseExtraJSONCell(v interface{}) {
	if bp, ok := v.(*[]byte); ok && bp != nil {
		sbPool.Put(bp)
	}
}

// cellToDriverValue converts a batch cell to a driver value. Pooled JSON
// buffers become json.RawMessage: the upstream duckdb-go Appender runs
// json.Marshal on values destined for JSON columns, so a plain string would
// be re-encoded into a JSON string scalar ("{\"a\":1}" instead of {"a":1}),
// silently breaking every json_extract_* consumer (Lua call correlation,
// data_extra virtual-field search). json.RawMessage marshals to itself.
// The pooled buffer is only released after Appender.Close (putRowSlice),
// so aliasing it here is safe.
func cellToDriverValue(v interface{}) interface{} {
	if bp, ok := v.(*[]byte); ok {
		if bp == nil || len(*bp) == 0 {
			return emptyJSONObject
		}
		return json.RawMessage(*bp)
	}
	return v
}

// emptyJSONObject is the shared driver value for empty data_extra cells.
var emptyJSONObject = json.RawMessage("{}")

// buildExtraJSON builds the data_extra JSON string (legacy single-table path).
func buildExtraJSON(hep *decoder.HEP) string {
	cell := buildExtraJSONCell(hep)
	if m, ok := cell.(json.RawMessage); ok {
		return string(m)
	}
	bp := cell.(*[]byte)
	s := string(*bp)
	sbPool.Put(bp)
	return s
}

// simpleExtraCache caches {"version":N,"proto_type":M} for non-SIP packets.
// Keyed by version<<16 | protoType (both fit in 16 bits for known HEP protos).
var simpleExtraCache sync.Map // uint32 -> json.RawMessage

// buildSimpleExtraJSONCell builds a minimal extra JSON cell for non-SIP packets.
// Result is cached since version and proto_type rarely change between packets.
// Returned as json.RawMessage so the Appender writes it as a JSON document
// (see cellToDriverValue).
func buildSimpleExtraJSONCell(hep *decoder.HEP) json.RawMessage {
	key := uint32(hep.Version)<<16 | (hep.ProtoType & 0xffff)
	if v, ok := simpleExtraCache.Load(key); ok {
		return v.(json.RawMessage)
	}
	bp := sbPool.Get().(*[]byte)
	b := (*bp)[:0]
	b = append(b, `{"version":`...)
	b = strconv.AppendUint(b, uint64(hep.Version), 10)
	b = append(b, `,"proto_type":`...)
	b = strconv.AppendUint(b, uint64(hep.ProtoType), 10)
	b = append(b, '}')
	// The cached value lives indefinitely, so copy out of the pooled buffer.
	m := json.RawMessage(string(b))
	*bp = b
	sbPool.Put(bp)
	simpleExtraCache.Store(key, m)
	return m
}

// buildSimpleExtraJSON is the string variant for legacy single-table records.
func buildSimpleExtraJSON(hep *decoder.HEP) string {
	return string(buildSimpleExtraJSONCell(hep))
}

// GetWriter returns the underlying writer
func (a *HEPAdapter) GetWriter() *Writer {
	return a.writer
}

// GetReader returns a reader for the writer
func (a *HEPAdapter) GetReader() *Reader {
	return NewReader(a.writer)
}
