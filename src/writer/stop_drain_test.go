// Copyright (C) 2026 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package writer

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sipcapture/homer-core/src/config"
	"github.com/sipcapture/homer-core/src/decoder"
)

// Packets still queued when Stop runs are processed, not discarded. The
// packets are not HEP, so each processed one counts as a decode error and the
// test needs no storage.
func TestStopProcessesQueuedPackets(t *testing.T) {
	const queued = 500
	w := &Writer{
		inputCh:             make(chan incomingPacket, queued),
		exitWorker:          make(chan struct{}),
		wg:                  &sync.WaitGroup{},
		buffer:              &sync.Pool{New: func() interface{} { return make([]byte, maxPktLen) }},
		decoder:             decoder.NewDecoder(&decoder.DecoderConfig{}),
		metricsFlushPackets: 128,
		workersStarted:      1,
	}
	for i := 0; i < queued; i++ {
		w.inputCh <- incomingPacket{data: []byte("not a HEP packet"), protocol: "udp"}
	}

	// The worker starts only after Stop has begun, so the whole queue is
	// still there when Stop runs.
	w.wg.Add(1)
	stopped := make(chan struct{})
	go func() {
		_ = w.Stop()
		close(stopped)
	}()
	time.Sleep(100 * time.Millisecond)
	go w.worker()

	select {
	case <-stopped:
	case <-time.After(10 * time.Second):
		t.Fatal("Stop did not return")
	}
	if got := atomic.LoadUint64(&w.stats.ErrCount); got != queued {
		t.Fatalf("%d packets were queued at Stop, %d were processed", queued, got)
	}
}

// freePort returns a TCP port on 127.0.0.1 that was free a moment ago.
func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

// dialIdle connects to a stream receiver that is starting up and waits until
// the receiver has accepted the connection. The client then sends nothing.
func dialIdle(t *testing.T, w *Writer, dial func() (net.Conn, error)) net.Conn {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		conn, err := dial()
		if err == nil {
			for atomic.LoadInt64(&w.connCount) != 1 {
				if time.Now().After(deadline) {
					t.Fatal("the receiver did not accept the connection")
				}
				time.Sleep(10 * time.Millisecond)
			}
			t.Cleanup(func() { _ = conn.Close() })
			return conn
		}
		if time.Now().After(deadline) {
			t.Fatal(err)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// assertStopReturns fails when stop does not return well before the
// receiver's 60 s read timeout.
func assertStopReturns(t *testing.T, stop func()) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		stop()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Stop is blocked by an idle connection")
	}
}

func TestTCPStopClosesIdleConnection(t *testing.T) {
	w := &Writer{}
	port := freePort(t)
	srv := NewTCPServer(w, &config.TCPServerConfig{Host: "127.0.0.1", Port: port, ReadTimeoutSec: 60})
	go func() { _ = srv.Start() }()
	dialIdle(t, w, func() (net.Conn, error) {
		return net.Dial("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	})
	assertStopReturns(t, srv.Stop)
}

func TestTLSStopClosesIdleConnection(t *testing.T) {
	certFile, keyFile := writeTestCert(t)
	w := &Writer{}
	port := freePort(t)
	srv := NewTLSServer(w, &config.TLSServerConfig{
		Host: "127.0.0.1", Port: port, Cert: certFile, Key: keyFile,
		MinTLSVersion: "TLS1.2", MaxTLSVersion: "TLS1.3", ReadTimeoutSec: 60,
	})
	go func() { _ = srv.Start() }()
	dialIdle(t, w, func() (net.Conn, error) {
		conn, err := tls.Dial("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), &tls.Config{InsecureSkipVerify: true})
		if err != nil {
			return nil, err
		}
		return conn, conn.Handshake()
	})
	assertStopReturns(t, srv.Stop)
}

// writeTestCert writes a self-signed certificate for 127.0.0.1 and its key.
func writeTestCert(t *testing.T) (certFile, keyFile string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "127.0.0.1"},
		IPAddresses:  []net.IP{net.IPv4(127, 0, 0, 1)},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	certFile, keyFile = filepath.Join(dir, "server.crt"), filepath.Join(dir, "server.key")
	if err := os.WriteFile(certFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyFile, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}), 0o600); err != nil {
		t.Fatal(err)
	}
	return certFile, keyFile
}
