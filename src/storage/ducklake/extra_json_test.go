// Copyright (C) 2026 Homer Server Contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package ducklake

import (
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"strings"
	"testing"

	duckdb "github.com/duckdb/duckdb-go/v2"
	"github.com/sipcapture/homer-core/src/decoder"
)

func TestBuildExtraJSONCell_PooledBuffer(t *testing.T) {
	pkt := decoder.BenchHEP3SIPPacket()
	hep, err := decoder.DecodeHEP(pkt)
	if err != nil {
		t.Fatal(err)
	}
	defer decoder.ReleaseHEP(hep)

	cell := buildExtraJSONCell(hep)
	bp, ok := cell.(*[]byte)
	if !ok {
		t.Fatalf("expected *[]byte, got %T", cell)
	}
	if len(*bp) == 0 || (*bp)[0] != '{' {
		t.Fatalf("unexpected json: %q", *bp)
	}
	releaseExtraJSONCell(cell)
}

// The Appender json.Marshal()s values for JSON columns, so a plain Go string
// would be stored as a double-encoded JSON string scalar. data_extra cells
// must reach the Appender as json.RawMessage (issue #790).
func TestCellToDriverValue_pooledJSONIsRawMessage(t *testing.T) {
	b := []byte(`{"version":3}`)
	cell := any(&b)
	dv := cellToDriverValue(cell)
	m, ok := dv.(json.RawMessage)
	if !ok {
		t.Fatalf("expected json.RawMessage, got %T", dv)
	}
	if string(m) != string(b) {
		t.Fatalf("got %q", m)
	}
}

func TestCellToDriverValue_emptyPooledJSONIsRawObject(t *testing.T) {
	var b []byte
	dv := cellToDriverValue(any(&b))
	m, ok := dv.(json.RawMessage)
	if !ok {
		t.Fatalf("expected json.RawMessage, got %T", dv)
	}
	if string(m) != "{}" {
		t.Fatalf("got %q", m)
	}
}

func TestCachedSIPVersionOnlyJSON(t *testing.T) {
	a := cachedSIPVersionOnlyJSON(7)
	b := cachedSIPVersionOnlyJSON(7)
	if &a[0] != &b[0] {
		t.Fatalf("cache miss: %q vs %q", a, b)
	}
	if string(a) != `{"version":7}` {
		t.Fatalf("got %q", a)
	}
}

// TestAppenderJSONColumnNotDoubleEncoded is the end-to-end regression test
// for issue #790: data_extra written through the Appender must land as a
// JSON object (json_type='OBJECT'), not a double-encoded string scalar
// (json_type='VARCHAR'). With a VARCHAR scalar json_extract_string(...,
// '$.x_call_id') returns NULL and Lua call correlation silently finds
// nothing.
func TestAppenderJSONColumnNotDoubleEncoded(t *testing.T) {
	db, err := sql.Open("duckdb", "")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE t (session_id VARCHAR, data_extra JSON)`); err != nil {
		t.Fatal(err)
	}

	pkt := decoder.BenchHEP3SIPPacket()
	hep, err := decoder.DecodeHEP(pkt)
	if err != nil {
		t.Fatal(err)
	}
	defer decoder.ReleaseHEP(hep)
	hep.SIP.XCallID = "aleg-call-id-1"

	cells := map[string]interface{}{
		"pooled-sip-extra": buildExtraJSONCell(hep),
		"cached-simple":    buildSimpleExtraJSONCell(hep),
		"version-only":     cachedSIPVersionOnlyJSON(2),
	}

	conn, err := db.Conn(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	err = conn.Raw(func(driverConn interface{}) error {
		appender, aerr := duckdb.NewAppenderFromConn(driverConn.(driver.Conn), "", "t")
		if aerr != nil {
			return aerr
		}
		for name, cell := range cells {
			if aerr = appender.AppendRow(name, cellToDriverValue(cell)); aerr != nil {
				appender.Close()
				return aerr
			}
		}
		return appender.Close()
	})
	if err != nil {
		t.Fatal(err)
	}
	releaseExtraJSONCell(cells["pooled-sip-extra"])

	rows, err := db.Query(`SELECT session_id, json_type(data_extra) FROM t`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var name, jt string
		if err := rows.Scan(&name, &jt); err != nil {
			t.Fatal(err)
		}
		if jt != "OBJECT" {
			t.Fatalf("%s: json_type = %q, want OBJECT (double-encoded data_extra)", name, jt)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}

	var xcid sql.NullString
	err = db.QueryRow(`SELECT json_extract_string(data_extra, '$.x_call_id')
		FROM t WHERE session_id = 'pooled-sip-extra'`).Scan(&xcid)
	if err != nil {
		t.Fatal(err)
	}
	if !xcid.Valid || xcid.String != "aleg-call-id-1" {
		t.Fatalf("x_call_id = %#v, want aleg-call-id-1", xcid)
	}
}

func TestEffectiveNodeName(t *testing.T) {
	if got := effectiveNodeName(nil); got != "" {
		t.Fatalf("nil hep: got %q", got)
	}
	hep := &decoder.HEP{NodeID: 2002, NodeName: "2002"}
	if got := effectiveNodeName(hep); got != "" {
		t.Fatalf("id fallback name should be skipped, got %q", got)
	}
	hep.NodeName = "voice"
	if got := effectiveNodeName(hep); got != "voice" {
		t.Fatalf("got %q, want voice", got)
	}
	hep.NodeName = "  "
	if got := effectiveNodeName(hep); got != "" {
		t.Fatalf("whitespace: got %q", got)
	}
}

func TestBuildExtraJSONCell_NodeNameInDataExtra(t *testing.T) {
	pkt := decoder.BenchHEP3SIPPacket()
	hep, err := decoder.DecodeHEP(pkt)
	if err != nil {
		t.Fatal(err)
	}
	defer decoder.ReleaseHEP(hep)

	hep.NodeID = 2002
	hep.NodeName = "2002"
	cell := buildExtraJSONCell(hep)
	raw := cellJSONBytes(t, cell)
	releaseExtraJSONCell(cell)
	if strings.Contains(string(raw), `"node_name"`) {
		t.Fatalf("fallback NodeName must not be written: %s", raw)
	}

	hep.NodeName = "voice"
	cell = buildExtraJSONCell(hep)
	raw = cellJSONBytes(t, cell)
	releaseExtraJSONCell(cell)
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		t.Fatal(err)
	}
	if obj["node_name"] != "voice" {
		t.Fatalf("node_name=%v, want voice; json=%s", obj["node_name"], raw)
	}
}

func TestBuildSimpleExtraJSONCell_NodeName(t *testing.T) {
	hep := &decoder.HEP{Version: 3, ProtoType: 5, NodeID: 2002, NodeName: "2002"}
	raw := buildSimpleExtraJSONCell(hep)
	if strings.Contains(string(raw), `"node_name"`) {
		t.Fatalf("unexpected node_name: %s", raw)
	}

	hep.NodeName = "voice"
	raw = buildSimpleExtraJSONCell(hep)
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		t.Fatal(err)
	}
	if obj["node_name"] != "voice" {
		t.Fatalf("node_name=%v, want voice; json=%s", obj["node_name"], raw)
	}
}

func cellJSONBytes(t *testing.T, cell interface{}) []byte {
	t.Helper()
	switch v := cell.(type) {
	case json.RawMessage:
		return []byte(v)
	case *[]byte:
		return append([]byte(nil), (*v)...)
	default:
		t.Fatalf("unexpected cell type %T", cell)
		return nil
	}
}
