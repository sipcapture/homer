# Security hardening (11.0.282+)

Homer 11.0.282–11.0.284 closes three coordinator security issues. **11.0.313** hardens the node HTTP `/query` API and blocks DuckDB external scanners. This page summarizes **operator impact** and how upgrades stay compatible with existing installs.

## Summary

| Issue | Risk (before) | Fix | Typical upgrade impact |
|-------|----------------|-----|-------------------------|
| Empty `coordinator.jwt.secret` | Protected API routes were **unauthenticated** | JWT middleware always enforced; empty secret → auto-generated persisted secret | Installs with explicit JWT env/config: **no change**. Installs with empty secret: API now requires login or `Auth-Token` |
| Default admin `sipcapture` | Predictable first-login password | No compose hash; random bcrypt at first boot; JWT `must_change_password` until a new password is set | Existing `users` row with sipcapture hash: **login works**, then the UI/API require a password change |
| `POST /api/v4/statistics/query` `rawquery` | Unvalidated SQL passthrough | Same `ValidateRawSQL` rules as `/api/v4/query` | Legitimate `SELECT` / `WITH` / `SHOW` queries: **no change** |
| Node `POST /query` (port+1) | Unauthenticated arbitrary DuckDB SQL / file read | `ValidateRawSQL` always; Bearer auth when `flight_server.auth_token` set; auto-token on non-loopback bind | All-in-one with default `0.0.0.0`: token auto-persisted; coordinator local node gets `token` automatically |
| `sqlite_scan` / external scanners | Authenticated bypass of SQL denylist | Blocked in `ValidateRawSQL` | Legitimate lake `SELECT`s: **no change** |

