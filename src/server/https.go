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
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sync/atomic"
	"time"

	"github.com/fasthttp/websocket"
	"github.com/sipcapture/homer-core/src/homerconfig"
	logger "github.com/sipcapture/homer-core/src/utils/logging"
	"github.com/sipcapture/homer-core/src/utils/metrics"
	"github.com/valyala/fasthttp"
)

var httpsUpgrader = websocket.FastHTTPUpgrader{
	CheckOrigin: func(ctx *fasthttp.RequestCtx) bool {
		// Allow all origins - can be restricted in production
		return true
	},
	ReadBufferSize:  65536,
	WriteBufferSize: 65536,
}

// HTTPSServer handles HTTPS requests for HEP packet reception using fasthttp
type HTTPSServer struct {
	hepInput *HEPInput
	server   *fasthttp.Server
	tlsConfig *tls.Config
	stopped  uint32
}

// NewHTTPSServer creates a new HTTPS server for HEP packet reception
func NewHTTPSServer(hepInput *HEPInput) *HTTPSServer {
	hs := &HTTPSServer{
		hepInput: hepInput,
	}

	// Use new location (SERVER_SETTINGS.HTTPS_SERVER)
	cfg := &homerconfig.MainConfig.Setting.SERVER_SETTINGS.HTTPS_SERVER

	// Load TLS configuration directly (inline to avoid type mismatch)
	tlsConfig, err := loadHTTPSTLSConfigInline(cfg)
	if err != nil {
		logger.Error(fmt.Sprintf("Failed to load HTTPS TLS config: %v", err))
		return nil
	}
	hs.tlsConfig = tlsConfig

	// Get performance settings from HTTP_SETTINGS
	httpCfg := homerconfig.MainConfig.Setting.HTTP_SETTINGS
	readTimeout := httpCfg.ReadTimeout
	writeTimeout := httpCfg.WriteTimeout
	idleTimeout := httpCfg.IdleTimeout
	maxRequestBodySize := httpCfg.MaxRequestBodySize
	concurrency := httpCfg.Concurrency
	disableKeepalive := httpCfg.DisableKeepalive
	tcpKeepalive := httpCfg.TCPKeepalive
	tcpKeepalivePeriod := httpCfg.TCPKeepalivePeriod

	hs.server = &fasthttp.Server{
		Handler:            hs.handleRequest,
		ReadTimeout:        time.Duration(readTimeout) * time.Second,
		WriteTimeout:       time.Duration(writeTimeout) * time.Second,
		IdleTimeout:        time.Duration(idleTimeout) * time.Second,
		MaxRequestBodySize: maxRequestBodySize,
		Concurrency:        concurrency,
		DisableKeepalive:   disableKeepalive,
		TCPKeepalive:       tcpKeepalive,
		TCPKeepalivePeriod: time.Duration(tcpKeepalivePeriod) * time.Second,
		TLSConfig:          tlsConfig,
	}

	return hs
}

