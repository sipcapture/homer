# InfluxDB Line Protocol Receiver

homer-core can ingest **InfluxDB Line Protocol** over HTTP and route
the points into **dynamically-created DuckLake tables**. The receiver
speaks the InfluxDB v1 (`/write`) and v2 (`/api/v2/write`) write APIs
plus a gigapi-compatible alias (`/api/v3/write_lp`), so existing
Telegraf, line-protocol agents, and Influx clients work without
modification.

Unlike OTLP / HEP — both of which write into fixed schemas — Line
Protocol tables are **created on the fly per measurement** and **grow
their column set** as new fields appear. This makes the receiver a
good fit for application metrics, host telemetry, and any
"measurement-shaped" workload that doesn't have a stable schema up
front.

## Architecture

```
                    ┌──────────────────────────────────────────────┐
   InfluxDB         │           lineprotoreceiver.Module           │
   client / curl    │                                              │
   ─────────────▶  │  ┌──────────┐    ┌─────────────────────────┐ │
   /write           │  │  HTTP    │───▶│         Ingester        │ │
   /api/v2/write    │  │  server  │    │                         │ │
   /api/v3/write_lp │  │ (:8086)  │    │  parse → group by       │ │
                    │  └──────────┘    │  measurement → DDL +    │ │
                    │                  │  bulk INSERT            │ │
                    │                  └────────────┬────────────┘ │
                    └───────────────────────────────┼──────────────┘
                                                    ▼
                                ┌────────────────────────────────────┐
                                │     DuckLake (writer-owned)        │
                                │  ────────────────────────────────  │
                                │  schema = sanitised ?db= (or main) │
                                │                                    │
                                │  table  = <table_prefix><measurement> │
                                │  (default: no prefix = measurement    │
                                │  name; optional e.g. lp_; see Config) │
                                │  columns evolve via                │
                                │  ALTER TABLE ADD COLUMN IF NOT     │
                                │  EXISTS … as new fields appear     │
                                └────────────────────────────────────┘
```

The module is wired in `src/main.go` and only starts when the **writer
module is also enabled** — the ingester needs DuckLake's `*sql.DB` and
`lakeName` from the writer's `DuckLakeManager`. A coordinator-only
process will not start this listener.

## Configuration

```json
{
  "ingest": {
    "line_protocol": {
      "enable": false,
      "listen": ":8086",
      "max_body_bytes": 8388608,
      "default_precision": "ns",
      "default_db": "",
      "allow_hep_sip_call": false,
      "read_timeout_sec": 30,
      "write_timeout_sec": 30,
      "cert": "",
      "key": "",
      "cacert": ""
    }
  }
}
```

Omit **`table_prefix`** in JSON (or set **`""`**) for the default: the DuckLake table name equals the sanitised measurement. Set **`"table_prefix": "lp_"`** only if you want a namespace prefix on every LP table.

