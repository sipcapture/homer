# OpenTelemetry Protocol (OTLP) Receiver

homer-core ships a first-class **OTLP** ingest path that accepts
traces, metrics and logs over the three canonical OTLP transports:

- **OTLP/gRPC** on `:4317`
- **OTLP/HTTP + protobuf** on `:4318`
- **OTLP/HTTP + JSON** on `:4318`

All three signal types land in **dedicated DuckLake tables** —
`otlp_traces`, `otlp_metrics`, `otlp_logs` — preserving full OTel
fidelity (resource attributes, scope info, severity, trace/span ids,
events, links). The HEP pipeline is intentionally not on this path:
OTLP signals are not transcoded into HEPv3.

## Architecture

```
                      ┌────────────────────────────────────────┐
   OTLP/gRPC :4317    │            otlpreceiver.Module         │
   ─────────────────▶ │  ┌──────────┐    ┌─────────────────┐  │
                      │  │  gRPC    │───▶│                 │  │
                      │  │ server   │    │   sink.Multi    │  │
                      │  └──────────┘    │ (Push{Traces,   │  │
   OTLP/HTTP :4318    │  ┌──────────┐    │  Metrics,Logs}) │  │
   ─────────────────▶ │  │ HTTP     │───▶│                 │  │
   (proto + JSON)     │  │ server   │    └────────┬────────┘  │
                      │  └──────────┘             │           │
                      └───────────────────────────┼───────────┘
                                                  ▼
                                  ┌──────────────────────────────┐
                                  │   ducklake.OTLPStorage       │
                                  │  ──────────────────────────  │
                                  │  otlp_traces / metrics / logs│
                                  │  (partitioned by `date`,     │
                                  │   sorted by `timestamp`)     │
                                  └──────────────────────────────┘
```

The receiver is wired into the lifecycle as a regular `ModuleManager`
module (see `src/main.go`). It is **only constructed when the writer
module is enabled** — the sink stores rows directly via the writer's
DuckLake handle, so a coordinator-only deploy will not start the
listener.

## Configuration

```json
{
  "ingest": {
    "otlp": {
      "enable": false,
      "max_recv_msg_bytes": 4194304,
      "grpc": {
        "enable": true,
        "listen": ":4317",
        "cert": "",
        "key": "",
        "cacert": ""
      },
      "http": {
        "enable": true,
        "listen": ":4318",
        "cert": "",
        "key": "",
        "cacert": "",
        "read_timeout_sec": 30,
        "write_timeout_sec": 30
      },
      "sinks": {
        "store_traces":  true,
        "store_metrics": true,
        "store_logs":    true
      },
      "async_enable": true,
      "async_queue_depth": 512,
      "async_enqueue_timeout_ms": 0
    }
  }
}
```

| Field                          | Default     | Description                                                                 |
|--------------------------------|-------------|-----------------------------------------------------------------------------|
| `enable`                       | `false`     | Master switch. When `false` the module is not even constructed.             |
| `max_recv_msg_bytes`           | `4194304`   | Inbound size cap; applies to **both** gRPC frames and HTTP request bodies. |
| `grpc.enable` / `http.enable`  | `true`      | Per-transport opt-in. At least one must be on.                              |
| `grpc.listen`                  | `:4317`     | gRPC bind address.                                                          |
| `http.listen`                  | `:4318`     | HTTP bind address. Routes are fixed (see below).                            |
| `*.cert` / `*.key` / `*.cacert`| empty       | Optional TLS / mTLS material. Empty `cert`+`key` disables TLS on that port. |
| `http.{read,write}_timeout_sec`| `30`        | HTTP server timeouts.                                                       |
| `sinks.store_traces`           | `true`      | If `false`, spans are accepted (`Status=OK`) but discarded after parsing.   |
| `sinks.store_metrics`          | `true`      | Same semantics for metric points.                                           |
| `sinks.store_logs`             | `true`      | Same semantics for log records.                                             |
| `async_enable`                 | `true`      | When `true` (default), exports are queued in-process and handlers return after enqueue (lower latency; see **Async mode** below). Set `false` for synchronous writes to DuckLake on each request. |
| `async_queue_depth`            | `512`       | Max pending export batches in the async queue (`>= 1`; invalid values clamp to 512). |
| `async_enqueue_timeout_ms`     | `0`         | `0` = non-blocking enqueue (fail immediately if the queue is full). `> 0` = wait up to this long for a free slot. |

### Async mode (`async_enable`)

When enabled, the DuckLake sink is wrapped in a **bounded channel + single
worker**: gRPC/HTTP handlers clone the protobuf request, push one job, and
return success without waiting for `INSERT` into DuckLake.

**Trade-offs (by design, not hepic-lake-style staging):**

