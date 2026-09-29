// Copyright (C) 2026 Homer Server Contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package coordinator

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
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

	"github.com/apache/arrow-go/v18/arrow/flight"
	"github.com/apache/arrow-go/v18/arrow/flight/flightsql"
	"github.com/sipcapture/homer-core/src/config"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/status"
)

type minimalFlightSQLServer struct {
	flightsql.BaseServer
}

func writeSelfSigned(t *testing.T, dir, name string, dnsNames []string, ips []net.IP) (certPath, keyPath string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject:      pkix.Name{CommonName: name},
		NotBefore:    time.Now().Add(-time.Minute),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     dnsNames,
		IPAddresses:  ips,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	certPath = filepath.Join(dir, name+".crt")
	keyPath = filepath.Join(dir, name+".key")
	if err := os.WriteFile(certPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}), 0o600); err != nil {
		t.Fatal(err)
	}
	return certPath, keyPath
}

func startTLSFlightServer(t *testing.T, certPath, keyPath string) int {
	t.Helper()
	creds, err := credentials.NewServerTLSFromFile(certPath, keyPath)
	if err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	grpcServer := grpc.NewServer(grpc.Creds(creds))
	flight.RegisterFlightServiceServer(grpcServer, flightsql.NewFlightServer(&minimalFlightSQLServer{}))
	serveErrCh := make(chan error, 1)
	go func() { serveErrCh <- grpcServer.Serve(listener) }()
	t.Cleanup(func() {
		grpcServer.Stop()
		if err := <-serveErrCh; err != nil && !errors.Is(err, grpc.ErrServerStopped) {
			t.Errorf("flight server Serve: %v", err)
		}
	})

	_, portStr, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatal(err)
	}
	return port
}

func executeViaDialNode(t *testing.T, node config.NodeEndpoint) error {
	t.Helper()
	p, err := newFlightSQLProxy(config.FlightSQLServerConfig{}, []config.NodeEndpoint{node}, "")
	if err != nil {
		t.Fatal(err)
	}
	client, err := p.dialNode(context.Background(), node)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	_, err = client.Execute(context.Background(), "SELECT 1")
	return err
}

func requireHandshakeOK(t *testing.T, err error) {
	t.Helper()
	if status.Code(err) != codes.Unimplemented {
		t.Fatalf("expected BaseServer's Unimplemented after a completed handshake, got: %v", err)
	}
}

func requireErrContains(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("expected error containing %q, got: %v", want, err)
	}
}

func TestDialNodeTLS(t *testing.T) {
	dir := t.TempDir()
	ipCert, ipKey := writeSelfSigned(t, dir, "ip-san", nil, []net.IP{net.IPv4(127, 0, 0, 1)})
	dnsCert, dnsKey := writeSelfSigned(t, dir, "dns-only", []string{"flightsql.test"}, nil)
	otherCA, _ := writeSelfSigned(t, dir, "other-ca", []string{"other.test"}, nil)

	ipNode := config.NodeEndpoint{
		Name:          "ip-san",
		Host:          "127.0.0.1",
		FlightSQLPort: startTLSFlightServer(t, ipCert, ipKey),
		FlightSQLTLS:  true,
	}
	dnsNode := config.NodeEndpoint{
		Name:            "dns-only",
		Host:            "127.0.0.1",
		FlightSQLPort:   startTLSFlightServer(t, dnsCert, dnsKey),
		FlightSQLTLS:    true,
		FlightSQLCACert: dnsCert,
	}

	t.Run("trusted CA", func(t *testing.T) {
		node := ipNode
		node.FlightSQLCACert = ipCert
		requireHandshakeOK(t, executeViaDialNode(t, node))
	})

	t.Run("untrusted CA", func(t *testing.T) {
		node := ipNode
		node.FlightSQLCACert = otherCA
		requireErrContains(t, executeViaDialNode(t, node), "x509: certificate signed by unknown authority")
	})

	t.Run("IP host against DNS-only cert", func(t *testing.T) {
		requireErrContains(t, executeViaDialNode(t, dnsNode), "x509: cannot validate certificate for 127.0.0.1")
	})

	t.Run("server name override", func(t *testing.T) {
		node := dnsNode
		node.FlightSQLServerName = "flightsql.test"
		requireHandshakeOK(t, executeViaDialNode(t, node))
	})

	t.Run("use_tls does not enable FlightSQL TLS", func(t *testing.T) {
		node := ipNode
		node.FlightSQLTLS = false
		node.UseTLS = true
		if err := executeViaDialNode(t, node); err == nil || strings.Contains(err.Error(), "x509") {
			t.Fatalf("expected a plaintext dial failure against a TLS listener, got: %v", err)
		}
	})
}

func TestNewFlightSQLProxyRejectsBadCABundle(t *testing.T) {
	node := config.NodeEndpoint{
		Name:            "bad-ca",
		Host:            "127.0.0.1",
		FlightSQLPort:   50055,
		FlightSQLTLS:    true,
		FlightSQLCACert: filepath.Join(t.TempDir(), "missing.pem"),
	}
	if _, err := newFlightSQLProxy(config.FlightSQLServerConfig{}, []config.NodeEndpoint{node}, ""); err == nil {
		t.Fatal("expected newFlightSQLProxy to fail on an unreadable CA bundle")
	}
}
