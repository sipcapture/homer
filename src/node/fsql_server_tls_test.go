// Copyright (C) 2026 Homer Server Contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package node

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"database/sql"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/apache/arrow-go/v18/arrow/flight/flightsql"
	"github.com/sipcapture/homer-core/src/config"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"

	_ "github.com/duckdb/duckdb-go/v2"
)

func generateSelfSignedCert(t *testing.T, dir, name string) (certPath, keyPath string, cert *x509.Certificate) {
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
		IPAddresses:  []net.IP{net.IPv4(127, 0, 0, 1)},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert, err = x509.ParseCertificate(der)
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
	return certPath, keyPath, cert
}

func startTLSFsqlServer(t *testing.T, certPath, keyPath string) *fsqlServer {
	t.Helper()
	db, err := sql.Open("duckdb", "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	db.SetMaxOpenConns(1)

	n := &Node{
		db:     db,
		config: &config.NodeConfig{DuckLake: config.DuckLakeConfig{LakeName: "homer_lake"}},
	}
	fsql := newFsqlServer(n, config.FlightSQLServerConfig{
		Enable:    true,
		Host:      "127.0.0.1",
		Port:      0,
		TLSEnable: true,
		TLSCert:   certPath,
		TLSKey:    keyPath,
	}, "homer_lake")
	if err := fsql.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(fsql.Stop)
	return fsql
}

func executeWithRoots(t *testing.T, addr string, roots ...*x509.Certificate) error {
	t.Helper()
	pool := x509.NewCertPool()
	for _, c := range roots {
		pool.AddCert(c)
	}
	client, err := flightsql.NewClient(addr, nil, nil, grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{RootCAs: pool})))
	if err != nil {
		t.Fatalf("flightsql client: %v", err)
	}
	defer client.Close()
	_, err = client.Execute(context.Background(), "SELECT 1 AS x")
	return err
}

func TestFlightSQLServerTLSAcceptsTrustedClient(t *testing.T) {
	certPath, keyPath, cert := generateSelfSignedCert(t, t.TempDir(), "node")
	fsql := startTLSFsqlServer(t, certPath, keyPath)

	if err := executeWithRoots(t, fsql.listener.Addr().String(), cert); err != nil {
		t.Fatalf("execute over TLS: %v", err)
	}
}

func TestFlightSQLServerTLSRejectsUntrustedClient(t *testing.T) {
	certPath, keyPath, _ := generateSelfSignedCert(t, t.TempDir(), "node")
	fsql := startTLSFsqlServer(t, certPath, keyPath)
	addr := fsql.listener.Addr().String()

	t.Run("untrusted CA", func(t *testing.T) {
		err := executeWithRoots(t, addr)
		if err == nil || !strings.Contains(err.Error(), "x509: certificate signed by unknown authority") {
			t.Fatalf("expected an unknown-authority error, got: %v", err)
		}
	})

	t.Run("plaintext dial", func(t *testing.T) {
		client, err := flightsql.NewClient(addr, nil, nil, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			t.Fatalf("flightsql client: %v", err)
		}
		defer client.Close()
		_, err = client.Execute(context.Background(), "SELECT 1")
		if err == nil || !strings.Contains(err.Error(), "error reading server preface") {
			t.Fatalf("expected a plaintext-against-TLS-listener error, got: %v", err)
		}
	})
}

func TestFlightSQLServerTLSServesRenewedCert(t *testing.T) {
	dir := t.TempDir()
	certPath, keyPath, oldCert := generateSelfSignedCert(t, dir, "node")
	fsql := startTLSFsqlServer(t, certPath, keyPath)
	addr := fsql.listener.Addr().String()

	if err := executeWithRoots(t, addr, oldCert); err != nil {
		t.Fatalf("execute with original cert: %v", err)
	}

	newCertPath, newKeyPath, newCert := generateSelfSignedCert(t, dir, "renewed")
	if err := os.Rename(newCertPath, certPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(newKeyPath, keyPath); err != nil {
		t.Fatal(err)
	}

	if err := executeWithRoots(t, addr, newCert); err != nil {
		t.Fatalf("execute trusting only the renewed cert: %v", err)
	}
}
