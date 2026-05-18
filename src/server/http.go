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

package input

import (
	"fmt"
	"sync/atomic"
	"time"

	"github.com/fasthttp/websocket"
	"github.com/sipcapture/homer-core/src/homerconfig"
	logger "github.com/sipcapture/homer-core/src/utils/logging"
	"github.com/sipcapture/homer-core/src/utils/metrics"
	"github.com/valyala/fasthttp"
)

var upgrader = websocket.FastHTTPUpgrader{
	CheckOrigin: func(ctx *fasthttp.RequestCtx) bool {
		// Allow all origins - can be restricted in production
		return true
	},
	ReadBufferSize:  65536,
	WriteBufferSize: 65536,
}

// HTTPServer handles HTTP requests for HEP packet reception using fasthttp
type HTTPServer struct {
	hepInput    *HEPInput
	server      *fasthttp.Server
	stopped     uint32
	recvHTTP    ingestReceiveMetrics
	recvWS      ingestReceiveMetrics
}

// NewHTTPServer creates a new HTTP server for HEP packet reception
func NewHTTPServer(hepInput *HEPInput) *HTTPServer {
	hs := &HTTPServer{
		hepInput: hepInput,
		recvHTTP: newIngestReceiveMetrics("http"),
		recvWS:   newIngestReceiveMetrics("websocket"),
	}

	cfg := homerconfig.MainConfig.Setting.HTTP_SETTINGS

	hs.server = &fasthttp.Server{
		Handler:            hs.handleRequest,
		ReadTimeout:        time.Duration(cfg.ReadTimeout) * time.Second,
		WriteTimeout:       time.Duration(cfg.WriteTimeout) * time.Second,
		IdleTimeout:        time.Duration(cfg.IdleTimeout) * time.Second,
		MaxRequestBodySize: cfg.MaxRequestBodySize,
		Concurrency:        cfg.Concurrency,
		DisableKeepalive:   cfg.DisableKeepalive,
		TCPKeepalive:       cfg.TCPKeepalive,
		TCPKeepalivePeriod: time.Duration(cfg.TCPKeepalivePeriod) * time.Second,
	}

	return hs
}

// handleRequest routes requests to appropriate handlers
func (hs *HTTPServer) handleRequest(ctx *fasthttp.RequestCtx) {
	path := string(ctx.Path())
	method := string(ctx.Method())

	cfg := homerconfig.MainConfig.Setting.HTTP_SETTINGS

	// Build full paths with api_prefix
	apiPrefix := cfg.ApiPrefix
	healthPath := apiPrefix + cfg.Endpoints.Health
	hepPath := apiPrefix + cfg.Endpoints.HEP
	hepBinaryPath := apiPrefix + cfg.Endpoints.HEPBinary
	hepProtobufPath := apiPrefix + cfg.Endpoints.HEPProtobuf
	wsPath := apiPrefix + cfg.WebSocket.Path

	// Health check endpoint
	if method == "GET" && path == healthPath {
		hs.handleHealth(ctx)
		return
	}

	// WebSocket endpoint
	if method == "GET" && path == wsPath {
		if cfg.WebSocket.Enable {
			hs.handleWebSocket(ctx)
		} else {
			ctx.Error("WebSocket is disabled", fasthttp.StatusForbidden)
		}
		return
	}

	// HEP packet endpoints
	if method == "POST" {
		switch path {
		case hepPath:
			hs.handleHEPPacket(ctx)
		case hepBinaryPath:
			hs.handleHEPBinary(ctx)
		case hepProtobufPath:
			hs.handleHEPProtobuf(ctx)
		default:
			ctx.Error("Not Found", fasthttp.StatusNotFound)
		}
		return
	}

	ctx.Error("Method Not Allowed", fasthttp.StatusMethodNotAllowed)
}

// handleHEPPacket handles HEP packets in any format (auto-detect)
func (hs *HTTPServer) handleHEPPacket(ctx *fasthttp.RequestCtx) {
	if atomic.LoadUint32(&hs.stopped) == 1 {
		ctx.Error("Service Unavailable", fasthttp.StatusServiceUnavailable)
		return
	}

	body := ctx.PostBody()
	if len(body) == 0 {
		ctx.Error("Empty request body", fasthttp.StatusBadRequest)
		return
	}
	if len(body) >= 4 && string(body[0:4]) == "HEP3" {
		logger.Debug("HTTP: received HEP3 binary packet", "size", len(body))
	} else if len(body) >= 1 && (body[0] == 0x01 || body[0] == 0x02) {
		logger.Debug("HTTP: received HEP2 binary packet", "size", len(body))
	} else {
		logger.Debug("HTTP: received protobuf packet", "size", len(body))
	}
	handleIngestPOST(hs.hepInput, ctx, "http", &hs.recvHTTP)
}

