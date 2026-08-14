# Dashboard URL search

Homer 11 can open the dashboard with **prefilled search filters** and run a query automatically. This replaces the homer-app Share URL pattern ([homer-app#607](https://github.com/sipcapture/homer-app/issues/607)) with simple query parameters.

Related docs:

- [Search CLI](SEARCH.md) — terminal search and `--proto` names
- [Dashboard alerts](ALERTS.md) — `POST /api/v4/alerts` and Open in search
- [Mapping examples](../examples/mappings/README.md) — `fields_mapping` JSON per protocol
- [Search mappings and field types](SEARCH_MAPPINGS_AND_FIELDS.md) — form fields, virtual filters, `form_type` reference
- [UI and API tokens](UI_COORDINATOR_AUTH_AND_TOKENS.md) — `Auth-Token` for server-side integrations
- [Coordinator API](COORDINATOR.md) — `POST /api/v4/transactions/view/link`
- API: `POST /api/v4/transactions/search` — same `filter` object the UI sends

---

## External applications (Call-ID drill-down) {#external-apps}

Common question: an external call-search app has a **Call-ID** and you want one click to open Homer (SIP trace or search results) without copy-paste. See also [GitHub discussion #680](https://github.com/sipcapture/homer/discussions/680).

Browsers only follow **GET** links (`<a href>`, redirects). You cannot POST a Call-ID from HTML alone. Use one of the patterns below.

### Choose an integration pattern

| Goal | Pattern | User must log into Homer UI? | Auth |
|------|---------|-------------------------------|------|
| Open **dashboard search** with Call-ID prefilled | GET deep link (this page) | **Yes** (cookie or remembered JWT in same browser profile) | None in URL |
| Open **standalone SIP trace** HTML page | Backend `view/link` → redirect to `/export/view/<uuid>` | **No** (one-time view token) | `Auth-Token` or JWT on your server |
| Embed results in **your own UI** | `POST /api/v4/transactions/search` or `/messages` | No | `Auth-Token` or JWT on your server |

Always pass a **time window** (`from` / `to` in ms, or `minutes` / `m`) together with the Call-ID.

### Homer 11 — dashboard deep link (logged-in users)

After you sign in once, the UI session is kept in an **HttpOnly cookie** shared across tabs on the same Homer host (see [UI and API tokens](UI_COORDINATOR_AUTH_AND_TOKENS.md)). Opening a deep link in a **new tab** should not require login again. If it does, check that cookies are allowed for the Homer origin and that you are not in a private window with an isolated cookie jar.

Recommended URL (coordinator host, flat query params):

```text
https://<homer-host>/?call_id=<CALL-ID>&from=<from_ms>&to=<to_ms>#dashboard
```

- Alias: `callid` instead of `call_id`
- URL-encode special characters in the Call-ID (`@`, `:`, …)
- Defaults: `proto_type=1`, `event_type=call` (SIP calls)

Example:

```text
https://homer.example/?call_id=abc-def-ghi%4010.0.0.1&from=1710000000000&to=1710086400000#dashboard
```

Relative window:

```text
https://<homer-host>/?call_id=<CALL-ID>&m=60#dashboard
```

### Homer 11 — SIP trace without UI login (API token on server)

For portals that already have Homer API access but users are **not** logged into Homer:

1. Enable static API tokens: `coordinator.api_settings.enable_token_access` (see [UI and API tokens](UI_COORDINATOR_AUTH_AND_TOKENS.md)).
2. Your backend calls:

```http
POST /api/v4/transactions/view/link
Auth-Token: <secret>
Content-Type: application/json

{
  "session_id": "your-call-id-here",
  "proto_type": 1,
  "event_type": "call",
  "timestamp": {
    "from": 1710000000000,
    "to": 1710086400000
  }
}
```

3. Response `data.url_view` is e.g. `/export/view/<uuid>`.
4. Redirect the browser:

```text
https://<homer-host>/export/view/<uuid>
```

The view link is **time-limited** (default 72h) and capped by `coordinator.transaction_view_max_opens` (default 3 successful opens). The browser does **not** send `Auth-Token` to open that page.

Use `Authorization: Bearer <jwt>` instead of `Auth-Token` when calling the API with a service account session JWT (not the one-time view UUID).

### Homer 7 — legacy homer-ui JSON URL

On Homer 7 (homer-app + homer-ui, often port **9080**), operators use a single JSON blob as the query string on `/search/result`:

```text
http://homer:9080/search/result?{"timestamp":{"from":<from_ms>,"to":<to_ms>},"param":{"search":{"1_call":{"callid":["<call-id>"]}}}}=
```

Notes:

- Trailing `=` is required for homer-ui parsing.
- Profile key `1_call` is the SIP-calls mapping in homer-app.
- `callid` is a **JSON array** on Homer 7.

Homer 11 does **not** use path `/search/result`. Prefer flat `/?call_id=…#dashboard` above. Legacy JSON (without `/search/result`) is partially supported on the coordinator UI; `callid` as an array may not parse — use flat `call_id` for new links.

---

## URL shape

Use the dashboard hash and put filters in the query string.

**Recommended** — query **before** the hash:

```text
https://coordinator.example/?from_user=123123&minutes=10#dashboard
```

**Also supported** — query after the hash:

```text
https://coordinator.example/#dashboard?from_user=123123&minutes=10
```

On load the UI:

1. Switches to the dashboard (you must already be logged in, or complete OAuth/login first).
2. Applies the time window and filter fields to the Protocol Search widget (if present).
3. Runs search on the first **Results** or **Time Chart** widget.
4. Strips search parameters from the address bar (filters stay in the widget).

Copy a link from the UI: Protocol Search widget → link icon next to **Clear**.

Implementation: `src/ui/src/dashboard/searchDeepLink.ts`, `SearchDeepLinkBootstrap.tsx`.

---

## Query parameters

### Time range

| Parameter | Description |
|-----------|-------------|
| `minutes` or `m` | Relative window ending **now** (default **60** when only filters are set) |
| `from`, `to` | Absolute range, Unix epoch **milliseconds** |

Example — last 15 minutes:

```text
/?from_user=alice&m=15#dashboard
```

Example — fixed window (same as homer-app issue):

```text
/?from_user=123123&from=1743857605187&to=1743858205187#dashboard
```

### Protocol selection

| Parameter | Alias | Description |
|-----------|-------|-------------|
| `proto_type` | `proto` | HEP / virtual protocol id (see table below) |
| `event_type` | `event` | Mapping **profile** name (`call`, `registration`, `default`, …) |

Defaults if omitted: `proto_type=1`, `event_type=call` (SIP calls).

### SIP-oriented filters (built into URL parser)

At least one of these must be present or the deep link is ignored.

| Parameter | Aliases | Maps to API `filter` |
|-----------|---------|----------------------|
| `from_user` | `user_from`, `caller` | `from_user` |
| `to_user` | `user_to`, `callee` | `to_user` |
| `call_id` | `callid` | `call_id` |
| `method` | — | `method` |
| `src_ip` | — | `src_ip` |
| `dst_ip` | — | `dst_ip` |
| `limit` | — | `param.limit` (default 50) |

Example — SIP From user, registration profile:

```text
/?proto_type=1&event_type=registration&from_user=1000&minutes=30#dashboard
```

---

## Built-in protocols (`proto_type` + `event_type`)

These match seeded `mapping_schema` rows and the Search CLI `--proto` table in [SEARCH.md](SEARCH.md).

| Protocol | `proto_type` | Typical `event_type` | URL example (extra params) |
|----------|--------------|----------------------|----------------------------|
| SIP calls | `1` | `call` | `from_user`, `to_user`, `call_id`, `method` |
| SIP (generic) | `1` | `default` | same |
| SIP registration | `1` | `registration` | `from_user` (AOR/contact in UI) |
| RTCP | `5` | `default` | `src_ip`, `dst_ip`, `call_id` (payload search) |
| DNS | `53` | `default` | `src_ip`, `dst_ip`, `call_id` |
| LOG | `100` | `default` | `call_id` (message text search) |
| OTLP traces | `200` | `default` | set `proto_type=200`; use UI or API for `trace_id` (see below) |
| OTLP metrics | `201` | `default` | `proto_type=201` |
| OTLP logs | `202` | `default` | `proto_type=202` |
| RTP agent | `34` | `default` | CLI/API; URL uses shared IP / `call_id` fields |
| Line Protocol | `300` | `schema__table` | e.g. `event_type=main__cpu` (see LP section) |

Examples:

```text
# RTCP on a subnet
/?proto_type=5&event_type=default&src_ip=10.0.1.10&minutes=60#dashboard

# DNS
/?proto_type=53&dst_ip=8.8.8.8&m=120#dashboard

# OTLP traces (protocol only; refine in widget or API)
/?proto_type=200&event_type=default&minutes=15#dashboard

# SIP registration
/?proto_type=1&event_type=registration&from_user=sip:user@domain.com#dashboard
```

### Line Protocol (`proto_type=300`)

LP rows are created when measurements are ingested (`lp_mapping_sync`). The profile name is **`schema__table`**, for example `main__cpu`.

```text
/?proto_type=300&event_type=main__cpu&minutes=10#dashboard
```

Field filters for LP in the URL are not wired yet; use the Protocol Search widget after the link opens, or the CLI with `--proto lp --event-type main__cpu`.

---

## Legacy homer-app JSON URLs

homer-app used a single JSON blob as the query string:

```text
/search/result?{"timestamp":{"from":...,"to":...},"param":{"search":{"1_call":{"user_from":"123123"}}}}
```

Homer 11 accepts the same JSON when it starts with `{` (before or as `?q=...`). Known mappings:

- `param.search.<profile>.user_from` → `from_user`
- `param.search.<profile>.user_to` / `callee` → `to_user`
- `param.search.<profile>.call_id` / `callid` (string) → `call_id`
- `timestamp.from` / `timestamp.to` → time range

Prefer the flat query parameters above for new integrations. For Homer 7-style `callid` **arrays**, use [flat Call-ID links](#external-apps) instead.

---

## API payload (what the URL becomes)

Deep links end up as `POST /api/v4/transactions/search`:

```json
{
  "filter": {
    "proto_type": 1,
    "event_type": "call",
    "from_user": "123123"
  },
  "param": { "limit": 50 },
  "timestamp": { "from": 1743857605187, "to": 1743858205187 }
}
```

Field names in `filter` must match the mapping field **`id`** values (see `examples/mappings/fields_*.json`) and what `buildSearchSQLV4` understands in `transactions_v4.go`.

---

## Adding a new protocol (data + UI)

### 1. Ingest and storage

Ensure the node writes the protocol into DuckLake (HEP type or custom table). For HEP types, table suffixes follow `hep_proto_<id>_<profile>` (see [Storage layout](STORAGE_LAYOUT.md)).

### 2. `mapping_schema` row

Add a row so the Protocol Search widget can list fields and the coordinator can build SQL:

1. Copy a seed from [`examples/mappings/`](../examples/mappings/) or create `fields_<hepid>_<profile>.json`.
2. Each searchable column needs an `"id"` (filter key), `"name"`, `"form_type"`, etc.
3. Insert via Settings → Mappings UI, or SQL/API into settings DB `mapping_schema`:
   - `hepid` — same integer as `proto_type` in search
   - `profile` — same string as `event_type` in search
   - `hep_alias` — display label
   - `fields_mapping` — JSON array from your file

See [examples/mappings/README.md](../examples/mappings/README.md).

### 3. Backend filter support

In `src/coordinator/handlers/transactions_v4.go`, `buildSearchSQLV4` must translate your field ids into SQL for that table. SIP/RTCP/DNS/LOG/OTLP paths already exist; a **new** HEP type may need a new branch or generic column mapping.

### 4. Dashboard widget

Operators add a **Protocol Search** widget, pick the mapping (hepid + profile) in widget settings, and a **Results** widget. Presets can be registered in `src/ui/src/dashboard/widgets/registry.ts` (`search` extras) and `SearchPanel.tsx` (`SIP_PRESETS`, `OTLP_PRESETS`, …).

---

## Extending URL parameters for more fields

Today the URL parser only exposes a **fixed set** of query keys (SIP-friendly names + `proto_type` / `event_type`). Protocol-specific ids such as `trace_id`, `service_name`, or `aor` are **not** read from the URL unless you extend the code.

### Option A — Add named parameters (simple)

Edit `src/ui/src/dashboard/searchDeepLink.ts`:

1. Add the field to `SearchDeepLinkSpec` and `TRIGGER_KEYS` if it should alone trigger a search.
2. Parse it in `parseSearchDeepLink()` from `URLSearchParams`.
3. Copy it in `buildSearchPayload()` into `filter.<id>`.
4. Mirror in `buildSearchDeepLinkURL()` and `deepLinkSpecToFormFields()` for the copy-link button.
5. Add tests in `searchDeepLink.test.ts`.

Example — OTLP `trace_id`:

```typescript
// parse
spec.trace_id = p.get('trace_id')?.trim() || undefined
// payload
if (spec.trace_id) filter.trace_id = spec.trace_id
```

### Option B — Generic `filter.<field>=<value>` (scalable)

Support repeated or prefixed params, e.g. `filter.trace_id=abc` or `f.trace_id=abc`, and merge any `filter.*` key into the API `filter` object after `proto_type` / `event_type` are set. This avoids editing the parser for every mapping field but requires careful validation (allowlist or mapping-driven ids only).

### Option C — Encoded JSON filter (power users)

A single param `filter={"proto_type":200,"trace_id":"..."}` URL-encoded. Same flexibility as homer-app but harder to hand-edit; good for tools generating links.

### Option D — No URL change

Generate links only through the UI copy button after configuring the widget (dynamic mode already serializes all visible fields into the stored form; extending copy-link to emit every configured field id is the smallest UX win).

---

## Requirements and limitations

| Topic | Detail |
|-------|--------|
| Authentication | User must have a valid session/JWT; URL does not embed credentials. |
| Dashboard layout | At least one **results** or **chart** widget must exist on the active dashboard. |
| Search widget | Optional; without it, filters still run but the form may not show values. |
| Custom mappings | URL uses **field ids** from `fields_mapping`; aliases like `user_from` are SIP legacy only. |
| Dynamic widget fields | Extra mapping fields only appear in the URL after Option A/B/D above. |
| SQL / AI tabs | Deep links only drive structured **Form** search, not SQL or MCP tabs. |

---

## Troubleshooting

| Symptom | Check |
|---------|--------|
| Dashboard opens, no search | No `from_user` / `call_id` / … trigger param; add at least one filter key. |
| All rows returned | Filter empty in API body — wrong param name or legacy JSON profile key (`1_call` vs `call`). |
| Wrong protocol | Set `proto_type` and `event_type` to match the mapping row. |
| No results widget | Add Results Table from the empty-state buttons in Protocol Search. |
| homer-app link | Path `/search/result` is not used in Homer 11; use `/#dashboard` and coordinator host. |

---

## Quick reference

```text
# SIP by From user, last hour
/?from_user=123123&minutes=60#dashboard

# By Call-ID
/?call_id=abc-def-ghi@host&from=1700000000000&to=1700003600000#dashboard

# RTCP
/?proto_type=5&src_ip=10.0.0.1&m=30#dashboard

# OTLP traces (open correct protocol; add trace_id in UI or extend URL parser)
/?proto_type=200&minutes=15#dashboard
```
