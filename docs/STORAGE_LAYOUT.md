# Homer Server Storage Layout

Documentation for on-disk data storage structure.

## Overview

Homer Server uses **DuckLake** — a lakehouse format that combines:
- **Parquet files** for data storage
- **SQLite catalog** for metadata (snapshots, schemas, file statistics)
- **Hive-style partitioning** for efficient time-range queries

## Directory Structure

```
/data/homer/
├── homer_catalog.sqlite          # DuckLake catalog (metadata)
└── parquet/                    # Data directory
    └── main/                   # Schema "main"
        ├── hep_proto_1_call/           # SIP calls (INVITE, BYE, etc.)
        │   ├── date=2025-01-26/
        │   │   ├── data_00001.parquet
        │   │   └── data_00002.parquet
        │   └── date=2025-01-27/
        │       ├── data_00003.parquet
        │       └── data_00004.parquet
        │
        ├── hep_proto_1_registration/   # SIP registrations (REGISTER)
        │   └── date=2025-01-27/
        │       └── data_00001.parquet
        │
        ├── hep_proto_1_default/        # Other SIP (OPTIONS, NOTIFY, etc.)
        ├── hep_proto_5_default/        # RTCP JSON
        ├── hep_proto_34_default/       # RTCP binary
        ├── hep_proto_35_default/       # RTP
        ├── hep_proto_53_default/       # DNS
        └── hep_proto_100_default/      # Logs
```

## Catalog (homer_catalog.sqlite)

SQLite database containing:

| Table | Description |
|-------|-------------|
| `ducklake_table` | List of tables |
| `ducklake_column` | Column schemas |
| `ducklake_snapshot` | Snapshot history |
| `ducklake_data_file` | List of Parquet files |
| `ducklake_file_column_stats` | Min/max column statistics |
| `ducklake_partition_column` | Partitioning settings |
| `ducklake_partition_info` | Partition values in files |

### Catalog Contents Example

```sql
-- Tables
SELECT table_name FROM ducklake_table;
-- hep_proto_1_call
-- hep_proto_1_registration
-- hep_proto_5_default
-- ...

-- Files
SELECT file_path, row_count FROM ducklake_data_file WHERE table_id = 1;
-- main/hep_proto_1_call/date=2025-01-27/hour=14/data_00001.parquet, 50000

-- Partitioning
SELECT column_name, partition_type FROM ducklake_partition_column WHERE table_id = 1;
-- date, identity
```

## Tables by Protocol Type

### SIP (proto_type=1)

SIP messages are split into three tables by transaction type:

| Table | Methods | Description |
|-------|---------|-------------|
| `hep_proto_1_call` | INVITE, ACK, PRACK, UPDATE, BYE, CANCEL, INFO, REFER | Call messages |
| `hep_proto_1_registration` | REGISTER | Registrations |
| `hep_proto_1_default` | OPTIONS, NOTIFY, SUBSCRIBE, PUBLISH, MESSAGE | Other |
| `hep_proto_1_siprec` | SIPREC SRS signaling (in-process receiver) | Recording metadata + SIP |

**Important**: SIP responses (200 OK, 180 Ringing) are routed by the method from `CSeq` header.

### Other Protocols

| Proto Type | Table | Description |
|------------|-------|-------------|
| 5 | `hep_proto_5_default` | RTCP JSON (quality statistics) |
| 34 | `hep_proto_34_default` | RTCP binary |
| 35 | `hep_proto_35_default` | RTP packets |
| 53 | `hep_proto_53_default` | DNS queries |
| 100 | `hep_proto_100_default` | Logs |

### Dedicated ingest tables

| Table | Source | Description |
|-------|--------|-------------|
| `vqrtcpxr_stats` | VQRTCP SIP receiver (`ingest.vqrtcp`) | VQ-RTCPXR QoS reports (Call-ID correlated) |
| `otlp_traces` / `otlp_metrics` / `otlp_logs` | OTLP receiver | OpenTelemetry signals |

See [VQRTCP.md](VQRTCP.md) and [SIPREC.md](SIPREC.md).

