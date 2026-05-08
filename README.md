<img src="https://github.com/user-attachments/assets/3fc9d420-938a-44cd-b2ab-47a574e00c55" width=200 />
<img src="https://github.com/user-attachments/assets/85d47c71-0feb-452a-b45a-2519b44a7fac" width=215 />

# homer eleven
> 100% Opensource Telecom Observability since 2011

`homer` is the *all-in-one* HEP capture and API server monolith powering Homer 11.x data lake

<img width="600"  alt="homer11" src="https://github.com/user-attachments/assets/1ee359d4-f2e1-41d3-9965-22a244d06e1e" />

## Features

- All-in-One Application *(Writer, Reader, Coordinator, Compactor, API)*
- Modern Codebase in golang for X64/ARM64 on Linux/MacOS
- Powered by DuckDB 1.5 and Apache Arrow/IPC/Parquet
- Datalake design based on DuckLake Catalog and Local/Object Storage
- End-to-End Columnar OTLP Design w/ on-demand query execution
- Linear Scaling to query over shared Object Storage catalog/pool
- Flexible Schema support for growing problems and protocols
- Backwards compatible with all HEPv3 Agents
- Easy to maintain, operate and scale *(down to zero!)*
- Cloud Native Design for K8s and standard deployments
- Built-In User Interface for Humans
- MCP support and LLM/Agent friendly design *(steal our boring jobs)*


## Architecture

Homer uses a modular architecture with four main components:

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                           Homer Core                                        │
│                                                                             │
│  ┌─────────────┐   ┌─────────────┐   ┌──────────┐   ┌─────────────────┐     │
│  │   Ingest    │   │   Storage   │   │   Node   │   │   Coordinator   │     │
│  │  (HEP recv) │──▶│  (DuckLake) │──▶│ gRPC/HTTP│──▶│    (REST API)   │     │
│  └─────────────┘   └─────────────┘   └──────────┘   └─────────────────┘     │
│   UDP/TCP/HTTP      Parquet+S3     Airport :50051     HTTP :8080            │
│                                    FlightSQL :50055  (opt. proxy :32010)    │
└─────────────────────────────────────────────────────────────────────────────┘
```

### Modules

| Module | Description |
|--------|-------------|
| **Ingest** | Receives HEP packets via UDP/TCP/TLS/HTTP/HTTPS |
| **Storage** | Writes data to DuckLake (Parquet + catalog) |
| **Node** | Airport gRPC + HTTP `/query`; optional Arrow FlightSQL for Grafana ([docs/FLIGHTSQL.md](docs/FLIGHTSQL.md)) |
| **Coordinator** | REST API gateway for UI and external applications |

## Quick Start

### All-in-One Deployment

```json
{
  "ingest": {
    "enable": true,
    "udp": { "enable": true, "port": 9060 },
    "http": { "enable": true, "port": 9080 }
  },
  "storage": {
    "enable": true,
    "ducklake": {
      "catalog_path": "/data/homer/homer_catalog.sqlite",
      "data_path": "/data/homer/parquet"
    }
  },
  "node": {
    "enable": true,
    "flight_server": { "port": 50051 }
  },
  "coordinator": {
    "enable": true,
    "http_server": { "port": 8080 }
  }
}
```

### Build & Run

```bash
# Build
make

# Run as server (default mode)
./homer --config-path /etc/homer/homer.json

# With debug logging
./homer --config-path /etc/homer/homer.json --log-level debug
```

## Subcommands

Homer uses a subcommand-based CLI. Running `homer` without arguments starts the server.

```
homer                         Run the server (default)
homer search [flags]          Search Homer data via coordinator API
homer cli [flags]             Interactive DuckLake SQL shell
homer system [flags]          System operations (compaction, extensions, reload)
homer wizard [flags]          Interactive config generator wizard
homer mcp [flags]             Start MCP stdio server
homer version                 Show version
homer help                    Show full help with all flags
```

### Server Mode (default)

```bash
homer --config-path /etc/homer/homer.json
homer --config-path /etc/homer/homer.json --log-level debug --syslog-disable
```

| Flag | Description |
|------|-------------|
| `--config-path <path>` | Path to config file or directory |
| `--log-level <level>` | Log level: debug, info, warn, error, trace |
| `--syslog-disable` | Disable syslog, use only stdout |
| `--pid-file <path>` | PID file path (default: /var/run/homer-core.pid) |

### Search (via coordinator API)

Search Homer data from the command line with table, vertical, CSV, JSON, chart, or call flow output.

```bash
# Basic SIP search (last hour)
homer search --host 10.0.0.1:8081 --user admin --pass secret

