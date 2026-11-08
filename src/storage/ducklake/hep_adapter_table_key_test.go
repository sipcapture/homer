package ducklake

import (
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/sipcapture/homer-core/src/decoder"
	"github.com/sipcapture/homer-core/src/sipparser"
)

func TestMixedSIPPacketsReuseDefaultWriter(t *testing.T) {
	db, err := sql.Open("duckdb", "")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec("CREATE SCHEMA transfer_test"); err != nil {
		t.Fatal(err)
	}
	writer := &MultiTableWriter{
		db:       db,
		lakeName: "transfer_test",
		config:   Config{BatchSize: 1000},
		tables:   make(map[TableKey]*TableWriter),
		schemas:  GetTableSchemas(),
	}
	key := TableKey{ProtoType: ProtoTypeSIP, SubType: SIPTypeDefault}
	defaultWriter, err := writer.getOrCreateTable(key)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		for _, table := range writer.tables {
			if err := table.Close(); err != nil {
				t.Error(err)
			}
		}
	}()
	adapter := NewMultiTableAdapter(writer)
	for batch := 0; batch < 10; batch++ {
		packets := []*decoder.HEP{
			{ProtoType: ProtoTypeSIP, Timestamp: time.Now(), NodeID: 2},
			{ProtoType: ProtoTypeSIP, Timestamp: time.Now(), NodeID: 1, Payload: fmt.Sprintf("notify-%d", batch), SIP: &sipparser.SipMsg{FirstMethod: "NOTIFY"}},
		}
		for _, packet := range packets {
			if err := adapter.WriteHEP(packet); err != nil {
				t.Fatal(err)
			}
		}
		if len(writer.tables) != 1 || writer.GetTable(key) != defaultWriter {
			t.Fatalf("mixed SIP packets created %d writers for the default table", len(writer.tables))
		}
		if err := defaultWriter.flushBatch(); err != nil {
			t.Fatal(err)
		}
		defaultWriter.flushSlotDirect(int(defaultWriter.activeIdx.Load()), &writer.catalogMu)
	}
	var total, notifications int
	if err := db.QueryRow("SELECT count(*), count(*) FILTER (WHERE method = 'NOTIFY') FROM transfer_test.hep_proto_1_default").Scan(&total, &notifications); err != nil {
		t.Fatal(err)
	}
	if total != 20 || notifications != 10 {
		t.Fatalf("persisted %d packets and %d notifications, want 20 and 10", total, notifications)
	}
}

func TestSIPWithoutParsedPayloadUsesCanonicalDefaultTable(t *testing.T) {
	adapter := &MultiTableAdapter{}
	packet := &decoder.HEP{ProtoType: ProtoTypeSIP}
	key, values := adapter.convertHEPToValues(packet)
	want := TableKey{ProtoType: ProtoTypeSIP, SubType: SIPTypeDefault}
	if key != want {
		t.Fatalf("table key = %#v, want %#v", key, want)
	}
	if err := ValidateTableKey(key); err != nil {
		t.Fatal(err)
	}
	if len(values) != len(GetTableSchemas()[key].Columns)+1 {
		t.Fatalf("row has %d values for %d columns", len(values), len(GetTableSchemas()[key].Columns)+1)
	}
}

func TestSIPTableKeysKeepTransferSignalsSeparate(t *testing.T) {
	adapter := &MultiTableAdapter{}
	cases := []struct {
		method     string
		response   string
		cseqMethod string
		want       string
	}{
		{method: "REFER", want: SIPTypeCall},
		{response: "202", cseqMethod: "REFER", want: SIPTypeCall},
		{method: "NOTIFY", want: SIPTypeDefault},
		{response: "200", cseqMethod: "NOTIFY", want: SIPTypeDefault},
		{method: "REGISTER", want: SIPTypeRegistration},
	}
	for _, tc := range cases {
		t.Run(tc.method+tc.response+tc.cseqMethod, func(t *testing.T) {
			packet := &decoder.HEP{
				ProtoType: ProtoTypeSIP,
				SIP:       &sipparser.SipMsg{FirstMethod: tc.method, FirstResp: tc.response, CseqMethod: tc.cseqMethod},
			}
			if key := adapter.getTableKey(packet); key.SubType != tc.want {
				t.Fatalf("table subtype = %q, want %q", key.SubType, tc.want)
			}
		})
	}
}
