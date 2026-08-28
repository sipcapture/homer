// Copyright (C) 2025 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

// Package writer provides the HEP writer module for Homer Server.
// It receives HEP packets via UDP/TCP/TLS/HTTP and writes them to DuckLake storage.
package writer

import (
	"database/sql"
	"fmt"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/sipcapture/homer-core/src/config"
	"github.com/sipcapture/homer-core/src/decoder"
	"github.com/sipcapture/homer-core/src/remotelog"
	"github.com/sipcapture/homer-core/src/scripting"
	"github.com/sipcapture/homer-core/src/storage/ducklake"
	"github.com/sipcapture/homer-core/src/stream/hepstream"
	logger "github.com/sipcapture/homer-core/src/utils/logging"
	"github.com/sipcapture/homer-core/src/utils/metrics"
)

// Writer is the main HEP writer module
type Writer struct {
	ingestConfig     *config.IngestConfig
	storageConfig    *config.StorageConfig
	prometheusConfig *config.PrometheusConfig
	remoteLogConfig  *config.RemoteLogConfig
	inputCh          chan incomingPacket
	buffer           *sync.Pool
	wg               *sync.WaitGroup
	exitWorker       chan struct{}
	quit             chan struct{}
	stopped          uint32
	stopOnce         sync.Once
	workersStarted   uint32
	statsStarted     uint32
	stats            Stats
	ducklakeManager  *ducklake.Manager
	decoder          *decoder.Decoder
	scriptEngine     scripting.ScriptEngine
	sipMetrics       *metrics.SIPMetricsProcessor

	// Remote logging clients
	lokiClient          *remotelog.Loki
	elasticsearchClient *remotelog.Elasticsearch
	lineprotoClient     *remotelog.LineProto

	// Pipeline profiling (atomic counters, always active)
	profile pipelineProfile

	// Batched Prometheus flush interval (packets per worker); set from ingest config.
	metricsFlushPackets int

	// Active connection count (TCP, TLS - connection-oriented protocols)
	connCount int64

	// Services
	compactionService *CompactionService
	tieringService    *TieringService
	tieredStorage     *ducklake.TieredStorageManager

	// Receivers
	udpServer   *UDPServer
	tcpServer   *TCPServer
	tlsServer   *TLSServer
	httpServer  *HTTPServer
	httpsServer *HTTPSServer

	// broker is the optional live-stream publisher. Nil when
	// ingest.hep_stream.enable is false; Publish is a no-op on nil.
	broker *hepstream.Broker
}

// SetBroker wires a live-stream broker into the writer's hot path.
// Must be called before Start(); passing nil keeps the feature off.
func (w *Writer) SetBroker(b *hepstream.Broker) {
	w.broker = b
}

// Broker returns the broker wired into this writer (nil when the
// feature is disabled). Exposed so main.go can share the single
// process-local broker between Writer, Node and Coordinator without
// constructing it twice.
func (w *Writer) Broker() *hepstream.Broker {
	return w.broker
}

type incomingPacket struct {
	data       []byte
	protocol   string
	receivedAt time.Time
}

// Stats holds packet processing statistics
type Stats struct {
	PktCount  uint64 // received (all packets that arrived)
	DropCount uint64 // dropped: queue full
	ErrCount  uint64 // dropped: decode error
	DupCount  uint64 // dropped: filtered (invalid proto, script discard)
	HEPCount  uint64 // processed and written to storage
}

const maxPktLen = 65507

// defaultWorkerMetricsFlushPackets is used when ingest.worker_metrics_flush_packets is 0.
const defaultWorkerMetricsFlushPackets = 128

// applyAzureDuckLakeConfig copies storage.ducklake.azure into duckCfg when
// any Azure field is set — including the Managed Identity case where only
// AccountName is set (no key, no connection string). Extracted out of New
// so this specific gate is unit-testable without running the full writer
// constructor, which immediately does real DuckLake I/O.
func applyAzureDuckLakeConfig(duckCfg *ducklake.Config, az config.AzureConfig) {
	if az.AccountName == "" && az.AccountKey == "" && az.ConnectionString == "" {
		return
	}
	duckCfg.AzureAccountName = az.AccountName
	duckCfg.AzureAccountKey = az.AccountKey
	duckCfg.AzureConnectionString = az.ConnectionString
	duckCfg.AzureEndpoint = az.Endpoint
}