GitHub Security Advisories: [GHSA-rqcc-94gv-wjm9](https://github.com/sipcapture/homer/security/advisories/GHSA-rqcc-94gv-wjm9), [GHSA-6xp5-7rcx-xfgx](https://github.com/sipcapture/homer/security/advisories/GHSA-6xp5-7rcx-xfgx), [GHSA-f46q-3v67-fmm4](https://github.com/sipcapture/homer/security/advisories/GHSA-f46q-3v67-fmm4), [GHSA-rm5w-rqr7-2h54](https://github.com/sipcapture/homer/security/advisories/GHSA-rm5w-rqr7-2h54), [GHSA-4687-q698-mccv](https://github.com/sipcapture/homer/security/advisories/GHSA-4687-q698-mccv), [GHSA-263f-5xrw-c34r](https://github.com/sipcapture/homer/security/advisories/GHSA-263f-5xrw-c34r).

---

## JWT secret (`coordinator.jwt.secret`)

**Before:** When the signing secret was empty, JWT middleware was not registered and handlers skipped validation — every protected route was open.

**After:**

1. **Non-empty secret in config or env** — unchanged (recommended for production).
2. **Empty secret** — on coordinator startup Homer generates a random secret, writes **`/.homer_jwt_secret`** beside `coordinator.settings_db_path` (mode `0600`), and reuses it on restart so sessions stay valid.
3. Protected routes under `/api/v1`, `/api/v3`, and `/api/v4` **always** require authentication (JWT cookie/Bearer, or valid **`Auth-Token`** when `api_settings.enable_token_access` is true).

```json
"coordinator": {
  "settings_db_path": "/var/lib/homer/homer_settings.duckdb",
  "jwt": {
    "secret": "use-a-long-random-string-at-least-32-bytes",
    "expire_hours": 24
  }
}
```

Environment: `HOMER_COORDINATOR_JWT_SECRET`.

**Docker Compose** (`examples/docker/docker-compose.yaml`) already sets `HOMER_COORDINATOR_JWT_SECRET` — no action required.

---

## Bootstrap admin password (`coordinator.auth`)

**Before:** Internal auth normalization injected a fixed SHA-256 hash for cleartext password **`sipcapture`**.

**After:**

| Config | First startup (no `users` row) | Existing `users` row |
|--------|--------------------------------|----------------------|
| Explicit `admin_password_hash` or env | Uses configured hash | Unchanged if row already has a password |
| Empty hash | Random password generated; **logged once** at startup | Unchanged if row already has a password |
| Docker examples | Hash omitted; random password in coordinator logs | Existing sipcapture hash in `users`: login then **forced password change** |

Legacy SHA-256 rows from **migrated homer-app users** still authenticate. The well-known **`sipcapture`** digest still logs in, but the session is limited to `GET/PATCH /api/v4/me` and logout until the password is changed. `--reset-admin-password` still refuses that digest as a *new* hash.

**Wizard:** empty admin password field → random password (bcrypt in JSON) shown once after save.

**Recovery:** `homer --config-path … --reset-admin-password` still requires explicit `admin_password_hash` (SHA-256 hex) in config or env.

See [AUTH_LDAP_AND_OAUTH.md](./AUTH_LDAP_AND_OAUTH.md#internal-duckdb-authentication) and [COORDINATOR.md](./COORDINATOR.md#auth).

---

## Statistics SQL (`POST /api/v4/statistics/query`)

**Before:** `rawquery` in the request body was passed to FlightSQL without validation.

**After:** Each `rawquery` is validated with the same rules as `POST /api/v4/query`:

- Allowed statement starts: `SELECT`, `WITH`, `SHOW`, `DESCRIBE`, `EXPLAIN`, `PRAGMA`
- No semicolons (multi-statement blocked)
- Blocked DML/DDL keywords and dangerous functions (token-aware: keywords inside string literals / Call-IDs are ignored; real `CALL` / `DELETE` identifiers remain blocked)
- External scanners blocked: `sqlite_scan`, `postgres_scan`, `mysql_scan`, `iceberg_scan`, `delta_scan`, `spatial_scan` (11.0.313+)

Invalid SQL returns **400** with `SQL validation failed`. Grafana-style read-only panels using single `SELECT` statements continue to work.

See [LINE_PROTOCOL.md](./LINE_PROTOCOL.md) (Statistics API section).

---

## Node HTTP `/query` (11.0.313+)

**Before:** The node HTTP API on `flight_server.port + 1` (default **50052**) accepted `POST /query` with **no authentication** and **no SQL validation**, so an unauthenticated client could run arbitrary DuckDB SQL (including `read_text('/etc/passwd')` and full lake dumps).

**After:**

1. Every `/query` body is validated with `ValidateRawSQL` (same rules as coordinator raw SQL).
2. When `node.flight_server.auth_token` is non-empty, `/query` and `/vacuum` require `Authorization: Bearer <token>`. `/health` stays open for probes.
3. If `auth_token` is empty and `flight_server.host` is **not** loopback (default `0.0.0.0`), Homer generates a random token, persists **`.homer_node_auth_token`** beside the DuckLake catalog (mode `0600`), and enables Bearer auth. Loopback-only binds (`127.0.0.1` / `::1` / `localhost`) may keep an empty token.
4. The coordinator sends `coordinator.nodes[].token` as Bearer on `/query`. Same-process local nodes with empty `token` inherit `node.flight_server.auth_token` automatically.

```json
"node": {
  "flight_server": {
    "host": "127.0.0.1",
    "port": 50051,
    "auth_token": "change-me-node-http"
  }
},
"coordinator": {
  "nodes": [
    {
      "name": "local",
      "host": "127.0.0.1",
      "port": 50051,
      "token": "change-me-node-http"
    }
  ]
}
```

**Recommendation:** bind `flight_server.host` to **`127.0.0.1`** when the node is only used by a co-located coordinator, and set an explicit `auth_token` / `nodes[].token` in production. Firewall port+1 from untrusted networks.

The live HEP WebSocket `/stream` on the same HTTP port is still intended for private networks (see [HEP_STREAM.md](./HEP_STREAM.md)); prefer not exposing it publicly.

---

## Upgrade checklist

1. **Review JWT** — If you never set `coordinator.jwt.secret`, after upgrade check coordinator logs for `jwt_secret_file` or set an explicit secret in config.
2. **Review admin access** — If you relied on open API without JWT, enable `Auth-Token` tokens or use UI login.
3. **Docker / explicit env** — `HOMER_COORDINATOR_JWT_SECRET` + `HOMER_COORDINATOR_AUTH_ADMIN_PASSWORD_HASH` (as in `examples/docker/`) → no credential changes.
4. **Automation** — Scripts calling protected endpoints need `Authorization: Bearer …` or `Auth-Token` after upgrade if JWT was previously empty.
5. **Node `/query`** — After 11.0.313, check logs for `auth_token_file` / Bearer requirement; set matching `coordinator.nodes[].token` for remote nodes; prefer loopback bind for local-only nodes.

---

## See also

- [COORDINATOR.md](./COORDINATOR.md) — `jwt`, `auth` parameters
- [NODE.md](./NODE.md) — `flight_server.auth_token`, HTTP `/query`
- [UI_COORDINATOR_AUTH_AND_TOKENS.md](./UI_COORDINATOR_AUTH_AND_TOKENS.md) — login, cookies, API tokens
- [WIZARD.md](./WIZARD.md) — generated credentials
- [AUTH_LDAP_AND_OAUTH.md](./AUTH_LDAP_AND_OAUTH.md) — `--reset-admin-password`
