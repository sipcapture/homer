package mover

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	_ "github.com/duckdb/duckdb-go/v2"
)

// rustfsConfigFromEnv matches examples/docker/docker-compose_s3direct.yaml
// (RUSTFS_ACCESS_KEY=rustfsadmin, host port 9000). Override with HOMER_TEST_S3_*.
func rustfsConfigFromEnv() (S3Config, string) {
	endpoint := getenv("HOMER_TEST_S3_ENDPOINT", "http://127.0.0.1:9000")
	useSSL := strings.HasPrefix(strings.ToLower(endpoint), "https://")
	return S3Config{
		Region:    getenv("HOMER_TEST_S3_REGION", "us-east-1"),
		AccessKey: getenv("HOMER_TEST_S3_ACCESS_KEY", "rustfsadmin"),
		SecretKey: getenv("HOMER_TEST_S3_SECRET_KEY", "rustfsadmin"),
		Endpoint:  endpoint,
		URLStyle:  "path",
		UseSSL:    useSSL,
	}, getenv("HOMER_TEST_S3_BUCKET", "homer-native-move-test")
}

func getenv(k, def string) string {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		return v
	}
	return def
}

func rustfsCopier(t *testing.T) (*s3Copier, string) {
	t.Helper()
	cfg, bucket := rustfsConfigFromEnv()
	c, err := newS3Copier(cfg)
	if err != nil {
		t.Fatalf("newS3Copier: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := c.client.ListBuckets(ctx, &s3.ListBucketsInput{}); err != nil {
		t.Skipf("RustFS/MinIO not reachable at %s: %v", cfg.Endpoint, err)
	}
	_, err = c.client.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: aws.String(bucket)})
	if err != nil {
		msg := strings.ToLower(err.Error())
		if !strings.Contains(msg, "already") {
			t.Fatalf("CreateBucket %s: %v", bucket, err)
		}
	}
	return c, bucket
}

func TestS3CopierMultipartRoundTripRustFS(t *testing.T) {
	c, bucket := rustfsCopier(t)

	dir := t.TempDir()
	src := filepath.Join(dir, "blob.bin")
	// Larger than s3MultipartPartSize so manager.Uploader uses multipart.
	size := int64(s3MultipartPartSize + (2 << 20))
	sum := writeRandomFile(t, src, size)

	key := fmt.Sprintf("native-move/%d.bin", time.Now().UnixNano())
	dst := fmt.Sprintf("s3://%s/%s", bucket, key)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if err := c.Copy(ctx, src, dst, size); err != nil {
		t.Fatalf("Copy: %v", err)
	}

	got, err := c.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		t.Fatalf("GetObject: %v", err)
	}
	defer got.Body.Close()
	h := sha256.New()
	n, err := io.Copy(h, got.Body)
	if err != nil {
		t.Fatalf("read object: %v", err)
	}
	if n != size {
		t.Fatalf("downloaded %d bytes, want %d", n, size)
	}
	if !bytes.Equal(h.Sum(nil), sum) {
		t.Fatal("downloaded object hash mismatch")
	}
	_, _ = c.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
}

func TestNativeMoveLocalToRustFS(t *testing.T) {
	c, bucket := rustfsCopier(t)

	cfg, _ := rustfsConfigFromEnv()
	prefix := fmt.Sprintf("lake-%d", time.Now().UnixNano())
	coldPath := fmt.Sprintf("s3://%s/%s", bucket, prefix)

	f := newTwoLakeFixtureColdRemote(t, cfg, coldPath)
	if ok, reason := addDataFilesSupported(context.Background(), f.db); !ok {
		t.Skip(reason)
	}
	day := "2026-07-18"
	f.insert(t, day, 0, 40)
	if got := f.count(t, "hot", day); got != 40 {
		t.Fatalf("hot before = %d", got)
	}

	opts := f.options(day)
	opts.DstDataPath = coldPath
	opts.S3 = &cfg
	opts.Copier = c

	res, err := Move(context.Background(), opts)
	if err != nil {
		t.Fatalf("Move: %v", err)
	}
	if res.RowsMoved != 40 {
		t.Errorf("rows moved = %d, want 40", res.RowsMoved)
	}
	if got := f.count(t, "hot", day); got != 0 {
		t.Errorf("hot after = %d, want 0", got)
	}
	if got := f.count(t, "cold", day); got != 40 {
		t.Errorf("cold after = %d, want 40", got)
	}
}

