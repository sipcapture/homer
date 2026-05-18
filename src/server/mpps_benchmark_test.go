// Copyright (C) 2026 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// SPDX-License-Identifier: AGPL-3.0-or-later

//go:build !vet
// +build !vet

// Package input — Mpps benchmarks for HEP UDP/TCP ingest.
//
// These benchmarks measure end-to-end ingest throughput in millions of
// packets per second (Mpps) for the HEP UDP and TCP transports. They
// exercise the full hot path: kernel socket → gnet event loop →
// inputCh queue → worker goroutine → DecodeHEP → SIP zero-copy parse.
//
// The existing BenchmarkUDPThroughput / BenchmarkTCPThroughput are kept
// for backwards-compat smoke testing but they only measure the rate at
// which OnTraffic enqueues packets into inputCh: they do not start any
// workers, so the channel back-pressures after queue_size packets and
// the reported number is bound by the channel buffer rather than real
// end-to-end ingest.
//
// Run:
//
//	# optional: raise net.core rmem/wmem (root) before UDP benches
//	sudo ./scripts/tune-udp-sysctl.sh
//	# or: HOMER_BENCH_TUNE_SYSCTL=1 go test ... (attempts sysctl from the test)
//
//	go test -vet=off -tags '!vet' ./server -run='^$' \
//	    -bench=BenchmarkUDPMpps -benchtime=1x -timeout=2m
//	go test -vet=off -tags '!vet' ./server -run='^$' \
//	    -bench=BenchmarkTCPMpps -benchtime=1x -timeout=2m
//
// CPU profile:
//
//	go test -vet=off -tags '!vet' ./server -run='^$' \
//	    -bench=BenchmarkUDPMpps -benchtime=1x -timeout=3m \
//	    -cpuprofile=cpu_udp.prof -memprofile=mem_udp.prof
//	go tool pprof -top -cum cpu_udp.prof | head -40
//
// Both benchmarks: 1s warmup, 5s measurement window, then drain.
// Reported metrics use the measurement window only (steady-state).

package input

import (
	"context"
	"encoding/binary"
	"fmt"
	"net"
	"os"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/panjf2000/gnet/v2"
	"github.com/sipcapture/homer-core/src/decoder"
	"github.com/sipcapture/homer-core/src/homerconfig"
	"github.com/sipcapture/homer-core/src/utils/sysctl"
)

// HEP3 chunk type IDs (mirror of decoder/decoder.go constants — kept
// local to avoid coupling the benchmark to internal layout).
const (
	bcChunkIPFamily = 0x0001
	bcChunkIPProto  = 0x0002
	bcChunkIP4Src   = 0x0003
	bcChunkIP4Dst   = 0x0004
	bcChunkSrcPort  = 0x0007
	bcChunkDstPort  = 0x0008
	bcChunkTsec     = 0x0009
	bcChunkTmsec    = 0x000a
	bcChunkProtoT   = 0x000b
	bcChunkNodeID   = 0x000c
	bcChunkNodePW   = 0x000e
	bcChunkPayload  = 0x000f
	bcChunkCID      = 0x0011
)

// mppsWindow is the steady-state measurement window: long enough to
// average out gnet event-loop scheduling noise and short enough to
// keep the suite under a few minutes wall-clock.
const (
	mppsWarmup = 1 * time.Second
	mppsWindow = 5 * time.Second
)

