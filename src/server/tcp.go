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
	"encoding/binary"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/panjf2000/gnet/v2"
	logger "github.com/sipcapture/homer-core/src/utils/logging"
	"github.com/sipcapture/homer-core/src/utils/metrics"
)

const (
	// HEP header size
	hepHeaderSize = 6
	// Batch size for packet processing
	batchSize = 64
)

// tcpServer implements gnet.EventHandler for high-performance TCP server
type tcpServer struct {
	gnet.BuiltinEventEngine
	hepInput   *HEPInput
	eng        gnet.Engine
	multicore  bool
	serverAddr string
	packetPool *sync.Pool
	// Per-connection batch buffer for efficient packet batching
	batchBuffers sync.Pool
}

// OnBoot is called when the engine is ready for accepting connections
func (ts *tcpServer) OnBoot(eng gnet.Engine) gnet.Action {
	ts.eng = eng
	logger.Info("TCP server started", "addr", ts.serverAddr, "multicore", ts.multicore)
	return gnet.None
}

// OnOpen is called when a new connection is opened
func (ts *tcpServer) OnOpen(c gnet.Conn) ([]byte, gnet.Action) {
	// Set read/write buffer sizes for better performance
	c.SetReadBuffer(128 * 1024) // 128KB for high throughput
	c.SetWriteBuffer(128 * 1024)
	return nil, gnet.None
}

// OnTraffic handles incoming data with optimized batch processing
func (ts *tcpServer) OnTraffic(c gnet.Conn) gnet.Action {
	if atomic.LoadUint32(&ts.hepInput.stopped) == 1 {
		return gnet.Close
	}

	// Get batch buffer from pool (reuse for zero allocation)
	var packets [][]byte
	if buf := ts.batchBuffers.Get(); buf != nil {
		packets = buf.([][]byte)[:0] // Reset length but keep capacity
	} else {
		packets = make([][]byte, 0, batchSize)
	}
	defer ts.batchBuffers.Put(packets) // Return to pool

	// Process as many packets as possible in one batch
	// Limit iterations to prevent infinite loops
	maxIterations := batchSize * 2
	iterations := 0

	for len(packets) < batchSize && iterations < maxIterations {
		iterations++

		// Check available data
		in := c.InboundBuffered()
		if in < hepHeaderSize {
			break // Not enough data for header
		}

		// Peek at header to get packet size (zero-copy operation)
		header, err := c.Peek(hepHeaderSize)
		if err != nil || len(header) < hepHeaderSize {
			break
		}

		// Extract packet size from HEP header (bytes 4-5, big endian)
		pktSize := binary.BigEndian.Uint16(header[4:6])

		// Fast validation
		if pktSize < hepHeaderSize || pktSize > maxPktLen {
			logger.Error("invalid HEP packet size", "pktSize", pktSize)
			atomic.AddUint64(&ts.hepInput.stats.ErrCount, 1)
			metrics.RecordHEPPacketFailed("tcp", "invalid_size")
			return gnet.Close
		}

		// Check if we have enough data for complete packet
		if in < int(pktSize) {
			break // Wait for more data - this is critical to prevent busy loop
		}

		// Get buffer from pool
		buf := ts.packetPool.Get().([]byte)
		if cap(buf) < int(pktSize) {
			// Buffer too small, allocate new one
			ts.packetPool.Put(buf)
			buf = make([]byte, pktSize)
		} else {
			buf = buf[:pktSize]
		}

		// Read packet data (single syscall for entire packet)
		n, err := c.Read(buf)
		if err != nil || n != int(pktSize) {
			if err == nil {
				logger.Error("read size mismatch", "expected", pktSize, "got", n)
			}
			ts.packetPool.Put(buf)
			atomic.AddUint64(&ts.hepInput.stats.ErrCount, 1)
			if err != nil {
				return gnet.Close
			}
			// Don't continue here - break to avoid infinite loop
			break
		}

		// Log complete HEP packet received
		if logger.GetLogger() != nil {
			logger.GetLogger().Debug("received complete HEP packet via TCP", "size", pktSize, "remote", c.RemoteAddr())
		}

		// Record Prometheus metrics
		metrics.RecordHEPPacketReceived("tcp")
		metrics.RecordHEPPacketSize("tcp", int(pktSize))
		metrics.RecordBytesReceived("tcp", int64(pktSize))

		// Add to batch
		packets = append(packets, buf)
		atomic.AddUint64(&ts.hepInput.stats.PktCount, 1)
	}

	// Send packets to input channel efficiently
	if len(packets) > 0 {
		ts.sendBatchOptimized(packets)
	}

	return gnet.None
}

// OnClose is called when a connection is closed
func (ts *tcpServer) OnClose(c gnet.Conn, err error) gnet.Action {
	if err != nil {
		logger.Debug(fmt.Sprintf("connection closed: %v", err))
	}
	return gnet.None
}

// sendBatchOptimized sends packets to input channel with optimized batching
func (ts *tcpServer) sendBatchOptimized(packets [][]byte) {
	// Fast path: try non-blocking send for all packets
	for _, pkt := range packets {
		// Try non-blocking send first
		select {
		case ts.hepInput.inputCh <- incomingPacket{
			data:       pkt,
			protocol:   "tcp",
			receivedAt: time.Now(),
		}:
			// Successfully sent
		default:
			// Channel full - this should be rare with 40k buffer capacity
			// Use blocking send but check if server is stopped
			if atomic.LoadUint32(&ts.hepInput.stopped) == 1 {
				// Server stopped, return buffer to pool and skip
				if cap(pkt) >= maxPktLen {
					ts.packetPool.Put(pkt[:maxPktLen])
				}
				return
			}
			// Blocking send - this will wait until channel has space
			ts.hepInput.inputCh <- incomingPacket{
				data:       pkt,
				protocol:   "tcp",
				receivedAt: time.Now(),
			}
		}
	}
}

// serveTCP starts a high-performance TCP server using gnet
func (h *HEPInput) serveTCP(addr string, port int, multicore bool) {
	atomic.StoreUint32(&h.tcpStarted, 1)
	defer close(h.exitTCP)

	serverAddr := fmt.Sprintf("tcp://%s:%d", addr, port)

	// Create packet pool for zero-allocation packet handling
	packetPool := &sync.Pool{
		New: func() interface{} {
			return make([]byte, maxPktLen)
		},
	}

	ts := &tcpServer{
		hepInput:   h,
		multicore:  multicore,
		serverAddr: serverAddr,
		packetPool: packetPool,
	}

	// Initialize batch buffer pool
	ts.batchBuffers = sync.Pool{
		New: func() interface{} {
			return make([][]byte, 0, batchSize)
		},
	}

	// Start gnet server with optimizations
	err := gnet.Run(ts, serverAddr,
		gnet.WithMulticore(multicore),
		gnet.WithReusePort(true), // Enable SO_REUSEPORT for better load distribution
		gnet.WithTicker(false),   // Disable ticker to reduce CPU usage when idle
		gnet.WithTCPKeepAlive(time.Minute*5),
		gnet.WithReadBufferCap(64*1024),  // 64KB read buffer
		gnet.WithWriteBufferCap(64*1024), // 64KB write buffer
		gnet.WithLockOSThread(false),     // Don't lock OS thread for better scheduling
	)

	if err != nil {
		logger.Error(fmt.Sprintf("TCP server error: %v", err))
		return
	}

	logger.Info("TCP server stopped")
}