## Partitioning

### Partitioning Scheme

```sql
ALTER TABLE homer_lake.hep_proto_1_call 
SET PARTITIONED BY (date);
```

Creates structure:
```
date=2025-01-27/data.parquet
```

### Benefits

1. **Partition pruning** — queries with date filter scan only required directories
2. **Minimal nesting** — 1 level only (date)
3. **Readable paths** — date is immediately visible in directory name
4. **Simple retention** — easy to delete old data by removing date directories

### Query Optimization Example

```sql
-- DuckDB will automatically skip all partitions except date=2025-01-27
SELECT * FROM homer_lake.hep_proto_1_call
WHERE date = '2025-01-27'
  AND timestamp >= '2025-01-27 10:00:00'
  AND timestamp <  '2025-01-27 12:00:00';
```

## Column Schemas

### SIP Call (hep_proto_1_call)

```sql
uuid VARCHAR,           -- Unique record ID
date DATE,              -- Date (for partitioning)
timestamp TIMESTAMP,    -- Packet timestamp
session_id VARCHAR,     -- Call-ID
caller VARCHAR,         -- From user
callee VARCHAR,         -- To user
src_ip VARCHAR,         -- Source IP
dst_ip VARCHAR,         -- Destination IP
src_port UINTEGER,      -- Source port
dst_port UINTEGER,      -- Destination port
method VARCHAR,         -- SIP method (INVITE, BYE, etc.)
response_code VARCHAR,  -- Response code (200, 404, etc.)
cseq_method VARCHAR,    -- Method from CSeq header
protocol UINTEGER,      -- Transport (17=UDP, 6=TCP)
node_id VARCHAR,        -- HEP node ID
cid VARCHAR,            -- Correlation ID
payload VARCHAR,        -- Raw SIP message
data_extra JSON         -- Additional headers (via, user_agent, etc.)
```

### SIP Registration (hep_proto_1_registration)

```sql
uuid VARCHAR,
date DATE,
timestamp TIMESTAMP,
session_id VARCHAR,     -- Call-ID
aor VARCHAR,            -- Address of Record (user@domain)
contact VARCHAR,        -- Contact header
expires VARCHAR,        -- Expires value
user_agent VARCHAR,     -- User-Agent header
src_ip VARCHAR,
dst_ip VARCHAR,
src_port UINTEGER,
dst_port UINTEGER,
method VARCHAR,
response_code VARCHAR,
protocol UINTEGER,
node_id VARCHAR,
payload VARCHAR,
data_extra JSON
```

### Generic (RTCP, RTP, DNS, LOG)

```sql
uuid VARCHAR,
date DATE,
timestamp TIMESTAMP,
session_id VARCHAR,     -- Correlation ID
src_ip VARCHAR,
dst_ip VARCHAR,
src_port UINTEGER,
dst_port UINTEGER,
protocol UINTEGER,
node_id VARCHAR,
cid VARCHAR,
payload VARCHAR,        -- Raw payload / JSON
data_extra JSON         -- Metadata
```

## Parquet Files

### Format

- **Compression**: Snappy (DuckDB default)
- **Row groups**: ~100K rows
- **Column statistics**: Min/max for each column

### Sizes

Typical file sizes:
- SIP message: ~2-5 KB (with payload)
- RTCP JSON: ~500 bytes
- RTP: ~100-200 bytes (without payload)

With batch size 1000 and flush every 10 seconds:
- ~10-50 MB per file for SIP
- ~5-10 MB per file for RTCP

## Throughput and Compaction Tuning (Single Node)

Recommended baseline for ~10k packets/sec on local disk:
- `writer.worker_count`: 8 (or match CPU cores if higher)
- `writer.queue_size`: 80000-200000 (depends on RAM, smooths bursts)
- `ducklake.batch_size`: 50000 (target 128-256 MB Parquet files)
- `ducklake.flush_interval_sec`: 15-30 (controls latency and file size)
- `ducklake.compaction.check_interval_sec`: 900-1800
- `ducklake.compaction.snapshot_expire_interval_sec`: 3600-7200
- `ducklake.compaction.retention_days`: set to policy (e.g. 7/30) — see [Data retention](RETENTION.md)

