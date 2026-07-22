# Storage Policies - Tiered Storage for Homer Server

Storage policies allow you to configure tiered storage, automatically moving old data from fast local storage (hot) to cheaper object storage like S3 or Cloudflare R2 (cold).

## Overview

```
┌─────────────────────────────────────────────────────────┐
│                    Homer Storage                         │
│                         │                                │
│                         ▼                                │
│              ┌─────────────────────┐                    │
│              │    Hot Volume       │ ◄── New data       │
│              │   (Local SSD)       │     written here   │
│              │  /data/homer/       │                    │
│              │  max_age: 7 days    │                    │
│              └─────────┬───────────┘                    │
│                        │                                │
│                        │ TieringService                 │
│                        │ (automatic, daily)             │
│                        ▼                                │
│              ┌─────────────────────┐                    │
│              │    Cold Volume      │ ◄── Old data       │
│              │   (S3/R2 bucket)    │     moved here     │
│              │  s3://bucket/cold/  │                    │
│              │  max_age: unlimited │                    │
│              └─────────────────────┘                    │
└─────────────────────────────────────────────────────────┘
```

## Configuration

Add the `storage_policy` section to your `storage.ducklake` configuration:

```json
{
  "storage": {
    "enable": true,
    "ducklake": {
      "storage_policy": {
        "enable": true,
        "ttl_move_interval_sec": 3600,
        "move_factor": 0.8,
        "concurrent_moves": 2,
        "move_on_startup": false,
        "volumes": [
          {
            "name": "hot",
            "type": "local",
            "path": "/data/homer/parquet",
            "priority": 0,
            "max_data_age_days": 7,
            "max_size_gb": 100
          },
          {
            "name": "cold",
            "type": "s3",
            "path": "s3://your-bucket/homer/cold/",
            "priority": 1,
            "max_data_age_days": 0,
            "s3_region": "us-east-1",
            "s3_access_key_id": "YOUR_ACCESS_KEY",
            "s3_secret_access_key": "YOUR_SECRET_KEY",
            "s3_endpoint": "",
            "s3_use_ssl": true
          }
        ]
      }
    }
  }
}
```

## Configuration Options

### Storage Policy Settings

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `enable` | bool | false | Enable tiered storage |
| `ttl_move_interval_sec` | int | 3600 | How often to check for data to move (seconds) |
| `move_factor` | float | 0.8 | Move data when volume fill ratio exceeds this value (0.0-1.0) |
| `concurrent_moves` | int | 2 | Maximum concurrent partition moves |
| `move_on_startup` | bool | false | Run tiering check on server startup |

#### move_factor Explained

The `move_factor` parameter works similar to ClickHouse storage policies. It controls when data starts moving from a volume based on disk usage:

- **Value range**: 0.0 to 1.0 (percentage as decimal)
- **Default**: 0.8 (80%)
- **Behavior**: When volume usage exceeds `move_factor * max_size_gb`, oldest partitions are moved to the next volume

**Example scenarios:**

| move_factor | max_size_gb | Trigger Point |
|-------------|-------------|---------------|
| 0.8 | 100 GB | Move starts when volume has 80 GB of data |
| 0.9 | 500 GB | Move starts when volume has 450 GB of data |
| 0.5 | 200 GB | Move starts when volume has 100 GB of data |
| 1.0 | any | Only TTL-based moves (age), no size-based moves |

**Note**: If `max_size_gb` is 0 (unlimited), only TTL-based moves (`max_data_age_days`) will trigger data movement.

### Volume Settings

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `name` | string | required | Volume name (e.g., "hot", "cold") |
| `type` | string | "local" | Storage type: "local" or "s3" |
| `path` | string | required | Local path or S3 URL |
| `priority` | int | 0 | Lower = higher priority. Writes go to lowest priority |
| `max_data_age_days` | int | 0 | On intermediate volumes: move partitions whose DuckLake **`date`** is **on or before** `calendar(today) − N days` (inclusive) to the next volume. On the **final** volume: delete (expire) those partitions instead. Example: `N=1` on May 12 includes partition `date=2026-05-11`. `0` disables TTL for that volume. |
| `max_size_gb` | int | 0 | Max volume size in GB (0 = no limit) |

### S3-specific Settings (for `type: "s3"`)

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `s3_region` | string | "" | AWS region |
| `s3_access_key_id` | string | "" | Access key |
| `s3_secret_access_key` | string | "" | Secret key |
| `s3_endpoint` | string | "" | Custom endpoint for S3-compatible services (R2, MinIO, RustFS) |
| `s3_use_ssl` | bool | true | Use HTTPS for S3 connections |
| `s3_url_style` | string | "path" | DuckDB S3 URL style for custom endpoints (`path` or `vhost`) |

## Examples

