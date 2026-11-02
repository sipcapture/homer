# PostgreSQL-backed settings through DuckDB

Settings can use PostgreSQL as their persistent store while DuckDB remains the
query client. Local file-backed DuckDB remains the default. This change is
separate from the DuckLake metadata backend; see [PostgreSQL DuckLake catalogs](postgres-ducklake.md).

## Configuration

Install the PostgreSQL extension for the same DuckDB engine and OS user that runs
Homer (`homer-core --install-extensions` now includes `postgres`), then configure:

```json
{
  "coordinator": {
    "settings_db_path": "postgres://homer:password@localhost:5432/homer_settings?sslmode=require"
  }
}
```

Alternatively set `HOMER_COORDINATOR_SETTINGS_DB_PATH`. PostgreSQL URLs beginning
with `postgres://` or `postgresql://`, and libpq keyword strings such as
`host=localhost dbname=homer_settings user=homer`, select PostgreSQL automatically.
Recognized connection strings are validated before use; malformed ones fail
rather than opening a local file.

A file path such as `/var/lib/homer/homer_settings.duckdb` keeps local DuckDB
behavior, which remains the default. The PostgreSQL database must already exist;
Homer creates settings tables in `public`. Existing local settings files are not
migrated or opened when a PostgreSQL connection string is configured. For
PostgreSQL, local JWT and coordinator authentication secrets use the default
`/var/lib/homer` directory; for file-backed settings they remain beside the file.

Config display redacts the DSN unless `--include-secrets` is supplied, including
when selecting the DSN directly with `--section`. The administrator password reset
command uses the same backend configuration. The existing migration command
continues to require a local DuckDB destination; when settings use PostgreSQL,
pass a local file with `--duckdb`. Connection strings are not logged as paths.

## DuckDB PostgreSQL extension limitations

> PostgreSQL supports sequences and `RETURNING`. The workarounds here are needed
> because the DuckDB PostgreSQL extension does not expose those operations on
> attached tables in the tested versions.

The original DuckDB 1.5.0 / PostgreSQL 17 probe found:

| Operation | Result |
| --- | --- |
| Create sequence through ordinary DuckDB SQL | Unsupported |
| Create table using DuckDB `nextval` for the remote sequence | Unsupported |
| Plain insert, update, delete | Works |
| Bound reads, JSON, booleans, timestamps | Works |
| Insert, update, delete with `RETURNING` on attached tables | Unsupported |

The engine-level probe remains in `scripts/probe_postgres_settings.py`. It creates
and removes a unique test schema. Its nonzero exit status records the known
unsupported operations; the Go integration test validates the actual workaround
against the repository's DuckDB and pgx versions.

## Implementation

`OpenSettingsDB` still takes one path string and returns `*sql.DB`. When the path
is a PostgreSQL connection string, it opens an
in-memory DuckDB instance and attaches PostgreSQL's `public` schema as
`homer_settings`. Every pooled DuckDB session selects that catalog. Ordinary
service queries continue to use DuckDB and existing constructors are unchanged.

Schema initialization sends DDL through `postgres_execute` on DuckDB's extension
connection. PostgreSQL handles sequences and defaults, and the extension catalog
cache is refreshed after bootstrap. This does not add new uniqueness constraints.

Methods containing the 21 existing `RETURNING` sites select a local `queryRow`
function. It defaults to `db.QueryRowContext`; PostgreSQL mode selects
`postgresQueryRow(db)`. SQL and arguments stay at the call site. Both functions
return standard `*sql.Row` values, without service fields or custom row types.

The PostgreSQL function opens a one-query database/sql handle using pgx's stdlib
driver. After starting the query it closes the handle to new work. The active
result retains its connection until `Row.Scan` consumes and releases the rows;
that connection then closes rather than returning to an idle pool. Callers must
scan returned rows, just as with other `QueryRowContext` calls. Local DuckDB never
enters this implementation.
The four parameterized sites use numbered placeholders supported by both engines.
The standard driver returns `sql.ErrNoRows` for missing rows, preserving current behavior.
Backend configuration is kept privately on the database connector, without a
global connection registry or additional service fields.

No temporary result tables, second-query ID lookups, SQL text detection, or
cross-backend write retries are used. An error after a write has reached the server can still
have an ambiguous commit outcome, as with other database writes.

Each native query has its own connection and transaction; it cannot join a DuckDB
transaction. Existing settings services do not use explicit multi-statement
transactions. PostgreSQL writes commit before subsequent autocommit DuckDB reads.
An already-open DuckDB transaction may retain an earlier snapshot. Opening a
connection for each unsupported query adds authentication/network overhead.

## Verification

Run local regression tests from `src`:

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

The integration test creates and drops its own database. It checks schema
initialization, CRUD across the eight services with RETURNING operations,
read-after-write visibility through DuckDB, JSON and Lua content, missing rows,
pooled-session catalog selection, cancellation, and native connection cleanup.
Eight concurrent token consumers must produce exactly the configured three
successful opens, preserving the atomic update-and-return operation.

References: [DuckDB PostgreSQL extension](https://duckdb.org/docs/stable/extensions/postgres),
[upstream RETURNING limitation](https://github.com/duckdb/duckdb-postgres/issues/175).
