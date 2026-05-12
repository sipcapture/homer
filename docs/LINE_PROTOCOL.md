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
                                │  table  = lp_<measurement>         │
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
      "table_prefix": "lp_",
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

| Field                | Default     | Description                                                                                       |
|----------------------|-------------|---------------------------------------------------------------------------------------------------|
| `enable`             | `false`     | Master switch. When `false` the module is not constructed and `:8086` stays free.                |
| `listen`             | `:8086`     | HTTP bind address. `:8086` matches InfluxDB v1's default.                                         |
| `max_body_bytes`     | `8388608`   | Inbound request body cap (8 MiB — same as InfluxDB v2's `max-body-size`). Over-limit ⇒ `413`.    |
| `default_precision`  | `ns`        | Used when the client doesn't pass `?precision=`. Valid: `ns`, `us`, `ms`, `s`.                    |
| `default_db`         | empty       | Used when the client doesn't pass `?db=` / `?bucket=`. Empty ⇒ DuckDB's default schema (`main`). |
| `table_prefix`       | `lp_`       | Prepended to the (sanitised) measurement name to form the table name. Keeps LP data namespaced.   |
| `allow_hep_sip_call` | `false`     | When `true`, allows `?hep_table=call` to insert into `{lake}.main.hep_proto_1_call` (see below). When `false`, any `?hep_table=` request returns **400**. |
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

### Optional: `hep_proto_1_call` (SIP call table)

When **`ingest.line_protocol.allow_hep_sip_call`** is `true`, add **`?hep_table=call`** to a write request. Each parsed LP line becomes one row in **`{lakeName}.main.hep_proto_1_call`** (same layout as SIP call rows from HEP). In this mode **`?db=` / `?bucket=`** do not change the destination table.

If **`allow_hep_sip_call`** is `false` (default) and the client sends **`?hep_table=`** with any value, the server returns **400** before ingesting.

**Security:** an exposed LP port with `allow_hep_sip_call=true` allows unauthenticated append to the call table unless you use network controls or mTLS (`cacert`).

**Column mapping:** tags and fields are merged (Influx-sanitised identifiers). Use **quoted** strings for text fields (`caller="alice"`). Ports and `protocol` should be integer fields (`5060i`, `17i`). Optional **`data_extra`** must be a JSON **object** string (e.g. `data_extra="{}"`); if omitted, `{}` is stored. The LP **measurement** name is ignored when `hep_table=call`.

Example:

```bash
curl -i -X POST "http://127.0.0.1:8086/write?hep_table=call&precision=ns" \
  --data-binary 'sip,method=INVITE,session_id=abc caller="alice",callee="bob",src_ip="10.0.0.1",dst_ip="10.0.0.2",src_port=5060i,dst_port=5060i,protocol=17i,payload="INVITE sip:b SIP/2.0" 1700000000000000000'
```

### Query parameters

| Param         | v1 / v2     | Meaning                                                                                |
|---------------|-------------|----------------------------------------------------------------------------------------|
| `db`          | v1          | Routes points into DuckDB schema `<sanitised db>`.                                     |
| `bucket`      | v2          | Same as `db`. v2 clients use this; v1 clients use `db`.                                |
| `org`         | v2          | Accepted for compatibility, ignored.                                                   |
| `precision`   | both        | `ns` / `us` / `ms` / `s`. Falls back to `default_precision`.                           |
| `hep_table`   | both        | `call` — write into `hep_proto_1_call` when `allow_hep_sip_call` is true; otherwise **400**. |
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
| `400 Bad Request` | Parse error, invalid precision, `hep_table` rejected (feature off or unknown value), mapping error (e.g. invalid `data_extra`), or zero parseable points. |
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
homer_lake.apps.lp_cpu  (time TIMESTAMP, host VARCHAR, region VARCHAR,
                         usage_idle DOUBLE, usage_user DOUBLE)
homer_lake.apps.lp_mem  (time TIMESTAMP, host VARCHAR, free BIGINT)
```

Schema rules:

- **Schema name** = sanitised `?db=` / `?bucket=` / `default_db`
  (alphanumerics + `_`, leading digit gets a `_` prefix). Empty ⇒ `main`.
- **Table name** = `<table_prefix><sanitised_measurement>` (default
  prefix `lp_`).
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
| GET    | `/api/v4/line_protocol/tables`                      | Every `lp_*` table across all schemas.                        |
| GET    | `/api/v4/line_protocol/tables/:schema/:table`       | One table's column metadata.                                  |

Query parameters (list endpoint):

| Param           | Default | Description                                                                     |
|-----------------|---------|---------------------------------------------------------------------------------|
| `prefix`        | `lp_`   | Override the table-name prefix used as `LIKE '<prefix>%'`. ASCII alphanumerics + `_` only. |
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
        "name":    "lp_cpu",
        "fqn":     "homer_lake.apps.lp_cpu",
        "columns": [
          { "name": "time",        "data_type": "TIMESTAMP", "nullable": false, "position": 1 },
          { "name": "host",        "data_type": "VARCHAR",   "nullable": true,  "position": 2 },
          { "name": "region",      "data_type": "VARCHAR",   "nullable": true,  "position": 3 },
          { "name": "usage_idle",  "data_type": "DOUBLE",    "nullable": true,  "position": 4 },
          { "name": "usage_user",  "data_type": "DOUBLE",    "nullable": true,  "position": 5 }
        ]
      }
    ],
    "prefix": "lp_"
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
into SQL.

The Statistics API also works out of the box for the same tables:

- `GET /api/v4/statistics/databases` → schemas (catalogs)
- `GET /api/v4/statistics/measurements?db=<schema>` → table names
- `GET /api/v4/statistics/metrics?db=<schema>` → column names
- `POST /api/v4/statistics/query` with a `rawquery` ⇒ free-form SQL
  (`SELECT ... FROM lp_cpu WHERE host = 'node-1'`) for Grafana-style
  panels.

## Auto-published mapping_schema rows (11.0.123+)

Since 11.0.123 the coordinator also keeps `mapping_schema` (the table
that drives **Settings → Protocol Mappings** and the Proto Search
widget's protocol picker) in sync with the live `lp_*` set. Operators
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
| `hep_alias`      | `LP_<TABLE>` (uppercased). E.g. `lp_cpu` → `LP_CPU`. Schema is intentionally omitted so the picker reads like a measurement name. |
| `profile`        | `<schema>__<table>` — the encoding decoded by `services.SplitLPProfile`. The double-underscore separator means schemas/tables containing a single underscore (`lp_cpu`) round-trip cleanly. |
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
`/api/v4/line_protocol/tables`. The sync respects the same `lp_`
prefix the receiver writes to — change the receiver prefix and
restart both ends if you are using a custom one.

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
FROM homer_lake.apps.lp_cpu
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
- **Retention.** Each `lp_*` table is a normal DuckLake table — apply
  retention with the standard policy primitives (see
  [`STORAGE_POLICIES.md`](./STORAGE_POLICIES.md)).
- **Testing.** End-to-end coverage lives in
  `src/lineprotoreceiver/{parser,ingest,http}_test.go`. The ingest
  tests use an in-memory DuckDB so they run in normal `go test ./...`
  without external services.

## Related docs

- [OTLP.md](./OTLP.md) — sister OpenTelemetry Protocol receiver.
- [STORAGE_LAYOUT.md](./STORAGE_LAYOUT.md) — DuckLake on-disk layout.
- [STORAGE_POLICIES.md](./STORAGE_POLICIES.md) — compaction / retention.
- [FLIGHTSQL.md](./FLIGHTSQL.md) — the FlightSQL transport used by the discovery and statistics endpoints.
