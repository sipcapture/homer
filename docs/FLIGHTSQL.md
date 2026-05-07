# FlightSQL Overview

Homer exposes **two different gRPC stacks** on the node:

| Listener | Port (typical) | Protocol | Typical clients |
|----------|----------------|----------|-----------------|
| `node.flight_server` | 50051 | [DuckDB Airport](https://github.com/hugr-lab/airport-go) (Arrow Flight with PATH descriptors) | DuckDB `ATTACH 'arrow_flight://…'`, pyarrow against Airport |
| `node.flightsql_server` | 50055 | **Apache Arrow FlightSQL** (`flightsql.BaseServer`) | Grafana **InfluxDB** datasource with **FlightSQL** / ADBC FlightSQL |

The coordinator REST API still talks to nodes over **HTTP** `POST /query` on `node.flight_server.port + 1`. Optional **`coordinator.flightsql_server`** (default **32010** when enabled with port `0`) is a **FlightSQL proxy** that fans out to each `coordinator.nodes[].flightsql_port` (must match the node’s FlightSQL listener).

## Grafana (InfluxDB datasource, FlightSQL)

Step-by-step datasource setup, proxy configuration, and troubleshooting: **[GRAFANA_INTEGRATION.md](GRAFANA_INTEGRATION.md)**.

1. Enable `node.flightsql_server.enable` and set `auth_token` if you want Bearer auth (same header pattern as Airport).
2. Datasource type: **InfluxDB**, version **InfluxQL** or **SQL** / **FlightSQL** per your Grafana build (use the FlightSQL / InfluxDB 3 style URL).
3. URL: `grpc://<node-host>:50055` (or `grpc://<coordinator-host>:32010` when using the coordinator proxy and `flightsql_port` on each node entry).
4. If you set `auth_token`, add **Metadata** or **HTTP Headers** so requests include `Authorization: Bearer <token>` (Grafana field names vary by version).

FlightSQL is a protocol for high-performance SQL database access built on Apache Arrow Flight.

## What is FlightSQL?

```
┌──────────────┐                    ┌──────────────┐
│    Client    │  ◄── Arrow Data ──►│    Server    │
│  (DuckDB,    │     (columnar,     │  (Homer      │
│   Python,    │      zero-copy)    │   Node)      │
│   Go, etc.)  │                    │              │
└──────────────┘                    └──────────────┘
```

**Traditional SQL access (JDBC/ODBC):**
- Row-by-row data transfer
- Serialization/deserialization overhead
- Multiple round-trips

**FlightSQL:**
- Columnar data transfer (Apache Arrow format)
- Zero-copy reads where possible
- Streaming with minimal round-trips
- 10-100x faster for analytical queries

## How It Works

```
1. Client sends SQL query
   ┌────────┐  "SELECT * FROM ..."   ┌────────┐
   │ Client │ ───────────────────────► │ Server │
   └────────┘                          └────────┘

2. Server executes query, returns Arrow RecordBatches
   ┌────────┐  ◄─── Arrow Batches ─── ┌────────┐
   │ Client │       (columnar)        │ Server │
   └────────┘                          └────────┘

3. Client processes data directly (no deserialization)
```

## Why FlightSQL for Homer?

| Feature | Benefit |
|---------|---------|
| **Columnar format** | Efficient for analytical queries (SELECT specific columns) |
| **Streaming** | Handle large result sets without loading all into memory |
| **gRPC transport** | Efficient binary protocol, supports TLS |
| **Language support** | Python, Go, Java, Rust, C++, JavaScript |
| **DuckDB native** | Direct integration without conversion |

## Homer Architecture with FlightSQL

```
┌─────────────────────────────────────────────────────────────┐
│                     Homer Coordinator                        │
│         (REST API :8080, optional FlightSQL proxy :32010)   │
└─────────────────────────┬───────────────────────────────────┘
                          │ HTTP/JSON
                          ▼
┌─────────────────────────────────────────────────────────────┐
│                       Homer Node                             │
│            (Airport gRPC :50051, FlightSQL gRPC :50055)      │
│                                                              │
│  ┌────────────────────────────────────────────────────────┐ │
│  │                    DuckDB Engine                        │ │
│  │  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐    │ │
│  │  │ homer_lake_hot│  │homer_lake_cold│  │     ...     │    │ │
│  │  └──────┬──────┘  └──────┬──────┘  └──────┬──────┘    │ │
│  └─────────┼────────────────┼────────────────┼───────────┘ │
└────────────┼────────────────┼────────────────┼─────────────┘
             │                │                │
             ▼                ▼                ▼
      ┌───────────┐    ┌───────────┐    ┌───────────┐
      │  Parquet  │    │  Parquet  │    │  Parquet  │
      │  (Local)  │    │   (S3)    │    │   (R2)    │
      └───────────┘    └───────────┘    └───────────┘
```

## Client Examples

### Python (DuckDB Airport on :50051)

```python
from pyarrow.flight import FlightClient

client = FlightClient("grpc://localhost:50051")
info = client.get_flight_info(
    FlightDescriptor.for_command(b"SELECT * FROM homer_lake.main.sip LIMIT 10")
)
reader = client.do_get(info.endpoints[0].ticket)
df = reader.read_pandas()
```

### DuckDB (Airport attach on :50051)

```sql
ATTACH 'arrow_flight://localhost:50051' AS homer;
SELECT * FROM homer.homer_lake.main.sip LIMIT 10;
```

### Go (Apache FlightSQL on :50055)

```go
ctx := context.Background()
client, _ := flightsql.NewClient("localhost:50055", nil, nil,
    grpc.WithTransportCredentials(insecure.NewCredentials()))
info, _ := client.Execute(ctx, "SELECT * FROM homer_lake.main.sip LIMIT 10")
reader, _ := client.DoGet(ctx, info.Endpoint[0].Ticket)
defer reader.Release()
```

## Performance Comparison

| Operation | JDBC/ODBC | FlightSQL |
|-----------|-----------|-----------|
| 1M rows SELECT | ~5 sec | ~0.3 sec |
| Column subset | Same cost | Only selected columns transferred |
| Memory usage | Full result in memory | Streaming batches |
| Network | Text/binary serialization | Zero-copy Arrow |

## Key Concepts

- **Flight** - Base protocol for Arrow data streaming
- **FlightSQL** - SQL extension over Flight
- **RecordBatch** - Unit of columnar data transfer
- **Ticket** - Reference to retrieve query results
- **FlightInfo** - Metadata about available data

## Further Reading

- [Apache Arrow Flight](https://arrow.apache.org/docs/format/Flight.html)
- [FlightSQL Specification](https://arrow.apache.org/docs/format/FlightSql.html)
- [DuckDB Arrow Integration](https://duckdb.org/docs/guides/python/sql_on_arrow)
