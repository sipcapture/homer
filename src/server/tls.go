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
	"context"
	"crypto/x509"
	"encoding/binary"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	gtls "github.com/gnet-io/tls"
	"github.com/panjf2000/gnet/v2"
	logger "github.com/sipcapture/homer-core/src/utils/logging"
	"github.com/sipcapture/homer-core/src/utils/metrics"
)

const (
	// TLS batch size for packet processing
	tlsBatchSize = 64
)

// tlsServer implements gnet.EventHandler for high-performance TLS server
type tlsServer struct {
	gnet.BuiltinEventEngine
	hepInput   *HEPInput
	eng        gnet.Engine
	multicore  bool
	serverAddr string
	tlsConfig   *gtls.Config
	recvMetrics ingestReceiveMetrics
	// Per-connection batch buffer for efficient packet batching
	batchBuffers sync.Pool
	// TLS connections map: gnet.Conn -> gtls.Conn
	tlsConns    sync.Map
	engineReady chan gnet.Engine
}

// TLSSettings represents TLS configuration
type TLSSettings struct {
	Enable             bool
	Port               int
	Cert               string
	Key                string
	CaCert             string
	MinTLSVersion      string
	MaxTLSVersion      string
	MutualTLS          bool
	InsecureSkipVerify bool
}

// OnBoot is called when the engine is ready for accepting connections
func (ts *tlsServer) OnBoot(eng gnet.Engine) gnet.Action {
	ts.eng = eng
	if ts.engineReady != nil {
		select {
		case ts.engineReady <- eng:
		default:
		}
	}
	logger.Info("TLS server started", "addr", ts.serverAddr, "multicore", ts.multicore)
	return gnet.None
}

// OnOpen is called when a new connection is opened
func (ts *tlsServer) OnOpen(c gnet.Conn) ([]byte, gnet.Action) {
	// Set read/write buffer sizes for better performance
	c.SetReadBuffer(128 * 1024) // 128KB for high throughput
	c.SetWriteBuffer(128 * 1024)

	// Get underlying TCP connection and wrap it with TLS
	// Note: gnet.Conn doesn't expose underlying net.Conn directly
	// We need to handle TLS handshake in OnTraffic
	return nil, gnet.None
}

