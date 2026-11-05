// Copyright (C) 2026 Homer Server Contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package ducklake

import (
	"bytes"
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"

	duckdb "github.com/duckdb/duckdb-go/v2"
)

func TestBuildParameterizedInsertAllowlist(t *testing.T) {
	key := TableKey{ProtoType: ProtoTypeSIP, SubType: SIPTypeCall}
	n := len(InsertColumnNamesForKey(key))
	row := make([]interface{}, n)
	for i := range row {
		row[i] = "x"
	}
	payload := "INVITE sip:bob@evil'; DROP TABLE t;-- SIP/2.0"
	row[16] = payload

	q, args, err := BuildParameterizedInsert("homer_lake", key, [][]interface{}{row})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(q, "INSERT INTO homer_lake.main.hep_proto_1_call") {
		t.Fatalf("unexpected query: %s", q)
	}
	if strings.Contains(q, payload) || strings.Contains(q, "DROP TABLE") {
		t.Fatalf("payload leaked into SQL: %s", q)
	}
	if !strings.Contains(q, "CAST(? AS JSON)") {
		t.Fatalf("expected JSON placeholder: %s", q)
	}
	found := false
	for _, a := range args {
		if s, ok := a.(string); ok && s == payload {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("payload missing from bound args: %#v", args)
	}
}

func TestBuildParameterizedInsertRejectsUnknownTable(t *testing.T) {
	_, _, err := BuildParameterizedInsert("homer_lake", TableKey{ProtoType: 99, SubType: "nope"}, [][]interface{}{{1}})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "unknown table") {
		t.Fatalf("got %v", err)
	}
}

func TestBuildParameterizedInsertJSONRoundTrip(t *testing.T) {
	key := TableKey{ProtoType: ProtoTypeSIP, SubType: SIPTypeCall}
	n := len(InsertColumnNamesForKey(key))
	row := make([]interface{}, n)
	for i := range row {
		row[i] = "x"
	}
	row[1] = time.Date(2020, 5, 1, 0, 0, 0, 0, time.UTC)
	row[2] = time.Date(2020, 5, 1, 12, 0, 0, 0, time.UTC)
	row[8] = uint32(5060)
	row[16] = "INVITE sip:bob@biloxi.example.com SIP/2.0"
	row[17] = json.RawMessage(`{"k":"v"}`)

	safe := JSONSafeInsertRows([][]interface{}{row})
	raw, err := json.Marshal(safe)
	if err != nil {
		t.Fatal(err)
	}
	var decoded [][]interface{}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	q, args, err := BuildParameterizedInsert("homer_lake", key, decoded)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(q, "INVITE sip:bob") {
		t.Fatalf("payload in SQL: %s", q)
	}
	if len(args) != n {
		t.Fatalf("args=%d cols=%d", len(args), n)
	}
}

func TestISUPBodySurvivesJSONInsert(t *testing.T) {
	key := TableKey{ProtoType: ProtoTypeSIP, SubType: SIPTypeCall}
	n := len(InsertColumnNamesForKey(key))
	row := make([]interface{}, n)
	for i := range row {
		row[i] = "x"
	}
	body := []byte{0x01, 0x00, 0xff, 0x60}
	row[n-1] = body

	safe := JSONSafeInsertRows([][]interface{}{row})
	raw, err := json.Marshal(safe)
	if err != nil {
		t.Fatal(err)
	}
	var decoded [][]interface{}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	got, ok := decoded[0][n-1].(string)
	if !ok || got != base64.StdEncoding.EncodeToString(body) {
		t.Fatalf("wire form %v", decoded[0][n-1])
	}
	_, args, err := BuildParameterizedInsert("homer_lake", key, decoded)
	if err != nil {
		t.Fatal(err)
	}
	bound, ok := args[len(args)-1].([]byte)
	if !ok || !bytes.Equal(bound, body) {
		t.Fatalf("bound %#v", args[len(args)-1])
	}

	sqlText, err := BuildInsertMultiValues("homer_lake", key, [][]interface{}{row})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sqlText, `\x01\x00\xff\x60'::BLOB`) {
		t.Fatalf("blob literal missing: %s", sqlText)
	}
}

func TestAppenderKeepsInvalidUTF8Blob(t *testing.T) {
	db, err := sql.Open("duckdb", "")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE t (payload VARCHAR, isup_body BLOB)`); err != nil {
		t.Fatal(err)
	}
	body := []byte{0x01, 0x00, 0xff}
	conn, err := db.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	err = conn.Raw(func(driverConn any) error {
		appender, err := duckdb.NewAppenderFromConn(driverConn.(driver.Conn), "", "t")
		if err != nil {
			return err
		}
		if err := appender.AppendRow("INVITE", body); err != nil {
			appender.Close()
			return err
		}
		if err := appender.AppendRow("OPTIONS", nil); err != nil {
			appender.Close()
			return err
		}
		return appender.Close()
	})
	if err != nil {
		t.Fatal(err)
	}
	var got []byte
	if err := db.QueryRow(`SELECT isup_body FROM t WHERE payload = 'INVITE'`).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, body) {
		t.Fatalf("blob %x", got)
	}
	var ordinary []byte
	if err := db.QueryRow(`SELECT isup_body FROM t WHERE payload = 'OPTIONS'`).Scan(&ordinary); err != nil {
		t.Fatal(err)
	}
	if ordinary != nil {
		t.Fatalf("ordinary sip stored %x", ordinary)
	}
}
