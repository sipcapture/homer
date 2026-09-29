// Copyright (C) 2026 Homer Server Contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package fsqltls

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeKeyPair(t *testing.T, dir, name string, serial int64) (certPath, keyPath string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(serial),
		Subject:      pkix.Name{CommonName: "flightsql.test"},
		NotBefore:    time.Now().Add(-time.Minute),
		NotAfter:     time.Now().Add(time.Hour),
		DNSNames:     []string{"flightsql.test"},
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
	writePEM(t, certPath, "CERTIFICATE", der)
	writePEM(t, keyPath, "PRIVATE KEY", keyDER)
	return certPath, keyPath
}

func writePEM(t *testing.T, path, blockType string, der []byte) {
	t.Helper()
	if err := os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: blockType, Bytes: der}), 0o600); err != nil {
		t.Fatal(err)
	}
}

func servedSerial(t *testing.T, r *certReloader) int64 {
	t.Helper()
	cert, err := r.getCertificate(&tls.ClientHelloInfo{})
	if err != nil {
		t.Fatal(err)
	}
	leaf, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		t.Fatal(err)
	}
	return leaf.SerialNumber.Int64()
}

func rename(t *testing.T, from, to string) {
	t.Helper()
	if err := os.Rename(from, to); err != nil {
		t.Fatal(err)
	}
}

func TestServerCredentialsRejectsMissingFiles(t *testing.T) {
	dir := t.TempDir()
	if _, err := ServerCredentials(filepath.Join(dir, "missing.crt"), filepath.Join(dir, "missing.key")); err == nil {
		t.Fatal("expected an error for a missing key pair")
	}
}

func TestCertReloaderPicksUpReplacedPair(t *testing.T) {
	dir := t.TempDir()
	certPath, keyPath := writeKeyPair(t, dir, "live", 1)
	r := &certReloader{certPath: certPath, keyPath: keyPath}
	if err := r.reload(); err != nil {
		t.Fatal(err)
	}
	if got := servedSerial(t, r); got != 1 {
		t.Fatalf("serial: got %d want 1", got)
	}

	newCert, newKey := writeKeyPair(t, dir, "renewed", 2)
	rename(t, newCert, certPath)
	rename(t, newKey, keyPath)

	if got := servedSerial(t, r); got != 2 {
		t.Fatalf("serial after renewal: got %d want 2", got)
	}
}

func TestCertReloaderKeepsServingOnBrokenPair(t *testing.T) {
	dir := t.TempDir()
	certPath, keyPath := writeKeyPair(t, dir, "live", 1)
	r := &certReloader{certPath: certPath, keyPath: keyPath}
	if err := r.reload(); err != nil {
		t.Fatal(err)
	}

	_, otherKey := writeKeyPair(t, dir, "other", 2)
	rename(t, otherKey, keyPath)

	if got := servedSerial(t, r); got != 1 {
		t.Fatalf("serial after mismatched key: got %d want 1", got)
	}
}

func TestFileChangedReportsChangedOnStatError(t *testing.T) {
	dir := t.TempDir()
	certPath, _ := writeKeyPair(t, dir, "live", 1)
	info, err := os.Stat(certPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(certPath); err != nil {
		t.Fatal(err)
	}
	if !fileChanged(certPath, info) {
		t.Fatal("expected fileChanged to report changed when Stat fails, so the caller retries instead of going stale silently")
	}
}

func TestCertReloaderKeepsServingWhenFileGoesMissing(t *testing.T) {
	dir := t.TempDir()
	certPath, keyPath := writeKeyPair(t, dir, "live", 1)
	r := &certReloader{certPath: certPath, keyPath: keyPath}
	if err := r.reload(); err != nil {
		t.Fatal(err)
	}

	if err := os.Remove(keyPath); err != nil {
		t.Fatal(err)
	}
	if got := servedSerial(t, r); got != 1 {
		t.Fatalf("serial while key is missing: got %d want 1", got)
	}

	newCert, newKey := writeKeyPair(t, dir, "renewed", 2)
	rename(t, newCert, certPath)
	rename(t, newKey, keyPath)

	if got := servedSerial(t, r); got != 2 {
		t.Fatalf("serial after key restored and renewed: got %d want 2", got)
	}
}

func TestClientCredentialsRejectsBadCABundle(t *testing.T) {
	dir := t.TempDir()
	if _, err := ClientCredentials(filepath.Join(dir, "missing.pem"), ""); err == nil {
		t.Fatal("expected an error for a missing CA bundle")
	}
	empty := filepath.Join(dir, "empty.pem")
	if err := os.WriteFile(empty, []byte("not a certificate\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ClientCredentials(empty, ""); err == nil {
		t.Fatal("expected an error for a bundle with no certificates")
	}
}
