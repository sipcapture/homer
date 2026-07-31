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
	if !strings.Contains(sql, "json_extract_string(data_extra, '$.capture_id')") {
		t.Fatalf("expected json_extract_string for capture_id, got:\n%s", sql)
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
	if !strings.Contains(sql, "aor = 'bob@'") {
		t.Fatalf("expected aor =, got:\n%s", sql)
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
	if !strings.Contains(sql, "user_agent = 'Polycom'") {
		t.Fatalf("expected top-level user_agent = for registration, got:\n%s", sql)
	}
}

func TestBuildSearchSQLV4_RegistrationCallIDUsesSessionIDOnly(t *testing.T) {
	// hep_proto_1_registration has session_id but no cid (#884).
	req := SearchObjectV4{}
	req.Filter.ProtoType = 1
	req.Filter.EventType = "registration"
	req.Filter.CallID = "bb2d844e-ba46-4dac-86b3-c0afce431376"

	sql, err := buildSearchSQLV4("homer_lake", &req, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sql, "FROM homer_lake.main.hep_proto_1_registration") {
		t.Fatalf("expected registration table, got:\n%s", sql)
	}
	if !strings.Contains(sql, "session_id = 'bb2d844e-ba46-4dac-86b3-c0afce431376'") {
		t.Fatalf("expected session_id = Call-ID, got:\n%s", sql)
	}
	if strings.Contains(sql, "cid") {
		t.Fatalf("registration Call-ID filter must not reference cid, got:\n%s", sql)
	}
}

func TestBuildSearchSQLV4_RegistrationCIDAliasToSessionID(t *testing.T) {
	req := SearchObjectV4{}
	req.Filter.ProtoType = 1
	req.Filter.EventType = "registration"
	req.Filter.CID = "reg-cid-1"

	sql, err := buildSearchSQLV4("homer_lake", &req, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sql, "session_id = 'reg-cid-1'") {
		t.Fatalf("expected cid filter aliased to session_id, got:\n%s", sql)
	}
	if strings.Contains(sql, "cid =") {
		t.Fatalf("registration must not filter on cid column, got:\n%s", sql)
	}
}

func TestBuildSearchSQLV4_CallStillMatchesSessionOrCID(t *testing.T) {
	req := SearchObjectV4{}
	req.Filter.ProtoType = 1
	req.Filter.EventType = "call"
	req.Filter.CallID = "call-1"

	sql, err := buildSearchSQLV4("homer_lake", &req, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := "(session_id = 'call-1' OR cid = 'call-1')"
	if !strings.Contains(sql, want) {
		t.Fatalf("expected %q in call search SQL, got:\n%s", want, sql)
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
		{"main schema", "main__cpu", "homer_lake.main.cpu"},
		{"app schema", "apps__http_requests", "homer_lake.apps.http_requests"},
		{"missing separator falls back to main", "legacy", "homer_lake.main.legacy"},
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

func TestLpTableForProfile_RejectsInjection(t *testing.T) {
	// GHSA-94cf-g6mg-6gv7: crafted event_type must never become part of FROM.
	cases := []struct {
		name    string
		profile string
	}{
		{"union injection", "main__cpu UNION SELECT 1"},
		{"comment injection", "main__cpu--"},
		{"semicolon breakout", "main__cpu;DROP TABLE x"},
		{"quote breakout", `main__cpu" OR 1=1`},
		{"space in table", "main__cpu stats"},
		{"dot path traversal", "main__cpu.secret"},
		{"schema information_schema", "information_schema__tables"},
		{"schema pg_catalog", "pg_catalog__pg_tables"},
		{"ducklake catalog table", "main__ducklake_snapshot"},
		{"legacy unsafe bare profile", "cpu;SELECT 1"},
		{"empty", ""},
		{"leading digit schema", "1main__cpu"},
		{"leading digit table", "main__1cpu"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := lpTableForProfile("homer_lake", tc.profile)
			if ok {
				t.Fatalf("lpTableForProfile(%q) = %q, ok=true; want ok=false", tc.profile, got)
			}
		})
	}
}

func TestBuildSearchSQLV4_LPRejectsUnsafeEventType(t *testing.T) {
	req := SearchObjectV4{}
	req.Filter.ProtoType = lpHepID
	req.Filter.EventType = "main__cpu UNION SELECT password FROM users"
	req.Timestamp.From = 1714400000000
	req.Timestamp.To = 1714403600000

	sql, err := buildSearchSQLV4("homer_lake", &req, nil)
	if err == nil {
		t.Fatalf("expected error for unsafe LP event_type, got SQL:\n%s", sql)
	}
	if sql != "" {
		t.Fatalf("expected empty SQL on rejection, got:\n%s", sql)
	}
}

func TestBuildSearchSQLV4_LPRoutesToTimeColumn(t *testing.T) {
	req := SearchObjectV4{}
	req.Filter.ProtoType = lpHepID
	req.Filter.EventType = "main__cpu"
	req.Timestamp.From = 1714400000000
	req.Timestamp.To = 1714403600000
	req.Param.Limit = 100

	sql, err := buildSearchSQLV4("homer_lake", &req, nil)
	if err != nil {
		t.Fatalf("buildSearchSQLV4 LP: %v", err)
	}
	// LP tables expose `time`, not `timestamp` — make sure the
	// builder emits clauses against the right column.
	if !strings.Contains(sql, "FROM homer_lake.main.cpu") {
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
	if !strings.Contains(sql, "body = 'panic'") {
		t.Fatalf("expected body = clause for OTLP logs payload, got:\n%s", sql)
	}
	if !strings.Contains(sql, "service_name = 'checkout-svc'") {
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
	if !strings.Contains(sql, "name = 'http.server.duration'") {
		t.Fatalf("expected metric name = clause from session/call id, got:\n%s", sql)
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
	if !strings.Contains(sql, "service_name = 'checkout'") {
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
	if !strings.Contains(sql, "json_extract_string(data_extra, '$.to_tag')") {
		t.Fatalf("expected virtual to_tag extract, got:\n%s", sql)
	}
	if !strings.Contains(sql, "= 'abc7'") {
		t.Fatalf("expected exact match for virtual like mapping, got:\n%s", sql)
	}
	if strings.Contains(sql, "LIKE '%abc7%'") {
		t.Fatalf("virtual field without %% must not use substring LIKE, got:\n%s", sql)
	}
}

func TestBuildSearchSQLV4_FormExactMatchUnlessPercent(t *testing.T) {
	req := SearchObjectV4{}
	req.Filter.ProtoType = 1
	req.Filter.EventType = "call"
	req.Filter.FromUser = "alice"

	sql, err := buildSearchSQLV4("homer_lake", &req, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sql, "caller = 'alice'") {
		t.Fatalf("expected caller = for plain value, got:\n%s", sql)
	}
	if strings.Contains(sql, "caller LIKE") {
		t.Fatalf("plain value must not use LIKE, got:\n%s", sql)
	}

	req.Filter.FromUser = "ali%"
	sql2, err := buildSearchSQLV4("homer_lake", &req, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sql2, "caller LIKE 'ali%'") {
		t.Fatalf("expected caller LIKE when value contains %%, got:\n%s", sql2)
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
	if !strings.Contains(sql, "json_extract_string(data_extra, '$.branch')") {
		t.Fatalf("expected branch extract, got:\n%s", sql)
	}
	if !strings.Contains(sql, "= 'z9hG4bK1'") {
		t.Fatalf("expected equals clause, got:\n%s", sql)
	}
}

func TestBuildSearchSQLV4_VirtualDataExtraCustomHeader(t *testing.T) {
	req := SearchObjectV4{}
	req.Filter.ProtoType = 1
	req.Filter.EventType = "call"
	req.Filter.Virtual = map[string]string{"x_icc_uuid": "uuid-123"}

	rules := map[string]services.VirtualFieldRule{
		"x_icc_uuid": {
			Kind:  services.VirtualKindDataExtraJSON,
			Path:  "custom_headers.X-icc_uuid",
			Match: services.VirtualMatchEquals,
		},
	}

	sql, err := buildSearchSQLV4("homer_lake", &req, rules)
	if err != nil {
		t.Fatal(err)
	}
	want := `json_extract_string(data_extra, '$.custom_headers."X-icc_uuid"')`
	if !strings.Contains(sql, want) {
		t.Fatalf("expected %q in SQL, got:\n%s", want, sql)
	}
	if !strings.Contains(sql, "= 'uuid-123'") {
		t.Fatalf("expected filter value in SQL, got:\n%s", sql)
	}
}

// Regression: data_extra value comparisons must use json_extract_string(), not the
// `->>` operator. `data_extra ->> '$.path'` combined with a timestamp-range filter
// (which the search UI always sends) triggers a DuckDB planner error at runtime —
// "Conversion Error: Failed to cast value to numerical" — and 500s the search
// endpoint. json_extract_string() returns the same unquoted value without the issue.
// See PR #859 (which introduced `->>`).
func TestBuildSearchSQLV4_VirtualEqualsUsesJsonExtractStringNotArrowOp(t *testing.T) {
	req := SearchObjectV4{}
	req.Filter.ProtoType = 1
	req.Filter.EventType = "call"
	req.Filter.Virtual = map[string]string{"x_grp": "GRP-1"}
	req.Timestamp.From = 1700000000000
	req.Timestamp.To = 1700003600000

	rules := map[string]services.VirtualFieldRule{
		"x_grp": {
			Kind:  services.VirtualKindDataExtraJSON,
			Path:  "custom_headers.X-Group-Id",
			Match: services.VirtualMatchEquals,
		},
	}

	sql, err := buildSearchSQLV4("homer_lake", &req, rules)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sql, `json_extract_string(data_extra, '$.custom_headers."X-Group-Id"')`) {
		t.Fatalf("virtual equals must use json_extract_string, got:\n%s", sql)
	}
	if strings.Contains(sql, "data_extra ->>") {
		t.Fatalf("virtual field must NOT use the ->> operator (breaks combined with a timestamp filter), got:\n%s", sql)
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
	if !strings.Contains(sql, "json_extract_string(data_extra, '$.to_tag')") {
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
