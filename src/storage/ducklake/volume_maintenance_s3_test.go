// Copyright (C) 2026 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package ducklake

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// rustfsVolumeFromEnv matches examples/docker/docker-compose_s3direct.yaml
// (RustFS on 127.0.0.1:9000, rustfsadmin/rustfsadmin). Override with HOMER_TEST_S3_*.
func rustfsVolumeFromEnv(t *testing.T) (Volume, *s3.Client, string, string) {
	t.Helper()
	env := func(k, def string) string {
		if v := strings.TrimSpace(os.Getenv(k)); v != "" {
			return v
		}
		return def
	}
	endpoint := env("HOMER_TEST_S3_ENDPOINT", "http://127.0.0.1:9000")
	region := env("HOMER_TEST_S3_REGION", "us-east-1")
	access := env("HOMER_TEST_S3_ACCESS_KEY", "rustfsadmin")
	secret := env("HOMER_TEST_S3_SECRET_KEY", "rustfsadmin")
	bucket := env("HOMER_TEST_S3_BUCKET", "homer-maintenance-test")

	client := s3.New(s3.Options{
		Region:       region,
		BaseEndpoint: aws.String(endpoint),
		UsePathStyle: true,
		Credentials:  credentials.NewStaticCredentialsProvider(access, secret, ""),
	})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if _, err := client.ListBuckets(ctx, &s3.ListBucketsInput{}); err != nil {
		t.Skipf("RustFS/MinIO not reachable at %s: %v", endpoint, err)
	}
	if _, err := client.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: aws.String(bucket)}); err != nil &&
		!strings.Contains(strings.ToLower(err.Error()), "already") {
		t.Fatalf("CreateBucket %s: %v", bucket, err)
	}

	prefix := fmt.Sprintf("maintenance-%d", time.Now().UnixNano())
	return Volume{
		Type:        VolumeTypeS3,
		Path:        fmt.Sprintf("s3://%s/%s/", bucket, prefix),
		S3Region:    region,
		S3AccessKey: access,
		S3SecretKey: secret,
		S3Endpoint:  endpoint,
		S3UseSSL:    strings.HasPrefix(strings.ToLower(endpoint), "https://"),
		S3URLStyle:  "path",
	}, client, bucket, prefix
}

func countS3Objects(t *testing.T, client *s3.Client, bucket, prefix string) int {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	n := 0
	p := s3.NewListObjectsV2Paginator(client, &s3.ListObjectsV2Input{
		Bucket: aws.String(bucket),
		Prefix: aws.String(prefix + "/"),
	})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			t.Fatalf("list s3://%s/%s: %v", bucket, prefix, err)
		}
		n += len(page.Contents)
	}
	return n
}

// TestRunVolumeMaintenanceS3RemovesObjectsRustFS runs the full cold S3
// maintenance path against a real object store: the maintenance DuckDB gets
// its own secret, attaches the cold catalog and deletes the objects.
func TestRunVolumeMaintenanceS3RemovesObjectsRustFS(t *testing.T) {
	coldCfg, client, bucket, prefix := rustfsVolumeFromEnv(t)
	tsm, cold := startTieredFixture(t, coldCfg)

	scheduleColdFiles(t, tsm.GetDB())
	if n := countS3Objects(t, client, bucket, prefix); n == 0 {
		t.Fatal("expected parquet objects on S3 before maintenance")
	}

	if err := tsm.RunVolumeMaintenance(cold, 1); err != nil {
		t.Fatalf("RunVolumeMaintenance: %v", err)
	}

	if !tsm.maint.attached[cold.LakeName] {
		t.Fatal("S3 file cleanup must attach the cold lake on the maintenance DuckDB")
	}
	if n := countS3Objects(t, client, bucket, prefix); n != 0 {
		t.Fatalf("objects left on S3 after maintenance = %d, want 0", n)
	}
	scheduled, err := scheduledFileCount(tsm.GetDB(), cold.LakeName)
	if err != nil {
		t.Fatal(err)
	}
	if scheduled != 0 {
		t.Fatalf("files still scheduled for deletion = %d, want 0", scheduled)
	}
	if tsm.maint.orphanScanDue(cold, time.Now()) {
		t.Fatal("orphan scan must be recorded after the first S3 maintenance run")
	}
}

// TestRunVolumeMaintenanceS3WithoutDeletePermissionRustFS reproduces
// sipcapture/homer#1037: the cold credentials may write but not delete.
// Needs a RustFS/MinIO user whose policy omits s3:DeleteObject, passed via
// HOMER_TEST_S3_NODELETE_ACCESS_KEY / HOMER_TEST_S3_NODELETE_SECRET_KEY.
func TestRunVolumeMaintenanceS3WithoutDeletePermissionRustFS(t *testing.T) {
	access := strings.TrimSpace(os.Getenv("HOMER_TEST_S3_NODELETE_ACCESS_KEY"))
	secret := strings.TrimSpace(os.Getenv("HOMER_TEST_S3_NODELETE_SECRET_KEY"))
	if access == "" || secret == "" {
		t.Skip("HOMER_TEST_S3_NODELETE_ACCESS_KEY / _SECRET_KEY not set")
	}
	coldCfg, client, bucket, prefix := rustfsVolumeFromEnv(t)
	coldCfg.S3AccessKey, coldCfg.S3SecretKey = access, secret
	tsm, cold := startTieredFixture(t, coldCfg)

	scheduleColdFiles(t, tsm.GetDB())
	before := countS3Objects(t, client, bucket, prefix)
	logs := captureWarnings(t)

	err := tsm.RunVolumeMaintenance(cold, 1)
	if err == nil || !strings.Contains(err.Error(), "AccessDenied") {
		t.Fatalf("RunVolumeMaintenance error = %v, want AccessDenied", err)
	}
	if after := countS3Objects(t, client, bucket, prefix); after != before {
		t.Fatalf("objects after denied cleanup = %d, want %d", after, before)
	}
	scheduled, err := scheduledFileCount(tsm.GetDB(), cold.LakeName)
	if err != nil {
		t.Fatal(err)
	}
	if scheduled == 0 {
		t.Fatal("denied deletes must stay scheduled for the next cycle")
	}
	out := logs.String()
	if !strings.Contains(out, "did not reduce files scheduled for deletion") || !strings.Contains(out, "s3:DeleteObject") {
		t.Fatalf("expected delete-permission hint, got: %s", out)
	}
}
