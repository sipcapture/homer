// Copyright (C) 2026 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package cli

import (
	"context"
	"database/sql"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"sort"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// ---------------------------------------------------------------------------
// HEP3 packet reconstruction
//
// HEP3 framing:
//
//	0      4   "HEP3"  magic
//	4      6   uint16 total length (big-endian)
//	6..N      chunks: vendorID(2) | typeID(2) | length(2) | payload(length-6)
//
// We always set vendor=0x0000 (HEP standard chunks).
// ---------------------------------------------------------------------------

// hep3ProtoHeader carries the pieces of homer-app's protocol_header JSON
// that we need to rebuild a HEP3 packet.
type hep3ProtoHeader struct {
	ProtocolFamily int    `json:"protocolFamily"`
	Protocol       int    `json:"protocol"`
	SrcIP          string `json:"srcIp"`
	DstIP          string `json:"dstIp"`
	SrcPort        int    `json:"srcPort"`
	DstPort        int    `json:"dstPort"`
	TimeSec        int64  `json:"timeSeconds"`
	TimeUsec       int64  `json:"timeUseconds"`
	PayloadType    int    `json:"payloadType"`
	CaptureID      uint32 `json:"captureId"`
	CapturePass    string `json:"capturePass"`
	CorrelationID  string `json:"correlation_id"`
	NodeName       string `json:"node"`
	MOS            int    `json:"mos"`
}

// reconstructHEP3 rebuilds an HEP3 datagram from a homer-app row.
// Defaults are filled in when the source JSON is incomplete:
//   - family=2 (IPv4), proto=17 (UDP)
//   - timestamp: createDate fallback if header has no time
//   - capture id: forceCaptureID if non-zero, else header CaptureID, else 1
func reconstructHEP3(rawPayload, callID string, header hep3ProtoHeader, createDate time.Time, forceCaptureID uint32) ([]byte, error) {
	if rawPayload == "" {
		return nil, fmt.Errorf("empty raw payload")
	}

	family := byte(2) // IPv4
	if header.ProtocolFamily == 10 {
		family = 10
	}
	proto := byte(17) // UDP
	if header.Protocol > 0 && header.Protocol < 256 {
		proto = byte(header.Protocol)
	}

	srcIP := parseIPOrZero(header.SrcIP, family == 10)
	dstIP := parseIPOrZero(header.DstIP, family == 10)

	timeSec := header.TimeSec
	timeUsec := header.TimeUsec
	if timeSec == 0 && !createDate.IsZero() {
		timeSec = createDate.Unix()
		timeUsec = int64(createDate.Nanosecond() / 1000)
	}

	payloadType := byte(1) // SIP
	if header.PayloadType > 0 && header.PayloadType < 256 {
		payloadType = byte(header.PayloadType)
	}

	captureID := forceCaptureID
	if captureID == 0 {
		captureID = header.CaptureID
	}
	if captureID == 0 {
		captureID = 1
	}

	chunks := []hep3Chunk{}
	chunks = append(chunks,
		uint8Chunk(0x0001, family),
		uint8Chunk(0x0002, proto),
		ipChunk(0x0003, srcIP),
		ipChunk(0x0004, dstIP),
		uint16Chunk(0x0007, uint16(header.SrcPort)),
		uint16Chunk(0x0008, uint16(header.DstPort)),
		uint32Chunk(0x0009, uint32(timeSec)),
		uint32Chunk(0x000a, uint32(timeUsec)),
		uint8Chunk(0x000b, payloadType),
		uint32Chunk(0x000c, captureID),
	)

	if header.CapturePass != "" {
		chunks = append(chunks, bytesChunk(0x000e, []byte(header.CapturePass)))
	}
	chunks = append(chunks, bytesChunk(0x000f, []byte(rawPayload)))

	if id := strings.TrimSpace(header.CorrelationID); id != "" {
		chunks = append(chunks, bytesChunk(0x0011, []byte(id)))
	} else if cid := strings.TrimSpace(callID); cid != "" {
		chunks = append(chunks, bytesChunk(0x0011, []byte(cid)))
	}
	if header.NodeName != "" {
		chunks = append(chunks, bytesChunk(0x0013, []byte(header.NodeName)))
	}

	return assembleHEP3(chunks), nil
}

// hep3Chunk is one TLV inside an HEP3 packet (vendor 0x0000).
type hep3Chunk struct {
	typeID uint16
	body   []byte
}

func uint8Chunk(t uint16, v byte) hep3Chunk    { return hep3Chunk{typeID: t, body: []byte{v}} }
func uint16Chunk(t uint16, v uint16) hep3Chunk { b := []byte{0, 0}; binary.BigEndian.PutUint16(b, v); return hep3Chunk{typeID: t, body: b} }
func uint32Chunk(t uint16, v uint32) hep3Chunk { b := []byte{0, 0, 0, 0}; binary.BigEndian.PutUint32(b, v); return hep3Chunk{typeID: t, body: b} }
func bytesChunk(t uint16, body []byte) hep3Chunk {
	cp := make([]byte, len(body))
	copy(cp, body)
	return hep3Chunk{typeID: t, body: cp}
}
func ipChunk(t uint16, ip net.IP) hep3Chunk {
	if ip == nil {
		return hep3Chunk{typeID: t, body: []byte{0, 0, 0, 0}}
	}
	if v4 := ip.To4(); v4 != nil {
		return hep3Chunk{typeID: t, body: v4}
	}
	return hep3Chunk{typeID: t, body: ip.To16()}
}

func assembleHEP3(chunks []hep3Chunk) []byte {
	body := make([]byte, 0, 256)
	for _, c := range chunks {
		header := make([]byte, 6)
		binary.BigEndian.PutUint16(header[0:2], 0) // vendor 0
		binary.BigEndian.PutUint16(header[2:4], c.typeID)
		binary.BigEndian.PutUint16(header[4:6], uint16(6+len(c.body)))
		body = append(body, header...)
		body = append(body, c.body...)
	}
	out := make([]byte, 6+len(body))
	copy(out[0:4], []byte("HEP3"))
	binary.BigEndian.PutUint16(out[4:6], uint16(6+len(body)))
	copy(out[6:], body)
	return out
}

func parseIPOrZero(s string, ipv6 bool) net.IP {
	s = strings.TrimSpace(s)
	if s == "" {
		if ipv6 {
			return net.IPv6unspecified
		}
		return net.IPv4zero
	}
	if ip := net.ParseIP(s); ip != nil {
		return ip
	}
	if ipv6 {
		return net.IPv6unspecified
	}
	return net.IPv4zero
}

// ---------------------------------------------------------------------------
// runner
// ---------------------------------------------------------------------------

// hepCheckpoint records the highest row id we have replayed for each table.
type hepCheckpoint struct {
	Tables map[string]int64 `json:"tables"`
}

func loadCheckpoint(path string) (*hepCheckpoint, error) {
	cp := &hepCheckpoint{Tables: map[string]int64{}}
	if path == "" {
		return cp, nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cp, nil
		}
		return nil, err
	}
	if err := json.Unmarshal(b, cp); err != nil {
		return nil, fmt.Errorf("parse checkpoint: %w", err)
	}
	if cp.Tables == nil {
		cp.Tables = map[string]int64{}
	}
	return cp, nil
}

