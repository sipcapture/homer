package ducklake

import (
	"io"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// Ambient AWS_SESSION_TOKEN must not be attached to static-key DuckLake
// writes. Ceph/RGW answers that header with HTTP 400 (sipcapture/homer#1086,
// duckdb/duckdb-httpfs#254). The writer pool replays S3ClientSettingsSQL on
// every connection, so the clear has to be in that list, not a one-shot Exec.
func TestStaticS3WriteDropsAmbientSessionToken(t *testing.T) {
	t.Setenv("AWS_SESSION_TOKEN", "garbage-token")
	t.Setenv("AWS_ACCESS_KEY_ID", "envkey")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "envsecret")
	t.Setenv("AWS_DEFAULT_REGION", "eu-west-1")

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	var sawToken bool
	var requests int
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		mu.Lock()
		requests++
		if r.Header.Get("X-Amz-Security-Token") != "" {
			sawToken = true
		}
		mu.Unlock()
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `<?xml version="1.0" encoding="UTF-8"?><Error><Code>InvalidArgument</Code><Message>no</Message></Error>`)
	})}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })

	endpoint := ln.Addr().String()
	db, err := newLakeDuckDB(Config{
		S3Region:          "us-east-1",
		S3AccessKeyID:     "k",
		S3SecretAccessKey: "s",
		S3Endpoint:        "http://" + endpoint,
		S3UseSSL:          false,
		S3URLStyle:        "path",
	}, 2)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if err := EnsureWriterS3Secret(db, "us-east-1", "k", "s", "http://"+endpoint, false, "path"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("LOAD ducklake; LOAD sqlite;"); err != nil {
		t.Fatal(err)
	}
	cat := filepath.Join(t.TempDir(), "cat.sqlite")
	attach := "ATTACH 'ducklake:sqlite:" + cat + "' AS lake (DATA_PATH 's3://mybucket/hep/');"
	if _, err := db.Exec(attach); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE lake.main.t (id INTEGER, date DATE);`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CALL lake.set_option('DATA_INLINING_ROW_LIMIT', 0);`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`ALTER TABLE lake.main.t SET PARTITIONED BY (date);`); err != nil {
		t.Fatal(err)
	}
	// A second connection (the pool is 2) is what flush uses. It must not
	// inherit the process environment's session token.
	_, err = db.Exec(`INSERT INTO lake.main.t VALUES (1, DATE '2026-10-25');`)
	if err == nil || !strings.Contains(err.Error(), "HTTP") {
		t.Fatalf("expected the stub to reject the PUT, got %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if requests == 0 {
		t.Fatal("DuckLake did not attempt an S3 PUT")
	}
	if sawToken {
		t.Fatal("static-key S3 PUT included ambient AWS_SESSION_TOKEN")
	}
}
