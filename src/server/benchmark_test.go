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

//go:build !vet
// +build !vet

package input

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/binary"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

const (
	benchmarkDuration   = 5 * time.Second
	benchmarkPacketSize = 100 // bytes per HEP packet payload
)

// generateBenchCert generates a test certificate and key for TLS benchmarking
func generateBenchCert() (certPath, keyPath string, err error) {
	tmpDir, err := os.MkdirTemp("", "bench_tls_*")
	if err != nil {
		return "", "", err
	}

	certPath = filepath.Join(tmpDir, "test.crt")
	keyPath = filepath.Join(tmpDir, "test.key")

	privKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return "", "", err
	}

	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization: []string{"Test"},
			Country:      []string{"US"},
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IPAddresses:           []net.IP{net.IPv4(127, 0, 0, 1), net.IPv6loopback},
		DNSNames:              []string{"localhost"},
	}

	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &privKey.PublicKey, privKey)
	if err != nil {
		return "", "", err
	}

	certFile, err := os.Create(certPath)
	if err != nil {
		return "", "", err
	}
	defer certFile.Close()
	pem.Encode(certFile, &pem.Block{Type: "CERTIFICATE", Bytes: certDER})

	keyFile, err := os.Create(keyPath)
	if err != nil {
		return "", "", err
	}
	defer keyFile.Close()
	privKeyDER := x509.MarshalPKCS1PrivateKey(privKey)
	pem.Encode(keyFile, &pem.Block{Type: "RSA PRIVATE KEY", Bytes: privKeyDER})

	return certPath, keyPath, nil
}

