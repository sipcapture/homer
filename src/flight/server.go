// Copyright (C) 2025 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package flight

import (
	"fmt"
	"net"
	"sync"

	airport "github.com/hugr-lab/airport-go"
	logger "github.com/sipcapture/homer-core/src/utils/logging"
	"google.golang.org/grpc"
)

// ServerConfig holds configuration for the Flight server
type ServerConfig struct {
	// Listen address for gRPC server
	ListenAddr string
	// Maximum message size in bytes
	MaxMessageSize int
	// Authentication token (optional)
	AuthToken string
	// Buffer size for HEP packets (ring buffer)
	BufferSize int
}

// DefaultServerConfig returns default Flight server configuration
func DefaultServerConfig() ServerConfig {
	return ServerConfig{
		ListenAddr:     ":50051",
		MaxMessageSize: 16 * 1024 * 1024, // 16MB
		BufferSize:     100000,           // 100k packets in memory
	}
}

// FlightServer wraps the airport Flight server with HEP data
type FlightServer struct {
	config     ServerConfig
	grpcServer *grpc.Server
	catalog    *HEPCatalog
	listener   net.Listener
	mu         sync.RWMutex
	running    bool
}

// NewFlightServer creates a new Flight server for HEP data
func NewFlightServer(config ServerConfig) (*FlightServer, error) {
	if config.BufferSize <= 0 {
		config.BufferSize = 100000
	}
	if config.MaxMessageSize <= 0 {
		config.MaxMessageSize = 16 * 1024 * 1024
	}

	// Create HEP catalog
	catalog := NewHEPCatalog(config.BufferSize)

	// Build airport config
	airportConfig := airport.ServerConfig{
		Catalog:        catalog,
		MaxMessageSize: config.MaxMessageSize,
	}

	// Add authentication if configured
	if config.AuthToken != "" {
		airportConfig.Auth = airport.BearerAuth(func(token string) (string, error) {
			if token == config.AuthToken {
				return "homer-user", nil
			}
			return "", airport.ErrUnauthorized
		})
	}

	// Create gRPC server with airport options
	opts := airport.ServerOptions(airportConfig)
	grpcServer := grpc.NewServer(opts...)

	// Register airport Flight service
	airport.NewServer(grpcServer, airportConfig)

	return &FlightServer{
		config:     config,
		grpcServer: grpcServer,
		catalog:    catalog,
	}, nil
}

// Start starts the Flight server
func (fs *FlightServer) Start() error {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	if fs.running {
		return fmt.Errorf("flight server already running")
	}

	listener, err := net.Listen("tcp", fs.config.ListenAddr)
	if err != nil {
		return fmt.Errorf("failed to listen: %w", err)
	}
	fs.listener = listener
	fs.running = true

	go func() {
		logger.Info(fmt.Sprintf("Flight server started on %s", fs.config.ListenAddr))
		if err := fs.grpcServer.Serve(listener); err != nil {
			logger.Error(fmt.Sprintf("Flight server error: %v", err))
		}
	}()

	return nil
}

// Stop stops the Flight server gracefully
func (fs *FlightServer) Stop() error {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	if !fs.running {
		return nil
	}

	fs.grpcServer.GracefulStop()
	fs.running = false
	logger.Info("Flight server stopped")
	return nil
}

// GetCatalog returns the HEP catalog for adding data
func (fs *FlightServer) GetCatalog() *HEPCatalog {
	return fs.catalog
}

// IsRunning returns true if the server is running
func (fs *FlightServer) IsRunning() bool {
	fs.mu.RLock()
	defer fs.mu.RUnlock()
	return fs.running
}

// GetListenAddr returns the actual listen address
func (fs *FlightServer) GetListenAddr() string {
	fs.mu.RLock()
	defer fs.mu.RUnlock()
	if fs.listener != nil {
		return fs.listener.Addr().String()
	}
	return fs.config.ListenAddr
}
