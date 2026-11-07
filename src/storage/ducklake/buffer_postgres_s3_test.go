//go:build !vet

package ducklake

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/sipcapture/homer-core/src/decoder"
)

func TestPostgresObjectStorageBufferReuse(t *testing.T) {
	for _, queue := range []bool{false, true} {
		t.Run(fmt.Sprintf("central_queue=%t", queue), func(t *testing.T) {
			cfg, client, bucket, _ := postgresObjectStorageFixture(t, time.Hour, queue)
			cfg.BatchSize = 1
			mtw, err := NewMultiTableWriter(cfg)
			if err != nil {
				t.Fatal(err)
			}
			mtw.Start()
			t.Cleanup(func() {
				if err := mtw.Stop(); err != nil {
					t.Error(err)
				}
			})
			hep, err := decoder.DecodeHEP(decoder.BenchHEP3SIPPacket())
			if err != nil {
				t.Fatal(err)
			}
			defer decoder.ReleaseHEP(hep)
			key, values, err := ConvertHEPToLakeRow(hep)
			if err != nil {
				t.Fatal(err)
			}
			putRowSlice(values)
			if _, err := mtw.getOrCreateTable(key); err != nil {
				t.Fatal(err)
			}
			write := func(i int) error {
				hep.CID = fmt.Sprintf("buffer-reuse-%d", i)
				_, row, err := ConvertHEPToLakeRow(hep)
				if err != nil {
					return err
				}
				return mtw.WriteRecord(key, row)
			}
			mtw.catalogMu.Lock()
			var once sync.Once
			release := func() { once.Do(mtw.catalogMu.Unlock) }
			defer release()
			if err := write(0); err != nil {
				t.Fatal(err)
			}
			mtw.flushAll()
			if err := write(1); err != nil {
				t.Fatal(err)
			}
			mtw.flushAll()
			done := make(chan error, 1)
			go func() { done <- write(2) }()
			select {
			case err := <-done:
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("queued PostgreSQL/S3 flush blocked buffer reuse")
			}
			release()
			if err := mtw.Stop(); err != nil {
				t.Fatal(err)
			}
			reader, err := OpenMaintenanceDB(cfg, "256MB", nil)
			if err != nil {
				t.Fatal(err)
			}
			defer reader.Close()
			var rows, distinct int
			if err := reader.DB.QueryRow("SELECT count(*), count(DISTINCT cid) FROM lake.hep_proto_"+key.String()).Scan(&rows, &distinct); err != nil || rows != 3 || distinct != 3 {
				t.Fatalf("remote flush lost or duplicated rows: %d/%d %v", rows, distinct, err)
			}
			objects, err := client.ListObjectsV2(context.Background(), &s3.ListObjectsV2Input{Bucket: aws.String(bucket)})
			if err != nil || len(objects.Contents) == 0 {
				t.Fatalf("no remote Parquet objects: %v", err)
			}
		})
	}
}

func TestPostgresObjectStorageFailedClear(t *testing.T) {
	cfg, _, _, _ := postgresObjectStorageFixture(t, time.Hour, false)
	cfg.BatchSize = 1
	mtw, err := NewMultiTableWriter(cfg)
	if err != nil {
		t.Fatal(err)
	}
	mtw.Start()
	t.Cleanup(func() {
		if err := mtw.Stop(); err != nil {
			t.Error(err)
		}
	})
	hep, err := decoder.DecodeHEP(decoder.BenchHEP3SIPPacket())
	if err != nil {
		t.Fatal(err)
	}
	defer decoder.ReleaseHEP(hep)
	write := func(id string) (TableKey, error) {
		hep.CID = id
		key, row, err := ConvertHEPToLakeRow(hep)
		if err != nil {
			return key, err
		}
		return key, mtw.WriteRecord(key, row)
	}
	key, err := write("committed")
	if err != nil {
		t.Fatal(err)
	}
	tw, err := mtw.getOrCreateTable(key)
	if err != nil {
		t.Fatal(err)
	}
	truncate := tw.flushTruncateSQL[0]
	tw.flushTruncateSQL[0] = "TRUNCATE TABLE missing_buffer"
	if err := tw.flushSlotDirect(0, &mtw.catalogMu); err == nil {
		t.Fatal("expected clear failure after remote INSERT")
	}
	if _, err := write("retained"); err == nil {
		t.Fatal("expected failed clear to block appending")
	}
	tw.flushTruncateSQL[0] = truncate
	if err := tw.flushBatch(); err != nil {
		t.Fatal(err)
	}
	if err := tw.flushSlotDirect(0, &mtw.catalogMu); err != nil {
		t.Fatal(err)
	}
	var rows, distinct int
	if err := mtw.db.QueryRow("SELECT count(*), count(DISTINCT cid) FROM lake.hep_proto_"+key.String()).Scan(&rows, &distinct); err != nil || rows != 2 || distinct != 2 {
		t.Fatalf("remote clear retry lost or duplicated rows: %d/%d %v", rows, distinct, err)
	}
}
