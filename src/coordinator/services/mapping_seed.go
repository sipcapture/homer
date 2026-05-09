// Copyright (C) 2026 Homer Server Contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package services

import (
	"context"
	"database/sql"
	_ "embed"
	"fmt"
)

// SIP / RTCP / DNS / LOG field lists align with storage/ducklake/tables.go and
// filter keys in buildSearchSQLV4 (coordinator/handlers/transactions_v4.go).
// Optional fields_mapping[].virtual declares JSON data_extra filters — see
// services.VirtualRulesFromFieldsMapping (match: like | equals | absent | present).

//go:embed seeds/fields_sip_call.json
var fieldsMappingSIPCall []byte

//go:embed seeds/fields_sip_default.json
var fieldsMappingSIPDefault []byte

//go:embed seeds/fields_sip_registration.json
var fieldsMappingSIPRegistration []byte

//go:embed seeds/fields_rtcp_5_default.json
var fieldsMappingRTCP5Default []byte

//go:embed seeds/fields_dns_53_default.json
var fieldsMappingDNS53Default []byte

//go:embed seeds/fields_log_100_default.json
var fieldsMappingLOG100Default []byte

// OTLP "virtual" mappings — they are not real HEP types but reuse the same
// mapping_schema row so the Proto Search widget can list them and route the
// search to the dedicated otlp_* DuckLake tables. Hepid 200/201/202 are
// chosen to sit above the conventional HEP range (1..199) to avoid clashes
// with future official HEP type assignments.

//go:embed seeds/fields_otlp_traces.json
var fieldsMappingOTLPTraces []byte

//go:embed seeds/fields_otlp_metrics.json
var fieldsMappingOTLPMetrics []byte

//go:embed seeds/fields_otlp_logs.json
var fieldsMappingOTLPLogs []byte

// correlationMappingEmpty is stored for UI/API parity; homer-core correlation
// uses Lua scripts (correlation_scripts), not this JSON blob.
const correlationMappingEmpty = `[]`

// Stable GUIDs for default rows (documentation / support); do not change once shipped.
const (
	defaultMappingGUIDCall         = "a1111111-1111-4111-8111-111111111101"
	defaultMappingGUIDDefault      = "a1111111-1111-4111-8111-111111111102"
	defaultMappingGUIDRegistration = "a1111111-1111-4111-8111-111111111103"
	defaultMappingGUIDRTCP5        = "a1111111-1111-4111-8111-111111111104"
	defaultMappingGUIDDNS53        = "a1111111-1111-4111-8111-111111111105"
	defaultMappingGUIDLOG100       = "a1111111-1111-4111-8111-111111111106"
	defaultMappingGUIDOTLPTraces   = "a1111111-1111-4111-8111-111111111107"
	defaultMappingGUIDOTLPMetrics  = "a1111111-1111-4111-8111-111111111108"
	defaultMappingGUIDOTLPLogs     = "a1111111-1111-4111-8111-111111111109"
)

// homer-app install placeholder; DuckLake DDL is owned by the node writer.
const defaultMappingCreateTable = "CREATE TABLE test(id integer, data text);"

// SeedDefaultMappingSchema inserts default mapping_schema rows for every
// built-in protocol/profile pair we ship: SIP (call / default /
// registration), RTCP JSON (5), DNS (53), LOG (100), and the three
// virtual OTLP profiles (200 / 201 / 202).
//
// Human-readable copies of the embedded fields_*.json live under
// examples/mappings/ — keep them in sync when editing seeds.
//
// The function is idempotent **per row** rather than per-table: it
// looks up each well-known GUID and inserts only the missing ones. The
// previous "INSERT ALL when table is empty" shortcut meant that
// upgrading a deployment which already had any seeded row (SIP, etc.)
// would never receive new defaults — most notably the OTLP entries
// added in 11.0.118 — even though they were declared in this file.
func SeedDefaultMappingSchema(ctx context.Context, db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("settings db not available")
	}

	type row struct {
		guid, profile, hepAlias string
		hepid                     int
		fields                    []byte
	}
	rows := []row{
		{defaultMappingGUIDCall, "call", "SIP", 1, fieldsMappingSIPCall},
		{defaultMappingGUIDDefault, "default", "SIP", 1, fieldsMappingSIPDefault},
		{defaultMappingGUIDRegistration, "registration", "SIP", 1, fieldsMappingSIPRegistration},
		{defaultMappingGUIDRTCP5, "default", "RTCP", 5, fieldsMappingRTCP5Default},
		{defaultMappingGUIDDNS53, "default", "DNS", 53, fieldsMappingDNS53Default},
		{defaultMappingGUIDLOG100, "default", "LOG", 100, fieldsMappingLOG100Default},
		{defaultMappingGUIDOTLPTraces, "default", "OTLP_TRACES", 200, fieldsMappingOTLPTraces},
		{defaultMappingGUIDOTLPMetrics, "default", "OTLP_METRICS", 201, fieldsMappingOTLPMetrics},
		{defaultMappingGUIDOTLPLogs, "default", "OTLP_LOGS", 202, fieldsMappingOTLPLogs},
	}

	for _, r := range rows {
		// Per-row presence check — keyed on the well-known guid so
		// operator-renamed profiles aren't duplicated and we never
		// overwrite a row the user has customised.
		var present int64
		if err := db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM mapping_schema WHERE guid = '`+escapeSQL(r.guid)+`'`,
		).Scan(&present); err != nil {
			return fmt.Errorf("check mapping guid=%s: %w", r.guid, err)
		}
		if present > 0 {
			continue
		}

		q := fmt.Sprintf(`INSERT INTO mapping_schema (
			guid, profile, hepid, hep_alias, partid, version, retention, partition_step,
			create_index, create_table, correlation_mapping, fields_mapping, mapping_settings,
			schema_mapping, schema_settings, create_date
		) VALUES (
			'%s', '%s', %d, '%s', 10, 1, 14, 3600,
			'%s',
			'%s',
			'%s',
			'%s',
			'%s',
			'%s',
			'%s',
			current_timestamp
		)`,
			escapeSQL(r.guid),
			escapeSQL(r.profile),
			r.hepid,
			escapeSQL(r.hepAlias),
			escapeJSONData("{}"),
			escapeSQL(defaultMappingCreateTable),
			escapeJSONData(correlationMappingEmpty),
			escapeJSONData(string(r.fields)),
			escapeJSONData("{}"),
			escapeJSONData("{}"),
			escapeJSONData("{}"),
		)
		if _, err := db.ExecContext(ctx, q); err != nil {
			return fmt.Errorf("insert mapping hepid=%d profile=%s: %w", r.hepid, r.profile, err)
		}
	}
	return nil
}