// loadHTTPSTLSConfigInline loads TLS configuration from HTTPS settings using reflection
func loadHTTPSTLSConfigInline(cfgPtr interface{}) (*tls.Config, error) {
	cfgValue := reflect.ValueOf(cfgPtr).Elem()
	
	enable := cfgValue.FieldByName("Enable").Bool()
	if !enable {
		return nil, nil
	}
	
	cert := cfgValue.FieldByName("Cert").String()
	key := cfgValue.FieldByName("Key").String()
	caCert := cfgValue.FieldByName("CaCert").String()
	minTLSVersion := uint16(cfgValue.FieldByName("MinTLSVersionNum").Uint())
	maxTLSVersion := uint16(cfgValue.FieldByName("MaxTLSVersionNum").Uint())
	preferServerCipher := cfgValue.FieldByName("PreferServerCipher").Bool()
	mutualTLS := cfgValue.FieldByName("MutualTLS").Bool()
	insecureSkipVerify := cfgValue.FieldByName("InsecureSkipVerify").Bool()

	if cert == "" || key == "" {
		return nil, fmt.Errorf("HTTPS certificate and key must be provided (cert=%q, key=%q)", cert, key)
	}

	cwd, _ := os.Getwd()
	logger.Info("HTTPS server: current working directory", "cwd", cwd)

	logger.Info("HTTPS server: checking certificate file", "cert", cert)
	if _, err := os.Stat(cert); os.IsNotExist(err) {
		absPath, _ := filepath.Abs(cert)
		return nil, fmt.Errorf("HTTPS certificate file not found: %s (absolute: %s)", cert, absPath)
	}
	logger.Info("HTTPS server: certificate file found", "cert", cert)

	logger.Info("HTTPS server: checking key file", "key", key)
	if _, err := os.Stat(key); os.IsNotExist(err) {
		absPath, _ := filepath.Abs(key)
		return nil, fmt.Errorf("HTTPS key file not found: %s (absolute: %s)", key, absPath)
	}
	logger.Info("HTTPS server: key file found", "key", key)

	logger.Info("HTTPS server: loading certificate pair...")
	tlsCert, err := tls.LoadX509KeyPair(cert, key)
	if err != nil {
		return nil, fmt.Errorf("failed to load HTTPS certificate from cert=%s key=%s: %w", cert, key, err)
	}
	logger.Info("HTTPS server: certificate pair loaded successfully")

	config := &tls.Config{
		Certificates: []tls.Certificate{tlsCert},
		MinVersion:   minTLSVersion,
		MaxVersion:   maxTLSVersion,
		PreferServerCipherSuites: preferServerCipher,
	}

	if mutualTLS && caCert != "" {
		logger.Info("HTTPS server: checking CA certificate file", "cacert", caCert)
		if _, err := os.Stat(caCert); os.IsNotExist(err) {
			absPath, _ := filepath.Abs(caCert)
			return nil, fmt.Errorf("HTTPS CA certificate file not found: %s (absolute: %s)", caCert, absPath)
		}
		logger.Info("HTTPS server: CA certificate file found", "cacert", caCert)

		caCertData, err := os.ReadFile(caCert)
		if err != nil {
			return nil, fmt.Errorf("failed to load CA certificate: %w", err)
		}

		caCertPool := x509.NewCertPool()
		if !caCertPool.AppendCertsFromPEM(caCertData) {
			return nil, fmt.Errorf("failed to parse CA certificate")
		}

		config.ClientCAs = caCertPool
		config.ClientAuth = tls.RequireAndVerifyClientCert
		logger.Info("HTTPS server: Mutual TLS enabled", "cacert", caCert)
	} else if mutualTLS && caCert == "" {
		return nil, fmt.Errorf("Mutual TLS is enabled but CA certificate (cacert) is not provided")
	}

	config.SessionTicketsDisabled = false
	config.CipherSuites = []uint16{
		tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
		tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
		tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
		tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
		tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,
		tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305,
	}

	if insecureSkipVerify {
		config.InsecureSkipVerify = true
		logger.Warn("HTTPS server: InsecureSkipVerify is enabled. This is not recommended for production.")
	}

	logger.Info("HTTPS server: TLS config loaded successfully")
	return config, nil
}

