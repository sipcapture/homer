// Copyright (C) 2026 Homer Server Contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package input

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/sipcapture/homer-core/src/utils/metrics"
)

// ingestMetricsFlushInterval batches Prometheus updates on the gnet
// receive path (RecordHEPPacketReceived / Size / BytesReceived).
const ingestMetricsFlushInterval = 128

// ingestReceiveMetrics accumulates receive-side counters and flushes
// them in batches. Label lookup is done once at construction.
type ingestReceiveMetrics struct {
	pktsCounter  prometheus.Counter
	bytesCounter prometheus.Counter
	sizeHist     prometheus.Observer

	pending   int
	bytesSum  int64
	sizeBatch []int
}

func newIngestReceiveMetrics(protocol string) ingestReceiveMetrics {
	return ingestReceiveMetrics{
		pktsCounter:  metrics.HEPPacketsReceived.WithLabelValues(protocol),
		bytesCounter: metrics.BytesReceived.WithLabelValues(protocol),
		sizeHist:     metrics.HEPPacketSize.WithLabelValues(protocol),
		sizeBatch:    make([]int, 0, ingestMetricsFlushInterval),
	}
}

func (m *ingestReceiveMetrics) record(pktSize int) {
	m.pending++
	m.bytesSum += int64(pktSize)
	m.sizeBatch = append(m.sizeBatch, pktSize)
	if m.pending >= ingestMetricsFlushInterval {
		m.flush()
	}
}

func (m *ingestReceiveMetrics) flush() {
	if m.pending == 0 {
		return
	}
	m.pktsCounter.Add(float64(m.pending))
	m.bytesCounter.Add(float64(m.bytesSum))
	for _, sz := range m.sizeBatch {
		m.sizeHist.Observe(float64(sz))
	}
	m.pending = 0
	m.bytesSum = 0
	m.sizeBatch = m.sizeBatch[:0]
}
