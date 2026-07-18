# Node Module - Data Server (Airport + FlightSQL)

The Node module exposes **DuckDB Airport** on `flight_server` (Arrow Flight with PATH-style access) and optionally **Apache Arrow FlightSQL** on `flightsql_server` (Grafana InfluxDB / FlightSQL). Both share the same DuckDB engine and DuckLake-backed data.

## Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                        Homer Node                                │
│                                                                  │
│  DuckDB Airport (gRPC :50051)  ──┐                              │
│  Arrow FlightSQL (gRPC :50055) ──┼──▶  DuckDB + DuckLake        │
│  HTTP /query (port+1)          ──┘                              │
│                                                                  │
└───────────────────────────────┬────────────────────────────────┘
                                ▼
                       ┌────────────────┐
                       │ Parquet volumes │
                       └────────────────┘
```

## Key Features

- **DuckDB Airport** (`flight_server`) — Arrow Flight for DuckDB `ATTACH 'arrow_flight://…'` and the coordinator’s HTTP `/query` path
- **Apache FlightSQL** (`flightsql_server`, optional) — same SQL stack as HTTP queries, for Grafana FlightSQL / ADBC
- **Multi-volume support** - read from multiple storage backends (hot/cold)
- **Automatic UNION ALL** - transparent data merging from all volumes
- **Partition pruning** - DuckDB automatically skips partitions without matching data
- **HTTP API** - REST endpoints for queries and management

## Configuration

### Single Volume (Simple Case)

```json
{
  "node": {
    "enable": true,
    "flight_server": {
      "host": "0.0.0.0",
      "port": 50051,
      "auth_token": "your-secret-token",
      "max_message_size": 16777216
    },
    "flightsql_server": {
      "enable": false,
      "host": "0.0.0.0",
      "port": 50055,
      "auth_token": "",
      "catalog_refresh_interval_sec": 30
    },
    "ducklake": {
      "lake_name": "homer_lake",
      "volumes": [
        {
          "name": "default",
          "type": "local",
          "catalog_type": "sqlite",
          "catalog_path": "/data/homer/homer_catalog.sqlite",
          "path": "/data/homer/parquet"
        }
      ]
    }
  }
}
```

### Multiple Volumes (Tiered Storage)

```json
{
  "node": {
    "enable": true,
    "flight_server": {
      "host": "0.0.0.0",
      "port": 50051,
      "auth_token": "your-secret-token"
    },
    "ducklake": {
      "lake_name": "homer_lake",
      "volumes": [
        {
          "name": "hot",
          "type": "local",
          "catalog_type": "sqlite",
          "catalog_path": "/data/homer/homer_catalog_hot.sqlite",
          "path": "/data/homer/parquet"
        },
        {
          "name": "cold",
          "type": "s3",
          "catalog_type": "sqlite",
          "catalog_path": "/data/homer/homer_catalog_cold.sqlite",
          "path": "s3://homer-bucket/cold/",
          "s3_region": "us-east-1",
          "s3_access_key_id": "your-key",
          "s3_secret_access_key": "your-secret",
          "s3_endpoint": "https://s3.amazonaws.com",
          "s3_use_ssl": true
        }
      ]
    }
  }
}
```

## Configuration Parameters

### flight_server

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `host` | string | "0.0.0.0" | Listen address |
| `port` | int | 50051 | gRPC server port |
| `auth_token` | string | "" | Bearer token for authentication |
| `max_message_size` | int | 16777216 | Maximum message size (16MB) |

### flightsql_server (optional, Grafana FlightSQL)

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `enable` | bool | false | Listen for Apache Arrow FlightSQL (separate from Airport) |
| `host` | string | "0.0.0.0" | Listen address |
| `port` | int | 50055 | gRPC FlightSQL port |
| `auth_token` | string | "" | If set, require `Authorization: Bearer <token>` |
| `catalog_refresh_interval_sec` | int | 30 | Periodic `DETACH`/`ATTACH` refresh when not using a shared writer DB |

See [FLIGHTSQL.md](FLIGHTSQL.md) for Grafana setup and coordinator proxy (`coordinator.flightsql_server`, default port **32010**).

### ducklake

| Parameter | Type | Description |
|-----------|------|-------------|
| `lake_name` | string | Base DuckLake name (used as prefix) |
| `volumes` | array | Array of storage volumes |

### volume

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `name` | string | - | Volume label. Use **`"default"`** for the primary single-volume catalog (see below). Other labels (`hot`, `cold`, …) form the DuckDB catalog name together with `lake_name`. |
| `type` | string | "local" | Storage type: "local" or "s3" |
| `catalog_type` | string | "sqlite" | DuckLake catalog — sqlite |
| `catalog_path` | string | - | Path to catalog file |
| `path` | string | - | Data path (local or s3://) |
| `s3_region` | string | - | AWS region (for S3) |
| `s3_access_key_id` | string | - | AWS Access Key ID |
| `s3_secret_access_key` | string | - | AWS Secret Access Key |
| `s3_endpoint` | string | - | Custom S3 endpoint (MinIO, R2) |
| `s3_use_ssl` | bool | true | Use SSL for S3 connections |
| `s3_url_style` | string | path | DuckDB S3 URL style for custom endpoints (`path` or `vhost`) |
| `override_data_path` | bool | false | If true, DuckLake `ATTACH` uses `OVERRIDE_DATA_PATH TRUE` so `path` may differ from the `DATA_PATH` already stored in the catalog (e.g. bucket rename). Prefer keeping `path` identical to the writer volume that created the catalog. |

### Volume `name` and the DuckDB catalog

When Node attaches DuckLake, the volume’s **`name`** determines the **DuckDB catalog** identifier used in SQL (e.g. `homer_lake.main.hep_proto_1_call`):

- If **`name`** is **`"default"`**, the catalog is attached **as** `node.ducklake.lake_name` only (e.g. `homer_lake`). This matches the default single-catalog writer (`storage.ducklake.lake_name`).
- For any **other** `name`, the catalog is **`{lake_name}_{name}`** — the same pattern as in the [Multiple Volumes](#multiple-volumes-tiered-storage) example (`homer_lake_hot`, `homer_lake_cold`).

API rows may include a **`storage_volume`** column (your volume label). Qualified **`FROM`** clauses must use the **catalog** name, not the label alone; the coordinator typically emits `lake_name.main…`, and Node may rewrite it per volume.

For **tiered storage**, keep **`node.ducklake.volumes`** in sync with **`storage.ducklake.storage_policy.volumes`** (same `name` values and matching `catalog_path` / `path` per volume). See [STORAGE_POLICIES.md](STORAGE_POLICIES.md).

## How Multi-Volume Works

### UNION ALL Mechanism

When multiple volumes are configured, Node automatically rewrites incoming queries:

**Incoming query:**
```sql
SELECT * FROM homer_lake.main.sip_transactions 
WHERE timestamp BETWEEN '2025-01-01 10:00:00' AND '2025-01-01 12:00:00'
```

**Rewritten query:**
```sql
SELECT * FROM (
  SELECT * FROM homer_lake_hot.main.sip_transactions
  UNION ALL
  SELECT * FROM homer_lake_cold.main.sip_transactions
) 
WHERE timestamp BETWEEN '2025-01-01 10:00:00' AND '2025-01-01 12:00:00'
```

### Partition Pruning

DuckDB automatically optimizes UNION ALL queries through:

1. **Predicate pushdown** - WHERE conditions are applied to each subquery
2. **Partition pruning** - partitions not containing data in the requested timerange are skipped
3. **File statistics** - DuckLake stores min/max statistics for each file

This means if the data for the requested period exists only in the hot volume, the cold volume won't be scanned at all.

## HTTP API

### GET /api/v4/node/stats

Returns node statistics.

```json
{
  "running": true,
  "address": "0.0.0.0:50051",
  "volumes_count": 2,
  "volumes": ["hot", "cold"]
}
```

### POST /api/v4/node/query

Executes an SQL query.

**Request:**
```json
{
  "sql": "SELECT * FROM homer_lake.main.sip_transactions LIMIT 10"
}
```

**Response:**
```json
{
  "columns": ["timestamp", "call_id", "method", ...],
  "data": [...]
}
```

### POST /api/v4/node/vacuum

Runs vacuum on all volumes.

**Request:**
```json
{
  "expire_older_than": "7 days"
}
```

## FlightSQL Clients

### Python (pyarrow)

```python
from pyarrow.flight import FlightClient, FlightCallOptions