// New creates a new Writer module
func New(ingestCfg *config.IngestConfig, storageCfg *config.StorageConfig, promCfg *config.PrometheusConfig, remoteLogCfg *config.RemoteLogConfig) (*Writer, error) {
	queueSize := ingestCfg.QueueSize
	if queueSize <= 0 {
		queueSize = 40000
	}

	w := &Writer{
		ingestConfig:     ingestCfg,
		storageConfig:    storageCfg,
		prometheusConfig: promCfg,
		remoteLogConfig:  remoteLogCfg,
		inputCh:          make(chan incomingPacket, queueSize),
		buffer:           &sync.Pool{New: func() interface{} { return make([]byte, maxPktLen) }},
		wg:               &sync.WaitGroup{},
		quit:             make(chan struct{}),
		exitWorker:       make(chan struct{}),
	}

	mfp := ingestCfg.WorkerMetricsFlushPackets
	if mfp <= 0 {
		mfp = defaultWorkerMetricsFlushPackets
	} else if mfp > 1<<20 {
		mfp = 1 << 20
	}
	w.metricsFlushPackets = mfp

	// Initialize decoder with ingest config
	w.decoder = decoder.NewDecoder(&decoder.DecoderConfig{
		HepV2Enable:     ingestCfg.HEP.HepV2Enable,
		HepV3Enable:     ingestCfg.HEP.HepV3Enable,
		ProtobufEnable:  ingestCfg.HEP.ProtobufEnable,
		Deduplicate:     ingestCfg.HEP.Deduplicate,
		AlegIDs:         ingestCfg.SIP.AlegIDs,
		CustomHeaders:   ingestCfg.SIP.CustomHeaders,
		ForceALegID:     ingestCfg.SIP.ForceALegID,
		CensorMethods:   ingestCfg.SIP.CensorMethods,
		DiscardMethods:  ingestCfg.SIP.DiscardMethods,
		ForceHEPPayload: ingestCfg.ForceHEPPayload,
	})

	// Initialize scripting engine if enabled
	if ingestCfg.Scripting.Enable {
		scriptCfg := &scripting.ScriptConfig{
			Enable:    ingestCfg.Scripting.Enable,
			Engine:    ingestCfg.Scripting.Engine,
			Folder:    ingestCfg.Scripting.Folder,
			HEPFilter: ingestCfg.Scripting.HEPFilter,
		}
		if remoteLogCfg != nil {
			scriptCfg.LokiCustomLabels = remoteLogCfg.Loki.LokiCustomLabels
		}
		scriptEngine, err := scripting.NewScriptEngine(scriptCfg)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize script engine: %w", err)
		}
		w.scriptEngine = scriptEngine
		logger.Info("Writer: Lua scripting engine initialized")
	}

	// Initialize SIP metrics processor if Prometheus is enabled
	if promCfg != nil && promCfg.Enable {
		w.sipMetrics = metrics.NewSIPMetricsProcessor(promCfg.TargetIP, promCfg.TargetName, promCfg.AgentLabel)
		logger.Info("Writer: SIP metrics processor initialized", "agent_label", config.NormalizePrometheusAgentLabel(promCfg.AgentLabel))
	}

	// Initialize remote logging clients
	if remoteLogCfg != nil {
		// Loki
		if remoteLogCfg.Loki.Enable {
			lokiCfg := &remotelog.LokiConfig{
				Enable:       remoteLogCfg.Loki.Enable,
				URL:          remoteLogCfg.Loki.URL,
				Bulk:         remoteLogCfg.Loki.Bulk,
				Timer:        remoteLogCfg.Loki.Timer,
				Buffer:       remoteLogCfg.Loki.Buffer,
				HEPFilter:    remoteLogCfg.Loki.HEPFilter,
				IPPortLabels: remoteLogCfg.Loki.IPPortLabels,
			}
			loki, err := remotelog.NewLoki(lokiCfg)
			if err != nil {
				logger.Warn(fmt.Sprintf("Writer: Failed to initialize Loki client: %v", err))
			} else {
				w.lokiClient = loki
			}
		}

		// Elasticsearch
		if remoteLogCfg.Elasticsearch.Enable {
			esCfg := &remotelog.ElasticsearchConfig{
				Enable:     remoteLogCfg.Elasticsearch.Enable,
				Addr:       remoteLogCfg.Elasticsearch.Addr,
				User:       remoteLogCfg.Elasticsearch.User,
				Pass:       remoteLogCfg.Elasticsearch.Pass,
				Discovery:  remoteLogCfg.Elasticsearch.Discovery,
				IndexDaily: remoteLogCfg.Elasticsearch.IndexDaily,
				IndexName:  remoteLogCfg.Elasticsearch.IndexName,
				HEPFilter:  remoteLogCfg.Elasticsearch.HEPFilter,
			}
			es, err := remotelog.NewElasticsearch(esCfg)
			if err != nil {
				logger.Warn(fmt.Sprintf("Writer: Failed to initialize Elasticsearch client: %v", err))
			} else {
				w.elasticsearchClient = es
			}
		}

		// LineProto
		if remoteLogCfg.LineProto.Enable {
			lpCfg := &remotelog.LineProtoConfig{
				Enable:    remoteLogCfg.LineProto.Enable,
				URL:       remoteLogCfg.LineProto.URL,
				Bulk:      remoteLogCfg.LineProto.Bulk,
				Timer:     remoteLogCfg.LineProto.Timer,
				Buffer:    remoteLogCfg.LineProto.Buffer,
				HEPFilter: remoteLogCfg.LineProto.HEPFilter,
			}
			lp, err := remotelog.NewLineProto(lpCfg)
			if err != nil {
				logger.Warn(fmt.Sprintf("Writer: Failed to initialize LineProto client: %v", err))
			} else {
				w.lineprotoClient = lp
			}
		}
	}

	// Set initial queue metrics
	metrics.SetWorkerQueueCapacity(float64(cap(w.inputCh)))

	// Initialize DuckLake storage
	duckCfg := ducklake.Config{
		CatalogType:          ducklake.CatalogType(storageCfg.DuckLake.CatalogType),
		CatalogPath:          storageCfg.DuckLake.CatalogPath,
		DataPath:             storageCfg.DuckLake.DataPath,
		LakeName:             storageCfg.DuckLake.LakeName,
		BatchSize:            storageCfg.DuckLake.BatchSize,
		FlushInterval:        time.Duration(storageCfg.DuckLake.FlushInterval) * time.Second,
		SearchBuffer:         storageCfg.DuckLake.SearchBuffer,
		ShardCount:           storageCfg.DuckLake.ShardCount,
		FlushQueue:           storageCfg.DuckLake.FlushQueue,
		DataInliningRowLimit: storageCfg.DuckLake.DataInliningRowLimit,
		TuningThreads:        storageCfg.DuckLake.Tuning.Threads,
		TuningMemoryLimit:    storageCfg.DuckLake.Tuning.MemoryLimit,
		TuningTempDirectory:  storageCfg.DuckLake.Tuning.TempDirectory,
		// Writer path: take an exclusive catalog lock so a second writer process
		// (e.g. a duplicate container) refuses to start instead of corrupting
		// the SQLite catalog.
		ExclusiveLock: true,
		// Autofix duplicate snapshot/table rows on startup (default on).
		AutoRepairCatalog: storageCfg.DuckLake.AutoRepairCatalog == nil ||
			*storageCfg.DuckLake.AutoRepairCatalog,
	}

	// S3 config
	if storageCfg.DuckLake.S3.AccessKeyID != "" {
		duckCfg.S3Region = storageCfg.DuckLake.S3.Region
		duckCfg.S3AccessKeyID = storageCfg.DuckLake.S3.AccessKeyID
		duckCfg.S3SecretAccessKey = storageCfg.DuckLake.S3.SecretAccessKey
		duckCfg.S3Endpoint = storageCfg.DuckLake.S3.Endpoint
		duckCfg.S3UseSSL = storageCfg.DuckLake.S3.UseSSL
		duckCfg.S3URLStyle = storageCfg.DuckLake.S3.URLStyle
	}

	// Azure config
	applyAzureDuckLakeConfig(&duckCfg, storageCfg.DuckLake.Azure)

	duckMgr, err := ducklake.NewManager(duckCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create DuckLake manager: %w", err)
	}
	w.ducklakeManager = duckMgr

	// Initialize receivers based on ingest config
	if ingestCfg.UDP.Enable {
		w.udpServer = NewUDPServer(w, &ingestCfg.UDP)
	}
	if ingestCfg.TCP.Enable {
		w.tcpServer = NewTCPServer(w, &ingestCfg.TCP)
	}
	if ingestCfg.TLS.Enable {
		w.tlsServer = NewTLSServer(w, &ingestCfg.TLS)
	}
	if ingestCfg.HTTP.Enable {
		w.httpServer = NewHTTPServer(w, &ingestCfg.HTTP)
	}
	if ingestCfg.HTTPS.Enable {
		w.httpsServer = NewHTTPSServer(w, &ingestCfg.HTTPS)
	}

	return w, nil
}

