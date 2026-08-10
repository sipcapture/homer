# Coordinator Module - API Gateway

The Coordinator module provides REST API for frontend applications and orchestrates queries across multiple Node instances via FlightSQL/HTTP.

## Architecture

```
┌─────────────────────────────────────────────────────────────────────┐
│                         Homer Coordinator                            │
│                                                                      │
│  ┌──────────────────────────────────────────────────────────────┐   │
│  │                      REST API (:8080)                         │   │
│  │  /api/v4/transactions/search                                  │   │
│  │  /api/v4/auth/sessions                                        │   │
│  │  /api/v4/dashboards                                           │   │
│  │  ...                                                          │   │
│  └──────────────────────────────────────────────────────────────┘   │
│                              │                                       │
│                              ▼                                       │
│  ┌──────────────────────────────────────────────────────────────┐   │
│  │                    FlightService                              │   │
│  │         (Query routing to nodes via HTTP)                     │   │
│  └──────────────────────────────────────────────────────────────┘   │
│                              │                                       │
│         ┌────────────────────┼────────────────────┐                 │
│         ▼                    ▼                    ▼                 │
│  ┌─────────────┐      ┌─────────────┐      ┌─────────────┐         │
│  │   Node 1    │      │   Node 2    │      │   Node 3    │         │
│  │ :50051/HTTP │      │ :50051/HTTP │      │ :50051/HTTP │         │
│  └─────────────┘      └─────────────┘      └─────────────┘         │
│                                                                      │
│  ┌──────────────────────────────────────────────────────────────┐   │
│  │                   Settings DB (DuckDB)                        │   │
│  │        Users, Dashboards, User Settings                       │   │
│  └──────────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────────┘
```

## Key Features

- **REST API** - Full API for Homer UI (v1, v3, v4 endpoints)
- **Multi-node routing** - Query distribution across multiple nodes
- **JWT Authentication** - Secure API access with tokens
- **OAuth2 Support** - External authentication providers
- **Settings Storage** - DuckDB-based user and dashboard storage
- **Embedded UI** - Optional built-in web interface

## Configuration

### Basic Configuration

```json
{
  "coordinator": {
    "enable": true,
    "http_server": {
      "enable": true,
      "host": "0.0.0.0",
      "port": 8080,
      "read_timeout": 30,
      "write_timeout": 30,
      "static_path": ""
    },
    "nodes": [
      {
        "name": "local",
        "host": "127.0.0.1",
        "port": 50051,
        "use_tls": false,
        "token": "your-node-token",
        "priority": 1
      }
    ],
    "settings_db_path": "/var/lib/homer/homer_settings.duckdb",
    "transaction_view_max_opens": 3,
    "ip_alias_cache_ttl_sec": 30,
    "jwt": {
      "secret": "your-jwt-secret-minimum-32-characters",
      "expire_hours": 24
    },
    "auth": { "type": "internal" }
  }
}
```

`transaction_view_max_opens` (default `3`) caps how many successful `GET /export/view/:uuid` responses each shared view token may serve before it is exhausted (the `expires_at` TTL still applies).

`ip_alias_cache_ttl_sec` (default `30`, min `5`, max `86400`) controls how long the coordinator reuses the in-memory IP-alias lookup table when enriching search/QoS/message rows. Creating, updating, or deleting an alias still clears the cache immediately.

### Multi-Node Configuration

```json
{
  "coordinator": {
    "enable": true,
    "http_server": {
      "host": "0.0.0.0",
      "port": 8080
    },
    "nodes": [
      {
        "name": "node-eu",
        "host": "node-eu.example.com",
        "port": 50051,
        "use_tls": true,
        "token": "eu-node-token",
        "priority": 1
      },
      {
        "name": "node-us",
        "host": "node-us.example.com",
        "port": 50051,
        "use_tls": true,
        "token": "us-node-token",
        "priority": 2
      },
      {
        "name": "node-asia",
        "host": "node-asia.example.com",
        "port": 50051,
        "use_tls": true,
        "token": "asia-node-token",
        "priority": 3
      }
    ],
    "settings_db_path": "/var/lib/homer/homer_settings.duckdb",
    "jwt": {
      "secret": "your-jwt-secret-minimum-32-characters",
      "expire_hours": 24
    },
    "auth": { "type": "internal" }
  }
}
```

