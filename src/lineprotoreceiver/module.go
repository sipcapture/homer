// Copyright (C) 2026 Homer Server Contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package lineprotoreceiver

import (
	"database/sql"
	"fmt"

	"github.com/sipcapture/homer-core/src/config"
	logger "github.com/sipcapture/homer-core/src/utils/logging"
)

// Module is the lifecycle wrapper plugged into ModuleManager. It owns
// the HTTP listener and the shared Ingester. The receiver writes to
// the writer's primary DuckLake shard via a *sql.DB borrowed at
// construction time.
type Module struct {
	cfg *config.LineProtoConfig
	ing *Ingester

	http *httpServer
}

// New constructs a Line Protocol receiver module. Returns (nil, nil)
// when the module is disabled — main.go can skip the AddModule call
// without branching on the err value.
//
// db is the writer's primary DuckLake *sql.DB; lakeName is the catalog
// identifier used to qualify table names.
func New(cfg *config.LineProtoConfig, db *sql.DB, lakeName string) (*Module, error) {
	if cfg == nil || !cfg.Enable {
		return nil, nil
	}
	if db == nil {
		return nil, fmt.Errorf("line-proto receiver: db is required")
	}
	ing := NewIngester(db, lakeName, cfg.TablePrefix)
	hs, err := newHTTPServer(cfg, ing)
	if err != nil {
		return nil, fmt.Errorf("line-proto http: %w", err)
	}
	return &Module{cfg: cfg, ing: ing, http: hs}, nil
}

// Start launches the HTTP listener.
func (m *Module) Start() error {
	if m == nil {
		return nil
	}
	if err := m.http.Start(); err != nil {
		return fmt.Errorf("line-proto http start: %w", err)
	}
	logger.Info("Line Protocol receiver started", "listen", m.cfg.Listen)
	return nil
}

// Stop drains the HTTP listener with a small grace period.
func (m *Module) Stop() error {
	if m == nil {
		return nil
	}
	if err := m.http.Stop(); err != nil {
		logger.Warn(fmt.Sprintf("line-proto http stop: %v", err))
	}
	logger.Info("Line Protocol receiver stopped")
	return nil
}
