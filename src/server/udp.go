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
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/panjf2000/gnet/v2"
	"github.com/sipcapture/homer-core/src/homerconfig"
	logger "github.com/sipcapture/homer-core/src/utils/logging"
	"github.com/sipcapture/homer-core/src/utils/metrics"
)

// udpServer implements gnet.EventHandler for high-performance UDP server
type udpServer struct {
	gnet.BuiltinEventEngine
	hepInput   *HEPInput
	eng        gnet.Engine
	serverAddr string
	packetPool *sync.Pool
}

// OnBoot is called when the engine is ready
func (us *udpServer) OnBoot(eng gnet.Engine) gnet.Action {
	us.eng = eng
	logger.Info("UDP server OnBoot called", "addr", us.serverAddr)
	return gnet.None
}

// OnTraffic handles incoming UDP packets (gnet v2 uses OnTraffic for UDP too)
func (us *udpServer) OnTraffic(c gnet.Conn) gnet.Action {
	if atomic.LoadUint32(&us.hepInput.stopped) == 1 {
		return gnet.Shutdown
	}

	// Zero-copy read: Next(-1) returns the internal buffer slice without copying
	packet, err := c.Next(-1)
	if err != nil || len(packet) == 0 {
		return gnet.None
	}

	if len(packet) > maxPktLen {
		atomic.AddUint64(&us.hepInput.stats.ErrCount, 1)
		return gnet.None
	}

	if len(packet) < hepHeaderSize {
		atomic.AddUint64(&us.hepInput.stats.ErrCount, 1)
		metrics.RecordHEPPacketFailed("udp", "too_small")
		return gnet.None
	}

	pktSize := binary.BigEndian.Uint16(packet[4:6])
	if pktSize < hepHeaderSize || pktSize > maxPktLen {
		atomic.AddUint64(&us.hepInput.stats.ErrCount, 1)
		metrics.RecordHEPPacketFailed("udp", "invalid_size")
		return gnet.None
	}

	if len(packet) < int(pktSize) {
		atomic.AddUint64(&us.hepInput.stats.ErrCount, 1)
		return gnet.None
	}

	// Copy packet data into pooled buffer (Next buffer is reused by gnet)
	buf := us.packetPool.Get().([]byte)
	buf = buf[:len(packet)]
	copy(buf, packet)

	// Single time.Now() call for the entire hot path
	now := time.Now()

	metrics.RecordHEPPacketReceived("udp")
	metrics.RecordHEPPacketSize("udp", int(pktSize))
	metrics.RecordBytesReceived("udp", int64(pktSize))

	pkt := incomingPacket{data: buf, protocol: "udp", receivedAt: now}

	select {
	case us.hepInput.inputCh <- pkt:
		atomic.AddUint64(&us.hepInput.stats.PktCount, 1)
	default:
		if atomic.LoadUint32(&us.hepInput.stopped) == 1 {
			us.packetPool.Put(buf[:maxPktLen])
			return gnet.Shutdown
		}
		us.hepInput.inputCh <- pkt
		atomic.AddUint64(&us.hepInput.stats.PktCount, 1)
	}

	return gnet.None
}