// Start starts the writer module
func (w *Writer) Start() error {
	// Start DuckLake manager
	if err := w.ducklakeManager.Start(); err != nil {
		return fmt.Errorf("failed to start DuckLake manager: %w", err)
	}
	logger.Info("Writer: DuckLake storage started",
		"catalog", w.storageConfig.DuckLake.CatalogPath,
		"data", w.storageConfig.DuckLake.DataPath)

	// Start compaction service if enabled (on primary shard only — each shard
	// has its own catalog, but compaction on the primary covers the main workload;
	// for multi-shard compaction, a per-shard loop can be added later).
	//
	// When multi-volume storage_policy is active with a local volume (hot parquet),
	// compaction is always enabled on the writer DuckLake manager — hot data
	// accumulates small files and cannot be safely turned off for that mode.
	// The compaction service is started unconditionally: when compaction is
	// enabled it runs the full merge/expire/cleanup cycle, otherwise it runs a
	// lightweight inline-flush-only loop so inlined-data backlog still drains
	// (disabling inlining alone does not flush rows inlined earlier).
	compactionEnable := w.storageConfig.DuckLake.Compaction.Enable || w.shouldAutoEnableCompactionForTieredHot()
	{
		compactionCfg := CompactionConfig{
			Enable:                    compactionEnable,
			CheckIntervalSec:          w.storageConfig.DuckLake.Compaction.CheckIntervalSec,
			RetentionDays:             w.storageConfig.DuckLake.Compaction.RetentionDays,
			RetentionDaysByTable:      w.storageConfig.DuckLake.Compaction.RetentionDaysByTable,
			RetentionUnit:             w.storageConfig.DuckLake.Compaction.RetentionUnit,
			SnapshotExpireIntervalSec: w.storageConfig.DuckLake.Compaction.SnapshotExpireIntervalSec,
			MinAgeSec:                 w.storageConfig.DuckLake.Compaction.MinAgeSec,
			MinFileSizeBytes:          w.storageConfig.DuckLake.Compaction.MinFileSizeBytes,
			MaxFileSizeBytes:          w.storageConfig.DuckLake.Compaction.MaxFileSizeBytes,
			MaxCompactedFiles:         w.storageConfig.DuckLake.Compaction.MaxCompactedFiles,
			Engine:                    w.storageConfig.DuckLake.Compaction.Engine,
			TargetFileSizeBytes:       w.storageConfig.DuckLake.Compaction.TargetFileSizeBytes,
		}
		if !w.storageConfig.DuckLake.Compaction.Enable && w.shouldAutoEnableCompactionForTieredHot() {
			logger.Info("Writer: DuckLake compaction auto-enabled (tiered storage with local hot volume)",
				"lake", w.ducklakeManager.GetLakeName())
		}
		if compactionCfg.CheckIntervalSec <= 0 {
			compactionCfg.CheckIntervalSec = 3600 // default 1 hour
		}
		if compactionCfg.SnapshotExpireIntervalSec <= 0 {
			compactionCfg.SnapshotExpireIntervalSec = 3600 // default 1 hour
		}
		if compactionCfg.MinAgeSec <= 0 {
			// Short enough that a continuously busy writer still consolidates
			// during the day: partitions are keyed by date, so an hour of quiet
			// never arrives under steady ingest.
			compactionCfg.MinAgeSec = defaultCompactionMinAgeSec
		}
		if compactionCfg.MaxCompactedFiles <= 0 {
			// Unbounded merge rewrites every small parquet of a table in one
			// CALL. 32 operations/cycle is a drain-vs-lock compromise
			// (sipcapture/homer#945); peak memory is bounded by
			// MaxFileSizeBytes and merge threads=1, not by this count.
			compactionCfg.MaxCompactedFiles = defaultDuckDBMaxCompactedFiles
		}
		if compactionCfg.MaxFileSizeBytes <= 0 {
			// Without max_file_size DuckLake defaults to target_file_size
			// and will try to rewrite already-large files. Bound inputs so
			// a merge group stays within memory_limit.
			compactionCfg.MaxFileSizeBytes = defaultDuckDBMaxFileSizeBytes
		}
		var compactionS3 *CompactionS3Client
		var compactionAzure *CompactionAzureClient
		if ducklake.IsRemoteLakeDataPath(w.storageConfig.DuckLake.DataPath) {
			s := w.storageConfig.DuckLake.S3
			if ak := strings.TrimSpace(s.AccessKeyID); ak != "" {
				compactionS3 = &CompactionS3Client{
					Region:          s.Region,
					AccessKeyID:     ak,
					SecretAccessKey: s.SecretAccessKey,
					Endpoint:        s.Endpoint,
					UseSSL:          s.UseSSL,
					URLStyle:        s.URLStyle,
				}
			}
			az := w.storageConfig.DuckLake.Azure
			if az.AccountName != "" || az.AccountKey != "" || az.ConnectionString != "" {
				compactionAzure = &CompactionAzureClient{
					AccountName:      az.AccountName,
					AccountKey:       az.AccountKey,
					ConnectionString: az.ConnectionString,
					Endpoint:         az.Endpoint,
				}
			}
		}
		w.compactionService = NewCompactionService(
			w.ducklakeManager.GetDB(),
			w.ducklakeManager.GetLakeName(),
			w.storageConfig.DuckLake.DataPath,
			w.storageConfig.DuckLake.CatalogPath,
			compactionCfg,
			w.ducklakeManager,
			compactionS3,
			compactionAzure,
		)
		if err := w.compactionService.Start(); err != nil {
			logger.Error(fmt.Sprintf("Writer: Failed to start compaction service: %v", err))
		}
	}

	// Start tiering service if storage policy is enabled
	if w.storageConfig.DuckLake.StoragePolicy.Enable && len(w.storageConfig.DuckLake.StoragePolicy.Volumes) > 1 {
		if err := w.startTieringService(); err != nil {
			logger.Error(fmt.Sprintf("Writer: Failed to start tiering service: %v", err))
		}
	}

	// Start workers
	// Auto-detect: NumCPU/2, minimum 2 (even on single-core hosts/containers), maximum 4.
	numWorkers := w.ingestConfig.WorkerCount
	if numWorkers <= 0 {
		numWorkers = runtime.NumCPU() / 2
		if numWorkers < 2 {
			numWorkers = 2
		}
		if numWorkers > 4 {
			numWorkers = 4
		}
	}

	for n := 0; n < numWorkers; n++ {
		w.wg.Add(1)
		go w.worker()
	}
	atomic.StoreUint32(&w.workersStarted, 1)
	logger.Info("Writer: Started workers", "count", numWorkers)

	// Start stats logger
	atomic.StoreUint32(&w.statsStarted, 1)
	go w.logStats()

	// Start receivers
	if w.udpServer != nil {
		go func() {
			if err := w.udpServer.Start(); err != nil {
				logger.Error(fmt.Sprintf("Writer: UDP server error: %v", err))
			}
		}()
	}
	if w.tcpServer != nil {
		go func() {
			if err := w.tcpServer.Start(); err != nil {
				logger.Error(fmt.Sprintf("Writer: TCP server error: %v", err))
			}
		}()
	}
	if w.tlsServer != nil {
		go func() {
			if err := w.tlsServer.Start(); err != nil {
				logger.Error(fmt.Sprintf("Writer: TLS server error: %v", err))
			}
		}()
	}
	if w.httpServer != nil {
		go func() {
			if err := w.httpServer.Start(); err != nil {
				logger.Error(fmt.Sprintf("Writer: HTTP server error: %v", err))
			}
		}()
	}
	if w.httpsServer != nil {
		go func() {
			if err := w.httpsServer.Start(); err != nil {
				logger.Error(fmt.Sprintf("Writer: HTTPS server error: %v", err))
			}
		}()
	}

	logger.Info("Writer module started")
	return nil
}

