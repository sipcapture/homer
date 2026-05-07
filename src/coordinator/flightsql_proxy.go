// Copyright (C) 2026 Homer Server Contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package coordinator

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
	"github.com/sipcapture/homer-core/src/config"
	logger "github.com/sipcapture/homer-core/src/utils/logging"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// flightSQLProxy is an Arrow FlightSQL gRPC server on the coordinator that
// proxies SQL to node flightsql_port endpoints (Grafana single entrypoint).
type flightSQLProxy struct {
	flightsql.BaseServer

	cfg        config.FlightSQLServerConfig
	nodes      []config.NodeEndpoint
	grpcServer *grpc.Server
	listener   net.Listener
	mu         sync.RWMutex
	running    bool
}

type flightSQLProxyTicket struct {
	Query string `json:"q"`
}

func newFlightSQLProxy(cfg config.FlightSQLServerConfig, nodes []config.NodeEndpoint) *flightSQLProxy {
	p := &flightSQLProxy{cfg: cfg, nodes: nodes}
	p.RegisterSqlInfo(flightsql.SqlInfoFlightSqlServerName, "homer-core-coordinator")
	p.RegisterSqlInfo(flightsql.SqlInfoFlightSqlServerVersion, "1.0")
	p.RegisterSqlInfo(flightsql.SqlInfoFlightSqlServerArrowVersion, "1.3")
	p.RegisterSqlInfo(flightsql.SqlInfoFlightSqlServerReadOnly, true)
	return p
}

func (p *flightSQLProxy) GetFlightInfoStatement(
	ctx context.Context, cmd flightsql.StatementQuery, desc *flight.FlightDescriptor,
) (*flight.FlightInfo, error) {
	query := cmd.GetQuery()
	if query == "" {
		return nil, status.Error(codes.InvalidArgument, "empty SQL query")
	}
	ticketBytes, err := json.Marshal(flightSQLProxyTicket{Query: query})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to encode ticket: %v", err)
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

func (p *flightSQLProxy) GetFlightInfoTables(
	_ context.Context, _ flightsql.GetTables, desc *flight.FlightDescriptor,
) (*flight.FlightInfo, error) {
	ticketBytes, err := json.Marshal(flightSQLProxyTicket{Query: "__tables__"})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "encode ticket: %v", err)
	}
	encodedTicket, err := flightsql.CreateStatementQueryTicket(ticketBytes)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "encode ticket: %v", err)
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

func (p *flightSQLProxy) DoGetTables(
	ctx context.Context, cmd flightsql.GetTables,
) (*arrow.Schema, <-chan flight.StreamChunk, error) {
	nodes := p.flightSQLNodes()
	if len(nodes) == 0 {
		return nil, nil, status.Error(codes.Unavailable, "no nodes with flightsql_port configured")
	}
	return p.streamFromNode(ctx, nodes[0], "SHOW ALL TABLES")
}

func (p *flightSQLProxy) DoGetStatement(
	ctx context.Context, ticket flightsql.StatementQueryTicket,
) (*arrow.Schema, <-chan flight.StreamChunk, error) {
	var payload flightSQLProxyTicket
	if err := json.Unmarshal(ticket.GetStatementHandle(), &payload); err != nil {
		return nil, nil, status.Errorf(codes.InvalidArgument, "invalid ticket: %v", err)
	}
	if payload.Query == "" {
		return nil, nil, status.Error(codes.InvalidArgument, "empty query in ticket")
	}
	query := payload.Query
	if query == "__tables__" {
		query = "SHOW ALL TABLES"
	}
	nodes := p.flightSQLNodes()
	if len(nodes) == 0 {
		return nil, nil, status.Error(codes.Unavailable, "no nodes with flightsql_port configured")
	}
	if len(nodes) == 1 {
		return p.streamFromNode(ctx, nodes[0], query)
	}
	return p.streamFromMultipleNodes(ctx, nodes, query)
}