// OnTraffic handles incoming data with TLS processing
func (ts *tlsServer) OnTraffic(c gnet.Conn) gnet.Action {
	if atomic.LoadUint32(&ts.hepInput.stopped) == 1 {
		return gnet.Close
	}

	// Get or create TLS connection for this gnet connection
	tlsConn, exists := ts.tlsConns.Load(c)
	if !exists {
		// First time - need to get underlying connection and wrap with TLS
		// Since gnet doesn't expose underlying net.Conn, we'll use a different approach:
		// Create a wrapper that implements net.Conn using gnet.Conn
		wrapper := &gnetConnWrapper{conn: c, inbound: make([]byte, 0, 65536)}
		tlsConn = gtls.Server(wrapper, ts.tlsConfig)
		ts.tlsConns.Store(c, tlsConn)

		// Start handshake
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := tlsConn.(*gtls.Conn).HandshakeContext(ctx); err != nil {
			if err == gtls.ErrNotEnough {
				// Not enough data for handshake yet, wait for more
				return gnet.None
			}
			logger.Error(fmt.Sprintf("TLS handshake failed: %v", err))
			ts.tlsConns.Delete(c)
			return gnet.Close
		}
	}

	conn := tlsConn.(*gtls.Conn)

	// Get batch buffer from pool
	var packets [][]byte
	if buf := ts.batchBuffers.Get(); buf != nil {
		packets = buf.([][]byte)[:0]
	} else {
		packets = make([][]byte, 0, tlsBatchSize)
	}
	defer ts.batchBuffers.Put(packets)

	// Read decrypted data from TLS connection
	readBuf := make([]byte, maxPktLen*4)
	totalRead := 0

	// Limit iterations to prevent infinite loops
	maxIterations := tlsBatchSize * 2
	iterations := 0

	for len(packets) < tlsBatchSize && iterations < maxIterations {
		iterations++

		n, err := conn.Read(readBuf[totalRead:])
		if err != nil {
			if err == gtls.ErrNotEnough {
				// Not enough data, wait for more
				break
			}
			logger.Error(fmt.Sprintf("TLS read error: %v", err))
			ts.tlsConns.Delete(c)
			return gnet.Close
		}

		if n == 0 {
			break
		}

		totalRead += n
		processed := 0

		// Process all complete packets in buffer
		// Limit inner loop iterations to prevent busy loops
		maxInnerIterations := tlsBatchSize * 2
		innerIterations := 0
		for processed < totalRead && innerIterations < maxInnerIterations {
			innerIterations++
			remaining := totalRead - processed
			if remaining < hepHeaderSize {
				// Not enough data for header, move to beginning
				copy(readBuf, readBuf[processed:totalRead])
				totalRead = remaining
				break
			}

			// Extract packet size from HEP header
			pktSize := binary.BigEndian.Uint16(readBuf[processed+4 : processed+6])

			// Validate packet size
			if pktSize < hepHeaderSize || pktSize > maxPktLen {
				logger.Error("invalid HEP packet size", "pktSize", pktSize)
				atomic.AddUint64(&ts.hepInput.stats.ErrCount, 1)
				metrics.RecordHEPPacketFailed("tls", "invalid_size")
				ts.tlsConns.Delete(c)
				return gnet.Close
			}

			// Check if we have complete packet
			if remaining < int(pktSize) {
				// Not enough data, move partial data to beginning
				copy(readBuf, readBuf[processed:totalRead])
				totalRead = remaining
				break
			}

			// Get buffer from shared ingest pool and copy packet
			pktBuf := ts.hepInput.getPacketBuf()
			if cap(pktBuf) < int(pktSize) {
				ts.hepInput.putPacketBuf(pktBuf)
				pktBuf = make([]byte, pktSize)
			} else {
				pktBuf = pktBuf[:pktSize]
			}
			copy(pktBuf, readBuf[processed:processed+int(pktSize)])

			// Log complete HEP packet received
			if logger.GetLogger() != nil {
				logger.GetLogger().Debug("received complete HEP packet via TLS", "size", pktSize, "remote", c.RemoteAddr())
			}

			packets = append(packets, pktBuf)
			atomic.AddUint64(&ts.hepInput.stats.PktCount, 1)
			ts.recvMetrics.record(int(pktSize))
			processed += int(pktSize)

			// Send batch when full
			if len(packets) >= tlsBatchSize {
				ts.sendBatchOptimized(packets)
				packets = packets[:0]
				// Reset iteration counter after sending batch
				iterations = 0
			}
		}

		// Reset buffer if all data processed
		if processed == totalRead {
			totalRead = 0
		}
	}

	// Send remaining packets
	if len(packets) > 0 {
		ts.sendBatchOptimized(packets)
	}

	return gnet.None
}

// OnClose is called when a connection is closed
func (ts *tlsServer) OnClose(c gnet.Conn, err error) gnet.Action {
	// Clean up TLS connection
	if tlsConn, ok := ts.tlsConns.LoadAndDelete(c); ok {
		if conn, ok := tlsConn.(*gtls.Conn); ok {
			conn.Close()
		}
	}
	if err != nil {
		logger.Debug(fmt.Sprintf("TLS connection closed: %v", err))
	}
	return gnet.None
}

// sendBatchOptimized sends packets to input channel with optimized batching
func (ts *tlsServer) sendBatchOptimized(packets [][]byte) {
	for _, pkt := range packets {
		// Try non-blocking send first
		select {
		case ts.hepInput.inputCh <- incomingPacket{
			data:       pkt,
			protocol:   "tls",
			receivedAt: time.Now(),
		}:
			// Successfully sent
		default:
			// Channel full - check if server stopped before blocking
			if atomic.LoadUint32(&ts.hepInput.stopped) == 1 {
				ts.hepInput.putPacketBuf(pkt)
				ts.recvMetrics.flush()
				return
			}
			// Fallback to blocking send
			ts.hepInput.inputCh <- incomingPacket{
				data:       pkt,
				protocol:   "tls",
				receivedAt: time.Now(),
			}
		}
	}
}

