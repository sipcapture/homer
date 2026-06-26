# Troubleshooting

Common operator issues for the Homer coordinator, search API, and UI.

| Symptom | Section |
|---------|---------|
| Search times out after ~30 seconds | [Search timeouts](#search-timeouts-30-seconds) |
| `500` on `/api/v4/transactions/search`, OOM / memory errors | [OOM.md](OOM.md) |
| Grafana / FlightSQL query issues | [GRAFANA_INTEGRATION.md](GRAFANA_INTEGRATION.md) |
| Auth / JWT / bootstrap password | [SECURITY.md](SECURITY.md) |

---

## Search timeouts (~30 seconds)

### Symptoms

- Dashboard search or **Call search** stops after about **30 seconds**
- Browser shows a failed request, `504`, connection reset, or generic network error
- `homer search` or `curl` to `/api/v4/transactions/search` fails around the same duration
- Coordinator logs may show context deadline / timeout on node queries

Homer uses **three separate 30-second defaults**. Long-running searches must raise **all relevant** values together.

### 1. Node query timeout (main search limit)

**Setting:** `coordinator.query_timeout_sec` (default **30**)

Bounds how long the coordinator waits for each **node** `POST /query` call (`FlightService`). This applies to transaction search, raw SQL (`/api/v4/query`), statistics queries, and most read paths that fan out to nodes.

```json
{
  "coordinator": {
    "query_timeout_sec": 120
  }
}
```

**Environment:**

```bash
HOMER_COORDINATOR_QUERY_TIMEOUT_SEC=120
```

Defined in [`src/config/config.go`](../src/config/config.go) (`CoordinatorConfig.QueryTimeoutSec`). Applied in [`src/coordinator/coordinator.go`](../src/coordinator/coordinator.go) when creating `FlightService`.

### 2. Coordinator HTTP server timeouts (UI / API clients)

**Settings:** `coordinator.http_server.read_timeout` and `coordinator.http_server.write_timeout` (default **30** each)

If the coordinator spends longer than `write_timeout` building a search response, the **browser or API client** disconnects even when `query_timeout_sec` is higher.

```json
{
  "coordinator": {
    "http_server": {
      "read_timeout": 120,
      "write_timeout": 120
    },
    "query_timeout_sec": 120
  }
}
```

**Environment:**

```bash
HOMER_COORDINATOR_HTTP_SERVER_READ_TIMEOUT=120
HOMER_COORDINATOR_HTTP_SERVER_WRITE_TIMEOUT=120
HOMER_COORDINATOR_QUERY_TIMEOUT_SEC=120
```

See [COORDINATOR.md](COORDINATOR.md#http_server) for other `http_server` fields.

### 3. Not the same as ingest or MCP timeouts

| Setting | Scope |
|---------|--------|
| `coordinator.query_timeout_sec` | Coordinator → node search/query |
| `coordinator.http_server.read_timeout` / `write_timeout` | Client → coordinator HTTP API |
| `ingest.http.read_timeout` / `write_timeout` | HEP ingest HTTP receiver only |
| `mcp.request_timeout_sec` | MCP assistant backend calls only |

### Recommended values

| Workload | `query_timeout_sec` | `read_timeout` / `write_timeout` |
|----------|---------------------|----------------------------------|
| Interactive UI (default) | 30 | 30 |
| Large time ranges, busy clusters | 120–300 | 120–300 (match or exceed query timeout) |

After changing config, restart Homer (or at least the coordinator process):

```bash
sudo systemctl restart homer-core
```

### Verify

1. Set all three timeouts to the same higher value (e.g. 120).
2. Run a search that previously failed at ~30s.
3. If it still fails quickly, check [OOM.md](OOM.md) — DuckDB may be aborting with memory errors rather than a wall-clock timeout.

### Related

- [SEARCH.md](SEARCH.md) — CLI search (`homer search` uses its own HTTP client timeout; increase with tooling flags or proxy settings if needed)
- [DUCKDB_TUNING.md](DUCKDB_TUNING.md) — reduce query duration with `threads` / `memory_limit`
- [COORDINATOR.md](COORDINATOR.md) — full coordinator configuration
