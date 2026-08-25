// Copyright (C) 2026 Homer Server Contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package fsqlauth

import (
	"context"
	"encoding/base64"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func TestCheckIncomingBearer(t *testing.T) {
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs("authorization", "Bearer secret"))
	if err := CheckIncoming(ctx, "secret"); err != nil {
		t.Fatal(err)
	}
	if err := CheckIncoming(ctx, "other"); err == nil {
		t.Fatal("expected mismatch")
	}
}

func TestCheckIncomingBearerCaseInsensitive(t *testing.T) {
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs("authorization", "bearer secret"))
	if err := CheckIncoming(ctx, "secret"); err != nil {
		t.Fatal(err)
	}
}

func TestCheckIncomingRawToken(t *testing.T) {
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs("authorization", "secret"))
	if err := CheckIncoming(ctx, "secret"); err != nil {
		t.Fatal(err)
	}
}

func TestCheckIncomingBasicPassword(t *testing.T) {
	b64 := base64.StdEncoding.EncodeToString([]byte("grafana:secret"))
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs("authorization", "Basic "+b64))
	if err := CheckIncoming(ctx, "secret"); err != nil {
		t.Fatal(err)
	}
}

func TestCheckIncomingMissingMetadata(t *testing.T) {
	err := CheckIncoming(context.Background(), "secret")
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("got %v", err)
	}
}

func TestStreamInterceptorSkipsHandshake(t *testing.T) {
	called := false
	interceptor := streamInterceptor("secret")
	err := interceptor(nil, &stubStream{ctx: context.Background()}, &grpc.StreamServerInfo{
		FullMethod: "/arrow.flight.protocol.FlightService/Handshake",
	}, func(interface{}, grpc.ServerStream) error {
		called = true
		return nil
	})
	if err != nil || !called {
		t.Fatalf("handshake should pass without metadata: err=%v called=%v", err, called)
	}
}

func TestStreamInterceptorRequiresTokenOnDoGet(t *testing.T) {
	interceptor := streamInterceptor("secret")
	err := interceptor(nil, &stubStream{ctx: context.Background()}, &grpc.StreamServerInfo{
		FullMethod: "/arrow.flight.protocol.FlightService/DoGet",
	}, func(interface{}, grpc.ServerStream) error {
		t.Fatal("handler should not run")
		return nil
	})
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("got %v", err)
	}
}

type stubStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (s *stubStream) Context() context.Context { return s.ctx }