// Stop stops the writer module
func (w *Writer) Stop() error {
	w.stopOnce.Do(func() {
		atomic.StoreUint32(&w.stopped, 1)

		// Stop receivers
		if w.udpServer != nil {
			w.udpServer.Stop()
		}
		if w.tcpServer != nil {
			w.tcpServer.Stop()
		}
		if w.tlsServer != nil {
			w.tlsServer.Stop()
		}
		if w.httpServer != nil {
			w.httpServer.Stop()
		}
		if w.httpsServer != nil {
			w.httpsServer.Stop()
		}

		// Stop compaction service
		if w.compactionService != nil {
			w.compactionService.Stop()
		}

		// Stop tiering service
		if w.tieringService != nil {
			w.tieringService.Stop()
		}
		if w.tieredStorage != nil {
			w.tieredStorage.Stop()
		}

		// Stop script engine
		if w.scriptEngine != nil {
			w.scriptEngine.Close()
		}

		// Stop remote logging clients
		if w.lokiClient != nil {
			w.lokiClient.Close()
		}
		if w.elasticsearchClient != nil {
			w.elasticsearchClient.Close()
		}
		if w.lineprotoClient != nil {
			w.lineprotoClient.Close()
		}

		if atomic.LoadUint32(&w.workersStarted) == 1 {
			// Drain remaining packets from the queue before stopping workers.
			// All receivers are stopped above, so no new packets will arrive.
		drainLoop:
			for {
				select {
				case <-w.inputCh:
					// discard
				default:
					break drainLoop
				}
			}

			logger.Info("Writer: stopping workers...")
			close(w.exitWorker)
			w.wg.Wait()
			logger.Info("Writer: workers stopped")
		}

		if atomic.LoadUint32(&w.statsStarted) == 1 {
			close(w.quit)
		}

		// Stop DuckLake storage
		logger.Info("Writer: stopping DuckLake storage...")
		if err := w.ducklakeManager.Stop(); err != nil {
			logger.Error(fmt.Sprintf("Writer: Failed to stop DuckLake manager: %v", err))
		}
		logger.Info("Writer: DuckLake storage stopped")
		logger.Info("Writer module stopped")
	})
	return nil
}

