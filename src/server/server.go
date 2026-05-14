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
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/sipcapture/homer-core/src/decoder"
	"github.com/sipcapture/homer-core/src/homerconfig"
	"github.com/sipcapture/homer-core/src/storage/ducklake"
	"github.com/sipcapture/homer-core/src/stream/hepstream"
	logger "github.com/sipcapture/homer-core/src/utils/logging"
	"github.com/sipcapture/homer-core/src/utils/metrics"
)

type HEPInput struct {
	inputCh         chan incomingPacket
	dbCh            chan *decoder.HEP
	promCh          chan *decoder.HEP
	wg              *sync.WaitGroup
	buffer          *sync.Pool
	exitUDP         chan bool
	exitTCP         chan bool
	exitTLS         chan bool
	exitWS          chan bool
	exitWorker      chan bool
	quit            chan bool
	tlsStop         chan struct{}
	stopped         uint32
	workersStarted  uint32
	udpStarted      uint32
	tcpStarted      uint32
	tlsStarted      uint32
	stats           HEPStats
	useDB           bool
	usePM           bool
	useES           bool
	useLK           bool
	ducklakeManager *ducklake.Manager
	httpServer      *HTTPServer
	httpsServer     *HTTPSServer

	// broker is optional — set by main.go when ingest.hep_stream.enable
	// is true. Nil means "feature disabled"; Publish is a no-op on a
	// nil *Broker so the hot path stays branch-cheap (one nil check
	// per packet).
	broker *hepstream.Broker
}

// SetBroker wires a live-stream broker into the HEP ingest hot path.
// Called once from main.go after NewHEPInput and before the workers
// start. Passing nil is equivalent to not calling SetBroker at all.
func (h *HEPInput) SetBroker(b *hepstream.Broker) {
	h.broker = b
}

// Broker returns the configured hepstream.Broker (or nil when the
// live-stream feature is disabled). Exposed so the node module can
// subscribe to the same in-process broker that ingest publishes to.
func (h *HEPInput) Broker() *hepstream.Broker {
	return h.broker
}

type incomingPacket struct {
	data       []byte
	protocol   string
	receivedAt time.Time
}

type HEPStats struct {
	DupCount uint64
	ErrCount uint64
	HEPCount uint64
	PktCount uint64
}

const maxPktLen = 65507

func NewHEPInput() *HEPInput {
	queueSize := 200000
	if homerconfig.MainConfig != nil {
		if qs := homerconfig.MainConfig.Setting.SERVER_SETTINGS.QueueSize; qs > 0 {
			queueSize = qs
		}
	}

	h := &HEPInput{
		inputCh:    make(chan incomingPacket, queueSize),
		buffer:     &sync.Pool{New: func() interface{} { return make([]byte, maxPktLen) }},
		wg:         &sync.WaitGroup{},
		quit:       make(chan bool),
		exitUDP:    make(chan bool),
		exitTCP:    make(chan bool),
		exitTLS:    make(chan bool),
		exitWS:     make(chan bool),
		exitWorker: make(chan bool),
		tlsStop:    make(chan struct{}),
	}

	// Set initial queue metrics
	metrics.SetWorkerQueueCapacity(float64(cap(h.inputCh)))

	// Initialize DuckLake storage if enabled
	if homerconfig.MainConfig != nil && homerconfig.MainConfig.Setting.DUCKLAKE_SETTINGS.Enable {
		duckMgr, err := ducklake.NewManagerFromConfig()
		if err != nil {
			logger.Error(fmt.Sprintf("Failed to create DuckLake manager: %v", err))
		} else {
			h.ducklakeManager = duckMgr
			if err := h.ducklakeManager.Start(); err != nil {
				logger.Error(fmt.Sprintf("Failed to start DuckLake manager: %v", err))
				h.ducklakeManager = nil
			} else {
				logger.Info("DuckLake storage enabled", "catalog", homerconfig.MainConfig.Setting.DUCKLAKE_SETTINGS.CatalogPath, "data", homerconfig.MainConfig.Setting.DUCKLAKE_SETTINGS.DataPath)
			}
		}
	}

	// if len(homerconfig.MainConfig.Setting.DBAddr) > 2 {
	// 	h.useDB = true
	// 	h.dbCh = make(chan *decoder.HEP, homerconfig.MainConfig.Setting.DBBuffer)
	// }
	// if len(homerconfig.MainConfig.Setting.PromAddr) > 2 {
	// 	h.usePM = true
	// 	h.promCh = make(chan *decoder.HEP, 40000)
	// }
	return h
}

