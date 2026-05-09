package handlers

import (
	"strings"
	"testing"

	"github.com/sipcapture/homer-core/src/coordinator/services"
)

func TestBuildSearchSQLV4_CaptureID(t *testing.T) {
	req := SearchObjectV4{}
	req.Filter.ProtoType = 1
	req.Filter.EventType = "call"
	req.Filter.CaptureID = 42

	sql, err := buildSearchSQLV4("homer_lake", &req, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sql, "node_id") || !strings.Contains(sql, "42") {
		t.Fatalf("expected node_id and capture value in SQL, got:\n%s", sql)
	}
	if !strings.Contains(sql, "json_extract") {
		t.Fatalf("expected json_extract for data_extra capture_id, got:\n%s", sql)
	}
}

func TestBuildSearchSQLV4_SIPDefaultUsesDataExtraHosts(t *testing.T) {
	req := SearchObjectV4{}
	req.Filter.ProtoType = 1
	req.Filter.EventType = "default"
	req.Filter.FromUser = "alice.net"

	sql, err := buildSearchSQLV4("homer_lake", &req, nil)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(sql, "caller LIKE") {
		t.Fatalf("default profile must not use caller column, got:\n%s", sql)
	}
	if !strings.Contains(sql, "from_host") {
		t.Fatalf("expected from_host in data_extra extract, got:\n%s", sql)
	}
}

func TestBuildSearchSQLV4_SIPRegistrationUsesAOR(t *testing.T) {
	req := SearchObjectV4{}
	req.Filter.ProtoType = 1
	req.Filter.EventType = "registration"
	req.Filter.Aor = "bob@"

	sql, err := buildSearchSQLV4("homer_lake", &req, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sql, "aor LIKE") {
		t.Fatalf("expected aor LIKE, got:\n%s", sql)
	}
}

func TestBuildSearchSQLV4_UserAgentRegistrationColumn(t *testing.T) {
	req := SearchObjectV4{}
	req.Filter.ProtoType = 1
	req.Filter.EventType = "registration"
	req.Filter.UserAgent = "Polycom"

	sql, err := buildSearchSQLV4("homer_lake", &req, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sql, "user_agent LIKE") {
		t.Fatalf("expected top-level user_agent filter for registration, got:\n%s", sql)
	}
}

func TestBuildSearchSQLV4_SIPMethodAndResponseMultiFilter(t *testing.T) {
	req := SearchObjectV4{}
	req.Filter.ProtoType = 1
	req.Filter.EventType = "call"
	req.Filter.Method = "INVITE, BYE"
	req.Filter.Methods = []string{"OPTIONS"}
	req.Filter.ResponseCode = "200"
	req.Filter.ResponseCodes = []string{"486", "603"}

	sql, err := buildSearchSQLV4("homer_lake", &req, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sql, "method IN (") || !strings.Contains(sql, "'INVITE'") || !strings.Contains(sql, "'BYE'") || !strings.Contains(sql, "'OPTIONS'") {
		t.Fatalf("expected merged method IN with INVITE, BYE, OPTIONS, got:\n%s", sql)
	}
	if !strings.Contains(sql, "response_code IN (") || !strings.Contains(sql, "'200'") || !strings.Contains(sql, "'486'") || !strings.Contains(sql, "'603'") {
		t.Fatalf("expected merged response_code IN with 200, 486, 603, got:\n%s", sql)
	}
}

func TestBuildSearchSQLV4_SIPMethodDedupe(t *testing.T) {
	req := SearchObjectV4{}
	req.Filter.ProtoType = 1
	req.Filter.EventType = "call"
	req.Filter.Method = "INVITE"
	req.Filter.Methods = []string{"INVITE", "ACK"}

	sql, err := buildSearchSQLV4("homer_lake", &req, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sql, "method IN ('INVITE','ACK')") {
		t.Fatalf("expected deduped methods, got:\n%s", sql)
	}
}

func TestGetTableName_LPVirtualMapping(t *testing.T) {
	cases := []struct {
		name     string
		profile  string
		expected string
	}{
		{"main schema", "main__lp_cpu", "homer_lake.main.lp_cpu"},
		{"app schema", "apps__lp_http_requests", "homer_lake.apps.lp_http_requests"},
		{"missing separator falls back to main", "lp_legacy", "homer_lake.main.lp_legacy"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := getTableName("homer_lake", lpHepID, tc.profile)
			if got != tc.expected {
				t.Fatalf("getTableName(LP, %q) = %q, want %q", tc.profile, got, tc.expected)
			}
			if strings.Contains(got, "hep_proto_") {
				t.Fatalf("LP table must not use hep_proto_ namespace, got: %s", got)
			}
		})
	}
}