// Reload reloads the writer module configuration (scripts, etc.)
func (w *Writer) Reload() error {
	logger.Info("Writer: Reloading configuration...")

	// Reload Lua scripts if script engine is enabled
	if w.scriptEngine != nil {
		if luaEngine, ok := w.scriptEngine.(*scripting.LuaEngine); ok {
			if err := luaEngine.Reload(); err != nil {
				logger.Error(fmt.Sprintf("Writer: Failed to reload Lua scripts: %v", err))
				return err
			}
			logger.Info("Writer: Lua scripts reloaded")
		}
	}

	// Reload target mapping for SIP metrics
	if w.sipMetrics != nil && w.prometheusConfig != nil {
		newProcessor := metrics.NewSIPMetricsProcessor(
			w.prometheusConfig.TargetIP,
			w.prometheusConfig.TargetName,
			w.prometheusConfig.AgentLabel,
		)
		w.sipMetrics = newProcessor
		logger.Info("Writer: SIP metrics target mapping reloaded",
			"agent_label", config.NormalizePrometheusAgentLabel(w.prometheusConfig.AgentLabel))
	}

	logger.Info("Writer: Configuration reloaded")
	return nil
}

// EnqueuePacket adds a packet to the processing queue
func (w *Writer) EnqueuePacket(data []byte, protocol string) {
	if atomic.LoadUint32(&w.stopped) == 1 {
		return
	}

	atomic.AddUint64(&w.stats.PktCount, 1)
	if protocol == "" {
		protocol = "unknown"
	}
	receivedAt := time.Now()

	// Copy data to buffer from pool
	buf := w.buffer.Get().([]byte)
	n := copy(buf, data)

	select {
	case w.inputCh <- incomingPacket{
		data:       buf[:n],
		protocol:   protocol,
		receivedAt: receivedAt,
	}:
		metrics.SetWorkerQueueDepth(float64(len(w.inputCh)))
	default:
		// Queue full, drop packet
		atomic.AddUint64(&w.stats.DropCount, 1)
		w.buffer.Put(buf[:maxPktLen])
		metrics.RecordHEPPacketFailed(protocol, "queue_full")
		metrics.RecordPipelineStageError(protocol, "enqueue", "queue_full")
	}
}

// workerMetrics holds per-worker counters that are flushed to Prometheus
// in batches to avoid per-packet atomic contention.
type workerMetrics struct {
	received  int64
	processed int64
	bytes     int64
	count     int
}

func (wm *workerMetrics) flush(protocol string) {
	if wm.count == 0 {
		return
	}
	metrics.HEPPacketsReceived.WithLabelValues(protocol).Add(float64(wm.received))
	metrics.HEPPacketsProcessed.WithLabelValues(protocol).Add(float64(wm.processed))
	metrics.BytesReceived.WithLabelValues(protocol).Add(float64(wm.bytes))
	wm.received = 0
	wm.processed = 0
	wm.bytes = 0
	wm.count = 0
}

// pipelineProfile accumulates per-stage nanosecond durations across all workers.
// Flushed periodically by logStats. All fields are accessed via atomic ops.
type pipelineProfile struct {
	hepParseNs uint64
	sipParseNs uint64
	adapterNs  uint64
	totalNs    uint64
	sampleCnt  uint64
}

