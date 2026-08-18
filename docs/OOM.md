# OOM Troubleshooting (DuckLake Search and Compaction)

> **Search stops at ~30s without OOM errors?** That is usually a **timeout**, not memory — see [TROUBLESHOOTING.md](TROUBLESHOOTING.md#search-timeouts-30-seconds).

This guide is for Homer deployments that fail with DuckDB errors like:

- `Out of Memory Error: failed to allocate ...`
- `(... GiB/... GiB used)`
- Search API returns `500` on `/api/v4/transactions/search`

Typical failing query shape:

- Large time range
- `ORDER BY timestamp DESC LIMIT ...`
- Union of lake + in-memory buffers (`mem_hep_proto_*`)

Ingest flush (SIPREC / wide SIP payloads) can hit the same cap:

```
flush mem_hep_proto_1_siprec_b → homer_lake.hep_proto_1_siprec failed
FATAL Error: database has been invalidated
Original error: Failed to create checkpoint ... failed to pin block
(... GiB/... GiB used)
```

A FATAL pin/checkpoint error **invalidates the in-memory DuckDB**. The
process keeps running but every later flush/search fails until restart.
Raise `memory_limit` and restart; retrying the flush cannot recover it.

## Docker image (`ghcr.io/sipcapture/homer`)

The generic image (`Dockerfile`) sets writer DuckDB caps as `ENV`, so
`docker run` and Compose pick them up without a `homer.json`. Override
at runtime — do not rebuild:

```bash
docker run -e HOMER_STORAGE_DUCKLAKE_TUNING_MEMORY_LIMIT=8GB ghcr.io/sipcapture/homer:latest
```

Image / Compose defaults:

```bash
HOMER_STORAGE_DUCKLAKE_TUNING_MEMORY_LIMIT=4GB
HOMER_STORAGE_DUCKLAKE_TUNING_THREADS=2
HOMER_STORAGE_DUCKLAKE_TUNING_TEMP_DIRECTORY=/data/homer/.duckdb_spill
```

Give the Homer container **at least 8GB RAM**. Under SIPREC or high
ingest, raise `MEMORY_LIMIT` toward 50% of that budget (and the
container limit accordingly). See [examples/docker](../examples/docker/README.md).

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

> ℹ️ **The native engine no longer writes the catalog itself.** Builds before
> 11.0.321 allocated snapshot ids out-of-band as `MAX(snapshot_id)+1` in SQLite,
> while the DuckLake writer kept its snapshot/id counter cached in DuckDB memory
> and reused the same id on its next flush — producing a **duplicate**
> `ducklake_snapshot` row and:
>
> ```
> Invalid Input Error: Corrupt DuckLake - multiple snapshots returned from database
> ```
>
> The mitigation at the time was to refresh the writer's cache with a
> `DETACH`/`ATTACH` right after each commit, which is exactly what fails under
> load — and when it failed the corruption still happened, because the snapshot
> was already written.
>
> That whole protocol is gone. The compactor never opens the catalog file: it
> registers merged parquet through DuckLake's own `ducklake_add_data_files`, so
> **DuckLake allocates every snapshot and file id**. The writer's cache cannot
> fall behind ids it issued itself, and no cache refresh (hence no `DETACH`) is
> needed — ingest and search are never interrupted. The engine remains **opt-in**
> (default `duckdb`).
>
> Note: this only ever protected against the *compactor*. Running **two writer
> processes** on the same catalog still corrupts it — homer takes an exclusive
> writer lock (`<catalog>.lock`) to prevent that. See "Recovering a corrupted
> catalog" below.

The default DuckDB merge (`ducklake_merge_adjacent_files`) loads a whole
partition into memory to sort/rewrite it. On wide SIP data (large `payload`
columns) this can exceed `memory_limit` and OOM even for a handful of ~77MB
files, because the parquet write buffers are not spillable. Homer then often
dies **silently** (Linux OOM killer / DuckDB abort) during
`hep_proto_1_call` merge — no `Out of Memory` line, UI returns 5xx, ingest
`adapter_us` spikes because the catalog lock is held for the whole CALL.

Mitigations already in the writer, and the first thing to try:

- DuckDB merge defaults to 32 operations per table per cycle and
  `max_file_size` 64MB. `max_compacted_files` counts output files, not
  inputs; peak memory is the size of one group (`max_file_size`) because
  merge runs on a dedicated connection with `threads=1`.
- Explicit `max_compacted_files` is not clamped. Lower it if RSS still
  climbs; raise it if file count grows between cycles.
- Leftover files are picked up by the next cycle.
- If it still OOMs, lower `max_file_size_bytes` / `max_compacted_files` or
  temporarily set `compaction.enable: false` (see section 4).

The `native` engine does the merge itself instead of asking DuckDB to do it:

- groups a partition's parquet files into batches up to `target_file_size_bytes`
  (default 512MB), based on their on-disk sizes;
- concatenates each batch by copying parquet **row groups** one at a time, so
  peak memory is bounded by a single row group, never the whole partition. That
  bound is confirmed: growing a partition 8x (97MB -> 779MB) moved peak Go heap
  only from 870MB to 1046MB. **But a row group is sized in rows, not bytes**, so
  wide rows make the bound itself large: real Homer files have been measured with
  1.5GB row groups (10,568 rows of SIP payload), and merging three of them peaked
  at ~10GB RSS. This memory is Arrow's heap and is **not** covered by DuckDB's
  `memory_limit`;
- refuses partitions whose largest source row group exceeds
  `max_row_group_bytes` (default 256MB uncompressed, read from the parquet footer
  so it costs no column reads). Expect peak RSS several times the budget. This is
  the guard that keeps the merge from becoming the OOM it was written to avoid;
  raise it only if the writer has the memory to spare;
- leaves a partition alone until nothing has been written to it for
  `min_age_sec`, measured from the snapshot that added its newest file. Ingest
  targets today's partition continuously, and merging it races the writer's
  flush: the swap then finds a changed row count and discards the merged file,
  repeating the same wasted work every cycle;
- swaps each partition in with **one short DuckDB transaction**: a `DELETE` over
  the partition column retires the old files (the predicate covers every row of
  every file, so DuckLake drops whole files instead of writing row-level delete
  files), then `ducklake_add_data_files` registers the merged output. DuckLake
  allocates the snapshot and the file ids;
- leaves physical removal of superseded files to the normal
  `ducklake_expire_snapshots` / `ducklake_cleanup_old_files` /
  `ducklake_delete_orphaned_files` calls, which honour
  `snapshot_expire_interval_sec`. The compactor never deletes parquet itself, so
  it cannot pull a file out from under a search node that still references it.

Because retirement is per partition, coverage is all-or-nothing: every file in
the partition is rewritten, or the partition is skipped. A partition is skipped
when rewriting its already-large files would cost more bytes than consolidating
the small ones gains, which lets small files accumulate until the merge is worth
it rather than rewriting a 512MB file every cycle.

Safety checks per partition, all inside the swap transaction: the partition's
live row count must equal the merged row count both **before** and **after** the
swap. Any mismatch — a concurrent flush, or rows still inlined in the catalog —
rolls the transaction back and defers the partition to the next cycle, leaving
the catalog exactly as it was. Tables with active row-level delete files, tables
not partitioned by a single identity column, and lakes with un-flushed inlined
rows are skipped entirely.

Partitions are skipped individually, so one bad partition never stops the rest of
a table: a partition is left alone when it holds row-level deletes, when it was
written to more recently than `min_age_sec`, or when a source row group exceeds
`max_row_group_bytes`.

Two schema-level refusals are permanent for the life of the process, because
retrying them can only repeat the same discarded work:

- a column type that the parquet -> Arrow -> parquet round trip cannot reproduce;
- a column type DuckLake will not accept from an added file at all. Some DuckDB
  types have no exact parquet form — `HUGEINT` is stored as `DOUBLE` — and
  `ducklake_add_data_files` then rejects the file against the table's declared
  type, even though DuckLake wrote the sources the same way. Homer's schema uses
  only `VARCHAR`, `UINTEGER`, `BIGINT`, `DOUBLE`, `TIMESTAMP`, `DATE` and `JSON`,
  all of which round-trip; this guards a column added later.

**Retention interacts with this.** `runRetention` deletes on `timestamp` while
tables are partitioned by `date`, so a cutoff inside a day removes only part of a
file and DuckLake records a **row-level delete file**. Since the cutoff is `now`
minus the window it almost never falls on midnight, and with `retention_unit:
hours` it essentially never does, so expect one such partition per table with
retention on. Only that partition is skipped — every other partition still
compacts — and it becomes eligible again once the cutoff moves past it.

Additionally, each native cycle takes a `VACUUM INTO` copy of the catalog first
(keeping the last 3 as `<catalog>.bak-<timestamp>`), and verifies afterwards that
the catalog still has exactly one latest snapshot and no duplicate `snapshot_id`.
If that check fails the native engine **switches itself off** until the process
restarts and falls back to the DuckDB merge. To take an extra snapshot or rewind
metadata from one of those copies, see [Catalog backup and restore](CATALOG.md):

```bash
homer catalog backup --config-path /etc/homer/config.json
# stop the writer, then:
homer catalog restore --config-path /etc/homer/config.json
```

### Configuration

The default is `duckdb`; the native engine is **opt-in**. It requires a local
`data_path` and a DuckLake build that provides `ducklake_add_data_files` — if
either is missing, the writer logs the reason and transparently uses the DuckDB
merge. A `data_path` containing a `key=value` directory component is also
rejected, because DuckLake infers partition values from an added file's whole
path and would read that component as a column the table does not have.

```json
{
  "storage": {
    "ducklake": {
      "catalog_path": "/data/homer/homer_catalog.sqlite",
      "data_path": "/data/homer/parquet",
      "compaction": {
        "enable": true,
        "engine": "native",
        "target_file_size_bytes": 536870912,
        "max_row_group_bytes": 268435456,
        "min_age_sec": 300
      }
    }
  }
}
```

Leaving `engine` unset (or `"duckdb"`) uses the DuckLake merge.

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
| Scope | duplicate `ducklake_snapshot`/`ducklake_table` rows (the common case); also **detects** (does not auto-fix) duplicate table names and non-INTEGER `ducklake_data_file.table_id` values ([#900](https://github.com/sipcapture/homer/issues/900)) | catalog unreadable / desynced beyond duplicate rows |
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

**If a recent catalog snapshot is still valid** (compaction or `homer catalog backup`
wrote a `.bak-*` copy before the bad cycle), stop the writer and restore it.
See [Catalog backup and restore](CATALOG.md).

```bash
homer catalog list --config-path /etc/homer/config.json
homer catalog restore --config-path /etc/homer/config.json --from homer_catalog.sqlite.bak-YYYYMMDDTHHMMSSZ
```

**If the autofix cannot recover it** and there is no usable backup (e.g. corruption
beyond duplicate metadata rows), rebuild the catalog from disk. Stop the Homer
writer, then run:

```bash
homer system --config-path /etc/homer/config.json --rebuild-catalog
```

This backs up the existing catalog to `*.corrupt.<timestamp>`, attaches a fresh
empty catalog, and **registers every on-disk parquet file in place** via
`ducklake_add_data_files` (so all snapshot/file ids are allocated by DuckLake and
the result is consistent). The files are not read, decompressed or rewritten, so
the rebuild is fast, needs almost no memory, and — crucially — **keeps the
original parquet files** instead of replacing them with fresh copies. There are
therefore no orphaned originals to reclaim afterwards.

The `--rebuild-cleanup-orphans` flag now only sweeps genuinely unreferenced
leftover files (e.g. half-written files); the registered originals are always
kept:

```bash
homer system --config-path /etc/homer/config.json --rebuild-catalog --rebuild-cleanup-orphans
```

#### Symptom: `Mismatch Type Error` on `table_id` ([#900](https://github.com/sipcapture/homer/issues/900))

If lake queries fail with:

```text
Mismatch Type Error: Failed to get data file list from DuckLake:
Invalid type in column "table_id": column was declared as integer,
found "date=YYYY-MM-DD/ducklake-….parquet" of type "text" instead.
```

at least one `ducklake_data_file` row has a **parquet path string stored in the
integer `table_id` column**. Startup auto-repair **detects** this and logs an
error pointing at `--rebuild-catalog`; it does **not** rewrite those rows
(doing so safely requires re-registering files). Confirm with:

```sql
SELECT rowid, typeof(table_id), table_id, path
FROM ducklake_data_file
WHERE typeof(table_id) != 'integer'
LIMIT 50;
```

Then stop the writer and run `--rebuild-catalog` as above. This is catalog
corruption, not a cross-table `date=` glob leak on the query path.

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

Then set `engine` to `duckdb` (or remove it) and restart. A native run also
leaves `<catalog>.bak-<timestamp>` copies; restoring the newest one from before
the run is the fastest recovery.

Requirements and behavior:

- **Safe with a live writer** — the compactor never writes the catalog itself.
  DuckLake allocates every snapshot and file id inside the swap transaction, so
  there is no id the writer's cache could miss and no `DETACH` to fail.
- **Local storage** — for remote (`s3://`) `data_path` the writer **automatically
  falls back to the `duckdb` engine**, so compaction always runs.
- **Partitioned, append-only tables only** — tables with active row-level delete
  files, tables not partitioned by a single identity column, and lakes with
  un-flushed inlined rows are skipped. HEP ingest is append-only and partitioned
  by `date`, so Homer's tables qualify.
- Holds the `CatalogLock` for the fast metadata reads that plan a cycle and for
  the short per-partition swap transaction, never during the slow merge, so
  flush/ingest stays responsive. Reading the catalog without the lock would make
  both the flush and the compactor fail with SQLite's `database is locked`.
- **Column logical types are preserved.** The merge goes through Arrow, which
  reads parquet `JSON` as plain `binary` and writes it back unannotated. Homer's
  `data_extra` is `JSON`, so the merged file was rejected with `Expected type
  "JSON" but found type "BLOB"`. Such columns are re-declared with the
  `arrow.json` extension type on write, which restores the annotation.
- If a flush commits into a partition while it is being merged, the swap's row
  count no longer matches and the partition is **deferred** to the next cycle
  rather than retired, so no row can be dropped. Because a partition is only busy
  while ingest targets it, and Homer partitions by `date`, this only delays the
  current day; older partitions compact on the first attempt.

Memory profile: bounded by one row group regardless of partition size.
`TestScaleMergeMemoryIsBounded` shows the independence — growing the partition 8x
(97MB -> 779MB, 1.6M -> 13.1M rows) moved peak Go heap from 870MB to 1046MB — but
independence from partition size is not the same as being small. A row group is
sized in rows, so its bytes scale with row width: real Homer files with 1.5GB row
groups took ~10GB RSS to merge (5.3GB at `GOGC=20`, 3.7GB at
`GOMEMLIMIT=2GiB`, at twice the wall time). `max_row_group_bytes` therefore caps
what will be attempted, and this heap is Arrow's, outside DuckDB's
`memory_limit`, so size the container for both. The retirement `DELETE` reads the
partition to confirm the files are fully covered, and runs with `threads = 1` to
keep that scan's memory bounded too.

> Note: the search node picks up the new snapshot on its next periodic catalog
> refresh, exactly as it does for a normal writer flush.

## Notes

- Runtime config changes do **not** require recompilation.
- Recompile only when changing Homer source code itself.