// sampleSIPInvite is a realistic SIP INVITE used as the HEP3 payload.
// It exercises the full SIP parse path (Via/From/To/Call-ID/CSeq/SDP)
// the way a production capture would.
const sampleSIPInvite = "INVITE sip:bob@biloxi.example.com SIP/2.0\r\n" +
	"Via: SIP/2.0/UDP pc33.atlanta.example.com;branch=z9hG4bK776asdhds\r\n" +
	"Max-Forwards: 70\r\n" +
	"To: Bob <sip:bob@biloxi.example.com>\r\n" +
	"From: Alice <sip:alice@atlanta.example.com>;tag=1928301774\r\n" +
	"Call-ID: bench-callid-12345@atlanta.example.com\r\n" +
	"CSeq: 1 INVITE\r\n" +
	"Contact: <sip:alice@pc33.atlanta.example.com>\r\n" +
	"User-Agent: bench-ua/1.0\r\n" +
	"Content-Type: application/sdp\r\n" +
	"Content-Length: 150\r\n" +
	"\r\n" +
	"v=0\r\n" +
	"o=alice 53655765 2353687637 IN IP4 pc33.atlanta.example.com\r\n" +
	"s=-\r\n" +
	"c=IN IP4 pc33.atlanta.example.com\r\n" +
	"t=0 0\r\n" +
	"m=audio 3456 RTP/AVP 0 1 3 99\r\n" +
	"a=rtpmap:0 PCMU/8000\r\n"

// buildHEP3SIPPacket constructs a HEP3-encapsulated SIP INVITE.
// Returns a fully-formed packet ready to send over UDP/TCP.
func buildHEP3SIPPacket() []byte {
	now := time.Now()
	chunks := make([]byte, 0, 256)
	chunks = appendU8Chunk(chunks, bcChunkIPFamily, 0x02)
	chunks = appendU8Chunk(chunks, bcChunkIPProto, 0x11)
	chunks = appendIP4Chunk(chunks, bcChunkIP4Src, net.IPv4(10, 0, 0, 1))
	chunks = appendIP4Chunk(chunks, bcChunkIP4Dst, net.IPv4(10, 0, 0, 2))
	chunks = appendU16Chunk(chunks, bcChunkSrcPort, 5060)
	chunks = appendU16Chunk(chunks, bcChunkDstPort, 5060)
	chunks = appendU32Chunk(chunks, bcChunkTsec, uint32(now.Unix()))
	chunks = appendU32Chunk(chunks, bcChunkTmsec, uint32(now.UnixMicro()%1_000_000))
	chunks = appendU8Chunk(chunks, bcChunkProtoT, 1) // SIP
	chunks = appendU32Chunk(chunks, bcChunkNodeID, 2001)
	chunks = appendStrChunk(chunks, bcChunkNodePW, "myHep")
	chunks = appendStrChunk(chunks, bcChunkPayload, sampleSIPInvite)
	chunks = appendStrChunk(chunks, bcChunkCID, "bench-callid-12345@atlanta.example.com")

	totalLen := 6 + len(chunks)
	packet := make([]byte, totalLen)
	copy(packet[0:4], []byte("HEP3"))
	binary.BigEndian.PutUint16(packet[4:6], uint16(totalLen))
	copy(packet[6:], chunks)
	return packet
}

func appendChunkHeader(buf []byte, chunkType uint16, body uint16) []byte {
	var hdr [6]byte
	// vendor 0x0000 (everything we use is the generic vendor)
	binary.BigEndian.PutUint16(hdr[0:2], 0)
	binary.BigEndian.PutUint16(hdr[2:4], chunkType)
	binary.BigEndian.PutUint16(hdr[4:6], 6+body)
	return append(buf, hdr[:]...)
}

func appendU8Chunk(buf []byte, chunkType uint16, val byte) []byte {
	buf = appendChunkHeader(buf, chunkType, 1)
	return append(buf, val)
}

func appendU16Chunk(buf []byte, chunkType uint16, val uint16) []byte {
	buf = appendChunkHeader(buf, chunkType, 2)
	var b [2]byte
	binary.BigEndian.PutUint16(b[:], val)
	return append(buf, b[:]...)
}

func appendU32Chunk(buf []byte, chunkType uint16, val uint32) []byte {
	buf = appendChunkHeader(buf, chunkType, 4)
	var b [4]byte
	binary.BigEndian.PutUint32(b[:], val)
	return append(buf, b[:]...)
}

