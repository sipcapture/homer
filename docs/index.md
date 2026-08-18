<div class="homer-hero" markdown="1">

![Homer logo](assets/homer-logo.png)

<p class="homer-tagline">Homer eleven — telecom observability since 2011</p>

</div>

# Homer 11 Documentation

Homer eleven is the all-in-one HEP capture and API server for the Homer 11.x data lake:
ingest, DuckLake storage, Node (FlightSQL), and Coordinator (REST API).

## Quick links

| Topic | Guide |
|-------|--------|
| First-time setup | [Config wizard](WIZARD.md) |
| CLI search | [Search CLI](SEARCH.md) |
| Catalog backup / restore | [Catalog CLI](CATALOG.md) |
| Dashboard search & mappings | [URL search](SEARCH_URL.md) · [External app Call-ID links](SEARCH_URL.md#external-apps) · [Mappings & fields](SEARCH_MAPPINGS_AND_FIELDS.md) |
| REST API | [Coordinator](COORDINATOR.md) · [OpenAPI (Swagger)](swagger/index.html) |
| Authentication & security | [Security hardening](SECURITY.md) · [LDAP & OAuth](AUTH_LDAP_AND_OAUTH.md) · [UI tokens](UI_COORDINATOR_AUTH_AND_TOKENS.md) |
| Alerts & drill-down | [Dashboard alerts](ALERTS.md) · [URL search](SEARCH_URL.md) |
| Storage | [Architecture](STORAGE_ARCHITECTURE.md) · [Layout](STORAGE_LAYOUT.md) · [Policies](STORAGE_POLICIES.md) · [Retention](RETENTION.md) · [Catalog backup](CATALOG.md) |
| Performance | [Ingest tuning](INGEST_PERFORMANCE.md) · [DuckDB tuning](DUCKDB_TUNING.md) · [Troubleshooting](TROUBLESHOOTING.md) · [OOM](OOM.md) |
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