- **Durability**: After `200 OK` / gRPC OK the batch may still sit in RAM
  until the worker writes it. A crash can **lose** accepted batches that
  were not yet written.
- **Back-pressure**: If the worker is slower than producers, the queue
  fills and further `Push*` calls fail → clients see **5xx** / gRPC
  errors and should retry (same as synchronous sink overload).
- **Throughput**: One worker serialises DuckLake writes; sustained overload
  will hit the queue cap rather than unbounded goroutine growth.

Shutdown: OTLP listeners stop first, then the queue is **drained** while
DuckLake is still open. `ModuleManager` stops modules in **reverse
registration order** so OTLP (and similar ingest) stops before the writer
closes the database.

### TLS / mTLS

- HTTP: when `cert`+`key` are set the server runs HTTPS. If `cacert` is
  also set, mTLS is enforced (clients must present a cert chained to it).
- gRPC: same convention. Empty cert/key keeps the listener insecure
  (use only behind a trusted ingress).

## Endpoints

### OTLP/gRPC (`:4317`)

Standard OTel collector services:

- `opentelemetry.proto.collector.trace.v1.TraceService/Export`
- `opentelemetry.proto.collector.metrics.v1.MetricsService/Export`
- `opentelemetry.proto.collector.logs.v1.LogsService/Export`

### OTLP/HTTP (`:4318`)

| Path           | Methods | Content-Type                            |
|----------------|---------|-----------------------------------------|
| `/v1/traces`   | POST    | `application/x-protobuf`, `application/json` |
| `/v1/metrics`  | POST    | `application/x-protobuf`, `application/json` |
| `/v1/logs`     | POST    | `application/x-protobuf`, `application/json` |

The HTTP receiver auto-detects the body format from `Content-Type`.
Successful exports return **`200 OK`** with the standard empty
`ExportXxxServiceResponse` body. Failures use the OTel partial-success
convention (`partial_success.rejected_*`) when the server can isolate a
bad record, otherwise `4xx`/`5xx` with a JSON error envelope.

## Storage layout

Each signal type has a fixed-schema DuckLake table. The full original
payload is also stored in a `raw` JSON column so re-derivation of
attributes / events / links is always possible without re-ingest.

### `otlp_traces`

| Column          | Type        | Notes                                                |
|-----------------|-------------|------------------------------------------------------|
| `date`          | `DATE`      | Partition key (`ALTER TABLE ... PARTITIONED BY (date)`). |
| `timestamp`     | `TIMESTAMP` | Span start time. Sort key.                           |
| `end_timestamp` | `TIMESTAMP` | Span end time.                                       |
| `duration_ns`   | `BIGINT`    | `end_timestamp - timestamp` in nanoseconds.          |
| `trace_id`      | `VARCHAR`   | Hex-encoded 16-byte trace id.                        |
| `span_id`       | `VARCHAR`   | Hex-encoded 8-byte span id.                          |
| `parent_span_id`| `VARCHAR`   | Empty for root spans.                                |
| `name`          | `VARCHAR`   | Span name.                                           |
| `kind`          | `INTEGER`   | OTel `SpanKind` enum.                                |
| `status_code`   | `INTEGER`   | OTel `StatusCode` enum.                              |
| `status_message`| `VARCHAR`   | Optional status description.                         |
| `service_name`  | `VARCHAR`   | Resolved from resource attribute `service.name`.     |
| `scope_name`    | `VARCHAR`   | Instrumentation scope name.                          |
| `scope_version` | `VARCHAR`   | Instrumentation scope version.                       |
| `resource_attrs`| `JSON`      | Full resource attribute map.                         |
| `span_attrs`    | `JSON`      | Span attribute map.                                  |
| `events_count`  | `INTEGER`   | `len(span.events)` — events themselves live in `raw`.|
| `links_count`   | `INTEGER`   | `len(span.links)` — links themselves live in `raw`.  |
| `raw`           | `JSON`      | Untouched OTLP span document.                        |

### `otlp_metrics`

| Column          | Type        | Notes                                                  |
|-----------------|-------------|--------------------------------------------------------|
| `date`          | `DATE`      | Partition key.                                         |
| `timestamp`     | `TIMESTAMP` | Sample timestamp. Sort key.                            |
| `name`          | `VARCHAR`   | Metric name (instrument name).                         |
| `description`   | `VARCHAR`   | Instrument description.                                |
| `unit`          | `VARCHAR`   | Instrument unit.                                       |
| `type`          | `VARCHAR`   | `Gauge` / `Sum` / `Histogram` / `Summary` / `ExponentialHistogram`. |
| `value_double`  | `DOUBLE`    | Set when point is float-valued.                        |
| `value_int`     | `BIGINT`    | Set when point is integer-valued.                      |
| `service_name`  | `VARCHAR`   | From resource attribute `service.name`.                |
| `scope_name`    | `VARCHAR`   | Instrumentation scope name.                            |
| `scope_version` | `VARCHAR`   | Instrumentation scope version.                         |
| `attributes`    | `JSON`      | Per-point attributes.                                  |
| `resource_attrs`| `JSON`      | Resource attribute map.                                |
| `raw`           | `JSON`      | Untouched OTLP metric document (preserves histogram buckets, etc.). |

