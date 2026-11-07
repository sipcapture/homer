// Build from src: go build -o ../local/stateless-maintenance-probe ../scripts/stateless-scenario/maintenance_probe.go
// Exercises the production tiering manager against the scenario's real catalog.
package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/sipcapture/homer-core/src/storage/ducklake"
)

func run() error {
	dsn := os.Getenv("HOMER_STORAGE_DUCKLAKE_CATALOG_PATH")
	path := os.Getenv("SCENARIO_DATA_PATH")
	if dsn == "" || path == "" {
		return fmt.Errorf("scenario catalog and data path required")
	}
	volume := ducklake.Volume{Name: "hot", LakeName: "hot", Type: ducklake.VolumeTypeS3,
		CatalogPath: dsn, Path: path, S3Endpoint: os.Getenv("AWS_ENDPOINT_URL_S3"),
		S3Region: os.Getenv("AWS_REGION"), S3URLStyle: "path",
		S3UseSSL:    strings.HasPrefix(os.Getenv("AWS_ENDPOINT_URL_S3"), "https://"),
		S3AccessKey: os.Getenv("AWS_ACCESS_KEY_ID"), S3SecretKey: os.Getenv("AWS_SECRET_ACCESS_KEY")}
	manager, err := ducklake.NewTieredStorageManager(ducklake.TieredStorageConfig{
		CatalogType: ducklake.CatalogPostgres, CatalogPath: dsn, Volumes: []ducklake.Volume{volume},
		TuningThreads: 1, TuningMemoryLimit: "256MB", TuningTempDirectory: os.Getenv("SCENARIO_SPILL_PATH"),
	})
	if err != nil {
		return fmt.Errorf("initialize tiering manager")
	}
	if err := manager.Start(); err != nil {
		return fmt.Errorf("attach scenario tiering catalog")
	}
	defer manager.Stop()
	for _, operation := range []struct {
		name string
		call func() error
	}{
		// A date without scenario data also prevents deletion if contention regresses.
		{"delete", func() error { return manager.DeletePartition("hep_proto_1_call", "1900-01-01", &volume) }},
		{"cleanup", func() error { return manager.RunVolumeMaintenance(&volume, 3600) }},
	} {
		if err := operation.call(); err == nil || !strings.Contains(err.Error(), "already running") {
			return fmt.Errorf("tier %s did not reject maintenance contention", operation.name)
		}
	}
	fmt.Println("SCENARIO_TIER_CONTENTION_PASSED")
	return nil
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
