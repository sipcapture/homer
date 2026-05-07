# Compaction & Metadata Store Setup

This guide explains how to enable and configure automatic file compaction and metadata indexing in Homer Server.

## Overview

Homer Server provides two key features for optimizing Parquet storage:

1. **CompactionService** — Automatically merges small Parquet files into larger ones (LSM-tree style)
2. **MetadataStore** — Maintains bloom filter indexes for fast field lookups

```
┌─────────────────────────────────────────────────────────────┐
│                    SchedulerManager.Start()                  │
│                                                              │
│  ┌─────────────────┐  ┌──────────────────┐  ┌─────────────┐ │
│  │ HivePartition   │  │ MetadataStore    │  │ Compaction  │ │
│  │ Manager         │  │ (DuckDB)         │  │ Service     │ │
│  │                 │  │                  │  │             │ │
│  │ WriteHEP() ─────┼──► RegisterFile()   │  │ Every 60s:  │ │
│  │                 │  │ + Bloom Filters  │  │ Merge files │ │
│  └─────────────────┘  └──────────────────┘  └─────────────┘ │
└─────────────────────────────────────────────────────────────┘
```

## Configuration

### Minimal Configuration

```json
{
  "arrow_settings": {
    "enable": true,
    "output_dir": "/data/homer_parquet",
    "hive_partitioning": true,
    "compaction": {
      "enable": true
    },
    "metadata": {
      "enable": true
    }
  }
}
```

### Full Configuration

```json
{
  "arrow_settings": {
    "enable": true,
    "output_dir": "/data/homer_parquet",
    "dump_interval_sec": 10,
    "batch_size": 1000,
    "max_file_size_mb": 100,
    "max_file_age_sec": 3600,
    "hive_partitioning": true,
    "order_by": "timestamp",
    
    "compaction": {
      "enable": true,
      "check_interval_sec": 60,
      "level1_timeout_sec": 30,
      "level1_max_size_mb": 100,
      "level2_timeout_sec": 300,
      "level2_max_size_mb": 400,
      "level3_timeout_sec": 1800,
      "level3_max_size_mb": 4000
    },
    
    "metadata": {
      "enable": true,
      "db_path": "metadata.duckdb",
      "bloom_filter_fpp": 0.01,
      "indexed_fields": [
        {"field": "session_id", "type": "bloom"},
        {"field": "caller", "type": "bloom"},
        {"field": "callee", "type": "bloom"},
        {"field": "src_ip", "type": "bloom"},
        {"field": "dst_ip", "type": "bloom"}
      ],
      "auto_vacuum_hours": 24,
      "retention_days": 14
    }
  }
}
```

## Configuration Options

### Compaction Settings

| Option | Default | Description |
|--------|---------|-------------|
| `enable` | `true` | Enable automatic compaction |
| `check_interval_sec` | `60` | How often to check for files to merge (seconds) |
| `level1_timeout_sec` | `10` | Merge L1→L2 after this many seconds |
| `level1_max_size_mb` | `100` | Target size for L2 files (MB) |
| `level2_timeout_sec` | `100` | Merge L2→L3 after this many seconds |
| `level2_max_size_mb` | `400` | Target size for L3 files (MB) |
| `level3_timeout_sec` | `1000` | Merge L3→L4 after this many seconds |
| `level3_max_size_mb` | `4000` | Target size for L4 files (MB) |

### Metadata Settings

| Option | Default | Description |
|--------|---------|-------------|
| `enable` | `false` | Enable metadata store and bloom indexes |
| `db_path` | `metadata.duckdb` | DuckDB database file name |
| `bloom_filter_fpp` | `0.01` | Bloom filter false positive probability (1%) |
| `indexed_fields` | `[]` | Fields to index (see below) |
| `auto_vacuum_hours` | `24` | Run vacuum every N hours |
| `retention_days` | `14` | Delete metadata older than N days |

### Indexed Fields Format

```json
"indexed_fields": [
  {"field": "session_id", "type": "bloom"},
  {"field": "caller", "type": "bloom"},
  {"field": "callee", "type": "bloom"},
  {"field": "src_ip", "type": "bloom"},
  {"field": "dst_ip", "type": "bloom"},
  {"field": "user_agent", "type": "bloom"}
]
```

Supported index types:
- `bloom` — Probabilistic filter for "may contain" checks (recommended for high cardinality)
- `minmax` — Min/max values for range queries
- `exact` — Exact value index (for low cardinality fields)

## Compaction Levels

Files progress through 4 levels:

```
Level 1 (raw)     Level 2           Level 3           Level 4
 ~10MB             ~100MB            ~400MB            ~4GB
┌────────┐       ┌────────┐       ┌────────┐       ┌────────┐
│ .1.pq  │──┐    │        │──┐    │        │──┐    │        │
├────────┤  │    │ .2.pq  │  │    │ .3.pq  │  │    │ .4.pq  │
│ .1.pq  │──┼───►│ merged │  ├───►│ merged │  ├───►│ final  │
├────────┤  │    │        │  │    │        │  │    │        │
│ .1.pq  │──┘    │        │──┘    │        │──┘    │        │
└────────┘       └────────┘       └────────┘       └────────┘
```

File naming: `{uuid}.{level}.parquet`
- `abc123.1.parquet` — Level 1 (raw)
- `def456.2.parquet` — Level 2 (first merge)
- `ghi789.4.parquet` — Level 4 (final)

## Verification

### 1. Check Logs on Startup

After starting homer-core, you should see:

```
INFO MetadataStore enabled with bloom filter indexes
INFO CompactionService configured, check interval: 1m0s
INFO SchedulerManager started with Hive partitioning
INFO MetadataStore auto vacuum started (interval: 24h0m0s)
INFO CompactionService started
```

### 2. Check Compaction Activity

After `check_interval_sec` seconds, compaction logs appear:

```
INFO Starting merge: 3 files -> /data/homer_parquet/date=2025-01-23/hour=14/abc123.2.parquet
INFO Completed merge: 3 files -> .../abc123.2.parquet (level 2)
INFO Registered merged file with bloom indexes: .../abc123.2.parquet (15000 rows)
```

### 3. Check File Levels

```bash
# List files by level
ls -la /data/homer_parquet/date=*/hour=*/*.parquet | grep -E '\.[1-4]\.parquet'

# Count files per level
for level in 1 2 3 4; do
  echo "Level $level: $(find /data/homer_parquet -name "*.$level.parquet" | wc -l) files"
done
```

### 4. Check Metadata Store

```bash
# Check metadata database exists
ls -la /data/homer_parquet/metadata.duckdb

# Query metadata (using duckdb CLI)
duckdb /data/homer_parquet/metadata.duckdb \
  "SELECT level, COUNT(*) as files, SUM(row_count) as rows FROM file_metadata GROUP BY level"
```

### 5. Check via API

```bash
# Get storage stats
curl http://localhost:8080/api/v1/metadata/stats | jq

# Check bloom filter
curl -X POST http://localhost:8080/api/v1/metadata/check \
  -H "Content-Type: application/json" \
  -d '{"field": "session_id", "values": ["abc123@host"]}'
```

## Troubleshooting

### Problem: Files stay at Level 1

**Possible causes:**

1. **Compaction disabled** — Check `compaction.enable: true`
2. **Not enough files** — Need at least 2 files to merge
3. **Timeout not reached** — Wait for `level1_timeout_sec`
4. **Check interval too long** — Lower `check_interval_sec`

**Solution:**
```json
{
  "compaction": {
    "enable": true,
    "check_interval_sec": 30,
    "level1_timeout_sec": 10
  }
}
```

### Problem: No bloom indexes

**Possible causes:**

1. **Metadata disabled** — Check `metadata.enable: true`
2. **No indexed fields** — Add fields to `indexed_fields`

**Solution:**
```json
{
  "metadata": {
    "enable": true,
    "indexed_fields": [
      {"field": "session_id", "type": "bloom"}
    ]
  }
}
```

### Problem: Metadata store too large

**Solution:** Reduce retention:
```json
{
  "metadata": {
    "retention_days": 7,
    "auto_vacuum_hours": 12
  }
}
```

### Problem: High CPU during compaction

**Solution:** Increase timeouts to reduce frequency:
```json
{
  "compaction": {
    "check_interval_sec": 120,
    "level1_timeout_sec": 60
  }
}
```

## Performance Tuning

### High-Volume Deployment (>10k PPS)

```json
{
  "arrow_settings": {
    "batch_size": 50000,
    "dump_interval_sec": 30,
    "compaction": {
      "check_interval_sec": 120,
      "level1_timeout_sec": 60,
      "level1_max_size_mb": 200
    }
  }
}
```

### Low-Latency Search

```json
{
  "arrow_settings": {
    "metadata": {
      "enable": true,
      "bloom_filter_fpp": 0.001,
      "indexed_fields": [
        {"field": "session_id", "type": "bloom"},
        {"field": "caller", "type": "bloom"},
        {"field": "callee", "type": "bloom"}
      ]
    }
  }
}
```

### Minimal Storage Overhead

```json
{
  "arrow_settings": {
    "compaction": {
      "level1_max_size_mb": 500,
      "level2_max_size_mb": 2000,
      "level3_max_size_mb": 8000
    },
    "metadata": {
      "retention_days": 7
    }
  }
}
```

## Architecture

### Data Flow

```
HEP Packet
    │
    ▼
┌─────────────────────────────┐
│   HivePartitionManager      │
│   - Partition by date/hour  │
│   - Write to .1.parquet     │
│   - Collect field values    │
└─────────────────────────────┘
    │
    ▼
┌─────────────────────────────┐
│   MetadataStore             │
│   - Register file metadata  │
│   - Create bloom filters    │
│   - Store min/max values    │
└─────────────────────────────┘
    │
    ▼
┌─────────────────────────────┐
│   CompactionService         │
│   - Find files to merge     │
│   - Merge via DuckDB        │
│   - Rebuild bloom indexes   │
│   - Delete old files        │
└─────────────────────────────┘
```

### Query Optimization

When Smart Routing is enabled in Homer Hub, queries benefit from:

1. **Node-level pruning** — Skip nodes without matching data
2. **File-level pruning** — Skip files based on bloom filters
3. **Time-range pruning** — Skip files outside time range

```
Query: session_id = "abc123"
    │
    ▼
┌─────────────────────────────┐
│ Homer Hub: Smart Router     │
│ Check bloom on each node    │
│ → Node 1: may_contain=true  │
│ → Node 2: may_contain=false │ ← Skip!
│ → Node 3: may_contain=true  │
└─────────────────────────────┘
    │
    ▼
┌─────────────────────────────┐
│ Homer Server: Query Opt     │
│ Check local bloom filters   │
│ → file1: may_contain=true   │
│ → file2: may_contain=false  │ ← Skip!
│ → file3: may_contain=true   │
└─────────────────────────────┘
    │
    ▼
┌─────────────────────────────┐
│ DuckDB: Read only           │
│ file1.parquet, file3.parquet│
└─────────────────────────────┘
```

## License

AGPL-3.0 License - QXIP / SIPCapture Team