// gnetConnWrapper wraps gnet.Conn to implement net.Conn interface for TLS
type gnetConnWrapper struct {
	conn    gnet.Conn
	inbound []byte
	mu      sync.Mutex
}

func (w *gnetConnWrapper) Read(b []byte) (n int, err error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	// First, try to read from internal buffer
	if len(w.inbound) > 0 {
		n = copy(b, w.inbound)
		w.inbound = w.inbound[n:]
		return n, nil
	}

	// Read from gnet connection
	available := w.conn.InboundBuffered()
	if available == 0 {
		return 0, gtls.ErrNotEnough
	}

	// Read available data
	readSize := len(b)
	if readSize > available {
		readSize = available
	}

	// Ensure buffer is large enough
	if cap(w.inbound) < readSize {
		w.inbound = make([]byte, readSize)
	} else {
		w.inbound = w.inbound[:readSize]
	}

	n, err = w.conn.Read(w.inbound)
	if err != nil {
		return 0, err
	}
	if n == 0 {
		return 0, gtls.ErrNotEnough
	}

	// Copy to output buffer
	copied := copy(b, w.inbound[:n])
	if copied < n {
		// Save remaining data for next read
		w.inbound = w.inbound[copied:n]
	} else {
		w.inbound = w.inbound[:0]
	}

	return copied, nil
}

func (w *gnetConnWrapper) Write(b []byte) (n int, err error) {
	return w.conn.Write(b)
}

func (w *gnetConnWrapper) Close() error {
	return w.conn.Close()
}

func (w *gnetConnWrapper) LocalAddr() net.Addr {
	return w.conn.LocalAddr()
}

func (w *gnetConnWrapper) RemoteAddr() net.Addr {
	return w.conn.RemoteAddr()
}

func (w *gnetConnWrapper) SetDeadline(t time.Time) error {
	return w.conn.SetDeadline(t)
}

func (w *gnetConnWrapper) SetReadDeadline(t time.Time) error {
	return w.conn.SetReadDeadline(t)
}

func (w *gnetConnWrapper) SetWriteDeadline(t time.Time) error {
	return w.conn.SetWriteDeadline(t)
}

// loadTLSConfig loads TLS configuration from settings
func loadTLSConfig(tlsSettings *TLSSettings) (*gtls.Config, error) {
	if !tlsSettings.Enable {
		logger.Info("TLS server: Enable=false, skipping TLS config loading")
		return nil, nil
	}

	if tlsSettings.Cert == "" || tlsSettings.Key == "" {
		return nil, fmt.Errorf("TLS certificate and key must be provided (cert=%q, key=%q)", tlsSettings.Cert, tlsSettings.Key)
	}

	// Get current working directory for debugging
	wd, _ := os.Getwd()
	logger.Info("TLS server: current working directory", "cwd", wd)
	logger.Info("TLS server: checking certificate file", "cert", tlsSettings.Cert)

	// Check if certificate files exist
	if _, err := os.Stat(tlsSettings.Cert); os.IsNotExist(err) {
		absPath, _ := filepath.Abs(tlsSettings.Cert)
		return nil, fmt.Errorf("TLS certificate file not found: %s (absolute: %s)", tlsSettings.Cert, absPath)
	}
	logger.Info("TLS server: certificate file found", "cert", tlsSettings.Cert)

	logger.Info("TLS server: checking key file", "key", tlsSettings.Key)
	if _, err := os.Stat(tlsSettings.Key); os.IsNotExist(err) {
		absPath, _ := filepath.Abs(tlsSettings.Key)
		return nil, fmt.Errorf("TLS key file not found: %s (absolute: %s)", tlsSettings.Key, absPath)
	}
	logger.Info("TLS server: key file found", "key", tlsSettings.Key)

	// Load certificate
	logger.Info("TLS server: loading certificate pair...")
	cert, err := gtls.LoadX509KeyPair(tlsSettings.Cert, tlsSettings.Key)
	if err != nil {
		return nil, fmt.Errorf("failed to load TLS certificate from cert=%s key=%s: %w", tlsSettings.Cert, tlsSettings.Key, err)
	}
	logger.Info("TLS server: certificate pair loaded successfully")

	config := &gtls.Config{
		Certificates: []gtls.Certificate{cert},
		MinVersion:   parseTLSVersion(tlsSettings.MinTLSVersion),
		MaxVersion:   parseTLSVersion(tlsSettings.MaxTLSVersion),
	}

	// Load CA certificate for mutual TLS
	if tlsSettings.MutualTLS && tlsSettings.CaCert != "" {
		caCert, err := os.ReadFile(tlsSettings.CaCert)
		if err != nil {
			return nil, fmt.Errorf("failed to load CA certificate: %w", err)
		}

		caCertPool := x509.NewCertPool()
		if !caCertPool.AppendCertsFromPEM(caCert) {
			return nil, fmt.Errorf("failed to parse CA certificate")
		}

		config.ClientCAs = caCertPool
		config.ClientAuth = gtls.RequireAndVerifyClientCert
	}

	// TLS optimizations for performance
	config.PreferServerCipherSuites = true
	config.SessionTicketsDisabled = false // Enable session tickets for better performance
	config.CipherSuites = []uint16{
		gtls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
		gtls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
		gtls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
		gtls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
		gtls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,
		gtls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305,
	}

	if tlsSettings.InsecureSkipVerify {
		config.InsecureSkipVerify = true
	}

	return config, nil
}

