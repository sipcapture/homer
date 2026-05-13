// Copyright (C) 2026 Homer Server Contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package lineprotoreceiver

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sipcapture/homer-core/src/config"
)

// newTestServer wires the real HTTP mux on top of a fresh in-memory
// DuckDB so tests exercise the full handler → ingester → DuckDB path.
func newTestServer(t *testing.T) (*httptest.Server, string) {
	t.Helper()
	db, lake := newLPDB(t)
	cfg := &config.LineProtoConfig{
		Enable:           true,
		Listen:           ":0",
		MaxBodyBytes:     8 * 1024 * 1024,
		DefaultPrecision: "ns",
		TablePrefix:      "lp_",
		ReadTimeoutSec:   10,
		WriteTimeoutSec:  10,
	}
	ing := NewIngester(db, lake, cfg)
	hs, err := newHTTPServer(cfg, ing)
	if err != nil {
		t.Fatalf("newHTTPServer: %v", err)
	}
	srv := httptest.NewServer(hs.server.Handler)
	t.Cleanup(srv.Close)
	return srv, lake
}

func TestHTTP_WriteV1_NoContentOnSuccess(t *testing.T) {
	srv, _ := newTestServer(t)

	resp, err := http.Post(srv.URL+"/write?db=metrics&precision=ns",
		"text/plain; charset=utf-8",
		strings.NewReader("cpu,host=a value=0.5"))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status: got %d (body=%s), want 204", resp.StatusCode, string(body))
	}
}

func TestHTTP_WriteV2_BucketRoutesToSchema(t *testing.T) {
	srv, _ := newTestServer(t)

	resp, err := http.Post(srv.URL+"/api/v2/write?bucket=apps&org=main&precision=ns",
		"text/plain",
		strings.NewReader("requests,svc=api count=1i"))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status: got %d (body=%s), want 204", resp.StatusCode, string(body))
	}
}

func TestHTTP_WriteV3_GigapiAlias(t *testing.T) {
	srv, _ := newTestServer(t)

	resp, err := http.Post(srv.URL+"/api/v3/write_lp?db=apps",
		"text/plain",
		strings.NewReader("hits,page=home v=1i"))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status: got %d (body=%s), want 204", resp.StatusCode, string(body))
	}
}

func TestHTTP_GzipBody(t *testing.T) {
	srv, _ := newTestServer(t)

	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	if _, err := io.WriteString(gz, "cpu,host=z value=0.1"); err != nil {
		t.Fatalf("gz write: %v", err)
	}
	_ = gz.Close()

	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/write?db=metrics", &buf)
	req.Header.Set("Content-Encoding", "gzip")
	req.Header.Set("Content-Type", "text/plain")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status: got %d (body=%s), want 204", resp.StatusCode, string(body))
	}
}

func TestHTTP_BadParseReturns400(t *testing.T) {
	srv, _ := newTestServer(t)

	resp, err := http.Post(srv.URL+"/write",
		"text/plain",
		strings.NewReader("broken_no_field"))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status: got %d (body=%s), want 400", resp.StatusCode, string(body))
	}
	if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "application/json") {
		t.Errorf("error content-type: got %q, want application/json", ct)
	}
}

func TestHTTP_EmptyBodyIsAccepted(t *testing.T) {
	srv, _ := newTestServer(t)

	resp, err := http.Post(srv.URL+"/write?db=metrics", "text/plain", strings.NewReader(""))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status: got %d, want 204", resp.StatusCode)
	}
}

func TestHTTP_MethodNotAllowed(t *testing.T) {
	srv, _ := newTestServer(t)

	resp, err := http.Get(srv.URL + "/write")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("status: got %d, want 405", resp.StatusCode)
	}
}

