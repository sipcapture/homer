// Copyright (C) 2026 Homer Server Contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package node

import (
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/sipcapture/homer-core/src/config"
	logger "github.com/sipcapture/homer-core/src/utils/logging"
)

// ensureFlightAuthToken resolves and assigns node.flight_server.auth_token for
// exposed binds (GHSA-rm5w-rqr7-2h54). Safe to call when config already resolved
// the token during applyDefaults.
func ensureFlightAuthToken(cfg *config.NodeConfig) error {
	if cfg == nil {
		return nil
	}
	catalogPath := strings.TrimSpace(cfg.DuckLake.CatalogPath)
	if catalogPath == "" && len(cfg.DuckLake.Volumes) > 0 {
		catalogPath = strings.TrimSpace(cfg.DuckLake.Volumes[0].CatalogPath)
	}
	tok, autoGen, tokenFile, err := config.ResolveNodeFlightAuthToken(
		cfg.FlightServer.Host,
		cfg.FlightServer.AuthToken,
		catalogPath,
	)
	if err != nil {
		return err
	}
	cfg.FlightServer.AuthToken = tok
	if autoGen {
		logger.Warn("node: flight_server.auth_token was empty on a non-loopback bind; generated and persisted a Bearer token",
			"auth_token_file", tokenFile,
			"host", cfg.FlightServer.Host,
			"hint", "set node.flight_server.auth_token (and coordinator.nodes[].token) explicitly; prefer binding flight_server.host to 127.0.0.1 when the node is local-only")
	} else if tok == "" && !config.IsLoopbackBindHost(cfg.FlightServer.Host) {
		logger.Warn("node: HTTP /query has no auth_token on a non-loopback bind",
			"host", cfg.FlightServer.Host,
			"hint", "set node.flight_server.auth_token or bind flight_server.host to 127.0.0.1")
	} else if tok != "" {
		logger.Info("node: HTTP /query, /exec and /vacuum require Authorization Bearer token")
	}
	return nil
}

// ensureFlightSQLAuthToken always assigns a Bearer token when Arrow FlightSQL
// is enabled, including loopback binds (GHSA-w9hq-83jw-w7h9).
func ensureFlightSQLAuthToken(cfg *config.NodeConfig) error {
	if cfg == nil || !cfg.FlightSQLServer.Enable {
		return nil
	}
	catalogPath := strings.TrimSpace(cfg.DuckLake.CatalogPath)
	if catalogPath == "" && len(cfg.DuckLake.Volumes) > 0 {
		catalogPath = strings.TrimSpace(cfg.DuckLake.Volumes[0].CatalogPath)
	}
	configured := strings.TrimSpace(cfg.FlightSQLServer.AuthToken)
	if configured == "" {
		configured = strings.TrimSpace(cfg.FlightServer.AuthToken)
	}
	tok, autoGen, tokenFile, err := config.ResolveRequiredAuthToken(configured, catalogPath)
	if err != nil {
		return err
	}
	cfg.FlightSQLServer.AuthToken = tok
	if autoGen {
		logger.Warn("node: flightsql_server.auth_token was empty; generated and persisted a Bearer token",
			"auth_token_file", tokenFile,
			"host", cfg.FlightSQLServer.Host,
			"hint", "set node.flightsql_server.auth_token (Grafana FlightSQL) explicitly")
	} else if tok != "" {
		logger.Info("node: Arrow FlightSQL requires Authorization Bearer token")
	}
	return nil
}

// withBearerAuth wraps an HTTP handler so that when expectedToken is non-empty,
// requests must present Authorization: Bearer <token>. Empty expectedToken
// disables auth (loopback / trusted deployments).
func withBearerAuth(expectedToken string, next http.HandlerFunc) http.HandlerFunc {
	expectedToken = strings.TrimSpace(expectedToken)
	if expectedToken == "" {
		return next
	}
	want := []byte(expectedToken)
	return func(w http.ResponseWriter, r *http.Request) {
		got := bearerTokenFromRequest(r)
		if subtle.ConstantTimeCompare([]byte(got), want) != 1 {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

func bearerTokenFromRequest(r *http.Request) string {
	if r == nil {
		return ""
	}
	h := strings.TrimSpace(r.Header.Get("Authorization"))
	const prefix = "Bearer "
	if len(h) >= len(prefix) && strings.EqualFold(h[:len(prefix)], prefix) {
		return strings.TrimSpace(h[len(prefix):])
	}
	return ""
}
