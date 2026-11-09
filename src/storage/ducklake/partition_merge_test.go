package ducklake

import (
	"database/sql"
	"reflect"
	"testing"
)

func partitionMergeFixture(t *testing.T, coldPayload bool) (*TieredStorageManager, *Volume, *Volume) {
	t.Helper()
	db, err := sql.Open("duckdb", "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	db.SetMaxOpenConns(1)
	coldSchema := "uuid VARCHAR, date DATE"
	if coldPayload {
		coldSchema += ", payload VARCHAR"
	}
	for _, statement := range []string{
		"ATTACH ':memory:' AS hot",
		"ATTACH ':memory:' AS cold",
		"CREATE TABLE hot.main.hep_proto_1_call (uuid VARCHAR, date DATE, payload VARCHAR)",
		"CREATE TABLE cold.main.hep_proto_1_call (" + coldSchema + ")",
	} {
		if _, err := db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	hot := &Volume{Name: "hot", LakeName: "hot", Type: VolumeTypeLocal}
	cold := &Volume{Name: "cold", LakeName: "cold", Type: VolumeTypeLocal}
	return &TieredStorageManager{db: db, primaryVolume: hot, volumes: []*Volume{hot, cold}, catalogLocker: &fakeCatalogLocker{}}, hot, cold
}

func TestMovePartitionMergesPacketsByIdentity(t *testing.T) {
	cases := []struct {
		name                          string
		source, destination, expected []string
	}{
		{"empty destination", []string{"a", "b"}, nil, []string{"a", "b"}},
		{"late source subset", []string{"late"}, []string{"old-a", "old-b"}, []string{"late", "old-a", "old-b"}},
		{"overlapping partial copy", []string{"a", "b"}, []string{"a", "old"}, []string{"a", "b", "old"}},
		{"equal counts with different packets", []string{"a", "b"}, []string{"c", "d"}, []string{"a", "b", "c", "d"}},
		{"already copied packets", []string{"a", "b"}, []string{"a", "b"}, []string{"a", "b"}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			manager, hot, cold := partitionMergeFixture(t, true)
			for volume, packets := range map[string][]string{"hot": test.source, "cold": test.destination} {
				for _, packet := range packets {
					if _, err := manager.db.Exec("INSERT INTO "+volume+".main.hep_proto_1_call VALUES (?, DATE '2026-11-02', 'payload')", packet); err != nil {
						t.Fatal(err)
					}
				}
			}
			for attempt := 0; attempt < 2; attempt++ {
				if err := manager.MovePartition("hep_proto_1_call", "2026-11-02", hot, cold); err != nil {
					t.Fatal(err)
				}
			}
			rows, err := manager.db.Query("SELECT uuid FROM cold.main.hep_proto_1_call ORDER BY uuid")
			if err != nil {
				t.Fatal(err)
			}
			var packets []string
			for rows.Next() {
				var packet string
				if err := rows.Scan(&packet); err != nil {
					t.Fatal(err)
				}
				packets = append(packets, packet)
			}
			if err := rows.Err(); err != nil {
				t.Fatal(err)
			}
			rows.Close()
			if !reflect.DeepEqual(packets, test.expected) {
				t.Fatalf("packets=%v expected=%v", packets, test.expected)
			}
			var remaining int
			if err := manager.db.QueryRow("SELECT COUNT(*) FROM hot.main.hep_proto_1_call").Scan(&remaining); err != nil {
				t.Fatal(err)
			}
			if remaining != 0 {
				t.Fatalf("source still has %d copied packets", remaining)
			}
		})
	}
}

func TestMovePartitionPreservesBothVolumesOnSchemaMismatch(t *testing.T) {
	manager, hot, cold := partitionMergeFixture(t, false)
	for _, statement := range []string{
		"INSERT INTO hot.main.hep_proto_1_call VALUES ('late', DATE '2026-11-02', 'payload')",
		"INSERT INTO cold.main.hep_proto_1_call VALUES ('old-a', DATE '2026-11-02'), ('old-b', DATE '2026-11-02')",
	} {
		if _, err := manager.db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	if err := manager.MovePartition("hep_proto_1_call", "2026-11-02", hot, cold); err == nil {
		t.Fatal("expected schema mismatch")
	}
	for volume, expected := range map[string]int{"hot": 1, "cold": 2} {
		var count int
		if err := manager.db.QueryRow("SELECT COUNT(*) FROM " + volume + ".main.hep_proto_1_call").Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != expected {
			t.Fatalf("%s rows=%d expected=%d", volume, count, expected)
		}
	}
}

func TestSyncSIPColumnsMigratesExistingDestination(t *testing.T) {
	manager, hot, cold := partitionMergeFixture(t, true)
	for _, statement := range []string{
		"ALTER TABLE hot.main.hep_proto_1_call ADD COLUMN isup_body BLOB",
		"INSERT INTO hot.main.hep_proto_1_call VALUES ('new', DATE '2026-11-02', 'new', '\\x01'::BLOB)",
		"INSERT INTO cold.main.hep_proto_1_call VALUES ('old', DATE '2026-11-02', 'old')",
	} {
		if _, err := manager.db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	for attempt := 0; attempt < 2; attempt++ {
		if err := manager.syncSIPColumns(); err != nil {
			t.Fatal(err)
		}
	}
	if err := manager.MovePartition("hep_proto_1_call", "2026-11-02", hot, cold); err != nil {
		t.Fatal(err)
	}
	var rows, bodies int
	if err := manager.db.QueryRow("SELECT COUNT(*), COUNT(isup_body) FROM cold.main.hep_proto_1_call").Scan(&rows, &bodies); err != nil {
		t.Fatal(err)
	}
	if rows != 2 || bodies != 1 {
		t.Fatalf("rows=%d bodies=%d", rows, bodies)
	}
}

type latePacketLocker struct {
	db       *sql.DB
	inserted bool
	err      error
}

func (locker *latePacketLocker) CatalogLock() {}

func (locker *latePacketLocker) CatalogUnlock() {
	if locker.inserted {
		return
	}
	locker.inserted = true
	_, locker.err = locker.db.Exec("INSERT INTO hot.main.hep_proto_1_call VALUES ('after-copy', DATE '2026-11-02', 'late payload')")
}

func TestMovePartitionKeepsPacketsArrivingAfterCopy(t *testing.T) {
	manager, hot, cold := partitionMergeFixture(t, true)
	if _, err := manager.db.Exec("INSERT INTO hot.main.hep_proto_1_call VALUES ('initial', DATE '2026-11-02', 'payload')"); err != nil {
		t.Fatal(err)
	}
	locker := &latePacketLocker{db: manager.db}
	manager.catalogLocker = locker
	if err := manager.MovePartition("hep_proto_1_call", "2026-11-02", hot, cold); err != nil {
		t.Fatal(err)
	}
	if locker.err != nil {
		t.Fatal(locker.err)
	}
	var remaining string
	if err := manager.db.QueryRow("SELECT uuid FROM hot.main.hep_proto_1_call").Scan(&remaining); err != nil {
		t.Fatal(err)
	}
	if remaining != "after-copy" {
		t.Fatalf("unexpected remaining packet %q", remaining)
	}
	if err := manager.MovePartition("hep_proto_1_call", "2026-11-02", hot, cold); err != nil {
		t.Fatal(err)
	}
	var sourceRows, destinationRows int
	if err := manager.db.QueryRow("SELECT COUNT(*) FROM hot.main.hep_proto_1_call").Scan(&sourceRows); err != nil {
		t.Fatal(err)
	}
	if err := manager.db.QueryRow("SELECT COUNT(*) FROM cold.main.hep_proto_1_call").Scan(&destinationRows); err != nil {
		t.Fatal(err)
	}
	if sourceRows != 0 || destinationRows != 2 {
		t.Fatalf("source=%d destination=%d", sourceRows, destinationRows)
	}
}