func writeRandomFile(t *testing.T, path string, size int64) []byte {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	h := sha256.New()
	w := io.MultiWriter(f, h)
	buf := make([]byte, 64<<10)
	var written int64
	for written < size {
		n := len(buf)
		if left := size - written; int64(n) > left {
			n = int(left)
		}
		if _, err := rand.Read(buf[:n]); err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(buf[:n]); err != nil {
			t.Fatal(err)
		}
		written += int64(n)
	}
	return h.Sum(nil)
}

func newTwoLakeFixtureColdRemote(t *testing.T, s3cfg S3Config, coldPath string) *twoLakeFixture {
	t.Helper()
	dir := t.TempDir()
	hotData := filepath.Join(dir, "hot")
	if err := os.MkdirAll(hotData, 0o755); err != nil {
		t.Fatal(err)
	}
	hotCat := filepath.Join(dir, "hot.sqlite")
	coldCat := filepath.Join(dir, "cold.sqlite")

	db, err := sql.Open("duckdb", "")
	if err != nil {
		t.Skipf("duckdb unavailable: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	db.SetMaxOpenConns(1)
	for _, stmt := range []string{"LOAD ducklake;", "LOAD sqlite;", "LOAD httpfs;"} {
		if _, err := db.Exec(stmt); err != nil {
			t.Skipf("extension unavailable (%q): %v", stmt, err)
		}
	}
	_, _ = db.Exec("LOAD aws;")

	endpoint := strings.TrimPrefix(strings.TrimPrefix(s3cfg.Endpoint, "http://"), "https://")
	secret := fmt.Sprintf(`
		CREATE SECRET s3_secret_cold (
			TYPE S3,
			KEY_ID '%s',
			SECRET '%s',
			REGION '%s',
			ENDPOINT '%s',
			URL_STYLE 'path',
			USE_SSL %t
		);`, s3cfg.AccessKey, s3cfg.SecretKey, s3cfg.Region, endpoint, s3cfg.UseSSL)
	if _, err := db.Exec(secret); err != nil {
		t.Skipf("CREATE SECRET: %v", err)
	}

	if _, err := db.Exec(fmt.Sprintf(
		"ATTACH 'ducklake:sqlite:%s' AS hot (DATA_PATH '%s');", hotCat, hotData)); err != nil {
		t.Skipf("ATTACH hot: %v", err)
	}
	if _, err := db.Exec(fmt.Sprintf(
		"ATTACH 'ducklake:sqlite:%s' AS cold (DATA_PATH '%s');", coldCat, coldPath)); err != nil {
		t.Skipf("ATTACH cold s3: %v", err)
	}

	f := &twoLakeFixture{
		db: db, hotData: hotData, coldData: coldPath,
		hotLake: "hot", coldLake: "cold", table: "calls",
	}
	f.mustExec(t, `CALL hot.set_option('data_inlining_row_limit', 0)`)
	f.mustExec(t, `CALL cold.set_option('data_inlining_row_limit', 0)`)
	ddl := `CREATE TABLE %s.calls (date DATE, id BIGINT, method VARCHAR, payload VARCHAR)`
	f.mustExec(t, fmt.Sprintf(ddl, "hot"))
	f.mustExec(t, fmt.Sprintf(ddl, "cold"))
	f.mustExec(t, `ALTER TABLE hot.calls SET PARTITIONED BY (date)`)
	f.mustExec(t, `ALTER TABLE cold.calls SET PARTITIONED BY (date)`)
	return f
}