// handleHEPBinary handles HEP packets in binary format
func (hs *HTTPServer) handleHEPBinary(ctx *fasthttp.RequestCtx) {
	if atomic.LoadUint32(&hs.stopped) == 1 {
		ctx.Error("Service Unavailable", fasthttp.StatusServiceUnavailable)
		return
	}

	handleIngestPOST(hs.hepInput, ctx, "http", &hs.recvHTTP)
}

// handleHEPProtobuf handles HEP packets in protobuf format
func (hs *HTTPServer) handleHEPProtobuf(ctx *fasthttp.RequestCtx) {
	if atomic.LoadUint32(&hs.stopped) == 1 {
		ctx.Error("Service Unavailable", fasthttp.StatusServiceUnavailable)
		return
	}

	handleIngestPOST(hs.hepInput, ctx, "http", &hs.recvHTTP)
}

// handleWebSocket handles WebSocket connections for HEP packet reception
func (hs *HTTPServer) handleWebSocket(ctx *fasthttp.RequestCtx) {
	if atomic.LoadUint32(&hs.stopped) == 1 {
		ctx.Error("Service Unavailable", fasthttp.StatusServiceUnavailable)
		return
	}

	err := upgrader.Upgrade(ctx, func(conn *websocket.Conn) {
		defer conn.Close()

		logger.Info("WebSocket: new connection", "remote", conn.RemoteAddr())
		metrics.IncrementActiveConnections("websocket")

		// Set read deadline
		conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		conn.SetPongHandler(func(string) error {
			conn.SetReadDeadline(time.Now().Add(60 * time.Second))
			return nil
		})

		// Start ping ticker
		ticker := time.NewTicker(54 * time.Second)
		defer ticker.Stop()

		go func() {
			for range ticker.C {
				if atomic.LoadUint32(&hs.stopped) == 1 {
					return
				}
				if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
					return
				}
			}
		}()

		// Read messages
		for {
			if atomic.LoadUint32(&hs.stopped) == 1 {
				break
			}

			messageType, message, err := conn.ReadMessage()
			if err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
					logger.Error(fmt.Sprintf("WebSocket: read error: %v", err))
				}
				break
			}

			if messageType == websocket.BinaryMessage || messageType == websocket.TextMessage {
				if len(message) == 0 {
					continue
				}

				packet := copyPacketToPool(hs.hepInput, message)
				if !enqueueHTTPPacket(hs.hepInput, "websocket", packet, &hs.recvWS) {
					break
				}
			}
		}

		logger.Info("WebSocket: connection closed", "remote", conn.RemoteAddr())
		metrics.DecrementActiveConnections("websocket")
	})

	if err != nil {
		logger.Error(fmt.Sprintf("WebSocket: failed to upgrade connection: %v", err))
		ctx.Error("Internal Server Error", fasthttp.StatusInternalServerError)
	}
}

// handleHealth handles health check requests
func (hs *HTTPServer) handleHealth(ctx *fasthttp.RequestCtx) {
	ctx.SetContentType("application/json")
	ctx.SetStatusCode(fasthttp.StatusOK)
	ctx.SetBodyString(`{"status":"ok","service":"homer-core"}`)
}

// Run starts the HTTP server
func (hs *HTTPServer) Run() {
	if !homerconfig.MainConfig.Setting.SERVER_SETTINGS.HTTP_SERVER.Enable {
		logger.Info("HTTP server is disabled")
		return
	}

	host := homerconfig.MainConfig.Setting.SERVER_SETTINGS.HTTP_SERVER.Host
	port := homerconfig.MainConfig.Setting.SERVER_SETTINGS.HTTP_SERVER.Port
	addr := fmt.Sprintf("%s:%d", host, port)

	logger.Info("HTTP server for HEP packets starting", "addr", fmt.Sprintf("http://%s", addr))

	if err := hs.server.ListenAndServe(addr); err != nil {
		logger.Error(fmt.Sprintf("HTTP server error: %v", err))
	}
}

// End stops the HTTP server
func (hs *HTTPServer) End() {
	atomic.StoreUint32(&hs.stopped, 1)
	if hs.server != nil {
		logger.Info("Stopping HTTP server...")
		hs.server.Shutdown()
	}
}