### Local + S3 (AWS)

```json
{
  "volumes": [
    {
      "name": "hot",
      "type": "local",
      "path": "/data/homer/parquet",
      "priority": 0,
      "max_data_age_days": 7
    },
    {
      "name": "cold",
      "type": "s3",
      "path": "s3://homer-archive/data/",
      "priority": 1,
      "s3_region": "us-east-1",
      "s3_access_key_id": "AKIAIOSFODNN7EXAMPLE",
      "s3_secret_access_key": "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"
    }
  ]
}
```

### Local + Cloudflare R2

```json
{
  "volumes": [
    {
      "name": "hot",
      "type": "local",
      "path": "/data/homer/parquet",
      "priority": 0,
      "max_data_age_days": 30
    },
    {
      "name": "cold",
      "type": "s3",
      "path": "s3://homer-bucket/cold/",
      "priority": 1,
      "s3_region": "auto",
      "s3_access_key_id": "YOUR_R2_ACCESS_KEY",
      "s3_secret_access_key": "YOUR_R2_SECRET_KEY",
      "s3_endpoint": "https://ACCOUNT_ID.r2.cloudflarestorage.com"
    }
  ]
}
```

### Local + MinIO

```json
{
  "volumes": [
    {
      "name": "hot",
      "type": "local",
      "path": "/data/homer/parquet",
      "priority": 0,
      "max_data_age_days": 7
    },
    {
      "name": "cold",
      "type": "s3",
      "path": "s3://homer/archive/",
      "priority": 1,
      "s3_region": "us-east-1",
      "s3_access_key_id": "minioadmin",
      "s3_secret_access_key": "minioadmin",
      "s3_endpoint": "http://minio:9000",
      "s3_use_ssl": false
    }
  ]
}
```

### Local + RustFS

