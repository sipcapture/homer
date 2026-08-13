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
- The same knobs are honoured on both sides:
  - writer connection — opened in `src/storage/ducklake/ducklake.go`,
    reads `storage.ducklake.tuning`.
  - node connection — opened in `src/node/node.go`,
    reads `node.ducklake.tuning`.

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