func TestBuildSearchSQLV4_LPRoutesToTimeColumn(t *testing.T) {
	req := SearchObjectV4{}
	req.Filter.ProtoType = lpHepID
	req.Filter.EventType = "main__lp_cpu"
	req.Timestamp.From = 1714400000000
	req.Timestamp.To = 1714403600000
	req.Param.Limit = 100

	sql, err := buildSearchSQLV4("homer_lake", &req, nil)
	if err != nil {
		t.Fatalf("buildSearchSQLV4 LP: %v", err)
	}
	// LP tables expose `time`, not `timestamp` — make sure the
	// builder emits clauses against the right column.
	if !strings.Contains(sql, "FROM homer_lake.main.lp_cpu") {
		t.Fatalf("LP search SQL missing fully-qualified table:\n%s", sql)
	}
	if !strings.Contains(sql, "time >= (to_timestamp(") || !strings.Contains(sql, "time <= (to_timestamp(") {
		t.Fatalf("LP search SQL must filter on time column, got:\n%s", sql)
	}
	if strings.Contains(sql, "timestamp >=") || strings.Contains(sql, "timestamp <=") {
		t.Fatalf("LP search SQL must not reference timestamp column, got:\n%s", sql)
	}
	if !strings.Contains(sql, "ORDER BY time DESC") {
		t.Fatalf("LP search SQL should order by time DESC, got:\n%s", sql)
	}
	if !strings.Contains(sql, "LIMIT 100") {
		t.Fatalf("LP search SQL missing LIMIT, got:\n%s", sql)
	}
	// Critical: none of the SIP-specific WHERE clauses must leak in.
	for _, banned := range []string{"caller", "callee", "method", "response_code", "src_port", "dst_port", "node_id"} {
		if strings.Contains(sql, banned) {
			t.Fatalf("LP search SQL leaked SIP filter %q:\n%s", banned, sql)
		}
	}
}

func TestGetTableName_OTLPVirtualMappings(t *testing.T) {
	cases := []struct {
		name     string
		hepid    int
		profile  string
		expected string
	}{
		{"traces", otlpHepIDTraces, "default", "homer_lake.otlp_traces"},
		{"metrics", otlpHepIDMetrics, "default", "homer_lake.otlp_metrics"},
		{"logs", otlpHepIDLogs, "default", "homer_lake.otlp_logs"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := getTableName("homer_lake", tc.hepid, tc.profile)
			if got != tc.expected {
				t.Fatalf("getTableName(%d) = %q, want %q", tc.hepid, got, tc.expected)
			}
			if strings.Contains(got, ".main.hep_proto_") {
				t.Fatalf("OTLP table must not use hep_proto_ namespace, got: %s", got)
			}
		})
	}
}

func TestBuildSearchSQLV4_OTLPTracesByTraceID(t *testing.T) {
	req := SearchObjectV4{}
	req.Filter.ProtoType = otlpHepIDTraces
	req.Filter.CallID = "deadbeefcafebabe1122334455667788"
	req.Timestamp.From = 1714400000000
	req.Timestamp.To = 1714403600000

	sql, err := buildSearchSQLV4("homer_lake", &req, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sql, "FROM homer_lake.otlp_traces") {
		t.Fatalf("expected SELECT from otlp_traces, got:\n%s", sql)
	}
	if !strings.Contains(sql, "trace_id = 'deadbeefcafebabe1122334455667788'") {
		t.Fatalf("expected trace_id equality clause, got:\n%s", sql)
	}
	if strings.Contains(sql, "session_id LIKE") || strings.Contains(sql, "cid LIKE") {
		t.Fatalf("OTLP SQL must not reference HEP session_id/cid columns, got:\n%s", sql)
	}
	if !strings.Contains(sql, "ORDER BY timestamp DESC") {
		t.Fatalf("expected default ORDER BY timestamp DESC, got:\n%s", sql)
	}
}

func TestBuildSearchSQLV4_OTLPLogsBodyAndService(t *testing.T) {
	req := SearchObjectV4{}
	req.Filter.ProtoType = otlpHepIDLogs
	req.Filter.Payload = "panic"
	req.Filter.UserAgent = "checkout-svc"

	sql, err := buildSearchSQLV4("homer_lake", &req, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sql, "FROM homer_lake.otlp_logs") {
		t.Fatalf("expected SELECT from otlp_logs, got:\n%s", sql)
	}
	if !strings.Contains(sql, "body LIKE '%panic%'") {
		t.Fatalf("expected body LIKE clause for OTLP logs payload, got:\n%s", sql)
	}
	if !strings.Contains(sql, "service_name LIKE '%checkout-svc%'") {
		t.Fatalf("expected service_name filter sourced from UserAgent, got:\n%s", sql)
	}
	if strings.Contains(sql, "node_id") || strings.Contains(sql, "src_port") {
		t.Fatalf("OTLP SQL must not include HEP-only filters, got:\n%s", sql)
	}
}