### `otlp_logs`

| Column              | Type        | Notes                                              |
|---------------------|-------------|----------------------------------------------------|
| `date`              | `DATE`      | Partition key.                                     |
| `timestamp`         | `TIMESTAMP` | Event time (or observed time when missing). Sort key. |
| `observed_timestamp`| `TIMESTAMP` | Wall-clock observation time.                       |
| `severity_number`   | `INTEGER`   | OTel severity number (1..24).                      |
| `severity_text`     | `VARCHAR`   | Free-form severity (TRACE/DEBUG/INFO/WARN/ERROR/FATAL). |
| `body`              | `VARCHAR`   | Stringified body (for fast `LIKE`).                |
| `body_json`         | `JSON`      | Body when it is structured (map / array).          |
| `trace_id`          | `VARCHAR`   | Linked trace id (hex), if any.                     |
| `span_id`           | `VARCHAR`   | Linked span id (hex), if any.                      |
| `flags`             | `INTEGER`   | OTel log record flags.                             |
| `service_name`      | `VARCHAR`   | From resource attribute `service.name`.            |
| `scope_name`        | `VARCHAR`   | Instrumentation scope name.                        |
| `scope_version`     | `VARCHAR`   | Instrumentation scope version.                     |
| `attributes`        | `JSON`      | Log record attributes.                             |
| `resource_attrs`    | `JSON`      | Resource attribute map.                            |
| `raw`               | `JSON`      | Untouched OTLP log record.                         |

All three tables are best-effort `PARTITIONED BY (date)` and
`SORTED BY (timestamp ASC)` at create time — older DuckLake builds
without these clauses simply keep a single partition.

## CLI Search

OTLP data can be queried directly from the terminal via `homer search`.
OTLP signals use **virtual proto_type** values (`otlp_traces` / `otlp_metrics` / `otlp_logs` in the CLI, or `200` / `201` / `202`);
`--event-type` must be `default` (or omitted) for all three.

```bash
# Traces — last hour
homer search --host coordinator:8081 --proto otlp_traces --from 1h

# Traces for a specific trace_id  (--call-id maps to trace_id)
homer search --host coordinator:8081 --proto otlp_traces --call-id "a1b2c3d4e5f60718a9b0c1d2e3f40516"

# Traces by service name  (--user-agent maps to service_name)
homer search --host coordinator:8081 --proto otlp_traces --from 2h --user-agent "payment-service"

# Error spans via raw SQL
homer search --host coordinator:8081 --sql "SELECT timestamp,trace_id,name,status_message FROM default.otlp_traces WHERE status_code=2 ORDER BY timestamp DESC LIMIT 50"

# Metrics — last 30 minutes
homer search --host coordinator:8081 --proto otlp_metrics --from 30m

# Metrics by name  (--call-id maps to name LIKE)
homer search --host coordinator:8081 --proto otlp_metrics --from 1h --call-id "http.server.duration"

# Log records — last hour
homer search --host coordinator:8081 --proto otlp_logs --from 1h

# Logs containing "error"  (--payload maps to body/raw LIKE)
homer search --host coordinator:8081 --proto otlp_logs --from 30m --payload "error"

# Logs linked to a trace  (--call-id maps to trace_id)
homer search --host coordinator:8081 --proto otlp_logs --call-id "a1b2c3d4e5f60718a9b0c1d2e3f40516"

# Interactive TUI with OTLP Traces pre-selected
homer search --host coordinator:8081 --proto otlp_traces --interactive
```

For the full CLI reference, filter-to-column mapping, and raw SQL examples see [`SEARCH.md`](./SEARCH.md).

## Search / UI integration

Each OTLP table is exposed in the Proto Search widget through a
**virtual mapping_schema entry** (no real `hep_proto_*` table is
involved):

| Signal     | hepid | profile   | hep_alias     |
|------------|-------|-----------|---------------|
| traces     | 200   | `default` | `OTLP_TRACES` |
| metrics    | 201   | `default` | `OTLP_METRICS`|
| logs       | 202   | `default` | `OTLP_LOGS`   |

The seed lives in
`src/coordinator/services/mapping_seed.go` and the field definitions
are embedded from `seeds/fields_otlp_{traces,metrics,logs}.json`.

