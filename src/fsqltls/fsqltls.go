// Copyright (C) 2026 Homer Server Contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

// Package fsqltls builds TLS transport credentials for the Arrow FlightSQL
// listeners and the coordinator's dial to node FlightSQL ports.
package fsqltls

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"sync"

	logger "github.com/sipcapture/homer-core/src/utils/logging"
	"google.golang.org/grpc/credentials"
)

// ServerCredentials serves the certificate at certPath/keyPath and reloads it
// on the next handshake after either file is replaced or modified.
func ServerCredentials(certPath, keyPath string) (credentials.TransportCredentials, error) {
	r := &certReloader{certPath: certPath, keyPath: keyPath}
	if err := r.reload(); err != nil {
		return nil, err
	}
	return credentials.NewTLS(&tls.Config{GetCertificate: r.getCertificate}), nil
}

// ClientCredentials verifies the server against the PEM bundle at caPath
// (system roots when empty) and serverName (the dialed host when empty).
// The returned credentials share a session cache, so dialNode's per-query
// client still pays one full handshake per node rather than resuming zero.
func ClientCredentials(caPath, serverName string) (credentials.TransportCredentials, error) {
	cfg := &tls.Config{ServerName: serverName, ClientSessionCache: tls.NewLRUClientSessionCache(0)}
	if caPath != "" {
		pemBytes, err := os.ReadFile(caPath)
		if err != nil {
			return nil, fmt.Errorf("read CA bundle %s: %w", caPath, err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(pemBytes) {
			return nil, fmt.Errorf("no certificates found in %s", caPath)
		}
		cfg.RootCAs = pool
	}
	return credentials.NewTLS(cfg), nil
}

type certReloader struct {
	certPath, keyPath string

	mu                sync.Mutex
	cert              *tls.Certificate
	certInfo, keyInfo os.FileInfo
}

func (r *certReloader) getCertificate(*tls.ClientHelloInfo) (*tls.Certificate, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.changed() {
		if err := r.reload(); err != nil {
			logger.Warn(fmt.Sprintf("FlightSQL TLS: reload of %s failed, serving previous certificate: %v", r.certPath, err))
		}
	}
	return r.cert, nil
}

func (r *certReloader) changed() bool {
	return fileChanged(r.certPath, r.certInfo) || fileChanged(r.keyPath, r.keyInfo)
}

func fileChanged(path string, prev os.FileInfo) bool {
	cur, err := os.Stat(path)
	if err != nil {
		// Can't tell whether it changed. Report changed so changed() forces
		// a reload() attempt, which stats the same path again and, on the
		// same error, logs it -- rather than silently going stale.
		return true
	}
	return prev == nil || !os.SameFile(prev, cur) || !cur.ModTime().Equal(prev.ModTime())
}

func (r *certReloader) reload() error {
	certInfo, err := os.Stat(r.certPath)
	if err != nil {
		return fmt.Errorf("FlightSQL TLS cert: %w", err)
	}
	keyInfo, err := os.Stat(r.keyPath)
	if err != nil {
		return fmt.Errorf("FlightSQL TLS key: %w", err)
	}
	cert, err := tls.LoadX509KeyPair(r.certPath, r.keyPath)
	if err != nil {
		return fmt.Errorf("FlightSQL TLS key pair: %w", err)
	}
	// Only record the new file identity once the pair actually loads, so a
	// failed attempt never poisons the baseline changed() compares against
	// -- otherwise a transient failure at the moment the files already
	// match their final content permanently hides that they ever changed.
	r.certInfo, r.keyInfo, r.cert = certInfo, keyInfo, &cert
	return nil
}
