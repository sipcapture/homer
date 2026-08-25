// Copyright (C) 2026 Homer Server Contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

// Package fsqlauth implements Arrow FlightSQL gRPC auth for Grafana / ADBC /
// GizmoSQL clients. Flight Handshake is allowed through without Bearer
// metadata (clients authenticate on subsequent RPCs, or send Basic/Bearer
// on Handshake itself).
package fsqlauth

import (
	"context"
	"crypto/subtle"
	"encoding/base64"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const (
	authorizationHeader = "authorization"
	authTokenBinHeader  = "auth-token-bin"
	bearerPrefix        = "bearer "
	basicPrefix         = "basic "
)

// ServerOptions returns gRPC interceptors that require expectedToken on every
// RPC except Flight Handshake. An empty token disables auth.
func ServerOptions(expectedToken string) []grpc.ServerOption {
	expectedToken = strings.TrimSpace(expectedToken)
	if expectedToken == "" {
		return nil
	}
	return []grpc.ServerOption{
		grpc.ChainUnaryInterceptor(unaryInterceptor(expectedToken)),
		grpc.ChainStreamInterceptor(streamInterceptor(expectedToken)),
	}
}

func unaryInterceptor(expected string) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		if err := CheckIncoming(ctx, expected); err != nil {
			return nil, err
		}
		return handler(ctx, req)
	}
}

func streamInterceptor(expected string) grpc.StreamServerInterceptor {
	return func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		// Grafana FlightSQL, GizmoSQL, and ADBC call Handshake before they
		// attach Authorization metadata. Rejecting it drops the connection
		// mid-handshake. Auth is enforced on GetFlightInfo / DoGet / etc.
		if strings.HasSuffix(info.FullMethod, "/Handshake") {
			return handler(srv, ss)
		}
		if err := CheckIncoming(ss.Context(), expected); err != nil {
			return err
		}
		return handler(srv, ss)
	}
}

// CheckIncoming validates Flight/gRPC auth metadata against expectedToken.
func CheckIncoming(ctx context.Context, expectedToken string) error {
	expectedToken = strings.TrimSpace(expectedToken)
	if expectedToken == "" {
		return nil
	}
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return status.Error(codes.Unauthenticated, "missing metadata")
	}
	for _, v := range md.Get(authorizationHeader) {
		if headerMatches(v, expectedToken) {
			return nil
		}
	}
	for _, v := range md.Get(authTokenBinHeader) {
		if tokenEq(expectedToken, v) {
			return nil
		}
	}
	return status.Error(codes.Unauthenticated, "invalid or missing Bearer token")
}

func headerMatches(header, expected string) bool {
	header = strings.TrimSpace(header)
	if header == "" {
		return false
	}
	lower := strings.ToLower(header)
	switch {
	case strings.HasPrefix(lower, bearerPrefix):
		return tokenEq(expected, strings.TrimSpace(header[len(bearerPrefix):]))
	case strings.HasPrefix(lower, basicPrefix):
		return basicMatches(strings.TrimSpace(header[len(basicPrefix):]), expected)
	default:
		// Grafana/ADBC sometimes send the raw token with no scheme.
		return tokenEq(expected, header)
	}
}

func basicMatches(b64, expected string) bool {
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		raw, err = base64.RawStdEncoding.DecodeString(b64)
		if err != nil {
			return false
		}
	}
	user, pass, ok := strings.Cut(string(raw), ":")
	if !ok {
		return tokenEq(expected, string(raw))
	}
	// Token as password (user ignored), as username, or either side.
	return tokenEq(expected, pass) || tokenEq(expected, user)
}

func tokenEq(expected, got string) bool {
	expected = strings.TrimSpace(expected)
	got = strings.TrimSpace(got)
	if expected == "" || got == "" {
		return false
	}
	if len(expected) != len(got) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(expected), []byte(got)) == 1
}
