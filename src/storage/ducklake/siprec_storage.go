// Copyright (C) 2026 Homer Server Contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package ducklake

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// SiprecRow is one SIPREC signaling event for hep_proto_1_siprec.
type SiprecRow struct {
	SessionID    string
	Caller       string
	Callee       string
	SrcIP        string
	DstIP        string
	SrcPort      uint16
	DstPort      uint16
	Method       string
	ResponseCode string
	CseqMethod   string
	CID          string
	NodeID       string
	Payload      string
	DataExtra    map[string]any
	Timestamp    time.Time
}

// WriteSiprecRow inserts a row into hep_proto_1_siprec via the multi-table writer.
func (m *Manager) WriteSiprecRow(row SiprecRow) error {
	if m == nil || m.sharded == nil {
		return fmt.Errorf("siprec storage: manager unavailable")
	}
	writer := m.sharded.Primary()
	if writer == nil {
		return fmt.Errorf("siprec storage: primary shard unavailable")
	}
	ts := row.Timestamp.UTC()
	if ts.IsZero() {
		ts = time.Now().UTC()
	}
	extraJSON := "{}"
	if len(row.DataExtra) > 0 {
		b, err := json.Marshal(row.DataExtra)
		if err != nil {
			return fmt.Errorf("siprec storage: marshal data_extra: %w", err)
		}
		extraJSON = string(b)
	}
	values := []interface{}{
		uuid.New().String(),
		ts.Format("2006-01-02"),
		ts,
		row.SessionID,
		row.Caller,
		row.Callee,
		row.SrcIP,
		row.DstIP,
		row.SrcPort,
		row.DstPort,
		row.Method,
		row.ResponseCode,
		row.CseqMethod,
		uint32(17),
		row.NodeID,
		row.CID,
		row.Payload,
		extraJSON,
	}
	key := TableKey{ProtoType: ProtoTypeSIP, SubType: SIPTypeSiprec}
	return writer.WriteRecord(key, values)
}
