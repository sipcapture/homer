# DuckLake Storage for Homer

DuckLake provides lakehouse-style storage for HEP packets with advanced features like time travel, snapshots, and ACID transactions.

## Overview

DuckLake is an alternative storage backend to the standard Arrow/Parquet storage. It uses the [DuckLake format](https://ducklake.select/) which combines:

- **Parquet files** for data storage
- **SQL catalog database** for metadata (DuckDB, PostgreSQL, MySQL, or SQLite)
- **Multi-table architecture** - separate tables per protocol type

## Features

| Feature | Arrow Storage | DuckLake Storage |
|---------|--------------|------------------|
| Parquet files | ✅ | ✅ |
| Time travel | ❌ | ✅ |
| Snapshots | ❌ | ✅ |
| ACID transactions | ❌ | ✅ |
| Multi-writer support | ❌ | ✅ (with PostgreSQL/MySQL catalog) |
| Schema evolution | Manual | ✅ Automatic |
| S3 storage | Custom code | ✅ Built-in |
| Protocol-specific tables | ❌ | ✅ |

## Multi-Table Architecture

HEP packets are automatically routed to separate tables based on `proto_type` and SIP method:

### SIP Tables (proto_type=1)

SIP messages are further split by method type:

| Table Name | Methods | Description |
|------------|---------|-------------|
| `hep_proto_1_call` | INVITE, ACK, PRACK, UPDATE, BYE, CANCEL, INFO | Call-related messages |
| `hep_proto_1_registration` | REGISTER | Registration messages |
| `hep_proto_1_default` | OPTIONS, NOTIFY, SUBSCRIBE, PUBLISH, MESSAGE, REFER | Other SIP messages |

**Important**: For SIP responses (e.g., "200 OK"), routing is based on `CSeq` method, not the response line.

### Other Protocol Tables

| Proto Type | Table Name | Description |
|------------|------------|-------------|
| 5 | `hep_proto_5_default` | RTCP JSON reports |
| 34 | `hep_proto_34_default` | RTCP binary |
| 35 | `hep_proto_35_default` | RTP |
| 53 | `hep_proto_53_default` | DNS |
| 100 | `hep_proto_100_default` | Logs |
| N | `hep_proto_N_default` | Other protocols |

### Time-Based Partitioning

All tables are automatically partitioned by `date`:

```sql
ALTER TABLE homer_lake.hep_proto_1_call 
SET PARTITIONED BY (date);
```

**Benefits:**
- **File pruning** - queries with date filter only scan relevant Parquet files
- **Query performance** - 10-100x faster for time-bounded queries
- **Hive-style paths** - files organized as `date=2025-01-27/`
- **Progressive** - only affects new data, existing data remains in place

**Example file layout:**
```
/data/homer/parquet/main/
└── hep_proto_1_call/
    ├── date=2025-01-26/
    │   ├── data_00001.parquet
    │   └── data_00002.parquet
    └── date=2025-01-27/
        ├── data_00003.parquet
        └── data_00004.parquet
```

**Example query (uses partition pruning):**
```sql
SELECT * FROM homer_lake.hep_proto_1_call
WHERE date = '2025-01-27'
  AND timestamp >= '2025-01-27 10:00:00'
  AND timestamp <  '2025-01-27 12:00:00';
```

### Table Schemas

**SIP Calls (proto_type=1, sub_type=call)** - optimized for call correlation:
```sql
uuid, timestamp, session_id, caller, callee, src_ip, dst_ip,
src_port, dst_port, method, response_code, cseq_method,
protocol, node_id, cid, payload, data_extra
```

**SIP Registrations (proto_type=1, sub_type=registration)** - optimized for AOR tracking:
```sql
uuid, timestamp, session_id, aor, contact, expires, user_agent,
src_ip, dst_ip, src_port, dst_port, method, response_code,
protocol, node_id, payload, data_extra
```

**SIP Default (proto_type=1, sub_type=default)** - other SIP messages:
```sql
uuid, timestamp, session_id, src_ip, dst_ip, src_port, dst_port,
method, response_code, protocol, node_id, cid, payload, data_extra
```

**RTCP/RTP (proto_type=5,34,35)** - simplified for media:
```sql
uuid, timestamp, session_id, src_ip, dst_ip, src_port, dst_port,
protocol, node_id, cid, payload, data_extra
```

**DNS (proto_type=53)** - minimal for lookups:
```sql
uuid, timestamp, src_ip, dst_ip, src_port, dst_port,
protocol, node_id, payload, data_extra
```

**LOG (proto_type=100)** - text-focused:
```sql
uuid, timestamp, session_id, src_ip, dst_ip, node_id, payload, data_extra
```

## Configuration

### Basic (SQLite catalog - default)

```json
{
  "storage": {
    "enable": true,
    "ducklake": {
      "catalog_type": "sqlite",
      "catalog_path": "/var/lib/homer/homer_catalog.sqlite",
      "data_path": "/var/lib/homer/parquet",
      "batch_size": 10000,
      "flush_interval_sec": 30
    }
  }
}
```

### S3 data path (SQLite catalog on disk)

Parquet files can live on S3 while the catalog remains a local SQLite file:

```json
{
  "storage": {
    "enable": true,
    "ducklake": {
      "catalog_type": "sqlite",
      "catalog_path": "/var/lib/homer/homer_catalog.sqlite",
      "data_path": "s3://my-bucket/homer-parquet/",
      "batch_size": 10000,
      "flush_interval_sec": 30,
      "s3": {
        "region": "us-east-1",
        "access_key_id": "AKIA...",
        "secret_access_key": "...",
        "endpoint": "http://127.0.0.1:9000",
        "use_ssl": false
      }
    }
  }
}
```

For **S3-compatible** endpoints (MinIO, RustFS, R2, …), set `s3.endpoint` to your base URL (for example `http://127.0.0.1:9000`); the writer strips the `http(s)://` scheme for DuckDB, enables **path-style** URLs (`s3_url_style=path`), and creates a matching **DuckDB `TYPE S3` secret** (same idea as the Flight node’s per-volume `CREATE SECRET`) so DuckLake maintenance (`delete_orphaned_files`, …) does not send `read_blob` to the wrong host. Ensure the **bucket** named in `data_path` (for example `s3://my-bucket/...`) already exists on that server — a missing bucket often returns **HTTP 404** on flush.

## Catalog

DuckLake catalog — sqlite. Parquet objects: `data_path` (local or `s3://`).

## API Endpoints

### Check Time Range (for Smart Routing)

```bash
POST /api/v1/ducklake/check
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
  "data_span_hours": 48,
  "table_count": 3,
  "proto_types": [1, 5, 100]
}
```

### Get Statistics

```bash
GET /api/v1/ducklake/stats
```

Response:
```json
{
  "total_row_count": 5000000,
  "total_buffer_size": 1234,
  "table_count": 3,
  "catalog_type": "sqlite",
  "data_path": "/var/lib/homer/parquet",
  "min_timestamp": 1737504000000000000,
  "max_timestamp": 1737676799999999999,
  "oldest_data": "2025-01-22T00:00:00Z",
  "newest_data": "2025-01-23T23:59:59Z",
  "tables": [
    {
      "proto_type": 1,
      "table": "homer_lake.hep_proto_1_default",
      "row_count": 4500000,
      "buffer_size": 500
    },
    {
      "proto_type": 5,
      "table": "homer_lake.hep_proto_5_default", 
      "row_count": 400000,
      "buffer_size": 234
    },
    {
      "proto_type": 100,
      "table": "homer_lake.hep_proto_100_default",
      "row_count": 100000,
      "buffer_size": 500
    }
  ]
}
```

### List Tables

```bash
GET /api/v1/ducklake/tables
```

Response:
```json
{
  "count": 3,
  "tables": [
    {
      "proto_type": 1,
      "table_name": "homer_lake.hep_proto_1_default",
      "row_count": 4500000,
      "min_timestamp": 1737504000000000000,
      "max_timestamp": 1737676799999999999,
      "oldest_data": "2025-01-22T00:00:00Z",
      "newest_data": "2025-01-23T23:59:59Z"
    },
    {
      "proto_type": 5,
      "table_name": "homer_lake.hep_proto_5_default",
      "row_count": 400000
    }
  ]
}
```

### List Snapshots (per proto_type)

Allowed `proto_type` / `sub_type` pairs match the canonical DuckLake schemas (see `GetTableSchemas()` in `tables.go`):

| proto_type | sub_type | Table |
|------------|----------|-------|
| 1 (SIP) | `call` (default), `registration`, `default` | SIP call / registration / other |
| 5 | _(empty)_ | RTCP JSON |
| 34, 35 | _(empty)_ | RTCP / RTP |
| 53 | _(empty)_ | DNS |
| 100 | _(empty)_ | LOG |

Invalid keys return **HTTP 400**. `limit` is clamped to 1000.

```bash
GET /api/v1/ducklake/snapshots?proto_type=1&sub_type=call&limit=10
```

Response:
```json
{
  "proto_type": 1,
  "snapshots": [
    {
      "id": 5,
      "created_at": "2025-01-23T10:00:00Z",
      "row_count": 5000000
    },
    {
      "id": 4,
      "created_at": "2025-01-23T09:00:00Z",
      "row_count": 4500000
    }
  ]
}
```

### Query

The `where` field is **not** arbitrary SQL. It must be a simple expression built from allowed columns for the target table(s):

- Predicates: `column = 'value'`, comparisons with integers, `column IS NULL`, combined with `AND` / `OR`
- Column names must exist on the schema (e.g. `session_id`, `src_ip`, `timestamp`)
- Max length 512; blocklisted tokens (`;`, `--`, `UNION`, `SELECT`, etc.) return **HTTP 400**

Query specific proto_type:

```bash
POST /api/v1/ducklake/query
Content-Type: application/json

{
  "proto_type": 1,
  "sub_type": "call",
  "where": "session_id = 'abc123@host'",
  "limit": 100
}
```

Query all tables:

```bash
POST /api/v1/ducklake/query
Content-Type: application/json

{
  "where": "src_ip = '192.168.1.100'",
  "limit": 100
}
```

## Usage Examples

### Command-line Maintenance

Run maintenance actions without starting modules (requires `--config-path`):

```bash
./homer-core --config-path /etc/homer/homer.json --compaction-force
./homer-core --config-path /etc/homer/homer.json --compaction-merge
./homer-core --config-path /etc/homer/homer.json --compaction-expire-snapshots --compaction-expire-older-than 2h
./homer-core --config-path /etc/homer/homer.json --compaction-retention-days 30
```

### Go Code

```go
import "github.com/sipcapture/homer-core/src/storage/ducklake"

// Create manager from config
manager, err := ducklake.NewManagerFromConfig()
if err != nil {
    log.Fatal(err)
}

// Start the manager
manager.Start()
defer manager.Stop()

// Write HEP packet (automatically routed to correct table by proto_type + SIP method)
err = manager.WriteHEP(hepPacket)

// Query SIP calls table
callKey := ducklake.TableKey{ProtoType: ducklake.ProtoTypeSIP, SubType: ducklake.SIPTypeCall}
records, err := manager.Query(callKey, "caller = '+1234567890'", 100)

// Query SIP registrations table
regKey := ducklake.TableKey{ProtoType: ducklake.ProtoTypeSIP, SubType: ducklake.SIPTypeRegistration}
records, err := manager.Query(regKey, "aor LIKE '%@example.com'", 100)

// Query RTCP table
rtcpKey := ducklake.TableKey{ProtoType: ducklake.ProtoTypeRTCPJSON}
records, err := manager.Query(rtcpKey, "session_id = 'abc123'", 100)

// Query all tables
records, err := manager.QueryAll("src_ip = '192.168.1.100'", 100)

// List all table keys (proto_type + sub_type combinations)
tableKeys := manager.ListTableKeys()

// List unique proto_types
protoTypes := manager.ListProtoTypes()

// Get stats for all tables
stats, err := manager.GetStats()

// List snapshots for SIP calls table
snapshots, err := manager.ListSnapshots(callKey, 10)
```

### DuckDB CLI

```sql
-- Install and load DuckLake
INSTALL ducklake;
LOAD ducklake;

-- Attach to existing DuckLake
ATTACH 'ducklake:sqlite:/data/homer_catalog.sqlite' AS homer_lake
(DATA_PATH '/data/homer_parquet');

-- List all tables
SHOW TABLES FROM homer_lake;

-- Query SIP Calls (INVITE, BYE, etc.)
SELECT * FROM homer_lake.hep_proto_1_call 
WHERE session_id = 'abc123@host'
ORDER BY timestamp DESC
LIMIT 100;

-- Query SIP Calls by caller/callee
SELECT * FROM homer_lake.hep_proto_1_call
WHERE caller = '+1234567890'
  AND timestamp > 1737504000000000000
ORDER BY timestamp DESC;

-- Query only INVITE requests
SELECT * FROM homer_lake.hep_proto_1_call
WHERE method = 'INVITE'
  AND response_code = '';

-- Query only 200 OK responses for INVITE (using cseq_method!)
SELECT * FROM homer_lake.hep_proto_1_call
WHERE cseq_method = 'INVITE'
  AND response_code = '200';

-- Query SIP Registrations
SELECT * FROM homer_lake.hep_proto_1_registration
WHERE aor LIKE '%@example.com'
ORDER BY timestamp DESC;

-- Query failed registrations (4xx responses)
SELECT * FROM homer_lake.hep_proto_1_registration
WHERE response_code LIKE '4%';

-- Query other SIP (OPTIONS, NOTIFY, etc.)
SELECT * FROM homer_lake.hep_proto_1_default
WHERE method = 'OPTIONS';

-- Query RTCP reports
SELECT * FROM homer_lake.hep_proto_5_default
WHERE session_id = 'abc123@host';

-- Query RTP
SELECT * FROM homer_lake.hep_proto_35_default
WHERE src_ip = '192.168.1.100';

-- Query Logs
SELECT * FROM homer_lake.hep_proto_100_default
WHERE payload LIKE '%ERROR%';

-- Count by table type
SELECT 'SIP Calls' as type, COUNT(*) as count FROM homer_lake.hep_proto_1_call
UNION ALL
SELECT 'SIP Registrations', COUNT(*) FROM homer_lake.hep_proto_1_registration
UNION ALL
SELECT 'SIP Other', COUNT(*) FROM homer_lake.hep_proto_1_default
UNION ALL
SELECT 'RTCP JSON', COUNT(*) FROM homer_lake.hep_proto_5_default
UNION ALL
SELECT 'LOG', COUNT(*) FROM homer_lake.hep_proto_100_default;

-- Time travel: query calls at snapshot
SELECT * FROM homer_lake.hep_proto_1_call AT SNAPSHOT 4
WHERE session_id = 'abc123@host';

-- Time travel: query registrations at timestamp
SELECT * FROM homer_lake.hep_proto_1_registration AT TIMESTAMP '2025-01-23 09:00:00'
WHERE aor LIKE '%@example.com';

-- List snapshots for SIP calls table
SELECT * FROM ducklake_snapshots('homer_lake.hep_proto_1_call');
```

## DuckLake Maintenance

DuckLake stores data in immutable Parquet files. Over time, many small files accumulate.
Maintenance operations help optimize storage and query performance.

### Via HTTP API (Node module)

```bash
# Run maintenance on all tables (expire snapshots + merge files + cleanup)
curl -X POST http://localhost:50052/vacuum

# With custom expire time
curl -X POST "http://localhost:50052/vacuum?expire_older_than=1%20hour"
```

### Via DuckDB CLI

Connect to DuckLake:

```bash
duckdb -c "
INSTALL ducklake; LOAD ducklake;
ATTACH 'ducklake:sqlite:/data/homer/homer_catalog.sqlite' AS homer_lake (DATA_PATH '/data/homer/parquet');
"
```

**1. Expire old snapshots** (mark as eligible for cleanup):

```sql
-- Expire snapshots older than 1 hour
CALL ducklake_expire_snapshots('homer_lake', older_than => CAST(NOW() - INTERVAL '1 hour' AS TIMESTAMPTZ));

-- Expire all but latest
CALL ducklake_expire_snapshots('homer_lake', older_than => CAST(NOW() AS TIMESTAMPTZ));
```

**2. Merge adjacent files** (combine small Parquet files):

```sql
-- Merge files across entire lake
CALL ducklake_merge_adjacent_files('homer_lake');

-- Merge specific table
CALL ducklake_merge_adjacent_files('homer_lake', 'hep_proto_1_call', schema => 'main');
```

**3. Cleanup old files** (delete files from expired snapshots):

```sql
CALL ducklake_cleanup_old_files('homer_lake');
```

**4. Delete orphaned files** (files not referenced by any snapshot):

```sql
CALL ducklake_delete_orphaned_files('homer_lake');
```

### Recommended Maintenance Schedule

| Operation | Frequency | Purpose |
|-----------|-----------|---------|
| `merge_adjacent_files` | Every 1-4 hours | Combine small files |
| `expire_snapshots` | Daily | Mark old snapshots |
| `cleanup_old_files` | After expire | Delete unreferenced files |
| `delete_orphaned_files` | Weekly | Clean orphan files |

### Known Issues

- **DuckLake 0.3.x**: `CHECKPOINT` command may fail with type comparison error. Use individual maintenance functions instead.
- **Merge behavior**: Only merges "adjacent" files (sequential snapshots). Files from different time ranges are not merged.
- **Time travel**: Once snapshots are expired, time travel to those snapshots is no longer possible.

## Architecture

```
┌─────────────────────────────────────────────────────────────────────────────────────┐
│                              Homer Server                                            │
│                                                                                      │
│  HEP Packets ──► Ingest ──► Storage ──► Parquet Files (local or S3)                 │
│       │           (decode)   │                                                       │
│       │                      │  SIP (proto_type=1) routed by method:                │
│       │                      ├──► hep_proto_1_call/         (INVITE,BYE,CANCEL...)  │
│       ▼                      ├──► hep_proto_1_registration/ (REGISTER)              │
│  proto_type +                ├──► hep_proto_1_default/      (OPTIONS,NOTIFY...)     │
│  SIP method router           │                                                       │
│       │                      │  Other protocols:                                     │
│       │                      ├──► hep_proto_5_default/      (RTCP JSON)             │
│       │                      ├──► hep_proto_34_default/     (RTCP)                   │
│       │                      ├──► hep_proto_35_default/     (RTP)                    │
│       │                      ├──► hep_proto_100_default/    (LOG)                    │
│       │                      └──► hep_proto_N_default/      (other)                  │
│       │                                                                              │
│       └─── For SIP responses: uses CSeq method for routing (not response line)      │
│                                                                                      │
│                    DuckLake Catalog                                                  │
│                    ┌─────────────────────────────────────────────────┐               │
│                    │  ┌───────────┐  ┌───────────┐  ┌─────────────┐ │               │
│                    │  │ Snapshots │  │ File List │  │ Table Schema│ │               │
│                    │  │ per table │  │ per table │  │  per table  │ │               │
│                    │  └───────────┘  └───────────┘  └─────────────┘ │               │
│                    └─────────────────────────────────────────────────┘               │
│                    (DuckDB / PostgreSQL / MySQL / SQLite)                            │
└─────────────────────────────────────────────────────────────────────────────────────┘
```

### Benefits of Multi-Table Architecture

1. **Optimized schemas** - Each message type has only the columns it needs
2. **Better query performance** - Smaller tables, faster scans
3. **Independent retention** - Calls may need longer retention than OPTIONS
4. **Parallel ingestion** - Tables can be written independently
5. **Simplified queries** - No need to filter by proto_type or method
6. **SIP-specific optimization** - Calls have caller/callee, Registrations have AOR/contact

## Comparison: Arrow vs DuckLake

### When to use Arrow Storage
- Simple, single-node deployments
- Maximum control over file layout
- Custom compaction logic needed
- Lower complexity preferred

### When to use DuckLake Storage
- Multi-node distributed deployments
- Time travel queries required (compliance, debugging)
- S3/cloud storage integration
- Automatic schema evolution needed
- Standard lakehouse format preferred

## License

AGPL-3.0 License - QXIP / SIPCapture Team