func appendIP4Chunk(buf []byte, chunkType uint16, ip net.IP) []byte {
	v4 := ip.To4()
	buf = appendChunkHeader(buf, chunkType, 4)
	return append(buf, v4...)
}

func appendStrChunk(buf []byte, chunkType uint16, s string) []byte {
	buf = appendChunkHeader(buf, chunkType, uint16(len(s)))
	return append(buf, s...)
}

// ensureBenchMainConfig installs a minimal homerconfig.MainConfig used
// by the server hot path. Called once per benchmark; preserves the
// original to play nice if other tests in the package depend on it.
func ensureBenchMainConfig(workerCount, queueSize, recvBuf int) (restore func()) {
	prev := homerconfig.MainConfig

	cfg := &homerconfig.HomerServerConfig{
		Setting: &homerconfig.HomerServerSettings{},
	}
	cfg.Setting.SERVER_SETTINGS.WorkerCount = workerCount
	cfg.Setting.SERVER_SETTINGS.QueueSize = queueSize
	cfg.Setting.SERVER_SETTINGS.UDP_SERVER.SocketRecvBuffer = recvBuf
	cfg.Setting.SERVER_SETTINGS.UDP_SERVER.SocketSendBuffer = 4 * 1024 * 1024
	cfg.Setting.SERVER_SETTINGS.UDP_SERVER.ReadBufferCap = 256 * 1024
	cfg.Setting.HEP_SETTINGS.HepV3Enable = true
	cfg.Setting.HEP_SETTINGS.HepV2Enable = true
	cfg.Setting.HEP_SETTINGS.ProtobufEnable = false
	cfg.Setting.HEP_SETTINGS.Deduplicate = false

	homerconfig.MainConfig = cfg
	return func() { homerconfig.MainConfig = prev }
}

// startWorkers manually spins up n decoder workers. We deliberately
// do NOT use HEPInput.Run() because it also starts logStats /
// reloadWorker goroutines and global signal handlers that we don't
// want during a benchmark.
//
// Returned stop function drains workers and waits for them via the
// internal wg. Safe to call exactly once.
func startWorkers(h *HEPInput, n int) func() {
	atomic.StoreUint32(&h.workersStarted, 1)
	for i := 0; i < n; i++ {
		h.wg.Add(1)
		go h.worker()
	}
	return func() {
		// One worker exits via the exitWorker handshake; the rest
		// exit when inputCh closes. Same teardown protocol as End()
		// minus the logStats/reloadWorker quit signal we never set
		// up here.
		h.exitWorker <- true
		<-h.exitWorker
		close(h.inputCh)
		h.wg.Wait()
	}
}

// stopGnetServer stops a gnet listener by address with a short
// timeout. gnet.Run blocks until Stop is called; without this, the
// per-sub-bench engine and event loops leak between cases.
func stopGnetServer(addr string) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_ = gnet.Stop(ctx, addr)
}

var benchSysctlOnce sync.Once

func maybeTuneBenchSysctl(b *testing.B) {
	benchSysctlOnce.Do(func() {
		if os.Getenv("HOMER_BENCH_TUNE_SYSCTL") != "1" {
			return
		}
		if err := sysctl.ApplyRecommendedUDPBuffers(); err != nil {
			b.Logf("UDP sysctl tune skipped (run as root or sudo scripts/tune-udp-sysctl.sh): %v", err)
			return
		}
		b.Logf("UDP sysctl tuned (rmem_max=%d)", sysctl.RecommendedRmemMaxBytes)
	})
}

