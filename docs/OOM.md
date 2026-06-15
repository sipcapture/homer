# OOM Troubleshooting (DuckLake Search and Compaction)

This guide is for Homer deployments that fail with DuckDB errors like:

- `Out of Memory Error: failed to allocate ...`
- `(... GiB/... GiB used)`
- Search API returns `500` on `/api/v4/transactions/search`

Typical failing query shape:

- Large time range
- `ORDER BY timestamp DESC LIMIT ...`
- Union of lake + in-memory buffers (`mem_hep_proto_*`)

## 1) Fast Runtime Fix (No Recompile)

Edit your runtime config (for example `/usr/local/homer-core/etc/homer.json`) and set:

```json
{
  "storage": {
    "ducklake": {
      "tuning": {
        "threads": 2,
        "memory_limit": "4GB",
        "temp_directory": "/data/homer/.duckdb_spill"
      }
    }
  },
  "node": {
    "ducklake": {
      "tuning": {
        "threads": 2,
        "memory_limit": "4GB",
        "temp_directory": "/data/homer/.duckdb_spill"
      }
    }
  }
}
```

Then:

```bash
sudo mkdir -p /data/homer/.duckdb_spill
sudo chmod 755 /data/homer/.duckdb_spill
sudo systemctl restart homer-core
```

## 2) Verify Tuning Applied

Check logs:

```bash
journalctl -u homer-core -n 200 --no-pager | rg "DuckDB tuning:"
```

Expected lines:

- `DuckDB tuning: memory_limit set`
- `DuckDB tuning: temp_directory set`
- `DuckDB tuning: preserve_insertion_order disabled`

## 3) Re-test Search

Repeat the same failing search (UI or API) and confirm:

- HTTP `200` for `/api/v4/transactions/search`
- No new `Node: Query failed ... Out of Memory`

Useful check:

```bash
journalctl -u homer-core -n 200 --no-pager | rg "V4TransactionsSearch|Node: Query failed|Out of Memory|POST /api/v4/transactions/search"
```

## 4) Low-RAM Profile (If You Cannot Allocate 4GB)

Use this profile:

```json
{
  "storage": {
    "ducklake": {
      "search_buffer": false,
      "tuning": {
        "threads": 1,
        "memory_limit": "1500MB",
        "temp_directory": "/data/homer/.duckdb_spill"
      },
      "compaction": {
        "enable": false
      }
    }
  },
  "node": {
    "ducklake": {
      "tuning": {
        "threads": 1,
        "memory_limit": "1500MB",
        "temp_directory": "/data/homer/.duckdb_spill"
      }
    }
  }
}
```

Why this helps:

- `threads=1` lowers concurrent memory pressure
- `search_buffer=false` avoids union with in-memory buffers
- `compaction.enable=false` removes heavy merge path
- `temp_directory` enables disk spill instead of hard OOM

## 5) If OOM Persists

1. Increase `memory_limit` one step (`4GB -> 6GB`).
2. Keep spill directory on fast disk (not tmpfs).
3. Reduce query range temporarily (narrower time window).
4. Re-enable compaction only after search is stable.

## 6) Native Go Compaction Engine (EXPERIMENTAL / UNSAFE)

> ⚠️ **Do not enable `engine: native` on a live writer.** It corrupts the
> DuckLake catalog. The DuckLake writer keeps its snapshot/id counters cached in
> DuckDB process memory and allocates new snapshot ids by incrementing that
> cache — it does **not** re-read the SQLite catalog before each commit. The
> native engine allocates snapshot ids out-of-band as `MAX(snapshot_id)+1` in
> SQLite, so the next flush reuses the same id and writes a **duplicate**
> `ducklake_snapshot` row. The search node then fails with:
>
> ```
> Invalid Input Error: Corrupt DuckLake - multiple snapshots returned from database
> ```
>
> The `CatalogLock` does not prevent this: it serializes the *writes*, but
> DuckLake's id counter lives in memory and is oblivious to out-of-band SQLite
> writes. As of 11.0.257 the engine is **opt-in** and the default is `duckdb`.
> See "Recovering a corrupted catalog" below if you already hit this.

The default DuckDB merge (`ducklake_merge_adjacent_files`) loads a whole
partition into memory to sort/rewrite it. On wide SIP data (large `payload`
columns) this can exceed `memory_limit` and OOM even for a handful of ~77MB
files, because the parquet write buffers are not spillable. Use the batching
knobs (`max_compacted_files`, `max_file_size_bytes`, lower `memory_limit`
headroom) from the sections above to keep it within budget.

The `native` engine avoids the DuckDB merge entirely. It does **not** use
DuckDB for compaction. Instead it:

- groups a partition's parquet files into batches up to `target_file_size_bytes`
  (default 512MB), based on their on-disk sizes;
