// Copyright (C) 2026 Homer Server Contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package lineprotoreceiver

import (
	"compress/gzip"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/sipcapture/homer-core/src/config"
	logger "github.com/sipcapture/homer-core/src/utils/logging"
	"github.com/sipcapture/homer-core/src/utils/metrics"
)

// httpServer hosts the InfluxDB Line Protocol HTTP endpoints. The
// per-route handlers all share the same Ingester; the only difference
// between v1 / v2 / v3 is the URL path and the response body shape on
// success / error.
type httpServer struct {
	cfg      *config.LineProtoConfig
	ing      *Ingester
	server   *http.Server
	listener net.Listener
}

func newHTTPServer(cfg *config.LineProtoConfig, ing *Ingester) (*httpServer, error) {
	if cfg == nil || ing == nil {
		return nil, fmt.Errorf("line-proto http: cfg and ingester required")
	}
	listen := cfg.Listen
	if listen == "" {
		listen = ":8086"
	}

	hs := &httpServer{cfg: cfg, ing: ing}

	mux := http.NewServeMux()
	mux.HandleFunc("/write", hs.handleWriteV1)
	mux.HandleFunc("/api/v2/write", hs.handleWriteV2)
	mux.HandleFunc("/api/v3/write_lp", hs.handleWriteV2) // gigapi alias
	mux.HandleFunc("/ping", hs.handlePing)
	mux.HandleFunc("/health", hs.handleHealth)

	hs.server = &http.Server{
		Addr:         listen,
		Handler:      mux,
		ReadTimeout:  time.Duration(cfg.ReadTimeoutSec) * time.Second,
		WriteTimeout: time.Duration(cfg.WriteTimeoutSec) * time.Second,
	}
	if cfg.Cert != "" && cfg.Key != "" {
		tlsCfg, err := loadHTTPTLS(cfg.Cert, cfg.Key, cfg.CaCert)
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
			logger.Warn(fmt.Sprintf("line-proto http serve exited: %v", serveErr))
		}
	}()
	logger.Info("Line Protocol HTTP listener ready",
		"listen", h.server.Addr, "tls", h.server.TLSConfig != nil)
	return nil
}

// Stop drains in-flight requests with a small grace period and force-
// closes the listener if Shutdown does not complete in time.
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

// handleWriteV1 implements InfluxDB v1 /write semantics.
//
//	POST /write?db=<name>&precision=<n|u|ms|s>
//
// Empty bodies return 204 No Content; parse errors return 400 with a
// JSON {"error": "..."}; storage errors return 500.
func (h *httpServer) handleWriteV1(w http.ResponseWriter, r *http.Request) {
	h.handleWrite(w, r, "v1", queryParam(r, "db", h.cfg.DefaultDB))
}

// handleWriteV2 implements InfluxDB v2 /api/v2/write semantics.
//
//	POST /api/v2/write?bucket=<name>&precision=<ns|us|ms|s>
//
// `org=` is accepted for client compatibility but ignored — homer-core
// has no organisation concept; the bucket name maps directly to a
// per-database DuckDB schema.
func (h *httpServer) handleWriteV2(w http.ResponseWriter, r *http.Request) {
	endpoint := "v2"
	if strings.HasPrefix(r.URL.Path, "/api/v3/") {
		endpoint = "v3"
	}
	bucket := queryParam(r, "bucket", queryParam(r, "db", h.cfg.DefaultDB))
	h.handleWrite(w, r, endpoint, bucket)
}

func (h *httpServer) handleWrite(w http.ResponseWriter, r *http.Request, endpoint, dbName string) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		metrics.RecordLineProtoRequest(endpoint, "method_not_allowed")
		return
	}

	if _, has := r.URL.Query()["hep_table"]; has {
		writeJSONError(w, http.StatusBadRequest, "query parameter hep_table is no longer supported; use measurement names that match real HEP tables (e.g. hep_proto_1_call) when ingest.line_protocol.allow_hep_sip_call is true")
		metrics.RecordLineProtoRequest(endpoint, "bad_request")
		return
	}

	body, reason, ok := h.readBody(w, r)
	if !ok {
		metrics.RecordLineProtoRequest(endpoint, reason)
		return
	}
	if len(body) == 0 {
		w.WriteHeader(http.StatusNoContent)
		metrics.RecordLineProtoRequest(endpoint, "ok")
		return
	}
	precision := ParsePrecision(queryParam(r, "precision", h.cfg.DefaultPrecision))
	n, err := h.ing.Ingest(r.Context(), dbName, body, precision)
	if err != nil {
		// Differentiate parse-vs-write so dashboards / metrics can
		// tell client misbehaviour from server-side problems.
		status := http.StatusInternalServerError
		outcome := "write_error"
		if strings.Contains(err.Error(), "parse error") ||
			strings.HasPrefix(err.Error(), "point ") {
			status = http.StatusBadRequest
			outcome = "parse_error"
		}
		writeJSONError(w, status, err.Error())
		metrics.RecordLineProtoRequest(endpoint, outcome)
		return
	}
	metrics.AddLineProtoPoints(dbName, n)
	metrics.RecordLineProtoRequest(endpoint, "ok")
	w.WriteHeader(http.StatusNoContent)
}

func (h *httpServer) handlePing(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	// InfluxDB sets X-Influxdb-Version on /ping; clients use it to
	// auto-detect the dialect (v1 vs v2).
	w.Header().Set("X-Influxdb-Version", "homer-core")
	w.Header().Set("X-Influxdb-Build", "OSS")
	w.WriteHeader(http.StatusNoContent)
	metrics.RecordLineProtoRequest("ping", "ok")
}

func (h *httpServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"name":"homer-core","status":"pass","version":"oss"}`))
	metrics.RecordLineProtoRequest("health", "ok")
}

// readBody respects Content-Length, max body size, and gzip
// Content-Encoding. Returns a low-cardinality reason string on error
// so the metrics labels stay stable.
func (h *httpServer) readBody(w http.ResponseWriter, r *http.Request) ([]byte, string, bool) {
	limit := h.cfg.MaxBodyBytes
	if limit <= 0 {
		limit = 8 * 1024 * 1024
	}
	r.Body = http.MaxBytesReader(w, r.Body, limit)
	defer r.Body.Close()

	var src io.Reader = r.Body
	if strings.EqualFold(r.Header.Get("Content-Encoding"), "gzip") {
		gz, err := gzip.NewReader(r.Body)
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid gzip body")
			return nil, "gzip_decode", false
		}
		defer gz.Close()
		src = gz
	}
	body, err := io.ReadAll(src)
	if err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			writeJSONError(w, http.StatusRequestEntityTooLarge, "request body too large")
			return nil, "body_too_large", false
		}
		writeJSONError(w, http.StatusBadRequest, "failed to read body")
		return nil, "read_error", false
	}
	return body, "", true
}

func writeJSONError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	_ = enc.Encode(map[string]string{"error": msg})
}

// queryParam returns the named query parameter or fallback if absent.
func queryParam(r *http.Request, name, fallback string) string {
	if v := r.URL.Query().Get(name); v != "" {
		return v
	}
	return fallback
}

// loadHTTPTLS builds a *tls.Config for the LP listener, optionally
// enabling mutual TLS when a CA file is provided.
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