client = FlightClient("grpc://localhost:50051")
options = FlightCallOptions(headers=[("authorization", "Bearer your-token")])

info = client.get_flight_info(
    FlightDescriptor.for_command(b"SELECT * FROM homer_lake.main.sip_transactions LIMIT 10"),
    options
)

reader = client.do_get(info.endpoints[0].ticket, options)
table = reader.read_all()
print(table.to_pandas())
```

### DuckDB

```sql
INSTALL arrow;
LOAD arrow;

-- Connect to FlightSQL server
ATTACH 'arrow_flight://localhost:50051?auth_token=your-token' AS homer;

-- Query data
SELECT * FROM homer.homer_lake.main.sip_transactions LIMIT 10;
```

## Ingest, Storage and Node Integration

Ingest, Storage and Node modules work together:

```
┌────────────────────────────────────────────────────────────┐
│                       Homer Server                          │
│                                                             │
│  ┌─────────────┐   ┌─────────────┐   ┌─────────────┐       │
│  │   Ingest    │──▶│   Storage   │──▶│    Node     │       │
│  │ (HEP recv)  │   │ (DuckLake)  │   │ (FlightSQL) │       │
│  └─────────────┘   └─────────────┘   └─────────────┘       │
│   UDP/TCP/HTTP       Parquet write      gRPC:50051         │
│                            │                  │            │
│                            ▼                  ▼            │
│  ┌─────────────────────────────────────────────────┐       │
│  │              DuckLake Catalog                    │       │
│  │       (shared catalog, same volumes)             │       │
│  └─────────────────────────────────────────────────┘       │
│                            │                               │
│              ┌─────────────┴─────────────┐                 │
│              ▼                           ▼                 │
│       ┌─────────────┐             ┌─────────────┐          │
│       │ Hot Volume  │             │ Cold Volume │          │
│       │ (local SSD) │             │ (S3/R2)     │          │
│       └─────────────┘             └─────────────┘          │
└────────────────────────────────────────────────────────────┘
```

**Important:** Storage and Node must use the same volumes with identical `catalog_path` (and data `path`) for each volume. For a **single** logical DuckLake that the writer owns, the node volume must point at the **same** catalog and Parquet tree as `storage.ducklake`.

### Writer + Node (all-in-one process)

When **Ingest**, **Storage**, and **Node** are enabled together, Node uses the **writer’s DuckDB connection** for queries so in-memory buffer tables and freshly flushed Parquet stay visible without a second attach.

- The SQL engine then only sees catalogs the **writer** actually attached — for the default (non–storage-policy) writer that is normally **`storage.ducklake.lake_name`** (e.g. `homer_lake`).
- Set **`node.ducklake.lake_name`** to the **same** value as **`storage.ducklake.lake_name`**, and align **`catalog_path`** / **`path`** on the single volume with storage.
- Using **`"name": "default"`** for that single volume keeps Node’s **own** DuckDB attach (used for catalog refresh when not sharing the writer DB) consistent with the writer catalog name.

Current code also rewrites coordinator SQL to use **`node.ducklake.lake_name`** when executing on the shared writer connection with **one** volume (`duckLakeCatalogForQuery` in [`src/node/node.go`](../src/node/node.go)), which avoids Binder errors if `name` is not `default`. Path and `lake_name` alignment with storage remains mandatory.

With **multi-volume** `node.ducklake.volumes` and a writer **storage_policy**, the Node’s HTTP **`/query`** path (coordinator → local node) **merges** results from the writer DuckDB (`homer_lake` + memory buffer) and the writer’s **TieredStorageManager** DuckDB (`homer_lake_hot` / `homer_lake_cold` UNION), so cold-tier data is visible in search. Apache Arrow FlightSQL still uses the writer connection only until extended similarly.

## Troubleshooting

| Symptom | Likely cause |
|---------|----------------|
| `Binder Error: Catalog "homer_lake_…" does not exist` on search / API queries | Rewritten SQL referenced a suffixed catalog (from `volumes[].name`) that is not attached on the **connection** running the query — e.g. all-in-one with shared writer DB while the node volume used a non-`default` label. Fix: same `lake_name` and paths as storage, mirror tiered volume names with [STORAGE_POLICIES.md](STORAGE_POLICIES.md), use `"name": "default"` for the single primary volume, or use a build that includes shared-DB single-volume catalog normalization (see `duckLakeCatalogForQuery` in [`src/node/node.go`](../src/node/node.go)). |
| `no volumes configured in node.ducklake.volumes` | Node requires an explicit `volumes` array. Prefer the layout in [examples/homer.json](../examples/homer.json). Older wizard configs that only set `catalog_path` / `data_path` are accepted at load time: Homer synthesizes a single `default` volume from those paths (`EnsureNodeDuckLakeVolumes`). Regenerate with `homer wizard` or copy `volumes` from the example. |
| `DATA_PATH … does not match existing data path in the catalog` on Node attach | The S3/local `path` for a volume must match the path recorded when the catalog was first created (usually by the writer). Fix: align `path` with `storage.ducklake.storage_policy.volumes` for that volume, or set **`override_data_path": true`** on that volume (DuckLake `OVERRIDE_DATA_PATH`) if you intentionally moved data or renamed the bucket prefix. |

## Example Configurations

See examples in the `examples/` directory:

- `homer-node.json` - Node only (read-only)
- `homer-writer.json` - Ingest + Storage + Node (combined)
- `homer-writer-rustfs.json` - Ingest + Storage + Node with S3 (RustFS/MinIO)
- `homer.json` - All-in-one (Ingest + Storage + Node + Coordinator)