func (p *flightSQLProxy) flightSQLNodes() []config.NodeEndpoint {
	var out []config.NodeEndpoint
	for _, n := range p.nodes {
		if n.FlightSQLPort > 0 {
			out = append(out, n)
		}
	}
	return out
}

func (p *flightSQLProxy) dialNode(_ context.Context, node config.NodeEndpoint) (*flightsql.Client, error) {
	addr := fmt.Sprintf("%s:%d", node.Host, node.FlightSQLPort)
	var opts []grpc.DialOption
	if node.UseTLS {
		// TODO: add TLS credentials when needed
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	} else {
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}
	client, err := flightsql.NewClient(addr, nil, nil, opts...)
	if err != nil {
		return nil, fmt.Errorf("dial node %s at %s: %w", node.Name, addr, err)
	}
	return client, err
}

func nodeOutgoingCtx(ctx context.Context, node config.NodeEndpoint) context.Context {
	if node.Token != "" {
		return metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+node.Token)
	}
	return ctx
}

func (p *flightSQLProxy) streamFromNode(
	ctx context.Context, node config.NodeEndpoint, query string,
) (*arrow.Schema, <-chan flight.StreamChunk, error) {
	client, err := p.dialNode(ctx, node)
	if err != nil {
		return nil, nil, status.Errorf(codes.Unavailable, "node %s: %v", node.Name, err)
	}
	nctx := nodeOutgoingCtx(ctx, node)
	info, err := client.Execute(nctx, query)
	if err != nil {
		client.Close()
		return nil, nil, status.Errorf(codes.Internal, "node %s execute: %v", node.Name, err)
	}
	if len(info.Endpoint) == 0 {
		client.Close()
		emptySchema := arrow.NewSchema([]arrow.Field{
			{Name: "result", Type: arrow.BinaryTypes.String, Nullable: true},
		}, nil)
		ch := make(chan flight.StreamChunk)
		close(ch)
		return emptySchema, ch, nil
	}
	reader, err := client.DoGet(nctx, info.Endpoint[0].Ticket)
	if err != nil {
		client.Close()
		return nil, nil, status.Errorf(codes.Internal, "node %s doget: %v", node.Name, err)
	}
	schema := reader.Schema()
	ch := make(chan flight.StreamChunk, 4)
	go func() {
		defer close(ch)
		defer reader.Release()
		defer client.Close()
		for reader.Next() {
			rec := reader.Record()
			rec.Retain()
			ch <- flight.StreamChunk{Data: rec}
		}
		if err := reader.Err(); err != nil {
			ch <- flight.StreamChunk{Err: err}
		}
	}()
	return schema, ch, nil
}

type proxyNodeReader struct {
	reader *flight.Reader
	client *flightsql.Client
	node   string
}

func (p *flightSQLProxy) streamFromMultipleNodes(
	ctx context.Context, nodes []config.NodeEndpoint, query string,
) (*arrow.Schema, <-chan flight.StreamChunk, error) {
	type readerResult struct {
		nr  *proxyNodeReader
		err error
	}
	results := make(chan readerResult, len(nodes))
	for _, node := range nodes {
		go func(n config.NodeEndpoint) {
			client, err := p.dialNode(ctx, n)
			if err != nil {
				logger.Error(fmt.Sprintf("Coordinator FlightSQL: node %s dial failed: %v", n.Name, err))
				results <- readerResult{err: err}
				return
			}
			nctx := nodeOutgoingCtx(ctx, n)
			info, err := client.Execute(nctx, query)
			if err != nil {
				client.Close()
				logger.Error(fmt.Sprintf("Coordinator FlightSQL: node %s execute failed: %v", n.Name, err))
				results <- readerResult{err: err}
				return
			}
			if len(info.Endpoint) == 0 {
				client.Close()
				results <- readerResult{err: fmt.Errorf("node %s: no endpoints", n.Name)}
				return
			}
			reader, err := client.DoGet(nctx, info.Endpoint[0].Ticket)
			if err != nil {
				client.Close()
				logger.Error(fmt.Sprintf("Coordinator FlightSQL: node %s doget failed: %v", n.Name, err))
				results <- readerResult{err: err}
				return
			}
			results <- readerResult{nr: &proxyNodeReader{reader: reader, client: client, node: n.Name}}
		}(node)
	}
	var readers []*proxyNodeReader
	for range nodes {
		r := <-results
		if r.err == nil && r.nr != nil {
			readers = append(readers, r.nr)
		}
	}
	if len(readers) == 0 {
		return nil, nil, status.Error(codes.Unavailable, "all nodes failed")
	}
	schema := readers[0].reader.Schema()
	ch := make(chan flight.StreamChunk, 4)
	go func() {
		defer close(ch)
		for _, nr := range readers {
			func() {
				defer nr.reader.Release()
				defer nr.client.Close()
				for nr.reader.Next() {
					rec := nr.reader.Record()
					rec.Retain()
					ch <- flight.StreamChunk{Data: rec}
				}
				if err := nr.reader.Err(); err != nil {
					logger.Warn(fmt.Sprintf("Coordinator FlightSQL: node %s stream error: %v", nr.node, err))
				}
			}()
		}
	}()
	return schema, ch, nil
}

