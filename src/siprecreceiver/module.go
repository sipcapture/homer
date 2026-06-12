// Copyright (C) 2026 Homer Server Contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package siprecreceiver

import (
	"context"
	"fmt"
	"sync"

	"github.com/sipcapture/homer-core/src/config"
	"github.com/sipcapture/homer-core/src/storage/ducklake"
	logger "github.com/sipcapture/homer-core/src/utils/logging"
)

// Module wraps the SIPREC signaling server for Homer lifecycle management.
type Module struct {
	cfg     *config.SiprecConfig
	storage *ducklake.Manager
	server  *Server
	cancel  context.CancelFunc
	wg      sync.WaitGroup
}

// New returns a module or (nil, nil) when disabled.
func New(cfg *config.SiprecConfig, storage *ducklake.Manager) (*Module, error) {
	if cfg == nil || !cfg.Enable {
		return nil, nil
	}
	if storage == nil {
		return nil, fmt.Errorf("siprec receiver: storage is required")
	}
	srv, err := NewServer(cfg, storage, logger.GetLogger())
	if err != nil {
		return nil, err
	}
	return &Module{cfg: cfg, storage: storage, server: srv}, nil
}

// Start launches SIP listeners.
func (m *Module) Start() error {
	if m == nil {
		return nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		if err := m.server.ListenAndServe(ctx); err != nil && ctx.Err() == nil {
			logger.Error(fmt.Sprintf("siprec receiver: %v", err))
		}
	}()
	logger.Info("SIPREC receiver started",
		"bind", m.cfg.BindIP,
		"port", m.cfg.SIPPort,
		"advertise", m.cfg.AdvertiseIP,
	)
	return nil
}

// Stop shuts down the SIPREC server.
func (m *Module) Stop() error {
	if m == nil {
		return nil
	}
	if m.cancel != nil {
		m.cancel()
	}
	m.wg.Wait()
	if m.server != nil {
		_ = m.server.Close()
	}
	logger.Info("SIPREC receiver stopped")
	return nil
}