// BenchmarkTCPThroughput benchmarks TCP server throughput
func BenchmarkTCPThroughput(b *testing.B) {
	hepInput := NewHEPInput()
	defer hepInput.End()

	// Start TCP server
	serverPort := 19060
	go func() {
		hepInput.serveTCP("127.0.0.1", serverPort, true)
	}()

	// Wait for server to start
	time.Sleep(200 * time.Millisecond)

	// Create test packet
	payload := make([]byte, benchmarkPacketSize)
	for i := range payload {
		payload[i] = byte(i % 256)
	}
	// Create HEP packet manually to avoid duplicate function
	packet := make([]byte, 6+len(payload))
	copy(packet[0:4], []byte("HEP3"))
	binary.BigEndian.PutUint16(packet[4:6], uint16(6+len(payload)))
	copy(packet[6:], payload)

	// Connect to server
	conn, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", serverPort))
	if err != nil {
		b.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()

	var packetsSent int64
	var packetsProcessed uint64

	// Start packet counter
	go func() {
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		for range ticker.C {
			if atomic.LoadUint32(&hepInput.stopped) == 1 {
				break
			}
			processed := atomic.LoadUint64(&hepInput.stats.PktCount)
			atomic.StoreUint64(&packetsProcessed, processed)
		}
	}()

	// Send packets
	b.ResetTimer()
	startTime := time.Now()
	endTime := startTime.Add(benchmarkDuration)

	for time.Now().Before(endTime) {
		_, err := conn.Write(packet)
		if err != nil {
			b.Fatalf("Failed to write: %v", err)
		}
		atomic.AddInt64(&packetsSent, 1)
	}

	// Wait a bit for processing
	time.Sleep(100 * time.Millisecond)
	processed := atomic.LoadUint64(&hepInput.stats.PktCount)

	b.StopTimer()
	hepInput.End()

	duration := time.Since(startTime)
	packetsPerSec := float64(processed) / duration.Seconds()

	b.ReportMetric(packetsPerSec, "packets/sec")
	b.ReportMetric(float64(processed), "packets")
	b.Logf("TCP: Sent %d packets, Processed %d packets in %v (%.0f packets/sec)",
		atomic.LoadInt64(&packetsSent), atomic.LoadUint64(&packetsProcessed), duration, packetsPerSec)
}

// BenchmarkUDPThroughput benchmarks UDP server throughput
func BenchmarkUDPThroughput(b *testing.B) {
	hepInput := NewHEPInput()
	defer hepInput.End()

	// Start UDP server
	serverAddr := "127.0.0.1:19061"
	go func() {
		hepInput.serveUDP(serverAddr)
	}()

	// Wait for server to start
	time.Sleep(200 * time.Millisecond)

	// Create test packet
	payload := make([]byte, benchmarkPacketSize)
	for i := range payload {
		payload[i] = byte(i % 256)
	}
	// Create HEP packet manually to avoid duplicate function
	packet := make([]byte, 6+len(payload))
	copy(packet[0:4], []byte("HEP3"))
	binary.BigEndian.PutUint16(packet[4:6], uint16(6+len(payload)))
	copy(packet[6:], payload)

	// Connect to server
	addr, err := net.ResolveUDPAddr("udp", serverAddr)
	if err != nil {
		b.Fatalf("Failed to resolve address: %v", err)
	}
	conn, err := net.DialUDP("udp", nil, addr)
	if err != nil {
		b.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()

	var packetsSent int64
	var packetsProcessed uint64

	// Start packet counter
	go func() {
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		for range ticker.C {
			if atomic.LoadUint32(&hepInput.stopped) == 1 {
				break
			}
			processed := atomic.LoadUint64(&hepInput.stats.PktCount)
			atomic.StoreUint64(&packetsProcessed, processed)
		}
	}()

	// Send packets
	b.ResetTimer()
	startTime := time.Now()
	endTime := startTime.Add(benchmarkDuration)

	for time.Now().Before(endTime) {
		_, err := conn.Write(packet)
		if err != nil {
			b.Fatalf("Failed to write: %v", err)
		}
		atomic.AddInt64(&packetsSent, 1)
	}

	// Wait a bit for processing
	time.Sleep(100 * time.Millisecond)
	processed := atomic.LoadUint64(&hepInput.stats.PktCount)

	b.StopTimer()
	hepInput.End()

	duration := time.Since(startTime)
	packetsPerSec := float64(processed) / duration.Seconds()

	b.ReportMetric(packetsPerSec, "packets/sec")
	b.ReportMetric(float64(processed), "packets")
	b.Logf("UDP: Sent %d packets, Processed %d packets in %v (%.0f packets/sec)",
		atomic.LoadInt64(&packetsSent), atomic.LoadUint64(&packetsProcessed), duration, packetsPerSec)
}

// BenchmarkTLSThroughput benchmarks TLS server throughput
func BenchmarkTLSThroughput(b *testing.B) {
	// Generate test certificates
	certPath, keyPath, err := generateBenchCert()
	if err != nil {
		b.Fatalf("Failed to generate certificates: %v", err)
	}
	defer os.RemoveAll(filepath.Dir(certPath))

	hepInput := NewHEPInput()
	defer hepInput.End()

	// Start TLS server
	serverPort := 19062
	tlsSettings := &TLSSettings{
		Enable:        true,
		Port:          serverPort,
		Cert:          certPath,
		Key:           keyPath,
		MinTLSVersion: "TLS1.2",
		MaxTLSVersion: "TLS1.3",
	}

	go func() {
		hepInput.serveTLS("127.0.0.1", serverPort, tlsSettings)
	}()

	// Wait for server to start
	time.Sleep(500 * time.Millisecond)

	// Create test packet
	payload := make([]byte, benchmarkPacketSize)
	for i := range payload {
		payload[i] = byte(i % 256)
	}
	// Create HEP packet manually to avoid duplicate function
	packet := make([]byte, 6+len(payload))
	copy(packet[0:4], []byte("HEP3"))
	binary.BigEndian.PutUint16(packet[4:6], uint16(6+len(payload)))
	copy(packet[6:], payload)

	// Connect to server with TLS
	conn, err := tls.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", serverPort), &tls.Config{
		InsecureSkipVerify: true,
	})
	if err != nil {
		b.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()

	var packetsSent int64
	var packetsProcessed uint64

	// Start packet counter
	go func() {
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		for range ticker.C {
			if atomic.LoadUint32(&hepInput.stopped) == 1 {
				break
			}
			processed := atomic.LoadUint64(&hepInput.stats.PktCount)
			atomic.StoreUint64(&packetsProcessed, processed)
		}
	}()

	// Send packets
	b.ResetTimer()
	startTime := time.Now()
	endTime := startTime.Add(benchmarkDuration)

	for time.Now().Before(endTime) {
		_, err := conn.Write(packet)
		if err != nil {
			b.Fatalf("Failed to write: %v", err)
		}
		atomic.AddInt64(&packetsSent, 1)
	}

	// Wait a bit for processing
	time.Sleep(100 * time.Millisecond)
	processed := atomic.LoadUint64(&hepInput.stats.PktCount)

	b.StopTimer()
	hepInput.End()

	duration := time.Since(startTime)
	packetsPerSec := float64(processed) / duration.Seconds()

	b.ReportMetric(packetsPerSec, "packets/sec")
	b.ReportMetric(float64(processed), "packets")
	b.Logf("TLS: Sent %d packets, Processed %d packets in %v (%.0f packets/sec)",
		atomic.LoadInt64(&packetsSent), atomic.LoadUint64(&packetsProcessed), duration, packetsPerSec)
}

// BenchmarkComparison runs all benchmarks and compares results
func BenchmarkComparison(b *testing.B) {
	b.Run("TCP", BenchmarkTCPThroughput)
	b.Run("UDP", BenchmarkUDPThroughput)
	b.Run("TLS", BenchmarkTLSThroughput)
}

// BenchmarkTCPConcurrentConnections benchmarks TCP with multiple connections
func BenchmarkTCPConcurrentConnections(b *testing.B) {
	hepInput := NewHEPInput()
	defer hepInput.End()

	serverPort := 19063
	go func() {
		hepInput.serveTCP("127.0.0.1", serverPort, true)
	}()

	time.Sleep(200 * time.Millisecond)

	payload := make([]byte, benchmarkPacketSize)
	for i := range payload {
		payload[i] = byte(i % 256)
	}
	// Create HEP packet manually to avoid duplicate function
	packet := make([]byte, 6+len(payload))
	copy(packet[0:4], []byte("HEP3"))
	binary.BigEndian.PutUint16(packet[4:6], uint16(6+len(payload)))
	copy(packet[6:], payload)

	numConnections := 10
	var wg sync.WaitGroup
	var totalSent int64

	b.ResetTimer()
	startTime := time.Now()
	endTime := startTime.Add(benchmarkDuration)

	for i := 0; i < numConnections; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			conn, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", serverPort))
			if err != nil {
				b.Logf("Failed to connect: %v", err)
				return
			}
			defer conn.Close()

			for time.Now().Before(endTime) {
				conn.Write(packet)
				atomic.AddInt64(&totalSent, 1)
			}
		}()
	}

	wg.Wait()
	time.Sleep(100 * time.Millisecond)
	processed := atomic.LoadUint64(&hepInput.stats.PktCount)

	b.StopTimer()
	hepInput.End()

	duration := time.Since(startTime)
	packetsPerSec := float64(processed) / duration.Seconds()

	b.ReportMetric(packetsPerSec, "packets/sec")
	b.ReportMetric(float64(processed), "packets")
	b.Logf("TCP Concurrent (%d connections): Sent %d packets, Processed %d packets in %v (%.0f packets/sec)",
		numConnections, atomic.LoadInt64(&totalSent), processed, duration, packetsPerSec)
}