// parseTLSVersion converts TLS version string to tls.Version constant
func parseTLSVersion(version string) uint16 {
	switch version {
	case "TLS1.0":
		return gtls.VersionTLS10
	case "TLS1.1":
		return gtls.VersionTLS11
	case "TLS1.2":
		return gtls.VersionTLS12
	case "TLS1.3":
		return gtls.VersionTLS13
	default:
		return gtls.VersionTLS12 // Default to TLS 1.2
	}
}

// serveTLS starts a high-performance TLS server using gnet with gnet-io/tls
func (h *HEPInput) serveTLS(addr string, port int, tlsSettings *TLSSettings) {
	atomic.StoreUint32(&h.tlsStarted, 1)
	defer close(h.exitTLS)

	logger.Info("TLS server: attempting to start", "addr", fmt.Sprintf("%s:%d", addr, port))
	logger.Info("TLS server: TLS settings", "cert", tlsSettings.Cert, "key", tlsSettings.Key, "cacert", tlsSettings.CaCert)

	// Load TLS configuration
	tlsConfig, err := loadTLSConfig(tlsSettings)
	if err != nil {
		logger.Error(fmt.Sprintf("failed to load TLS config: %v", err))
		return
	}

	if tlsConfig == nil {
		logger.Error("TLS config is nil - TLS is not enabled or config loading failed")
		return
	}

	logger.Info("TLS server: TLS config loaded successfully")

	serverAddr := fmt.Sprintf("tcp://%s:%d", addr, port)

	ts := &tlsServer{
		hepInput:    h,
		multicore:   true, // TLS benefits from multicore
		serverAddr:  serverAddr,
		tlsConfig:   tlsConfig,
		recvMetrics: newIngestReceiveMetrics("tls"),
		engineReady: make(chan gnet.Engine, 1),
	}

	// Initialize batch buffer pool
	ts.batchBuffers = sync.Pool{
		New: func() interface{} {
			return make([][]byte, 0, tlsBatchSize)
		},
	}

	logger.Info("TLS server using gnet + gnet-io/tls", "addr", fmt.Sprintf("%s:%d", addr, port))

	go func() {
		<-h.tlsStop
		select {
		case eng := <-ts.engineReady:
			_ = eng.Stop(context.Background())
		case <-time.After(2 * time.Second):
		}
	}()

	// Start gnet server with TLS support
	err = gnet.Run(ts, serverAddr,
		gnet.WithMulticore(true),
		gnet.WithReusePort(true),
		gnet.WithTicker(false), // Disable ticker to reduce CPU usage when idle
		gnet.WithTCPKeepAlive(time.Minute*5),
		gnet.WithReadBufferCap(64*1024),
		gnet.WithWriteBufferCap(64*1024),
		gnet.WithLockOSThread(false),
		gnet.WithTCPNoDelay(gnet.TCPNoDelay),
	)

	if err != nil {
		logger.Error(fmt.Sprintf("TLS server error: %v", err))
		return
	}

	logger.Info("TLS server stopped")
}