func saveCheckpoint(path string, cp *hepCheckpoint) error {
	if path == "" {
		return nil
	}
	b, err := json.MarshalIndent(cp, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// hepSink abstracts UDP/TCP send so we can unit-test reconstruction without
// opening sockets.
type hepSink interface {
	Send(packet []byte) error
	Close() error
}

type udpSink struct{ conn *net.UDPConn }

func (s *udpSink) Send(packet []byte) error { _, err := s.conn.Write(packet); return err }
func (s *udpSink) Close() error             { return s.conn.Close() }

type tcpSink struct{ conn net.Conn }

func (s *tcpSink) Send(packet []byte) error { _, err := s.conn.Write(packet); return err }
func (s *tcpSink) Close() error             { return s.conn.Close() }

func dialHEPSink(proto, target string) (hepSink, error) {
	switch strings.ToLower(proto) {
	case "udp":
		addr, err := net.ResolveUDPAddr("udp", target)
		if err != nil {
			return nil, err
		}
		c, err := net.DialUDP("udp", nil, addr)
		if err != nil {
			return nil, err
		}
		return &udpSink{conn: c}, nil
	case "tcp":
		c, err := net.Dial("tcp", target)
		if err != nil {
			return nil, err
		}
		return &tcpSink{conn: c}, nil
	default:
		return nil, fmt.Errorf("unsupported proto %q (expected udp or tcp)", proto)
	}
}

// RunMigrateHEP replays homer-app `hep_proto_*` rows as HEP3 packets to a
// homer-core node. The method preserves the original timestamp, capture
// metadata and correlation id, so post-migration data is indistinguishable
// from data originally captured by homer-core.
func RunMigrateHEP(f MigrateFlags) error {
	if f.HEPTarget == "" {
		return fmt.Errorf("--hep-target is required (host:port)")
	}
	tables := splitNonEmpty(f.Tables)
	if len(tables) == 0 {
		return fmt.Errorf("--tables is empty")
	}

	pg, err := sql.Open("pgx", f.PgDSN)
	if err != nil {
		return fmt.Errorf("open postgres: %w", err)
	}
	defer pg.Close()
	if err := pg.Ping(); err != nil {
		return fmt.Errorf("ping postgres: %w", err)
	}

	cpPath := f.CheckpointPath
	if cpPath == "" {
		cpPath = "homer7-migrate.checkpoint.json"
	}
	cp, err := loadCheckpoint(cpPath)
	if err != nil {
		return err
	}

	var sink hepSink
	if !f.DryRun {
		sink, err = dialHEPSink(f.HEPProto, f.HEPTarget)
		if err != nil {
			return fmt.Errorf("dial HEP sink: %w", err)
		}
		defer sink.Close()
	}

	totalSent := 0
	totalSkipped := 0
	startedAt := time.Now()
	pacer := newPacer(f.RatePPS)

	ctx := context.Background()
	for _, table := range tables {
		log.Printf("hep: replaying table %s (resume from id > %d)", table, cp.Tables[table])
		sent, skipped, err := replayTable(ctx, pg, sink, table, cp, f, pacer)
		totalSent += sent
		totalSkipped += skipped
		if err := saveCheckpoint(cpPath, cp); err != nil {
			log.Printf("hep: warn: save checkpoint: %v", err)
		}
		if err != nil {
			return fmt.Errorf("replay %s: %w", table, err)
		}
		if f.Limit > 0 && totalSent >= f.Limit {
			log.Printf("hep: reached --limit=%d; stopping", f.Limit)
			break
		}
	}

	verb := "sent"
	if f.DryRun {
		verb = "would send"
	}
	dur := time.Since(startedAt)
	pps := float64(totalSent) / dur.Seconds()
	log.Printf("=== hep replay summary ===  %s=%d  skipped=%d  duration=%s  rate=%.0f pps",
		verb, totalSent, totalSkipped, dur.Truncate(time.Second), pps)
	return nil
}

// replayTable streams a single hep_proto_* table in chronological order,
// reconstructs HEP3 packets and forwards them to the sink. Returns
// (sent, skipped, err).
func replayTable(ctx context.Context, pg *sql.DB, sink hepSink, table string, cp *hepCheckpoint, f MigrateFlags, pacer *pacer) (int, int, error) {
	if !isSafeTableName(table) {
		return 0, 0, fmt.Errorf("unsafe table name %q", table)
	}
	ok, err := pgTableExists(ctx, pg, table)
	if err != nil {
		return 0, 0, err
	}
	if !ok {
		log.Printf("hep: %s not found in source; skipped", table)
		return 0, 0, nil
	}

	var (
		sent, skipped int
		lastID        = cp.Tables[table]
	)

	since, until, err := parseTimeWindow(f.Since, f.Until)
	if err != nil {
		return 0, 0, err
	}

	for {
		rows, err := selectHEPBatch(ctx, pg, table, lastID, since, until, f.BatchSize)
		if err != nil {
			return sent, skipped, err
		}
		count := 0
		for rows.Next() {
			var (
				id           int64
				sid          sql.NullString
				createDate   time.Time
				protoHeader  string
				rawPayload   sql.NullString
			)
			if err := rows.Scan(&id, &sid, &createDate, &protoHeader, &rawPayload); err != nil {
				rows.Close()
				return sent, skipped, err
			}
			count++
			lastID = id
			if !rawPayload.Valid || rawPayload.String == "" {
				skipped++
				continue
			}
			var hdr hep3ProtoHeader
			if protoHeader != "" {
				_ = json.Unmarshal([]byte(protoHeader), &hdr)
			}
			pkt, err := reconstructHEP3(rawPayload.String, sid.String, hdr, createDate, f.HEPCaptureID)
			if err != nil {
				skipped++
				continue
			}
			if !f.DryRun {
				pacer.wait()
				if err := sink.Send(pkt); err != nil {
					rows.Close()
					return sent, skipped, fmt.Errorf("send packet (id=%d): %w", id, err)
				}
			}
			sent++
			if f.Verbose && sent%1000 == 0 {
				log.Printf("hep: %s sent=%d last_id=%d", table, sent, lastID)
			}
			if f.Limit > 0 && sent >= f.Limit {
				rows.Close()
				cp.Tables[table] = lastID
				return sent, skipped, nil
			}
		}
		rows.Close()
		cp.Tables[table] = lastID
		if count == 0 {
			break
		}
	}
	return sent, skipped, nil
}

// selectHEPBatch fetches one batch from a hep_proto_* table.
// homer-app v7 columns: id BIGINT PRIMARY KEY, sid VARCHAR, create_date
// TIMESTAMP, protocol_header JSONB, data_header JSONB, raw TEXT.
func selectHEPBatch(ctx context.Context, pg *sql.DB, table string, afterID int64, since, until time.Time, limit int) (*sql.Rows, error) {
	var (
		conds []string
		args  []any
	)
	args = append(args, afterID)
	conds = append(conds, fmt.Sprintf("id > $%d", len(args)))
	if !since.IsZero() {
		args = append(args, since)
		conds = append(conds, fmt.Sprintf("create_date >= $%d", len(args)))
	}
	if !until.IsZero() {
		args = append(args, until)
		conds = append(conds, fmt.Sprintf("create_date < $%d", len(args)))
	}
	args = append(args, limit)
	q := fmt.Sprintf(`
		SELECT id,
		       sid,
		       create_date,
		       COALESCE(protocol_header::text, '{}') AS protocol_header,
		       raw
		FROM %s
		WHERE %s
		ORDER BY id ASC
		LIMIT $%d`, table, strings.Join(conds, " AND "), len(args))
	return pg.QueryContext(ctx, q, args...)
}

func parseTimeWindow(since, until string) (time.Time, time.Time, error) {
	var s, u time.Time
	var err error
	if since != "" {
		s, err = time.Parse(time.RFC3339, since)
		if err != nil {
			return s, u, fmt.Errorf("--since: %w", err)
		}
	}
	if until != "" {
		u, err = time.Parse(time.RFC3339, until)
		if err != nil {
			return s, u, fmt.Errorf("--until: %w", err)
		}
	}
	return s, u, nil
}

// isSafeTableName guards against SQL injection via --tables. Allows
// alphanumeric characters and underscore only.
func isSafeTableName(name string) bool {
	if name == "" {
		return false
	}
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '_':
		default:
			return false
		}
	}
	return true
}

func splitNonEmpty(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	sort.Strings(out)
	return out
}

// pacer limits the rate of HEP packet sends. rateZero (0) means unlimited.
type pacer struct {
	rate     int
	interval time.Duration
	last     time.Time
}

func newPacer(rate int) *pacer {
	if rate <= 0 {
		return &pacer{rate: 0}
	}
	return &pacer{rate: rate, interval: time.Second / time.Duration(rate)}
}

func (p *pacer) wait() {
	if p.rate == 0 {
		return
	}
	now := time.Now()
	next := p.last.Add(p.interval)
	if now.Before(next) {
		time.Sleep(next.Sub(now))
		p.last = next
	} else {
		p.last = now
	}
}
