// Copyright (C) 2025 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package otlpreceiver

import (
	"compress/gzip"
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/sipcapture/homer-core/src/config"
	"github.com/sipcapture/homer-core/src/otlpreceiver/sink"
	logger "github.com/sipcapture/homer-core/src/utils/logging"
	"github.com/sipcapture/homer-core/src/utils/metrics"

	collogspb "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	colmetricspb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

// httpServer hosts the OTLP/HTTP endpoints. The server itself is
// transport-agnostic; per-signal logic lives in handleTraces /
// handleMetrics / handleLogs.
type httpServer struct {
	cfg      *config.OTLPConfig
	sink     sink.Sink
	server   *http.Server
	listener net.Listener
}

const (
	contentTypeProtobuf = "application/x-protobuf"
	contentTypeJSON     = "application/json"
)

func newHTTPServer(cfg *config.OTLPConfig, s sink.Sink) (*httpServer, error) {
	listen := cfg.HTTP.Listen
	if listen == "" {
		listen = ":4318"
	}

	hs := &httpServer{cfg: cfg, sink: s}

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/traces", hs.handleTraces)
	mux.HandleFunc("/v1/metrics", hs.handleMetrics)
	mux.HandleFunc("/v1/logs", hs.handleLogs)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "ok")
	})

	hs.server = &http.Server{
		Addr:         listen,
		Handler:      mux,
		ReadTimeout:  time.Duration(cfg.HTTP.ReadTimeoutSec) * time.Second,
		WriteTimeout: time.Duration(cfg.HTTP.WriteTimeoutSec) * time.Second,
	}
	if cfg.HTTP.Cert != "" && cfg.HTTP.Key != "" {
		tlsCfg, err := loadHTTPTLS(cfg.HTTP.Cert, cfg.HTTP.Key, cfg.HTTP.CaCert)
		if err != nil {
			return nil, fmt.Errorf("load tls: %w", err)
		}
		hs.server.TLSConfig = tlsCfg
	}
	return hs, nil
}

// Start opens the listener and runs Serve / ServeTLS in a goroutine.
func (h *httpServer) Start() error {
	ln, err := net.Listen("tcp", h.server.Addr)
	if err != nil {
		return fmt.Errorf("listen %s: %w", h.server.Addr, err)
	}
	h.listener = ln
	go func() {
		var serveErr error
		if h.server.TLSConfig != nil {
			serveErr = h.server.ServeTLS(ln, "", "")
		} else {
			serveErr = h.server.Serve(ln)
		}
		if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			logger.Warn(fmt.Sprintf("OTLP http serve exited: %v", serveErr))
		}
	}()
	logger.Info("OTLP HTTP listener ready", "listen", h.server.Addr, "tls", h.server.TLSConfig != nil)
	return nil
}

// Stop shuts the HTTP server with a short deadline; if Shutdown does
// not complete in time the listener is closed forcefully.
func (h *httpServer) Stop() error {
	if h == nil || h.server == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := h.server.Shutdown(ctx); err != nil {
		_ = h.server.Close()
		return err
	}
	return nil
}

// transportFromContentType maps the request Content-Type to one of the
// transport label values used in homer_otlp_* metrics.
func transportFromContentType(ct string) string {
	mt := strings.TrimSpace(strings.ToLower(strings.SplitN(ct, ";", 2)[0]))
	switch mt {
	case contentTypeProtobuf:
		return "http_proto"
	case contentTypeJSON:
		return "http_json"
	}
	return "http_proto"
}

// readBody respects Content-Length, max body size, and gzip
// Content-Encoding. Returns a low-cardinality reason string on error
// so the metrics labels stay stable.
func (h *httpServer) readBody(w http.ResponseWriter, r *http.Request, signal string) ([]byte, string, bool) {
	limit := int64(maxRecvBytes(h.cfg))
	r.Body = http.MaxBytesReader(w, r.Body, limit)
	defer r.Body.Close()

	var src io.Reader = r.Body
	if strings.EqualFold(r.Header.Get("Content-Encoding"), "gzip") {
		gz, err := gzip.NewReader(r.Body)
		if err != nil {
			http.Error(w, "invalid gzip body", http.StatusBadRequest)
			return nil, "gzip_decode", false
		}
		defer gz.Close()
		src = gz
	}

	body, err := io.ReadAll(src)
	if err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
			return nil, "body_too_large", false
		}
		http.Error(w, "failed to read body", http.StatusBadRequest)
		return nil, "read_error", false
	}
	_ = signal // kept for future per-signal limits
	return body, "", true
}