// worker processes packets from the queue
func (w *Writer) worker() {
	defer w.wg.Done()

	var wm workerMetrics
	lastProto := ""

	for {
		select {
		case <-w.exitWorker:
			if lastProto != "" {
				wm.flush(lastProto)
			}
			return
		case msg, ok := <-w.inputCh:
			if !ok {
				if lastProto != "" {
					wm.flush(lastProto)
				}
				return
			}

			protocol := msg.protocol
			if protocol == "" {
				protocol = "unknown"
			}

			// Flush batched metrics on protocol change or interval.
			if protocol != lastProto && lastProto != "" {
				wm.flush(lastProto)
			}
			lastProto = protocol

			wm.received++
			wm.bytes += int64(len(msg.data))
			wm.count++

			sample := wm.count&63 == 0

			hepPkt, err := w.decoder.Decode(msg.data)
			if err != nil {
				atomic.AddUint64(&w.stats.ErrCount, 1)
				metrics.RecordHEPPacketFailed(protocol, "decode_error")
				w.buffer.Put(msg.data[:cap(msg.data)])
				if wm.count >= w.metricsFlushPackets {
					wm.flush(protocol)
				}
				continue
			}

			if hepPkt.ProtoType == 0 {
				atomic.AddUint64(&w.stats.DupCount, 1)
				metrics.RecordHEPPacketFailed(protocol, "invalid_proto")
				decoder.ReleaseHEP(hepPkt)
				w.buffer.Put(msg.data[:cap(msg.data)])
				if wm.count >= w.metricsFlushPackets {
					wm.flush(protocol)
				}
				continue
			}

			// Run script engine if enabled
			if w.scriptEngine != nil {
				if err := w.scriptEngine.Run(hepPkt); err != nil {
					logger.Debug(fmt.Sprintf("Writer: Script error: %v", err))
				}
				if hepPkt.ProtoType == 0 {
					atomic.AddUint64(&w.stats.DupCount, 1)
					metrics.RecordHEPPacketFailed(protocol, "script_discard")
					decoder.ReleaseHEP(hepPkt)
					w.buffer.Put(msg.data[:cap(msg.data)])
					if wm.count >= w.metricsFlushPackets {
						wm.flush(protocol)
					}
					continue
				}
			}

			atomic.AddUint64(&w.stats.HEPCount, 1)
			wm.processed++

			// Live stream tap. Runs before the Loki/ES/DuckLake writes
			// so subscribers see events at the same point the storage
			// pipeline does, and so a slow WS client cannot back-
			// pressure any of those writers (Publish is select+default
			// on each subscriber's queue). No-op when broker is nil.
			if w.broker != nil {
				w.broker.Publish(hepstream.FromHEP(hepPkt))
			}

			// Process SIP metrics
			if w.sipMetrics != nil {
				w.processSIPMetrics(hepPkt)
			}

			// Send to remote logging backends
			if w.lokiClient != nil {
				if err := w.lokiClient.Send(hepPkt); err != nil {
					logger.Debug(fmt.Sprintf("Writer: Loki send error: %v", err))
				}
			}
			if w.elasticsearchClient != nil {
				if err := w.elasticsearchClient.Send(hepPkt); err != nil {
					logger.Debug(fmt.Sprintf("Writer: Elasticsearch send error: %v", err))
				}
			}
			if w.lineprotoClient != nil {
				if err := w.lineprotoClient.Send(hepPkt); err != nil {
					logger.Debug(fmt.Sprintf("Writer: LineProto send error: %v", err))
				}
			}

			var adapterStart int64
			if sample {
				adapterStart = time.Now().UnixNano()
			}

			// Write to DuckLake storage
			if err := w.ducklakeManager.WriteHEP(hepPkt); err != nil {
				logger.Debug(fmt.Sprintf("Writer: Failed to write HEP to DuckLake: %v", err))
				metrics.RecordPipelineStageError(protocol, "ducklake_write", "write_error")
			}

			if sample {
				adapterEnd := time.Now().UnixNano()
				atomic.AddUint64(&w.profile.hepParseNs, uint64(hepPkt.HEPParseNs))
				atomic.AddUint64(&w.profile.sipParseNs, uint64(hepPkt.SIPParseNs))
				atomic.AddUint64(&w.profile.adapterNs, uint64(adapterEnd-adapterStart))
				atomic.AddUint64(&w.profile.totalNs, uint64(hepPkt.HEPParseNs)+uint64(hepPkt.SIPParseNs)+uint64(adapterEnd-adapterStart))
				atomic.AddUint64(&w.profile.sampleCnt, 1)
			}

			decoder.ReleaseHEP(hepPkt)
			w.buffer.Put(msg.data[:cap(msg.data)])

			if wm.count >= w.metricsFlushPackets {
				wm.flush(protocol)
			}
		}
	}
}

