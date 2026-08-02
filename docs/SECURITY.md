# Security hardening (11.0.282+)

Homer 11.0.282–11.0.284 closes three coordinator security issues. This page summarizes **operator impact** and how upgrades stay compatible with existing installs.

## Summary

| Issue | Risk (before) | Fix | Typical upgrade impact |
|-------|----------------|-----|-------------------------|
| Empty `coordinator.jwt.secret` | Protected API routes were **unauthenticated** | JWT middleware always enforced; empty secret → auto-generated persisted secret | Installs with explicit JWT env/config: **no change**. Installs with empty secret: API now requires login or `Auth-Token` |
| Default admin `sipcapture` | Predictable first-login password | No auto-filled hash; random bootstrap password logged once when hash omitted | Docker examples with explicit `ADMIN_PASSWORD_HASH`: **no change**. Existing `users` row with old hash: **login still works** |
| `POST /api/v4/statistics/query` `rawquery` | Unvalidated SQL passthrough | Same `ValidateRawSQL` rules as `/api/v4/query` | Legitimate `SELECT` / `WITH` / `SHOW` queries: **no change** |

GitHub Security Advisories: [GHSA-rqcc-94gv-wjm9](https://github.com/sipcapture/homer/security/advisories/GHSA-rqcc-94gv-wjm9), [GHSA-6xp5-7rcx-xfgx](https://github.com/sipcapture/homer/security/advisories/GHSA-6xp5-7rcx-xfgx), [GHSA-f46q-3v67-fmm4](https://github.com/sipcapture/homer/security/advisories/GHSA-f46q-3v67-fmm4).

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
| Docker example env with sipcapture hash | Inserts configured hash | Login `admin` / `sipcapture` still works |

Legacy SHA-256 rows (including migrated homer-app users and explicit sipcapture hash in env) **still authenticate** at login.

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

Invalid SQL returns **400** with `SQL validation failed`. Grafana-style read-only panels using single `SELECT` statements continue to work.

See [LINE_PROTOCOL.md](./LINE_PROTOCOL.md) (Statistics API section).

---

## Upgrade checklist

1. **Review JWT** — If you never set `coordinator.jwt.secret`, after upgrade check coordinator logs for `jwt_secret_file` or set an explicit secret in config.
2. **Review admin access** — If you relied on open API without JWT, enable `Auth-Token` tokens or use UI login.
3. **Docker / explicit env** — `HOMER_COORDINATOR_JWT_SECRET` + `HOMER_COORDINATOR_AUTH_ADMIN_PASSWORD_HASH` (as in `examples/docker/`) → no credential changes.
4. **Automation** — Scripts calling protected endpoints need `Authorization: Bearer …` or `Auth-Token` after upgrade if JWT was previously empty.

---

## See also

- [COORDINATOR.md](./COORDINATOR.md) — `jwt`, `auth` parameters
- [UI_COORDINATOR_AUTH_AND_TOKENS.md](./UI_COORDINATOR_AUTH_AND_TOKENS.md) — login, cookies, API tokens
- [WIZARD.md](./WIZARD.md) — generated credentials
- [AUTH_LDAP_AND_OAUTH.md](./AUTH_LDAP_AND_OAUTH.md) — `--reset-admin-password`
