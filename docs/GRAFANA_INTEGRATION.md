# Grafana Integration (FlightSQL)

homer-core can expose **Apache Arrow FlightSQL** on the **node** and optionally on the **coordinator** (proxy). That endpoint is compatible with Grafana’s built-in **InfluxDB** datasource when **Query language** is set to **FlightSQL** (Grafana 11+ in most builds; exact UI labels vary slightly by version).

For protocol background (Airport vs FlightSQL, ports), see [FLIGHTSQL.md](FLIGHTSQL.md) and [NODE.md](NODE.md).

---

## Architecture

| Component | Port (typical) | Role |
|-----------|----------------|------|
| Node **Airport** (`node.flight_server`) | 50051 | DuckDB Airport (not used by Grafana FlightSQL) |
| Node **HTTP** `/query` | `flight_server.port + 1` (e.g. 50052) | Coordinator → node queries (JSON) |
| Node **FlightSQL** (`node.flightsql_server`) | 50055 | Direct Grafana / ADBC FlightSQL |
| Coordinator **REST** | 8080 | Homer UI, APIs |
| Coordinator **FlightSQL proxy** (`coordinator.flightsql_server`) | 32010 | Single Grafana entrypoint; fans out to each node’s `flightsql_port` |

**Recommended for Grafana:** point the datasource at the **coordinator FlightSQL proxy** (`:32010`) and set **`flightsql_port`** on every `coordinator.nodes[]` entry to match each node’s `node.flightsql_server.port`. The proxy issues the same SQL to each backend node and merges Arrow record batches (no JSON hop).

---

## 1. Enable FlightSQL on the node

Minimal additions under `node`:

```json
{
  "node": {
    "enable": true,
    "flight_server": {
      "host": "0.0.0.0",
      "port": 50051,
      "auth_token": "change-me-airport"
    },
    "flightsql_server": {
      "enable": true,
      "host": "0.0.0.0",
      "port": 50055,
      "auth_token": "change-me-flightsql",
      "catalog_refresh_interval_sec": 30
    },
    "ducklake": {
      "lake_name": "homer_lake",
      "catalog_type": "sqlite",
      "catalog_path": "/data/homer/homer_catalog.sqlite",
      "data_path": "/data/homer/parquet"
    }
  }
}
```

- **`auth_token`**: if non-empty, clients must send `Authorization: Bearer <token>`.
- **`catalog_refresh_interval_sec`**: periodic catalog refresh on the node when it does **not** share the writer’s DuckDB (`sharedDB`). Refresh reconnects DuckDB (open → ATTACH → swap → close old) so DuckLake ObjectCache does not grow from in-place `DETACH`/`ATTACH`. With an all-in-one writer+node process, hot rows usually come from the shared DB path instead.

Use the **same** lake name as `storage.ducklake.lake_name` / `node.ducklake.lake_name` (default `homer_lake`). In all-in-one deployments, **`node.ducklake`** should mirror **`storage.ducklake`** (same `lake_name`, `catalog_path`, data `path`, and volume `name` entries where applicable). If you use **tiered** storage, keep **`node.ducklake.volumes`** aligned with **`storage.ducklake.storage_policy.volumes`** (see [NODE.md](NODE.md) and [STORAGE_POLICIES.md](STORAGE_POLICIES.md)).

The minimal JSON above omits `volumes`; the supported layout is the **`volumes`** array under `node.ducklake` as in [NODE.md](NODE.md) — use that as the source of truth when copying config.

---

## 2. Optional: coordinator FlightSQL proxy

Enable the proxy and tell it which gRPC port to use on each node:

```json
{
  "coordinator": {
    "enable": true,
    "flightsql_server": {
      "enable": true,
      "host": "0.0.0.0",
      "port": 32010,
      "auth_token": "change-me-proxy",
      "catalog_refresh_interval_sec": 30
    },
    "nodes": [
      {
        "name": "node-a",
        "host": "10.0.0.11",
        "port": 50051,
        "flightsql_port": 50055,
        "token": "change-me-flightsql"
      }
    ]
  }
}
```

