// Copyright (C) 2026 Homer Server Contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later
//
// Apache Arrow FlightSQL gRPC server (Grafana InfluxDB / FlightSQL), separate
// from DuckDB Airport on flight_server.port.

package node

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/flight"
	"github.com/apache/arrow-go/v18/arrow/flight/flightsql"
	"github.com/apache/arrow-go/v18/arrow/memory"
	"github.com/sipcapture/homer-core/src/config"
	"github.com/sipcapture/homer-core/src/sqlrewrite"
	logger "github.com/sipcapture/homer-core/src/utils/logging"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// fsqlServer implements Arrow FlightSQL backed by the node's DuckDB/DuckLake.
type fsqlServer struct {
	flightsql.BaseServer

	node     *Node
	cfg      config.FlightSQLServerConfig
	lakeName string

	grpcServer *grpc.Server
	listener   net.Listener
	mu         sync.RWMutex
	running    bool

	refreshFn   func()
	stopRefresh chan struct{}
}

func newFsqlServer(node *Node, cfg config.FlightSQLServerConfig, lakeName string) *fsqlServer {
	s := &fsqlServer{
		node:     node,
		cfg:      cfg,
		lakeName: lakeName,
	}
	s.RegisterSqlInfo(flightsql.SqlInfoFlightSqlServerName, "homer-core")
	s.RegisterSqlInfo(flightsql.SqlInfoFlightSqlServerVersion, "1.0")
	s.RegisterSqlInfo(flightsql.SqlInfoFlightSqlServerArrowVersion, "1.3")
	s.RegisterSqlInfo(flightsql.SqlInfoFlightSqlServerReadOnly, true)
	return s
}

func (s *fsqlServer) withCatalogRefresher(fn func()) {
	s.refreshFn = fn
}

type fsqlTicketPayload struct {
	Query string `json:"q"`
}

func (s *fsqlServer) GetFlightInfoStatement(
	ctx context.Context, cmd flightsql.StatementQuery, desc *flight.FlightDescriptor,
) (*flight.FlightInfo, error) {
	query := cmd.GetQuery()
	if query == "" {
		return nil, status.Error(codes.InvalidArgument, "empty SQL query")
	}
	ticketBytes, err := json.Marshal(fsqlTicketPayload{Query: query})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to encode ticket payload: %v", err)
	}
	encodedTicket, err := flightsql.CreateStatementQueryTicket(ticketBytes)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to encode ticket: %v", err)
	}
	return &flight.FlightInfo{
		Schema:           []byte{},
		FlightDescriptor: desc,
		Endpoint: []*flight.FlightEndpoint{
			{Ticket: &flight.Ticket{Ticket: encodedTicket}},
		},
		TotalRecords: -1,
		TotalBytes:   -1,
	}, nil
}

func (s *fsqlServer) GetSchemaStatement(
	ctx context.Context, cmd flightsql.StatementQuery, desc *flight.FlightDescriptor,
) (*flight.SchemaResult, error) {
	query := cmd.GetQuery()
	if query == "" {
		return nil, status.Error(codes.InvalidArgument, "empty SQL query")
	}
	rewritten := s.prepareSQL(query)
	limitedQuery := fmt.Sprintf("SELECT * FROM (%s) _q LIMIT 0", rewritten)
	db := s.node.queryDB()
	rows, err := db.QueryContext(ctx, limitedQuery)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "schema query failed: %v", err)
	}
	defer rows.Close()
	schema, err := arrowSchemaFromRows(rows)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to derive schema: %v", err)
	}
	return &flight.SchemaResult{Schema: flight.SerializeSchema(schema, memory.DefaultAllocator)}, nil
}

func (s *fsqlServer) DoGetStatement(
	ctx context.Context, ticket flightsql.StatementQueryTicket,
) (*arrow.Schema, <-chan flight.StreamChunk, error) {
	var payload fsqlTicketPayload
	if err := json.Unmarshal(ticket.GetStatementHandle(), &payload); err != nil {
		return nil, nil, status.Errorf(codes.InvalidArgument, "invalid ticket: %v", err)
	}
	if payload.Query == "" {
		return nil, nil, status.Error(codes.InvalidArgument, "empty query in ticket")
	}
	query := s.prepareSQL(payload.Query)
	db := s.node.queryDB()
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, nil, status.Errorf(codes.Internal, "query failed: %v", err)
	}
	schema, err := arrowSchemaFromRows(rows)
	if err != nil {
		rows.Close()
		return nil, nil, status.Errorf(codes.Internal, "failed to build schema: %v", err)
	}
	ch := make(chan flight.StreamChunk, 4)
	go func() {
		defer close(ch)
		defer rows.Close()
		defer func() {
			if r := recover(); r != nil {
				logger.Error(fmt.Sprintf("FlightSQL: panic in stream goroutine: %v", r))
				ch <- flight.StreamChunk{Err: status.Errorf(codes.Internal, "internal error: %v", r)}
			}
		}()
		if err := streamRows(ctx, rows, schema, ch); err != nil {
			ch <- flight.StreamChunk{Err: err}
		}
	}()
	return schema, ch, nil
}

