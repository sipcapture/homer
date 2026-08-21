// Copyright (C) 2026 Homer Server Contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package node

import (
	"strings"
	"testing"

	"github.com/sipcapture/homer-core/src/config"
)

func TestPrepareSQLRejectsDML(t *testing.T) {
	n := &Node{config: &config.NodeConfig{DuckLake: config.DuckLakeConfig{LakeName: "homer_lake"}}}
	s := newFsqlServer(n, config.FlightSQLServerConfig{}, "homer_lake")
	if _, err := s.prepareSQL("INSERT INTO t VALUES (1)"); err == nil {
		t.Fatal("expected INSERT to be rejected")
	}
	if _, err := s.prepareSQL("COPY t TO 'x.parquet'"); err == nil {
		t.Fatal("expected COPY to be rejected")
	}
	q, err := s.prepareSQL("SELECT 1 AS x")
	if err != nil {
		t.Fatalf("SELECT should pass: %v", err)
	}
	if !strings.Contains(strings.ToUpper(q), "SELECT") {
		t.Fatalf("unexpected rewritten SQL: %s", q)
	}
}