[RustFS](https://github.com/rustfs/rustfs) is a high-performance S3-compatible object storage written in Rust.

```json
{
  "volumes": [
    {
      "name": "hot",
      "type": "local",
      "path": "/data/homer/parquet",
      "priority": 0,
      "max_data_age_days": 7
    },
    {
      "name": "cold",
      "type": "s3",
      "path": "s3://homer-cold/data/",
      "priority": 1,
      "s3_region": "us-east-1",
      "s3_access_key_id": "rustfsadmin",
      "s3_secret_access_key": "rustfsadmin",
      "s3_endpoint": "http://rustfs:9000",
      "s3_use_ssl": false
    }
  ]
}
```

### Three-tier Storage

```json
{
  "volumes": [
    {
      "name": "hot",
      "type": "local",
      "path": "/data/homer/ssd",
      "priority": 0,
      "max_data_age_days": 3
    },
    {
      "name": "warm",
      "type": "local",
      "path": "/data/homer/hdd",
      "priority": 1,
      "max_data_age_days": 30
    },
    {
      "name": "cold",
      "type": "s3",
      "path": "s3://homer-archive/data/",
      "priority": 2,
      "s3_region": "us-east-1",
      "s3_access_key_id": "...",
      "s3_secret_access_key": "..."
    }
  ]
}
```

## How It Works

### Data Flow

1. **Write**: All new data is written to the primary (hot) volume (lowest priority number)
2. **Tiering**: The TieringService periodically checks for old partitions
3. **Copy**: On intermediate volumes, data older than `max_data_age_days` is copied to the next volume (new parquet files created)
4. **Delete from source**: After successful copy, data is deleted from the source volume
5. **Final-volume expiry**: On the last volume, `max_data_age_days > 0` deletes matching partitions (no next tier)
6. **Cleanup**: Empty partition directories are automatically removed
7. **Query**: Queries automatically search across all volumes using UNION ALL

### Final volume expiry

When the last volume has `max_data_age_days: N` (for example cold S3 with `N=5`), the tiering cycle expires partitions with `date <= calendar(today) − N`. Logs look like:

```
level=INFO msg="TieringService: TTL partition expire scan" source=cold max_data_age_days=5 partition_date_cutoff=2026-07-17
level=INFO msg="TieredStorageManager: Partition expired" table=hep_proto_1_call date=2026-07-14 volume=cold rows=...
level=INFO msg="TieringService: Tiering cycle completed" partitions_moved=0 partitions_expired=4
```

Set `max_data_age_days: 0` on the final volume to keep data indefinitely (or rely on S3 lifecycle / writer `retention_days`).

### Partition Movement Process

Data is partitioned by date (`date` column). The tiering service copies entire date partitions to cold storage:

```sql
-- Step 1: Copy data to cold storage (creates new parquet files in S3)
INSERT INTO cold_lake.main.hep_proto_1_call 
SELECT * FROM hot_lake.main.hep_proto_1_call 
WHERE date = '2026-01-15';

-- Step 2: Delete from hot storage (marks records as deleted in DuckLake catalog)
DELETE FROM hot_lake.main.hep_proto_1_call 
WHERE date = '2026-01-15';

-- Step 3: Cleanup empty partition directories (automatic)
-- /data/homer/parquet/main/hep_proto_1_call/date=2026-01-15/ removed if empty
```

**Important notes:**
- This is a **copy + delete** operation, not physical file movement
- New parquet files are created in cold storage (S3/R2)
- Original parquet files in hot storage are marked for deletion (GC removes them later)
- If copy succeeds but delete fails, data exists in both places temporarily (no data loss)
- A failed source delete is **not** reported as `Partition moved`; the tiering cycle logs `Partition copied, source delete pending` and does not increment `partitions_moved`
- On the next tiering cycle, if cold already holds the partition, HOMER performs **delete-only** from hot (no duplicate cold copy)
- Under high ingest load, use `concurrent_moves=1` to reduce SQLite catalog contention on the shared hot catalog
- Tables in cold storage are created with `PARTITION BY (date)` for efficient queries

### Querying Across Volumes

When storage policy is enabled, queries automatically span all volumes:

```sql
-- Executed internally as:
(SELECT * FROM hot_lake.main.hep_proto_1_call WHERE ...)
UNION ALL
(SELECT * FROM cold_lake.main.hep_proto_1_call WHERE ...)
ORDER BY timestamp DESC
LIMIT 1000
```

## Monitoring

Monitor tiered storage via logs:

```
level=INFO msg="TieringService: Starting tiering cycle"
level=INFO msg="TieringService: Found old partitions" table=hep_proto_1_call count=3 dates=[2026-01-10 2026-01-11 2026-01-12]
level=INFO msg="TieredStorageManager: Partition moved" table=hep_proto_1_call date=2026-01-10 rows=150000
level=INFO msg="TieringService: Tiering cycle completed" duration=45.2s partitions_moved=3
```

If hot source delete fails after a successful cold copy (for example SQLite `database is locked` under ingest load):

```
level=WARN msg="TieredStorageManager: Partition copied, source delete pending" table=hep_proto_1_call date=2026-01-10 rows=150000 error="..."
level=ERROR msg="TieringService: Failed to move partition" table=hep_proto_1_call date=2026-01-10 error="source delete failed after copy: ..."
level=INFO msg="TieringService: Tiering cycle completed" duration=45.2s partitions_moved=0
```

The next cycle retries delete-only when cold already contains the partition:

```
level=INFO msg="TieredStorageManager: Destination already has partition; retrying source delete only" table=hep_proto_1_call date=2026-01-10 rows=150000
level=INFO msg="TieredStorageManager: Partition moved" table=hep_proto_1_call date=2026-01-10 rows=150000
```

## Migration from Non-Tiered Setup

If you have existing data without tiered storage and want to enable it, the system automatically handles migration:

### Automatic Migration

When tiered storage is enabled, the system checks for an existing legacy catalog:

| Scenario | Hot Catalog | Cold Catalog |
|----------|-------------|--------------|
| New installation | `homer_catalog_hot.sqlite` | `homer_catalog_cold.sqlite` |
| Migration from legacy | `homer_catalog.sqlite` (existing) | `homer_catalog_cold.sqlite` |

**What happens:**
1. If `homer_catalog.sqlite` exists, it's used as the hot volume catalog
2. A new `homer_catalog_cold.sqlite` is created for cold storage
3. Existing Parquet files in `/data/homer/parquet/` continue to work
4. Old data will gradually move to cold storage based on `max_data_age_days`

**Log output during migration:**
```
level=INFO msg="TieredStorageManager: Using legacy catalog for hot volume (migration mode)" path=/data/homer/homer_catalog.sqlite
```

### No Manual Steps Required

Simply enable `storage_policy` in your config and restart. The system handles the rest.

## Best Practices

1. **Start with longer retention on hot storage**: Begin with 30 days and reduce as needed — configure TTL via [`retention_days`](RETENTION.md#data-ttl-retention_days) / optional [`retention_days_by_table`](RETENTION.md#data-ttl-retention_days), not mapping schema alone.
2. **Use compaction before tiering**: Ensure compaction runs before tiering to minimize small files in cold storage
3. **Monitor S3 costs**: Object storage egress can be expensive for frequently queried data
4. **Test restore procedures**: Periodically verify you can query data from cold storage
5. **Use lifecycle policies**: Configure S3 lifecycle rules for further cost optimization (e.g., Glacier after 1 year)

## Limitations

- Currently supports moving by date partition only (not by size)
- No automatic data recall from cold to hot
- S3 query performance may be slower than local storage
- Each volume requires a separate DuckLake catalog file