// runMppsUDP runs a single UDP Mpps benchmark and reports steady-state
// metrics. workers is the number of decoder goroutines, senders is the
// number of UDP client goroutines (each with its own conn — mimics N
// independent capture agents).
func runMppsUDP(b *testing.B, workers, senders, port int) {
	b.Helper()
	b.StopTimer()

	restore := ensureBenchMainConfig(workers, 200000, 32*1024*1024)
	defer restore()
	maybeTuneBenchSysctl(b)

	h := NewHEPInput()
	stopWorkers := startWorkers(h, workers)

	addr := fmt.Sprintf("127.0.0.1:%d", port)
	go h.serveUDP(addr)

	// Wait for gnet to actually bind the socket. Probe the port with
	// a few short retries — gnet's OnBoot does not surface a sync
	// signal we can wait on, but a successful Dial implies the
	// listener is up.
	if err := waitUDPReady(addr, 2*time.Second); err != nil {
		b.Fatalf("UDP server did not start: %v", err)
	}

	packet := buildHEP3SIPPacket()
	pktSize := len(packet)

	var sentTotal, sendErrTotal int64
	stop := make(chan struct{})
	var swg sync.WaitGroup

	for s := 0; s < senders; s++ {
		swg.Add(1)
		go func() {
			defer swg.Done()
			conn, err := net.Dial("udp", addr)
			if err != nil {
				return
			}
			defer conn.Close()
			if uc, ok := conn.(*net.UDPConn); ok {
				_ = uc.SetWriteBuffer(4 * 1024 * 1024)
			}
			var sent, errs int64
			for {
				select {
				case <-stop:
					atomic.AddInt64(&sentTotal, sent)
					atomic.AddInt64(&sendErrTotal, errs)
					return
				default:
				}
				if _, err := conn.Write(packet); err != nil {
					errs++
				} else {
					sent++
				}
			}
		}()
	}

	// Warmup, then sample HEPCount at the start and end of a fixed
	// window — gives a steady-state Mpps figure that excludes ramp-up
	// and channel-fill transients.
	time.Sleep(mppsWarmup)
	hepStart := atomic.LoadUint64(&h.stats.HEPCount)
	pktStart := atomic.LoadUint64(&h.stats.PktCount)
	errStart := atomic.LoadUint64(&h.stats.ErrCount)
	dupStart := atomic.LoadUint64(&h.stats.DupCount)

	b.StartTimer()
	t0 := time.Now()
	time.Sleep(mppsWindow)
	elapsed := time.Since(t0)
	b.StopTimer()

	hepEnd := atomic.LoadUint64(&h.stats.HEPCount)
	pktEnd := atomic.LoadUint64(&h.stats.PktCount)
	errEnd := atomic.LoadUint64(&h.stats.ErrCount)
	dupEnd := atomic.LoadUint64(&h.stats.DupCount)

	close(stop)
	swg.Wait()

	// Tear down: stop gnet UDP listener, then drain decoder workers.
	atomic.StoreUint32(&h.stopped, 1)
	stopGnetServer("udp://" + addr)
	stopWorkers()

	hepDelta := hepEnd - hepStart
	pktDelta := pktEnd - pktStart
	errDelta := errEnd - errStart
	dupDelta := dupEnd - dupStart
	sec := elapsed.Seconds()
	pps := float64(hepDelta) / sec
	mpps := pps / 1_000_000
	mbps := float64(hepDelta) * float64(pktSize) * 8 / sec / 1_000_000
	dropPct := 0.0
	if total := atomic.LoadInt64(&sentTotal); total > 0 {
		dropPct = (1 - float64(pktDelta)/float64(total)) * 100
	}

	b.ReportMetric(pps, "pkts/sec")
	b.ReportMetric(mpps, "Mpps")
	b.ReportMetric(mbps, "Mbps")
	b.ReportMetric(float64(errDelta), "decode_errors")
	b.ReportMetric(dropPct, "drop_%")
	b.Logf("UDP w=%d s=%d port=%d pkt=%dB sent=%d enq=%d hep=%d errs=%d dups=%d "+
		"window=%v -> %.0f pps (%.3f Mpps, %.1f Mbps, drop=%.2f%%)",
		workers, senders, port, pktSize,
		atomic.LoadInt64(&sentTotal), pktDelta, hepDelta, errDelta, dupDelta,
		elapsed.Round(time.Millisecond), pps, mpps, mbps, dropPct)
}

