// Copyright (C) 2025 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package writer

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/panjf2000/gnet/v2"
	"github.com/sipcapture/homer-core/src/config"
	logger "github.com/sipcapture/homer-core/src/utils/logging"
	"github.com/sipcapture/homer-core/src/utils/sysctl"
)

// UDPServer handles UDP HEP packet reception
type UDPServer struct {
	writer *Writer
	config *config.UDPServerConfig
	addr   string
}

// udpEventHandler implements gnet.EventHandler
type udpEventHandler struct {
	writer *Writer
}

// NewUDPServer creates a new UDP server
func NewUDPServer(w *Writer, cfg *config.UDPServerConfig) *UDPServer {
	return &UDPServer{
		writer: w,
		config: cfg,
	}
}

// Start starts the UDP server
func (s *UDPServer) Start() error {
	s.addr = fmt.Sprintf("udp://%s:%d", s.config.Host, s.config.Port)

	socketRecvBuf := 8 * 1024 * 1024 // 8MB default
	socketSendBuf := 1024 * 1024     // 1MB default
	readBufCap := 128 * 1024         // 128KB default

	if s.config.SocketRecvBuffer > 0 {
		socketRecvBuf = s.config.SocketRecvBuffer
	}
	if s.config.SocketSendBuffer > 0 {
		socketSendBuf = s.config.SocketSendBuffer
	}
	if s.config.ReadBufferCap > 0 {
		readBufCap = s.config.ReadBufferCap
	}

	origRecv := socketRecvBuf
	socketRecvBuf = sysctl.EffectiveUDPSocketRecvBuffer(socketRecvBuf)
	if socketRecvBuf != origRecv {
		logger.Info("Writer: UDP socket_recv_buffer capped by net.core.rmem_max",
			"requested", origRecv, "effective", socketRecvBuf)
	}

	warnWriterUDPSysctlLimits(socketRecvBuf)

	handler := &udpEventHandler{writer: s.writer}

	opts := []gnet.Option{
		gnet.WithMulticore(s.config.Multicore),
		gnet.WithReusePort(true),
		gnet.WithTicker(false),
		gnet.WithReadBufferCap(readBufCap),
		gnet.WithSocketRecvBuffer(socketRecvBuf),
		gnet.WithSocketSendBuffer(socketSendBuf),
		gnet.WithLockOSThread(false),
	}

	logger.Info("Writer: Starting UDP server",
		"addr", fmt.Sprintf("%s:%d", s.config.Host, s.config.Port),
		"socket_recv_buffer", socketRecvBuf,
		"socket_send_buffer", socketSendBuf,
		"read_buffer_cap", readBufCap,
	)

	err := gnet.Run(handler, s.addr, opts...)
	if err != nil {
		return fmt.Errorf("UDP server error: %w", err)
	}
	return nil
}

func warnWriterUDPSysctlLimits(requestedRecvBuf int) {
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

// Stop stops the UDP server
func (s *UDPServer) Stop() {
	// gnet.Run blocks until shutdown, so nothing to do here
	// The server will be stopped when the process exits
}

// OnBoot is called when the server starts
func (h *udpEventHandler) OnBoot(eng gnet.Engine) gnet.Action {
	logger.Info("Writer: UDP server started")
	return gnet.None
}

// OnShutdown is called when the server stops
func (h *udpEventHandler) OnShutdown(eng gnet.Engine) {
	logger.Info("Writer: UDP server stopped")
}

// OnOpen is called when a new connection is opened
func (h *udpEventHandler) OnOpen(c gnet.Conn) (out []byte, action gnet.Action) {
	return nil, gnet.None
}

// OnClose is called when a connection is closed
func (h *udpEventHandler) OnClose(c gnet.Conn, err error) gnet.Action {
	return gnet.None
}

// OnTraffic is called when data is received
func (h *udpEventHandler) OnTraffic(c gnet.Conn) gnet.Action {
	data, err := c.Next(-1)
	if err != nil {
		return gnet.None
	}

	// Validate HEP header (basic check)
	if len(data) < 6 {
		return gnet.None
	}

	// HEPv3 magic: "HEP3"
	if data[0] == 'H' && data[1] == 'E' && data[2] == 'P' && data[3] == '3' {
		h.writer.EnqueuePacket(data, "udp")
		return gnet.None
	}

	// HEPv2 magic: 0x02 0x10 0x02
	if data[0] == 0x02 && data[1] == 0x10 && data[2] == 0x02 {
		h.writer.EnqueuePacket(data, "udp")
		return gnet.None
	}

	return gnet.None
}

// OnTick is called on each tick
func (h *udpEventHandler) OnTick() (delay time.Duration, action gnet.Action) {
	return 0, gnet.None
}
