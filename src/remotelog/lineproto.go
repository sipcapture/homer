// Copyright (C) 2025 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package remotelog

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/sipcapture/homer-core/src/decoder"
	logger "github.com/sipcapture/homer-core/src/utils/logging"
)

// LineProto is a remote logger for InfluxDB Line Protocol
type LineProto struct {
	config  *LineProtoConfig
	client  *http.Client
	inputCh chan *decoder.HEP
	quit    chan bool
}

// NewLineProto creates a new Line Protocol client
func NewLineProto(cfg *LineProtoConfig) (*LineProto, error) {
	if !cfg.Enable {
		return nil, nil
	}

	bufSize := cfg.Buffer
	if bufSize <= 0 {
		bufSize = 100000
	}

	l := &LineProto{
		config:  cfg,
		client:  &http.Client{Timeout: 10 * time.Second},
		inputCh: make(chan *decoder.HEP, bufSize),
		quit:    make(chan bool),
	}

	go l.run()
	logger.Info("LineProto: Remote logging client started", "url", cfg.URL)
	return l, nil
}

// Send queues a HEP packet for sending
func (l *LineProto) Send(hep *decoder.HEP) error {
	if !ShouldProcess(hep.ProtoType, l.config.HEPFilter) {
		return nil
	}

	select {
	case l.inputCh <- hep:
		return nil
	default:
		return fmt.Errorf("LineProto buffer full")
	}
}

// Close stops the LineProto client
func (l *LineProto) Close() error {
	close(l.quit)
	return nil
}

func (l *LineProto) run() {
	bulkSize := l.config.Bulk
	if bulkSize <= 0 {
		bulkSize = 400
	}

	timerSecs := l.config.Timer
	if timerSecs <= 0 {
		timerSecs = 4
	}

	var buf bytes.Buffer
	lineCount := 0
	maxWait := time.NewTimer(time.Duration(timerSecs) * time.Second)

	for {
		select {
		case <-l.quit:
			if buf.Len() > 0 {
				l.sendBatch(buf.Bytes())
			}
			return

		case pkt, ok := <-l.inputCh:
			if !ok {
				return
			}

			line := l.hepToLineProto(pkt)
			buf.WriteString(line)
			buf.WriteByte('\n')
			lineCount++

			if lineCount >= bulkSize {
				if err := l.sendBatch(buf.Bytes()); err != nil {
					logger.Debug(fmt.Sprintf("LineProto: send error: %v", err))
				}
				buf.Reset()
				lineCount = 0
				maxWait.Reset(time.Duration(timerSecs) * time.Second)
			}

		case <-maxWait.C:
			if buf.Len() > 0 {
				if err := l.sendBatch(buf.Bytes()); err != nil {
					logger.Debug(fmt.Sprintf("LineProto: timer send error: %v", err))
				}
				buf.Reset()
				lineCount = 0
			}
			maxWait.Reset(time.Duration(timerSecs) * time.Second)
		}
	}
}

func (l *LineProto) sendBatch(data []byte) error {
	req, err := http.NewRequest("POST", l.config.URL, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("request error: %w", err)
	}
	req.Header.Set("Content-Type", "text/plain")

	resp, err := l.client.Do(req)
	if err != nil {
		return fmt.Errorf("http error: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("lineproto error %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// hepToLineProto converts HEP packet to InfluxDB Line Protocol format
// Format: measurement,tag1=value1,tag2=value2 field1=value1,field2=value2 timestamp
func (l *LineProto) hepToLineProto(hep *decoder.HEP) string {
	var sb strings.Builder

	// Measurement name
	sb.WriteString("hep")

	// Tags (indexed, low cardinality)
	sb.WriteString(",proto_type=")
	sb.WriteString(fmt.Sprintf("%d", hep.ProtoType))

	sb.WriteString(",node_id=")
	sb.WriteString(fmt.Sprintf("%d", hep.NodeID))

	if hep.NodeName != "" {
		sb.WriteString(",node_name=")
		sb.WriteString(escapeTag(hep.NodeName))
	}

	if hep.ProtoString != "" {
		sb.WriteString(",proto_name=")
		sb.WriteString(escapeTag(hep.ProtoString))
	}

	// SIP-specific tags
	if hep.SIP != nil {
		if hep.SIP.CseqMethod != "" {
			sb.WriteString(",method=")
			sb.WriteString(escapeTag(hep.SIP.CseqMethod))
		}
		if hep.SIP.FirstResp != "" {
			sb.WriteString(",response=")
			sb.WriteString(escapeTag(hep.SIP.FirstResp))
		}
	}

	// Fields (not indexed)
	sb.WriteString(" ")

	sb.WriteString("src_ip=\"")
	sb.WriteString(escapeField(hep.SrcIP))
	sb.WriteString("\"")

	sb.WriteString(",src_port=")
	sb.WriteString(fmt.Sprintf("%di", hep.SrcPort))

	sb.WriteString(",dst_ip=\"")
	sb.WriteString(escapeField(hep.DstIP))
	sb.WriteString("\"")

	sb.WriteString(",dst_port=")
	sb.WriteString(fmt.Sprintf("%di", hep.DstPort))

	sb.WriteString(",payload_len=")
	sb.WriteString(fmt.Sprintf("%di", len(hep.Payload)))

	if hep.CID != "" {
		sb.WriteString(",cid=\"")
		sb.WriteString(escapeField(hep.CID))
		sb.WriteString("\"")
	}

	if hep.SIP != nil {
		if hep.SIP.CallID != "" {
			sb.WriteString(",call_id=\"")
			sb.WriteString(escapeField(hep.SIP.CallID))
			sb.WriteString("\"")
		}
		if hep.SIP.FromUser != "" {
			sb.WriteString(",from_user=\"")
			sb.WriteString(escapeField(hep.SIP.FromUser))
			sb.WriteString("\"")
		}
		if hep.SIP.ToUser != "" {
			sb.WriteString(",to_user=\"")
			sb.WriteString(escapeField(hep.SIP.ToUser))
			sb.WriteString("\"")
		}
	}

	// Timestamp in nanoseconds
	sb.WriteString(" ")
	sb.WriteString(fmt.Sprintf("%d", hep.Timestamp.UnixNano()))

	return sb.String()
}

// escapeTag escapes special characters in tag values
func escapeTag(s string) string {
	s = strings.ReplaceAll(s, ",", "\\,")
	s = strings.ReplaceAll(s, "=", "\\=")
	s = strings.ReplaceAll(s, " ", "\\ ")
	return s
}

// escapeField escapes special characters in field string values
func escapeField(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "\"", "\\\"")
	return s
}