// runMppsTCP — same shape as runMppsUDP but with multiple long-lived
// TCP connections. Each sender goroutine streams as many HEP3 packets
// as the kernel send buffer accepts; gnet on the server side reads
// them in batches in OnTraffic.
func runMppsTCP(b *testing.B, workers, senders, port int) {
	b.Helper()
	b.StopTimer()

	restore := ensureBenchMainConfig(workers, 200000, 32*1024*1024)
	defer restore()

	h := NewHEPInput()
	stopWorkers := startWorkers(h, workers)

	addr := fmt.Sprintf("127.0.0.1:%d", port)
	go h.serveTCP("127.0.0.1", port, true)

	if err := waitTCPReady(addr, 2*time.Second); err != nil {
		b.Fatalf("TCP server did not start: %v", err)
	}

	packet := buildHEP3SIPPacket()
	pktSize := len(packet)

	var sentTotal, sendErrTotal int64
	stop := make(chan struct{})
	var swg sync.WaitGroup

	for s := 0; s < senders; s++ {
		swg.Add(1)
		go func() {
			defer swg.Done()
			conn, err := net.Dial("tcp", addr)
			if err != nil {
				return
			}
			defer conn.Close()
			if tc, ok := conn.(*net.TCPConn); ok {
				_ = tc.SetWriteBuffer(4 * 1024 * 1024)
				_ = tc.SetNoDelay(true)
			}
			var sent, errs int64
			// Stream packets back-to-back; gnet on the server side
			// will batch up to batchSize per OnTraffic call.
			for {
				select {
				case <-stop:
					atomic.AddInt64(&sentTotal, sent)
					atomic.AddInt64(&sendErrTotal, errs)
					return
				default:
				}
				if _, err := conn.Write(packet); err != nil {
					errs++
				} else {
					sent++
				}
			}
		}()
	}

	time.Sleep(mppsWarmup)
	hepStart := atomic.LoadUint64(&h.stats.HEPCount)
	pktStart := atomic.LoadUint64(&h.stats.PktCount)
	errStart := atomic.LoadUint64(&h.stats.ErrCount)

	b.StartTimer()
	t0 := time.Now()
	time.Sleep(mppsWindow)
	elapsed := time.Since(t0)
	b.StopTimer()

	hepEnd := atomic.LoadUint64(&h.stats.HEPCount)
	pktEnd := atomic.LoadUint64(&h.stats.PktCount)
	errEnd := atomic.LoadUint64(&h.stats.ErrCount)

	close(stop)
	swg.Wait()

	atomic.StoreUint32(&h.stopped, 1)
	stopGnetServer(fmt.Sprintf("tcp://127.0.0.1:%d", port))
	stopWorkers()

	hepDelta := hepEnd - hepStart
	pktDelta := pktEnd - pktStart
	errDelta := errEnd - errStart
	sec := elapsed.Seconds()
	pps := float64(hepDelta) / sec
	mpps := pps / 1_000_000
	mbps := float64(hepDelta) * float64(pktSize) * 8 / sec / 1_000_000

	b.ReportMetric(pps, "pkts/sec")
	b.ReportMetric(mpps, "Mpps")
	b.ReportMetric(mbps, "Mbps")
	b.ReportMetric(float64(errDelta), "decode_errors")
	b.Logf("TCP w=%d s=%d port=%d pkt=%dB sent=%d enq=%d hep=%d errs=%d "+
		"window=%v -> %.0f pps (%.3f Mpps, %.1f Mbps)",
		workers, senders, port, pktSize,
		atomic.LoadInt64(&sentTotal), pktDelta, hepDelta, errDelta,
		elapsed.Round(time.Millisecond), pps, mpps, mbps)
}

