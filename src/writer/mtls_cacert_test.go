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
	"errors"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/sipcapture/homer-core/src/config"
)

// GHSA-2v9j-m2v3-x9qr: mutual_tls without cacert must not listen.
func TestTLSMutualTLSWithoutCACertDoesNotListen(t *testing.T) {
	certFile, keyFile := writeTestCert(t)
	srv := NewTLSServer(&Writer{}, &config.TLSServerConfig{
		Host:          "127.0.0.1",
		Port:          freePort(t),
		Cert:          certFile,
		Key:           keyFile,
		MutualTLS:     true,
		MinTLSVersion: "TLS1.2",
		MaxTLSVersion: "TLS1.3",
	})
	err := srv.Start()
	if err == nil {
		srv.Stop()
		t.Fatal("expected error for mutual TLS without cacert")
	}
	if !strings.Contains(err.Error(), "cacert") {
		t.Fatalf("error = %v", err)
	}
	if srv.listener != nil {
		t.Fatal("listener was opened")
	}
}

func TestHTTPSMutualTLSWithoutCACertDoesNotListen(t *testing.T) {
	certFile, keyFile := writeTestCert(t)
	srv := NewHTTPSServer(&Writer{}, &config.HTTPSServerConfig{
		Host:          "127.0.0.1",
		Port:          freePort(t),
		Cert:          certFile,
		Key:           keyFile,
		MutualTLS:     true,
		MinTLSVersion: "TLS1.2",
		MaxTLSVersion: "TLS1.3",
	})
	err := srv.Start()
	if err == nil {
		srv.Stop()
		t.Fatal("expected error for mutual TLS without cacert")
	}
	if !strings.Contains(err.Error(), "cacert") {
		t.Fatalf("error = %v", err)
	}
}

func TestTLSMutualTLSRequiresClientCert(t *testing.T) {
	certs := writeMTLSMaterial(t)
	port := freePort(t)
	srv := NewTLSServer(&Writer{}, &config.TLSServerConfig{
		Host:          "127.0.0.1",
		Port:          port,
		Cert:          certs.serverCert,
		Key:           certs.serverKey,
		CaCert:        certs.caCert,
		MutualTLS:     true,
		MinTLSVersion: "TLS1.2",
		MaxTLSVersion: "TLS1.3",
	})
	errCh := make(chan error, 1)
	go func() { errCh <- srv.Start() }()
	defer srv.Stop()

	addr := net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
	client := mustLoadKeyPair(t, certs.clientCert, certs.clientKey)
	if err := dialUntilAccepted(t, errCh, addr, &tls.Config{
		InsecureSkipVerify: true,
		Certificates:       []tls.Certificate{client},
	}); err != nil {
		t.Fatalf("client with certificate: %v", err)
	}
	if err := dialExpectRejected(t, addr, &tls.Config{InsecureSkipVerify: true}); err != nil {
		t.Fatal(err)
	}
}

func TestHTTPSMutualTLSRequiresClientCert(t *testing.T) {
	certs := writeMTLSMaterial(t)
	port := freePort(t)
	srv := NewHTTPSServer(&Writer{}, &config.HTTPSServerConfig{
		Host:          "127.0.0.1",
		Port:          port,
		Cert:          certs.serverCert,
		Key:           certs.serverKey,
		CaCert:        certs.caCert,
		MutualTLS:     true,
		MinTLSVersion: "TLS1.2",
		MaxTLSVersion: "TLS1.3",
	})
	errCh := make(chan error, 1)
	go func() { errCh <- srv.Start() }()
	defer srv.Stop()

	addr := net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
	client := mustLoadKeyPair(t, certs.clientCert, certs.clientKey)
	if err := dialUntilAccepted(t, errCh, addr, &tls.Config{
		InsecureSkipVerify: true,
		Certificates:       []tls.Certificate{client},
	}); err != nil {
		t.Fatalf("client with certificate: %v", err)
	}
	if err := dialExpectRejected(t, addr, &tls.Config{InsecureSkipVerify: true}); err != nil {
		t.Fatal(err)
	}
}

