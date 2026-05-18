// Copyright (C) 2026 Homer Server Contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package input

import (
	"sync/atomic"
	"time"

	"github.com/valyala/fasthttp"
)

// copyPacketToPool copies request body into a shared ingest buffer.
func copyPacketToPool(h *HEPInput, body []byte) []byte {
	buf := h.getPacketBuf()
	if cap(buf) < len(body) {
		h.putPacketBuf(buf)
		buf = make([]byte, len(body))
	} else {
		buf = buf[:len(body)]
	}
	copy(buf, body)
	return buf
}

// enqueueHTTPPacket records batched receive metrics and enqueues the packet.
// Returns false when the server is stopping and the packet was dropped.
func enqueueHTTPPacket(h *HEPInput, protocol string, data []byte, recv *ingestReceiveMetrics) bool {
	if recv != nil {
		recv.record(len(data))
	}
	pkt := incomingPacket{data: data, protocol: protocol, receivedAt: time.Now()}
	select {
	case h.inputCh <- pkt:
		atomic.AddUint64(&h.stats.PktCount, 1)
		return true
	default:
		if atomic.LoadUint32(&h.stopped) == 1 {
			h.putPacketBuf(data)
			return false
		}
		h.inputCh <- pkt
		atomic.AddUint64(&h.stats.PktCount, 1)
		return true
	}
}

// respondOK writes a minimal 200 response.
func respondOK(ctx *fasthttp.RequestCtx) {
	ctx.SetStatusCode(fasthttp.StatusOK)
	ctx.SetBodyString("OK")
}

// handleIngestPOST copies the POST body into the ingest pool, records
// batched metrics, and enqueues for workers.
func handleIngestPOST(h *HEPInput, ctx *fasthttp.RequestCtx, protocol string, recv *ingestReceiveMetrics) {
	body := ctx.PostBody()
	if len(body) == 0 {
		ctx.Error("Empty request body", fasthttp.StatusBadRequest)
		return
	}
	packet := copyPacketToPool(h, body)
	if !enqueueHTTPPacket(h, protocol, packet, recv) {
		ctx.Error("Service Unavailable", fasthttp.StatusServiceUnavailable)
		return
	}
	respondOK(ctx)
}