func TestBuildSearchSQLV4_OTLPMetricsByName(t *testing.T) {
	req := SearchObjectV4{}
	req.Filter.ProtoType = otlpHepIDMetrics
	req.Filter.SessionID = "http.server.duration"

	sql, err := buildSearchSQLV4("homer_lake", &req, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sql, "FROM homer_lake.otlp_metrics") {
		t.Fatalf("expected SELECT from otlp_metrics, got:\n%s", sql)
	}
	if !strings.Contains(sql, "name LIKE '%http.server.duration%'") {
		t.Fatalf("expected metric name LIKE clause from session/call id, got:\n%s", sql)
	}
}

func TestBuildSearchSQLV4_OTLPMetricsExplicitNameEquality(t *testing.T) {
	req := SearchObjectV4{}
	req.Filter.ProtoType = otlpHepIDMetrics
	req.Filter.Name = "queue.depth"
	req.Filter.SessionID = "ignored-when-name-set"

	sql, err := buildSearchSQLV4("homer_lake", &req, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sql, "name = 'queue.depth'") {
		t.Fatalf("expected exact name = from filter.name, got:\n%s", sql)
	}
	if strings.Contains(sql, "name LIKE '%ignored%'") {
		t.Fatalf("must not apply session/call id name LIKE when filter.name is set, got:\n%s", sql)
	}
}

func TestBuildSearchSQLV4_OTLPMetricsTypeInAndServiceName(t *testing.T) {
	req := SearchObjectV4{}
	req.Filter.ProtoType = otlpHepIDMetrics
	req.Filter.Name = "m"
	req.Filter.Types = []string{"gauge", "sum"}
	req.Filter.ServiceName = "checkout"

	sql, err := buildSearchSQLV4("homer_lake", &req, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sql, `"type" IN ('gauge','sum')`) {
		t.Fatalf("expected quoted type IN clause, got:\n%s", sql)
	}
	if !strings.Contains(sql, "service_name LIKE '%checkout%'") {
		t.Fatalf("expected service_name from filter.service_name, got:\n%s", sql)
	}
}

func TestBuildOTLPMetricNamesSQL(t *testing.T) {
	q := buildOTLPMetricNamesSQL("homer_lake", 1_700_000_000_000, 1_700_000_360_000, "api")
	if !strings.Contains(q, "FROM homer_lake.otlp_metrics") {
		t.Fatalf("expected otlp_metrics table, got:\n%s", q)
	}
	if !strings.Contains(q, "SELECT DISTINCT name") {
		t.Fatalf("expected DISTINCT name, got:\n%s", q)
	}
	if !strings.Contains(q, "service_name LIKE '%api%'") {
		t.Fatalf("expected service_name narrow, got:\n%s", q)
	}
	if !strings.Contains(q, "LIMIT 2000") {
		t.Fatalf("expected cap limit, got:\n%s", q)
	}
}

func TestTransactionMessagesSelectSQL_OTLPTracesUsesTraceIDAndLimit(t *testing.T) {
	q := transactionMessagesSelectSQL("homer_lake.otlp_traces", []string{"abc123", "def456"}, 0, 0)
	if !strings.Contains(q, "trace_id = 'abc123'") || !strings.Contains(q, "trace_id = 'def456'") {
		t.Fatalf("expected trace_id OR chain, got:\n%s", q)
	}
	if strings.Contains(q, "session_id =") {
		t.Fatalf("must not use session_id for otlp_traces, got:\n%s", q)
	}
	if !strings.Contains(q, "LIMIT 5000") {
		t.Fatalf("expected LIMIT 5000 for otlp_traces, got:\n%s", q)
	}
}

func TestTransactionMessagesSelectSQL_OTLPLogsUsesTraceIDAndLimit(t *testing.T) {
	q := transactionMessagesSelectSQL("homer_lake.otlp_logs", []string{"abc123"}, 0, 0)
	if !strings.Contains(q, "trace_id = 'abc123'") {
		t.Fatalf("expected trace_id clause for otlp_logs, got:\n%s", q)
	}
	if strings.Contains(q, "session_id =") {
		t.Fatalf("must not use session_id for otlp_logs, got:\n%s", q)
	}
	if !strings.Contains(q, "LIMIT 5000") {
		t.Fatalf("expected LIMIT 5000 for otlp_logs, got:\n%s", q)
	}
}

