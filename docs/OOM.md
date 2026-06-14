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

## 6) Native Go Compaction Engine (OOM-free merge)

The default DuckDB merge (`ducklake_merge_adjacent_files`) loads a whole
partition into memory to sort/rewrite it. On wide SIP data (large `payload`
columns) this can exceed `memory_limit` and OOM even for a handful of ~77MB
files, because the parquet write buffers are not spillable.

The `native_go` engine avoids this entirely. It does **not** use DuckDB for
compaction. Instead it:

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

### Enable it

```json
{
  "storage": {
    "ducklake": {
      "catalog_path": "/data/homer/homer_catalog.sqlite",
      "data_path": "/data/homer/parquet",
      "compaction": {
        "enable": true,
        "engine": "native_go",
        "target_file_size_bytes": 536870912
      }
    }
  }
}
```

Requirements and limits:

- **Local storage only** — `data_path` must be a local filesystem (not `s3://`).
- **Absolute `catalog_path`** — the compactor opens the SQLite file directly.
- **Append-only tables only** — tables with delete files are skipped (the engine
  reassigns row ids, which is only safe without positional deletes). HEP ingest
  is append-only, so this always holds for Homer.
- Runs under the same `CatalogLock` as flush, so it never races a DuckDB write.
- `engine` defaults to `duckdb`; set `native_go` to opt in.

Memory profile: bounded by one row group regardless of partition size, so a
512MB target safely compacts 76×77MB files on a writer capped well under the
multi-GB working set the DuckDB merge required.

> Note: after a native commit, the search node picks up the new snapshot on its
> next periodic catalog refresh.

## Notes

- Runtime config changes do **not** require recompilation.
- Recompile only when changing Homer source code itself.
