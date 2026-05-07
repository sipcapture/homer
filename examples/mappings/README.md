# Default `mapping_schema` field mappings

These JSON files are **copies** of
[`src/coordinator/services/seeds/`](../src/coordinator/services/seeds/).
They define the `fields_mapping` payload for each **default** row the
coordinator inserts into the settings DuckDB table **`mapping_schema`**
via `SeedDefaultMappingSchema` (`mapping_seed.go`).

## Files

| File | `hepid` | `profile` | `hep_alias` | Notes |
|------|---------|-------------|-------------|--------|
| `fields_sip_call.json` | 1 | `call` | SIP | SIP call search widget |
| `fields_sip_default.json` | 1 | `default` | SIP | Generic SIP profile |
| `fields_sip_registration.json` | 1 | `registration` | SIP | REGISTER flows |
| `fields_rtcp_5_default.json` | 5 | `default` | RTCP | |
| `fields_dns_53_default.json` | 53 | `default` | DNS | HEP DNS id used in seed |
| `fields_log_100_default.json` | 100 | `default` | LOG | HEP LOG |
| `fields_otlp_traces.json` | 200 | `default` | OTLP_TRACES | Virtual mapping → `otlp_traces` |
| `fields_otlp_metrics.json` | 201 | `default` | OTLP_METRICS | → `otlp_metrics` |
| `fields_otlp_logs.json` | 202 | `default` | OTLP_LOGS | → `otlp_logs` |

Row metadata (stable GUIDs, filenames) is also listed in
[`default_seed_rows.json`](./default_seed_rows.json).

## INSERT shape (reference)

`SeedDefaultMappingSchema` inserts one row per GUID if missing. Shared
defaults: `partid=10`, `version=1`, `retention=14`, `partition_step=3600`,
empty JSON for `mapping_settings` / `schema_*`, `correlation_mapping=[]`,
placeholder `create_table`, and `fields_mapping` = contents of the matching
`fields_*.json` (escaped for SQL — see `escapeJSONData` in code).

## Keeping in sync

When you change a seed file under `src/coordinator/services/seeds/`, copy
the same file here (or vice versa) so operators reading `examples/` see
what the next release will ship.

## Line Protocol (`hepid` 300)

Not listed above: **`mapping_schema` rows for LP** are maintained when
measurements are ingested (`lp_mapping_sync`). See
`src/coordinator/services/lp_mapping_sync.go`.