// loadHTTPSTLSConfig loads TLS configuration from HTTPS settings (deprecated)
func loadHTTPSTLSConfig(cfg *struct {
	Enable             bool
	Host               string
	Port               int
	Cert               string
	CaCert             string
	Key                string
	HttpRedirect       bool
	MinTLSVersionString string
	MaxTLSVersionString string
	MinTLSVersion      uint16
	MaxTLSVersion      uint16
	PreferServerCipher bool
	MutualTLS          bool
	InsecureSkipVerify bool
	ReadTimeout        int
	WriteTimeout       int
	IdleTimeout        int
	MaxRequestBodySize int
	Concurrency        int
	DisableKeepalive   bool
	TCPKeepalive       bool
	TCPKeepalivePeriod int
}) (*tls.Config, error) {
	if !cfg.Enable {
		return nil, nil
	}

	if cfg.Cert == "" || cfg.Key == "" {
		return nil, fmt.Errorf("HTTPS certificate and key must be provided (cert=%q, key=%q)", cfg.Cert, cfg.Key)
	}

	cwd, _ := os.Getwd()
	logger.Info("HTTPS server: current working directory", "cwd", cwd)

	// Check if certificate files exist
	logger.Info("HTTPS server: checking certificate file", "cert", cfg.Cert)
	if _, err := os.Stat(cfg.Cert); os.IsNotExist(err) {
		absPath, _ := filepath.Abs(cfg.Cert)
		return nil, fmt.Errorf("HTTPS certificate file not found: %s (absolute: %s)", cfg.Cert, absPath)
	}
	logger.Info("HTTPS server: certificate file found", "cert", cfg.Cert)

	logger.Info("HTTPS server: checking key file", "key", cfg.Key)
	if _, err := os.Stat(cfg.Key); os.IsNotExist(err) {
		absPath, _ := filepath.Abs(cfg.Key)
		return nil, fmt.Errorf("HTTPS key file not found: %s (absolute: %s)", cfg.Key, absPath)
	}
	logger.Info("HTTPS server: key file found", "key", cfg.Key)

	logger.Info("HTTPS server: loading certificate pair...")
	// Load certificate
	cert, err := tls.LoadX509KeyPair(cfg.Cert, cfg.Key)
	if err != nil {
		return nil, fmt.Errorf("failed to load HTTPS certificate from cert=%s key=%s: %w", cfg.Cert, cfg.Key, err)
	}
	logger.Info("HTTPS server: certificate pair loaded successfully")

	config := &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   cfg.MinTLSVersion,
		MaxVersion:   cfg.MaxTLSVersion,
		PreferServerCipherSuites: cfg.PreferServerCipher,
	}

	// Load CA certificate for mutual TLS
	if cfg.MutualTLS && cfg.CaCert != "" {
		logger.Info("HTTPS server: checking CA certificate file", "cacert", cfg.CaCert)
		if _, err := os.Stat(cfg.CaCert); os.IsNotExist(err) {
			absPath, _ := filepath.Abs(cfg.CaCert)
			return nil, fmt.Errorf("HTTPS CA certificate file not found: %s (absolute: %s)", cfg.CaCert, absPath)
		}
		logger.Info("HTTPS server: CA certificate file found", "cacert", cfg.CaCert)

		caCert, err := os.ReadFile(cfg.CaCert)
		if err != nil {
			return nil, fmt.Errorf("failed to load CA certificate: %w", err)
		}

		caCertPool := x509.NewCertPool()
		if !caCertPool.AppendCertsFromPEM(caCert) {
			return nil, fmt.Errorf("failed to parse CA certificate")
		}

		config.ClientCAs = caCertPool
		config.ClientAuth = tls.RequireAndVerifyClientCert
		logger.Info("HTTPS server: Mutual TLS enabled", "cacert", cfg.CaCert)
	} else if cfg.MutualTLS && cfg.CaCert == "" {
		return nil, fmt.Errorf("Mutual TLS is enabled but CA certificate (cacert) is not provided")
	}

	// TLS optimizations for performance
	config.SessionTicketsDisabled = false // Enable session tickets for better performance
	config.CipherSuites = []uint16{
		tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
		tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
		tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
		tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
		tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,
		tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305,
	}

	if cfg.InsecureSkipVerify {
		config.InsecureSkipVerify = true
		logger.Warn("HTTPS server: InsecureSkipVerify is enabled. This is not recommended for production.")
	}

	logger.Info("HTTPS server: TLS config loaded successfully")
	return config, nil
}

