// Copyright (C) 2025 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package writer

import (
	"fmt"
	"time"

	"github.com/sipcapture/homer-core/src/config"
	logger "github.com/sipcapture/homer-core/src/utils/logging"
	"github.com/valyala/fasthttp"
)

// HTTPServer handles HTTP HEP packet reception
type HTTPServer struct {
	writer *Writer
	config *config.HTTPServerConfig
	server *fasthttp.Server
}

// NewHTTPServer creates a new HTTP server
func NewHTTPServer(w *Writer, cfg *config.HTTPServerConfig) *HTTPServer {
	return &HTTPServer{
		writer: w,
		config: cfg,
	}
}

// Start starts the HTTP server
func (s *HTTPServer) Start() error {
	s.server = &fasthttp.Server{
		Handler:            s.handleRequest,
		ReadTimeout:        time.Duration(s.config.ReadTimeout) * time.Second,
		WriteTimeout:       time.Duration(s.config.WriteTimeout) * time.Second,
		MaxRequestBodySize: s.config.MaxRequestBodySize,
		Name:               "homer-writer",
	}

	addr := fmt.Sprintf("%s:%d", s.config.Host, s.config.Port)
	logger.Info("Writer: Starting HTTP server", "addr", addr)

	return s.server.ListenAndServe(addr)
}

// Stop stops the HTTP server
func (s *HTTPServer) Stop() {
	if s.server != nil {
		_ = s.server.Shutdown()
	}
}

// handleRequest handles incoming HTTP requests
func (s *HTTPServer) handleRequest(ctx *fasthttp.RequestCtx) {
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
func (s *HTTPServer) handleHEP(ctx *fasthttp.RequestCtx) {
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
		s.writer.EnqueuePacket(body, "http")
		ctx.SetStatusCode(fasthttp.StatusOK)
		ctx.SetBodyString("OK")
		return
	}

	// HEPv2 magic
	if body[0] == 0x02 && body[1] == 0x10 && body[2] == 0x02 {
		s.writer.EnqueuePacket(body, "http")
		ctx.SetStatusCode(fasthttp.StatusOK)
		ctx.SetBodyString("OK")
		return
	}

	ctx.SetStatusCode(fasthttp.StatusBadRequest)
	ctx.SetBodyString("Invalid HEP magic")
}

// handleHEPProtobuf handles HEP protobuf packets
func (s *HTTPServer) handleHEPProtobuf(ctx *fasthttp.RequestCtx) {
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

	// Protobuf packets are passed directly to decoder
	s.writer.EnqueuePacket(body, "http")
	ctx.SetStatusCode(fasthttp.StatusOK)
	ctx.SetBodyString("OK")
}

// handleHealth handles health check requests
func (s *HTTPServer) handleHealth(ctx *fasthttp.RequestCtx) {
	ctx.SetStatusCode(fasthttp.StatusOK)
	ctx.SetContentType("application/json")
	ctx.SetBodyString(`{"status":"ok","module":"writer"}`)
}
