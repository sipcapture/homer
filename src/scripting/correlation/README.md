# Lua call-id correlation engine

Example copies of the default SIP correlation scripts live in
[`examples/lua/correlation/`](../../examples/lua/correlation/) (mirrors
`src/coordinator/correlation_seed_lua/` used by `go:embed`).

This package implements the coordinator-side Lua correlation used by
`POST /api/v4/transactions/messages`.

When a user clicks on a call-id in the UI the frontend calls
`/api/v4/transactions/messages` as usual. If a correlation script is
registered and active for the requested `(proto_type, event_type)` pair,
the handler:

1. Runs the base query (same SQL as before).
2. Hands the resulting rows to the Lua function `correlate(data, nodes)`.
3. Takes the additional `session_id` values the script returns, merges them
   into the request and re-issues the query.
4. Returns the expanded row set, in the same response shape as before.

Script execution is sandboxed: SQL goes through `sqlvalidator`, has a row
cap, a per-request call counter and a context timeout. Script failure is
never fatal — the handler logs and returns the base rows.

## Configuration

Add the following under `coordinator` in your Homer config:

```json
{
  "coordinator": {
    "correlation": {
      "enable": true,
      "sql_timeout_ms": 5000,
      "script_timeout_ms": 3000,
      "max_sql_calls": 16,
      "max_sql_rows": 10000,
      "sync_interval_sec": 30,
      "seed_default": true
    }
  }
}
```

| Key                 | Default | Description                                                             |
| ------------------- | ------- | ----------------------------------------------------------------------- |
| `enable`            | `false` | Build the engine and install the handler hook. Zero cost when disabled. |
| `sql_timeout_ms`    | `5000`  | Per-`executeSQL` timeout.                                               |
| `script_timeout_ms` | `3000`  | Total per-request Lua budget.                                           |
| `max_sql_calls`     | `16`    | Max `executeSQL` invocations per correlation run.                       |
| `max_sql_rows`      | `10000` | Row cap per `executeSQL` response.                                      |
| `sync_interval_sec` | `30`    | Period between reloading `correlation_scripts` into the in-memory cache.        |
| `seed_default`      | `true`  | Insert disabled default templates for `1_call` + `1_registration`. Runs independently of `enable`. |

## Script storage

Scripts live in the settings DuckDB table `correlation_scripts`, manageable via
`/api/v4/scripts`:

| Column  | Meaning                                                                  |
| ------- | ------------------------------------------------------------------------ |
| `guid`  | Stable identifier for CRUD.                                              |
| `hepid` | Numeric HEP protocol id (SIP=1, RTCP=5, …).                              |
| `profile` | `event_type` string (`call`, `default`, `registration`, …).            |
| `type`    | Must be `correlation` for the engine to pick the row up.               |
| `status`  | Must be `true` for the script to be active.                            |
| `script`  | Lua source.                                                            |

Cache key is `"<hepid>_<profile>"`, e.g. `1_call` for SIP calls.

## Lua API

Each request builds a fresh Lua state, registers the helpers below,
executes the user script with `DoString`, and then calls `correlate`.

```lua
function correlate(data, nodes)
    -- data:  array of row tables (result of the base SELECT)
    -- nodes: array of configured node aliases (informational)
    -- return: array of extra session_id strings
    return { "call-id-2", "call-id-3" }
end
```

### `executeSQL(sql) -> rows`

Runs `sql` against the data nodes via the same path used by the rest of
`/api/v4/transactions/*`. All of:

- `sqlvalidator.ValidateRawSQL` (SELECT/WITH/SHOW/DESCRIBE/EXPLAIN/PRAGMA,
  no semicolons, no DML/DDL, no filesystem/network functions),
- forced `LIMIT max_sql_rows` if the statement is LIMIT-able and missing one,
- per-call timeout (`sql_timeout_ms`),
- per-request call quota (`max_sql_calls`),

are enforced. When blocked, `rows` is an empty table — scripts should check
`#rows == 0` rather than `rows == nil` because luar proxies a Go nil slice
as a non-nil userdata.

### `getDataByField(proto, event, field, values, from_ms, to_ms) -> rows`

Parameterised shortcut that builds a safe
`SELECT * FROM <lake>.hep_proto_<proto>_<event> WHERE <field> IN (...) AND timestamp BETWEEN ...`
query. `field` and `event` are checked against a strict identifier
whitelist; values are quoted with `sqlvalidator.SafeString`.

### `scriptLog(level, message)`

Emits a message to the process logger; also captured in
`CorrelationResult.Debug` (not currently returned to the client; reserved
for a future debug mode).

### `HashString(algo, s)`

Returns `md5`/`sha1`/`sha256` of `s` as hex. Unknown algos return `s`
unchanged.

### `HashTable(op, key, val)`

Process-wide memoisation helper:

- `HashTable("set", k, v)` writes,
- `HashTable("get", k, "")` reads (empty string if absent),
- `HashTable("del", k, "")` removes.

Backed by `VictoriaMetrics/fastcache` with a 32 MiB budget.

## Enabling a script

1. Create/update a row via `POST /api/v4/scripts`:

   ```json
   {
     "hepid": 1,
     "profile": "call",
     "hep_alias": "SIP",
     "type": "correlation",
     "status": false,
     "script": "function correlate(data, nodes) ... end"
   }
   ```

2. Review and then flip `status` to `true` via `PUT /api/v4/scripts/{guid}`.

The cache is reloaded from `correlation_scripts` on startup and every
`sync_interval_sec` thereafter — changes become active without restart.

## Default seed

When `seed_default` is `true` the coordinator inserts two disabled
templates on start-up, one per `(hepid, profile)` pair, but only when a
row for that pair does not already exist:

| hepid | profile        | alias | what it does                                                                 |
|-------|----------------|-------|------------------------------------------------------------------------------|
| 1     | `call`         | SIP   | Correlates B2B call-ids via `x_call_id` / `correlation_id` (heavy commented example with `executeSQL`). |
| 1     | `registration` | SIP   | Correlates REGISTERs by Address-of-Record using the typed `getDataByField` helper.                      |

Both seeds ship with `status=false`. Open Settings → Scripts, read the
inline comments (each template documents every available helper) and
flip `status` to `true` to activate.

Re-running the seeder is safe: rows are keyed by `(hepid, profile,
type='correlation')`, so an operator-authored script is never
overwritten and only missing defaults are re-inserted.

## Development notes

- `lua.State` is not goroutine-safe. The engine builds one state per
  request (cheap for click-level traffic, not acceptable for per-packet
  hot paths). A sync.Pool-based reuse is a straightforward follow-up.
- `settingsDB` is deliberately **not** exposed to scripts; `executeSQL`
  only hits the DuckLake data nodes via `FlightService`.
- The handler hook is fail-open: any correlation error logs a warning and
  the user sees the base rows, never a 500.
