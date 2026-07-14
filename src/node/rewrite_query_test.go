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

func multiVolumeNode() *Node {
	cfg := &config.NodeConfig{DuckLake: config.DuckLakeConfig{LakeName: "homer_lake"}}
	return &Node{
		config: cfg,
		volumes: []VolumeInfo{
			{Name: "hot", LakeName: "homer_lake_hot"},
			{Name: "cold", LakeName: "homer_lake_cold"},
		},
		sharedDB: new(sql.DB),
	}
}

func TestTryBuildVolumeUnionSQL_multiVolume(t *testing.T) {
	n := multiVolumeNode()
	sqlIn := "SELECT * FROM homer_lake.main.hep_proto_1_call WHERE 1=1"
	out, ok := n.tryBuildVolumeUnionSQL(sqlIn)
	if !ok {
		t.Fatal("expected ok")
	}
	if !strings.Contains(out, "homer_lake_hot.main.hep_proto_1_call") ||
		!strings.Contains(out, "UNION ALL") ||
		!strings.Contains(out, "homer_lake_cold.main.hep_proto_1_call") {
		t.Fatalf("expected union across hot+cold: %s", out)
	}
	// The derived table must be aliased back to the original table name so
	// column references keep resolving.
	if !strings.Contains(out, ") AS hep_proto_1_call") {
		t.Fatalf("expected derived table aliased to table name: %s", out)
	}
	// Volume labels are exposed as columns for the UI.
	if !strings.Contains(out, "'homer_lake_cold' AS storage_lake") || !strings.Contains(out, "'cold' AS storage_volume") {
		t.Fatalf("expected storage_* labels in union branches: %s", out)
	}
	// The rewritten query must pass the node's own read-only SQL gates —
	// the previous "(SELECT ...) UNION ALL (SELECT ...)" shape was rejected
	// by validateUserSQL and never reached the cold catalog (issue #868).
	if err := validateUserSQL(out); err != nil {
		t.Fatalf("rewritten SQL rejected by validateUserSQL: %v\nsql: %s", err, out)
	}
	if err := ensureReadOnlySingleStatement(out); err != nil {
		t.Fatalf("rewritten SQL rejected by ensureReadOnlySingleStatement: %v\nsql: %s", err, out)
	}
	if !shouldUseSQLQuery(out) {
		t.Fatalf("rewritten SQL must be recognized as a query: %s", out)
	}
}

func TestTryBuildVolumeUnionSQL_skipsStorageColsWhenPresent(t *testing.T) {
	n := multiVolumeNode()
	sqlIn := "SELECT *, 'homer_lake' AS storage_lake, 'default' AS storage_volume FROM homer_lake.main.hep_proto_1_call WHERE 1=1 ORDER BY timestamp DESC LIMIT 50"
	out, ok := n.tryBuildVolumeUnionSQL(sqlIn)
	if !ok {
		t.Fatal("expected ok")
	}
	if strings.Contains(out, "'hot' AS storage_volume") {
		t.Fatalf("expected no duplicate storage_* columns in branches: %s", out)
	}
	if !strings.Contains(out, "UNION ALL SELECT * FROM homer_lake_cold.main.hep_proto_1_call) AS hep_proto_1_call") {
		t.Fatalf("expected plain branches with alias: %s", out)
	}
}

func TestTryBuildVolumeUnionSQL_cteQuery(t *testing.T) {
	n := multiVolumeNode()
	sqlIn := "WITH x AS ( SELECT * FROM homer_lake.main.hep_proto_1_default WHERE date = DATE '2026-07-12' ) SELECT count(*) FROM x"
	out, ok := n.tryBuildVolumeUnionSQL(sqlIn)
	if !ok {
		t.Fatal("expected ok for CTE query")
	}
	if !strings.HasPrefix(strings.ToUpper(strings.TrimSpace(out)), "WITH") {
		t.Fatalf("expected rewritten CTE to keep WITH prefix: %s", out)
	}
	if !strings.Contains(out, "homer_lake_hot.main.hep_proto_1_default") ||
		!strings.Contains(out, "homer_lake_cold.main.hep_proto_1_default") {
		t.Fatalf("expected hot+cold branches inside CTE: %s", out)
	}
	if err := validateUserSQL(out); err != nil {
		t.Fatalf("rewritten CTE rejected by validateUserSQL: %v\nsql: %s", err, out)
	}
}

