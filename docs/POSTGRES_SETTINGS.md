# Configure PostgreSQL-backed settings

This guide explains how to store Homer coordinator settings in PostgreSQL.
DuckDB remains the query client; a local DuckDB file is the default settings store.

Settings storage is separate from DuckLake metadata. To store lake catalogs in
PostgreSQL, follow [Configure PostgreSQL DuckLake catalogs](POSTGRES_DUCKLAKE.md).
Use a separate PostgreSQL database for each purpose.

## Before you begin

You need:

- An existing PostgreSQL database and a role that can create settings tables in its `public` schema.
- Network access to PostgreSQL from the coordinator.
- The DuckDB PostgreSQL extension installed for the DuckDB engine and OS user that run Homer.

Switching the configuration does not migrate existing local settings. Homer does
not open the previous settings file while PostgreSQL is configured.

`settings_db_type` defaults to `sqlite`, which selects the existing local DuckDB
settings file (it does not create a SQLite database); `postgresql` is an alias for `postgres`.
The configured type must match the path. PostgreSQL DSNs with type `sqlite`,
and local paths with type `postgres`, are rejected. The environment override is
`HOMER_COORDINATOR_SETTINGS_DB_TYPE`.

## 1. Install the extension

Run this command as the OS user that runs Homer:

```sh
homer-core system --install-extensions
```

The command includes the `postgres` extension.

## 2. Configure the coordinator

Set `coordinator.settings_db_type` to `postgres` and configure
`coordinator.settings_db_path` in your Homer configuration. Replace the host,
credentials, and database name with your PostgreSQL connection details:

```json
{
  "coordinator": {
    "settings_db_type": "postgres",
    "settings_db_path": "postgres://homer:password@localhost:5432/homer_settings?sslmode=require"
  }
}
```

Alternatively, set the environment variable:

```sh
export HOMER_COORDINATOR_SETTINGS_DB_TYPE=postgres
export HOMER_COORDINATOR_SETTINGS_DB_PATH='postgres://homer:password@localhost:5432/homer_settings?sslmode=require'
```

Homer recognizes both `postgres://` and `postgresql://` URLs. Libpq keyword
strings also work, for example:

```text
host=localhost dbname=homer_settings user=homer
```

Recognized connection strings are validated before use. A malformed connection
string fails instead of opening a local file.

To use local DuckDB storage, set a file path such as
`/var/lib/homer/homer_settings.duckdb`.

## 3. Start Homer and verify settings

Start or restart the coordinator with the updated configuration. Homer creates
the settings tables in the PostgreSQL database's `public` schema.

Use the coordinator UI to create or update a setting, then reload it to verify
that it can be read back. Restart the coordinator and confirm the setting is
still present.

## Manage settings and local secrets

The administrator password reset command uses the configured settings backend.
The existing migration command requires a local DuckDB destination; when your
coordinator uses PostgreSQL, supply a local file with `--duckdb`.

With PostgreSQL settings or catalogs, configure the same writable, persistent
local directory for generated JWT and node/coordinator authentication secrets:

```json
{
  "node": {"local_secrets_dir": "/data/homer/secrets"},
  "coordinator": {"local_secrets_dir": "/data/homer/secrets"}
}
```

The default is `/var/lib/homer`. Environment overrides use
`HOMER_NODE_LOCAL_SECRETS_DIR` and `HOMER_COORDINATOR_LOCAL_SECRETS_DIR`.
Mount the directory with write permissions for the Homer process. Move existing
generated secrets into it to preserve tokens and JWT sessions.
With file-backed settings, secrets remain beside the settings file unless
`local_secrets_dir` is explicitly configured.

Config display redacts the connection string unless you supply
`--include-secrets`, including when selecting it directly with `--section`.
Connection strings are not logged as file paths.

## Troubleshooting

| Symptom | What to check |
| --- | --- |
| Settings initialization fails | Confirm that the database already exists, the credentials work, and the role can create tables in `public`. |
| PostgreSQL extension cannot load | Install extensions using the same DuckDB engine and OS user that run Homer. |
| Existing settings are missing after switching | Local settings are not migrated automatically. Check which backend is configured. |
| Local authentication secrets cannot be saved | Check permissions and persistence for the configured `local_secrets_dir` (default `/var/lib/homer`). |
| Migration command rejects PostgreSQL | Supply a local DuckDB destination with `--duckdb`. |

## Query behavior

Most settings queries run through DuckDB's PostgreSQL attachment. Operations that
need `RETURNING` use a native PostgreSQL connection for that query. Each native
query has its own connection and transaction, adding authentication and network
overhead.

These queries cannot join a DuckDB transaction. Existing settings services do not
use explicit multi-statement transactions. Writes commit before subsequent
DuckDB autocommit reads; an already-open DuckDB transaction may retain an earlier
snapshot. As with other database writes, an error after a write reaches the
server can leave its commit outcome uncertain.

## Run regression tests

These commands are for development verification. The integration test creates
and drops its own database, so use a disposable server and a test role with
`CREATE DATABASE` privileges.

Run local regression tests from `src`:

Download the checksum-verified test extensions from the repository root first
(run as the test user), then run Go tests from `src`:

```sh
./scripts/download_duckdb_extensions.sh
mkdir -p "$HOME/.duckdb/extensions"
cp -R bundled_extensions/v1.5.6 "$HOME/.duckdb/extensions/"
```

```sh
go test ./config ./coordinator/services ./coordinator/handlers ./coordinator ./cli
```

For integration tests, start a disposable PostgreSQL server:

```sh
docker run --detach --name homer-settings-pg-probe \
  --publish 127.0.0.1:55432:5432 \
  --env POSTGRES_PASSWORD=homer_probe \
  --env POSTGRES_DB=homer_probe postgres:17

docker exec homer-settings-pg-probe pg_isready -U postgres -d homer_probe

# Run from src; the test role needs CREATE DATABASE privileges.
HOMER_TEST_POSTGRES_DSN='postgres://postgres:homer_probe@127.0.0.1:55432/homer_probe?sslmode=disable' \
  go test ./coordinator/services -run 'TestPostgresSettingsServices' -count=1 -v

docker rm --force --volumes homer-settings-pg-probe
```

The integration test covers schema initialization, settings CRUD,
read-after-write visibility, JSON and Lua content, missing rows, pooled sessions,
cancellation, connection cleanup, and concurrent token consumption.

## Further reading

- [Configure PostgreSQL DuckLake catalogs](POSTGRES_DUCKLAKE.md)
- [DuckDB PostgreSQL extension](https://duckdb.org/docs/stable/extensions/postgres)
- [Upstream RETURNING limitation](https://github.com/duckdb/duckdb-postgres/issues/175)
