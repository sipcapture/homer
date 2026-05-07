# Lua Call-ID Correlation

Homer-core ships with a **coordinator-side Lua correlation engine** that
lets operators merge related dialogs (B2B legs, retransmitted REGISTERs,
cross-node transactions, …) into a single transaction view without
changing the UI or the storage layer.

The contract is simple: when the user clicks a call-id, the coordinator
runs a small Lua script, collects extra session ids from it and
re-queries the data layer.

- Engine code: `src/scripting/correlation/`
- Bundled default templates: `src/coordinator/correlation_seed.go`
- Handler hook: `src/coordinator/handlers/transactions_v4.go`
  (`queryTransactionMessages`)
- CRUD UI: **Settings → Scripts** (`src/ui/src/settings/ScriptsPanel.tsx`)
- Package README (developer-facing): `src/scripting/correlation/README.md`

---

## 1. How it works

```
┌─────────────┐    POST /api/v4/transactions/messages
│   Homer UI  │────────────────────────────────────────────┐
└─────────────┘                                            │
                                                            ▼
                         ┌──────────────────────────────────────────┐
                         │              Coordinator                  │
                         │                                           │
                         │   1. Base SELECT on hep_proto_<P>_<E>     │
                         │      WHERE session_id IN (…)              │
                         │                                           │
                         │   2. correlation.Has(P, E)?  ──── no ──► return base rows
                         │                │                          │
                         │               yes                         │
                         │                ▼                          │
                         │   3. correlate(data, nodes, ctx)  (Lua)   │
                         │        │                                  │
                         │        ├─ executeSQL(sql)                 │
                         │        ├─ getDataByField(...)             │
                         │        └─ returns extra session_ids       │
                         │                                           │
                         │   4. merge base + extras, re-query        │
                         │   5. return expanded rows                 │
                         └──────────────────────────────────────────┘
```

Key properties:

- **Transparent.** The UI calls the same endpoint as before; correlation
  kicks in only when a script is active for the requested
  `(hepid, profile)` pair.
- **Fail-open.** Any script error, timeout, validator rejection or SQL
  failure during the second pass is logged and the handler returns the
  base rows. Users never see a 500 caused by correlation.
- **Sandboxed SQL.** Every statement a script runs goes through
  `sqlvalidator` (SELECT/WITH/SHOW/DESCRIBE/EXPLAIN/PRAGMA only,
  single-statement, no DML/DDL, no filesystem/network functions) with
  row caps, a per-request call counter and a context timeout.
- **Keyed cache.** Scripts are compiled once, replaced atomically on
  reload, and looked up by `"<hepid>_<profile>"`. Non-matching requests
  pay no Lua cost.

---

## 2. Configuration

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

| Key                 | Default | Description                                                              |
| ------------------- | ------- | ------------------------------------------------------------------------ |
| `enable`            | `false` | Build the engine and install the handler hook. Zero cost when disabled.  |
| `sql_timeout_ms`    | `5000`  | Per-`executeSQL` timeout.                                                |
| `script_timeout_ms` | `3000`  | Total per-request Lua budget.                                            |
| `max_sql_calls`     | `16`    | Max `executeSQL` invocations per correlation run.                        |
| `max_sql_rows`      | `10000` | Row cap per `executeSQL` response.                                       |
| `sync_interval_sec` | `30`    | Period between reloading `correlation_scripts` into the in-memory cache.         |
| `seed_default`      | `true`  | Insert disabled default templates for `1_call` + `1_registration`.       |

When `enable=false` the engine is never built, the handler short-circuits
on `correlation == nil`, and both runtime and memory overhead are zero.

`seed_default` runs **independently** of `enable` — even with the engine
off, the two default rows are inserted (as `status=FALSE`) so the
**Settings → Scripts** UI is never empty on a fresh install. The seeds
are inert until an operator flips `status` to `true`.

---

## 3. Script storage