func (s *fsqlServer) prepareSQL(original string) string {
	rw := &sqlrewrite.Rewriter{LakeName: s.lakeName, DB: s.node.queryDB()}
	q, _ := rw.Rewrite(original)
	return s.node.prepareFlightSQLDataSQL(q)
}

func (s *fsqlServer) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.running {
		return fmt.Errorf("FlightSQL server already running")
	}
	addr := fmt.Sprintf("%s:%d", s.cfg.Host, s.cfg.Port)
	var opts []grpc.ServerOption
	if s.cfg.AuthToken != "" {
		opts = append(opts,
			grpc.ChainUnaryInterceptor(s.requireAuthUnary),
			grpc.ChainStreamInterceptor(s.requireAuthStream),
		)
	}
	s.grpcServer = grpc.NewServer(opts...)
	flight.RegisterFlightServiceServer(s.grpcServer, flightsql.NewFlightServer(s))
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("FlightSQL listen on %s: %w", addr, err)
	}
	s.listener = listener
	s.running = true
	go func() {
		auth := "disabled"
		if s.cfg.AuthToken != "" {
			auth = "bearer"
		}
		logger.Info(fmt.Sprintf("Node: Arrow FlightSQL (Grafana) listening on %s auth=%s", addr, auth))
		if err := s.grpcServer.Serve(listener); err != nil {
			logger.Error(fmt.Sprintf("Node: FlightSQL server error: %v", err))
		}
	}()
	if s.refreshFn != nil {
		intervalSec := s.cfg.CatalogRefreshIntervalSec
		if intervalSec <= 0 {
			intervalSec = 30
		}
		s.stopRefresh = make(chan struct{})
		go func() {
			ticker := time.NewTicker(time.Duration(intervalSec) * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					s.mu.Lock()
					if s.running {
						s.refreshFn()
					}
					s.mu.Unlock()
				case <-s.stopRefresh:
					return
				}
			}
		}()
		logger.Info(fmt.Sprintf("Node: FlightSQL catalog refresh enabled (interval=%ds)", intervalSec))
	}
	return nil
}

func (s *fsqlServer) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.running {
		return
	}
	if s.stopRefresh != nil {
		close(s.stopRefresh)
		s.stopRefresh = nil
	}
	grpcStopped := make(chan struct{})
	go func() {
		s.grpcServer.GracefulStop()
		close(grpcStopped)
	}()
	select {
	case <-grpcStopped:
	case <-time.After(5 * time.Second):
		logger.Warn("Node: FlightSQL gRPC did not stop gracefully in 5s, forcing stop")
		s.grpcServer.Stop()
	}
	s.running = false
	logger.Info("Node: Arrow FlightSQL server stopped")
}

func (s *fsqlServer) requireAuthUnary(
	ctx context.Context, req interface{}, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler,
) (interface{}, error) {
	if err := s.checkToken(ctx); err != nil {
		return nil, err
	}
	return handler(ctx, req)
}

func (s *fsqlServer) requireAuthStream(
	srv interface{}, ss grpc.ServerStream, _ *grpc.StreamServerInfo, handler grpc.StreamHandler,
) error {
	if err := s.checkToken(ss.Context()); err != nil {
		return err
	}
	return handler(srv, ss)
}

func (s *fsqlServer) checkToken(ctx context.Context) error {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return status.Error(codes.Unauthenticated, "missing metadata")
	}
	for _, v := range md.Get("authorization") {
		if strings.HasPrefix(v, "Bearer ") && strings.TrimPrefix(v, "Bearer ") == s.cfg.AuthToken {
			return nil
		}
	}
	return status.Error(codes.Unauthenticated, "invalid or missing Bearer token")
}
