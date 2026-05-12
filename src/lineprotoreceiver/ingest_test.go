// Copyright (C) 2026 Homer Server Contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package lineprotoreceiver

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"

	_ "github.com/duckdb/duckdb-go/v2"
	"github.com/sipcapture/homer-core/src/config"
	"github.com/sipcapture/homer-core/src/storage/ducklake"
)

// newLPDB opens a fresh in-memory DuckDB and attaches a second in-memory
// database under the name lakeName so that both two-part
// ("<lake>.<table>") and three-part ("<lake>.<schema>.<table>")
// identifiers work the same way they do in production DuckLake
// deployments.
func newLPDB(t *testing.T) (*sql.DB, string) {
	t.Helper()
	db, err := sql.Open("duckdb", "")
	if err != nil {
		t.Fatalf("open duckdb: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	lake := "test_lake"
	if _, err := db.Exec(fmt.Sprintf("ATTACH ':memory:' AS %s", lake)); err != nil {
		t.Fatalf("attach catalog: %v", err)
	}
	return db, lake
}

func createTestHepCallTable(t *testing.T, db *sql.DB, lake string) {
	t.Helper()
	key := ducklake.TableKey{ProtoType: ducklake.ProtoTypeSIP, SubType: ducklake.SIPTypeCall}
	sc := ducklake.GetTableSchemas()[key]
	if sc == nil {
		t.Fatal("sip call schema missing")
	}
	fqn := fmt.Sprintf("%s.main.hep_proto_%s", lake, sc.TableSuffix)
	ddl := fmt.Sprintf("CREATE TABLE IF NOT EXISTS %s (%s);", fqn, strings.TrimSpace(sc.CreateSQL))
	if _, err := db.Exec(ddl); err != nil {
		t.Fatalf("create hep call table: %v", err)
	}
}

func lpTestCfg() *config.LineProtoConfig {
	return &config.LineProtoConfig{TablePrefix: "lp_"}
}

func TestIngester_InsertsRowsPerMeasurement(t *testing.T) {
	db, lake := newLPDB(t)
	ing := NewIngester(db, lake, lpTestCfg())

	body := []byte(`
cpu,host=srv1 usage=0.42,active=t 1700000000000000000
cpu,host=srv2 usage=0.77,active=f 1700000001000000000
mem,host=srv1 used=1024i 1700000002000000000
`)
	n, err := ing.Ingest(context.Background(), "", body, PrecisionNanoseconds, "")
	if err != nil {
		t.Fatalf("ingest error: %v", err)
	}
	if n != 3 {
		t.Fatalf("expected 3 rows written, got %d", n)
	}

	var cpuCount, memCount int
	if err := db.QueryRow("SELECT count(*) FROM test_lake.lp_cpu").Scan(&cpuCount); err != nil {
		t.Fatalf("query lp_cpu: %v", err)
	}
	if err := db.QueryRow("SELECT count(*) FROM test_lake.lp_mem").Scan(&memCount); err != nil {
		t.Fatalf("query lp_mem: %v", err)
	}
	if cpuCount != 2 || memCount != 1 {
		t.Errorf("row counts: cpu=%d (want 2), mem=%d (want 1)", cpuCount, memCount)
	}

	// Verify schema types were inferred correctly.
	var usage float64
	var active bool
	var host string
	if err := db.QueryRow("SELECT usage, active, host FROM test_lake.lp_cpu WHERE host='srv1'").Scan(&usage, &active, &host); err != nil {
		t.Fatalf("scan lp_cpu row: %v", err)
	}
	if usage != 0.42 || !active || host != "srv1" {
		t.Errorf("row values: usage=%v active=%v host=%q", usage, active, host)
	}
}

func TestIngester_PerDBSchema(t *testing.T) {
	db, lake := newLPDB(t)
	ing := NewIngester(db, lake, lpTestCfg())

	body := []byte(`
weather,location=us-midwest temperature=82
weather,location=us-east temperature=80
weather,location=us-west temperature=99
`)
	n, err := ing.Ingest(context.Background(), "mydb", body, PrecisionNanoseconds, "")
	if err != nil {
		t.Fatalf("ingest: %v", err)
	}
	if n != 3 {
		t.Fatalf("want 3 rows, got %d", n)
	}
	var got int
	if err := db.QueryRow("SELECT count(*) FROM test_lake.mydb.lp_weather").Scan(&got); err != nil {
		t.Fatalf("query: %v", err)
	}
	if got != 3 {
		t.Fatalf("rows in mydb.lp_weather: want 3, got %d", got)
	}
	var exists int
	if err := db.QueryRow("SELECT count(*) FROM duckdb_tables() WHERE schema_name='main' AND table_name='lp_weather'").Scan(&exists); err != nil {
		t.Fatalf("probe main: %v", err)
	}
	if exists != 0 {
		t.Fatalf("default schema should not have lp_weather when db=mydb is used")
	}
}

func TestIngester_IsolatesDBs(t *testing.T) {
	db, lake := newLPDB(t)
	ing := NewIngester(db, lake, lpTestCfg())

	if _, err := ing.Ingest(context.Background(), "a",
		[]byte("metric,x=1 v=1i"), PrecisionNanoseconds, ""); err != nil {
		t.Fatalf("db=a: %v", err)
	}
	if _, err := ing.Ingest(context.Background(), "b",
		[]byte("metric,x=2 v=2i"), PrecisionNanoseconds, ""); err != nil {
		t.Fatalf("db=b: %v", err)
	}
	var a, b int64
	if err := db.QueryRow("SELECT v FROM test_lake.a.lp_metric").Scan(&a); err != nil {
		t.Fatalf("read a: %v", err)
	}
	if err := db.QueryRow("SELECT v FROM test_lake.b.lp_metric").Scan(&b); err != nil {
		t.Fatalf("read b: %v", err)
	}
	if a != 1 || b != 2 {
		t.Fatalf("want a=1 b=2, got a=%d b=%d", a, b)
	}
}

func TestIngester_SanitizesDBName(t *testing.T) {
	db, lake := newLPDB(t)
	ing := NewIngester(db, lake, lpTestCfg())
	if _, err := ing.Ingest(context.Background(), "my-weird db",
		[]byte("m,x=1 v=1i"), PrecisionNanoseconds, ""); err != nil {
		t.Fatalf("ingest: %v", err)
	}
	var cnt int
	if err := db.QueryRow(`SELECT count(*) FROM test_lake.my_weird_db.lp_m`).Scan(&cnt); err != nil {
		t.Fatalf("query sanitised schema: %v", err)
	}
	if cnt != 1 {
		t.Fatalf("want 1 row, got %d", cnt)
	}
}

func TestIngester_AutoExtendsColumns(t *testing.T) {
	db, lake := newLPDB(t)
	ing := NewIngester(db, lake, lpTestCfg())

	n1, err := ing.Ingest(context.Background(), "", []byte("metric,region=a value=1i"), PrecisionNanoseconds, "")
	if err != nil {
		t.Fatalf("first ingest: %v", err)
	}
	if n1 != 1 {
		t.Fatalf("expected 1 row, got %d", n1)
	}
	n2, err := ing.Ingest(context.Background(), "",
		[]byte("metric,region=b,zone=east value=2i,extra=\"xyz\""), PrecisionNanoseconds, "")
	if err != nil {
		t.Fatalf("second ingest: %v", err)
	}
	if n2 != 1 {
		t.Fatalf("expected 1 row, got %d", n2)
	}
	var total int
	if err := db.QueryRow("SELECT count(*) FROM test_lake.lp_metric").Scan(&total); err != nil {
		t.Fatalf("count: %v", err)
	}
	if total != 2 {
		t.Errorf("row total: got %d, want 2", total)
	}
	var extra string
	if err := db.QueryRow("SELECT extra FROM test_lake.lp_metric WHERE region='b'").Scan(&extra); err != nil {
		t.Fatalf("read extra col: %v", err)
	}
	if extra != "xyz" {
		t.Errorf("extra: got %q, want xyz", extra)
	}
}

func TestIngester_ParseErrorIsReported(t *testing.T) {
	db, lake := newLPDB(t)
	ing := NewIngester(db, lake, lpTestCfg())
	_, err := ing.Ingest(context.Background(), "",
		[]byte("good v=1\nbroken_no_field"), PrecisionNanoseconds, "")
	if err == nil {
		t.Fatalf("expected parse error, got nil")
	}
}

func TestIngester_PrecisionMilliseconds(t *testing.T) {
	db, lake := newLPDB(t)
	ing := NewIngester(db, lake, lpTestCfg())
	n, err := ing.Ingest(context.Background(), "",
		[]byte("events v=1 1700000000000"), PrecisionMilliseconds, "")
	if err != nil {
		t.Fatalf("ingest: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected 1 row, got %d", n)
	}
	var ts string
	if err := db.QueryRow("SELECT strftime(ts, '%Y-%m-%d %H:%M:%S') FROM test_lake.lp_events").Scan(&ts); err != nil {
		t.Fatalf("read ts: %v", err)
	}
	if ts != "2023-11-14 22:13:20" {
		t.Errorf("ts: got %q, want 2023-11-14 22:13:20", ts)
	}
}

func TestIngester_HepSipCallLP(t *testing.T) {
	db, lake := newLPDB(t)
	createTestHepCallTable(t, db, lake)
	cfg := &config.LineProtoConfig{TablePrefix: "lp_", AllowHepSipCall: true}
	ing := NewIngester(db, lake, cfg)

	body := []byte(`sip,method=INVITE,session_id=call-1 caller="alice",callee="bob",src_ip="10.0.0.1",dst_ip="10.0.0.2",src_port=5060i,dst_port=5060i,protocol=17i,payload="INVITE sip:b SIP/2.0" 1700000000000000000`)
	n, err := ing.Ingest(context.Background(), "ignored_db", body, PrecisionNanoseconds, "call")
	if err != nil {
		t.Fatalf("ingest: %v", err)
	}
	if n != 1 {
		t.Fatalf("rows: want 1, got %d", n)
	}
	var method, session string
	if err := db.QueryRow("SELECT method, session_id FROM test_lake.main.hep_proto_1_call LIMIT 1").Scan(&method, &session); err != nil {
		t.Fatalf("select: %v", err)
	}
	if method != "INVITE" || session != "call-1" {
		t.Errorf("method=%q session_id=%q", method, session)
	}
}

func TestIngester_HepSipCallLP_Disabled(t *testing.T) {
	db, lake := newLPDB(t)
	createTestHepCallTable(t, db, lake)
	ing := NewIngester(db, lake, lpTestCfg())
	_, err := ing.Ingest(context.Background(), "", []byte(`m a=1 1`), PrecisionNanoseconds, "call")
	if err == nil {
		t.Fatal("expected error when hep sip call disabled")
	}
}

func TestIngester_HepSipCallLP_InvalidDataExtra(t *testing.T) {
	db, lake := newLPDB(t)
	createTestHepCallTable(t, db, lake)
	cfg := &config.LineProtoConfig{AllowHepSipCall: true}
	ing := NewIngester(db, lake, cfg)
	_, err := ing.Ingest(context.Background(), "", []byte(`sip,data_extra="not-json" x=1i 1700000000000000000`), PrecisionNanoseconds, "call")
	if err == nil {
		t.Fatal("expected error for invalid data_extra JSON")
	}
}

func TestColType(t *testing.T) {
	cases := []struct {
		col  string
		val  interface{}
		want string
	}{
		{"ts", nil, "TIMESTAMP"},
		{"event_ts", "x", "TIMESTAMP"},
		{"created_at", 0.0, "TIMESTAMP"},
		{"flag", true, "BOOLEAN"},
		{"count", int64(1), "BIGINT"},
		{"ratio", 3.14, "DOUBLE"},
		{"name", "abc", "VARCHAR"},
	}
	for _, c := range cases {
		if got := colType(c.col, c.val); got != c.want {
			t.Errorf("colType(%q, %T) = %s, want %s", c.col, c.val, got, c.want)
		}
	}
}

// Compile-time reference so the resetForTests helper stays exported to
// future test files (golint flags otherwise-unused private helpers in
// _test.go packages).
var _ = (*Ingester).resetForTests