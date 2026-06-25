# Data retention

Homer 11 has **several retention-related settings**. Only one of them actually **deletes** captured data from DuckLake. The others control tiering, snapshot housekeeping, or legacy mapping metadata.

## Quick reference

| What you want | Setting | Where |
|---------------|---------|--------|
| **Delete data older than N days** (TTL) | `storage.ducklake.compaction.retention_days` | JSON config, [wizard](WIZARD.md), env, CLI |
| Move old partitions to cold / S3 (keep data) | `storage.ducklake.storage_policy.volumes[].max_data_age_days` | [Storage policies](STORAGE_POLICIES.md) |
| DuckLake snapshot / file housekeeping | `storage.ducklake.compaction.snapshot_expire_interval_sec` | Compaction cycle (not the same as TTL) |
| Mapping row `retention` in Settings UI | `mapping_schema.retention` | Legacy Homer 7 field — **does not run TTL in Homer 11** |

---

## Data TTL (`retention_days`)

The writer **CompactionService** deletes rows where `timestamp` is older than the cutoff:

```sql
DELETE FROM <table> WHERE timestamp < TIMESTAMP '<now - retention_days>'
```

This runs **per DuckLake table** during the periodic compaction cycle (after tables are discovered, before merge / expire / cleanup).

### JSON config

```json
{
  "storage": {
    "ducklake": {
      "compaction": {
        "enable": true,
        "check_interval_sec": 1800,
        "retention_days": 30,
        "snapshot_expire_interval_sec": 3600
      }
    }
  }
}
```

| Field | Default | Meaning |
|-------|---------|---------|
| `retention_days` | `0` | Delete data older than **N calendar days**. **`0` = disabled** (no TTL deletes). |
| `check_interval_sec` | `3600` | How often the compaction worker runs (retention runs inside this cycle). |
| `enable` | `true` | Compaction (merge + snapshot maintenance + optional retention) on the writer catalog. |

See also: [Storage layout](STORAGE_LAYOUT.md#throughput-and-compaction-tuning-single-node), [example `homer.json`](../examples/homer.json).

### Environment variables

With prefix `HOMER` and dots → underscores ([rules](ENVIRONMENT_VARIABLES.md)):

```bash
HOMER_STORAGE_DUCKLAKE_COMPACTION_RETENTION_DAYS=30
HOMER_STORAGE_DUCKLAKE_COMPACTION_ENABLE=true
HOMER_STORAGE_DUCKLAKE_COMPACTION_CHECK_INTERVAL_SEC=1800
HOMER_STORAGE_DUCKLAKE_COMPACTION_SNAPSHOT_EXPIRE_INTERVAL_SEC=3600
```

Example in Compose: [`examples/docker/docker-compose.yaml`](../examples/docker/docker-compose.yaml).

### Config wizard

Step **Storage** asks for **Retention days** (default **30**, **`0` = unlimited**). It maps to `storage.ducklake.compaction.retention_days`. See [WIZARD.md](WIZARD.md).

### One-off CLI

Run retention without waiting for the scheduler:

```bash
homer-core system --config-path /etc/homer/homer.json --compaction-retention-days 30
```

Other compaction flags: `homer-core system --help` (`--compaction-force`, `--compaction-expire-snapshots`, …).

### Logs

On each cycle when TTL is enabled:

```
CompactionService: Retention completed  table=hep_proto_1_call  rows_deleted=…
```

Failures are logged per table; missing Parquet files are skipped and cleaned up in a later orphan-file pass.

### Applies to all lake tables

Retention runs on **every table** in the writer DuckLake catalog (HEP `hep_proto_*`, OTLP `otlp_*`, Line Protocol `lp_*`, etc.). There is no per-protocol TTL in config — use separate volumes/tiering or external lifecycle rules on object storage if you need different policies per dataset.

OTLP and Line Protocol docs point here: [OTLP.md](OTLP.md), [LINE_PROTOCOL.md](LINE_PROTOCOL.md).

---

## Tiering vs deletion (`max_data_age_days`)

**Storage policy** moves date partitions from hot → cold volume based on age. It **does not delete** data by itself.

```json
"storage_policy": {
  "volumes": [
    { "name": "hot", "type": "local", "max_data_age_days": 7 },
    { "name": "cold", "type": "s3", "max_data_age_days": 0 }
  ]
}
```

Use this for cost/latency tiering. Combine with:

- **`retention_days`** on the writer to drop old data entirely, and/or
- **S3 lifecycle rules** on the cold bucket (Glacier, expire after 1y, …).

Details: [STORAGE_POLICIES.md](STORAGE_POLICIES.md).

---

## Snapshot housekeeping (`snapshot_expire_interval_sec`)

DuckLake keeps **snapshots** for time travel and merge semantics. The compaction service expires old snapshots and reaps superseded Parquet files using:

- `snapshot_expire_interval_sec` — retention window for snapshot metadata / orphaned files during merge (see [OOM.md](OOM.md) if snapshots pile up).

This is **not** a substitute for `retention_days`: it does not implement “keep 30 days of calls” by itself. Set **`retention_days`** for business TTL; tune **`snapshot_expire_interval_sec`** for catalog health.

Manual maintenance (advanced): [STORAGE_LAYOUT.md](STORAGE_LAYOUT.md#maintenance).

---

## Mapping schema `retention` (Settings → Mappings)

The Coordinator stores a **`retention`** integer on each `mapping_schema` row (default **14** in seeds/UI). This value is **carried over from Homer 7** for compatibility and appears in the Mappings panel.

**Homer 11 does not use this field to delete DuckLake rows.** Operational TTL is **`storage.ducklake.compaction.retention_days`** only.

When migrating from Homer 7/10, align **`retention_days`** in `homer.json` with your compliance window; treat mapping `retention` as documentation unless you have custom tooling that reads it.

---

## Recommended starting points

1. **Single-node / all-in-one:** `retention_days: 30`, compaction enabled, `check_interval_sec: 1800`.
2. **Hot + S3 tiering:** short `max_data_age_days` on hot (e.g. 2–7), longer or zero on cold, plus `retention_days` on the writer if cold must also be trimmed.
3. **Compliance / legal hold:** set `retention_days: 0` (disabled) and manage expiry outside Homer (bucket lifecycle, offline archive).

For OOM or runaway file counts, see [OOM.md](OOM.md) and [INGEST_PERFORMANCE.md](INGEST_PERFORMANCE.md).