- concatenates each batch by copying parquet **row groups** one at a time, so
  peak memory is bounded by a single row group (a few hundred MB), never the
  whole partition;
- registers the new files and retires the old ones by writing the DuckLake
  SQLite catalog directly (new snapshot, `ducklake_data_file`,
  `ducklake_file_column_stats`, `ducklake_file_partition_value`, retire via
  `end_snapshot`, `ducklake_table_stats` bookkeeping);
- reaps fully-superseded files and aged-out snapshots once they fall outside
  the retention window (`snapshot_expire_interval_sec`).

### Configuration

The default is `duckdb`. The native engine is **opt-in** and unsafe with a live
writer (see the warning above); only enable it if the writer is fully quiescent:

```json
{
  "storage": {
    "ducklake": {
      "catalog_path": "/data/homer/homer_catalog.sqlite",
      "data_path": "/data/homer/parquet",
      "compaction": {
        "enable": true,
        "engine": "native",
        "target_file_size_bytes": 536870912
      }
    }
  }
}
```

Leaving `engine` unset (or `"duckdb"`) uses the DuckLake merge, which is the
recommended, catalog-safe path.

### Recovering a corrupted catalog

If you already see `Corrupt DuckLake - multiple snapshots returned from
database`, the SQLite catalog has duplicate `snapshot_id` rows.

**Preferred: rebuild the catalog from disk.** Stop the Homer writer, then run:

```bash
homer system --config-path /etc/homer/config.json --rebuild-catalog
```

This backs up the existing catalog to `*.corrupt.<timestamp>`, attaches a fresh
empty catalog, and re-ingests every on-disk parquet table through DuckLake (so
all snapshot/file ids are allocated by DuckLake and the result is consistent).
After verifying queries work, reclaim the old files with either:

```bash
homer system --config-path /etc/homer/config.json --rebuild-catalog --rebuild-cleanup-orphans
# or, later:
homer system --config-path /etc/homer/config.json --compaction-force
```

Notes:
- Run with the writer **stopped** (it discards the live catalog).
- Local `data_path` + SQLite catalog only; single-catalog (non-sharded) layouts.
- Rows that were only ever inlined in the old catalog (never written to parquet)
  cannot be recovered this way.

**Manual alternative (dedup in place).** If you prefer to repair the existing
catalog, **back up the catalog file first**, then dedup:

```sql
-- 1) Back up first:  cp homer_catalog.sqlite homer_catalog.sqlite.bak
-- 2) Inspect the damage:
SELECT snapshot_id, COUNT(*) c FROM ducklake_snapshot GROUP BY snapshot_id HAVING c > 1;

-- 3) Keep one row per snapshot_id (the most-advanced next_file_id / next_catalog_id):
DELETE FROM ducklake_snapshot
WHERE rowid NOT IN (
  SELECT rowid FROM ducklake_snapshot s
  WHERE s.next_file_id = (SELECT MAX(next_file_id) FROM ducklake_snapshot x WHERE x.snapshot_id = s.snapshot_id)
  GROUP BY s.snapshot_id
);

-- 4) Re-base the latest snapshot's counters above every registered id so the
--    writer cannot reuse one:
UPDATE ducklake_snapshot
SET next_file_id    = (SELECT COALESCE(MAX(data_file_id),0)+1 FROM ducklake_data_file),
    next_catalog_id = (SELECT MAX(next_catalog_id) FROM ducklake_snapshot)
WHERE snapshot_id = (SELECT MAX(snapshot_id) FROM ducklake_snapshot);
```

Then set `engine` to `duckdb` (or remove it) and restart. If you keep a backup
of the catalog from before the native run, restoring it is the safest recovery.

Requirements and behavior:

- **Live-writer hazard** — see the warning at the top of this section. Only run
  native compaction when no flush can land concurrently.
- **Local storage + SQLite catalog** — for remote (`s3://`) `data_path` or a
  missing `catalog_path`, the writer **automatically falls back to the `duckdb`
  engine**, so compaction always runs.
- **Append-only tables only** — tables with delete files are skipped (the engine
  reassigns row ids, which is only safe without positional deletes). HEP ingest
  is append-only, so this always holds for Homer.
- Holds the `CatalogLock` only for the short per-partition commit/reap phases,
  never during the slow merge, so flush/ingest stays responsive.

Memory profile: bounded by one row group regardless of partition size, so a
512MB target safely compacts 76×77MB files on a writer capped well under the
multi-GB working set the DuckDB merge required.

> Note: after a native commit, the search node picks up the new snapshot on its
> next periodic catalog refresh.

## Notes

- Runtime config changes do **not** require recompilation.
- Recompile only when changing Homer source code itself.
