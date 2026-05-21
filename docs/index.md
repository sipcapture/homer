# Homer 11 Documentation

Homer eleven is the all-in-one HEP capture and API server for the Homer 11.x data lake:
ingest, DuckLake storage, Node (FlightSQL), and Coordinator (REST API).

## Quick links

| Topic | Guide |
|-------|--------|
| First-time setup | [Config wizard](WIZARD.md) |
| CLI search | [Search CLI](SEARCH.md) |
| REST API | [Coordinator](COORDINATOR.md) · [OpenAPI (Swagger)](swagger/index.html) |
| Storage | [Architecture](STORAGE_ARCHITECTURE.md) · [Policies](STORAGE_POLICIES.md) |
| Performance | [Ingest tuning](INGEST_PERFORMANCE.md) · [DuckDB tuning](DUCKDB_TUNING.md) |
| MCP / LLM | [MCP server](MCP.md) · [MCP UI guide](MCP_UI_GUIDE.md) |

## Repository

- Source: [github.com/sipcapture/homer](https://github.com/sipcapture/homer) (branch `homer11`)
- Releases: [GitHub Releases](https://github.com/sipcapture/homer/releases)
- Docker: `ghcr.io/sipcapture/homer:latest`

## Modules

```
┌─────────────┐   ┌─────────────┐   ┌──────────┐   ┌─────────────────┐
│   Ingest    │──▶│   Storage   │──▶│   Node   │──▶│   Coordinator   │
│  (HEP recv) │   │  (DuckLake) │   │ gRPC/HTTP│   │    (REST API)   │
└─────────────┘   └─────────────┘   └──────────┘   └─────────────────┘
```

See the sidebar for the full documentation index.
