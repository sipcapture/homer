# Configure PostgreSQL DuckLake catalogs

This guide explains how to use PostgreSQL for DuckLake metadata catalogs.
DuckDB continues to execute lake queries and write Parquet files.

For application settings, follow
[Configure PostgreSQL-backed settings](POSTGRES_SETTINGS.md). Settings use a
separate connection and should normally have their own PostgreSQL database.

## Before you begin

You need:

- A PostgreSQL database owned by the Homer role, reachable from writers and nodes.
- A data location that writers and nodes can both access.
- A writable local spill directory for each Homer process.

Existing SQLite catalogs are not migrated automatically. Use this procedure for
a new PostgreSQL-backed catalog; converting an existing catalog requires a
separate migration.

## 1. Install the extensions

Run this command using the DuckDB engine and OS user that run Homer:

```sh
homer-core system --install-extensions
```

## 2. Configure a single catalog

Set the writer and node catalog fields in your Homer configuration. Replace the
connection details and filesystem paths for your deployment:

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

## 3. Start Homer and verify access

Start or restart the writer and node with the updated configuration. Ingest data
through your normal input, then search for it through Homer to check that the
writer and node can access the catalog and its data files.

## Configure multiple shards

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

## Maintain PostgreSQL catalogs

DuckLake SQL compaction and separate maintenance DuckDB attachments support
PostgreSQL. Existing automatic compaction still targets the primary writer shard;
this change does not add an automatic maintenance loop for every shard.

SQLite WAL, file locks, startup repair, integrity checks and catalog file backups
are skipped for PostgreSQL. Native compaction and native tier moves fall back to
DuckLake SQL operations for PostgreSQL. Manage PostgreSQL catalog backups and
recovery with PostgreSQL tools. SQLite-only catalog backup/restore/rebuild CLI
commands reject PostgreSQL paths rather than trying to open them as files.

## Configure tiered volumes

For a single writer catalog with tiered volumes, the priority-zero hot volume uses
the base database and other volumes use `<base_database>_<volume_name>`. Provision
those databases as well. Sharding and tiering cannot currently be combined for
PostgreSQL: the existing tier manager manages one hot catalog, and configuration
validation rejects the combination instead of attaching an empty base catalog.

## Local secrets and connection details

Catalog DSNs are redacted from config output and attachment errors. Node auth
and JWT secrets use `node.local_secrets_dir` and
`coordinator.local_secrets_dir` (default `/var/lib/homer`). Configure both to the
same writable, persistent directory, for example `/data/homer/secrets`.
Environment overrides are `HOMER_NODE_LOCAL_SECRETS_DIR` and
`HOMER_COORDINATOR_LOCAL_SECRETS_DIR`. When upgrading, move the existing
`/data/.homer_node_auth_token` and JWT secret into that directory to preserve
authentication. DSNs are never treated as filesystem directories.

## Troubleshooting

| Symptom | What to check |
| --- | --- |
| Catalog connection fails | Confirm the database exists, the Homer role owns it, and the connection details work from the writer and node. |
| PostgreSQL catalog configuration is rejected | Set `catalog_path` to a connection string when using `catalog_type=postgres`. |
| Node cannot read ingested data | Check that writer and node use the same catalog and can access the same data files. For shards, configure one node volume per shard. |
| Spill directory is missing or unwritable | Set an explicit local `tuning.temp_directory` for the process. |
| Sharding and tiering configuration is rejected | Use either PostgreSQL sharding or tiered volumes; combining them is not supported. |
| Catalog backup, restore, or rebuild CLI command rejects the path | Those commands support SQLite catalog files. Use PostgreSQL backup and recovery tools for PostgreSQL catalogs. |

## Run regression tests

With a disposable PostgreSQL server and an administrative test DSN (CREATE
DATABASE required for the tests only):

Download the checksum-verified test extensions from the repository root first
(run as the test user), then run Go tests from `src`:

```sh
./scripts/download_duckdb_extensions.sh
mkdir -p "$HOME/.duckdb/extensions"
cp -R bundled_extensions/v1.5.6 "$HOME/.duckdb/extensions/"
```

```sh
HOMER_TEST_POSTGRES_DSN='postgres://postgres:password@127.0.0.1:5432/test?sslmode=disable' \
  go test ./storage/ducklake ./node ./writer ./config ./cli ./coordinator/services -count=1 -v
```

Tests create and drop private databases. They cover shard persistence after
shutdown, independent shard databases and spill paths, maintenance attachment and
merge calls, a Parquet-backed partition move between PostgreSQL volumes and its
retry, node reads, CLI search path and completion, spill warnings, local secret
placement and persistence, URL scheme variants and keyword DSNs, and the
SQLite-specific maintenance guards. PostgreSQL integration cases skip when
`HOMER_TEST_POSTGRES_DSN` is unset; compiling alone does not execute them.

## Further reading

- [Configure PostgreSQL-backed settings](POSTGRES_SETTINGS.md)
- [DuckLake catalog connection options](https://ducklake.select/docs/stable/duckdb/usage/connecting)
