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

## 5b) Read-Path OOM: Long-Range `ORDER BY timestamp DESC LIMIT N`

A `SELECT * ... WHERE timestamp >= A AND timestamp < B ORDER BY timestamp DESC
LIMIT N` over a wide range (e.g. 24h) can OOM even with `split_lake_and_mem`
active: the lake sub-query still has to feed DuckDB's Top-N operator from a
full-range scan, and for payload-heavy SIP rows the operator buffers more than a
small `memory_limit` allows.

`storage.ducklake.search.lake_topn_strategy` controls how that lake sub-query is
executed (it only applies to timestamp-DESC top-N queries over a range wider than
one hour):

| Strategy  | Memory | Ordering | Notes |
|-----------|--------|----------|-------|
| `stream` (default) | minimal | newest-first (Go-sorted), **sample** | Drops `ORDER BY` so DuckDB stops after N rows (flat memory, fastest), then Homer re-sorts the N rows newest-first in Go before returning. The N rows are an arbitrary scan-order sample of the range, so they are correctly ordered but not guaranteed to be the globally newest N. |
| `chunked` | bounded to one window | newest-first, **exact** | Scans descending time windows (`lake_chunk_sec` wide), newest first, stops once N rows are collected. Use when exact newest-N is required. |
| `full`    | unbounded | newest-first, exact | Original single ORDER BY scan over the whole range — can OOM on wide data. |

```jsonc
"storage": {
  "ducklake": {
    "search": {
      "lake_topn_strategy": "stream", // stream (default) | chunked | full
      "lake_chunk_sec": 3600,          // window width for "chunked"
      "lazy_payload": true             // narrow search + by-uuid payload hydration (default true)
    }
  }
}
```

The chosen strategy is reflected in the execution-plan log line
(`mode=split_lake_and_mem:chunked|stream|full`, plus `lake_chunks` for chunked).

The **coordinator transaction search** (`V4TransactionsSearch`, the path the UI
uses for the transactions table) honours the same strategy — it is propagated to
`coordinator.lake_topn_strategy` from `storage.ducklake.search.lake_topn_strategy`
at startup.

Both `stream` and `chunked` execute the range as **newest-first 24h time slices
with early exit**, because a single `SELECT *` over 30 days of wide SIP rows
OOMs at the node regardless of `LIMIT` — the parallel Parquet decompression
alone exceeds a ~2 GB `memory_limit`, so capping the per-window scan breadth is
what actually prevents the OOM (dropping `ORDER BY` is not enough on its own).
They differ only in how each window is read:

- `stream` (default) — **one query over the whole range** with `ORDER BY`
  dropped, so DuckDB streams a flat `LIMIT` and stops after N rows; the rows are
  re-sorted newest-first in Go. No time-slicing — simplest and fastest. Log:
  `V4TransactionsSearch: stream execution (single query)`.
- `chunked` — newest-first 24h windows (`coordinator.lake_chunk_sec`, default
  86400) with early exit; each window keeps `ORDER BY timestamp DESC`, so the
  node sub-splits it into 1h inner windows (`storage.ducklake.search.lake_chunk_sec`,
  default 3600) for memory safety. Originally added for sipcapture/homer#785.
  Log: `strategy=chunked`.
- `full` — a single `ORDER BY` query over the whole range (can OOM).

Note: a single unfiltered `SELECT *` over very long ranges can still pressure
memory at the node even with a flat `LIMIT` (parallel Parquet decompression of
wide rows). If `stream` OOMs on your data, switch to `chunked`.

**Filtered searches are never node-sub-sliced.** When the query carries a
predicate beyond the timestamp range (e.g. `session_id LIKE '%…%'`), the node
runs each window as a single efficient scan instead of slicing it into 1h
windows. Such queries return few rows (no memory risk), and slicing a
non-prunable full-scan filter into many tiny per-window scans multiplies the
catalog/Parquet open overhead and serialises them — which previously timed out.

### Lazy payload hydration (`search.lazy_payload`, default on)

The dominant memory cost of a wide-row search is decompressing the `payload`
(and `data_extra`) columns. DuckDB only does late materialization for small
`Top-N`; for large `LIMIT`s it falls back to a full `ORDER BY` and eagerly
decompresses the wide columns for **every scanned row** — so even a query that
returns 0 rows can OOM (`LIMIT 50` is fine, `LIMIT 50000` is not).

