package node

import (
	"database/sql"
	"fmt"
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

func TestDuckDBForTieredTablePresence(t *testing.T) {
	cfg := &config.NodeConfig{DuckLake: config.DuckLakeConfig{LakeName: "homer_lake"}}
	dbMain := new(sql.DB)
	dbTier := new(sql.DB)
	n := &Node{config: cfg, db: dbMain}
	if g := n.duckDBForTieredTablePresence(); g != dbMain {
		t.Fatalf("without sharedDB: got %p want db", g)
	}
	n.sharedDB = new(sql.DB)
	if g := n.duckDBForTieredTablePresence(); g != dbMain {
		t.Fatalf("sharedDB without tieredQueryDB: got %p want db", g)
	}
	n.tieredQueryDB = dbTier
	if g := n.duckDBForTieredTablePresence(); g != dbTier {
		t.Fatalf("sharedDB with tieredQueryDB: got %p want tieredQueryDB", g)
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

func defaultMemoryUnionNode() *Node {
	return &Node{
		config: &config.NodeConfig{
			DuckLake: config.DuckLakeConfig{LakeName: "homer_lake"},
		},
		sharedDB: new(sql.DB),
		volumes: []VolumeInfo{
			{Name: "default", LakeName: "homer_lake"},
		},
	}
}

func searchSQLWithRange(fromMs, toMs int64) string {
	return fmt.Sprintf(
		"SELECT *, 'homer_lake' AS storage_lake, 'default' AS storage_volume FROM homer_lake.main.hep_proto_1_call WHERE timestamp >= (to_timestamp(%d / 1000.0) AT TIME ZONE 'UTC') AND timestamp <= (to_timestamp(%d / 1000.0) AT TIME ZONE 'UTC') ORDER BY timestamp DESC LIMIT 50",
		fromMs, toMs,
	)
}

func TestBuildMemoryUnionQueries(t *testing.T) {
	n := defaultMemoryUnionNode()
	sqlIn := searchSQLWithRange(1_700_000_000_000, 1_700_000_300_000)
	q := n.buildMemoryUnionQueries(sqlIn)
	if !q.ok {
		t.Fatal("expected memory union query plan")
	}
	if !strings.Contains(q.combinedSQL, "mem_hep_proto_1_call_a") || !strings.Contains(q.combinedSQL, "mem_hep_proto_1_call_b") {
		t.Fatalf("expected combined SQL to include both memory buffers, got: %s", q.combinedSQL)
	}
	if !strings.Contains(q.memSQL, "UNION ALL") {
		t.Fatalf("expected mem SQL to union both buffers, got: %s", q.memSQL)
	}
}

func TestShouldSplitLakeAndMemThresholdAndFallback(t *testing.T) {
	smallRangeSQL := searchSQLWithRange(1_700_000_000_000, 1_700_003_000_000) // 50 min
	orderBy, limit, base := extractOrderLimit(smallRangeSQL)
	if shouldSplitLakeAndMem(smallRangeSQL, base, orderBy, limit) {
		t.Fatal("did not expect split for <= 1h range")
	}

	largeRangeSQL := searchSQLWithRange(1_700_000_000_000, 1_700_007_500_000) // > 2h
	orderBy, limit, base = extractOrderLimit(largeRangeSQL)
	if !shouldSplitLakeAndMem(largeRangeSQL, base, orderBy, limit) {
		t.Fatal("expected split for > 1h range")
	}

	customOrderSQL := strings.Replace(largeRangeSQL, "ORDER BY timestamp DESC", "ORDER BY date DESC", 1)
	orderBy, limit, base = extractOrderLimit(customOrderSQL)
	if shouldSplitLakeAndMem(customOrderSQL, base, orderBy, limit) {
		t.Fatal("expected fallback to legacy single-UNION path for custom ORDER BY")
	}
}

func TestPrepareFlightSQLDataSQLUsesThresholdDecision(t *testing.T) {
	n := defaultMemoryUnionNode()

	smallRangeSQL := searchSQLWithRange(1_700_000_000_000, 1_700_003_000_000)
	gotSmall := n.prepareFlightSQLDataSQL(smallRangeSQL)
	if !strings.Contains(gotSmall, "mem_hep_proto_1_call_a") {
		t.Fatalf("expected mem union for <=1h in FlightSQL rewrite, got: %s", gotSmall)
	}

	largeRangeSQL := searchSQLWithRange(1_700_000_000_000, 1_700_007_500_000)
	gotLarge := n.prepareFlightSQLDataSQL(largeRangeSQL)
	if strings.Contains(gotLarge, "mem_hep_proto_1_call_a") || strings.Contains(gotLarge, "mem_hep_proto_1_call_b") {
		t.Fatalf("expected no mem union SQL for >1h in FlightSQL rewrite, got: %s", gotLarge)
	}
}
