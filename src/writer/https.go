// Copyright (C) 2025 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package writer

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"time"

	"github.com/sipcapture/homer-core/src/config"
	logger "github.com/sipcapture/homer-core/src/utils/logging"
	"github.com/valyala/fasthttp"
)

// HTTPSServer handles HTTPS HEP packet reception
type HTTPSServer struct {
	writer *Writer
	config *config.HTTPSServerConfig
	server *fasthttp.Server
}

// NewHTTPSServer creates a new HTTPS server
func NewHTTPSServer(w *Writer, cfg *config.HTTPSServerConfig) *HTTPSServer {
	return &HTTPSServer{
		writer: w,
		config: cfg,
	}
}

// Start starts the HTTPS server
func (s *HTTPSServer) Start() error {
	// Load certificate
	cert, err := tls.LoadX509KeyPair(s.config.Cert, s.config.Key)
	if err != nil {
		return fmt.Errorf("HTTPS load cert error: %w", err)
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
			return fmt.Errorf("HTTPS load CA cert error: %w", err)
		}
		caCertPool := x509.NewCertPool()
		caCertPool.AppendCertsFromPEM(caCert)
		tlsConfig.ClientCAs = caCertPool
		tlsConfig.ClientAuth = tls.RequireAndVerifyClientCert
	}

	if s.config.InsecureSkipVerify {
		tlsConfig.InsecureSkipVerify = true
	}

	s.server = &fasthttp.Server{
		Handler:            s.handleRequest,
		ReadTimeout:        time.Duration(30) * time.Second,
		WriteTimeout:       time.Duration(30) * time.Second,
		MaxRequestBodySize: 67108864,
		Name:               "homer-writer-https",
		TLSConfig:          tlsConfig,
	}

	addr := fmt.Sprintf("%s:%d", s.config.Host, s.config.Port)
	logger.Info("Writer: Starting HTTPS server", "addr", addr)

	return s.server.ListenAndServeTLS(addr, s.config.Cert, s.config.Key)
}

// Stop stops the HTTPS server
func (s *HTTPSServer) Stop() {
	if s.server != nil {
		_ = s.server.Shutdown()
	}
}

// handleRequest handles incoming HTTPS requests
func (s *HTTPSServer) handleRequest(ctx *fasthttp.RequestCtx) {
	path := string(ctx.Path())

	switch path {
	case "/api/hep", "/api/hep/binary":
		s.handleHEP(ctx)
	case "/api/hep/protobuf":
		s.handleHEPProtobuf(ctx)
	case "/health":
		s.handleHealth(ctx)
	default:
		ctx.SetStatusCode(fasthttp.StatusNotFound)
		ctx.SetBodyString("Not Found")
	}
}

// handleHEP handles HEP binary packets
func (s *HTTPSServer) handleHEP(ctx *fasthttp.RequestCtx) {
	if !ctx.IsPost() {
		ctx.SetStatusCode(fasthttp.StatusMethodNotAllowed)
		return
	}

	body := ctx.PostBody()
	if len(body) < 6 {
		ctx.SetStatusCode(fasthttp.StatusBadRequest)
		ctx.SetBodyString("Invalid HEP packet: too short")
		return
	}

	// Validate HEP magic
	if body[0] == 'H' && body[1] == 'E' && body[2] == 'P' && body[3] == '3' {
		s.writer.EnqueuePacket(body, "https")
		ctx.SetStatusCode(fasthttp.StatusOK)
		ctx.SetBodyString("OK")
		return
	}

	// HEPv2 magic
	if body[0] == 0x02 && body[1] == 0x10 && body[2] == 0x02 {
		s.writer.EnqueuePacket(body, "https")
		ctx.SetStatusCode(fasthttp.StatusOK)
		ctx.SetBodyString("OK")
		return
	}

	ctx.SetStatusCode(fasthttp.StatusBadRequest)
	ctx.SetBodyString("Invalid HEP magic")
}

// handleHEPProtobuf handles HEP protobuf packets
func (s *HTTPSServer) handleHEPProtobuf(ctx *fasthttp.RequestCtx) {
	if !ctx.IsPost() {
		ctx.SetStatusCode(fasthttp.StatusMethodNotAllowed)
		return
	}

	body := ctx.PostBody()
	if len(body) < 1 {
		ctx.SetStatusCode(fasthttp.StatusBadRequest)
		ctx.SetBodyString("Empty body")
		return
	}

	s.writer.EnqueuePacket(body, "https")
	ctx.SetStatusCode(fasthttp.StatusOK)
	ctx.SetBodyString("OK")
}

// handleHealth handles health check requests
func (s *HTTPSServer) handleHealth(ctx *fasthttp.RequestCtx) {
	ctx.SetStatusCode(fasthttp.StatusOK)
	ctx.SetContentType("application/json")
	ctx.SetBodyString(`{"status":"ok","module":"writer"}`)
}
