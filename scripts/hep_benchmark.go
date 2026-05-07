// Copyright (C) 2025 Homer Server Contributors
//
// HEP3 UDP Benchmark Tool
// Sends realistic SIP INVITE HEP3 packets over UDP to homer-core
// and measures throughput (packets/sec).
//
// Usage:
//   go run scripts/hep_benchmark.go -addr 127.0.0.1:9060 -duration 10s -workers 4
//
//go:build ignore

package main

import (
	"encoding/binary"
	"flag"
	"fmt"
	"math/rand"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

// HEP3 chunk types
const (
	chunkIPFamily = 0x0001
	chunkIPProto  = 0x0002
	chunkIP4Src   = 0x0003
	chunkIP4Dst   = 0x0004
	chunkSrcPort  = 0x0007
	chunkDstPort  = 0x0008
	chunkTsec     = 0x0009
	chunkTmsec    = 0x000a
	chunkProtoT   = 0x000b
	chunkNodeID   = 0x000c
	chunkNodePW   = 0x000e
	chunkPayload  = 0x000f
	chunkCID      = 0x0011
)

func makeChunk(vendorID, chunkType uint16, body []byte) []byte {
	chunkLen := uint16(6 + len(body))
	buf := make([]byte, chunkLen)
	binary.BigEndian.PutUint16(buf[0:2], vendorID)
	binary.BigEndian.PutUint16(buf[2:4], chunkType)
	binary.BigEndian.PutUint16(buf[4:6], chunkLen)
	copy(buf[6:], body)
	return buf
}

func makeU8Chunk(chunkType uint16, val byte) []byte {
	return makeChunk(0x0000, chunkType, []byte{val})
}

func makeU16Chunk(chunkType uint16, val uint16) []byte {
	b := make([]byte, 2)
	binary.BigEndian.PutUint16(b, val)
	return makeChunk(0x0000, chunkType, b)
}

func makeU32Chunk(chunkType uint16, val uint32) []byte {
	b := make([]byte, 4)
	binary.BigEndian.PutUint32(b, val)
	return makeChunk(0x0000, chunkType, b)
}

func makeStrChunk(chunkType uint16, s string) []byte {
	return makeChunk(0x0000, chunkType, []byte(s))
}

func makeIP4Chunk(chunkType uint16, ip net.IP) []byte {
	return makeChunk(0x0000, chunkType, ip.To4())
}

// buildSIPInvite generates a realistic SIP INVITE message with proper CRLF line endings
func buildSIPInvite(callID string, seq int) string {
	return fmt.Sprintf(
		"INVITE sip:bob@biloxi.example.com SIP/2.0\r\n"+
			"Via: SIP/2.0/UDP pc33.atlanta.example.com;branch=z9hG4bK776asdhds\r\n"+
			"Max-Forwards: 70\r\n"+
			"To: Bob <sip:bob@biloxi.example.com>\r\n"+
			"From: Alice <sip:alice@atlanta.example.com>;tag=1928301774\r\n"+
			"Call-ID: %s\r\n"+
			"CSeq: %d INVITE\r\n"+
			"Contact: <sip:alice@pc33.atlanta.example.com>\r\n"+
			"Content-Type: application/sdp\r\n"+
			"Content-Length: 150\r\n"+
			"\r\n"+
			"v=0\r\n"+
			"o=alice 53655765 2353687637 IN IP4 pc33.atlanta.example.com\r\n"+
			"s=-\r\n"+
			"c=IN IP4 pc33.atlanta.example.com\r\n"+
			"t=0 0\r\n"+
			"m=audio 3456 RTP/AVP 0 1 3 99\r\n"+
			"a=rtpmap:0 PCMU/8000\r\n",
		callID, seq)
}

func buildMinimalInvite(callID string) string {
	return fmt.Sprintf(
		"INVITE sip:bench@localhost SIP/2.0\r\n"+
			"Max-Forwards: 70\r\n"+
			"Call-ID: %s\r\n"+
			"CSeq: 1 INVITE\r\n"+
			"Content-Length: 0\r\n"+
			"\r\n",
		callID)
}

// buildHEP3Packet builds a complete HEP3 binary packet
func buildHEP3Packet(callID string, seq int, protoType uint16, compactPayload bool) []byte {
	now := time.Now()

	var sipPayload string
	if compactPayload {
		sipPayload = buildMinimalInvite(callID)
	} else {
		sipPayload = buildSIPInvite(callID, seq)
	}

	// Build chunks
	var chunks []byte
	chunks = append(chunks, makeU8Chunk(chunkIPFamily, 0x02)...) // IPv4
	chunks = append(chunks, makeU8Chunk(chunkIPProto, 0x11)...)  // UDP
	chunks = append(chunks, makeIP4Chunk(chunkIP4Src, net.ParseIP("10.0.0.1"))...)
	chunks = append(chunks, makeIP4Chunk(chunkIP4Dst, net.ParseIP("10.0.0.2"))...)
	chunks = append(chunks, makeU16Chunk(chunkSrcPort, 5060)...)
	chunks = append(chunks, makeU16Chunk(chunkDstPort, 5060)...)
	chunks = append(chunks, makeU32Chunk(chunkTsec, uint32(now.Unix()))...)
	chunks = append(chunks, makeU32Chunk(chunkTmsec, uint32(now.UnixMicro()%1_000_000))...)
	chunks = append(chunks, makeU8Chunk(chunkProtoT, byte(protoType))...) // SIP
	chunks = append(chunks, makeU32Chunk(chunkNodeID, 2001)...)
	chunks = append(chunks, makeStrChunk(chunkNodePW, "myHep")...)
	chunks = append(chunks, makeStrChunk(chunkPayload, sipPayload)...)
	chunks = append(chunks, makeStrChunk(chunkCID, callID)...)

	// Build HEP3 header: "HEP3" + uint16(totalLength)
	totalLen := 4 + 2 + len(chunks)
	packet := make([]byte, totalLen)
	copy(packet[0:4], []byte("HEP3"))
	binary.BigEndian.PutUint16(packet[4:6], uint16(totalLen))
	copy(packet[6:], chunks)

	return packet
}

func main() {
	addr := flag.String("addr", "127.0.0.1:9060", "homer-core HEP UDP address")
	protoType := flag.Uint("proto-type", 1, "transport protocol type for chunkProtoT")
	compactPayload := flag.Bool("compact-payload", false, "send minimal SIP payload")
	duration := flag.Duration("duration", 10*time.Second, "benchmark duration")
	workers := flag.Int("workers", 1, "number of parallel sender goroutines")
	flag.Parse()

	fmt.Printf("HEP3 UDP Benchmark\n")
	fmt.Printf("  Target:   %s\n", *addr)
	fmt.Printf("  Duration: %s\n", *duration)
	fmt.Printf("  Workers:  %d\n\n", *workers)

	// Pre-build one packet to measure size
	samplePacket := buildHEP3Packet("bench-sample-callid@localhost", 1, uint16(*protoType), *compactPayload)
	fmt.Printf("  Packet size: %d bytes\n\n", len(samplePacket))

	var totalSent int64
	var totalErrors int64
	var wg sync.WaitGroup

	startBarrier := make(chan struct{})
	start := time.Now()

	for w := 0; w < *workers; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			// Each worker gets its own UDP connection
			udpAddr, err := net.ResolveUDPAddr("udp", *addr)
			if err != nil {
				fmt.Fprintf(os.Stderr, "worker %d: resolve error: %v\n", workerID, err)
				return
			}
			conn, err := net.DialUDP("udp", nil, udpAddr)
			if err != nil {
				fmt.Fprintf(os.Stderr, "worker %d: dial error: %v\n", workerID, err)
				return
			}
			defer conn.Close()

			// Increase write buffer
			conn.SetWriteBuffer(4 * 1024 * 1024)

			// Pre-build a batch of packets with unique call-IDs
			batchSize := 100
			packets := make([][]byte, batchSize)
			for i := 0; i < batchSize; i++ {
				callID := fmt.Sprintf("bench-%d-%d-%d@localhost", workerID, i, rand.Int63())
				packets[i] = buildHEP3Packet(callID, i+1, uint16(*protoType), *compactPayload)
			}

			<-startBarrier // wait for all workers to be ready

			deadline := time.Now().Add(*duration)
			var localSent, localErr int64
			idx := 0

			for time.Now().Before(deadline) {
				_, err := conn.Write(packets[idx%batchSize])
				if err != nil {
					localErr++
				} else {
					localSent++
				}
				idx++
			}

			atomic.AddInt64(&totalSent, localSent)
			atomic.AddInt64(&totalErrors, localErr)
		}(w)
	}

	// Start all workers simultaneously
	close(startBarrier)
	start = time.Now()

	wg.Wait()
	elapsed := time.Since(start)

	sent := atomic.LoadInt64(&totalSent)
	errors := atomic.LoadInt64(&totalErrors)
	pps := float64(sent) / elapsed.Seconds()
	mbps := float64(sent) * float64(len(samplePacket)) * 8 / elapsed.Seconds() / 1_000_000

	fmt.Printf("Results:\n")
	fmt.Printf("  Duration:    %v\n", elapsed.Round(time.Millisecond))
	fmt.Printf("  Packets:     %d sent, %d errors\n", sent, errors)
	fmt.Printf("  Throughput:  %.0f packets/sec\n", pps)
	fmt.Printf("  Bandwidth:   %.1f Mbps\n", mbps)
	fmt.Printf("  Per worker:  %.0f packets/sec\n", pps/float64(*workers))
}