Scripts live in the **settings DuckDB** (not in the DuckLake data
catalogue), table `correlation_scripts`. CRUD is exposed under `/api/v4/scripts*`
and the UI editor at **Settings → Scripts** (CodeMirror 6 with Lua
syntax highlighting).

| Column      | Meaning                                                        |
| ----------- | -------------------------------------------------------------- |
| `guid`      | Stable identifier used in the REST URL.                        |
| `profile`   | `event_type` string (`call`, `registration`, `default`, …).    |
| `hep_alias` | Human label shown in the UI (`SIP`, `RTCP`, …).                |
| `type`      | Must be `correlation` for the engine to pick the row up.       |
| `hepid`     | Numeric HEP protocol id (SIP=1, RTCP=5, RTP=35, LOG=100, …).   |
| `status`    | Must be `TRUE` for the script to be active.                    |
| `script`    | Lua source.                                                    |

The in-memory cache key is `"<hepid>_<profile>"`, e.g. `1_call` for SIP
calls. Only rows with `type='correlation' AND status=TRUE` are loaded.

The cache is reloaded on start-up and every `sync_interval_sec` — edits
become active without restart.

---

## 4. Lua API

Each request builds a **fresh Lua state**, registers the helpers below,
executes the user script with `DoString`, and then calls `correlate`.
`lua.State` is not goroutine-safe; per-request state keeps the engine
correct at the cost of some allocations (acceptable for click-level
traffic).

### 4.1 Entrypoint

```lua
function correlate(data, nodes, ctx)
  -- data:  array of row tables (the result of the base SELECT).
  --        Each row is a table keyed by column name and carries the full
  --        row from hep_proto_<hepid>_<profile> — session_id, caller,
  --        callee, ruri_user, auth_user, src_ip/dst_ip, src_port/dst_port,
  --        method, response_code, timestamp, data_extra, etc. data_extra
  --        is a JSON string — use json_extract_string(...) in SQL to read
  --        it, or parse it in Lua if you prefer.
  -- nodes: array of configured node aliases (informational).
  -- ctx:   request-level context table:
  --          ctx.time_from    int64 (ms)   user timerange start
  --          ctx.time_to      int64 (ms)   user timerange end
  --          ctx.session_ids  {str,...}    callids from the request body
  --          ctx.hepid        int          e.g. 1 (SIP)
  --          ctx.profile      string       e.g. "call"
  --          ctx.proto_type   int          mirrors hepid
  --          ctx.event_type   string       mirrors profile
  --          ctx.nodes        {str,...}    same as `nodes` arg
  --        Added in homer-core 11.0.64. Scripts written against the
  --        older two-argument signature (`correlate(data, nodes)`) keep
  --        working — Lua silently drops extra arguments.
  -- return: array of extra session_id strings to merge into the
  --         transaction. Duplicates and empty strings are dropped by
  --         the caller.
  return { "call-id-2", "call-id-3" }
end
```

#### Why ctx matters

The base SELECT the coordinator runs before calling your script is
already bounded to `ctx.time_from..ctx.time_to`, but any follow-up
`executeSQL` you issue is **not** — it is your script's responsibility
to re-apply that window. On a busy DuckLake a correlation query that
forgets the timerange will happily scan months of partitions for one UI
click.

The recommended pattern is:

```lua
local from_ms = (type(ctx) == "table" and tonumber(ctx.time_from)) or 0
local to_ms   = (type(ctx) == "table" and tonumber(ctx.time_to))   or 0

local rows = getDataByField(1, "call", "session_id", corr_ids, from_ms, to_ms)
```

Both bundled default templates use exactly this pattern.

### 4.2 `executeSQL(sql) -> rows`

Runs `sql` against the data nodes via `FlightService`. All of:

- `sqlvalidator.ValidateRawSQL`
- forced `LIMIT max_sql_rows` when the statement is LIMIT-able and
  missing one
- per-call timeout (`sql_timeout_ms`)
- per-request call quota (`max_sql_calls`)

