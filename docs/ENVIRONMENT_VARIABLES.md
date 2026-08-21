# Environment variables (modular Homer server)

When Homer starts **without** a usable JSON/YAML config file, `config.Load` still applies **defaults** and reads **environment variables** ([`src/config/config.go`](../src/config/config.go)).

## Rules

| Mechanism | Detail |
|-----------|--------|
| Prefix | All variables for this path use the prefix **`HOMER`**. |
| Nesting | JSON / mapstructure keys use dots in code (e.g. `storage.ducklake.catalog_path`). In the environment, **dots become underscores**: `HOMER_STORAGE_DUCKLAKE_CATALOG_PATH`. |
| Arrays / slices | Use a **numeric index** in the name (Viper convention), e.g. `HOMER_COORDINATOR_NODES_0_HOST`, `HOMER_STORAGE_DUCKLAKE_STORAGE_POLICY_VOLUMES_1_S3_ENDPOINT`. |
| Precedence | Defaults → optional config file → **environment overrides** (via Viper `AutomaticEnv`). |

Legacy **Homer Server** config (`homerconfig`) uses a different prefix (`HOMERCORE`) and only applies in legacy mode — not the modular stack described here.

## Discovering names

Field names follow the `mapstructure` tags on [`Config` in `src/config/config.go`](../src/config/config.go) and nested structs (`ingest`, `storage`, `node`, `coordinator`, `log`, `prometheus`, …). Example:

- `storage.ducklake.storage_policy.volumes[1].s3_endpoint`  
  → `HOMER_STORAGE_DUCKLAKE_STORAGE_POLICY_VOLUMES_1_S3_ENDPOINT`
- `storage.ducklake.s3.url_style` → `HOMER_STORAGE_DUCKLAKE_S3_URL_STYLE` (`path` by default; set `vhost` for S3 virtual-hosted-style endpoints)
- `node.ducklake.volumes[0].s3_url_style` → `HOMER_NODE_DUCKLAKE_VOLUMES_0_S3_URL_STYLE`
- `storage.ducklake.storage_policy.volumes[1].s3_url_style` → `HOMER_STORAGE_DUCKLAKE_STORAGE_POLICY_VOLUMES_1_S3_URL_STYLE`

## References

- Loader: `config.Load` — `SetEnvPrefix("HOMER")`, `SetEnvKeyReplacer(".", "_")`, `AutomaticEnv()` ([source](../src/config/config.go)).
- SIP ingest (`ingest.sip`): `aleg_ids`, `custom_headers`, `force_aleg_id`; see [LUA_CORRELATION.md](LUA_CORRELATION.md#sip-header-lists-writer-ingest) (CID / correlation).
- High-PPS ingest / DuckLake / Prometheus batching: [INGEST_PERFORMANCE.md](INGEST_PERFORMANCE.md).
- Prometheus agent label (`prometheus.agent_label` → `HOMER_PROMETHEUS_AGENT_LABEL`): `node_id` (HEP 0x000c / heplify `-hi`, default) or `node_name` (HEP 0x0013 / heplify `-hn`). Controls the value of the Prometheus `node_id` label for SIP/RTCP/RTP metrics.
- Tiered storage fields: [`docs/STORAGE_POLICIES.md`](STORAGE_POLICIES.md) (conceptual); same paths appear under `storage.ducklake.storage_policy` and `node.ducklake` in JSON — mirror them as `HOMER_*` as above. Native file move (opt-in): `HOMER_STORAGE_DUCKLAKE_STORAGE_POLICY_MOVE_ENGINE=native` — see [NATIVE_TIER_MOVE.md](NATIVE_TIER_MOVE.md).
- Example with variables declared inline in Compose: [`examples/docker/docker-compose.yaml`](../examples/docker/docker-compose.yaml) (`homer.environment`).
- DuckDB engine caps: `HOMER_STORAGE_DUCKLAKE_TUNING_MEMORY_LIMIT`, `HOMER_STORAGE_DUCKLAKE_TUNING_THREADS`, `HOMER_STORAGE_DUCKLAKE_TUNING_TEMP_DIRECTORY` (writer) and the `HOMER_NODE_DUCKLAKE_TUNING_*` equivalents (reader). See [DUCKDB_TUNING.md](DUCKDB_TUNING.md) and [OOM.md](OOM.md).
- **Data retention (TTL):** [`RETENTION.md`](RETENTION.md) — `HOMER_STORAGE_DUCKLAKE_COMPACTION_RETENTION_DAYS` and related compaction env vars. Per-table overrides (`retention_days_by_table`) are configured in JSON (map), not as a single env scalar.
- **Search timeouts:** [`TROUBLESHOOTING.md`](TROUBLESHOOTING.md) — `HOMER_COORDINATOR_QUERY_TIMEOUT_SEC`, `HOMER_COORDINATOR_HTTP_SERVER_READ_TIMEOUT`, `HOMER_COORDINATOR_HTTP_SERVER_WRITE_TIMEOUT`.