// processSIPMetrics processes SIP-specific metrics for the HEP packet
func (w *Writer) processSIPMetrics(hepPkt *decoder.HEP) {
	if w.sipMetrics == nil {
		return
	}

	nodeID := fmt.Sprintf("%d", hepPkt.NodeID)
	nodeName := hepPkt.NodeName

	// Process SIP packets (ProtoType 1)
	if hepPkt.ProtoType == 1 && hepPkt.SIP != nil {
		pkt := &metrics.SIPPacketInfo{
			NodeName:      nodeName,
			NodeID:        nodeID,
			ProtoType:     hepPkt.ProtoType,
			ProtoString:   hepPkt.ProtoString,
			PayloadSize:   len(hepPkt.Payload),
			SrcIP:         hepPkt.SrcIP,
			DstIP:         hepPkt.DstIP,
			TimestampNano: hepPkt.Timestamp.UnixNano(),
			CallID:        hepPkt.SIP.CallID,
			Method:        hepPkt.SIP.FirstMethod,
			Response:      hepPkt.SIP.FirstMethod,
			CseqMethod:    hepPkt.SIP.CseqMethod,
			ReasonVal:     hepPkt.SIP.ReasonVal,
			RTPStatVal:    hepPkt.SIP.RTPStatVal,
		}
		// If it's a response, set Response to response code
		if hepPkt.SIP.FirstMethod == "" && hepPkt.SIP.FirstResp != "" {
			pkt.Response = hepPkt.SIP.FirstResp
		}
		w.sipMetrics.ProcessSIPPacket(pkt)
	}

	// Process RTCP packets (ProtoType 5)
	if hepPkt.ProtoType == 5 {
		w.sipMetrics.ProcessRTCPPacket(nodeID, nodeName, hepPkt.SrcIP, hepPkt.DstIP, []byte(hepPkt.Payload))
	}

	// Process RTPAgent packets (ProtoType 34)
	if hepPkt.ProtoType == 34 {
		w.sipMetrics.ProcessRTPAgentPacket(nodeID, nodeName, []byte(hepPkt.Payload))
	}
}

// logStats periodically logs statistics
func (w *Writer) logStats() {
	statsTicker := time.NewTicker(5 * time.Minute)
	profileTicker := time.NewTicker(10 * time.Second)
	defer statsTicker.Stop()
	defer profileTicker.Stop()

	for {
		select {
		case <-statsTicker.C:
			received := atomic.LoadUint64(&w.stats.PktCount)
			dropCount := atomic.LoadUint64(&w.stats.DropCount)
			errCount := atomic.LoadUint64(&w.stats.ErrCount)
			dupCount := atomic.LoadUint64(&w.stats.DupCount)
			processed := atomic.LoadUint64(&w.stats.HEPCount)
			dropped := dropCount + errCount + dupCount

			var bufferRows int
			var tableCount int
			if w.ducklakeManager != nil {
				bufferRows, tableCount = w.ducklakeManager.GetBufferStats()
			}

			conns := atomic.LoadInt64(&w.connCount)

			// Go runtime memory breakdown
			var ms runtime.MemStats
			runtime.ReadMemStats(&ms)
			goHeapMB := ms.HeapInuse / (1024 * 1024)
			goStackMB := ms.StackInuse / (1024 * 1024)
			goSysMB := ms.Sys / (1024 * 1024)
			numGoroutines := runtime.NumGoroutine()

			logger.Info("Writer stats (5min)",
				"received", received,
				"dropped", dropped,
				"drop_queue", dropCount,
				"drop_err", errCount,
				"drop_dup", dupCount,
				"processed", processed,
				"connections", conns,
				"ducklake_buf_rows", bufferRows,
				"ducklake_tables", tableCount,
				"go_heap_mb", goHeapMB,
				"go_stack_mb", goStackMB,
				"go_sys_mb", goSysMB,
				"goroutines", numGoroutines,
				"queue_len", len(w.inputCh),
				"queue_cap", cap(w.inputCh),
			)

			atomic.StoreUint64(&w.stats.PktCount, 0)
			atomic.StoreUint64(&w.stats.DropCount, 0)
			atomic.StoreUint64(&w.stats.ErrCount, 0)
			atomic.StoreUint64(&w.stats.DupCount, 0)
			atomic.StoreUint64(&w.stats.HEPCount, 0)

		case <-profileTicker.C:
			cnt := atomic.SwapUint64(&w.profile.sampleCnt, 0)
			if cnt == 0 {
				continue
			}
			hepNs := atomic.SwapUint64(&w.profile.hepParseNs, 0)
			sipNs := atomic.SwapUint64(&w.profile.sipParseNs, 0)
			adpNs := atomic.SwapUint64(&w.profile.adapterNs, 0)
			totNs := atomic.SwapUint64(&w.profile.totalNs, 0)

			avgHep := float64(hepNs) / float64(cnt) / 1000.0
			avgSip := float64(sipNs) / float64(cnt) / 1000.0
			avgAdp := float64(adpNs) / float64(cnt) / 1000.0
			avgTot := float64(totNs) / float64(cnt) / 1000.0

			var hepPct, sipPct, adpPct float64
			if totNs > 0 {
				hepPct = float64(hepNs) * 100.0 / float64(totNs)
				sipPct = float64(sipNs) * 100.0 / float64(totNs)
				adpPct = float64(adpNs) * 100.0 / float64(totNs)
			}

			logger.Info("Pipeline profile",
				"samples", cnt,
				"hep_decode_us", fmt.Sprintf("%.1f", avgHep),
				"hep_pct", fmt.Sprintf("%.1f", hepPct),
				"sip_parse_us", fmt.Sprintf("%.1f", avgSip),
				"sip_pct", fmt.Sprintf("%.1f", sipPct),
				"adapter_us", fmt.Sprintf("%.1f", avgAdp),
				"adapter_pct", fmt.Sprintf("%.1f", adpPct),
				"total_us", fmt.Sprintf("%.1f", avgTot),
			)

		case <-w.quit:
			return
		}
	}
}