are enforced. When blocked, `rows` is **an empty table** — scripts
should use `#rows == 0` instead of `rows == nil` because luar proxies
a Go nil slice as non-nil userdata.

### 4.3 `getDataByField(proto, event, field, values, from_ms, to_ms) -> rows`

Typed shortcut that builds:

```sql
SELECT *
FROM <lake>.hep_proto_<proto>_<event>
WHERE <field> IN (<values>)
  AND timestamp BETWEEN <from_ms> AND <to_ms>
```

`field` and `event` are matched against a strict `[A-Za-z0-9_]+`
whitelist; values are quoted with `sqlvalidator.SafeString`. Pass
`ctx.time_from, ctx.time_to` to bound the lookup to the user's visible
timerange (strongly recommended on a busy lake); passing `0, 0` skips
the timestamp filter and scans every partition the executor exposes.

### 4.4 `scriptLog(level, message)`

Writes to the process logger at `debug | info | warn | error`. Also
captured in `CorrelationResult.Debug` (reserved for a future
`?debug=1` mode on the transactions endpoint).

### 4.5 `HashString(algo, s) -> hex`

Returns `md5 | sha1 | sha256` of `s` as hex. Unknown algos return `s`
unchanged.

### 4.6 `HashTable(op, key, val)`

Process-wide memoisation helper backed by `VictoriaMetrics/fastcache`
(32 MiB budget):

- `HashTable("set", k, v)` — store
- `HashTable("get", k, "")` — read (returns empty string if absent)
- `HashTable("del", k, "")` — remove

Use it to cache expensive lookups across clicks, for example a
database→vendor-name mapping.

---

## 5. Bundled default templates

When `seed_default=true` the coordinator inserts two disabled templates
on first start-up, one per `(hepid, profile)` pair. Rows are keyed by
`(hepid, profile, type='correlation')` so operator-authored scripts are
never overwritten, and only missing defaults are re-inserted on
upgrade.

| hepid | profile        | alias | Purpose                                                                                               |
| ----- | -------------- | ----- | ----------------------------------------------------------------------------------------------------- |
| 1     | `call`         | SIP   | B2B call-id correlation via `x_call_id` / `correlation_id`. Heavy inline comments + `executeSQL` example. |
| 1     | `registration` | SIP   | Group REGISTERs by Address-of-Record using the typed `getDataByField` helper.                         |

Both seeds ship with `status=FALSE`. Open **Settings → Scripts**, read
the inline comments (each template documents every available helper)
and flip `status` to `true` to activate.

### 5.1 How `correlation_id` maps into homer-core

The homer-core SIP writer stores the `X-Call-ID` custom header inside
the **`data_extra` JSON** column as `x_call_id` (see
`src/storage/ducklake/hep_adapter.go:480`), which is the value a
B2B-aware script needs to correlate the peer leg. Some deployments
additionally project that value into a top-level `correlation_id`
column via a custom mapping.

The default call template transparently supports both layouts:

- In Lua it reads `row.correlation_id` first, then falls back to
  `row.x_call_id` / `row.xcid` / `row.callid_aleg`.
- In SQL it uses `json_extract_string(data_extra, '$.x_call_id')`,
  with a commented-out alternative for deployments that expose a
  top-level `correlation_id` column.

### 5.2 Default call correlation (short walk-through)