// handleRequest routes requests to appropriate handlers
func (hs *HTTPSServer) handleRequest(ctx *fasthttp.RequestCtx) {
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
func (hs *HTTPSServer) handleHEPPacket(ctx *fasthttp.RequestCtx) {
	if atomic.LoadUint32(&hs.stopped) == 1 {
		ctx.Error("Service Unavailable", fasthttp.StatusServiceUnavailable)
		return
	}

	body := ctx.PostBody()
	if len(body) == 0 {
		ctx.Error("Empty request body", fasthttp.StatusBadRequest)
		return
	}

	// Copy body to avoid referencing fasthttp's internal buffer
	packet := make([]byte, len(body))
	copy(packet, body)

	// Auto-detect format
	if len(packet) >= 4 && string(packet[0:4]) == "HEP3" {
		logger.Debug("HTTPS: received HEP3 binary packet", "size", len(packet))
	} else if len(packet) >= 1 && (packet[0] == 0x01 || packet[0] == 0x02) {
		logger.Debug("HTTPS: received HEP2 binary packet", "size", len(packet))
	} else {
		logger.Debug("HTTPS: received protobuf packet", "size", len(packet))
	}

	// Record metrics
	metrics.RecordHEPPacketReceived("https")
	metrics.RecordHEPPacketSize("https", len(packet))
	metrics.RecordBytesReceived("https", int64(len(packet)))

	// Send to input channel (non-blocking)
	select {
	case hs.hepInput.inputCh <- incomingPacket{
		data:       packet,
		protocol:   "https",
		receivedAt: time.Now(),
	}:
		atomic.AddUint64(&hs.hepInput.stats.PktCount, 1)
		ctx.SetStatusCode(fasthttp.StatusOK)
		ctx.SetBodyString("OK")
	default:
		if atomic.LoadUint32(&hs.stopped) == 1 {
			ctx.Error("Service Unavailable", fasthttp.StatusServiceUnavailable)
			return
		}
		hs.hepInput.inputCh <- incomingPacket{
			data:       packet,
			protocol:   "https",
			receivedAt: time.Now(),
		}
		atomic.AddUint64(&hs.hepInput.stats.PktCount, 1)
		ctx.SetStatusCode(fasthttp.StatusOK)
		ctx.SetBodyString("OK")
	}
}

// handleHEPBinary handles HEP packets in binary format
func (hs *HTTPSServer) handleHEPBinary(ctx *fasthttp.RequestCtx) {
	if atomic.LoadUint32(&hs.stopped) == 1 {
		ctx.Error("Service Unavailable", fasthttp.StatusServiceUnavailable)
		return
	}

	body := ctx.PostBody()
	if len(body) == 0 {
		ctx.Error("Empty request body", fasthttp.StatusBadRequest)
		return
	}

	packet := make([]byte, len(body))
	copy(packet, body)

	metrics.RecordHEPPacketReceived("https")
	metrics.RecordHEPPacketSize("https", len(packet))
	metrics.RecordBytesReceived("https", int64(len(packet)))

	select {
	case hs.hepInput.inputCh <- incomingPacket{
		data:       packet,
		protocol:   "https",
		receivedAt: time.Now(),
	}:
		atomic.AddUint64(&hs.hepInput.stats.PktCount, 1)
		ctx.SetStatusCode(fasthttp.StatusOK)
		ctx.SetBodyString("OK")
	default:
		if atomic.LoadUint32(&hs.stopped) == 1 {
			ctx.Error("Service Unavailable", fasthttp.StatusServiceUnavailable)
			return
		}
		hs.hepInput.inputCh <- incomingPacket{
			data:       packet,
			protocol:   "https",
			receivedAt: time.Now(),
		}
		atomic.AddUint64(&hs.hepInput.stats.PktCount, 1)
		ctx.SetStatusCode(fasthttp.StatusOK)
		ctx.SetBodyString("OK")
	}
}

// handleHEPProtobuf handles HEP packets in protobuf format
func (hs *HTTPSServer) handleHEPProtobuf(ctx *fasthttp.RequestCtx) {
	if atomic.LoadUint32(&hs.stopped) == 1 {
		ctx.Error("Service Unavailable", fasthttp.StatusServiceUnavailable)
		return
	}

	body := ctx.PostBody()
	if len(body) == 0 {
		ctx.Error("Empty request body", fasthttp.StatusBadRequest)
		return
	}

	packet := make([]byte, len(body))
	copy(packet, body)

	metrics.RecordHEPPacketReceived("https")
	metrics.RecordHEPPacketSize("https", len(packet))
	metrics.RecordBytesReceived("https", int64(len(packet)))

	select {
	case hs.hepInput.inputCh <- incomingPacket{
		data:       packet,
		protocol:   "https",
		receivedAt: time.Now(),
	}:
		atomic.AddUint64(&hs.hepInput.stats.PktCount, 1)
		ctx.SetStatusCode(fasthttp.StatusOK)
		ctx.SetBodyString("OK")
	default:
		if atomic.LoadUint32(&hs.stopped) == 1 {
			ctx.Error("Service Unavailable", fasthttp.StatusServiceUnavailable)
			return
		}
		hs.hepInput.inputCh <- incomingPacket{
			data:       packet,
			protocol:   "https",
			receivedAt: time.Now(),
		}
		atomic.AddUint64(&hs.hepInput.stats.PktCount, 1)
		ctx.SetStatusCode(fasthttp.StatusOK)
		ctx.SetBodyString("OK")
	}
}

// handleWebSocket handles WebSocket connections for HEP packet reception
func (hs *HTTPSServer) handleWebSocket(ctx *fasthttp.RequestCtx) {
	if atomic.LoadUint32(&hs.stopped) == 1 {
		ctx.Error("Service Unavailable", fasthttp.StatusServiceUnavailable)
		return
	}

	err := httpsUpgrader.Upgrade(ctx, func(conn *websocket.Conn) {
		defer conn.Close()

		logger.Info("HTTPS WebSocket: new connection", "remote", conn.RemoteAddr())
		metrics.IncrementActiveConnections("websocket")

		conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		conn.SetPongHandler(func(string) error {
			conn.SetReadDeadline(time.Now().Add(60 * time.Second))
			return nil
		})

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

		for {
			if atomic.LoadUint32(&hs.stopped) == 1 {
				break
			}

			messageType, message, err := conn.ReadMessage()
			if err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
					logger.Error(fmt.Sprintf("HTTPS WebSocket: read error: %v", err))
				}
				break
			}

			if messageType == websocket.BinaryMessage || messageType == websocket.TextMessage {
				if len(message) == 0 {
					continue
				}

				metrics.RecordHEPPacketReceived("websocket")
				metrics.RecordHEPPacketSize("websocket", len(message))
				metrics.RecordBytesReceived("websocket", int64(len(message)))

				select {
				case hs.hepInput.inputCh <- incomingPacket{
					data:       message,
					protocol:   "wss",
					receivedAt: time.Now(),
				}:
					atomic.AddUint64(&hs.hepInput.stats.PktCount, 1)
				default:
					if atomic.LoadUint32(&hs.stopped) == 1 {
						break
					}
					hs.hepInput.inputCh <- incomingPacket{
						data:       message,
						protocol:   "wss",
						receivedAt: time.Now(),
					}
					atomic.AddUint64(&hs.hepInput.stats.PktCount, 1)
				}
			}
		}

		logger.Info("HTTPS WebSocket: connection closed", "remote", conn.RemoteAddr())
		metrics.DecrementActiveConnections("websocket")
	})

	if err != nil {
		logger.Error(fmt.Sprintf("HTTPS WebSocket: failed to upgrade connection: %v", err))
		ctx.Error("Internal Server Error", fasthttp.StatusInternalServerError)
	}
}

