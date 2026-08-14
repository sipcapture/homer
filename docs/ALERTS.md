# Dashboard alerts and search drill-down

Homer can store alert records and open the **dashboard search that shows what fired**. Idea from [Daniel-Constantin Mierla](https://github.com/miconda) (Kamailio): close the loop after a notification (Grafana / VoxAgent) so the operator lands on the right query.

Related:

- [Dashboard URL search](SEARCH_URL.md) — `/?call_id=&from=&to=#dashboard`
- [UI and API tokens](UI_COORDINATOR_AUTH_AND_TOKENS.md) — `Auth-Token` for Grafana webhooks
- Coordinator: `GET`/`POST`/`DELETE /api/v4/alerts`

## Flow

1. Grafana (or any source) `POST /api/v4/alerts` with title/severity/message and a **payload that retains the query**.
2. **Settings → Alerts** lists those records and keeps the original query.
3. **Open in search** (or a click on the Alert widget with source **Alert store**) applies the stored filters on the dashboard.

## Create an alert

Authenticated (`Authorization: Bearer` or **`Auth-Token`** when `api_settings.enable_token_access` is true):

```bash
curl -sS -H "Auth-Token: <secret>" -H "Content-Type: application/json" \
  -X POST http://127.0.0.1:8080/api/v4/alerts \
  -d '{
    "severity": "critical",
    "title": "ASR drop on SBC-A",
    "message": "ASR < 80% last 5m",
    "payload": {
      "source": "grafana",
      "query": "SELECT count(*) FROM homer_lake.main.hep_proto_1_call WHERE src_ip = '\''10.0.0.1'\''",
      "search": {
        "src_ip": "10.0.0.1",
        "from": 1710000000000,
        "to": 1710000300000,
        "proto_type": 1,
        "event_type": "call"
      },
      "homer_url": "https://homer.example/?src_ip=10.0.0.1&from=1710000000000&to=1710000300000#dashboard"
    }
  }'
```

`title` or `message` is required. `payload` is optional JSON.

### Payload fields the UI understands

| Field | Purpose |
|-------|---------|
| `search` | Filters for dashboard search (`call_id`, `src_ip`, `dst_ip`, `from_user`, `to_user`, `method`, `proto_type`, `event_type`, `from`/`to` ms, `minutes`) |
| `query` | Retained SQL (or other query text) shown in the Alerts tab; not executed automatically |
| `homer_url` | Full or relative Homer deep link. Used if `search` is missing. Must be `http(s)` or a same-app path (`/…`) |
| `source` | Label only (`grafana`, …) |

Prefer **`search` with absolute `from`/`to`** so replay is exact. Relative `minutes` is evaluated at click time.

## Grafana contact point

Point a Grafana webhook at `POST https://<homer>/api/v4/alerts` with header `Auth-Token`. Map annotations/labels into `payload.search` (and optionally `payload.query` / `payload.homer_url`). Homer does not replace Grafana as the alerting engine.

## UI

| Place | Behaviour |
|-------|-----------|
| **Settings → Alerts** | List stored rows, show retained query, **Open in search** |
| **Alert widget** (source **Alert store**) | Same list; click a row with stored filters to run dashboard search |

SQL source on the widget still runs a lake query on an interval (unchanged).

## Optional: VoxAgent

Include `payload.homer_url` (or the same query string) in the voice/notification payload so the call and the UI land on the same investigation.