func TestHTTP_PingAndHealth(t *testing.T) {
	srv, _ := newTestServer(t)

	pingResp, err := http.Get(srv.URL + "/ping")
	if err != nil {
		t.Fatalf("ping: %v", err)
	}
	defer pingResp.Body.Close()
	if pingResp.StatusCode != http.StatusNoContent {
		t.Errorf("ping status: got %d, want 204", pingResp.StatusCode)
	}
	if pingResp.Header.Get("X-Influxdb-Version") == "" {
		t.Errorf("ping should set X-Influxdb-Version")
	}

	healthResp, err := http.Get(srv.URL + "/health")
	if err != nil {
		t.Fatalf("health: %v", err)
	}
	defer healthResp.Body.Close()
	if healthResp.StatusCode != http.StatusOK {
		t.Errorf("health status: got %d, want 200", healthResp.StatusCode)
	}
	body, _ := io.ReadAll(healthResp.Body)
	if !strings.Contains(string(body), `"status":"pass"`) {
		t.Errorf("health body: got %s, want to contain status:pass", string(body))
	}
}

func TestHTTP_BodyTooLarge(t *testing.T) {
	db, lake := newLPDB(t)
	cfg := &config.LineProtoConfig{
		Enable:          true,
		Listen:          ":0",
		MaxBodyBytes:    32, // tiny so any reasonable LP exceeds it
		ReadTimeoutSec:  10,
		WriteTimeoutSec: 10,
	}
	ing := NewIngester(db, lake, cfg)
	hs, err := newHTTPServer(cfg, ing)
	if err != nil {
		t.Fatalf("newHTTPServer: %v", err)
	}
	srv := httptest.NewServer(hs.server.Handler)
	defer srv.Close()

	body := strings.Repeat("cpu,host=a value=0.5\n", 16)
	resp, err := http.Post(srv.URL+"/write", "text/plain", strings.NewReader(body))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("status: got %d, want 413", resp.StatusCode)
	}
}

func TestHTTP_WriteHepCall(t *testing.T) {
	db, lake := newLPDB(t)
	createTestHepCallTable(t, db, lake)
	cfg := &config.LineProtoConfig{
		Enable:           true,
		Listen:           ":0",
		MaxBodyBytes:     8 * 1024 * 1024,
		DefaultPrecision: "ns",
		TablePrefix:      "lp_",
		ReadTimeoutSec:   10,
		WriteTimeoutSec:  10,
		AllowHepSipCall:  true,
	}
	ing := NewIngester(db, lake, cfg)
	hs, err := newHTTPServer(cfg, ing)
	if err != nil {
		t.Fatalf("newHTTPServer: %v", err)
	}
	srv := httptest.NewServer(hs.server.Handler)
	defer srv.Close()

	lp := `sip,method=BYE,session_id=s1 caller="alice",callee="bob",src_ip="10.0.0.1",dst_ip="10.0.0.2",src_port=5060i,dst_port=5060i,protocol=17i,payload="BYE sip:x SIP/2.0" 1700000000000000000`
	resp, err := http.Post(srv.URL+"/write?hep_table=call&precision=ns", "text/plain", strings.NewReader(lp))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status: got %d body=%s", resp.StatusCode, string(body))
	}
	var n int
	if err := db.QueryRow("SELECT count(*) FROM test_lake.main.hep_proto_1_call").Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 1 {
		t.Fatalf("rows in hep_proto_1_call: want 1, got %d", n)
	}
}

func TestHTTP_WriteHepTableDisabled(t *testing.T) {
	db, lake := newLPDB(t)
	cfg := &config.LineProtoConfig{
		Enable:           true,
		Listen:           ":0",
		MaxBodyBytes:     8 * 1024 * 1024,
		DefaultPrecision: "ns",
		TablePrefix:      "lp_",
		ReadTimeoutSec:   10,
		WriteTimeoutSec:  10,
		AllowHepSipCall:  false,
	}
	ing := NewIngester(db, lake, cfg)
	hs, err := newHTTPServer(cfg, ing)
	if err != nil {
		t.Fatalf("newHTTPServer: %v", err)
	}
	srv := httptest.NewServer(hs.server.Handler)
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/write?hep_table=call", "text/plain", strings.NewReader("cpu v=1i"))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status: got %d, want 400", resp.StatusCode)
	}
}