// decodeRequest unmarshals the body into msg according to the request
// content type. Returns false (and writes 400/415) on failure.
func (h *httpServer) decodeRequest(w http.ResponseWriter, r *http.Request, body []byte, msg proto.Message) (string, bool) {
	ct := r.Header.Get("Content-Type")
	mt := strings.TrimSpace(strings.ToLower(strings.SplitN(ct, ";", 2)[0]))
	switch mt {
	case contentTypeProtobuf, "":
		if err := proto.Unmarshal(body, msg); err != nil {
			http.Error(w, "protobuf decode error", http.StatusBadRequest)
			return "decode_error", false
		}
	case contentTypeJSON:
		if err := protojson.Unmarshal(body, msg); err != nil {
			http.Error(w, "json decode error", http.StatusBadRequest)
			return "decode_error", false
		}
	default:
		http.Error(w, "unsupported media type", http.StatusUnsupportedMediaType)
		return "unsupported_media_type", false
	}
	return "", true
}

// writeResponse marshals resp using the same wire format as the request
// and writes a 200 OK. The OTLP spec requires the response to be the
// same content type as the request body.
func (h *httpServer) writeResponse(w http.ResponseWriter, r *http.Request, resp proto.Message) {
	ct := r.Header.Get("Content-Type")
	mt := strings.TrimSpace(strings.ToLower(strings.SplitN(ct, ";", 2)[0]))
	switch mt {
	case contentTypeJSON:
		out, err := protojson.Marshal(resp)
		if err != nil {
			http.Error(w, "marshal error", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", contentTypeJSON)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(out)
	default:
		out, err := proto.Marshal(resp)
		if err != nil {
			http.Error(w, "marshal error", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", contentTypeProtobuf)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(out)
	}
}

func (h *httpServer) handleTraces(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	transport := transportFromContentType(r.Header.Get("Content-Type"))
	metrics.RecordOTLPRequest("traces", transport)

	body, reason, ok := h.readBody(w, r, "traces")
	if !ok {
		metrics.RecordOTLPFailure("traces", transport, reason)
		return
	}
	req := &coltracepb.ExportTraceServiceRequest{}
	if reason, ok := h.decodeRequest(w, r, body, req); !ok {
		metrics.RecordOTLPFailure("traces", transport, reason)
		return
	}
	if err := h.sink.PushTraces(r.Context(), req); err != nil {
		metrics.RecordOTLPFailure("traces", transport, "sink_error")
		http.Error(w, "sink error", http.StatusInternalServerError)
		return
	}
	h.writeResponse(w, r, &coltracepb.ExportTraceServiceResponse{})
}

func (h *httpServer) handleMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	transport := transportFromContentType(r.Header.Get("Content-Type"))
	metrics.RecordOTLPRequest("metrics", transport)

	body, reason, ok := h.readBody(w, r, "metrics")
	if !ok {
		metrics.RecordOTLPFailure("metrics", transport, reason)
		return
	}
	req := &colmetricspb.ExportMetricsServiceRequest{}
	if reason, ok := h.decodeRequest(w, r, body, req); !ok {
		metrics.RecordOTLPFailure("metrics", transport, reason)
		return
	}
	if err := h.sink.PushMetrics(r.Context(), req); err != nil {
		metrics.RecordOTLPFailure("metrics", transport, "sink_error")
		http.Error(w, "sink error", http.StatusInternalServerError)
		return
	}
	h.writeResponse(w, r, &colmetricspb.ExportMetricsServiceResponse{})
}

func (h *httpServer) handleLogs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	transport := transportFromContentType(r.Header.Get("Content-Type"))
	metrics.RecordOTLPRequest("logs", transport)

	body, reason, ok := h.readBody(w, r, "logs")
	if !ok {
		metrics.RecordOTLPFailure("logs", transport, reason)
		return
	}
	req := &collogspb.ExportLogsServiceRequest{}
	if reason, ok := h.decodeRequest(w, r, body, req); !ok {
		metrics.RecordOTLPFailure("logs", transport, reason)
		return
	}
	if err := h.sink.PushLogs(r.Context(), req); err != nil {
		metrics.RecordOTLPFailure("logs", transport, "sink_error")
		http.Error(w, "sink error", http.StatusInternalServerError)
		return
	}
	h.writeResponse(w, r, &collogspb.ExportLogsServiceResponse{})
}

// loadHTTPTLS builds a *tls.Config for the OTLP/HTTP listener,
// optionally enabling mutual TLS when a CA file is provided.
func loadHTTPTLS(certFile, keyFile, caFile string) (*tls.Config, error) {
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, fmt.Errorf("load keypair: %w", err)
	}
	cfg := &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}
	if caFile != "" {
		caBytes, err := os.ReadFile(caFile)
		if err != nil {
			return nil, fmt.Errorf("read ca: %w", err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(caBytes) {
			return nil, fmt.Errorf("parse ca: no certificates in %s", caFile)
		}
		cfg.ClientCAs = pool
		cfg.ClientAuth = tls.RequireAndVerifyClientCert
	}
	return cfg, nil
}
