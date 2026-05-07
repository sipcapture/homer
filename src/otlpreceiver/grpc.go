// Copyright (C) 2025 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package otlpreceiver

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"os"

	"github.com/sipcapture/homer-core/src/config"
	"github.com/sipcapture/homer-core/src/otlpreceiver/sink"
	logger "github.com/sipcapture/homer-core/src/utils/logging"
	"github.com/sipcapture/homer-core/src/utils/metrics"

	collogspb "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	colmetricspb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

// grpcServer implements the three OTLP collector gRPC services and
// owns the underlying *grpc.Server + listener.
type grpcServer struct {
	cfg      *config.OTLPConfig
	sink     sink.Sink
	server   *grpc.Server
	listener net.Listener
	listen   string
}

func newGRPCServer(cfg *config.OTLPConfig, s sink.Sink) (*grpcServer, error) {
	listen := cfg.GRPC.Listen
	if listen == "" {
		listen = ":4317"
	}

	opts := []grpc.ServerOption{
		grpc.MaxRecvMsgSize(maxRecvBytes(cfg)),
	}
	if cfg.GRPC.Cert != "" && cfg.GRPC.Key != "" {
		creds, err := loadServerTLS(cfg.GRPC.Cert, cfg.GRPC.Key, cfg.GRPC.CaCert)
		if err != nil {
			return nil, fmt.Errorf("load tls: %w", err)
		}
		opts = append(opts, grpc.Creds(creds))
	}

	gs := &grpcServer{
		cfg:    cfg,
		sink:   s,
		server: grpc.NewServer(opts...),
		listen: listen,
	}

	coltracepb.RegisterTraceServiceServer(gs.server, &grpcTraceService{sink: s})
	colmetricspb.RegisterMetricsServiceServer(gs.server, &grpcMetricsService{sink: s})
	collogspb.RegisterLogsServiceServer(gs.server, &grpcLogsService{sink: s})

	return gs, nil
}

// Start binds the listener and launches Serve in a background goroutine.
// Returns once the socket is open so the module Start can move on to
// the HTTP listener without racing against the first inbound RPC.
func (g *grpcServer) Start() error {
	ln, err := net.Listen("tcp", g.listen)
	if err != nil {
		return fmt.Errorf("listen %s: %w", g.listen, err)
	}
	g.listener = ln
	go func() {
		if err := g.server.Serve(ln); err != nil {
			logger.Warn(fmt.Sprintf("OTLP grpc serve exited: %v", err))
		}
	}()
	logger.Info("OTLP gRPC listener ready", "listen", g.listen)
	return nil
}

// Stop gracefully drains in-flight RPCs and closes the socket.
func (g *grpcServer) Stop() error {
	if g == nil || g.server == nil {
		return nil
	}
	g.server.GracefulStop()
	if g.listener != nil {
		_ = g.listener.Close()
	}
	return nil
}

// loadServerTLS builds a credentials.TransportCredentials from the
// receiver TLS config. caCert, when set, switches the listener to
// mutual TLS (clients must present a cert signed by it).
func loadServerTLS(certFile, keyFile, caFile string) (credentials.TransportCredentials, error) {
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, fmt.Errorf("load keypair: %w", err)
	}
	tlsCfg := &tls.Config{
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
		tlsCfg.ClientCAs = pool
		tlsCfg.ClientAuth = tls.RequireAndVerifyClientCert
	}
	return credentials.NewTLS(tlsCfg), nil
}

// grpcTraceService is the OTLP Trace gRPC service. The Export RPC just
// hands the request straight to the sink; partial-success accounting
// is not yet exposed (the standard says a count of zero rejected
// records means "all good", which is what an empty response signals).
type grpcTraceService struct {
	coltracepb.UnimplementedTraceServiceServer
	sink sink.Sink
}

// Export implements the OTLP Trace Export RPC.
func (s *grpcTraceService) Export(ctx context.Context, req *coltracepb.ExportTraceServiceRequest) (*coltracepb.ExportTraceServiceResponse, error) {
	metrics.RecordOTLPRequest("traces", "grpc")
	if err := s.sink.PushTraces(ctx, req); err != nil {
		metrics.RecordOTLPFailure("traces", "grpc", "sink_error")
		return nil, err
	}
	return &coltracepb.ExportTraceServiceResponse{}, nil
}

type grpcMetricsService struct {
	colmetricspb.UnimplementedMetricsServiceServer
	sink sink.Sink
}

// Export implements the OTLP Metrics Export RPC.
func (s *grpcMetricsService) Export(ctx context.Context, req *colmetricspb.ExportMetricsServiceRequest) (*colmetricspb.ExportMetricsServiceResponse, error) {
	metrics.RecordOTLPRequest("metrics", "grpc")
	if err := s.sink.PushMetrics(ctx, req); err != nil {
		metrics.RecordOTLPFailure("metrics", "grpc", "sink_error")
		return nil, err
	}
	return &colmetricspb.ExportMetricsServiceResponse{}, nil
}

type grpcLogsService struct {
	collogspb.UnimplementedLogsServiceServer
	sink sink.Sink
}

// Export implements the OTLP Logs Export RPC.
func (s *grpcLogsService) Export(ctx context.Context, req *collogspb.ExportLogsServiceRequest) (*collogspb.ExportLogsServiceResponse, error) {
	metrics.RecordOTLPRequest("logs", "grpc")
	if err := s.sink.PushLogs(ctx, req); err != nil {
		metrics.RecordOTLPFailure("logs", "grpc", "sink_error")
		return nil, err
	}
	return &collogspb.ExportLogsServiceResponse{}, nil
}

func maxRecvBytes(cfg *config.OTLPConfig) int {
	if cfg.MaxRecvMsgBytes > 0 {
		return cfg.MaxRecvMsgBytes
	}
	return 4 * 1024 * 1024
}