func (h *HEPInput) Run() {
	atomic.StoreUint32(&h.workersStarted, 1)

	numWorkers := resolveWorkerCount()

	for n := 0; n < numWorkers; n++ {
		h.wg.Add(1)
		go h.worker()
	}

	logger.Info("start", "version", homerconfig.SystemSettingsGlobal.VersionApp,
		"workers", numWorkers, "queue_size", cap(h.inputCh))
	go h.logStats()
	go h.reloadWorker()

	// if len(homerconfig.MainConfig.Setting.HEPAddr) > 2 {
	// 	go h.serveUDP(homerconfig.MainConfig.Setting.HEPAddr)
	// }

	// Start UDP server if enabled
	if homerconfig.MainConfig.Setting.SERVER_SETTINGS.UDP_SERVER.Enable {
		udpAddr := fmt.Sprintf("%s:%d", homerconfig.MainConfig.Setting.SERVER_SETTINGS.UDP_SERVER.Host, homerconfig.MainConfig.Setting.SERVER_SETTINGS.UDP_SERVER.Port)
		go h.serveUDP(udpAddr)
	}

	// Start TCP server if enabled
	if homerconfig.MainConfig.Setting.SERVER_SETTINGS.TCP_SERVER.Enable {
		go h.serveTCP(homerconfig.MainConfig.Setting.SERVER_SETTINGS.TCP_SERVER.Host, homerconfig.MainConfig.Setting.SERVER_SETTINGS.TCP_SERVER.Port, homerconfig.MainConfig.Setting.SERVER_SETTINGS.TCP_SERVER.Multicore)
	}

	// Start TLS server if enabled
	if homerconfig.MainConfig.Setting.SERVER_SETTINGS.TLS_SERVER.Enable {
		tlsSettings := &TLSSettings{
			Enable:             homerconfig.MainConfig.Setting.SERVER_SETTINGS.TLS_SERVER.Enable,
			Port:               homerconfig.MainConfig.Setting.SERVER_SETTINGS.TLS_SERVER.Port,
			Cert:               homerconfig.MainConfig.Setting.SERVER_SETTINGS.TLS_SERVER.Cert,
			Key:                homerconfig.MainConfig.Setting.SERVER_SETTINGS.TLS_SERVER.Key,
			CaCert:             homerconfig.MainConfig.Setting.SERVER_SETTINGS.TLS_SERVER.CaCert,
			MinTLSVersion:      homerconfig.MainConfig.Setting.SERVER_SETTINGS.TLS_SERVER.MinTLSVersion,
			MaxTLSVersion:      homerconfig.MainConfig.Setting.SERVER_SETTINGS.TLS_SERVER.MaxTLSVersion,
			MutualTLS:          homerconfig.MainConfig.Setting.SERVER_SETTINGS.TLS_SERVER.MutualTLS,
			InsecureSkipVerify: homerconfig.MainConfig.Setting.SERVER_SETTINGS.TLS_SERVER.InsecureSkipVerify,
		}
		go h.serveTLS(
			homerconfig.MainConfig.Setting.SERVER_SETTINGS.TLS_SERVER.Host,
			tlsSettings.Port,
			tlsSettings,
		)
	}

	// Start HTTP server for HEP packet reception if enabled (includes WebSocket support)
	if homerconfig.MainConfig.Setting.SERVER_SETTINGS.HTTP_SERVER.Enable {
		httpSrv := NewHTTPServer(h)
		h.httpServer = httpSrv
		go httpSrv.Run()
	}

	// Start HTTPS server for HEP packet reception if enabled (includes WebSocket support)
	if homerconfig.MainConfig.Setting.SERVER_SETTINGS.HTTPS_SERVER.Enable {
		httpsSrv := NewHTTPSServer(h)
		if httpsSrv != nil {
			h.httpsServer = httpsSrv
			go httpsSrv.Run()
		}
	}

	h.wg.Wait()
}

func (h *HEPInput) End() {
	if !atomic.CompareAndSwapUint32(&h.stopped, 0, 1) {
		return
	}

	if h.httpServer != nil {
		h.httpServer.End()
	}
	if h.httpsServer != nil {
		h.httpsServer.End()
	}

	close(h.tlsStop)

	if atomic.LoadUint32(&h.tlsStarted) == 1 {
		h.waitForServerStopBool(h.exitTLS, "TLS")
	}
	if atomic.LoadUint32(&h.tcpStarted) == 1 {
		h.waitForServerStopBool(h.exitTCP, "TCP")
	}
	if atomic.LoadUint32(&h.udpStarted) == 1 {
		h.waitForServerStopBool(h.exitUDP, "UDP")
	}

	if atomic.LoadUint32(&h.workersStarted) == 1 {
		h.exitWorker <- true
		<-h.exitWorker
	}

	// Stop DuckLake storage
	if h.ducklakeManager != nil {
		if err := h.ducklakeManager.Stop(); err != nil {
			logger.Error(fmt.Sprintf("Failed to stop DuckLake manager: %v", err))
		}
		logger.Info("DuckLake storage stopped")
	}

	if atomic.LoadUint32(&h.workersStarted) == 1 {
		h.quit <- true
		<-h.quit
	}
	close(h.inputCh)
}