func TestTryBuildVolumeUnionSQL_aggregateQuery(t *testing.T) {
	n := multiVolumeNode()
	sqlIn := "SELECT count(*) FROM homer_lake.main.hep_proto_1_default WHERE date = DATE '2026-07-12'"
	out, ok := n.tryBuildVolumeUnionSQL(sqlIn)
	if !ok {
		t.Fatal("expected ok for aggregate query")
	}
	// The aggregate must run once over the combined rows: exactly one
	// count(*) in the outer query, table replaced by the derived union.
	if strings.Count(out, "count(*)") != 1 {
		t.Fatalf("expected single outer aggregate: %s", out)
	}
	if !strings.Contains(out, "FROM (SELECT *") || !strings.Contains(out, ") AS hep_proto_1_default WHERE date") {
		t.Fatalf("expected derived-table substitution before WHERE: %s", out)
	}
}

func TestTryBuildVolumeUnionSQL_preservesExplicitAlias(t *testing.T) {
	n := multiVolumeNode()
	sqlIn := "SELECT t.sid FROM homer_lake.main.hep_proto_1_call t WHERE 1=1"
	out, ok := n.tryBuildVolumeUnionSQL(sqlIn)
	if !ok {
		t.Fatal("expected ok")
	}
	if strings.Contains(out, "AS hep_proto_1_call") {
		t.Fatalf("expected original alias to be kept, no injected alias: %s", out)
	}
	if !strings.Contains(out, ") t WHERE 1=1") {
		t.Fatalf("expected derived table followed by original alias: %s", out)
	}
}

