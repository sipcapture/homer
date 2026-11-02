// Copyright (C) 2026 Homer Server Contributors
// SPDX-License-Identifier: AGPL-3.0-or-later
//go:build !vet

package ducklake

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/google/uuid"
	"github.com/sipcapture/homer-core/src/decoder"
	"github.com/sipcapture/homer-core/src/utils/logging"
)

type horizontalWriterConfig struct {
	Lake                 Config
	Ready, Start, Worker string
	Rows                 int
}

func TestPostgresHorizontalWriterProcess(t *testing.T) {
	path := os.Getenv("HOMER_TEST_LAKE_CHILD_CONFIG")
	if path == "" {
		t.Skip("subprocess helper")
	}
	logging.InitLoggerSimple("info", true, false)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var cfg horizontalWriterConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatal(err)
	}
	mgr, err := NewManager(cfg.Lake)
	if err != nil {
		t.Fatal(err)
	}
	if err := mgr.Start(); err != nil {
		t.Fatal(err)
	}
	defer mgr.Stop()
	if err := os.WriteFile(cfg.Ready, []byte("ready"), 0600); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(45 * time.Second)
	for {
		if _, err := os.Stat(cfg.Start); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("writer start barrier timed out")
		}
		time.Sleep(20 * time.Millisecond)
	}
	hep, err := decoder.DecodeHEP(decoder.BenchHEP3SIPPacket())
	if err != nil {
		t.Fatal(err)
	}
	defer decoder.ReleaseHEP(hep)
	for i := 0; i < cfg.Rows; i++ {
		hep.CID = fmt.Sprintf("%s-%04d", cfg.Worker, i)
		if err := mgr.WriteHEP(hep); err != nil {
			t.Fatal(err)
		}
		time.Sleep(time.Millisecond)
	}
	if err := mgr.Stop(); err != nil {
		t.Fatal(err)
	}
}

func TestPostgresHorizontalWritersAndReaders(t *testing.T) {
	for _, tc := range []struct {
		name     string
		interval time.Duration
		queue    bool
	}{
		{"normal_flush", time.Second, false},
		{"rapid_flush_stress", 20 * time.Millisecond, false},
		{"central_queue_rapid_flush", 20 * time.Millisecond, true},
	} {
		t.Run(tc.name, func(t *testing.T) { runPostgresHorizontalWritersAndReaders(t, tc.interval, tc.queue) })
	}
}

