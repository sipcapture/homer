package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestWizardProfileAllInOneEmitsNodeVolumes(t *testing.T) {
	out := filepath.Join(t.TempDir(), "homer.json")
	if err := RunWizardCmd(WizardFlags{Profile: "all-in-one", Output: out}); err != nil {
		t.Fatalf("RunWizardCmd: %v", err)
	}
	raw, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	var cfg map[string]any
	if err := json.Unmarshal(raw, &cfg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	node, _ := cfg["node"].(map[string]any)
	dl, _ := node["ducklake"].(map[string]any)
	vols, _ := dl["volumes"].([]any)
	if len(vols) != 1 {
		t.Fatalf("node.ducklake.volumes: want 1, got %d\n%s", len(vols), raw)
	}
	vol, _ := vols[0].(map[string]any)
	if vol["name"] != "default" || vol["type"] != "local" {
		t.Fatalf("volume: %+v", vol)
	}
	if vol["catalog_path"] != "/data/homer/homer_catalog.sqlite" || vol["path"] != "/data/homer/parquet" {
		t.Fatalf("volume paths: %+v", vol)
	}
}