// TestTryBuildVolumeUnionSQL_executesOnDuckDB verifies the rewritten SQL
// actually parses and runs on DuckDB across two attached catalogs, covering
// the issue #868 shapes: plain SELECT, CTE and aggregate over hot+cold.
func TestTryBuildVolumeUnionSQL_executesOnDuckDB(t *testing.T) {
	db, err := sql.Open("duckdb", "")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)

	setup := []string{
		"ATTACH ':memory:' AS homer_lake_hot",
		"ATTACH ':memory:' AS homer_lake_cold",
		"CREATE TABLE homer_lake_hot.main.hep_proto_1_default (uuid VARCHAR, date DATE, timestamp TIMESTAMP, sid VARCHAR)",
		"CREATE TABLE homer_lake_cold.main.hep_proto_1_default (uuid VARCHAR, date DATE, timestamp TIMESTAMP, sid VARCHAR)",
		"INSERT INTO homer_lake_hot.main.hep_proto_1_default VALUES ('u1', DATE '2026-07-14', TIMESTAMP '2026-07-14 10:00:00', 'a')",
		"INSERT INTO homer_lake_cold.main.hep_proto_1_default VALUES ('u2', DATE '2026-07-12', TIMESTAMP '2026-07-12 10:00:00', 'b'), ('u3', DATE '2026-07-12', TIMESTAMP '2026-07-12 11:00:00', 'c')",
	}
	for _, stmt := range setup {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("setup %q: %v", stmt, err)
		}
	}

	n := multiVolumeNode()
	n.tieredQueryDB = db

	queryOne := func(t *testing.T, query string) int {
		t.Helper()
		var got int
		if err := db.QueryRow(query).Scan(&got); err != nil {
			t.Fatalf("query failed: %v\nsql: %s", err, query)
		}
		return got
	}

	t.Run("aggregate over cold date", func(t *testing.T) {
		out, ok := n.tryBuildVolumeUnionSQL("SELECT count(*) FROM homer_lake.main.hep_proto_1_default WHERE date = DATE '2026-07-12'")
		if !ok {
			t.Fatal("expected rewrite")
		}
		if got := queryOne(t, out); got != 2 {
			t.Fatalf("expected 2 cold rows, got %d\nsql: %s", got, out)
		}
	})

	t.Run("CTE over cold timestamp range", func(t *testing.T) {
		out, ok := n.tryBuildVolumeUnionSQL("WITH x AS ( SELECT * FROM homer_lake.main.hep_proto_1_default WHERE timestamp >= TIMESTAMP '2026-07-12 00:00:00' AND timestamp < TIMESTAMP '2026-07-13 00:00:00' ) SELECT count(*) FROM x")
		if !ok {
			t.Fatal("expected rewrite")
		}
		if got := queryOne(t, out); got != 2 {
			t.Fatalf("expected 2 cold rows via CTE, got %d\nsql: %s", got, out)
		}
	})

	t.Run("plain select spans hot and cold", func(t *testing.T) {
		out, ok := n.tryBuildVolumeUnionSQL("SELECT * FROM homer_lake.main.hep_proto_1_default WHERE sid IS NOT NULL ORDER BY timestamp DESC LIMIT 50")
		if !ok {
			t.Fatal("expected rewrite")
		}
		rows, err := db.Query(out)
		if err != nil {
			t.Fatalf("query failed: %v\nsql: %s", err, out)
		}
		defer rows.Close()
		data, cols, err := scanAllSQLRows(rows)
		if err != nil {
			t.Fatal(err)
		}
		if len(data) != 3 {
			t.Fatalf("expected 3 rows across hot+cold, got %d\nsql: %s", len(data), out)
		}
		hasCol := func(name string) bool {
			for _, c := range cols {
				if c == name {
					return true
				}
			}
			return false
		}
		if !hasCol("storage_lake") || !hasCol("storage_volume") {
			t.Fatalf("expected storage_* columns in result, got %v", cols)
		}
	})

	// Call Search flow: phase 1 finds uuids by callid (narrow projection),
	// phase 2 hydrates full rows by uuid IN (...). Both must survive the
	// derived-table rewrite and reach cold rows.
	t.Run("callid search then hydrate by uuid", func(t *testing.T) {
		narrow, ok := n.tryBuildVolumeUnionSQL("SELECT uuid, timestamp FROM homer_lake.main.hep_proto_1_default WHERE (sid = 'b' OR sid = 'c') ORDER BY timestamp DESC LIMIT 200")
		if !ok {
			t.Fatal("expected rewrite for narrow phase")
		}
		rows, err := db.Query(narrow)
		if err != nil {
			t.Fatalf("narrow query failed: %v\nsql: %s", err, narrow)
		}
		data, cols, err := scanAllSQLRows(rows)
		rows.Close()
		if err != nil {
			t.Fatal(err)
		}
		if len(data) != 2 {
			t.Fatalf("expected 2 narrow rows from cold, got %d\nsql: %s", len(data), narrow)
		}
		if len(cols) != 2 || cols[0] != "uuid" || cols[1] != "timestamp" {
			t.Fatalf("narrow projection must stay uuid+timestamp, got %v", cols)
		}

		hydrate, ok := n.tryBuildVolumeUnionSQL("SELECT * FROM homer_lake.main.hep_proto_1_default WHERE uuid IN ('u2','u3') AND timestamp >= TIMESTAMP '2026-07-12 00:00:00'")
		if !ok {
			t.Fatal("expected rewrite for hydration phase")
		}
		rows, err = db.Query(hydrate)
		if err != nil {
			t.Fatalf("hydration query failed: %v\nsql: %s", err, hydrate)
		}
		data, _, err = scanAllSQLRows(rows)
		rows.Close()
		if err != nil {
			t.Fatal(err)
		}
		if len(data) != 2 {
			t.Fatalf("expected 2 hydrated rows from cold, got %d\nsql: %s", len(data), hydrate)
		}
	})

	t.Run("missing table on cold is skipped", func(t *testing.T) {
		if _, err := db.Exec("CREATE TABLE homer_lake_hot.main.hep_proto_1_call (uuid VARCHAR, date DATE, timestamp TIMESTAMP, sid VARCHAR)"); err != nil {
			t.Fatal(err)
		}
		out, ok := n.tryBuildVolumeUnionSQL("SELECT count(*) FROM homer_lake.main.hep_proto_1_call WHERE 1=1")
		if !ok {
			t.Fatal("expected rewrite with hot-only branch")
		}
		if strings.Contains(out, "homer_lake_cold") {
			t.Fatalf("expected cold volume skipped for missing table: %s", out)
		}
		if got := queryOne(t, out); got != 0 {
			t.Fatalf("expected 0 rows, got %d", got)
		}
	})
}

