# PostgreSQL DuckLake catalogs

DuckDB continues to execute lake queries and write Parquet. PostgreSQL replaces
SQLite only for the DuckLake metadata catalog. Application settings can also use
PostgreSQL through `coordinator.settings_db_path`; that is a separate connection
and should normally use its own PostgreSQL database.

## One catalog

Provision a PostgreSQL database owned by the Homer role, install extensions using
`homer-core --install-extensions`, and configure the existing catalog fields:

```json
{
  "storage": {
    "ducklake": {
      "catalog_type": "postgres",
      "catalog_path": "postgres://homer:password@localhost:5432/homer_catalog?sslmode=require",
      "data_path": "/var/lib/homer/parquet",
      "lake_name": "homer_lake",
      "shard_count": 1,
      "tuning": { "temp_directory": "/var/lib/homer/spill" }
    }
  },
  "node": {
    "ducklake": {
      "catalog_type": "postgres",
      "catalog_path": "postgres://homer:password@localhost:5432/homer_catalog?sslmode=require",
      "data_path": "/var/lib/homer/parquet",
      "tuning": { "temp_directory": "/var/lib/homer/node-spill" }
    }
  }
}
```

Libpq keyword DSNs such as `host=localhost dbname=homer_catalog user=homer` also
work. A connection string in `catalog_path` selects PostgreSQL even when
`catalog_type` retains its SQLite default. Setting `catalog_type=postgres` with
a file path is rejected. Empty catalog type and a normal file path preserve the
existing SQLite backend. `postgresql` is accepted as a catalog type alias.

Data paths can still be local, S3, or Azure using the existing credential settings.
Writers and nodes must see the same data files. Configure an explicit local spill
directory: a connection string has no catalog directory from which to derive one.

No catalog data is migrated automatically. This enables new PostgreSQL-backed
catalogs; converting existing SQLite catalogs requires a separate migration.

## Shards

Each writer shard keeps an independent catalog, matching the SQLite design:

| Shard count | Catalog databases for a base `homer_catalog` DSN |
| --- | --- |
| 1 | `homer_catalog` |
| 2 | `homer_catalog_0`, `homer_catalog_1` |
| N | `homer_catalog_0` through `homer_catalog_(N-1)` |

Provision all required databases before starting Homer. It does not create
PostgreSQL databases or require CREATE DATABASE privileges in production. Database
suffixes are applied to the DSN's database name; credentials and connection options
are retained. Database names longer than PostgreSQL's 63-byte limit are rejected.
Shard DuckDB instances also use separate spill subdirectories when configured.

For node reads, explicitly configure one volume for each shard, as with the
existing independent-catalog layout. For example, for two shards:

```json
{
  "node": {
    "ducklake": {
      "volumes": [
        {
          "name": "0", "type": "local",
          "catalog_type": "postgres",
          "catalog_path": "postgres://homer:password@localhost:5432/homer_catalog_0?sslmode=require",
          "path": "/var/lib/homer/parquet"
        },
        {
          "name": "1", "type": "local",
          "catalog_type": "postgres",
          "catalog_path": "postgres://homer:password@localhost:5432/homer_catalog_1?sslmode=require",
          "path": "/var/lib/homer/parquet"
        }
      ]
    }
  }
}
```

The node's existing volume-query logic handles these catalogs. The SQL CLI
attaches the PostgreSQL catalog selected by its configuration, including inline
rows, in read-only mode. To inspect a shard with the CLI, select that shard's DSN;
the CLI does not automatically union PostgreSQL shard catalogs.

## Maintenance and tiering

DuckLake SQL compaction and separate maintenance DuckDB attachments support
PostgreSQL. Existing automatic compaction still targets the primary writer shard;
this change does not add an automatic maintenance loop for every shard.

SQLite WAL, file locks, startup repair, integrity checks and catalog file backups
are skipped for PostgreSQL. Native compaction and native tier moves fall back to
DuckLake SQL operations for PostgreSQL. Manage PostgreSQL catalog backups and
recovery with PostgreSQL tools. SQLite-only catalog backup/restore/rebuild CLI
commands reject PostgreSQL paths rather than trying to open them as files.

For a single writer catalog with tiered volumes, the priority-zero hot volume uses
the base database and other volumes use `<base_database>_<volume_name>`. Provision
those databases as well. Sharding and tiering cannot currently be combined for
PostgreSQL: the existing tier manager manages one hot catalog, and configuration
validation rejects the combination instead of attaching an empty base catalog.

Catalog DSNs are redacted from config output and attachment errors. Node auth
secrets use the normal local `/data` fallback rather than treating the DSN as a
filesystem directory.

## Tests

With a disposable PostgreSQL server and an administrative test DSN (CREATE
DATABASE required for the tests only), run from `src`:

```sh
HOMER_TEST_POSTGRES_DSN='postgres://postgres:password@127.0.0.1:5432/test?sslmode=disable' \
  go test ./storage/ducklake ./node ./writer ./config ./cli
```

Tests create and drop private databases. They cover shard persistence after
shutdown, independent shard databases and spill paths, maintenance attachment and
merge calls, tiered catalog isolation, node reads, inline-row CLI reads, DSN
handling, and the SQLite-specific maintenance guards.

Reference: [DuckLake catalog connection options](https://ducklake.select/docs/stable/duckdb/usage/connecting).
