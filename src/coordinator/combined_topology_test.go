//go:build !vet

package coordinator

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/google/uuid"
	"github.com/sipcapture/homer-core/src/config"
	"github.com/sipcapture/homer-core/src/decoder"
	"github.com/sipcapture/homer-core/src/node"
	"github.com/sipcapture/homer-core/src/writer"
)

// Mirror main's combined process: network ingest, writer, shared local node DB,
// and coordinator. Both replicas use shared PostgreSQL and remote Parquet.
func startCombinedTestModules(t *testing.T, cfg *config.Config) func() {
	t.Helper()
	if !cfg.Ingest.Enable || !cfg.Storage.Enable || !cfg.Node.Enable {
		return func() {}
	}
	w, err := writer.New(&cfg.Ingest, &cfg.Storage, &cfg.Prometheus, &cfg.RemoteLogging)
	if err != nil {
		t.Fatal(err)
	}
	n, err := node.New(&cfg.Node)
	if err != nil {
		_ = w.Stop()
		t.Fatal(err)
	}
	n.SetSharedDB(w.GetDB())
	if err := w.Start(); err != nil {
		_ = n.Stop()
		_ = w.Stop()
		t.Fatal(err)
	}
	if err := n.Start(); err != nil {
		_ = n.Stop()
		_ = w.Stop()
		t.Fatal(err)
	}
	return func() { _ = n.Stop(); _ = w.Stop() }
}

func combinedTestPort(t *testing.T, pair bool) int {
	t.Helper()
	for i := 0; i < 100; i++ {
		l, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		port := l.Addr().(*net.TCPAddr).Port
		if pair {
			next, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port+1))
			if err != nil {
				_ = l.Close()
				continue
			}
			_ = next.Close()
		}
		_ = l.Close()
		return port
	}
	t.Fatal("no free node port pair")
	return 0
}

func combinedReplicaConfigs(t *testing.T, settings map[string]any) []string {
	t.Helper()
	endpoint := os.Getenv("HOMER_TEST_S3_ENDPOINT")
	if endpoint == "" {
		t.Skip("set HOMER_TEST_S3_ENDPOINT for combined topology integration")
	}
	lakeDSN := statelessTestDatabase(t)
	access, secret := os.Getenv("HOMER_TEST_S3_ACCESS_KEY"), os.Getenv("HOMER_TEST_S3_SECRET_KEY")
	client := s3.NewFromConfig(aws.Config{Region: "us-east-1", Credentials: credentials.NewStaticCredentialsProvider(access, secret, "")},
		func(o *s3.Options) { o.BaseEndpoint = aws.String(endpoint); o.UsePathStyle = true })
	bucket := "homer-combined-" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err := client.CreateBucket(context.Background(), &s3.CreateBucketInput{Bucket: aws.String(bucket)}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		pages := s3.NewListObjectsV2Paginator(client, &s3.ListObjectsV2Input{Bucket: aws.String(bucket)})
		for pages.HasMorePages() {
			page, err := pages.NextPage(ctx)
			if err != nil {
				t.Error(err)
				return
			}
			for _, object := range page.Contents {
				if _, err := client.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: aws.String(bucket), Key: object.Key}); err != nil {
					t.Error(err)
				}
			}
		}
		if _, err := client.DeleteBucket(ctx, &s3.DeleteBucketInput{Bucket: aws.String(bucket)}); err != nil {
			t.Error(err)
		}
	})
	base, _ := json.Marshal(settings)
	paths := make([]string, 2)
	for i := range paths {
		var replica map[string]any
		if err := json.Unmarshal(base, &replica); err != nil {
			t.Fatal(err)
		}
		dir := t.TempDir()
		lake := map[string]any{
			"catalog_type": "postgres", "catalog_path": lakeDSN, "lake_name": "homer_lake",
			"data_path": "s3://" + bucket + "/parquet", "batch_size": 8, "flush_interval_sec": 1, "data_inlining_row_limit": 0,
			"compaction": map[string]any{"enable": false},
			"s3": map[string]any{"region": "us-east-1", "access_key_id": access, "secret_access_key": secret,
				"endpoint": strings.TrimPrefix(strings.TrimPrefix(endpoint, "http://"), "https://"),
				"use_ssl":  strings.HasPrefix(endpoint, "https://"), "url_style": "path"},
			"tuning": map[string]any{"threads": 1, "memory_limit": "256MB", "temp_directory": filepath.Join(dir, "spill")},
		}
		replica["storage"] = map[string]any{"enable": true, "ducklake": lake}
		replica["ingest"] = map[string]any{"enable": true, "worker_count": 1,
			"hep_stream": map[string]any{"enable": false}, "udp": map[string]any{"enable": false},
			"http": map[string]any{"enable": true, "host": "127.0.0.1", "port": combinedTestPort(t, false)}}
		replica["node"] = map[string]any{"enable": true, "ducklake": lake,
			"flight_server": map[string]any{"host": "127.0.0.1", "port": combinedTestPort(t, true), "auth_token": "combined-node-token"}}
		replica["coordinator"].(map[string]any)["lake_name"] = "homer_lake"
		paths[i] = filepath.Join(dir, "combined.json")
		data, _ := json.Marshal(replica)
		if err := os.WriteFile(paths[i], data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	return paths
}

func verifyCombinedIngestAndSearch(t *testing.T, paths, addresses []string, token string) {
	t.Helper()
	cfg, err := config.Load(paths[0])
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Timeout: 10 * time.Second}
	const rows = 24
	for i := 0; i < rows; i++ {
		packet := decoder.BenchHEP3SIPPacket()
		// Equal-length Call-IDs preserve HEP chunk lengths.
		packet = bytes.ReplaceAll(packet, []byte("bench-callid-12345"), []byte(fmt.Sprintf("bench-callid-%05d", i)))
		resp, err := client.Post(fmt.Sprintf("http://127.0.0.1:%d/api/hep", cfg.Ingest.HTTP.Port), "application/octet-stream", bytes.NewReader(packet))
		if err != nil {
			t.Fatal(err)
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("ingest status %d", resp.StatusCode)
		}
	}
	now := time.Now()
	body, _ := json.Marshal(map[string]any{"filter": map[string]any{"proto_type": 1, "event_type": "call"},
		"param": map[string]any{"limit": 100}, "timestamp": map[string]any{"from": now.Add(-time.Minute).UnixMilli(), "to": now.Add(time.Minute).UnixMilli()}})
	deadline := time.Now().Add(45 * time.Second)
	var last string
	for time.Now().Before(deadline) {
		// B's coordinator only routes to B's node, never to the ingesting replica.
		req, _ := http.NewRequest(http.MethodPost, addresses[1]+"/api/v4/transactions/search", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		payload, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if err != nil {
			t.Fatal(err)
		}
		last = string(payload)
		var result struct {
			Data struct{ Items []map[string]any }
		}
		if resp.StatusCode == http.StatusOK && json.Unmarshal(payload, &result) == nil && len(result.Data.Items) == rows {
			seen := make(map[string]bool)
			for _, row := range result.Data.Items {
				id, _ := row["cid"].(string)
				if id == "" {
					id, _ = row["session_id"].(string)
				}
				if id == "" || seen[id] {
					t.Fatalf("missing/duplicate Call-ID: %v", row)
				}
				seen[id] = true
			}
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("replica B did not find %d unique rows ingested on A: %s", rows, last)
}