func (h *HEPInput) waitForServerStopBool(ch chan bool, name string) {
	select {
	case <-ch:
	case <-time.After(3 * time.Second):
		logger.Warn(fmt.Sprintf("%s server shutdown timed out", name))
	}
}

// serverWorkerMetrics holds per-worker counters flushed in batches.
type serverWorkerMetrics struct {
	processed int64
	count     int
}

const serverMetricsFlushInterval = 128

func (wm *serverWorkerMetrics) flush(protocol string) {
	if wm.count == 0 {
		return
	}
	metrics.HEPPacketsProcessed.WithLabelValues(protocol).Add(float64(wm.processed))
	wm.processed = 0
	wm.count = 0
}

func (h *HEPInput) worker() {
	defer h.wg.Done()

	var ok bool
	var msg incomingPacket
	var wm serverWorkerMetrics
	lastProto := ""

	for {
		select {
		case <-h.exitWorker:
			if lastProto != "" {
				wm.flush(lastProto)
			}
			h.exitWorker <- true
			return
		case msg, ok = <-h.inputCh:
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
			if protocol != lastProto && lastProto != "" {
				wm.flush(lastProto)
			}
			lastProto = protocol
			wm.count++

			hepPkt, err := decoder.DecodeHEP(msg.data)
			if err != nil {
				atomic.AddUint64(&h.stats.ErrCount, 1)
				metrics.RecordHEPPacketFailed(protocol, "decode_error")
				if cap(msg.data) >= maxPktLen {
					h.buffer.Put(msg.data[:maxPktLen])
				}
				if wm.count >= serverMetricsFlushInterval {
					wm.flush(protocol)
				}
				continue
			}
			if hepPkt.ProtoType == 0 {
				atomic.AddUint64(&h.stats.DupCount, 1)
				metrics.RecordHEPPacketFailed(protocol, "duplicate")
				if cap(msg.data) >= maxPktLen {
					h.buffer.Put(msg.data[:maxPktLen])
				}
				if wm.count >= serverMetricsFlushInterval {
					wm.flush(protocol)
				}
				continue
			}

			atomic.AddUint64(&h.stats.HEPCount, 1)
			wm.processed++

			// Live stream tap. We publish after the dup/error checks
			// above so subscribers see the same stream the storage
			// writer sees, and before the DuckLake write so a slow
			// storage flush can never back-pressure the WebSocket
			// clients. Publish is a no-op when broker is nil.
			if h.broker != nil {
				h.broker.Publish(hepstream.FromHEP(hepPkt))
			}

			if h.ducklakeManager != nil {
				if err := h.ducklakeManager.WriteHEP(hepPkt); err != nil {
					logger.Debug(fmt.Sprintf("Failed to write HEP to DuckLake storage: %v", err))
				}
			}

			if cap(msg.data) >= maxPktLen {
				h.buffer.Put(msg.data[:maxPktLen])
			}

			if wm.count >= serverMetricsFlushInterval {
				wm.flush(protocol)
			}
		}
	}
}

func (h *HEPInput) logStats() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			logger.Info("stats since last 5 minutes",
				"PPS", atomic.LoadUint64(&h.stats.PktCount)/300,
				"HEP", atomic.LoadUint64(&h.stats.HEPCount),
				"Filtered", atomic.LoadUint64(&h.stats.DupCount),
				"Error", atomic.LoadUint64(&h.stats.ErrCount),
			)
			atomic.StoreUint64(&h.stats.PktCount, 0)
			atomic.StoreUint64(&h.stats.HEPCount, 0)
			atomic.StoreUint64(&h.stats.DupCount, 0)
			atomic.StoreUint64(&h.stats.ErrCount, 0)

		case <-h.quit:
			h.quit <- true
			return
		}
	}
}

func (h *HEPInput) reloadWorker() {
	s := make(chan os.Signal, 1)
	defer close(s)
	signal.Notify(s, syscall.SIGHUP)

	for {
		select {
		case <-s:
			logger.Info("reload all worker")
			h.wg.Add(1)

			h.exitWorker <- true
			<-h.exitWorker

			numWorkers := resolveWorkerCount()
			for n := 0; n < numWorkers; n++ {
				h.wg.Add(1)
				go h.worker()
			}
			h.wg.Done()
		case <-h.quit:
			h.quit <- true
			return
		}
	}
}

// resolveWorkerCount determines the number of worker goroutines.
// Priority: config value > auto-detect (NumCPU/2, minimum 2, maximum 4).
func resolveWorkerCount() int {
	if homerconfig.MainConfig != nil {
		if wc := homerconfig.MainConfig.Setting.SERVER_SETTINGS.WorkerCount; wc > 0 {
			return wc
		}
	}
	n := runtime.NumCPU() / 2
	if n > 4 {
		n = 4
	}
	if n < 2 {
		n = 2
	}
	return n
}
