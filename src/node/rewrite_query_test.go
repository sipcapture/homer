package node

import (
	"database/sql"
	"strings"
	"testing"

	"github.com/sipcapture/homer-core/src/config"
)

func TestDuckLakeCatalogForQuery(t *testing.T) {
	cfg := &config.NodeConfig{
		DuckLake: config.DuckLakeConfig{LakeName: "homer_lake"},
	}
	vol := VolumeInfo{Name: "homer", LakeName: "homer_lake_homer"}

	n := &Node{config: cfg, volumes: []VolumeInfo{vol}}
	if got := n.duckLakeCatalogForQuery(vol); got != "homer_lake_homer" {
		t.Fatalf("without sharedDB: got %q want homer_lake_homer", got)
	}

	n.sharedDB = new(sql.DB)
	if got := n.duckLakeCatalogForQuery(vol); got != "homer_lake" {
		t.Fatalf("with sharedDB single volume: got %q want homer_lake", got)
	}
}

func TestRewriteQueryForVolumes_sharedDBSingleVolumeUsesBaseCatalog(t *testing.T) {
	cfg := &config.NodeConfig{
		DuckLake: config.DuckLakeConfig{LakeName: "homer_lake"},
	}
	vol := VolumeInfo{Name: "homer", LakeName: "homer_lake_homer"}
	n := &Node{
		config:   cfg,
		volumes:  []VolumeInfo{vol},
		sharedDB: new(sql.DB),
	}

	sqlIn := "SELECT * FROM homer_lake.main.hep_proto_1_call WHERE x = 1"
	got := n.rewriteQueryForVolumes(sqlIn)
	if !strings.Contains(got, "homer_lake.main.hep_proto_1_call") {
		t.Fatalf("expected base catalog in sql: %s", got)
	}
	if strings.Contains(got, "homer_lake_homer") {
		t.Fatalf("unexpected suffixed catalog: %s", got)
	}
}

func TestRewriteQueryForVolumes_sharedDBMultiVolumeSkipsUnion(t *testing.T) {
	cfg := &config.NodeConfig{
		DuckLake: config.DuckLakeConfig{LakeName: "homer_lake"},
	}
	n := &Node{
		config: cfg,
		volumes: []VolumeInfo{
			{Name: "hot", LakeName: "homer_lake_hot"},
			{Name: "cold", LakeName: "homer_lake_cold"},
		},
		sharedDB: new(sql.DB),
	}

	sqlIn := "SELECT * FROM homer_lake.main.hep_proto_1_call WHERE x = 1 ORDER BY timestamp DESC LIMIT 50"
	got := n.rewriteQueryForVolumes(sqlIn)
	if got != sqlIn {
		t.Fatalf("sharedDB + tier volumes: expected sql unchanged, got:\n%s", got)
	}
	if strings.Contains(got, "UNION ALL") {
		t.Fatalf("unexpected UNION ALL rewrite on sharedDB: %s", got)
	}
}

func TestTryBuildVolumeUnionSQL_multiVolume(t *testing.T) {
	cfg := &config.NodeConfig{DuckLake: config.DuckLakeConfig{LakeName: "homer_lake"}}
	n := &Node{
		config: cfg,
		volumes: []VolumeInfo{
			{Name: "hot", LakeName: "homer_lake_hot"},
			{Name: "cold", LakeName: "homer_lake_cold"},
		},
		sharedDB: new(sql.DB),
	}
	sql := "SELECT * FROM homer_lake.main.hep_proto_1_call WHERE 1=1"
	out, ok := n.tryBuildVolumeUnionSQL(sql)
	if !ok {
		t.Fatal("expected ok")
	}
	if !strings.Contains(out, "homer_lake_hot") || !strings.Contains(out, "UNION ALL") || !strings.Contains(out, "homer_lake_cold") {
		t.Fatalf("expected union across hot+cold: %s", out)
	}
}

func TestExtractSQLLimit(t *testing.T) {
	if g := extractSQLLimit("SELECT 1 ORDER BY x DESC LIMIT 25"); g != 25 {
		t.Fatalf("got %d", g)
	}
	if g := extractSQLLimit("SELECT 1"); g != 0 {
		t.Fatalf("got %d", g)
	}
}
