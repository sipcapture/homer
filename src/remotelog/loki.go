// Copyright (C) 2025 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package remotelog

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/sipcapture/homer-core/src/decoder"
	logger "github.com/sipcapture/homer-core/src/utils/logging"
)

const (
	lokiContentType = "application/json"
	lokiPostPath    = "/loki/api/v1/push"
)

// LokiStream represents a Loki stream
type LokiStream struct {
	Stream map[string]string `json:"stream"`
	Values [][]string        `json:"values"`
}

// LokiPush represents a Loki push request
type LokiPush struct {
	Streams []LokiStream `json:"streams"`
}

// Loki is a remote logger for Grafana Loki
type Loki struct {
	config   *LokiConfig
	url      string
	client   *http.Client
	batch    map[string]*LokiStream
	batchMu  sync.Mutex
	inputCh  chan *decoder.HEP
	quit     chan bool
	hostname string
}

// NewLoki creates a new Loki client
func NewLoki(cfg *LokiConfig) (*Loki, error) {
	if !cfg.Enable {
		return nil, nil
	}

	// Build URL
	u, err := url.Parse(cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("invalid Loki URL: %w", err)
	}
	if !strings.Contains(u.Path, "/loki") {
		u.Path = lokiPostPath
	}

	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown"
	}

	bufSize := cfg.Buffer
	if bufSize <= 0 {
		bufSize = 100000
	}

	l := &Loki{
		config:   cfg,
		url:      u.String(),
		client:   &http.Client{Timeout: 10 * time.Second},
		batch:    make(map[string]*LokiStream),
		inputCh:  make(chan *decoder.HEP, bufSize),
		quit:     make(chan bool),
		hostname: hostname,
	}

	go l.run()
	logger.Info("Loki: Remote logging client started", "url", l.url)
	return l, nil
}

// Send queues a HEP packet for sending to Loki
func (l *Loki) Send(hep *decoder.HEP) error {
	if !ShouldProcess(hep.ProtoType, l.config.HEPFilter) {
		return nil
	}

	select {
	case l.inputCh <- hep:
		return nil
	default:
		return fmt.Errorf("Loki buffer full")
	}
}

// Close stops the Loki client
func (l *Loki) Close() error {
	close(l.quit)
	l.flushBatch()
	return nil
}

func (l *Loki) run() {
	batchSize := l.config.Bulk
	if batchSize <= 0 {
		batchSize = 400
	}
	batchBytes := batchSize * 1024

	timerSecs := l.config.Timer
	if timerSecs <= 0 {
		timerSecs = 4
	}
	maxWait := time.NewTimer(time.Duration(timerSecs) * time.Second)

	currentBatchSize := 0

	for {
		select {
		case <-l.quit:
			return

		case pkt, ok := <-l.inputCh:
			if !ok {
				return
			}

			// Build labels
			labels := map[string]string{
				"job":      "homer",
				"hostname": l.hostname,
				"node":     pkt.NodeName,
				"type":     pkt.ProtoString,
			}

			// SIP-specific labels
			if pkt.SIP != nil && pkt.ProtoType == 1 {
				labels["method"] = pkt.SIP.CseqMethod
				labels["response"] = pkt.SIP.FirstMethod
				if pkt.SIP.FirstResp != "" {
					labels["response"] = pkt.SIP.FirstResp
				}
				switch pkt.Protocol {
				case 6:
					labels["protocol"] = "tcp"
				case 17:
					labels["protocol"] = "udp"
				}
			}

			// IP/Port labels if enabled
			if l.config.IPPortLabels {
				labels["src_ip"] = pkt.SrcIP
				labels["src_port"] = strconv.FormatUint(uint64(pkt.SrcPort), 10)
				labels["dst_ip"] = pkt.DstIP
				labels["dst_port"] = strconv.FormatUint(uint64(pkt.DstPort), 10)
			}

			if len(pkt.CustomLokiLabels) > 0 {
				MergeCustomLokiLabelsIntoStream(labels, pkt.CustomLokiLabels)
			}

			// Build line content
			line := pkt.Payload

			// Add entry to batch
			l.batchMu.Lock()
			labelKey := labelsToKey(labels)
			stream, exists := l.batch[labelKey]
			if !exists {
				stream = &LokiStream{
					Stream: labels,
					Values: [][]string{},
				}
				l.batch[labelKey] = stream
			}

			// Loki expects [timestamp_ns, line]
			ts := strconv.FormatInt(pkt.Timestamp.UnixNano(), 10)
			stream.Values = append(stream.Values, []string{ts, line})
			currentBatchSize += len(line)
			l.batchMu.Unlock()

			// Check if batch is full
			if currentBatchSize >= batchBytes {
				if err := l.flushBatch(); err != nil {
					logger.Debug(fmt.Sprintf("Loki: flush error: %v", err))
				}
				currentBatchSize = 0
				maxWait.Reset(time.Duration(timerSecs) * time.Second)
			}

		case <-maxWait.C:
			if err := l.flushBatch(); err != nil {
				logger.Debug(fmt.Sprintf("Loki: timer flush error: %v", err))
			}
			currentBatchSize = 0
			maxWait.Reset(time.Duration(timerSecs) * time.Second)
		}
	}
}

func (l *Loki) flushBatch() error {
	l.batchMu.Lock()
	if len(l.batch) == 0 {
		l.batchMu.Unlock()
		return nil
	}

	// Build push request
	push := LokiPush{
		Streams: make([]LokiStream, 0, len(l.batch)),
	}
	for _, stream := range l.batch {
		push.Streams = append(push.Streams, *stream)
	}
	l.batch = make(map[string]*LokiStream)
	l.batchMu.Unlock()

	// Marshal to JSON
	body, err := json.Marshal(push)
	if err != nil {
		return fmt.Errorf("json marshal error: %w", err)
	}

	// Compress with gzip
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	if _, err := gw.Write(body); err != nil {
		return fmt.Errorf("gzip write error: %w", err)
	}
	if err := gw.Close(); err != nil {
		return fmt.Errorf("gzip close error: %w", err)
	}

	// Send request
	req, err := http.NewRequest("POST", l.url, &buf)
	if err != nil {
		return fmt.Errorf("request error: %w", err)
	}
	req.Header.Set("Content-Type", lokiContentType)
	req.Header.Set("Content-Encoding", "gzip")

	resp, err := l.client.Do(req)
	if err != nil {
		return fmt.Errorf("http error: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("loki error %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

func labelsToKey(labels map[string]string) string {
	var sb strings.Builder
	for k, v := range labels {
		sb.WriteString(k)
		sb.WriteString("=")
		sb.WriteString(v)
		sb.WriteString(",")
	}
	return sb.String()
}
