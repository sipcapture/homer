# DuckDB engine tuning

*Shipped in `11.0.74`. Source: `src/storage/ducklake/tuning.go`,
`src/config/config.go` (`DuckDBTuning`).*

`homer-core` embeds DuckDB as the query engine on both the writer
(ingest) and the node (FlightSQL reader) sides. By default DuckDB
sizes its buffer pool at 80% of host RAM and uses `NumCPU` worker
threads, which is fine for a dedicated box but is dangerous in shared
deployments (containers, multi-tenant nodes, K8s with limits).

`11.0.74` adds a tuning section that maps directly to the
per-connection DuckDB SET statements:

```jsonc
{
  "storage": {
    "ducklake": {
      "tuning": {
        "memory_limit": "8GB",                  // empty = DuckDB default
        "threads": 4,                           // 0 = DuckDB default (NumCPU)
        "temp_directory": "/var/lib/homer/spill" // empty = DuckDB default
      }
    }
  },
  "node": {
    "ducklake": {
      "tuning": {
        "memory_limit": "4GB",
        "threads": 2
      }
    }
  }
}
```

## Behaviour

- Empty / zero values apply Homer defaults (not DuckDB's 80%-of-RAM
  built-in): `threads` = `min(NumCPU/2, 4)`, `memory_limit` = `2GB`,
  `temp_directory` = `<catalog dir>/.duckdb_spill`. Set explicit values
  to override. The Docker image sets `HOMER_STORAGE_DUCKLAKE_TUNING_*`
  (and node spill/threads) so the generic container is not stuck on
  the 2GB floor — see [OOM.md](OOM.md#docker-image-ghcriosipcapturehomer).
- Tuning is applied **before** `LOAD ducklake` and the catalog ATTACH
  on every fresh connection, so the limits are in effect during
  startup too.
- A bad value (e.g. `"memory_limit": "8 angstrom"`) only logs a WARN
  and leaves DuckDB on its default. The writer / node still come up.
- The same knobs are honoured on every in-memory DuckDB:
  - writer connection — opened in `src/storage/ducklake/ducklake.go`,
    reads `storage.ducklake.tuning`.
  - **tiering** connection — a **separate** DuckDB opened by
    `TieredStorageManager` (`src/storage/ducklake/tiered_storage.go`).
    It does not share the writer handle. Same `storage.ducklake.tuning`
    values. Without `temp_directory`, DuckDB stages parquet as `cwd/.tmp`
    and the official image (non-root, previously `WORKDIR /`) fails with
    `Failed to create directory ".tmp": Permission denied`
    ([#1020](https://github.com/sipcapture/homer/issues/1020)).
  - node connection — opened in `src/node/node.go`,
    reads `node.ducklake.tuning`.

## External file cache

`tuning.external_file_cache` (on both `storage.ducklake` and
`node.ducklake`) maps to DuckDB's instance-global
`enable_external_file_cache`, an in-memory cache of parquet byte ranges.

- Not set (default): the cache is **off** when every lake path of the
  DuckDB instance is local, and left at DuckDB's default (on) when any
  path is `s3://` or `az://`. Each instance decides on its own paths:
  the writer on `data_path`, the tiering instances on their volumes,
  the node on `node.ducklake.volumes`.
- `true` / `false`: force the setting on that instance.
- Env: `HOMER_STORAGE_DUCKLAKE_TUNING_EXTERNAL_FILE_CACHE`,
  `HOMER_NODE_DUCKLAKE_TUNING_EXTERNAL_FILE_CACHE`.

DuckLake opens its data files with `validate_external_file_cache=false`,
so a bad byte range in the cache is served until the process restarts.
Searches then fail on files that are valid on disk with
`No magic bytes found at end of file`
([#1061](https://github.com/sipcapture/homer/issues/1061)) or
`TProtocolException: Invalid data`
([#1076](https://github.com/sipcapture/homer/issues/1076)). Local files
are already held by the OS page cache, and the DuckDB cache takes its
memory from `memory_limit`, which flush, compaction and search share.
The startup log shows the decision:
`DuckDB tuning: external file cache set ... enable_external_file_cache=false reason="local data_path"`.

## Recommended values per role

| Role            | `memory_limit`       | `threads`              | `temp_directory`              |
|-----------------|----------------------|------------------------|-------------------------------|
| Ingest writer   | 50–60% of container  | `min(NumCPU, 8)`       | Fast SSD volume (≠ catalog)   |
| Read-only node  | 50–60% of container  | `min(NumCPU, 4)`       | Fast SSD volume               |
| Single-node lab | leave empty          | leave 0                | leave empty                   |

## Why "before extension load"?

DuckLake's `INSTALL` / `LOAD` and the initial `ATTACH` may allocate
substantial buffers (catalog scan, schema cache). Setting
`memory_limit` after these run can leave the engine briefly above the
limit, which will then trigger the next allocation to spill to disk
unnecessarily. Issuing the SET first avoids that footgun.

## Related metrics

These knobs do not yet expose Prometheus gauges in `homer-core`
(unlike `homer-lake`'s `homer_lake_duckdb_memory_*`); the
recommendation today is to:

1. Watch container-level `cgroup` memory metrics — the limits set here
   should keep DuckDB well below the container cap.
2. Watch the spill directory size — non-zero growth means the
   workload exceeds `memory_limit` and the engine is using disk.
