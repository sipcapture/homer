package writer

import (
	"testing"

	"github.com/sipcapture/homer-core/src/config"
)

func TestShouldAutoEnableCompactionForTieredHot(t *testing.T) {
	w := &Writer{
		storageConfig: &config.StorageConfig{
			DuckLake: config.DuckLakeConfig{
				StoragePolicy: config.StoragePolicyConfig{
					Enable: true,
					Volumes: []config.VolumeConfig{
						{Name: "hot", Type: "local", Path: "/tmp/h"},
						{Name: "cold", Type: "s3", Path: "s3://b/c/"},
					},
				},
			},
		},
	}
	if !w.shouldAutoEnableCompactionForTieredHot() {
		t.Fatal("expected true for multi-volume policy with local volume")
	}

	w.storageConfig.DuckLake.StoragePolicy.Volumes = []config.VolumeConfig{
		{Name: "cold", Type: "s3"},
		{Name: "colder", Type: "s3"},
	}
	if w.shouldAutoEnableCompactionForTieredHot() {
		t.Fatal("expected false when no local volume")
	}

	w.storageConfig.DuckLake.StoragePolicy.Enable = false
	if w.shouldAutoEnableCompactionForTieredHot() {
		t.Fatal("expected false when policy disabled")
	}
}