func (p *flightSQLProxy) Start() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.running {
		return fmt.Errorf("FlightSQL proxy already running")
	}
	addr := fmt.Sprintf("%s:%d", p.cfg.Host, p.cfg.Port)
	var opts []grpc.ServerOption
	if p.cfg.AuthToken != "" {
		opts = append(opts,
			grpc.ChainUnaryInterceptor(p.requireAuthUnary),
			grpc.ChainStreamInterceptor(p.requireAuthStream),
		)
	}
	p.grpcServer = grpc.NewServer(opts...)
	flight.RegisterFlightServiceServer(p.grpcServer, flightsql.NewFlightServer(p))
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("FlightSQL proxy listen on %s: %w", addr, err)
	}
	p.listener = listener
	p.running = true
	fsqlNodes := p.flightSQLNodes()
	go func() {
		auth := "disabled"
		if p.cfg.AuthToken != "" {
			auth = "bearer"
		}
		logger.Info(fmt.Sprintf("Coordinator: Arrow FlightSQL proxy on %s auth=%s backend_nodes=%d", addr, auth, len(fsqlNodes)))
		if err := p.grpcServer.Serve(listener); err != nil {
			logger.Error(fmt.Sprintf("Coordinator: FlightSQL proxy error: %v", err))
		}
	}()
	return nil
}

func (p *flightSQLProxy) Stop() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.running {
		return
	}
	stopped := make(chan struct{})
	go func() {
		p.grpcServer.GracefulStop()
		close(stopped)
	}()
	select {
	case <-stopped:
	case <-time.After(5 * time.Second):
		logger.Warn("Coordinator: FlightSQL proxy gRPC did not stop in 5s, forcing")
		p.grpcServer.Stop()
	}
	p.running = false
	logger.Info("Coordinator: FlightSQL proxy stopped")
}

func (p *flightSQLProxy) requireAuthUnary(
	ctx context.Context, req interface{}, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler,
) (interface{}, error) {
	if err := p.checkToken(ctx); err != nil {
		return nil, err
	}
	return handler(ctx, req)
}

func (p *flightSQLProxy) requireAuthStream(
	srv interface{}, ss grpc.ServerStream, _ *grpc.StreamServerInfo, handler grpc.StreamHandler,
) error {
	if err := p.checkToken(ss.Context()); err != nil {
		return err
	}
	return handler(srv, ss)
}

func (p *flightSQLProxy) checkToken(ctx context.Context) error {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return status.Error(codes.Unauthenticated, "missing metadata")
	}
	for _, v := range md.Get("authorization") {
		if strings.HasPrefix(v, "Bearer ") && strings.TrimPrefix(v, "Bearer ") == p.cfg.AuthToken {
			return nil
		}
	}
	return status.Error(codes.Unauthenticated, "invalid or missing Bearer token")
}
