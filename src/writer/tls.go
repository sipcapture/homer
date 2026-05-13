// Copyright (C) 2025 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package writer

import (
	"bufio"
	"crypto/tls"
	"crypto/x509"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/sipcapture/homer-core/src/config"
	logger "github.com/sipcapture/homer-core/src/utils/logging"
)

const (
	defaultTLSReadTimeout = 5 * time.Minute
	defaultTLSMaxConns    = 1024
)

// TLSServer handles TLS HEP packet reception
type TLSServer struct {
	writer      *Writer
	config      *config.TLSServerConfig
	listener    net.Listener
	wg          sync.WaitGroup
	quit        chan struct{}
	stopOnce    sync.Once
	connSem     chan struct{}
	readTimeout time.Duration
}

// NewTLSServer creates a new TLS server
func NewTLSServer(w *Writer, cfg *config.TLSServerConfig) *TLSServer {
	readTimeout := defaultTLSReadTimeout
	if cfg.ReadTimeoutSec > 0 {
		readTimeout = time.Duration(cfg.ReadTimeoutSec) * time.Second
	}

	maxConns := defaultTLSMaxConns
	if cfg.MaxConnections > 0 {
		maxConns = cfg.MaxConnections
	}

	return &TLSServer{
		writer:      w,
		config:      cfg,
		quit:        make(chan struct{}),
		connSem:     make(chan struct{}, maxConns),
		readTimeout: readTimeout,
	}
}

// Start starts the TLS server
func (s *TLSServer) Start() error {
	// Load certificate
	cert, err := tls.LoadX509KeyPair(s.config.Cert, s.config.Key)
	if err != nil {
		return fmt.Errorf("TLS load cert error: %w", err)
	}

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   getTLSVersion(s.config.MinTLSVersion),
		MaxVersion:   getTLSVersion(s.config.MaxTLSVersion),
	}

	// Load CA cert for client verification if mutual TLS is enabled
	if s.config.MutualTLS && s.config.CaCert != "" {
		caCert, err := os.ReadFile(s.config.CaCert)
		if err != nil {
			return fmt.Errorf("TLS load CA cert error: %w", err)
		}
		caCertPool := x509.NewCertPool()
		caCertPool.AppendCertsFromPEM(caCert)
		tlsConfig.ClientCAs = caCertPool
		tlsConfig.ClientAuth = tls.RequireAndVerifyClientCert
	}

	if s.config.InsecureSkipVerify {
		tlsConfig.InsecureSkipVerify = true
	}

	addr := fmt.Sprintf("%s:%d", s.config.Host, s.config.Port)
	listener, err := tls.Listen("tcp", addr, tlsConfig)
	if err != nil {
		return fmt.Errorf("TLS listen error: %w", err)
	}
	s.listener = listener

	logger.Info("Writer: Starting TLS server", "addr", addr,
		"read_timeout", s.readTimeout.String(),
		"max_connections", cap(s.connSem))

	for {
		conn, err := listener.Accept()
		if err != nil {
			select {
			case <-s.quit:
				return nil
			default:
				logger.Error(fmt.Sprintf("Writer: TLS accept error: %v", err))
				continue
			}
		}

		select {
		case s.connSem <- struct{}{}:
		default:
			conn.Close()
			logger.Warn("Writer: TLS max connections reached, rejecting")
			continue
		}

		atomic.AddInt64(&s.writer.connCount, 1)
		s.wg.Add(1)
		go s.handleConnection(conn)
	}
}

// Stop stops the TLS server
func (s *TLSServer) Stop() {
	s.stopOnce.Do(func() {
		close(s.quit)
		if s.listener != nil {
			s.listener.Close()
		}
		s.wg.Wait()
	})
}

// handleConnection handles a single TLS connection.
// Uses bufio.Reader for efficient reads and a pre-allocated buffer
// to avoid per-packet heap allocations (same pattern as TCP).
func (s *TLSServer) handleConnection(conn net.Conn) {
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
					logger.Debug("Writer: TLS connection idle timeout, closing")
				} else {
					logger.Debug(fmt.Sprintf("Writer: TLS read header error: %v", err))
				}
			}
			return
		}

		// Check HEPv3 magic
		if header[0] != 'H' || header[1] != 'E' || header[2] != 'P' || header[3] != '3' {
			logger.Debug("Writer: Invalid HEP magic in TLS stream")
			return
		}

		// Read length (big endian)
		length := binary.BigEndian.Uint16(header[4:6])
		if length < 6 || length > maxPktLen {
			logger.Debug(fmt.Sprintf("Writer: Invalid HEP length: %d", length))
			return
		}

		copy(buf[:6], header[:])
		if _, err := io.ReadFull(br, buf[6:length]); err != nil {
			logger.Debug(fmt.Sprintf("Writer: TLS read payload error: %v", err))
			return
		}

		s.writer.EnqueuePacket(buf[:length], "tls")
	}
}

// getTLSVersion converts TLS version string to uint16
func getTLSVersion(version string) uint16 {
	switch version {
	case "TLS1.0":
		return tls.VersionTLS10
	case "TLS1.1":
		return tls.VersionTLS11
	case "TLS1.2":
		return tls.VersionTLS12
	case "TLS1.3":
		return tls.VersionTLS13
	default:
		return tls.VersionTLS12
	}
}