func runPostgresHorizontalWritersAndReaders(t *testing.T, interval time.Duration, queue bool) {
	endpoint := os.Getenv("HOMER_TEST_S3_ENDPOINT")
	if endpoint == "" {
		t.Skip("set HOMER_TEST_S3_ENDPOINT for concurrent object-storage integration")
	}
	dsn, dir := postgresLakeFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	client := s3.NewFromConfig(aws.Config{Region: "us-east-1", Credentials: credentials.NewStaticCredentialsProvider(
		os.Getenv("HOMER_TEST_S3_ACCESS_KEY"), os.Getenv("HOMER_TEST_S3_SECRET_KEY"), "")}, func(o *s3.Options) { o.BaseEndpoint = aws.String(endpoint); o.UsePathStyle = true })
	bucket := "homer-scale-" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err := client.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: aws.String(bucket)}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupCtx, stop := context.WithTimeout(context.Background(), 30*time.Second)
		defer stop()
		pages := s3.NewListObjectsV2Paginator(client, &s3.ListObjectsV2Input{Bucket: aws.String(bucket)})
		for pages.HasMorePages() {
			page, err := pages.NextPage(cleanupCtx)
			if err != nil {
				break
			}
			for _, object := range page.Contents {
				_, _ = client.DeleteObject(cleanupCtx, &s3.DeleteObjectInput{Bucket: aws.String(bucket), Key: object.Key})
			}
		}
		_, _ = client.DeleteBucket(cleanupCtx, &s3.DeleteBucketInput{Bucket: aws.String(bucket)})
	})
	installer, err := sql.Open("duckdb", "")
	if err != nil {
		t.Fatal(err)
	}
	for _, ext := range []string{"httpfs", "aws"} {
		if _, err := installer.Exec("LOAD " + ext); err == nil {
			continue
		}
		if _, err := installer.Exec("INSTALL " + ext + " FROM 'https://extensions.duckdb.org'"); err != nil {
			_ = installer.Close()
			t.Fatal(err)
		}
	}
	_ = installer.Close()
	cfg := Config{CatalogPath: dsn, DataPath: "s3://" + bucket + "/parquet", LakeName: "lake", ShardCount: 1,
		BatchSize: 32, FlushInterval: interval, FlushQueue: &queue, DataInliningRowLimit: 0,
		S3Region: "us-east-1", S3AccessKeyID: os.Getenv("HOMER_TEST_S3_ACCESS_KEY"), S3SecretAccessKey: os.Getenv("HOMER_TEST_S3_SECRET_KEY"),
		S3Endpoint: strings.TrimPrefix(strings.TrimPrefix(endpoint, "http://"), "https://"), S3UseSSL: strings.HasPrefix(endpoint, "https://"), S3URLStyle: "path",
		TuningThreads: 1, TuningMemoryLimit: "256MB", TuningTempDirectory: filepath.Join(dir, "bootstrap-spill")}
	// Initialize once, then have independently started writers contend on the
	// same existing catalog, tables and remote Parquet directory.
	bootstrap, err := NewManager(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := bootstrap.Stop(); err != nil {
		t.Fatal(err)
	}
	hep, err := decoder.DecodeHEP(decoder.BenchHEP3SIPPacket())
	if err != nil {
		t.Fatal(err)
	}
	key, _, err := ConvertHEPToLakeRow(hep)
	decoder.ReleaseHEP(hep)
	if err != nil {
		t.Fatal(err)
	}
	table := "lake.main.hep_proto_" + key.String()
	readers := make([]*MaintenanceDB, 2)
	for i := range readers {
		readerCfg := cfg
		readerCfg.TuningTempDirectory = filepath.Join(dir, fmt.Sprintf("reader-%d", i))
		readers[i], err = OpenMaintenanceDB(readerCfg, "256MB", nil)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = readers[i].Close() })
	}
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	barrier := filepath.Join(dir, "start")
	type result struct {
		worker int
		err    error
	}
	done := make(chan result, 2)
	ready := make([]string, 2)
	logs := make([]string, 2)
	for i := 0; i < 2; i++ {
		workerCfg := cfg
		workerCfg.TuningTempDirectory = filepath.Join(dir, fmt.Sprintf("writer-%d", i))
		ready[i], logs[i] = filepath.Join(dir, fmt.Sprintf("ready-%d", i)), filepath.Join(dir, fmt.Sprintf("writer-%d.log", i))
		data, err := json.Marshal(horizontalWriterConfig{Lake: workerCfg, Ready: ready[i], Start: barrier, Worker: fmt.Sprintf("writer-%d", i), Rows: 2048})
		if err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(dir, fmt.Sprintf("writer-%d.json", i))
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
		log, err := os.OpenFile(logs[i], os.O_CREATE|os.O_WRONLY, 0600)
		if err != nil {
			t.Fatal(err)
		}
		cmd := exec.CommandContext(ctx, binary, "-test.run=^TestPostgresHorizontalWriterProcess$", "-test.v")
		cmd.Env = append(os.Environ(), "HOMER_TEST_LAKE_CHILD_CONFIG="+path)
		cmd.Stdout, cmd.Stderr = log, log
		if err := cmd.Start(); err != nil {
			_ = log.Close()
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = cmd.Process.Kill() })
		go func(i int) { err := cmd.Wait(); _ = log.Close(); done <- result{i, err} }(i)
	}
	deadline := time.Now().Add(45 * time.Second)
	for _, path := range ready {
		for {
			if _, err := os.Stat(path); err == nil {
				break
			}
			if time.Now().After(deadline) {
				t.Fatal("writer startup timed out")
			}
			select {
			case res := <-done:
				data, _ := os.ReadFile(logs[res.worker])
				t.Fatalf("writer exited before barrier: %v\n%s", res.err, data)
			default:
			}
			time.Sleep(20 * time.Millisecond)
		}
	}
	if err := os.WriteFile(barrier, []byte("start"), 0600); err != nil {
		t.Fatal(err)
	}
	remaining, merged := 2, false
	lastCounts := []int{0, 0}
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for remaining > 0 {
		select {
		case <-ctx.Done():
			t.Fatal("concurrent lake test timed out")
		case res := <-done:
			if res.err != nil {
				data, _ := os.ReadFile(logs[res.worker])
				t.Fatalf("writer %d: %v\n%s", res.worker, res.err, data)
			}
			remaining--
		case <-ticker.C:
			for i, reader := range readers {
				var n int
				if err := reader.DB.QueryRowContext(ctx, "SELECT count(*) FROM "+table).Scan(&n); err != nil {
					t.Fatal(err)
				}
				if n < lastCounts[i] {
					t.Fatalf("reader %d lost rows: %d -> %d", i, lastCounts[i], n)
				}
				lastCounts[i] = n
			}
			if !merged && lastCounts[0] > 0 {
				if _, err := execWithRetry(readers[0].DB, 5, 100*time.Millisecond, nil, "CALL ducklake_merge_adjacent_files('lake')"); err != nil {
					t.Fatal(err)
				}
				merged = true
			}
		}
	}
	for i, reader := range readers {
		var count, distinct int
		if err := reader.DB.QueryRow("SELECT count(*), count(DISTINCT cid) FROM "+table).Scan(&count, &distinct); err != nil {
			t.Fatal(err)
		}
		if count != 4096 || distinct != 4096 {
			freshCfg := cfg
			freshCfg.TuningTempDirectory = filepath.Join(dir, "fresh-reader")
			fresh, freshErr := OpenMaintenanceDB(freshCfg, "256MB", nil)
			if freshErr == nil {
				var freshCount int
				freshErr = fresh.DB.QueryRow("SELECT count(*) FROM " + table).Scan(&freshCount)
				t.Logf("fresh reader rows=%d error=%v", freshCount, freshErr)
				_ = fresh.Close()
			}
			for _, path := range logs {
				data, _ := os.ReadFile(path)
				t.Logf("writer log:\n%s", data)
			}
			t.Fatalf("reader %d: rows=%d unique IDs=%d, want 4096 of each", i, count, distinct)
		}
	}
	if !merged {
		t.Fatal("no compaction overlapped writer activity")
	}
	objects, err := client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{Bucket: aws.String(bucket)})
	if err != nil || len(objects.Contents) == 0 {
		t.Fatalf("no remote Parquet objects: %v", err)
	}
	t.Log("two writer processes, two independent readers, concurrent SQL compaction: 4096 rows, no missing or duplicate IDs")
}