Goal: fewer small files, predictable compaction cycles, stable write latency.
Adjust `batch_size` and `flush_interval_sec` together to hit desired file size.
If needed, tune these values while monitoring files/hour and target 128-256 MB per file.

## DuckLake Operations

### Reading Data

```sql
-- Via DuckDB CLI
duckdb -c "
INSTALL ducklake; LOAD ducklake;
ATTACH 'ducklake:sqlite:/data/homer/homer_catalog.sqlite' AS homer_lake 
  (DATA_PATH '/data/homer/parquet');

SELECT * FROM homer_lake.hep_proto_1_call
WHERE date = '2025-01-27'
LIMIT 10;
"
```

### Maintenance

```sql
-- Expire old snapshots
CALL ducklake_expire_snapshots('homer_lake', 
  older_than => CAST(NOW() - INTERVAL '1 hour' AS TIMESTAMPTZ));

-- Merge small files
CALL ducklake_merge_adjacent_files('homer_lake');

-- Delete files from expired snapshots
CALL ducklake_cleanup_old_files('homer_lake');

-- Delete orphan files
CALL ducklake_delete_orphaned_files('homer_lake');
```

### Time Travel

```sql
-- Query specific snapshot
SELECT * FROM homer_lake.hep_proto_1_call AT SNAPSHOT 5;

-- Query at specific time
SELECT * FROM homer_lake.hep_proto_1_call AT TIMESTAMP '2025-01-27 10:00:00';

-- List snapshots
SELECT * FROM ducklake_snapshots('homer_lake.hep_proto_1_call');
```

## Configuration

### Writer config (homer-core-writer.json)

```json
{
  "ducklake": {
    "catalog_path": "/data/homer/homer_catalog.sqlite",
    "data_path": "/data/homer/parquet",
    "lake_name": "homer_lake",
    "batch_size": 1000,
    "flush_interval": "10s"
  }
}
```

### Disk Recommendations

| Component | Recommendation |
|-----------|----------------|
| Catalog (SQLite) | SSD, low latency |
| Parquet data | HDD/SSD, capacity over speed |
| Temporary files | SSD for merge operations |

### Storage Estimation

At 1000 SIP msg/sec:
- ~86M messages/day
- ~200-500 GB/day (with payload)
- ~20-50 GB/day (without payload, metadata only)

## Modular Architecture

```
┌─────────────────────────────────────────────────────────────────────┐
│                         Homer Server                                 │
│                                                                      │
│  ┌──────────────┐    ┌──────────────┐    ┌──────────────────────┐   │
│  │    Writer    │    │     Node     │    │     Coordinator      │   │
│  │              │    │              │    │                      │   │
│  │ HEP receivers│    │ FlightSQL    │    │ REST API + UI        │   │
│  │ UDP/TCP/TLS  │    │ Server       │    │ FlightSQL Client     │   │
│  │              │    │              │    │                      │   │
│  │ DuckLake     │    │ DuckLake     │    │ Aggregates queries   │   │
│  │ Writer       │    │ Read-only    │    │ from multiple Nodes  │   │
│  └──────┬───────┘    └──────┬───────┘    └──────────────────────┘   │
│         │                   │                                        │
│         ▼                   ▼                                        │
│  ┌──────────────────────────────────────────────────────────────┐   │
│  │                     DuckLake Storage                          │   │
│  │                                                               │   │
│  │   SQLite Catalog ◄──────────────────► Parquet Files          │   │
│  │   (metadata)                          (data)                  │   │
│  │                                                               │   │
│  │   /data/homer/homer_catalog.sqlite      /data/homer/parquet/   │   │
│  └──────────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────────┘
```

## See Also

- [DuckLake README](../src/storage/ducklake/README.md) — DuckLake integration details
- [STORAGE_ARCHITECTURE.md](STORAGE_ARCHITECTURE.md) — General architecture
- [DuckLake Documentation](https://ducklake.select/docs/) — Official documentation
