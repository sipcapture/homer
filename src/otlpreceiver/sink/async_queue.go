// Copyright (C) 2026 Homer Server Contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package sink

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/sipcapture/homer-core/src/utils/metrics"
	logger "github.com/sipcapture/homer-core/src/utils/logging"
	collogspb "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	colmetricspb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/protobuf/proto"
)

const (
	jobTraces int8 = iota
	jobMetrics
	jobLogs
)

// Drainer is implemented by sinks that must flush after OTLP listeners
// stop (e.g. bounded async queue). Optional for the OTLP module.
type Drainer interface {
	Drain(ctx context.Context) error
}

type otlpJob struct {
	kind    int8
	traces  *coltracepb.ExportTraceServiceRequest
	metrics *colmetricspb.ExportMetricsServiceRequest
	logs    *collogspb.ExportLogsServiceRequest
}

// AsyncQueue wraps a Sink with a single worker and bounded channel.
// Export handlers return after enqueue (proto clone + send). Suitable
// for low traffic; on queue full Push* returns an error so clients can retry.
type AsyncQueue struct {
	inner   Sink
	ch      chan otlpJob
	wg      sync.WaitGroup
	once    sync.Once
	timeout time.Duration // 0 = non-blocking enqueue when channel is full
}

// NewAsyncQueue starts a background worker. queueDepth must be >= 1
// (values < 1 are clamped to 512). enqueueTimeoutMs 0 means a full
// buffer returns immediately with an error; >0 waits up to that long
// for a slot.
func NewAsyncQueue(inner Sink, queueDepth, enqueueTimeoutMs int) *AsyncQueue {
	if queueDepth < 1 {
		queueDepth = 512
	}
	q := &AsyncQueue{
		inner:   inner,
		ch:      make(chan otlpJob, queueDepth),
		timeout: time.Duration(enqueueTimeoutMs) * time.Millisecond,
	}
	q.wg.Add(1)
	go q.run()
	return q
}

func (q *AsyncQueue) run() {
	defer q.wg.Done()
	for job := range q.ch {
		q.dispatch(job)
	}
}

func (q *AsyncQueue) dispatch(job otlpJob) {
	ctx := context.Background()
	var err error
	switch job.kind {
	case jobTraces:
		err = q.inner.PushTraces(ctx, job.traces)
		if err != nil {
			metrics.RecordOTLPAsyncWorkerError("traces")
			logger.Warn("OTLP async worker: traces write failed", "error", err.Error())
		}
	case jobMetrics:
		err = q.inner.PushMetrics(ctx, job.metrics)
		if err != nil {
			metrics.RecordOTLPAsyncWorkerError("metrics")
			logger.Warn("OTLP async worker: metrics write failed", "error", err.Error())
		}
	case jobLogs:
		err = q.inner.PushLogs(ctx, job.logs)
		if err != nil {
			metrics.RecordOTLPAsyncWorkerError("logs")
			logger.Warn("OTLP async worker: logs write failed", "error", err.Error())
		}
	}
}

func (q *AsyncQueue) tryEnqueue(signal string, job otlpJob) error {
	if q.timeout <= 0 {
		select {
		case q.ch <- job:
			metrics.RecordOTLPAsyncEnqueue(signal, "ok")
			return nil
		default:
			metrics.RecordOTLPAsyncEnqueue(signal, "queue_full")
			return fmt.Errorf("otlp async queue full (%s)", signal)
		}
	}
	t := time.NewTimer(q.timeout)
	defer t.Stop()
	select {
	case q.ch <- job:
		metrics.RecordOTLPAsyncEnqueue(signal, "ok")
		return nil
	case <-t.C:
		metrics.RecordOTLPAsyncEnqueue(signal, "queue_full")
		return fmt.Errorf("otlp async queue full (%s)", signal)
	}
}

// PushTraces implements Sink.
func (q *AsyncQueue) PushTraces(ctx context.Context, req *coltracepb.ExportTraceServiceRequest) error {
	if req == nil {
		return nil
	}
	cloned, ok := proto.Clone(req).(*coltracepb.ExportTraceServiceRequest)
	if !ok {
		return fmt.Errorf("otlp async: clone traces request")
	}
	return q.tryEnqueue("traces", otlpJob{kind: jobTraces, traces: cloned})
}

// PushMetrics implements Sink.
func (q *AsyncQueue) PushMetrics(ctx context.Context, req *colmetricspb.ExportMetricsServiceRequest) error {
	if req == nil {
		return nil
	}
	cloned, ok := proto.Clone(req).(*colmetricspb.ExportMetricsServiceRequest)
	if !ok {
		return fmt.Errorf("otlp async: clone metrics request")
	}
	return q.tryEnqueue("metrics", otlpJob{kind: jobMetrics, metrics: cloned})
}

// PushLogs implements Sink.
func (q *AsyncQueue) PushLogs(ctx context.Context, req *collogspb.ExportLogsServiceRequest) error {
	if req == nil {
		return nil
	}
	cloned, ok := proto.Clone(req).(*collogspb.ExportLogsServiceRequest)
	if !ok {
		return fmt.Errorf("otlp async: clone logs request")
	}
	return q.tryEnqueue("logs", otlpJob{kind: jobLogs, logs: cloned})
}

// Drain closes the job channel and waits for the worker to finish.
// Call only after OTLP HTTP/gRPC listeners have stopped accepting work.
func (q *AsyncQueue) Drain(ctx context.Context) error {
	q.once.Do(func() {
		close(q.ch)
	})
	done := make(chan struct{})
	go func() {
		q.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