func TestSortMergedRowsForQueryOrder(t *testing.T) {
	ts := func(s string) map[string]interface{} {
		return map[string]interface{}{"timestamp": []byte(s)}
	}
	rows := []map[string]interface{}{
		ts("2026-07-12 12:00:00"),
		ts("2026-07-12 10:00:00"),
		ts("2026-07-12 11:00:00"),
	}

	descSQL := "SELECT * FROM t ORDER BY timestamp DESC LIMIT 50"
	got := sortMergedRowsForQueryOrder(rows, descSQL, 50)
	if len(got) != 3 || string(got[0]["timestamp"].([]byte)) != "2026-07-12 12:00:00" {
		t.Fatalf("DESC query must be left untouched: %v", got)
	}

	ascSQL := "SELECT * FROM t WHERE sid = 'x' ORDER BY timestamp ASC"
	got = sortMergedRowsForQueryOrder(rows, ascSQL, 0)
	if string(got[0]["timestamp"].([]byte)) != "2026-07-12 10:00:00" ||
		string(got[2]["timestamp"].([]byte)) != "2026-07-12 12:00:00" {
		t.Fatalf("expected ascending order, got: %v", got)
	}

	got = sortMergedRowsForQueryOrder(rows, ascSQL, 2)
	if len(got) != 2 || string(got[1]["timestamp"].([]byte)) != "2026-07-12 11:00:00" {
		t.Fatalf("expected oldest 2 rows for ASC LIMIT, got: %v", got)
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

func TestBuildMemoryUnionQueries_aggregateSingleRow(t *testing.T) {
	n := defaultMemoryUnionNode()
	sqlIn := "SELECT count(*) FROM homer_lake.main.hep_proto_1_default WHERE date = DATE '2026-07-12'"
	q := n.buildMemoryUnionQueries(sqlIn)
	if !q.ok {
		t.Fatal("expected memory union query plan")
	}
	// Unioning whole aggregate queries returned one count row per branch
	// (issue #868 showed rows=3); the derived-table substitution must keep a
	// single outer aggregate over lake + both memory buffers.
	if strings.Count(q.combinedSQL, "count(*)") != 1 {
		t.Fatalf("expected single outer aggregate, got: %s", q.combinedSQL)
	}
	if !strings.Contains(q.combinedSQL, "mem_hep_proto_1_default_a") ||
		!strings.Contains(q.combinedSQL, "mem_hep_proto_1_default_b") {
		t.Fatalf("expected both memory buffers in derived union: %s", q.combinedSQL)
	}
	if !strings.Contains(q.combinedSQL, ") AS hep_proto_1_default WHERE date") {
		t.Fatalf("expected derived table aliased before WHERE: %s", q.combinedSQL)
	}
	if q.memSQL != "" {
		t.Fatalf("derived rewrite must not produce a separate mem query: %s", q.memSQL)
	}
}

func TestBuildMemoryUnionQueries_cteQuery(t *testing.T) {
	n := defaultMemoryUnionNode()
	sqlIn := "WITH x AS ( SELECT * FROM homer_lake.main.hep_proto_1_default WHERE date = DATE '2026-07-12' ) SELECT count(*) FROM x"
	q := n.buildMemoryUnionQueries(sqlIn)
	if !q.ok {
		t.Fatal("expected memory union query plan for CTE")
	}
	if !strings.HasPrefix(strings.ToUpper(strings.TrimSpace(q.combinedSQL)), "WITH") {
		t.Fatalf("expected combined SQL to keep WITH prefix: %s", q.combinedSQL)
	}
	if !strings.Contains(q.combinedSQL, "mem_hep_proto_1_default_a") {
		t.Fatalf("expected memory buffer inside CTE: %s", q.combinedSQL)
	}
	if err := validateUserSQL(q.combinedSQL); err != nil {
		t.Fatalf("combined CTE SQL rejected by validateUserSQL: %v\nsql: %s", err, q.combinedSQL)
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
