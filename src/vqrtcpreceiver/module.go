// Copyright (C) 2026 Homer Server Contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package vqrtcpreceiver

import (
	"context"
	"fmt"
	"sync"

	"github.com/sipcapture/homer-core/src/config"
	"github.com/sipcapture/homer-core/src/storage/ducklake"
	logger "github.com/sipcapture/homer-core/src/utils/logging"
)

// Module is the Homer lifecycle wrapper for the VQRTCP SIP receiver.
type Module struct {
	cfg     *config.VqrtcpConfig
	storage *ducklake.VqrtcpStorage
	server  *Server
	cancel  context.CancelFunc
	wg      sync.WaitGroup
}

// New constructs the module. Returns (nil, nil) when disabled.
func New(cfg *config.VqrtcpConfig, storage *ducklake.VqrtcpStorage) (*Module, error) {
	if cfg == nil || !cfg.Enable {
		return nil, nil
	}
	if storage == nil {
		return nil, fmt.Errorf("vqrtcp receiver: storage is required")
	}
	srv, err := NewServer(cfg, storage, logger.GetLogger())
	if err != nil {
		return nil, err
	}
	return &Module{cfg: cfg, storage: storage, server: srv}, nil
}

// Start ensures schema and launches SIP listeners.
func (m *Module) Start() error {
	if m == nil {
		return nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	if err := m.storage.EnsureVqrtcpSchema(ctx); err != nil {
		cancel()
		return fmt.Errorf("vqrtcp receiver: schema: %w", err)
	}
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		if err := m.server.ListenAndServe(ctx); err != nil && ctx.Err() == nil {
			logger.Error(fmt.Sprintf("vqrtcp receiver: %v", err))
		}
	}()
	logger.Info("VQRTCP receiver started",
		"bind", m.cfg.BindIP,
		"port", m.cfg.SIPPort,
		"transports", m.cfg.Transports,
	)
	return nil
}

// Stop shuts down listeners and drains the write queue.
func (m *Module) Stop() error {
	if m == nil {
		return nil
	}
	if m.cancel != nil {
		m.cancel()
	}
	m.wg.Wait()
	if m.server != nil {
		m.server.Close()
	}
	logger.Info("VQRTCP receiver stopped")
	return nil
}