With `lazy_payload` enabled (default), a transaction search runs in two phases:

1. **Search/sort over a narrow projection** — every column *except*
   `payload` / `data_extra`. This is what does the filtering, ordering and
   `LIMIT`, and it stays memory-flat because the wide blobs are never read.
2. **By-uuid hydration** — a single bounded point-lookup
   (`SELECT uuid, payload, data_extra … WHERE uuid IN (…)`, pruned to the
   timestamp span of the matched rows) re-attaches the wide columns for *only*
   the `<= LIMIT` rows actually returned.

Net effect: the heavy `payload` column is decompressed for at most `LIMIT`
rows, never for the whole scanned range. The API response is unchanged
(full rows including `payload`). Disable with
`storage.ducklake.search.lazy_payload: false` (e.g. for debugging). Applies to
default full-row searches only — custom `select` / `group_by` and OTLP/LP
tables bypass it. Log: `V4TransactionsSearch: hydrating payload by uuid`.

## 6) Native Go Compaction Engine

> ℹ️ **The native engine is safe to run alongside the live DuckDB writer as of
> 11.0.260.** Earlier builds corrupted the catalog because the native engine
> allocates snapshot ids out-of-band as `MAX(snapshot_id)+1` in SQLite, while the
> DuckLake writer kept its snapshot/id counter cached in DuckDB memory and reused
> the same id on its next flush — producing a **duplicate** `ducklake_snapshot`
> row and:
>
> ```
> Invalid Input Error: Corrupt DuckLake - multiple snapshots returned from database
> ```
>
> This is now prevented by a two-part protocol: (1) every native commit/reap runs
> under the `CatalogLock`, so it never overlaps a flush's catalog `INSERT`; and
> (2) right after each commit, **while still holding the lock**, the compactor
> refreshes the writer's DuckLake metadata cache (`DETACH`/`ATTACH`), so the
> writer's next flush re-reads the latest snapshot from SQLite and never reuses an
> id the compactor allocated. The engine remains **opt-in** (default `duckdb`).
> Note: this protects against the *compactor*; running **two writer processes**
> on the same catalog still corrupts it — homer takes an exclusive writer lock
> (`<catalog>.lock`) to prevent that. See "Recovering a corrupted catalog" below.

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

The default is `duckdb`. The native engine is **opt-in** and safe to enable on a
live writer (see the note above); it is the recommended choice when the DuckDB
merge OOMs on wide SIP data:

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

There are two independent recovery mechanisms — they complement each other, you
do **not** have to choose one over the other:

| | Startup auto-repair | `--rebuild-catalog` (CLI) |
|---|---|---|
| Trigger | automatic, on every restart | manual, operator-run |
| Data | **lossless** — only drops duplicate metadata rows | discards the catalog, re-ingests from parquet; **inline-only rows are lost** |
| Downtime | none (runs before attach, under the writer lock) | yes — writer must be **stopped** |
| Scope | duplicate `ducklake_snapshot`/`ducklake_table` rows (the common case) | catalog unreadable / desynced beyond duplicate rows |
| Cost | negligible | heavy (rewrites every table, re-allocates all ids) |

Rule of thumb: **auto-repair is the first line of defence** — it self-heals the
common "multiple snapshots" corruption on the next restart with no data loss and
no manual step. Reach for `--rebuild-catalog` only when the catalog is so broken
that auto-repair cannot attach it at all (or files and catalog have diverged).
Keep auto-repair enabled even though the CLI exists.

**Automatic (default).** On startup the writer runs a lossless autofix
(`storage.ducklake.auto_repair_catalog`, default enabled) that collapses
duplicate `ducklake_snapshot` / `ducklake_table` rows — keeping the most
recently written row, which still references the same Parquet files. It runs
before the catalog is attached, while the writer holds the exclusive lock, so it
fixes the corruption on the next restart with no data loss and no manual step.
The repair logs a warning listing how many duplicate rows it removed. Disable
with:

```jsonc
"storage": { "ducklake": { "auto_repair_catalog": false } }
```

**If the autofix cannot recover it** (e.g. corruption beyond duplicate metadata
rows), rebuild the catalog from disk. Stop the Homer writer, then run:

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

- **Safe with a live writer** — each commit/reap holds the `CatalogLock` and then
  refreshes the writer's DuckLake cache, so the writer never reuses a snapshot id
  the compactor allocated (see the note at the top of this section).
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
