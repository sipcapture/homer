package cli

import (
	"encoding/json"
	tea "github.com/charmbracelet/bubbletea"
	"os"
	"path/filepath"
	"strings"
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

func TestWizardPostgresDatabaseChoices(t *testing.T) {
	for _, backend := range []string{"sqlite", "postgres", "postgresql"} {
		t.Run(backend, func(t *testing.T) {
			m := newWizardModel(filepath.Join(t.TempDir(), "homer.json"))
			m.inputs[wfCatalogType].SetValue(backend)
			m.inputs[wfSettingsDBType].SetValue(backend)
			want := "sqlite"
			if backend != "sqlite" {
				want = "postgres"
				m.inputs[wfCatalogPath].SetValue("POSTGRES://homer:private@localhost/catalog")
				m.inputs[wfSettingsDB].SetValue("host=localhost dbname=settings user=homer password=private")
			}
			cfg := m.buildConfigFromInputs()
			if cfg.Storage.DuckLake.CatalogType != want || cfg.Node.DuckLake.CatalogType != want || cfg.Node.DuckLake.Volumes[0].CatalogType != want || cfg.Coordinator.SettingsDBType != want {
				t.Fatal("wizard lost the selected backend")
			}
			m.step = wizStepReview
			updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
			saved := updated.(wizardModel)
			if !saved.saved || saved.errMsg != "" {
				t.Fatalf("save failed: %s", saved.errMsg)
			}
			data, err := os.ReadFile(m.inputs[wfOutputPath].Value())
			if err != nil {
				t.Fatal(err)
			}
			var decoded struct {
				Coordinator struct {
					SettingsDBType string `json:"settings_db_type"`
				} `json:"coordinator"`
			}
			if err := json.Unmarshal(data, &decoded); err != nil {
				t.Fatal(err)
			}
			if decoded.Coordinator.SettingsDBType != want {
				t.Fatal("saved settings backend differs")
			}
		})
	}
}

func TestWizardRejectsDatabaseMismatch(t *testing.T) {
	for _, field := range []int{wfCatalogPath, wfSettingsDB} {
		t.Run(map[int]string{wfCatalogPath: "catalog", wfSettingsDB: "settings"}[field], func(t *testing.T) {
			out := filepath.Join(t.TempDir(), "homer.json")
			m := newWizardModel(out)
			m.step = wizStepReview
			m.inputs[field].SetValue("postgres://homer:private@localhost/database")
			updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
			got := updated.(wizardModel)
			if got.saved || got.errMsg == "" || strings.Contains(got.errMsg, "private") {
				t.Fatalf("invalid configuration handling: saved=%t error=%q", got.saved, got.errMsg)
			}
			if _, err := os.Stat(out); !os.IsNotExist(err) {
				t.Fatal("invalid configuration was written")
			}
		})
	}
}

func TestWizardNodeProfileOffersCatalogChoice(t *testing.T) {
	m := newWizardModel("")
	applyProfileDefaults(m.inputs, "node")
	m.step = wizStepIngest
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if got := updated.(wizardModel); got.step != wizStepStorage {
		t.Fatal("node-only wizard skipped catalog configuration")
	}
}