// dialUntilAccepted waits until addr accepts a handshake that stays open.
// TLS 1.3 servers read the client certificate after the client has already
// finished its half of the handshake, so Dial alone cannot tell acceptance
// from a later rejection.
func dialUntilAccepted(t *testing.T, errCh <-chan error, addr string, cfg *tls.Config) error {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	var last error
	for time.Now().Before(deadline) {
		select {
		case err := <-errCh:
			t.Fatalf("server exited: %v", err)
		default:
		}
		conn, err := tls.Dial("tcp", addr, cfg)
		if err != nil {
			last = err
			time.Sleep(20 * time.Millisecond)
			continue
		}
		if connectionKept(conn) {
			conn.Close()
			return nil
		}
		conn.Close()
		last = errRejected
		time.Sleep(20 * time.Millisecond)
	}
	return last
}

var errRejected = errors.New("server closed the connection")

func dialExpectRejected(t *testing.T, addr string, cfg *tls.Config) error {
	t.Helper()
	conn, err := tls.Dial("tcp", addr, cfg)
	if err != nil {
		return nil
	}
	defer conn.Close()
	if connectionKept(conn) {
		return errors.New("server kept a connection with no client certificate")
	}
	return nil
}

// connectionKept reports whether the server left the connection open long
// enough to read. A mutual-TLS rejection surfaces as a fast close.
func connectionKept(conn net.Conn) bool {
	_ = conn.SetDeadline(time.Now().Add(300 * time.Millisecond))
	_, err := conn.Read(make([]byte, 1))
	var ne net.Error
	return err != nil && errors.As(err, &ne) && ne.Timeout()
}

func mustLoadKeyPair(t *testing.T, certFile, keyFile string) tls.Certificate {
	t.Helper()
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		t.Fatal(err)
	}
	return cert
}

type mtlsMaterial struct {
	caCert, serverCert, serverKey, clientCert, clientKey string
}

func writeMTLSMaterial(t *testing.T) mtlsMaterial {
	t.Helper()
	caKey := mustECDSA(t)
	caTmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "test-ca"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
	}
	caDER := mustCert(t, caTmpl, caTmpl, &caKey.PublicKey, caKey)

	serverKey := mustECDSA(t)
	serverTmpl := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: "127.0.0.1"},
		IPAddresses:  []net.IP{net.IPv4(127, 0, 0, 1)},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	serverDER := mustCert(t, serverTmpl, caTmpl, &serverKey.PublicKey, caKey)

	clientKey := mustECDSA(t)
	clientTmpl := &x509.Certificate{
		SerialNumber: big.NewInt(3),
		Subject:      pkix.Name{CommonName: "test-client"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}
	clientDER := mustCert(t, clientTmpl, caTmpl, &clientKey.PublicKey, caKey)

	dir := t.TempDir()
	m := mtlsMaterial{
		caCert:     filepath.Join(dir, "ca.crt"),
		serverCert: filepath.Join(dir, "server.crt"),
		serverKey:  filepath.Join(dir, "server.key"),
		clientCert: filepath.Join(dir, "client.crt"),
		clientKey:  filepath.Join(dir, "client.key"),
	}
	writeCertPEM(t, m.caCert, caDER)
	writeCertPEM(t, m.serverCert, serverDER)
	writeKeyPEM(t, m.serverKey, serverKey)
	writeCertPEM(t, m.clientCert, clientDER)
	writeKeyPEM(t, m.clientKey, clientKey)
	return m
}

func mustECDSA(t *testing.T) *ecdsa.PrivateKey {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return key
}

func mustCert(t *testing.T, tmpl, parent *x509.Certificate, pub any, signer any) []byte {
	t.Helper()
	der, err := x509.CreateCertificate(rand.Reader, tmpl, parent, pub, signer)
	if err != nil {
		t.Fatal(err)
	}
	return der
}

func writeCertPEM(t *testing.T, path string, der []byte) {
	t.Helper()
	if err := os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600); err != nil {
		t.Fatal(err)
	}
}

func writeKeyPEM(t *testing.T, path string, key *ecdsa.PrivateKey) {
	t.Helper()
	der, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: der}), 0o600); err != nil {
		t.Fatal(err)
	}
}
