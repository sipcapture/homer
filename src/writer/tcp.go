// Copyright (C) 2025 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package writer

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/sipcapture/homer-core/src/config"
	logger "github.com/sipcapture/homer-core/src/utils/logging"
)

const (
	defaultTCPReadTimeout = 5 * time.Minute
	defaultTCPMaxConns    = 1024
)

// TCPServer handles TCP HEP packet reception
type TCPServer struct {
	writer      *Writer
	config      *config.TCPServerConfig
	listener    net.Listener
	wg          sync.WaitGroup
	quit        chan struct{}
	stopOnce    sync.Once
	connSem     chan struct{} // semaphore for max connections
	readTimeout time.Duration
}

// NewTCPServer creates a new TCP server
func NewTCPServer(w *Writer, cfg *config.TCPServerConfig) *TCPServer {
	readTimeout := defaultTCPReadTimeout
	if cfg.ReadTimeoutSec > 0 {
		readTimeout = time.Duration(cfg.ReadTimeoutSec) * time.Second
	}

	maxConns := defaultTCPMaxConns
	if cfg.MaxConnections > 0 {
		maxConns = cfg.MaxConnections
	}

	return &TCPServer{
		writer:      w,
		config:      cfg,
		quit:        make(chan struct{}),
		connSem:     make(chan struct{}, maxConns),
		readTimeout: readTimeout,
	}
}

// Start starts the TCP server
func (s *TCPServer) Start() error {
	addr := fmt.Sprintf("%s:%d", s.config.Host, s.config.Port)

	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("TCP listen error: %w", err)
	}
	s.listener = listener

	logger.Info("Writer: Starting TCP server", "addr", addr,
		"read_timeout", s.readTimeout.String(),
		"max_connections", cap(s.connSem))

	for {
		conn, err := listener.Accept()
		if err != nil {
			select {
			case <-s.quit:
				return nil
			default:
				logger.Error(fmt.Sprintf("Writer: TCP accept error: %v", err))
				continue
			}
		}

		select {
		case s.connSem <- struct{}{}:
		default:
			conn.Close()
			logger.Warn("Writer: TCP max connections reached, rejecting")
			continue
		}

		atomic.AddInt64(&s.writer.connCount, 1)
		s.wg.Add(1)
		go s.handleConnection(conn)
	}
}

// Stop stops the TCP server
func (s *TCPServer) Stop() {
	s.stopOnce.Do(func() {
		close(s.quit)
		if s.listener != nil {
			s.listener.Close()
		}
		s.wg.Wait()
	})
}

// handleConnection handles a single TCP connection.
// Uses bufio.Reader for efficient reads and a pre-allocated buffer
// to avoid per-packet heap allocations.
func (s *TCPServer) handleConnection(conn net.Conn) {
	defer s.wg.Done()
	defer conn.Close()
	defer atomic.AddInt64(&s.writer.connCount, -1)
	defer func() { <-s.connSem }()

	br := bufio.NewReaderSize(conn, 128*1024)
	var header [6]byte
	buf := make([]byte, maxPktLen)

	for {
		select {
		case <-s.quit:
			return
		default:
		}

		conn.SetReadDeadline(time.Now().Add(s.readTimeout))

		if _, err := io.ReadFull(br, header[:]); err != nil {
			if err != io.EOF {
				if ne, ok := err.(net.Error); ok && ne.Timeout() {
					logger.Debug("Writer: TCP connection idle timeout, closing")
				} else {
					logger.Debug(fmt.Sprintf("Writer: TCP read header error: %v", err))
				}
			}
			return
		}

		if header[0] != 'H' || header[1] != 'E' || header[2] != 'P' || header[3] != '3' {
			logger.Debug("Writer: Invalid HEP magic in TCP stream")
			return
		}

		length := binary.BigEndian.Uint16(header[4:6])
		if length < 6 || length > maxPktLen {
			logger.Debug(fmt.Sprintf("Writer: Invalid HEP length: %d", length))
			return
		}

		copy(buf[:6], header[:])
		if _, err := io.ReadFull(br, buf[6:length]); err != nil {
			logger.Debug(fmt.Sprintf("Writer: TCP read payload error: %v", err))
			return
		}

		s.writer.EnqueuePacket(buf[:length], "tcp")
	}
}
