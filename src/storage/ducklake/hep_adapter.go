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

	extraJSON := buildExtraJSON(hep)

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

	extraJSON := buildExtraJSON(hep)

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

	extraJSON := buildExtraJSON(hep)

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
	row[12] = buildSimpleExtraJSON(hep)
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
	row[10] = buildSimpleExtraJSON(hep)
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
	row[8] = buildSimpleExtraJSON(hep)
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
	row[12] = buildSimpleExtraJSON(hep)
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
	b := make([]byte, 0, 256)
	return &b
}}

// buildExtraJSON builds the data_extra JSON string directly without
// map allocation or reflection-based encoding/json.Marshal.
// Uses a pooled []byte buffer; the result string borrows from it via unsafe
// so the caller must not hold the string after the pool slot is reclaimed.
// Since the string is passed to TableWriter.Write which appends it to a batch
// slice ([]interface{}), and the batch is flushed (copied into DuckDB Appender)
// before the pool slot can be reused, this is safe.
func buildExtraJSON(hep *decoder.HEP) string {
	bp := sbPool.Get().(*[]byte)
	b := (*bp)[:0]

	b = append(b, '{')
	b = append(b, `"version":`...)
	b = strconv.AppendUint(b, uint64(hep.Version), 10)
	first := false

	if hep.SIP != nil {
		appendJSONField(&b, &first, "from_host", hep.SIP.FromHost)
		appendJSONField(&b, &first, "to_host", hep.SIP.ToHost)
		appendJSONField(&b, &first, "user_agent", hep.SIP.UserAgent)
		appendJSONField(&b, &first, "server", hep.SIP.Server)
		appendJSONField(&b, &first, "via", hep.SIP.ViaOne)
		appendJSONField(&b, &first, "contact", hep.SIP.ContactVal)
		appendJSONField(&b, &first, "authorization", hep.SIP.AuthVal)
		appendJSONField(&b, &first, "content_type", hep.SIP.ContentType)
		appendJSONField(&b, &first, "content_length", hep.SIP.ContentLength)
		appendJSONField(&b, &first, "cseq", hep.SIP.CseqVal)
		appendJSONField(&b, &first, "expires", hep.SIP.Expires)
		appendJSONField(&b, &first, "max_forwards", hep.SIP.MaxForwards)
		appendJSONField(&b, &first, "response_code", hep.SIP.FirstResp)
		appendJSONField(&b, &first, "response_reason", hep.SIP.FirstRespText)
		appendJSONField(&b, &first, "request_uri", hep.SIP.URIRaw)
		appendJSONField(&b, &first, "from_tag", hep.SIP.FromTag)
		appendJSONField(&b, &first, "to_tag", hep.SIP.ToTag)
		appendJSONField(&b, &first, "branch", hep.SIP.ViaOneBranch)
		appendJSONField(&b, &first, "x_call_id", hep.SIP.XCallID)

		if len(hep.SIP.CustomHeader) > 0 {
			if !first {
				b = append(b, ',')
			}
			b = append(b, `"custom_headers":{`...)
			cfirst := true
			for k, v := range hep.SIP.CustomHeader {
				if !cfirst {
					b = append(b, ',')
				}
				cfirst = false
				b = append(b, '"')
				writeJSONStringEscaped(&b, k)
				b = append(b, '"', ':', '"')
				writeJSONStringEscaped(&b, v)
				b = append(b, '"')
			}
			b = append(b, '}')
		}
	}

	b = append(b, '}')
	// string(b) copies the bytes — necessary because the pool slot is returned
	// immediately below and could be reused by another goroutine before
	// the Appender consumes this row from tw.batch.
	s := string(b)
	*bp = b
	sbPool.Put(bp)
	return s
}

// simpleExtraCache caches "{\"version\":N,\"proto_type\":M}" strings for non-SIP packets.
// Keyed by version<<16 | protoType (both fit in 16 bits for known HEP protos).
var simpleExtraCache sync.Map // uint32 -> string

// buildSimpleExtraJSON builds a minimal extra JSON for non-SIP packets.
// Result is cached since version and proto_type rarely change between packets.
func buildSimpleExtraJSON(hep *decoder.HEP) string {
	key := uint32(hep.Version)<<16 | (hep.ProtoType & 0xffff)
	if v, ok := simpleExtraCache.Load(key); ok {
		return v.(string)
	}
	bp := sbPool.Get().(*[]byte)
	b := (*bp)[:0]
	b = append(b, `{"version":`...)
	b = strconv.AppendUint(b, uint64(hep.Version), 10)
	b = append(b, `,"proto_type":`...)
	b = strconv.AppendUint(b, uint64(hep.ProtoType), 10)
	b = append(b, '}')
	// For the cache we need a real heap string (lives indefinitely).
	s := string(b)
	*bp = b
	sbPool.Put(bp)
	simpleExtraCache.Store(key, s)
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