// handleHealth handles health check requests
func (hs *HTTPSServer) handleHealth(ctx *fasthttp.RequestCtx) {
	ctx.SetContentType("application/json")
	ctx.SetStatusCode(fasthttp.StatusOK)
	ctx.SetBodyString(`{"status":"ok","service":"homer-core"}`)
}

// Run starts the HTTPS server
func (hs *HTTPSServer) Run() {
	// Check if HTTPS is enabled
	if !homerconfig.MainConfig.Setting.SERVER_SETTINGS.HTTPS_SERVER.Enable {
		logger.Info("HTTPS server is disabled")
		return
	}

	if hs.tlsConfig == nil {
		logger.Error("HTTPS server: TLS config is nil, cannot start")
		return
	}

	// Get config from SERVER_SETTINGS.HTTPS_SERVER
	host := homerconfig.MainConfig.Setting.SERVER_SETTINGS.HTTPS_SERVER.Host
	port := homerconfig.MainConfig.Setting.SERVER_SETTINGS.HTTPS_SERVER.Port
	cert := homerconfig.MainConfig.Setting.SERVER_SETTINGS.HTTPS_SERVER.Cert
	key := homerconfig.MainConfig.Setting.SERVER_SETTINGS.HTTPS_SERVER.Key
	
	addr := host
	if addr == "" || addr == "0.0.0.0" {
		addr = ""
	} else {
		addr += ":"
	}
	addr += fmt.Sprintf("%d", port)

	logger.Info("HTTPS server for HEP packets starting", "addr", fmt.Sprintf("https://%s", addr))

	if err := hs.server.ListenAndServeTLS(addr, cert, key); err != nil {
		logger.Error(fmt.Sprintf("HTTPS server error: %v", err))
	}
}

// End stops the HTTPS server
func (hs *HTTPSServer) End() {
	atomic.StoreUint32(&hs.stopped, 1)
	if hs.server != nil {
		logger.Info("Stopping HTTPS server...")
		hs.server.Shutdown()
	}
}