# Search INVITE messages with call flow diagram
homer search --host 10.0.0.1:8081 --method INVITE --format callflow

# Search by Call-ID
homer search --host 10.0.0.1:8081 --call-id "abc123@host" --format vertical

# Post-filter: only INVITEs and BYEs, exclude provisional responses
homer search --host 10.0.0.1:8081 --grep "INVITE,BYE" --exclude "100,183"

# Interactive TUI mode
homer search --host 10.0.0.1:8081 --interactive
```

See [docs/SEARCH.md](docs/SEARCH.md) for full documentation and examples.

### CLI (DuckLake SQL Shell)

Interactive SQL shell for direct DuckLake queries:

```bash
# Start interactive CLI
homer cli --config-path /etc/homer/homer.json

# Execute single query and exit
homer cli --config-path /etc/homer/homer.json --query "SELECT COUNT(*) FROM homer_lake.main.hep_proto_1_call"
```

| Command | Description |
|---------|-------------|
| `help`, `\h`, `\?` | Show help |
| `tables`, `\dt` | List available tables |
| `clear`, `\c` | Clear screen |
| `exit`, `quit`, `\q` | Exit CLI |

### System Operations

```bash
# Run full compaction
homer system --config-path /etc/homer/homer.json --compaction-force

# Install DuckDB extensions
homer system --config-path /etc/homer/homer.json --install-extensions

# Show DuckDB version
homer system --config-path /etc/homer/homer.json --duckdb-version

# Generate example config
homer system --generate-example-config > homer.json

# Reload running process
homer system --reload
```

### Wizard (Config Generator)

Interactive wizard that generates a complete `homer.json` config:

```bash
# Interactive TUI wizard
homer wizard

# Non-interactive: generate config for a specific deployment profile
homer wizard --profile all-in-one --output homer.json
homer wizard --profile writer --output homer-writer.json
homer wizard --profile coordinator --output homer-coordinator.json
homer wizard --profile edge --output homer-edge.json
homer wizard --profile node --output homer-node.json
```

| Profile | Modules Enabled |
|---------|----------------|
| `all-in-one` | ingest + storage + node + coordinator |
| `writer` | ingest + storage |
| `edge` | ingest + storage + node |
| `coordinator` | coordinator only |
| `node` | node only |

## Configuration Examples

See the `examples/` directory:

| File | Description |
|------|-------------|
| `homer.json` | All-in-one deployment |
| `homer-writer.json` | Ingest + Storage + Node |
| `homer-node.json` | Node only (read-only) |
| `homer-coordinator.json` | Coordinator only |
| `homer-edge.json` | Edge deployment |

## Documentation

- [Search CLI](docs/SEARCH.md) - Search from the command line (examples, formats, call flow)
- [Config Wizard](docs/WIZARD.md) - Interactive config generator (TUI + presets)
- [Coordinator Module](docs/COORDINATOR.md) - REST API gateway
- [MCP UI Guide](docs/MCP_UI_GUIDE.md) - Natural-language query assistant (configuration + UI usage)
- [Node Module](docs/NODE.md) - FlightSQL data server
- [Storage Architecture](docs/STORAGE_ARCHITECTURE.md) - DuckLake storage
- [Storage Policies](docs/STORAGE_POLICIES.md) - Tiered storage (hot/cold)
- [Compaction Setup](docs/COMPACTION_SETUP.md) - File compaction

## License
Released under the [AGPL-3.0 License](LICENSE.md)

> Copyright (C) 2025 QXIP BV