```lua
function correlate(data, nodes, ctx)
  local extras, seen = {}, {}

  local from_ms = (type(ctx) == "table" and tonumber(ctx.time_from)) or 0
  local to_ms   = (type(ctx) == "table" and tonumber(ctx.time_to))   or 0

  -- 1) Pull base session_ids and their correlation_ids from the rows.
  local base_sids, seen_sid = {}, {}
  local corr_ids,  seen_cid = {}, {}
  for i = 1, #data do
    local row = data[i]
    if type(row) == "table" then
      local sid = row.session_id or row.call_id
      if type(sid) == "string" and #sid > 0 and not seen_sid[sid] then
        seen_sid[sid] = true
        base_sids[#base_sids + 1] = sid
      end

      local cid = row.correlation_id or row.x_call_id
               or row.xcid or row.callid_aleg
      if type(cid) == "string" and #cid > 0 and not seen_cid[cid] then
        seen_cid[cid] = true
        corr_ids[#corr_ids + 1] = cid
        extras[#extras + 1] = cid  -- return the peer leg immediately
        seen[cid] = true
      end
    end
  end

  -- 2) Ask the DB for every other session_id linked to our base sids
  --    via data_extra.x_call_id, or whose own session_id equals one
  --    of our correlation_ids.
  if #base_sids == 0 and #corr_ids == 0 then return extras end

  local function quote_list(a)
    local p = {}
    for i = 1, #a do p[i] = "'" .. tostring(a[i]):gsub("'", "''") .. "'" end
    return table.concat(p, ",")
  end

  local parts = {}
  if #base_sids > 0 then
    parts[#parts+1] = "json_extract_string(data_extra, '$.x_call_id') IN ("
                   .. quote_list(base_sids) .. ")"
  end
  if #corr_ids > 0 then
    parts[#parts+1] = "session_id IN (" .. quote_list(corr_ids) .. ")"
  end

  local sql = "SELECT DISTINCT session_id FROM homer_lake.hep_proto_1_call"
           .. " WHERE (" .. table.concat(parts, " OR ") .. ")"
  if from_ms > 0 and to_ms > 0 and from_ms < to_ms then
    sql = sql
       .. " AND timestamp >= (to_timestamp(" .. tostring(from_ms) .. " / 1000.0) AT TIME ZONE 'UTC')"
       .. " AND timestamp <= (to_timestamp(" .. tostring(to_ms)   .. " / 1000.0) AT TIME ZONE 'UTC')"
  end

  local rows = executeSQL(sql) or {}
  for i = 1, #rows do
    local s = rows[i].session_id
    if type(s) == "string" and #s > 0 and not seen[s] then
      seen[s] = true
      extras[#extras + 1] = s
    end
  end
  return extras
end
```

The bundled template is the same flow, expanded with B2BUA-suffix
stripping (`_b2b-NNN`) and commented alternatives for
`getDataByField` and native `correlation_id` columns.

### 5.3 Default registration correlation (short walk-through)

```lua
function correlate(data, nodes, ctx)
  local from_ms = (type(ctx) == "table" and tonumber(ctx.time_from)) or 0
  local to_ms   = (type(ctx) == "table" and tonumber(ctx.time_to))   or 0

  local aors, seen_aor = {}, {}
  for i = 1, #data do
    local row = data[i]
    local a = row.aor or row.callee or row.ruri_user or row.auth_user
    if type(a) == "string" and #a > 0 and not seen_aor[a] then
      seen_aor[a] = true
      aors[#aors + 1] = a
    end
  end
  if #aors == 0 then return {} end

  -- Typed helper: no hand-written SQL, safer quoting, timerange-bounded.
  local rows = getDataByField(1, "registration", "aor", aors, from_ms, to_ms) or {}
  local extras, seen = {}, {}
  for i = 1, #rows do
    local s = rows[i].session_id
    if type(s) == "string" and #s > 0 and not seen[s] then
      seen[s] = true
      extras[#extras + 1] = s
    end
  end
  return extras
end
```

---

## 6. Recipes for writing your own scripts

### 6.1 Correlate by a custom column

If your deployment exposes `X-CID` (or any other header) as a real
top-level column:

```lua
local sids = {}
for i = 1, #data do sids[#sids+1] = data[i].session_id end

local rows = getDataByField(1, "call", "correlation_id", sids, 0, 0) or {}
local out = {}
for i = 1, #rows do out[#out+1] = rows[i].session_id end
return out
```

### 6.2 Correlate SIP ↔ ISUP (cross-protocol)