// React handles incoming UDP packets (fallback for gnet v1 compatibility)
func (us *udpServer) React(packet []byte, c gnet.Conn) (out []byte, action gnet.Action) {
	if atomic.LoadUint32(&us.hepInput.stopped) == 1 {
		return nil, gnet.Shutdown
	}

	if len(packet) == 0 || len(packet) > maxPktLen {
		atomic.AddUint64(&us.hepInput.stats.ErrCount, 1)
		return nil, gnet.None
	}

	if len(packet) < hepHeaderSize {
		atomic.AddUint64(&us.hepInput.stats.ErrCount, 1)
		return nil, gnet.None
	}

	pktSize := binary.BigEndian.Uint16(packet[4:6])
	if pktSize < hepHeaderSize || pktSize > maxPktLen || len(packet) < int(pktSize) {
		atomic.AddUint64(&us.hepInput.stats.ErrCount, 1)
		return nil, gnet.None
	}

	buf := us.packetPool.Get().([]byte)
	buf = buf[:len(packet)]
	copy(buf, packet)

	now := time.Now()
	pkt := incomingPacket{data: buf, protocol: "udp", receivedAt: now}

	select {
	case us.hepInput.inputCh <- pkt:
		atomic.AddUint64(&us.hepInput.stats.PktCount, 1)
	default:
		if atomic.LoadUint32(&us.hepInput.stopped) == 1 {
			us.packetPool.Put(buf[:maxPktLen])
			return nil, gnet.Shutdown
		}
		us.hepInput.inputCh <- pkt
		atomic.AddUint64(&us.hepInput.stats.PktCount, 1)
	}

	return nil, gnet.None
}

// serveUDP starts a high-performance UDP server using gnet
func (h *HEPInput) serveUDP(addr string) {
	atomic.StoreUint32(&h.udpStarted, 1)
	defer close(h.exitUDP)

	serverAddr := fmt.Sprintf("udp://%s", addr)

	// Read configurable buffer sizes (legacy config path)
	socketRecvBuf := 8 * 1024 * 1024 // 8MB default
	socketSendBuf := 1024 * 1024     // 1MB default
	readBufCap := 128 * 1024         // 128KB default

	if homerconfig.MainConfig != nil {
		udpCfg := homerconfig.MainConfig.Setting.SERVER_SETTINGS.UDP_SERVER
		if udpCfg.SocketRecvBuffer > 0 {
			socketRecvBuf = udpCfg.SocketRecvBuffer
		}
		if udpCfg.SocketSendBuffer > 0 {
			socketSendBuf = udpCfg.SocketSendBuffer
		}
		if udpCfg.ReadBufferCap > 0 {
			readBufCap = udpCfg.ReadBufferCap
		}
	}

	warnUDPSysctlLimits(socketRecvBuf)

	// Create packet pool for zero-allocation packet handling
	packetPool := &sync.Pool{
		New: func() interface{} {
			return make([]byte, maxPktLen)
		},
	}

	us := &udpServer{
		hepInput:   h,
		serverAddr: serverAddr,
		packetPool: packetPool,
	}

	logger.Info("Starting UDP server",
		"addr", serverAddr,
		"socket_recv_buffer", socketRecvBuf,
		"socket_send_buffer", socketSendBuf,
		"read_buffer_cap", readBufCap,
	)

	err := gnet.Run(us, serverAddr,
		gnet.WithMulticore(true),
		gnet.WithReusePort(true),
		gnet.WithTicker(false),
		gnet.WithReadBufferCap(readBufCap),
		gnet.WithWriteBufferCap(64*1024),
		gnet.WithLockOSThread(false),
		gnet.WithSocketRecvBuffer(socketRecvBuf),
		gnet.WithSocketSendBuffer(socketSendBuf),
	)

	if err != nil {
		logger.Error(fmt.Sprintf("UDP server error: %v", err))
		return
	}

	logger.Info("UDP server stopped")
}

// warnUDPSysctlLimits checks OS-level UDP buffer limits and warns if they are
// too low for the requested socket receive buffer size.
func warnUDPSysctlLimits(requestedRecvBuf int) {
	data, err := os.ReadFile("/proc/sys/net/core/rmem_max")
	if err != nil {
		return
	}
	var rmemMax int
	if _, err := fmt.Sscanf(strings.TrimSpace(string(data)), "%d", &rmemMax); err != nil {
		return
	}
	if rmemMax < requestedRecvBuf {
		logger.Warn(fmt.Sprintf(
			"net.core.rmem_max (%d) is below requested socket_recv_buffer (%d). "+
				"UDP packets will be dropped under load. Fix: sudo sysctl -w net.core.rmem_max=%d",
			rmemMax, requestedRecvBuf, requestedRecvBuf))
	}
}

// min returns the minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
