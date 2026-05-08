// Copyright (C) 2026 Homer Server Contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package sink

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	collogspb "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	colmetricspb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
)

// blockingTraceSink blocks PushTraces until unblock receives, so the async
// worker stays inside the inner sink while more jobs can pile up in the channel.
type blockingTraceSink struct {
	unblock       chan struct{}
	innerStarted  atomic.Int32
	innerFinished atomic.Int32
}

func (b *blockingTraceSink) PushTraces(_ context.Context, _ *coltracepb.ExportTraceServiceRequest) error {
	b.innerStarted.Add(1)
	<-b.unblock
	b.innerFinished.Add(1)
	return nil
}

func (b *blockingTraceSink) PushMetrics(_ context.Context, _ *colmetricspb.ExportMetricsServiceRequest) error {
	return nil
}

func (b *blockingTraceSink) PushLogs(_ context.Context, _ *collogspb.ExportLogsServiceRequest) error {
	return nil
}

func TestAsyncQueuePushReturnsBeforeInnerCompletes(t *testing.T) {
	inner := &blockingTraceSink{unblock: make(chan struct{})}
	q := NewAsyncQueue(inner, 8, 0)
	defer func() {
		close(inner.unblock)
		_ = q.Drain(context.Background())
	}()

	req := &coltracepb.ExportTraceServiceRequest{}
	if err := q.PushTraces(context.Background(), req); err != nil {
		t.Fatalf("PushTraces: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for inner.innerStarted.Load() < 1 {
		if time.Now().After(deadline) {
			t.Fatal("worker did not start inner PushTraces")
		}
		time.Sleep(2 * time.Millisecond)
	}
	if got := inner.innerFinished.Load(); got != 0 {
		t.Fatalf("PushTraces returned but inner already finished (finished=%d)", got)
	}
}

func TestAsyncQueueFullReturnsError(t *testing.T) {
	inner := &blockingTraceSink{unblock: make(chan struct{})}
	// depth 1: one batch can wait in the channel while the worker is blocked
	// inside the inner sink on the first batch; a third enqueue must fail.
	q := NewAsyncQueue(inner, 1, 0)
	defer func() {
		close(inner.unblock)
		_ = q.Drain(context.Background())
	}()

	req := &coltracepb.ExportTraceServiceRequest{}
	if err := q.PushTraces(context.Background(), req); err != nil {
		t.Fatalf("first push: %v", err)
	}
	// Ensure the worker has dequeued the first job and is blocked inside inner;
	// otherwise the channel may still hold batch 1 and the second push races as "full".
	deadline := time.Now().Add(2 * time.Second)
	for inner.innerStarted.Load() < 1 {
		if time.Now().After(deadline) {
			t.Fatal("worker did not start inner PushTraces")
		}
		time.Sleep(2 * time.Millisecond)
	}
	if err := q.PushTraces(context.Background(), req); err != nil {
		t.Fatalf("second push: %v", err)
	}
	if err := q.PushTraces(context.Background(), req); err == nil {
		t.Fatal("expected queue full error on third push")
	}
}

func TestAsyncQueueDrainProcessesTail(t *testing.T) {
	inner := &blockingTraceSink{unblock: make(chan struct{})}
	q := NewAsyncQueue(inner, 4, 0)

	req := &coltracepb.ExportTraceServiceRequest{}
	if err := q.PushTraces(context.Background(), req); err != nil {
		t.Fatalf("push: %v", err)
	}
	close(inner.unblock)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := q.Drain(ctx); err != nil {
		t.Fatalf("Drain: %v", err)
	}
	if inner.innerFinished.Load() != 1 {
		t.Fatalf("expected 1 completed inner write, got %d", inner.innerFinished.Load())
	}
}