### Smart Routing (time-range node pruning)

Opt-in optimization for multi-node clusters. When enabled, the Coordinator skips
querying any node whose data time range cannot overlap a search's time window —
so a "last 15 minutes" search never wakes a node holding only cold archives.
**Off by default; when off, every connected node is queried (unchanged behavior).**

```json
{
  "coordinator": {
    "enable": true,
    "nodes": [
      { "name": "hot",  "host": "hot.example.com",  "port": 50051 },
      { "name": "cold", "host": "cold.example.com", "port": 50051 }
    ],
    "smart_routing": {
      "enable": true
    }
  }
}
```

How it works: each node exposes `GET /metadata/stats` (`{min_ts, max_ts}`); the
Coordinator polls it on the existing node health-check tick and caches each
node's range. A node is skipped only when its cached range provably does not
overlap the query window — its newest data is older than the window start, or
its oldest data is newer than the window end. If pruning would leave no nodes,
all connected nodes are queried, so pruning never drops results.

> **Precondition — historical ingest.** The "newer than the window end" skip
> direction assumes a node's oldest (`min`) timestamp only rises over time (true
> for live capture plus retention). **Historical pcap import or replay that
> ingests packets older than a node's current `min` violates this.** During the
> ≤1 health-check interval before the cache refreshes, such a node could be
> skipped for a query that it now has matching (older) rows for. If you run
> historical/backfill ingest, keep `smart_routing.enable` **off**, or accept up
> to one health-interval of staleness. The "older than the window start"
> direction has no such hazard.

### With OAuth2 Providers

