// Copyright (C) 2025 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.
//

package metrics

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/sipcapture/homer-core/src/homerconfig"
	logger "github.com/sipcapture/homer-core/src/utils/logging"
)

// MetricsServerConfig holds modular config for metrics server
type MetricsServerConfig struct {
	Enable bool
	Host   string
	Port   int
	Path   string
}

// MetricsServer handles Prometheus metrics HTTP endpoint
type MetricsServer struct {
	server *http.Server
	config *MetricsServerConfig
}

// NewMetricsServer creates a new Prometheus metrics server (for legacy mode)
func NewMetricsServer() *MetricsServer {
	return &MetricsServer{}
}

// NewMetricsServerWithConfig creates a new Prometheus metrics server with explicit config
func NewMetricsServerWithConfig(cfg *MetricsServerConfig) *MetricsServer {
	return &MetricsServer{config: cfg}
}

// Start starts the Prometheus metrics HTTP server
func (ms *MetricsServer) Start() error {
	// Use explicit config if provided (modular mode)
	if ms.config != nil {
		return ms.startWithConfig()
	}

	// Legacy mode - use homerconfig
	if homerconfig.MainConfig == nil || !homerconfig.MainConfig.Setting.PROMETHEUS_SETTINGS.Enable {
		logger.Info("Prometheus metrics server is disabled")
		return nil
	}

	addr := fmt.Sprintf("%s:%d",
		homerconfig.MainConfig.Setting.PROMETHEUS_SETTINGS.Host,
		homerconfig.MainConfig.Setting.PROMETHEUS_SETTINGS.Port)

	path := homerconfig.MainConfig.Setting.PROMETHEUS_SETTINGS.Path
	if path == "" {
		path = "/metrics"
	}

	mux := http.NewServeMux()
	mux.Handle(path, promhttp.Handler())

	ms.server = &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	logger.Info("Prometheus metrics server starting", "addr", fmt.Sprintf("http://%s%s", addr, path))

	go func() {
		if err := ms.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error(fmt.Sprintf("Prometheus metrics server error: %v", err))
		}
	}()

	return nil
}

// startWithConfig starts with explicit config (modular mode)
func (ms *MetricsServer) startWithConfig() error {
	if !ms.config.Enable {
		logger.Info("Prometheus metrics server is disabled")
		return nil
	}

	addr := fmt.Sprintf("%s:%d", ms.config.Host, ms.config.Port)
	path := ms.config.Path
	if path == "" {
		path = "/metrics"
	}

	mux := http.NewServeMux()
	mux.Handle(path, promhttp.Handler())

	ms.server = &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	logger.Info("Prometheus metrics server starting", "addr", fmt.Sprintf("http://%s%s", addr, path))

	go func() {
		if err := ms.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error(fmt.Sprintf("Prometheus metrics server error: %v", err))
		}
	}()

	return nil
}

// Stop gracefully stops the metrics server
func (ms *MetricsServer) Stop(ctx context.Context) error {
	if ms.server == nil {
		return nil
	}

	logger.Info("Stopping Prometheus metrics server...")
	return ms.server.Shutdown(ctx)
}