`getTableName()` (in `coordinator/handlers/search.go`) detects the
virtual hepids via `isOTLPProtoType()` and rewrites the SQL target to
`<lakeName>.otlp_traces` / `otlp_metrics` / `otlp_logs` instead of the
default `hep_proto_<id>_<profile>`.

Generic UI filters are remapped per signal in `buildOTLPSearchSQLV4`:

| UI filter (`SearchRequestV4.Filter`) | traces / logs                             | metrics                              |
|---------------------------------------|--------------------------------------------|---------------------------------------|
| `call_id`, `session_id`, `cid`        | `trace_id = …`                             | `name LIKE …`                         |
| `payload`                             | `body LIKE …` (logs) / `name LIKE …` (traces) plus `CAST(raw AS VARCHAR) LIKE …` | `name LIKE …` plus `CAST(raw AS VARCHAR) LIKE …` |
| `user_agent`                          | `service_name LIKE …`                      | `service_name LIKE …`                 |
| SIP-only fields (method, src_ip, …)   | ignored (no SQL is emitted for them)       | ignored                               |

The "Add Widget" dialog also exposes three pre-configured presets —
**OTLP Trace Search**, **OTLP Metric Search**, **OTLP Log Search** —
that bootstrap the widget with the right `protocol_id`, profile and a
sensible default field selection (`trace_id` / `name+type` /
`severity_text+body`).

## Metrics

Exposed on the standard `/metrics` Prometheus endpoint:

| Metric                               | Labels                       | Meaning                                            |
|--------------------------------------|------------------------------|----------------------------------------------------|
| `homer_otlp_requests_received_total` | `signal`, `transport`        | Successful Export RPC / HTTP request counts.       |
| `homer_otlp_requests_failed_total`   | `signal`, `transport`, `reason` | Failed Export attempts (decode/validate/transport).|
| `homer_otlp_records_received_total`  | `signal`                     | Records (spans / points / log records) ingested.   |
| `homer_otlp_sink_errors_total`       | `signal`, `sink`             | Errors during downstream persistence.              |
| `homer_otlp_async_enqueue_total`     | `signal`, `outcome`          | Async queue enqueues (`outcome` = `ok` \| `queue_full`). Only when `async_enable` is on. |
| `homer_otlp_async_worker_errors_total` | `signal`                   | Inner sink errors observed by the async worker.   |

`signal` ∈ `traces` / `metrics` / `logs`,
`transport` ∈ `grpc` / `http_proto` / `http_json`.

## Sample queries

Top services by error span count over the last hour:

```sql
SELECT service_name, COUNT(*) AS errors
FROM homer_lake.otlp_traces
WHERE date    = CURRENT_DATE
  AND timestamp >= NOW() - INTERVAL 1 HOUR
  AND status_code = 2          -- ERROR
GROUP BY service_name
ORDER BY errors DESC
LIMIT 20;
```

Find log records linked to a specific trace:

```sql
SELECT timestamp, severity_text, service_name, body
FROM homer_lake.otlp_logs
WHERE date = CURRENT_DATE
  AND trace_id = 'a1b2c3d4e5f60718a9b0c1d2e3f40516'
ORDER BY timestamp;
```

Histogram of metric value distribution by service:

```sql
SELECT service_name,
       AVG(value_double) AS avg_v,
       MAX(value_double) AS max_v
FROM homer_lake.otlp_metrics
WHERE date = CURRENT_DATE
  AND name = 'http.server.duration'
GROUP BY service_name
ORDER BY avg_v DESC;
```

## Operational notes

- **No HEP transcoding**. If you need OTLP spans in your existing
  Homer SIP dashboards as HEP type 100 LOGs, do that conversion in the
  client SDK or an OTel collector — homer-core deliberately keeps the
  two pipelines separate.
- **`raw` is the source of truth**. Schema columns are convenient
  index/filter shortcuts; rare attributes that aren't promoted are
  always recoverable via `json_extract(raw, '$. ...')`.
- **Schema migration**. Adding a new top-level column to one of the
  three tables is a manual `ALTER TABLE`. The receiver does not
  auto-extend the OTLP schemas (unlike Line Protocol, which does).
- **Capacity planning**. Each signal table is partitioned by `date`,
  so retention is set with the standard DuckLake compaction/retention
  policies on `homer_lake.otlp_*` (see [`STORAGE_POLICIES.md`](./STORAGE_POLICIES.md)).
- **Testing**. End-to-end tests live in `src/otlpreceiver/{http,grpc}_test.go`
  and exercise both transports against an in-process server.

## Related docs

- [STORAGE_LAYOUT.md](./STORAGE_LAYOUT.md) — DuckLake on-disk layout.
- [LINE_PROTOCOL.md](./LINE_PROTOCOL.md) — sister InfluxDB Line Protocol receiver.
- [SEARCH.md](./SEARCH.md) — the v4 search engine that consumes `otlp_*` tables.