func TestTransactionMessagesSelectSQL_OTLPMetricsUsesNameAndLimit(t *testing.T) {
	q := transactionMessagesSelectSQL("homer_lake.otlp_metrics", []string{"http.server.duration"}, 0, 0)
	if !strings.Contains(q, "name = 'http.server.duration'") {
		t.Fatalf("expected name equality for otlp_metrics, got:\n%s", q)
	}
	if strings.Contains(q, "session_id =") {
		t.Fatalf("must not use session_id for otlp_metrics, got:\n%s", q)
	}
	if !strings.Contains(q, "LIMIT 5000") {
		t.Fatalf("expected LIMIT 5000 for otlp_metrics, got:\n%s", q)
	}
}

func TestTransactionMessagesSelectSQL_SIPUsesSessionID(t *testing.T) {
	q := transactionMessagesSelectSQL("homer_lake.main.hep_proto_1_call", []string{"call-id-1"}, 0, 0)
	if !strings.Contains(q, "session_id = 'call-id-1'") {
		t.Fatalf("expected session_id clause for SIP table, got:\n%s", q)
	}
	if strings.Contains(q, "LIMIT 5000") {
		t.Fatalf("SIP query must not apply OTLP span limit, got:\n%s", q)
	}
}

func TestBuildSearchSQLV4_VirtualDataExtra(t *testing.T) {
	req := SearchObjectV4{}
	req.Filter.ProtoType = 1
	req.Filter.EventType = "call"
	req.Filter.Virtual = map[string]string{"to_tag": "abc7"}

	rules := map[string]services.VirtualFieldRule{
		"to_tag": {
			Kind:  services.VirtualKindDataExtraJSON,
			Path:  "to_tag",
			Match: services.VirtualMatchLike,
		},
	}

	sql, err := buildSearchSQLV4("homer_lake", &req, rules)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sql, "json_extract(data_extra, '$.to_tag')") {
		t.Fatalf("expected virtual to_tag extract, got:\n%s", sql)
	}
	if !strings.Contains(sql, "abc7") {
		t.Fatalf("expected filter value in SQL, got:\n%s", sql)
	}
}

func TestBuildSearchSQLV4_VirtualDataExtraEquals(t *testing.T) {
	req := SearchObjectV4{}
	req.Filter.ProtoType = 1
	req.Filter.EventType = "call"
	req.Filter.Virtual = map[string]string{"branch": "z9hG4bK1"}

	rules := map[string]services.VirtualFieldRule{
		"branch": {
			Kind:  services.VirtualKindDataExtraJSON,
			Path:  "branch",
			Match: services.VirtualMatchEquals,
		},
	}

	sql, err := buildSearchSQLV4("homer_lake", &req, rules)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sql, "json_extract(data_extra, '$.branch')") {
		t.Fatalf("expected branch extract, got:\n%s", sql)
	}
	if !strings.Contains(sql, "= 'z9hG4bK1'") {
		t.Fatalf("expected equals clause, got:\n%s", sql)
	}
}

func TestBuildSearchSQLV4_VirtualAbsentToTag(t *testing.T) {
	req := SearchObjectV4{}
	req.Filter.ProtoType = 1
	req.Filter.EventType = "call"
	req.Filter.Method = "INVITE"
	req.Filter.VirtualAbsent = []string{"no_to_tag"}

	rules := map[string]services.VirtualFieldRule{
		"no_to_tag": {
			Kind:  services.VirtualKindDataExtraJSON,
			Path:  "to_tag",
			Match: services.VirtualMatchAbsent,
		},
	}

	sql, err := buildSearchSQLV4("homer_lake", &req, rules)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sql, "method IN ('INVITE')") {
		t.Fatalf("expected INVITE filter, got:\n%s", sql)
	}
	if !strings.Contains(sql, "json_extract(data_extra, '$.to_tag')") {
		t.Fatalf("expected to_tag extract for absent, got:\n%s", sql)
	}
	if !strings.Contains(sql, "IS NULL") {
		t.Fatalf("expected IS NULL for absent, got:\n%s", sql)
	}
}

func TestBuildSearchSQLV4_VirtualPresentToTag(t *testing.T) {
	req := SearchObjectV4{}
	req.Filter.ProtoType = 1
	req.Filter.EventType = "call"
	req.Filter.VirtualPresent = []string{"has_to_tag"}

	rules := map[string]services.VirtualFieldRule{
		"has_to_tag": {
			Kind:  services.VirtualKindDataExtraJSON,
			Path:  "to_tag",
			Match: services.VirtualMatchPresent,
		},
	}

	sql, err := buildSearchSQLV4("homer_lake", &req, rules)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sql, "IS NOT NULL") {
		t.Fatalf("expected IS NOT NULL for present, got:\n%s", sql)
	}
}
