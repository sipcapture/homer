// Copyright (C) 2026 Homer Server Contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package ducklake

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	logger "github.com/sipcapture/homer-core/src/utils/logging"
)

const vqrtcpTableName = "vqrtcpxr_stats"

const vqrtcpTableSQL = `
	uuid VARCHAR,
	date DATE,
	timestamp TIMESTAMP,
	create_ts BIGINT,
	callid VARCHAR,
	mos DOUBLE,
	event VARCHAR,
	source_ip VARCHAR,
	source_port UINTEGER,
	destination_ip VARCHAR,
	destination_port UINTEGER,
	vlan UINTEGER,
	capture_ip VARCHAR,
	proto UINTEGER,
	node VARCHAR,
	type VARCHAR,
	captid UINTEGER,
	data VARCHAR,
	message JSON
`

// VqrtcpRow is one parsed VQ-RTCPXR report ready for DuckLake insert.
type VqrtcpRow struct {
	CallID         string
	MosLQ          float64
	MosCQ          float64
	Event          string
	SourceIP       string
	SourcePort     uint16
	DestinationIP  string
	DestinationPort uint16
	Node           string
	SIPMethod      string
	RawData        string
	Message        map[string]any
	Timestamp      time.Time
}

// VqrtcpStorage owns the vqrtcpxr_stats table on the primary shard.
type VqrtcpStorage struct {
	db       *sql.DB
	lakeName string
	once     sync.Once
	ddlErr   error
}

// VqrtcpStorage returns the VQRTCP table accessor on the primary shard.
func (m *Manager) VqrtcpStorage() *VqrtcpStorage {
	if m == nil || m.sharded == nil {
		return nil
	}
	primary := m.sharded.Primary()
	if primary == nil {
		return nil
	}
	return &VqrtcpStorage{
		db:       primary.GetDB(),
		lakeName: primary.GetLakeName(),
	}
}

// LakeName returns the DuckLake catalog name.
func (s *VqrtcpStorage) LakeName() string {
	if s == nil {
		return ""
	}
	return s.lakeName
}

// TableFQN returns the fully qualified table name.
func (s *VqrtcpStorage) TableFQN() string {
	if s == nil || s.lakeName == "" {
		return vqrtcpTableName
	}
	return fmt.Sprintf("%s.%s", s.lakeName, vqrtcpTableName)
}

// EnsureVqrtcpSchema creates vqrtcpxr_stats if missing.
func (s *VqrtcpStorage) EnsureVqrtcpSchema(ctx context.Context) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("vqrtcp storage: nil database handle")
	}
	s.once.Do(func() {
		fqn := s.TableFQN()
		alreadyExisted := duckLakeTableExists(s.db, s.lakeName, vqrtcpTableName)
		if _, err := s.db.ExecContext(ctx,
			fmt.Sprintf("CREATE TABLE IF NOT EXISTS %s (%s);", fqn, vqrtcpTableSQL)); err != nil {
			s.ddlErr = fmt.Errorf("vqrtcp storage: %w", err)
			return
		}
		if !alreadyExisted {
			if _, err := s.db.ExecContext(ctx, fmt.Sprintf("ALTER TABLE %s SET PARTITIONED BY (date);", fqn)); err != nil {
				logger.Warn(fmt.Sprintf("vqrtcp storage: failed to set partitioning for %s: %v", fqn, err))
			}
			if _, err := s.db.ExecContext(ctx, fmt.Sprintf("ALTER TABLE %s SET SORTED BY (timestamp ASC);", fqn)); err != nil {
				logger.Warn(fmt.Sprintf("vqrtcp storage: failed to set sort order for %s: %v", fqn, err))
			}
		}
		logger.Info("VQRTCP storage schema ready", "lake", s.lakeName, "table", vqrtcpTableName)
	})
	return s.ddlErr
}

// WriteRow inserts one VQ-RTCPXR report.
func (s *VqrtcpStorage) WriteRow(ctx context.Context, row VqrtcpRow) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("vqrtcp storage: nil database handle")
	}
	ts := row.Timestamp.UTC()
	if ts.IsZero() {
		ts = time.Now().UTC()
	}
	mos := row.MosLQ
	if mos <= 0 && row.MosCQ > 0 {
		mos = row.MosCQ
	}
	msgJSON := "{}"
	if len(row.Message) > 0 {
		b, err := json.Marshal(row.Message)
		if err != nil {
			return fmt.Errorf("vqrtcp storage: marshal message: %w", err)
		}
		msgJSON = string(b)
	}
	id := uuid.New().String()
	createTS := ts.UnixMilli()
	sql := fmt.Sprintf(`INSERT INTO %s (
		uuid, date, timestamp, create_ts, callid, mos, event,
		source_ip, source_port, destination_ip, destination_port,
		vlan, capture_ip, proto, node, type, captid, data, message
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?::JSON)`,
		s.TableFQN())
	_, err := s.db.ExecContext(ctx, sql,
		id,
		ts.Format("2006-01-02"),
		ts,
		createTS,
		row.CallID,
		mos,
		row.Event,
		row.SourceIP,
		row.SourcePort,
		row.DestinationIP,
		row.DestinationPort,
		0,
		"",
		17,
		row.Node,
		row.SIPMethod,
		0,
		row.RawData,
		msgJSON,
	)
	return err
}
