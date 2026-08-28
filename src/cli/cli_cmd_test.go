package cli

import (
	"database/sql"
	"strings"
	"testing"

	"github.com/sipcapture/homer-core/src/config"
	"github.com/sipcapture/homer-core/src/storage/ducklake"
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

// assertWriterSecret checks whether the named DuckDB secret (homer_writer_s3
// or homer_writer_azure — see tuning.go's writerLakeS3Secret /
// writerLakeAzureSecret, unexported so referenced here by literal string)
// exists after openDuckLakeReadOnly. Skips rather than fails when
// duckdb_secrets() itself is unavailable (matches the ducklake package's own
// extension-unavailable skip convention).
func assertWriterSecret(t *testing.T, db *sql.DB, name string, wantExists bool) {
	t.Helper()
	var count int
	row := db.QueryRow("SELECT COUNT(*) FROM duckdb_secrets() WHERE name = '" + name + "'")
	if err := row.Scan(&count); err != nil {
		t.Skipf("duckdb_secrets() query unavailable: %v", err)
	}
	exists := count > 0
	if exists != wantExists {
		t.Errorf("secret %q exists=%v, want %v", name, exists, wantExists)
	}
}

// TestDuckLakeConfigFromModular_AzureAccountNameOnly is a regression test
// for PR review should-fix (github.com/sipcapture/homer/pull/983): "no
// writer/CLI wiring test that Azure is copied when only account_name is set
// (MI)". The Managed Identity case has no account_key and no
// connection_string — only account_name — so a gate checking the wrong
// fields could silently drop it.
func TestDuckLakeConfigFromModular_AzureAccountNameOnly(t *testing.T) {
	cfg := &config.Config{
		Storage: config.StorageConfig{
			Enable: true,
			DuckLake: config.DuckLakeConfig{
				DataPath: "az://container/lake/",
				Azure:    config.AzureConfig{AccountName: "myaccount"},
			},
		},
	}
	got := duckLakeConfigFromModular(cfg)
	if got.AzureAccountName != "myaccount" {
		t.Errorf("AzureAccountName = %q, want %q", got.AzureAccountName, "myaccount")
	}
	if got.AzureAccountKey != "" || got.AzureConnectionString != "" {
		t.Errorf("expected no key/connection string, got key=%q connstr=%q",
			got.AzureAccountKey, got.AzureConnectionString)
	}
}

// TestOpenDuckLakeReadOnly_S3PathDoesNotCreateAzureSecret is a regression
// test for PR review must-fix #3 (github.com/sipcapture/homer/pull/983):
// an S3-only data_path used to also attempt LOAD azure and create an azure
// secret via the generic IsRemoteLakeDataPath gate, spamming S3-only homer
// cli users with a spurious "failed to load azure extension" warning.
func TestOpenDuckLakeReadOnly_S3PathDoesNotCreateAzureSecret(t *testing.T) {
	cfg := ducklake.Config{
		DataPath:          "s3://fake-bucket/lake/",
		LakeName:          "homer_lake",
		S3Region:          "us-east-1",
		S3AccessKeyID:     "fakekey",
		S3SecretAccessKey: "fakesecret",
	}
	db, err := openDuckLakeReadOnly(cfg)
	if err != nil {
		t.Skipf("openDuckLakeReadOnly unavailable: %v", err)
	}
	defer db.Close()

	assertWriterSecret(t, db, "homer_writer_s3", true)
	assertWriterSecret(t, db, "homer_writer_azure", false)
}

// TestOpenDuckLakeReadOnly_AzurePathDoesNotCreateS3Secret is the mirror
// case: an az:// data_path must create the azure secret and must not
// attempt S3 secret creation (EnsureWriterS3Secret is already a no-op with
// no S3AccessKeyID, so this mainly documents the intended branching).
//
// Uses a fake CONNECTION_STRING, not a bare account_name (which would
// create a PROVIDER credential_chain secret): the immediately-following
// createParquetViews glob() attempt would then try to resolve Managed
// Identity via IMDS (169.254.169.254), which has no route in a sandboxed
// test environment and hangs for ~10 minutes before Go's test timeout
// kills it, rather than failing fast — confirmed by first writing the test
// that way and watching it time out. A connection string secret never
// attempts credential resolution, so the subsequent glob() fails fast on
// DNS/connection instead.
func TestOpenDuckLakeReadOnly_AzurePathDoesNotCreateS3Secret(t *testing.T) {
	cfg := ducklake.Config{
		DataPath:              "az://fake-container/lake/",
		LakeName:              "homer_lake",
		AzureConnectionString: "DefaultEndpointsProtocol=https;AccountName=fake;AccountKey=ZmFrZQ==;EndpointSuffix=core.windows.net",
	}
	db, err := openDuckLakeReadOnly(cfg)
	if err != nil {
		t.Skipf("openDuckLakeReadOnly unavailable: %v", err)
	}
	defer db.Close()

	assertWriterSecret(t, db, "homer_writer_azure", true)
	assertWriterSecret(t, db, "homer_writer_s3", false)
}