- **`port`**: still the node’s **Airport** gRPC port (used by the coordinator’s HTTP `/query` path and health checks).
- **`flightsql_port`**: node’s **FlightSQL** gRPC port. **`0` means the node is skipped** by the FlightSQL proxy.
- **`token`**: forwarded to the node as `Authorization: Bearer …` when dialing FlightSQL (should match the node’s `node.flightsql_server.auth_token` if set).

If `coordinator.flightsql_server.enable` is true but **no** node has `flightsql_port > 0`, the proxy will return **unavailable** for FlightSQL operations.

**TLS:** today the proxy dials nodes with **insecure** gRPC credentials (same limitation as the reference stack). Terminate TLS on a reverse proxy or extend the dial path when you need mTLS.

---

## 3. Configure the Grafana datasource

1. **Connections → Data sources → Add new data source**
2. Choose **InfluxDB**
3. Set **Query language** to **FlightSQL** (or the option your build labels as FlightSQL / SQL over Flight)
4. **URL / Host**: `grpc://<host>:32010` when using the coordinator proxy, or `grpc://<node-host>:50055` for a single node
5. **TLS**: off unless you terminate TLS elsewhere
6. **Token / API token** (or **Metadata**): set to the same value as `auth_token` on the server you connect to (coordinator proxy or node). Grafana should send it as a Bearer token compatible with homer-core’s gRPC interceptors.

Save and test.

---

## 4. SQL and schema discovery

The node applies **`sqlrewrite`** before tiered-volume and in-memory SQL rewrites used by HTTP `/query`, so many Grafana-generated probes work unchanged.

| Pattern (Grafana / Influx-style) | homer-core behavior |
|--------------------------------|---------------------|
| `information_schema.tables` with `TABLE_SCHEMA` and `'IOX'` | Rewritten to list tables in DuckLake catalog **`lake_name`** / schema `main` |
| `information_schema.columns` with `'IOX'` and a `table_name` | Rewritten to **`DESCRIBE lake.main.<table>`** (optional synthetic **`time`** column if `TableSettings` supplies a per-table time column) |
| Bare quoted `"time"` in queries | Mapped to **`timestamp`** by default, or to a configured column when hints are used |
| Bare quoted table names | Qualified to **`homer_lake.main."table"`** when the rewriter has a live DB handle and `lake_name` is set |

If something still fails in the query editor, run the same SQL against the node’s **HTTP** `/query` or `duckdb` CLI against the catalog to see the raw DuckDB error, then adjust the panel query or extend `sqlrewrite` / table hints as needed.

---

## 5. Example panel query

After the datasource connects, use standard SQL against DuckLake-qualified names, for example:

```sql
SELECT timestamp, callid, method
FROM homer_lake.main.hep_proto_1_call
ORDER BY timestamp DESC
LIMIT 500
```

Adjust **`homer_lake`**, table name, and columns to match your deployment (`DESCRIBE homer_lake.main.<table>` in DuckDB is authoritative).

---

## 6. Troubleshooting

| Symptom | Things to check |
|---------|------------------|
| **Save & test** fails immediately | Firewall / bind address; `grpc://` host and port; token mismatch |
| Empty or partial table list | `lake_name` consistent across storage, node, coordinator; catalog path readable by the node process |
| **Unauthenticated** | `auth_token` set on server but missing or wrong Bearer token from Grafana |
| Proxy errors with multiple nodes | Every node must expose the **same** logical DuckLake schema where Grafana expects a single database; the proxy concatenates streams, it does not deduplicate rows across shards |

---

## Further reading

- [Apache Arrow FlightSQL](https://arrow.apache.org/docs/format/FlightSql.html)
- [FLIGHTSQL.md](FLIGHTSQL.md) — ports and protocol split in homer-core
- [NODE.md](NODE.md) — `flight_server` vs `flightsql_server` configuration