// startTieringService initializes and starts the tiering service
func (w *Writer) startTieringService() error {
	policy := w.storageConfig.DuckLake.StoragePolicy

	// Build volumes configuration
	volumes := make([]ducklake.Volume, len(policy.Volumes))
	for i, vol := range policy.Volumes {
		volumes[i] = ducklake.Volume{
			Name:                  vol.Name,
			Type:                  ducklake.VolumeType(vol.Type),
			Path:                  vol.Path,
			Priority:              vol.Priority,
			MaxDataAgeDays:        vol.MaxDataAgeDays,
			MaxSizeGB:             vol.MaxSizeGB,
			LakeName:              w.storageConfig.DuckLake.LakeName + "_" + vol.Name,
			S3Region:              vol.S3Region,
			S3AccessKey:           vol.S3AccessKeyID,
			S3SecretKey:           vol.S3SecretKey,
			S3Endpoint:            vol.S3Endpoint,
			S3UseSSL:              vol.S3UseSSL,
			S3URLStyle:            vol.S3URLStyle,
			AzureAccountName:      vol.AzureAccountName,
			AzureAccountKey:       vol.AzureAccountKey,
			AzureConnectionString: vol.AzureConnectionString,
			AzureEndpoint:         vol.AzureEndpoint,
			OverrideDataPath:      vol.OverrideDataPath,
		}
	}

	// Create tiered storage manager
	tsmConfig := ducklake.TieredStorageConfig{
		Enable:             policy.Enable,
		Volumes:            volumes,
		TTLMoveIntervalSec: policy.TTLMoveIntervalSec,
		MoveFactor:         policy.MoveFactor,
		ConcurrentMoves:    policy.ConcurrentMoves,
		MoveOnStartup:      policy.MoveOnStartup,
		MoveEngine:         policy.MoveEngine,
		CatalogType:        ducklake.CatalogType(w.storageConfig.DuckLake.CatalogType),
		CatalogPath:        w.storageConfig.DuckLake.CatalogPath,
		CatalogLocker:      w.ducklakeManager,
	}

	tsm, err := ducklake.NewTieredStorageManager(tsmConfig)
	if err != nil {
		return fmt.Errorf("failed to create tiered storage manager: %w", err)
	}

	if err := tsm.Start(); err != nil {
		return fmt.Errorf("failed to start tiered storage manager: %w", err)
	}
	w.tieredStorage = tsm

	// Create tiering service
	tieringCfg := TieringConfig{
		Enable:            policy.Enable,
		CheckIntervalSec:  policy.TTLMoveIntervalSec,
		ConcurrentMoves:   policy.ConcurrentMoves,
		MoveOnStartup:     policy.MoveOnStartup,
		MoveFactor:        policy.MoveFactor,
		SnapshotExpireSec: w.storageConfig.DuckLake.Compaction.SnapshotExpireIntervalSec,
	}
	if tieringCfg.CheckIntervalSec <= 0 {
		tieringCfg.CheckIntervalSec = 3600 // default 1 hour
	}
	if tieringCfg.ConcurrentMoves <= 0 {
		tieringCfg.ConcurrentMoves = 2
	}
	if tieringCfg.MoveFactor <= 0 {
		tieringCfg.MoveFactor = 0.8 // default 80%
	}

	w.tieringService = NewTieringService(tieringCfg, tsm)
	if err := w.tieringService.Start(); err != nil {
		return fmt.Errorf("failed to start tiering service: %w", err)
	}

	logger.Info("Writer: Tiering service started", "volumes", len(volumes))
	return nil
}

// shouldAutoEnableCompactionForTieredHot reports whether DuckLake compaction must
// stay on for the writer catalog: multi-volume storage_policy with at least one
// local volume (hot parquet). In that mode hot data churns many small files;
// compaction.enable=false is ignored so operators cannot accidentally disable it.
func (w *Writer) shouldAutoEnableCompactionForTieredHot() bool {
	p := w.storageConfig.DuckLake.StoragePolicy
	if !p.Enable || len(p.Volumes) < 2 {
		return false
	}
	for _, v := range p.Volumes {
		if strings.EqualFold(strings.TrimSpace(v.Type), "local") {
			return true
		}
	}
	return false
}

// GetDB returns the underlying DuckDB connection used by the writer.
// This allows the Node module to share the same DuckDB instance so that
// newly flushed data is immediately visible to queries.
func (w *Writer) GetDB() *sql.DB {
	if w.ducklakeManager != nil {
		return w.ducklakeManager.GetDB()
	}
	return nil
}

// GetTieredQueryDB returns the TieredStorageManager DuckDB when multi-volume
// tiering is active, or nil. Used by the Node for merged search across hot+cold.
func (w *Writer) GetTieredQueryDB() *sql.DB {
	if w.tieredStorage == nil {
		return nil
	}
	return w.tieredStorage.GetDB()
}

// GetDuckLakeManager returns the writer's DuckLake manager so that
// peer modules (e.g. the OTLP receiver) can reuse it for their own
// auxiliary tables instead of opening a second DuckDB.
func (w *Writer) GetDuckLakeManager() *ducklake.Manager {
	return w.ducklakeManager
}
