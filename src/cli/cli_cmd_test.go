package cli

import (
	"strings"
	"testing"
)

func TestCreateParquetViewSQL_usesDatePartitionGlob(t *testing.T) {
	sql := createParquetViewSQL("homer_lake", "hep_proto_1_call", "/data/homer/parquet")
	want := "/data/homer/parquet/main/hep_proto_1_call/date=*/**/*.parquet"
	if !strings.Contains(sql, want) {
		t.Fatalf("expected date-partition glob %q, got:\n%s", want, sql)
	}
	if strings.Contains(sql, "SELECT file FROM glob") {
		t.Fatalf("read_parquet must not use subquery, got:\n%s", sql)
	}
	if !strings.Contains(sql, "hive_partitioning=true") {
		t.Fatalf("expected hive_partitioning, got:\n%s", sql)
	}
}
