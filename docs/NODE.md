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
| `name` | string | - | Volume name (hot, cold, default) |
| `type` | string | "local" | Storage type: "local" or "s3" |
| `catalog_type` | string | "sqlite" | DuckLake catalog — sqlite |
| `catalog_path` | string | - | Path to catalog file |
| `path` | string | - | Data path (local or s3://) |
| `s3_region` | string | - | AWS region (for S3) |
| `s3_access_key_id` | string | - | AWS Access Key ID |
| `s3_secret_access_key` | string | - | AWS Secret Access Key |
| `s3_endpoint` | string | - | Custom S3 endpoint (MinIO, R2) |
| `s3_use_ssl` | bool | true | Use SSL for S3 connections |

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

**Important:** Storage and Node must use the same volumes with identical `catalog_path` for each volume.

## Example Configurations

See examples in the `examples/` directory:

- `homer-node.json` - Node only (read-only)
- `homer-writer.json` - Ingest + Storage + Node (combined)
- `homer-writer-rustfs.json` - Ingest + Storage + Node with S3 (RustFS/MinIO)
- `homer.json` - All-in-one (Ingest + Storage + Node + Coordinator)