Authorization **code** flow (server exchanges `code`, loads userinfo, provisions DuckDB user). See [AUTH_LDAP_AND_OAUTH.md](./AUTH_LDAP_AND_OAUTH.md#oauth2).

```json
{
  "coordinator": {
    "enable": true,
    "http_server": {
      "host": "0.0.0.0",
      "port": 8080
    },
    "nodes": [
      {
        "name": "local",
        "host": "127.0.0.1",
        "port": 50051
      }
    ],
    "settings_db_path": "/var/lib/homer/homer_settings.duckdb",
    "jwt": {
      "secret": "your-jwt-secret",
      "expire_hours": 24
    },
    "auth": { "type": "internal" },
    "oauth2_provider": {
      "enable": true,
      "name": "google",
      "type": "oauth2",
      "provider_name": "Google",
      "provider_image": "/assets/google.svg",
      "position": 1,
      "client_id": "YOUR_CLIENT_ID.apps.googleusercontent.com",
      "client_secret": "YOUR_CLIENT_SECRET",
      "auth_url": "https://accounts.google.com/o/oauth2/v2/auth",
      "token_url": "https://oauth2.googleapis.com/token",
      "redirect_url": "http://localhost:8080/api/v4/auth/oauth2/google/callback",
      "profile_url": "https://openidconnect.googleapis.com/v1/userinfo",
      "scopes": ["openid", "email", "profile"],
      "use_pkce": false,
      "callback_url": "http://localhost:8080/",
      "auto_redirect": false
    }
  }
}
```

## Configuration Parameters

### http_server

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `enable` | bool | true | Enable HTTP server |
| `host` | string | "0.0.0.0" | Listen address |
| `port` | int | 8080 | HTTP server port |
| `read_timeout` | int | 30 | Read timeout in seconds (raise for long searches — see [TROUBLESHOOTING.md](TROUBLESHOOTING.md#search-timeouts-30-seconds)) |
| `write_timeout` | int | 30 | Write timeout in seconds (raise for long searches — see [TROUBLESHOOTING.md](TROUBLESHOOTING.md#search-timeouts-30-seconds)) |
| `static_path` | string | "" | Path to UI static files (optional) |
| `gamedata_dir` | string | "/usr/local/homer-core/gamedata" | On-disk directory served read-only at `/gamedata/` for large game assets (Doom widget IWAD). A missing directory yields 404s; empty string disables the route. See `docs/VOIPGames.md`. |

### nodes

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `name` | string | - | Node identifier |
| `host` | string | - | Node hostname or IP |
| `port` | int | 50051 | Node FlightSQL port |
| `use_tls` | bool | false | Use TLS for connection |
| `token` | string | "" | Authentication token for node |
| `priority` | int | 1 | Query routing priority (lower = higher priority) |

### query_timeout_sec

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `query_timeout_sec` | int | 30 | Per-query timeout (seconds) for coordinator → node `POST /query`. Raise for long transaction searches — see [TROUBLESHOOTING.md](TROUBLESHOOTING.md#search-timeouts-30-seconds). Env: `HOMER_COORDINATOR_QUERY_TIMEOUT_SEC`. |

### smart_routing

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `enable` | bool | false | Skip nodes whose cached data time range cannot overlap a search's time window (UI search path). Off = every connected node is queried. See [Smart Routing](#smart-routing-time-range-node-pruning) — note the historical-ingest precondition before enabling. |

### jwt

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `secret` | string | "" | JWT signing secret (min 32 characters recommended). When empty at startup, Homer generates a random secret, persists it as `.homer_jwt_secret` beside `settings_db_path`, and always enforces JWT on protected routes. Set explicitly in config for multi-host deployments. |
| `expire_hours` | int | 24 | Token expiration time in hours |

### auth

`coordinator.auth` may be a **string** (legacy), or an **object** with a **`type`** field:

| Form | Description |
|------|-------------|
| **`{"type":"internal"}`** (recommended) | Same bootstrap and defaults as the string `"internal"` (admin `admin`). On startup, if no `users` row exists for that admin username, the coordinator inserts it once. If **`admin_password_hash`** is omitted, a **random password** is generated and logged once at startup. Login checks **`users`** only. |
| **String** `"internal"` | Backward compatible; same semantics as `{"type":"internal"}`. |
| **`{"type":"ldap"}`** / **`{"type":"oauth"}`** | Declares preferred password-auth mode metadata; does **not** enable LDAP/OAuth by itself (`coordinator.ldap`, `oauth2_provider` still apply). No internal bootstrap. |
| **Omitted** (`coordinator` without `auth`) | Same as **`{"type":"internal"}`** (default admin bootstrap with random password when hash omitted). |
| **Object** without `type` (or empty `type`) | Same as **`{"type":"internal"}`**: internal bootstrap applies; unset `admin_user` defaults to `admin`; empty `admin_password_hash` triggers a one-time random password in logs. |

**First login (`type` internal or string `"internal"`):** username `admin`. Password is either from **`admin_password_hash`** (SHA-256 hex for `--reset-admin-password`, or bcrypt from the setup wizard), or the **random bootstrap password** printed in coordinator logs on first startup when no hash is configured.

**Reset admin password** — set `coordinator.auth.admin_password_hash` (and optional `admin_user`) in modular `homer.json` or via **`HOMER_COORDINATOR_AUTH_ADMIN_PASSWORD_HASH`**, then run:

```bash
homer --config-path /path/to/homer.json --reset-admin-password
```

The process opens **`coordinator.settings_db_path`**, ensures schema, updates or inserts the **`users`** row for `admin_user`, and **exits** (no HTTP server). Details, JSON examples, and env overrides: [AUTH_LDAP_AND_OAUTH.md](./AUTH_LDAP_AND_OAUTH.md#reset-admin-password).

**Password hashes:** Users created or updated via the API store **bcrypt** in `users.password_hash`. Login also accepts legacy **SHA-256 hex** (default bootstrap admin, migrated homer-app users). **`--reset-admin-password`** still expects **SHA-256 hex** in `admin_password_hash` (see [AUTH_LDAP_AND_OAUTH.md](./AUTH_LDAP_AND_OAUTH.md#reset-admin-password)).

**Generating a SHA-256 hex hash (for `admin_password_hash` / reset only):**

```bash
# Linux/macOS
echo -n "your-password" | sha256sum | cut -d' ' -f1

# Example: password "sipcapture" produces:
# 883ffc1f37fd0fe542b0fb9740035c4383e7d976c411161d24e62edace280f90
```

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `type` | string | *(see table above)* | `internal`, `ldap`, or `oauth`. If the whole `auth` section is omitted, or `type` is omitted / empty on the object, the effective type is **`internal`**. |
| `admin_user` | string | "admin" | Admin username when **`type`** is **`internal`** (including when `type` is omitted and normalized to internal). |
| `admin_password_hash` | string | "" | **SHA-256 hex** for bootstrap / **`--reset-admin-password`**, or **bcrypt** when set by the setup wizard. When empty on first bootstrap, the coordinator generates a random admin password and logs it once. API user password changes use bcrypt separately. |
| `fallback_auth_type` | string | "" | If set to **`internal`** or **`ldap`**, password login tries this backend **after** the client-selected `type` fails (wrong password or backend unavailable). Must not be **`oauth`**. Empty disables the second attempt. |
| `disable_password_login` | bool | false | If **`true`**, hide internal/LDAP from **`GET /auth/providers`** and return **403** on **`POST /auth/sessions`**. Use with OAuth2 for IdP-only login. Env: **`HOMER_COORDINATOR_AUTH_DISABLE_PASSWORD_LOGIN`**. |

### oauth2_provider

Single optional OAuth2 IdP object (`OAuthProviderConfig` in `src/config/config.go`). Uses the **OAuth 2.0 authorization code** flow on the coordinator (`redirect` → IdP → `callback` with `code` → token + userinfo → one-time browser token → `POST /auth/oauth2/token` for JWT).

| Parameter | Type | Description |
|-----------|------|-------------|
| `enable` | bool | Enable this provider |
| `name` | string | Stable id for URLs (`/api/v4/auth/oauth2/{name}/...`) |
| `type` | string | Usually `oauth2` |
| `provider_name` | string | Display name |
| `provider_image` | string | Provider logo path |
| `position` | int | Display order in discovery |
| `client_id` | string | OAuth2 client id |
| `client_secret` | string | Client secret (required when `use_pkce` is false) |
| `auth_url` | string | IdP authorization endpoint |
| `token_url` | string | IdP token endpoint |
| `redirect_url` | string | Registered redirect URI (must equal `https://<host>/api/v4/auth/oauth2/<name>/callback`) |
| `profile_url` | string | UserInfo / profile GET URL |
| `scopes` | string[] | Optional; default `openid`, `email` |
| `use_pkce` | bool | Public clients: set true; `client_secret` may be empty |
| `callback_url` | string | Browser redirect after callback with `?token=` or `?oauth_error=` |
| `skip_auto_provision` | bool | If true, user must pre-exist in DuckDB |
| `admin_groups` | string[] | Profile group claim values that grant admin JWT |
| `group_claim` | string | JSON claim for groups (default `groups`) |
| `auto_redirect` | bool | If true, UI may redirect immediately to this provider |

The deprecated **`oauth2_providers`** array is still accepted at startup and migrated with a log warning; prefer **`oauth2_provider`**.

## API Endpoints

### Authentication

| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/api/v4/auth/sessions` | Create session (login) |
| DELETE | `/api/v4/auth/sessions/:id` | Delete session (logout) |
| GET | `/api/v4/auth/providers` | List password backends (`internal`, `ldap`) and OAuth2 provider metadata |
| GET | `/api/v4/me` | Get current user info |

### Transactions (Search)

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/v4/transactions` | List transactions |
| POST | `/api/v4/transactions/search` | Search transactions |
| POST | `/api/v4/transactions/messages` | Get transaction messages (with optional Lua call-id correlation, see [`LUA_CORRELATION.md`](./LUA_CORRELATION.md)) |
| POST | `/api/v4/transactions/view/link` | Create a one-time SIP trace view URL (`data.url_view` → `GET /export/view/:uuid`) for external app redirects — see [Dashboard URL search — external apps](SEARCH_URL.md#external-apps) |
| GET | `/export/view/:uuid` | Standalone HTML SIP transaction view (no JWT; counts toward view token open limit) |
| GET | `/api/v4/messages/:id` | Get single message |
| GET | `/api/v4/messages/:id/decoded` | Get decoded message |
| POST | `/api/v4/transactions/qos` | Get QoS data |
| POST | `/api/v4/transactions/events` | Application log lines (proto 100) for session(s) |

### Dashboards

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/v4/dashboards` | List dashboards |
| POST | `/api/v4/dashboards` | Create dashboard |
| GET | `/api/v4/dashboards/:id` | Get dashboard |
| PUT | `/api/v4/dashboards/:id` | Update dashboard |
| DELETE | `/api/v4/dashboards/:id` | Delete dashboard |

### Users (Admin)

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/v4/users` | List users |
| POST | `/api/v4/users` | Create user |
| GET | `/api/v4/users/:id` | Get user |
| PATCH | `/api/v4/users/:id` | Update user |
| DELETE | `/api/v4/users/:id` | Delete user |

### Settings & Configuration

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/v4/mappings` | List field mappings |
| GET | `/api/v4/hepsubs` | List HEP subscriptions |
| GET | `/api/v4/aliases` | List aliases (auth) |
| GET | `/api/v4/aliases/lookup?ip=&port=&capture_id=` | LPM diagnostic lookup (auth) |
| POST | `/api/v4/aliases` | Create alias (**admin**) |
| PUT | `/api/v4/aliases/:aliasId` | Update alias (**admin**) |
| DELETE | `/api/v4/aliases/:aliasId` | Delete alias (**admin**) |
| GET | `/api/v4/db/nodes` | List configured nodes |
| GET | `/api/v4/modules` | Get modules status |

`/api/v4/ipaliases` is a mirror of the same handlers. The admin UI path is
**Settings → IP Aliases**. Full request/response schemas live in
`src/coordinator/docs/openapi.yaml` (tag `Aliases`).

Create body (required: `alias`, `ip`):

```json
{"alias":"SBC-A","ip":"10.0.0.1","port":0,"mask":32,"status":true}
```

Aliases are stored in the Coordinator settings DuckDB (`alias` table), not in
DuckLake. There is no startup file/env seed yet — for GitOps (Flux/k8s), apply
aliases with an admin JWT via `POST /api/v4/aliases` (for example from a Job
that curls the API). Homer 7 Postgres aliases can be imported with
`homer-core migrate settings`.

### Statistics

| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/api/v4/statistics/query` | Execute statistics query (`rawquery` validated; read-only SQL only) |
| GET | `/api/v4/statistics/databases` | List databases |
| GET | `/api/v4/statistics/measurements` | List measurements |
| GET | `/api/v4/statistics/metrics` | List metrics |

### Health & Status

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/health` | Health check |
| GET | `/api/v1/status` | System status with nodes info |

## Deployment Patterns

### Standalone (All-in-One)

Single server running Ingest, Storage, Node, and Coordinator:

```
┌───────────────────────────────────────────────────────────┐
│                       Homer Server                         │
│  ┌─────────┐ ┌─────────┐ ┌──────┐ ┌─────────────┐        │
│  │ Ingest  │ │ Storage │ │ Node │ │ Coordinator │        │
│  │ :9060   │ │         │ │:50051│ │   :8080     │        │
│  └────┬────┘ └────┬────┘ └───┬──┘ └──────┬──────┘        │
│       │           │          │           │                │
│       └───────────┴──────────┴───────────┘                │
│                         │                                  │
│                  ┌──────┴──────┐                          │
│                  │  DuckLake   │                          │
│                  └─────────────┘                          │
└───────────────────────────────────────────────────────────┘
```

### Distributed (Recommended for Production)

Separate instances for each module:

```
                       ┌─────────────────┐
                       │   Coordinator   │
                       │     :8080       │
                       └────────┬────────┘
                                │
           ┌────────────────────┼────────────────────┐
           ▼                    ▼                    ▼
    ┌──────────────┐    ┌──────────────┐    ┌──────────────┐
    │  Ingest 1    │    │  Ingest 2    │    │  Ingest 3    │
    │  Storage 1   │    │  Storage 2   │    │  Storage 3   │
    │  Node 1      │    │  Node 2      │    │  Node 3      │
    │  :50051      │    │  :50051      │    │  :50051      │
    └──────┬───────┘    └──────┬───────┘    └──────┬───────┘
           │                   │                   │
           ▼                   ▼                   ▼
    ┌─────────────┐     ┌─────────────┐     ┌─────────────┐
    │  DuckLake   │     │  DuckLake   │     │  DuckLake   │
    │  (Region 1) │     │  (Region 2) │     │  (Region 3) │
    └─────────────┘     └─────────────┘     └─────────────┘
```

### Read-Heavy Setup

Multiple Coordinators behind load balancer:

```
                    ┌─────────────────┐
                    │  Load Balancer  │
                    └────────┬────────┘
                             │
           ┌─────────────────┼─────────────────┐
           ▼                 ▼                 ▼
    ┌─────────────┐   ┌─────────────┐   ┌─────────────┐
    │ Coordinator │   │ Coordinator │   │ Coordinator │
    │    :8080    │   │    :8080    │   │    :8080    │
    └──────┬──────┘   └──────┬──────┘   └──────┬──────┘
           │                 │                 │
           └─────────────────┼─────────────────┘
                             │
                    ┌────────┴────────┐
                    │   Node Pool     │
                    │  (FlightSQL)    │
                    └─────────────────┘
```

## Settings Database

The Coordinator uses a DuckDB database for storing:

- **users** — accounts and credentials  
- **user_preferences** — generic per-user JSON blobs (`/api/v4/me/settings`)  
- **global_settings** — system-wide key/value rows (`/api/v4/advanced`)  
- **dashboard_settings** — saved dashboards  
- **user_mapping_settings** — per-user mapping widget overrides  
- **alias** — IP/host aliases for flow enrichment (`/api/v4/aliases`)  
- **correlation_scripts** — Lua correlation + script API rows  
- **dashboard_alerts**, **export_share_links**, **transaction_view_tokens** — supporting tables  

The database is created automatically at `settings_db_path`.

### Default schema

The canonical DDL is created in `EnsureSettingsSchema` in
`src/coordinator/services/settings_db.go` (DuckDB: BIGINT ids, JSON `data`, etc.).
Do not rely on older table names such as `user_settings` or `hep_scripts`.

## Example Configurations

See examples in the `examples/` directory:

- `homer-coordinator.json` - Coordinator only
- `homer-writer.json` - Ingest + Storage + Node
- `homer.json` - Ingest + Storage + Node + Coordinator (all-in-one)

## Security Considerations

1. **JWT secret** — Set `coordinator.jwt.secret` (or `HOMER_COORDINATOR_JWT_SECRET`) to a strong random value in production. If omitted, Homer persists **`/.homer_jwt_secret`** beside `settings_db_path` and always enforces JWT on protected routes ([SECURITY.md](./SECURITY.md)).
2. **Admin password** — Prefer bcrypt via Users API or wizard; explicit `admin_password_hash` (SHA-256 hex) for bootstrap and `--reset-admin-password`. No default cleartext password is injected when hash is omitted.
3. **Statistics SQL** — `POST /api/v4/statistics/query` validates `rawquery` (same rules as `/api/v4/query`).
4. **TLS** — Enable `use_tls` for node connections in production
5. **Network** — Restrict coordinator access via firewall/reverse proxy
6. **OAuth2** — Use HTTPS callback URLs in production