```lua
local ids = {}
for i = 1, #data do
  local s = data[i].session_id
  if type(s) == "string" then ids[#ids+1] = s end
end
-- Jump from hepid=1 (SIP) to hepid=1000 (ISUP) using a shared key
local rows = getDataByField(1000, "default", "correlation_id", ids, 0, 0) or {}
local out = {}
for i = 1, #rows do out[#out+1] = rows[i].session_id end
return out
```

### 6.3 Cache an expensive lookup per click

```lua
local key = "carrier:" .. (data[1].dst_ip or "")
local cached = HashTable("get", key, "")
if #cached > 0 then
  scriptLog("debug", "cache hit for " .. key)
else
  local rows = executeSQL(
    "SELECT carrier FROM homer_lake.main.hep_dict_carriers "
    .. "WHERE ip = '" .. (data[1].dst_ip or ""):gsub("'", "''") .. "' LIMIT 1")
  cached = (rows and rows[1] and rows[1].carrier) or ""
  HashTable("set", key, cached)
end
-- use `cached` to refine your correlation query …
```

### 6.4 Debug logging

```lua
scriptLog("info", string.format("correlate: #data=%d #nodes=%d", #data, #nodes))
```

Output lands in the coordinator log under the `correlation` component.

---

## 7. Security & limits

| Concern                               | Mitigation                                                                                |
| ------------------------------------- | ----------------------------------------------------------------------------------------- |
| Arbitrary SQL injection               | `sqlvalidator.ValidateRawSQL` rejects non-SELECT, multiple statements, dangerous fns.     |
| Table-identifier injection            | `field`/`event` parameters must match `^[A-Za-z0-9_]+$`.                                  |
| Unbounded result sets                 | `max_sql_rows` appended as `LIMIT` when absent.                                           |
| Runaway scripts                       | `script_timeout_ms` via `context.WithTimeout`.                                            |
| `executeSQL` abuse loop               | `max_sql_calls` counter per request.                                                      |
| Settings DB exposure                  | `executeSQL` only hits the data path (`FlightService`); the settings DB is unreachable.   |
| Cross-tenant leakage                  | Scripts run after the handler has already applied the user's time/search filters.         |
| Hot path slowdown                     | Correlation runs only on the `/transactions/messages` click path, never in ingest.        |

---

## 8. Troubleshooting

**"My script never runs."**
Check:
1. `coordinator.correlation.enable = true` in config.
2. Row exists in `correlation_scripts` with `type='correlation'`, `status=TRUE`,
   correct `hepid` and `profile`.
3. Cache reload time (`sync_interval_sec`) has elapsed, or restart the
   coordinator. The log line `correlation: reloaded scripts count=N`
   confirms the reload.
4. The handler log shows `correlation: has script for key=<hepid>_<profile>`.

**"`rows == nil` check doesn't work."**
Use `if rows == nil or #rows == 0 then …` — a Go nil slice crosses the
luar bridge as a non-nil userdata.

**"executeSQL returns nothing but the query works in DuckDB."**
Most likely `sqlvalidator` rejected it (multi-statement, DDL, file
functions). Search the coordinator log for `correlation: SQL rejected`.

**"My time window is ignored."**
`executeSQL` does not inject a timestamp filter — write it yourself, or
use `getDataByField(..., from_ms, to_ms)` which does. Remember these are
**milliseconds since epoch**.

**"Results include the legs I was already viewing."**
The caller dedupes against the base `session_ids` before the second
`SELECT`; returning them is harmless but increases the cap pressure
against `maxTransactionSessionIDs`. Strip them in Lua if you care.

---

## 9. Related reading

- `src/scripting/correlation/README.md` — developer notes, cache
  internals, test harness.
- `docs/COORDINATOR.md` — API surface; `POST /api/v4/transactions/messages`
  entry explicitly mentions Lua correlation.
- `docs/SEARCH.md` — how the base `session_id` filter is constructed.
- `src/coordinator/docs/openapi.yaml` — authoritative contract for
  `/api/v4/scripts*` and `/api/v4/transactions/messages`.
