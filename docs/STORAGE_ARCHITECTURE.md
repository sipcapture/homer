# Homer Server Storage Architecture

## Overview

Homer Server uses **DuckLake** for HEP packet storage. DuckLake is a lakehouse format that combines Parquet files with a SQL catalog database, providing:

- **Parquet files** for efficient columnar storage
- **DuckLake catalog** — sqlite
- **Time travel** queries and snapshots
- **ACID transactions** for data integrity

Parquet data may live on local disk, S3, or Azure Blob Storage (`data_path`).

## Architecture

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                           Homer Server                                       │
│                                                                             │
│  HEP Packets ──► Ingest ──► Storage (DuckLake) ──► Parquet Files           │
│       │                          │                                          │
│       │                          ▼                                          │
│       │                   DuckLake Catalog                                  │
│       │         ┌─────────────────────────────────────────────┐            │
│       │         │  ┌───────────┐  ┌───────────┐  ┌─────────┐ │            │
│       │         │  │ Snapshots │  │ File List │  │ Schema  │ │            │
│       │         │  └───────────┘  └───────────┘  └─────────┘ │            │
│       │         └─────────────────────────────────────────────┘            │
│       │         (DuckLake catalog — sqlite)                                  │
│       │                                                                     │
│       ▼                                                                     │
│  Metadata API ◄─────── Homer Hub (Smart Router)                            │
│  /api/v1/metadata/check                                                    │
│  /api/v1/metadata/stats                                                    │
└─────────────────────────────────────────────────────────────────────────────┘
```

## Data Flow

### 1. Ingestion

```
HEP Packet → Ingest → Storage → Buffer → Parquet File
             (decode)  (write)     │
                                   ▼
                             DuckLake Catalog
                             (tracks files, snapshots)
```

### 2. Query (via DuckDB)

```
DuckDB Query Engine
        │
        ├── Attach DuckLake catalog
        │
        ├── Query files using Parquet min/max statistics
        │
        └── Return results (supports time travel)
```

## Catalog

DuckLake catalog — sqlite. Scale Parquet storage with `data_path` on local disk, S3 (`s3` block), or Azure Blob Storage (`azure` block), or tiered volumes under `storage_policy`.

## Configuration

### Basic (SQLite catalog + local Parquet)

```json
{
  "storage": {
    "enable": true,
    "ducklake": {
      "catalog_type": "sqlite",
      "catalog_path": "/data/homer_catalog.sqlite",
      "data_path": "/data/homer_parquet",
      "batch_size": 10000,
      "flush_interval_sec": 30
    }
  }
}
```

### SQLite catalog + S3 Parquet data

The catalog stays on disk (SQLite); only Parquet objects are stored in the bucket:

```json
{
  "storage": {
    "enable": true,
    "ducklake": {
      "catalog_type": "sqlite",
      "catalog_path": "/data/homer_catalog.sqlite",
      "data_path": "s3://my-bucket/homer-parquet/",
      "s3": {
        "region": "us-east-1",
        "access_key_id": "AKIA...",
        "secret_access_key": "..."
      }
    }
  }
}
```

### SQLite catalog + Azure Blob Storage Parquet data

The catalog stays on disk (SQLite); only Parquet objects are stored in the container. With no `account_key`/`connection_string` set, Homer authenticates via DuckDB's Azure credential chain — this resolves **Managed Identity** automatically when running on an Azure VM:

```json
{
  "storage": {
    "enable": true,
    "ducklake": {
      "catalog_type": "sqlite",
      "catalog_path": "/data/homer_catalog.sqlite",
      "data_path": "az://my-container/homer-parquet/",
      "azure": {
        "account_name": "homerstorage"
      }
    }
  }
}
```

To use a static account key instead, add `"account_key": "..."` to the `azure` block (or a full `"connection_string"`, which takes precedence over `account_key`).

## Time Travel

DuckLake supports querying data at any point in time:

```sql
-- Current data
SELECT * FROM homer_lake.hep_messages 
WHERE session_id = 'abc123@host';

-- Data at specific snapshot
SELECT * FROM homer_lake.hep_messages AT SNAPSHOT 4
WHERE session_id = 'abc123@host';

-- Data at specific time
SELECT * FROM homer_lake.hep_messages AT TIMESTAMP '2025-01-23 09:00:00'
WHERE session_id = 'abc123@host';
```

## API Endpoints

### Check Time Range (Smart Routing)

```bash
POST /api/v1/metadata/check
Content-Type: application/json

{
  "min_ts": 1737590400000000000,
  "max_ts": 1737676799999999999
}
```

Response:
```json
{
  "has_data": true,
  "node_min_ts": 1737504000000000000,
  "node_max_ts": 1737676799999999999,
  "oldest_data": "2025-01-22T00:00:00Z",
  "newest_data": "2025-01-23T23:59:59Z",
  "data_span_hours": 48
}
```

### Get Statistics

```bash
GET /api/v1/metadata/stats
```

Response:
```json
{
  "row_count": 5000000,
  "min_timestamp": 1737504000000000000,
  "max_timestamp": 1737676799999999999,
  "oldest_data": "2025-01-22T00:00:00Z",
  "newest_data": "2025-01-23T23:59:59Z",
  "catalog_type": "sqlite",
  "data_path": "/data/homer_parquet"
}
```

## Smart Routing (Homer Hub)

Homer Hub uses the metadata API to route queries to appropriate nodes:

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                            Homer Hub                                         │
│                                                                             │
│  Query (time range: last 24h)                                              │
│         │                                                                   │
│         ▼                                                                   │
│  ┌─────────────────────────────────────────────────────────────┐           │
│  │  Smart Router                                                │           │
│  │                                                              │           │
│  │  Node A: 0-24h (hot)    ─────► has_data: true  ───► QUERY  │           │
│  │  Node B: 7-14 days (archive) ─► has_data: false ───► SKIP  │           │
│  │  Node C: 0-48h          ─────► has_data: true  ───► QUERY  │           │
│  └─────────────────────────────────────────────────────────────┘           │
│                                                                             │
│  Result: Query only Node A and Node C                                       │
└─────────────────────────────────────────────────────────────────────────────┘
```

## File Structure

```
src/storage/
└── ducklake/
    ├── ducklake.go      # Core Writer with batch writes
    ├── timetravel.go    # Time travel and snapshot queries
    ├── hep_adapter.go   # HEP → DuckLake record conversion
    ├── manager.go       # High-level manager
    ├── api.go           # HTTP API handlers
    └── README.md        # Package documentation
```

## Performance

- **Batch writes**: Records buffered before writing (default 10,000)
- **Parquet statistics**: DuckDB uses min/max stats for query pruning
- **Columnar storage**: Efficient compression and column projection
- **Parallel reads**: DuckDB parallelizes Parquet file reads

## License

AGPL-3.0 License - QXIP / SIPCapture Team