| Field                | Default     | Description                                                                                       |
|----------------------|-------------|---------------------------------------------------------------------------------------------------|
| `enable`             | `false`     | Master switch. When `false` the module is not constructed and `:8086` stays free.                |
| `listen`             | `:8086`     | HTTP bind address. `:8086` matches InfluxDB v1's default.                                         |
| `max_body_bytes`     | `8388608`   | Inbound request body cap (8 MiB — same as InfluxDB v2's `max-body-size`). Over-limit ⇒ `413`.    |
| `default_precision`  | `ns`        | Used when the client doesn't pass `?precision=`. Valid: `ns`, `us`, `ms`, `s`.                    |
| `default_db`         | empty       | Used when the client doesn't pass `?db=` / `?bucket=`. Empty ⇒ DuckDB's default schema (`main`). |
| `table_prefix`       | empty       | Prepended to the (sanitised) measurement name to form the DuckLake table name. **Default is empty** — the table name matches the measurement (after sanitisation). Set e.g. **`"lp_"`** if you want LP tables namespaced away from HEP tables. **Warning:** with an empty prefix, a measurement such as `hep_proto_1_call` names the real HEP table; the generic LP path may run `ALTER TABLE … ADD COLUMN` and corrupt the fixed HEP schema — use **`allow_hep_sip_call`** with the **exact** `hep_proto_*` measurement (see below), or use a non-empty prefix. |
| `allow_hep_sip_call` | `false`     | When `true`, a write body that contains **only** lines whose measurement is a **known** HEP table name (`hep_proto_1_call`, `hep_proto_1_registration`, `hep_proto_1_default`, `hep_proto_5_default`, `hep_proto_34_default`, `hep_proto_35_default`, `hep_proto_53_default`, `hep_proto_100_default`, … — same set as DuckLake `GetTableSchemas`) is inserted into `{lake}.main.<measurement>` using the fixed HEP column mapping for that table. All lines in one request must use the **same** measurement (one table per POST). Unknown `hep_proto_*` names are rejected. When `false`, any measurement starting with **`hep_proto_`** is rejected if **`table_prefix`** is empty. The query parameter **`hep_table`** is not supported and returns **400** if present. |
| `read/write_timeout_sec` | `30`    | HTTP server timeouts.                                                                             |
| `cert` / `key`       | empty       | Optional TLS material; both empty disables HTTPS.                                                 |
| `cacert`             | empty       | If set together with `cert`+`key`, mTLS is enforced.                                              |

### TLS / mTLS

Same convention as the rest of homer-core:

- empty `cert`+`key` → plain HTTP (use behind a trusted ingress);
- `cert`+`key` → HTTPS;
- `cert`+`key`+`cacert` → mTLS (clients must present a cert chained to `cacert`).

## HTTP endpoints

| Method | Path                  | Compatibility            |
|--------|-----------------------|--------------------------|
| POST   | `/write`              | InfluxDB v1 write API    |
| POST   | `/api/v2/write`       | InfluxDB v2 write API    |
| POST   | `/api/v3/write_lp`    | gigapi-compatible alias  |
| GET    | `/ping`               | InfluxDB v1 health probe |
| GET    | `/health`             | InfluxDB v2 health probe |

### Optional: structured `hep_proto_*` (real HEP tables)

When **`ingest.line_protocol.allow_hep_sip_call`** is `true`, send Line Protocol whose **measurement equals the DuckLake table name** on every line (Influx measurement names are case-sensitive — use the exact name, e.g. **`hep_proto_1_call`**, **`hep_proto_1_registration`**). Each line becomes one row in **`{lakeName}.main.<measurement>`** with the same column layout as HEP ingest for that table. **`?db=` / `?bucket=`** are ignored for this path (rows always land under `main`).

If **`allow_hep_sip_call`** is `false` (default), measurements starting with **`hep_proto_`** are **rejected** when **`table_prefix`** is empty, so generic LP cannot touch HEP tables; with a non-empty prefix, `hep_proto_1_call` becomes a separate LP table (e.g. `lp_hep_proto_1_call`).

You cannot mix **structured `hep_proto_*`** lines with other measurements, or **two different `hep_proto_*` table names**, in a **single** POST when **`allow_hep_sip_call`** is `true` — the server returns **400**.

**Removed:** the **`?hep_table=…`** query parameter. If a client still sends it, the server returns **400** with a short migration hint.

**Security:** an exposed LP port with `allow_hep_sip_call=true` allows unauthenticated append to the configured HEP tables unless you use network controls or mTLS (`cacert`).

**Column mapping:** tags and fields are merged (Influx-sanitised identifiers). Use **quoted** strings for text fields (`caller="alice"`). **`timestamp`** can be the line-ending epoch (as in normal LP), a tag/field `timestamp` with RFC3339 / `YYYY-MM-DD HH:MM:SS` / epoch integer, or falls back to wall clock if absent. **`date`**: optional tag/field `date="YYYY-MM-DD"`; if omitted, **`date` is set from the resolved `timestamp` (UTC calendar day)** — that value is what DuckLake uses for **`date=…`** partitioning. Ports and `protocol` should be integer fields (`5060i`, `17i`). Optional **`data_extra`** must be a JSON **object** string (e.g. `data_extra="{}"`); if omitted, `{}` is stored.

**Line layout (Influx LP):** `measurement,tag_key=tag_val,... field_key=field_val,... timestamp` — the **first unescaped space** separates **tags** (comma-separated) from **fields** (comma-separated). There is **no** comma between the last tag and the first field.

The **`curl`** examples below assume **`allow_hep_sip_call`** is **`true`** in config (measurement **`hep_proto_1_call`** for SIP call rows).

Example (partition `date` is inferred from the line timestamp — here 2023-11-14 UTC):

```bash
curl -i -X POST "http://127.0.0.1:8086/write?precision=ns" \
  --data-binary 'hep_proto_1_call,method=INVITE,session_id=abc caller="alice",callee="bob",src_ip="10.0.0.1",dst_ip="10.0.0.2",src_port=5060i,dst_port=5060i,protocol=17i,payload="INVITE sip:b SIP/2.0" 1700000000000000000'
```

Example with **explicit `date`** as a **tag** (partition `date=2023-11-14/…`; keep it consistent with `timestamp`):

```bash
curl -i -X POST "http://127.0.0.1:8086/write?precision=ns" \
  --data-binary 'hep_proto_1_call,method=INVITE,session_id=abc,date=2023-11-14 caller="alice",callee="bob",src_ip="10.0.0.1",dst_ip="10.0.0.2",src_port=5060i,dst_port=5060i,protocol=17i,payload="INVITE sip:b SIP/2.0" 1700000000000000000'
```

You can also send **`date` as a string field** (quoted): `...,date="2023-11-14"` in the field set.

### Query parameters

| Param         | v1 / v2     | Meaning                                                                                |
|---------------|-------------|----------------------------------------------------------------------------------------|
| `db`          | v1          | Routes points into DuckDB schema `<sanitised db>`.                                     |
| `bucket`      | v2          | Same as `db`. v2 clients use this; v1 clients use `db`.                                |
| `org`         | v2          | Accepted for compatibility, ignored.                                                   |
| `precision`   | both        | `ns` / `us` / `ms` / `s`. Falls back to `default_precision`.                           |
| `rp`          | v1          | Accepted for compatibility, ignored (DuckLake handles retention separately).           |

### Body format

Standard InfluxDB Line Protocol — one point per line, `#`-comments
allowed, blank lines ignored, `gzip` Content-Encoding supported:

```
<measurement>[,<tagk>=<tagv>...] <fieldk>=<fieldv>[,...] [<unix-timestamp>]
```

Field values are typed: integer (`42i`), unsigned (`42u`), float (`3.14`),
boolean (`true`/`false`/`t`/`f`/`T`/`F`), or quoted string
(`"hello\nworld"`). Spaces, commas, and equals signs in tag/field keys
must be backslash-escaped, exactly as in InfluxDB.

### Responses

| Code | When                                                                       |
|------|----------------------------------------------------------------------------|
| `204 No Content` | All points written successfully.                                  |
| `400 Bad Request` | Parse error, invalid precision, deprecated `hep_table` query parameter, mixed `hep_proto_*`/other measurements or two different `hep_proto_*` tables with `allow_hep_sip_call`, unknown `hep_proto_*` name, mapping error (e.g. invalid `data_extra`), or zero parseable points. |
| `405 Method Not Allowed` | Wrong HTTP verb on `/write*`.                              |
| `413 Request Entity Too Large` | Body exceeded `max_body_bytes`.                      |
| `500 Internal Server Error` | DuckLake DDL or INSERT failed (see `homer_lp_write_errors_total`). |

Error responses use a JSON envelope (`{"error": "..."}`) matching the
InfluxDB convention so existing clients show meaningful messages.

`/ping` returns `204` with the `X-Influxdb-Version` header set;
`/health` returns `200` with `{"status":"pass","name":"homer-core"}`.

## Storage layout

For a request like:

```
POST /api/v2/write?bucket=apps&precision=ns
cpu,host=node-1,region=eu  usage_idle=98.4,usage_user=1.1  1714521600000000000
mem,host=node-1            free=2147483648i               1714521600000000000
```

the receiver materialises:

```
homer_lake.apps.cpu  (time TIMESTAMP, host VARCHAR, region VARCHAR,
                         usage_idle DOUBLE, usage_user DOUBLE)
homer_lake.apps.mem  (time TIMESTAMP, host VARCHAR, free BIGINT)
```

Schema rules:

- **Schema name** = sanitised `?db=` / `?bucket=` / `default_db`
  (alphanumerics + `_`, leading digit gets a `_` prefix). Empty ⇒ `main`.
- **Table name** = `<table_prefix><sanitised_measurement>` (default
  **empty prefix** ⇒ table name equals the sanitised measurement; set
  `table_prefix` if you want a namespace such as `lp_`).
- **`time`** column is always created first as `TIMESTAMP` and is the
  natural sort/filter key. The point timestamp (or "now" if missing) is
  converted to UTC-`TIMESTAMP` using the negotiated precision.
- **Tag columns** are always `VARCHAR`.
- **Field columns** are typed from the *first* observed value:
  - `int64` ⇒ `BIGINT`
  - `uint64` ⇒ `BIGINT` (DuckDB lacks an unsigned type at this width)
  - `float64` ⇒ `DOUBLE`
  - `bool` ⇒ `BOOLEAN`
  - quoted string ⇒ `VARCHAR`
- **New columns** are added on the fly via `ALTER TABLE ADD COLUMN
  IF NOT EXISTS …`. Existing columns are never altered or widened —
  if a measurement starts emitting a different type for an existing
  field that field is **dropped from that batch** and counted in
  `homer_lp_write_errors_total{stage="type_mismatch"}` (the rest of
  the point is still inserted). This protects historical data from
  silent corruption.

DDL is serialised per-table with an internal mutex to keep concurrent
writes safe.

## Discovery API

The dynamic nature of the tables means the UI/Grafana/MCP cannot rely
on static `mapping_schema` rows. Instead, the coordinator exposes two
read-only v4 endpoints:

| Method | Path                                                | Returns                                                       |
|--------|-----------------------------------------------------|---------------------------------------------------------------|
| GET    | `/api/v4/line_protocol/tables`                      | Tables whose names match the configured prefix filter (default from `ingest.line_protocol.table_prefix`). |
| GET    | `/api/v4/line_protocol/tables/:schema/:table`       | One table's column metadata.                                  |

Query parameters (list endpoint):

| Param           | Default | Description                                                                     |
|-----------------|---------|---------------------------------------------------------------------------------|
| `prefix`        | from `ingest.line_protocol.table_prefix` (default empty) | Non-empty: `LIKE '<prefix>%'` (ASCII alphanumerics + `_` only). **Empty:** lists tables excluding built-in `hep_proto_%`, `otlp_%`, and `mem_hep_%` names (still potentially large). |
| `schema`        | empty   | Restrict to a single DuckDB schema (the per-`?db=` one). Validated as identifier. |
| `with_columns`  | `true`  | Set `false` to skip column enumeration for cheaper listings.                     |
| `page[limit]`   | v4 default | Pagination cap.                                                              |

Sample response:

```json
{
  "data": {
    "items": [
      {
        "catalog": "homer_lake",
        "schema":  "apps",
        "name":    "cpu",
        "fqn":     "homer_lake.apps.cpu",
        "columns": [
          { "name": "time",        "data_type": "TIMESTAMP", "nullable": false, "position": 1 },
          { "name": "host",        "data_type": "VARCHAR",   "nullable": true,  "position": 2 },
          { "name": "region",      "data_type": "VARCHAR",   "nullable": true,  "position": 3 },
          { "name": "usage_idle",  "data_type": "DOUBLE",    "nullable": true,  "position": 4 },
          { "name": "usage_user",  "data_type": "DOUBLE",    "nullable": true,  "position": 5 }
        ]
      }
    ],
    "prefix": ""
  },
  "total": 1,
  "meta": {
    "pagination": { "limit": 100, "total": 1, "has_more": false }
  }
}
```

Discovery uses `INFORMATION_SCHEMA.tables` and
`INFORMATION_SCHEMA.columns` over `flightService.Query`, so the
endpoint works against any node that participates in DuckLake (it
follows the same route as `/api/v4/statistics/*`). Identifiers and
prefix are validated against an allow-list before being interpolated
into SQL. When **`prefix`** is **empty** (the default `table_prefix`), the listing **excludes** `hep_proto_%`, `otlp_%`, and `mem_hep_%` table names instead of scanning every table.

The Statistics API also works out of the box for the same tables:

- `GET /api/v4/statistics/databases` → schemas (catalogs)
- `GET /api/v4/statistics/measurements?db=<schema>` → table names
- `GET /api/v4/statistics/metrics?db=<schema>` → column names
- `POST /api/v4/statistics/query` with a `rawquery` ⇒ read-only SQL for Grafana-style panels (`SELECT …`). Since 11.0.282+ each `rawquery` is validated (same rules as `POST /api/v4/query`); DML/DDL and multi-statement SQL are rejected.

## Auto-published mapping_schema rows (11.0.123+)

Since 11.0.123 the coordinator also keeps `mapping_schema` (the table
that drives **Settings → Protocol Mappings** and the Proto Search
widget's protocol picker) in sync with the live Line Protocol tables (respecting `table_prefix`). Operators
no longer have to edit `mapping_schema` by hand for every new
measurement — the LP receiver can be pointed at a fresh database and
the dashboards will discover the resulting tables automatically.

The sync runs in the coordinator process (`LPMappingSyncService`) and
ticks once on startup plus every 60 s. Each pass is a single
`INFORMATION_SCHEMA.tables` query plus one batched
`INFORMATION_SCHEMA.columns` query, so the cost is independent of the
number of measurements.

What lands in `mapping_schema` per discovered table:

| Column           | Value                                                                |
|------------------|----------------------------------------------------------------------|
| `hepid`          | **`300`** — the synthetic LP virtual hepid (`services.LPVirtualHepID`). The same value is used for every LP table; the table identity itself lives in `profile`. |
| `hep_alias`      | `LP_<TABLE>` (uppercased). E.g. `cpu` → `LP_CPU`. Schema is intentionally omitted so the picker reads like a measurement name. |
| `profile`        | `<schema>__<table>` — the encoding decoded by `services.SplitLPProfile`. The double-underscore separator means schemas/tables containing a single underscore (`x_y`) round-trip cleanly. |
| `guid`           | Stable UUIDv5-style hash of `lp:<schema>:<table>`. Idempotent across restarts and across nodes. |
| `fields_mapping` | JSON array generated from live `INFORMATION_SCHEMA.columns`. The `time` column is marked `index=primary` + `sid_type=true` (the widget treats it as the correlation key); `raw` / `_*` columns are marked `hide=true`; SQL types are mapped to UI form types (`TIMESTAMP→datetime`, `BOOLEAN→switch`, numeric → numeric input, everything else → free-text input). |

Per-row idempotency: the upsert is keyed on `guid`, so:

- New tables are **inserted** at the next tick after first ingest.
- Schema evolution (`ALTER TABLE ADD COLUMN`) triggers an **update**
  of `fields_mapping` only when the JSON differs (compared
  semantically — whitespace and key order are normalised).
- Tables that disappear are **left in place** by design — keeps the
  audit trail intact and means transient discovery failures never
  delete operator-customised rows.
- Other columns operators may have edited (`retention`,
  `partition_step`, `mapping_settings`, …) are never overwritten.

How the search path uses it (Proto Search widget → coordinator):

1. Widget POSTs to `/api/v4/transactions/search` with
   `proto_type=300` and `event_type=<schema>__<table>`.
2. `getTableName(...)` decodes the profile via `lpTableForProfile` and
   returns `<lakeName>.<schema>.<table>`.
3. `buildSearchSQLV4` routes via `isLPProtoType` to
   `buildLPSearchSQLV4`, which only emits clauses LP tables actually
   support: `time` range filters, optional `Filter.Payload` as
   `CAST(ROW(*) AS VARCHAR) LIKE '%…%'`, default `ORDER BY time DESC`,
   and `LIMIT`. SIP-specific fields (caller, callee, method, ports,
   node_id, …) are silently ignored, so existing search-bar widgets
   don't generate parse errors when pointed at LP tables.

If you don't see a measurement in the picker after writing, check the
coordinator log for `LPMappingSync: synced` lines (logged only when
something changed) and confirm the table actually exists via
`/api/v4/line_protocol/tables`. The sync uses the same
`ingest.line_protocol.table_prefix` as the receiver (including an
explicit empty string) — change the prefix and restart both ends if
you are using a custom one.

## Metrics

Exposed on the standard `/metrics` Prometheus endpoint:

| Metric                                | Labels             | Meaning                                                  |
|---------------------------------------|--------------------|----------------------------------------------------------|
| `homer_lp_requests_received_total`    | `endpoint`, `outcome` | HTTP request counts. `outcome` ∈ `ok` / `parse_error` / `body_too_large` / `bad_request` / `method_not_allowed`. |
| `homer_lp_points_ingested_total`      | `database`         | Successfully written points per schema (`?db=` value).    |
| `homer_lp_write_errors_total`         | `stage`            | Failures during ingest. `stage` ∈ `parse` / `ddl` / `insert` / `type_mismatch`. |

`endpoint` ∈ `v1_write` / `v2_write` / `v3_write_lp` / `ping` / `health`.

## Sample queries

CPU usage by host over the last hour:

```sql
SELECT host,
       AVG(usage_idle) AS idle,
       AVG(usage_user) AS user
FROM homer_lake.apps.cpu
WHERE time >= NOW() - INTERVAL 1 HOUR
GROUP BY host
ORDER BY user DESC;
```

Discover the schema before writing a Grafana panel:

```bash
curl -s -H "Authorization: Bearer $TOKEN" \
  'https://homer/api/v4/line_protocol/tables?schema=apps' | jq '.data.items[].columns'
```

Pipe Telegraf at homer-core:

```toml
# /etc/telegraf/telegraf.conf
[[outputs.influxdb_v2]]
  urls         = ["http://homer-node:8086"]
  bucket       = "apps"
  organization = "homer"
  token        = "ignored-by-receiver"
  content_encoding = "gzip"
```

Pipe a one-shot point with `curl`:

```bash
curl -i -XPOST 'http://homer-node:8086/api/v2/write?bucket=apps&precision=s' \
  --data-binary 'cpu,host=node-1 usage_idle=99.1 1714521600'
```

## Operational notes

- **DDL cost.** The first point for a new measurement triggers a
  `CREATE TABLE`; the first point that introduces a new field triggers
  an `ALTER TABLE`. Both are cheap on DuckLake but happen synchronously
  on the request path — pre-warm tables in low-traffic windows when
  rolling out a new agent.
- **Tag cardinality.** Tags become `VARCHAR` columns, **not** rows in a
  separate index. High-cardinality tags (`request_id`, full URLs, …)
  are better placed in fields, where they don't widen the schema.
- **Type stability.** Field type is locked at first observation. If you
  change a field from `int` to `float` mid-stream, switch to a new
  field name (`value_v2`) — homer-core will not silently truncate
  history.
- **Authentication.** The receiver itself does not validate Influx
  tokens; treat `:8086` like any other ingest port (firewall it,
  optionally require mTLS). Per-token auth can be layered in front via
  an ingress proxy.
- **Retention.** Each Line Protocol table is a normal DuckLake table — apply
  retention with the standard DuckLake TTL (see
  [Data retention](RETENTION.md)).
- **Testing.** End-to-end coverage lives in
  `src/lineprotoreceiver/{parser,ingest,http}_test.go`. The ingest
  tests use an in-memory DuckDB so they run in normal `go test ./...`
  without external services.

## Related docs

- [OTLP.md](./OTLP.md) — sister OpenTelemetry Protocol receiver.
- [STORAGE_LAYOUT.md](./STORAGE_LAYOUT.md) — DuckLake on-disk layout.
- [RETENTION.md](./RETENTION.md) — data TTL and compaction retention.
- [STORAGE_POLICIES.md](./STORAGE_POLICIES.md) — hot/cold tiering.
- [NATIVE_TIER_MOVE.md](./NATIVE_TIER_MOVE.md) — opt-in parquet file copy between volumes.
- [FLIGHTSQL.md](./FLIGHTSQL.md) — the FlightSQL transport used by the discovery and statistics endpoints.