func waitUDPReady(addr string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		c, err := net.Dial("udp", addr)
		if err == nil {
			_ = c.Close()
			return nil
		}
		time.Sleep(20 * time.Millisecond)
	}
	return fmt.Errorf("timeout dialing %s", addr)
}

func waitTCPReady(addr string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		c, err := net.DialTimeout("tcp", addr, 100*time.Millisecond)
		if err == nil {
			_ = c.Close()
			return nil
		}
		time.Sleep(20 * time.Millisecond)
	}
	return fmt.Errorf("timeout dialing %s", addr)
}

// BenchmarkUDPMpps reports steady-state Mpps for UDP HEP ingest as a
// function of decoder workers × concurrent UDP senders. The packet
// is a real HEP3-encapsulated SIP INVITE.
//
// Sub-bench naming: w<workers>_s<senders>.
func BenchmarkUDPMpps(b *testing.B) {
	cases := []struct {
		workers int
		senders int
	}{
		{2, 1},
		{4, 1},
		{4, 4},
		{8, 8},
		{16, 16},
	}
	port := 19070
	for _, tc := range cases {
		tc := tc
		name := fmt.Sprintf("w%d_s%d", tc.workers, tc.senders)
		b.Run(name, func(b *testing.B) {
			runMppsUDP(b, tc.workers, tc.senders, port)
			port++
		})
	}
}

// BenchmarkTCPMpps mirrors BenchmarkUDPMpps for the TCP transport.
// Each sender opens one long-lived TCP connection.
func BenchmarkTCPMpps(b *testing.B) {
	cases := []struct {
		workers int
		senders int
	}{
		{2, 1},
		{4, 1},
		{4, 4},
		{8, 8},
		{16, 16},
	}
	port := 19090
	for _, tc := range cases {
		tc := tc
		name := fmt.Sprintf("w%d_s%d", tc.workers, tc.senders)
		b.Run(name, func(b *testing.B) {
			runMppsTCP(b, tc.workers, tc.senders, port)
			port++
		})
	}
}

// BenchmarkHEPDecodeOnly isolates the cost of decoder.DecodeHEP +
// SIP zero-copy parse, without any networking, channels, or workers.
// Use it to compute a theoretical per-core ceiling and compare with
// the end-to-end Mpps numbers.
func BenchmarkHEPDecodeOnly(b *testing.B) {
	restore := ensureBenchMainConfig(0, 200000, 0)
	defer restore()

	packet := buildHEP3SIPPacket()
	b.SetBytes(int64(len(packet)))
	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			h, err := decoder.DecodeHEP(packet)
			if err != nil {
				b.Fatalf("DecodeHEP: %v", err)
			}
			if h == nil || h.ProtoType != 1 {
				decoder.ReleaseHEP(h)
				b.Fatalf("unexpected decode result: %+v", h)
			}
			decoder.ReleaseHEP(h)
		}
	})
}

// BenchmarkHEPDecodeSerial decodes on a single goroutine — used to
// derive ns/op and a per-core ceiling without contention.
func BenchmarkHEPDecodeSerial(b *testing.B) {
	restore := ensureBenchMainConfig(0, 200000, 0)
	defer restore()

	packet := buildHEP3SIPPacket()
	b.SetBytes(int64(len(packet)))
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		h, err := decoder.DecodeHEP(packet)
		if err != nil {
			b.Fatalf("DecodeHEP: %v", err)
		}
		if h == nil {
			b.Fatal("nil HEP")
		}
		decoder.ReleaseHEP(h)
	}
}

// init pins GOMAXPROCS to NumCPU so benchmarks see all cores even if
// the parent test runner already lowered it.
func init() {
	if runtime.GOMAXPROCS(0) < runtime.NumCPU() {
		runtime.GOMAXPROCS(runtime.NumCPU())
	}
}
