// Copyright (C) 2026 Homer Server Contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package input

import "testing"

func TestIngestReceiveMetrics_flush(t *testing.T) {
	m := newIngestReceiveMetrics("udp")
	for i := 0; i < ingestMetricsFlushInterval-1; i++ {
		m.record(100)
	}
	if m.pending != ingestMetricsFlushInterval-1 {
		t.Fatalf("pending = %d, want %d", m.pending, ingestMetricsFlushInterval-1)
	}
	m.record(200) // triggers auto-flush at 128
	if m.pending != 0 {
		t.Fatalf("pending after auto-flush = %d, want 0", m.pending)
	}
	if len(m.sizeBatch) != 0 {
		t.Fatalf("sizeBatch len = %d after flush, want 0", len(m.sizeBatch))
	}

	m.record(50)
	m.flush()
	if m.pending != 0 {
		t.Fatalf("pending after manual flush = %d, want 0", m.pending)
	}
}
