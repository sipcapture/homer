// Copyright (C) 2025 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package remotelog

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/sipcapture/homer-core/src/decoder"
	logger "github.com/sipcapture/homer-core/src/utils/logging"
)

// Elasticsearch is a remote logger for Elasticsearch
type Elasticsearch struct {
	config      *ElasticsearchConfig
	client      *http.Client
	inputCh     chan *decoder.HEP
	quit        chan bool
	currentDate string
	dateMu      sync.RWMutex
}

// ESDocument represents an Elasticsearch document
type ESDocument struct {
	Timestamp  string `json:"@timestamp"`
	ProtoType  uint32 `json:"proto_type"`
	ProtoName  string `json:"proto_name"`
	SrcIP      string `json:"src_ip"`
	SrcPort    uint32 `json:"src_port"`
	DstIP      string `json:"dst_ip"`
	DstPort    uint32 `json:"dst_port"`
	NodeID     uint32 `json:"node_id"`
	NodeName   string `json:"node_name"`
	CallID     string `json:"call_id,omitempty"`
	CID        string `json:"cid,omitempty"`
	Method     string `json:"method,omitempty"`
	Response   string `json:"response,omitempty"`
	FromUser   string `json:"from_user,omitempty"`
	ToUser     string `json:"to_user,omitempty"`
	FromHost   string `json:"from_domain,omitempty"`
	ToHost     string `json:"to_domain,omitempty"`
	UserAgent  string `json:"user_agent,omitempty"`
	Payload    string `json:"payload"`
	PayloadLen int    `json:"payload_len"`
}

// NewElasticsearch creates a new Elasticsearch client
func NewElasticsearch(cfg *ElasticsearchConfig) (*Elasticsearch, error) {
	if !cfg.Enable {
		return nil, nil
	}

	bufSize := 100000

	e := &Elasticsearch{
		config:      cfg,
		client:      &http.Client{Timeout: 30 * time.Second},
		inputCh:     make(chan *decoder.HEP, bufSize),
		quit:        make(chan bool),
		currentDate: time.Now().Format("2006.01.02"),
	}

	go e.run()
	logger.Info("Elasticsearch: Remote logging client started", "url", cfg.Addr)
	return e, nil
}

// Send queues a HEP packet for sending to Elasticsearch
func (e *Elasticsearch) Send(hep *decoder.HEP) error {
	if !ShouldProcess(hep.ProtoType, e.config.HEPFilter) {
		return nil
	}

	select {
	case e.inputCh <- hep:
		return nil
	default:
		return fmt.Errorf("Elasticsearch buffer full")
	}
}

// Close stops the Elasticsearch client
func (e *Elasticsearch) Close() error {
	close(e.quit)
	return nil
}

func (e *Elasticsearch) run() {
	bulkSize := 500
	bulkBuffer := make([]*decoder.HEP, 0, bulkSize)
	maxWait := time.NewTimer(5 * time.Second)

	for {
		select {
		case <-e.quit:
			if len(bulkBuffer) > 0 {
				e.sendBulk(bulkBuffer)
			}
			return

		case pkt, ok := <-e.inputCh:
			if !ok {
				return
			}

			bulkBuffer = append(bulkBuffer, pkt)
			if len(bulkBuffer) >= bulkSize {
				e.sendBulk(bulkBuffer)
				bulkBuffer = make([]*decoder.HEP, 0, bulkSize)
				maxWait.Reset(5 * time.Second)
			}

		case <-maxWait.C:
			if len(bulkBuffer) > 0 {
				e.sendBulk(bulkBuffer)
				bulkBuffer = make([]*decoder.HEP, 0, bulkSize)
			}
			maxWait.Reset(5 * time.Second)
		}
	}
}

func (e *Elasticsearch) sendBulk(packets []*decoder.HEP) error {
	if len(packets) == 0 {
		return nil
	}

	// Update date for daily index
	e.dateMu.Lock()
	e.currentDate = time.Now().Format("2006.01.02")
	e.dateMu.Unlock()

	var buf bytes.Buffer
	for _, pkt := range packets {
		doc := e.hepToDocument(pkt)

		// Build index name
		e.dateMu.RLock()
		indexName := e.config.IndexName
		if e.config.IndexDaily {
			indexName = fmt.Sprintf("%s-%s", e.config.IndexName, e.currentDate)
		}
		e.dateMu.RUnlock()

		// Bulk request format: {"index":{"_index":"name"}}\n{document}\n
		meta := map[string]map[string]string{
			"index": {"_index": indexName},
		}
		metaJSON, _ := json.Marshal(meta)
		docJSON, err := json.Marshal(doc)
		if err != nil {
			logger.Debug(fmt.Sprintf("Elasticsearch: marshal error: %v", err))
			continue
		}

		buf.Write(metaJSON)
		buf.WriteByte('\n')
		buf.Write(docJSON)
		buf.WriteByte('\n')
	}

	// Send bulk request
	url := strings.TrimSuffix(e.config.Addr, "/") + "/_bulk"
	req, err := http.NewRequest("POST", url, &buf)
	if err != nil {
		return fmt.Errorf("request error: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-ndjson")

	if e.config.User != "" && e.config.Pass != "" {
		req.SetBasicAuth(e.config.User, e.config.Pass)
	}

	resp, err := e.client.Do(req)
	if err != nil {
		return fmt.Errorf("http error: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("elasticsearch error %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

func (e *Elasticsearch) hepToDocument(hep *decoder.HEP) *ESDocument {
	doc := &ESDocument{
		Timestamp:  hep.Timestamp.Format(time.RFC3339Nano),
		ProtoType:  hep.ProtoType,
		ProtoName:  hep.ProtoString,
		SrcIP:      hep.SrcIP,
		SrcPort:    hep.SrcPort,
		DstIP:      hep.DstIP,
		DstPort:    hep.DstPort,
		NodeID:     hep.NodeID,
		NodeName:   hep.NodeName,
		CID:        hep.CID,
		Payload:    hep.Payload,
		PayloadLen: len(hep.Payload),
	}

	if hep.SIP != nil {
		doc.CallID = hep.SIP.CallID
		doc.Method = hep.SIP.FirstMethod
		doc.Response = hep.SIP.FirstResp
		doc.FromUser = hep.SIP.FromUser
		doc.ToUser = hep.SIP.ToUser
		doc.FromHost = hep.SIP.FromHost
		doc.ToHost = hep.SIP.ToHost
		doc.UserAgent = hep.SIP.UserAgent
	}

	return doc
}
