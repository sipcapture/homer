// Copyright (C) 2025 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

// Package otlpreceiver implements an OpenTelemetry Protocol receiver
// for traces, metrics and logs. It speaks all three OTLP transports:
//
//   - gRPC (default :4317)
//   - HTTP/protobuf (default :4318, /v1/{traces,metrics,logs})
//   - HTTP/JSON     (default :4318, /v1/{traces,metrics,logs})
//
// Decoded payloads are handed to a sink.Sink (typically the DuckLake
// sink which writes to dedicated otlp_traces / otlp_metrics / otlp_logs
// tables on the primary shard).
package otlpreceiver

import (
	"context"
	"fmt"
	"time"

	"github.com/sipcapture/homer-core/src/config"
	"github.com/sipcapture/homer-core/src/otlpreceiver/sink"
	logger "github.com/sipcapture/homer-core/src/utils/logging"
)

// Module is the lifecycle wrapper plugged into ModuleManager. It owns
// both the gRPC and HTTP listeners; each is conditionally created by
// New based on cfg.GRPC.Enable / cfg.HTTP.Enable.
type Module struct {
	cfg  *config.OTLPConfig
	sink sink.Sink

	grpc *grpcServer
	http *httpServer

	// schemaPrep is invoked from Start before either listener begins
	// accepting traffic. Hook for the DuckLake sink to issue its
	// CREATE TABLE DDL on the primary shard.
	schemaPrep func(context.Context) error

	// asyncDrain, when non-nil, is called after gRPC/HTTP listeners stop
	// to flush the optional async OTLP queue (bounded channel + worker).
	asyncDrain func(context.Context) error
}

// New constructs an OTLP receiver module. Returns (nil, nil) when the
// module is disabled — main.go can skip the AddModule call without
// branching on the err value.
//
// asyncDrain is optional: when the sink uses an in-process async queue,
// pass a function that drains pending writes (e.g. (*sink.AsyncQueue).Drain);
// otherwise pass nil.
func New(cfg *config.OTLPConfig, s sink.Sink, schemaPrep func(context.Context) error, asyncDrain func(context.Context) error) (*Module, error) {
	if cfg == nil || !cfg.Enable {
		return nil, nil
	}
	if s == nil {
		return nil, fmt.Errorf("otlp receiver: sink is required")
	}
	m := &Module{
		cfg:        cfg,
		sink:       s,
		schemaPrep: schemaPrep,
		asyncDrain: asyncDrain,
	}
	if cfg.GRPC.Enable {
		g, err := newGRPCServer(cfg, s)
		if err != nil {
			return nil, fmt.Errorf("otlp grpc: %w", err)
		}
		m.grpc = g
	}
	if cfg.HTTP.Enable {
		h, err := newHTTPServer(cfg, s)
		if err != nil {
			return nil, fmt.Errorf("otlp http: %w", err)
		}
		m.http = h
	}
	if m.grpc == nil && m.http == nil {
		return nil, fmt.Errorf("otlp receiver: both grpc and http are disabled")
	}
	return m, nil
}

// Start runs schemaPrep (best-effort), then launches the configured
// listeners in their own goroutines. Returns immediately on success;
// listener errors are surfaced via the logs.
func (m *Module) Start() error {
	if m == nil {
		return nil
	}
	if m.schemaPrep != nil {
		ctx, cancel := context.WithCancel(context.Background())
		err := m.schemaPrep(ctx)
		cancel()
		if err != nil {
			return fmt.Errorf("otlp receiver: schema prep: %w", err)
		}
	}
	if m.grpc != nil {
		if err := m.grpc.Start(); err != nil {
			return fmt.Errorf("otlp grpc start: %w", err)
		}
	}
	if m.http != nil {
		if err := m.http.Start(); err != nil {
			return fmt.Errorf("otlp http start: %w", err)
		}
	}
	logger.Info("OTLP receiver started",
		"grpc_listen", grpcListenForLog(m),
		"http_listen", httpListenForLog(m),
	)
	return nil
}

// Stop shuts both listeners down with a small grace period. Errors are
// logged but do not abort the shutdown of the sibling listener.
func (m *Module) Stop() error {
	if m == nil {
		return nil
	}
	if m.grpc != nil {
		if err := m.grpc.Stop(); err != nil {
			logger.Warn(fmt.Sprintf("OTLP grpc stop: %v", err))
		}
	}
	if m.http != nil {
		if err := m.http.Stop(); err != nil {
			logger.Warn(fmt.Sprintf("OTLP http stop: %v", err))
		}
	}
	if m.asyncDrain != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		err := m.asyncDrain(ctx)
		cancel()
		if err != nil {
			logger.Warn(fmt.Sprintf("OTLP async drain: %v", err))
		}
	}
	logger.Info("OTLP receiver stopped")
	return nil
}

func grpcListenForLog(m *Module) string {
	if m == nil || m.grpc == nil {
		return "disabled"
	}
	return m.cfg.GRPC.Listen
}

func httpListenForLog(m *Module) string {
	if m == nil || m.http == nil {
		return "disabled"
	}
	return m.cfg.HTTP.Listen
}
