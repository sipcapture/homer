# Authentication: internal, LDAP, and OAuth2 (Coordinator)

This guide describes **local (internal) authentication** in the coordinator settings DuckDB, how to enable **LDAP / Active Directory** password login and **OAuth2** redirects in the homer-core **coordinator** API v4, how the **web UI** discovers providers, and which JSON keys to set in configuration.

For JWT lifecycle and logout, see the notes at the end.

---

## Internal DuckDB authentication

> **Implementation:** homer-core accepts **`coordinator.auth`** as a JSON **string** (`"internal"`, backward compatible), as **`{"type":"internal"}`** (recommended), or as an **object** without `type` (normalized to **`internal`**) with optional **`admin_user`** / **`admin_password_hash`**. **`--reset-admin-password`** is documented below.

Password login with **`type`** omitted or **`internal`** on `POST /api/v4/auth/sessions` checks credentials **only** against rows in the coordinator **settings DuckDB** table **`users`** (`coordinator.settings_db_path`). There is **no** separate “login from JSON only” path: a user must exist in `users` with a matching `password_hash` for the presented password.

**Password hash formats in `users.password_hash`:**

| Format | When used | Verification |
|--------|-----------|--------------|
| **bcrypt** (`$2a$…`, `$2b$…`, `$2y$…`) | New or updated passwords via Users API / `PATCH /me` | `bcrypt.CompareHashAndPassword` |
| **SHA-256 hex** (64 chars) | Explicit bootstrap hash in config, migrated homer-app rows, `--reset-admin-password` | Legacy compare (hex digest of password) |

The coordinator accepts **either** format at login. Prefer letting the API create bcrypt hashes for new users instead of hand-editing SHA-256 in DuckDB.

If **`coordinator.auth` is omitted** entirely (no `auth` key), the loader behaves like **`{"type":"internal"}`**: default admin name **`admin`**, internal bootstrap on startup when no `users` row exists. If **`admin_password_hash`** is also omitted, the coordinator generates a **random** bootstrap password and logs it once (see [SECURITY.md](./SECURITY.md#bootstrap-admin-password-coordinatorauth)).

### Password login fallback (`fallback_auth_type`)

Optional **`coordinator.auth.fallback_auth_type`**: **`internal`** or **`ldap`**. On **`POST /api/v4/auth/sessions`** (and legacy v3 login), if authentication with the JSON **`type`** field fails, the coordinator tries **`fallback_auth_type`** once when it differs from the first attempt. Example: UI uses LDAP first; set **`"fallback_auth_type": "internal"`** so local **`users`** can still sign in when LDAP rejects the password or LDAP is temporarily unavailable.

### Recommended: `coordinator.auth` object with `type`

```json
"coordinator": {
  "auth": { "type": "internal" }
}
```

Optional explicit admin (otherwise defaults apply):

```json
"auth": {
  "type": "internal",
  "admin_user": "admin",
  "admin_password_hash": "<64-char hex sha256>"
}
```

Other **`type`** values (no internal bootstrap; LDAP/OAuth still follow their own sections):

```json
"auth": { "type": "ldap" }
```

```json
"auth": { "type": "oauth" }
```

### Backward compatible: `coordinator.auth` as the string `"internal"`

```json
"coordinator": {
  "auth": "internal"
}
```

Meaning (string or `{"type":"internal"}` without custom hash):

- The loader treats **`"internal"`** (case-insensitive) or **`{"type":"internal"}`** as **built-in local auth** with default admin name **`admin`**.
- On coordinator startup, if there is **no** row in `users` whose **`username`** equals the configured admin name (default `admin`), the coordinator **inserts** that admin once (`is_admin` / `is_active` true). If **`admin_password_hash`** is set (config or env), that hash is used; if empty, a **random password** is generated, stored as bcrypt, and logged once at startup.
- If a row for that username **already** exists with a non-empty `password_hash`, bootstrap does not change the password (upgrade-safe). Empty `password_hash` on an existing row may be repaired from config hash or a new random password when hash is omitted.
- After first login, operators should **change the password** (Settings → Users, or `PATCH /me`). Updates store a **bcrypt** hash. Further logins use the stored hash only.

**Docker / explicit bootstrap:** `examples/docker/docker-compose.yaml` sets `HOMER_COORDINATOR_AUTH_ADMIN_PASSWORD_HASH` to the SHA-256 hex of **`sipcapture`** — that path is unchanged and still yields first login **`admin` / `sipcapture`** on fresh installs using those examples.

### Legacy object: only `admin_user` / `admin_password_hash` (no `type`)

You may still use an object **without** `type` (for environment overrides, config fragments, or tooling):

```json
"auth": {
  "admin_user": "admin",
  "admin_password_hash": "<64-char lower-case hex sha256>"
}
```

- If **`admin_user`** is unset or **`admin`** and **`admin_password_hash`** is empty, the loader treats this as **`internal`** with random bootstrap password when no `users` row exists. If **`admin_password_hash`** is set (including the legacy sipcapture SHA-256 hex from docker examples), bootstrap uses that hash.
- For **`--reset-admin-password`**, the config field **`admin_password_hash`** remains **SHA-256 hex** (see [Reset admin password](#reset-admin-password)). User records created or updated through the API use **bcrypt** instead.

### Reset admin password

The homer-core CLI flag **`--reset-admin-password`** (together with **`--config-path`**) applies **`coordinator.auth.admin_password_hash`** to the **`users`** table in **`coordinator.settings_db_path`**.

**Requirements**

1. **Modular homer-core** with **`--config-path`** pointing at your `homer.json` (or equivalent). The flag is **mandatory**; without it the command exits with an error.
2. **`coordinator.settings_db_path`** must be non-empty in the loaded config — it must be the **same** settings DuckDB file the coordinator uses.
3. After `config.Load`, the command reads **`coordinator.auth.admin_password_hash`** and **`coordinator.auth.admin_user`** (if `admin_user` is empty, **`admin`** is used). **`admin_password_hash` must be non-empty** in the effective config or the command fails.

**Hash generation:** `echo -n 'your-new-password' | sha256sum` (64 lowercase hex chars).

**Example `coordinator.auth` fragment (recommended object with `type`):**

```json
"coordinator": {
  "settings_db_path": "/var/lib/homer/homer_settings.duckdb",
  "auth": {
    "type": "internal",
    "admin_user": "admin",
    "admin_password_hash": "PASTE_64_CHAR_HEX_SHA256_HERE"
  }
}
```

If you set **`"auth": "internal"`** or **`{"type":"internal"}`** **without** an explicit `admin_password_hash`, **`--reset-admin-password` fails** because `admin_password_hash` must be non-empty in the effective config. Set the hash in JSON or `HOMER_COORDINATOR_AUTH_ADMIN_PASSWORD_HASH` before running the command.

**Environment overrides:** you can set **`HOMER_COORDINATOR_AUTH_ADMIN_PASSWORD_HASH`** (and optionally **`HOMER_COORDINATOR_AUTH_ADMIN_USER`**) instead of embedding the hash in JSON — see `config/env.go` / `env_test` in this repo.

**Run (does not start the HTTP server; exits after updating DuckDB):**

```bash
homer --config-path /etc/homer/homer.json --reset-admin-password
```

On success the process prints a short message to stdout confirming the update for the configured username.

### LDAP / OAuth2 interaction

- **LDAP** is independent: it does not read `users` for password verification. Internal and LDAP can both be enabled; the UI sends `type` accordingly.
- **OAuth2** is orthogonal to password backends (see below).

---

## Discovery API (used by the login UI)

`GET /api/v4/auth/providers` (no auth) returns:

```json
{
  "data": {
    "internal": {
      "enable": true,
      "name": "Internal",
      "position": 0,
      "type": "internal"
    },
    "ldap": {
      "enable": false,
      "name": "LDAP",
      "position": 1,
      "type": "ldap"
    },
    "oauth2": [
      {
        "enable": true,
        "name": "keycloak",
        "position": 10,
        "type": "oauth2",
        "provider_name": "Keycloak",
        "provider_image": "",
        "url": "https://idp.example/realms/homer/protocol/openid-connect/auth?...",
        "auto_redirect": false
      }
    ]
  },
  "meta": {}
}
```

- **`internal`** — local users in the coordinator settings DuckDB (`users` table). Prefer **`coordinator.auth`** as **`{"type":"internal"}`** (or omitted `auth` / string **`"internal"`**) for first-time bootstrap (see [Internal DuckDB authentication](#internal-duckdb-authentication)); explicit **`admin_user`** / **`admin_password_hash`** in JSON or env supports **`--reset-admin-password`**.
- **`ldap.enable`** is `true` only when **`coordinator.ldap.enable`** is true **and** **`coordinator.ldap.host`** is non-empty.
- **`oauth2`** — **zero or one** entry: the single active OAuth2 provider (see below).

The UI shows a **password method** dropdown when more than one of `internal` / `ldap` is enabled. If OAuth is configured, **at most one** “Continue with …” button appears below “or”.

---

## LDAP password login

### Enable in configuration

Under the **`coordinator`** object, set **`ldap`** (see `LDAPConfig` in `src/config/config.go`):

| Field | Description |
|--------|-------------|
| `enable` | Master switch. |
| `host` | LDAP host(s). Multiple hosts: space-separated list (failover). **Required** for LDAP to be advertised and accepted. |
| `port` | Default `389`. |
| `use_ssl` | LDAPS (`true`) vs plain LDAP + optional StartTLS. |
| `skip_tls` | If `false` and not `use_ssl`, issue **StartTLS** after connect. |
| `insecure_skip_verify` | TLS client skips server cert verification (dev only). |
| `server_name` | TLS SNI / cert verification name. |
| `bind_dn` / `bind_password` | Service account for **search** before user bind (optional if `anonymous` mode). |
| `base` | Search base DN for users and groups. |
| `user_filter` | LDAP filter with **`%s`** replaced by the escaped username, default `(uid=%s)`. |
| `attributes` | Attributes to read on the user entry (defaults applied if empty). |
| `anonymous` | If `true`, skip search bind path; use **`user_dn`** template only. |
| `user_dn` | Format string with **`%s`** for username when `anonymous` is true (e.g. `uid=%s,ou=people,dc=example,dc=com`). |
| `admin_group` / `user_group` | Group DNs/CNs used after successful bind to decide **admin** vs normal user. |
| `admin_mode` / `user_mode` | Behaviour when group lookup fails or is empty (similar idea to homer-app `ldap_config`). |
| `group_filter` | Filter with **`%s`** for group search (default `(memberUid=%s)`). |
| `group_attributes` | LDAP attributes on group entries (default `["memberOf"]` if empty). |
| `use_dn_for_group_search` | Use user DN instead of login name in `group_filter`. |

### Minimal example (search + user bind)

```json
{
  "coordinator": {
    "ldap": {
      "enable": true,
      "host": "ldap.example.org",
      "port": 389,
      "skip_tls": true,
      "insecure_skip_verify": true,
      "bind_dn": "cn=readonly,dc=example,dc=org",
      "bind_password": "CHANGE_ME",
      "base": "ou=people,dc=example,dc=org",
      "user_filter": "(uid=%s)",
      "admin_group": "cn=homer-admins,ou=groups,dc=example,dc=org",
      "user_group": "cn=homer-users,ou=groups,dc=example,dc=org"
    }
  }
}
```

After restart, the login page should offer **LDAP** in the authentication dropdown (together with **Internal**). The browser sends:

```http
POST /api/v4/auth/sessions
Content-Type: application/json

{"username":"alice","password":"secret","type":"ldap"}
```

`type` may be omitted; it defaults to **`internal`**.

### Operational notes

- LDAP users receive a JWT like local users; they are **not** automatically inserted into DuckDB `users` unless you add separate provisioning.
- If `enable` is true but `host` is empty, the coordinator logs a warning and LDAP stays **disabled**.

---

## OAuth2

**Only one OAuth2 provider is supported.** Set it as a single object under **`coordinator.oauth2_provider`**. The deprecated array key **`coordinator.oauth2_providers`** is still read at startup for backward compatibility: the first usable enabled row (lowest `position`, then `name`) is migrated into memory and a **warning** is logged.

The coordinator implements the **OAuth 2.0 authorization code** flow on the server: **`GET .../redirect`** builds the IdP authorize URL (with **CSRF `state`** and optional **PKCE**), the IdP returns **`code`** to **`GET .../callback`**, the coordinator exchanges **`code`** for tokens, calls **`profile_url`**, maps the user to DuckDB **`users`**, then redirects the browser to **`callback_url`** with a **one-time `token`** (same as before). The SPA exchanges that token for a JWT via **`POST /api/v4/auth/oauth2/token`**.

Legacy **`url`**-only configuration (browser redirected to a static IdP URL without server-side code exchange) is **no longer supported**.

### Enable in configuration

Under **`coordinator.oauth2_provider`**, set one object. Required for the code flow (all non-empty unless noted):

| Field | Description |
|--------|-------------|
| `enable` | If `true`, provider appears in `/auth/providers` and redirect works when the row is complete. |
| `name` | **Stable id** used in URLs: `/api/v4/auth/oauth2/{name}/redirect` and `/callback`. |
| `position` | Display ordering in discovery responses. |
| `type` | Usually `oauth2`. |
| `provider_name` / `provider_image` | Optional display hints for clients. |
| `client_id` | OAuth2 client id registered at the IdP. |
| `client_secret` | Client secret. **Required** when `use_pkce` is `false` (confidential client). |
| `auth_url` | IdP authorization endpoint (e.g. OIDC `/authorize`). |
| `token_url` | IdP token endpoint (e.g. OIDC `/token`). |
| `redirect_url` | **Must** match the registered redirect URI at the IdP — typically `https://<homer-host>/api/v4/auth/oauth2/<name>/callback`. |
| `profile_url` | UserInfo endpoint (GET with access token), e.g. OIDC `.../userinfo`. |
| `scopes` | Optional; default if omitted: `["openid", "email"]`. |
| `use_pkce` | If `true`, use PKCE (public clients may omit `client_secret`). |
| `callback_url` | Where the coordinator redirects after success with `?token=<one-time>` (bundled UI: Homer origin + optional `?oauth=1`). On failure, `?oauth_error=...` is set instead. **Required for production** when using absolute URLs (see [Redirect URL safety](#redirect-url-safety)). |
| `skip_auto_provision` | If `true`, the IdP user must already exist in DuckDB (matched by `username` or `email`); otherwise login fails. |
| `admin_groups` | If non-empty, membership in any of these group **names** (see `group_claim`) grants **admin** for the JWT session. |
| `group_claim` | JSON claim on the profile response listing groups (default `groups`). |

### Example (Keycloak / generic OIDC)

```json
{
  "coordinator": {
    "oauth2_provider": {
      "enable": true,
      "name": "keycloak",
      "position": 10,
      "type": "oauth2",
      "provider_name": "Keycloak",
      "client_id": "homer-ui",
      "client_secret": "REPLACE_ME",
      "auth_url": "https://idp.example.org/realms/homer/protocol/openid-connect/auth",
      "token_url": "https://idp.example.org/realms/homer/protocol/openid-connect/token",
      "redirect_url": "https://homer.example.org/api/v4/auth/oauth2/keycloak/callback",
      "profile_url": "https://idp.example.org/realms/homer/protocol/openid-connect/userinfo",
      "scopes": ["openid", "email", "profile"],
      "use_pkce": false,
      "callback_url": "https://homer.example.org/",
      "auto_redirect": false
    }
  }
}
```

Register **`redirect_url`** exactly at the IdP. The **`auth_url`** / **`token_url`** / **`profile_url`** must match your issuer.

### Redirect URL safety

After OAuth callback, the coordinator redirects the browser to **`callback_url`** (configured on the provider) or, if that is empty, to the query parameter **`redirect_uri`**.

| Rule | Behaviour |
|------|-----------|
| **`callback_url` set** | That URL is used. Optional `redirect_uri` must be a **relative path** on the same site (e.g. `/login`), not an external absolute URL. |
| **`callback_url` empty** | Only **relative** `redirect_uri` values are allowed (must start with `/`, not `//`). |
| **Absolute URL without `callback_url`** | Rejected (**400**) — prevents open redirects to attacker-controlled hosts. |
| **Absolute `callback_url`** | Allowed; `redirect_uri` query must match the **same origin** (scheme + host) if both are sent. |

Configure a fixed **`callback_url`** (e.g. `https://homer.example.org/`) in production. Do not rely on user-supplied absolute `redirect_uri` values.

**Docker / environment variables:** Scalar fields under `coordinator.oauth2_provider` map to `HOMER_COORDINATOR_OAUTH2_PROVIDER_<FIELD>` with the field name uppercased (Viper + `HOMER_` prefix), for example `HOMER_COORDINATOR_OAUTH2_PROVIDER_CLIENT_ID`, `HOMER_COORDINATOR_OAUTH2_PROVIDER_AUTH_URL`, `HOMER_COORDINATOR_OAUTH2_PROVIDER_TOKEN_URL`, `HOMER_COORDINATOR_OAUTH2_PROVIDER_REDIRECT_URL`, `HOMER_COORDINATOR_OAUTH2_PROVIDER_PROFILE_URL`, `HOMER_COORDINATOR_OAUTH2_PROVIDER_CALLBACK_URL`, `HOMER_COORDINATOR_OAUTH2_PROVIDER_CLIENT_SECRET`, `HOMER_COORDINATOR_OAUTH2_PROVIDER_USE_PKCE`, `HOMER_COORDINATOR_OAUTH2_PROVIDER_ENABLE`, `HOMER_COORDINATOR_OAUTH2_PROVIDER_NAME`, `HOMER_COORDINATOR_OAUTH2_PROVIDER_PROVIDER_NAME`, `HOMER_COORDINATOR_OAUTH2_PROVIDER_AUTO_REDIRECT`. Slice fields such as **`scopes`** and **`admin_groups`** are easier to set in JSON than as flat env vars; see `src/config/env.go` and `src/config/env_test.go` for how `HOMER_*` overrides are merged.

### OAuth endpoints (v4)

| Method | Path | Role |
|--------|------|------|
| `GET` | `/api/v4/auth/providers` | Lists `internal`, `ldap`, and **at most one** object in `oauth2`. |
| `POST` | `/api/v4/auth/sessions` | Username/password (`type`: `internal` or `ldap`). |
| `GET` | `/api/v4/auth/oauth2/{provider}/redirect` | Builds authorize URL (state + optional PKCE), redirects browser to IdP. |
| `GET` | `/api/v4/auth/oauth2/{provider}/callback` | Validates `state`, exchanges **`code`**, loads profile, provisions user, issues **one-time** token, redirects to `callback_url`. |
| `POST` | `/api/v4/auth/oauth2/token` | Body `{"token":"<one-time>"}` → JWT. |

### One-time tokens and OAuth state

- **OAuth `state`** and optional **PKCE verifiers** are stored **in-process** (short TTL). Multiple coordinator replicas behind a load balancer require a shared store for reliable logins (same limitation as one-time session tokens).
- The callback stores a **short-lived** one-time token and redirects the browser with `?token=...`.
- The UI exchanges it once via **`POST /api/v4/auth/oauth2/token`**.

### User mapping and provisioning

- **Username** for JWT: `preferred_username` if present; else email with `@` replaced by `_`; else `oidc-<sub>`.
- Existing users are matched by **username** first, then by **email**.
- If no user exists and **`skip_auto_provision`** is `false` (default), a new DuckDB user is created with a random password hash (OAuth-only; password login is not intended for that row).
- **`admin_groups`** matching uses the **`group_claim`** list from userinfo. Values may be plain strings or objects with a **`name`** (or **`group_name`**) field; configure Authentik scope mappings so the claim you read actually contains the same strings as in **`admin_groups`** (including spaces). If login works but admin does not, inspect the raw userinfo JSON first. With **`log.level`** set to **`debug`**, the coordinator logs one line **`oauth2 admin group check`** (claim key, whether the claim exists, parsed groups, configured `admin_groups`, and `admin_match`) after each successful profile fetch when `admin_groups` is non-empty.

### OAuth-only login (hide username/password)

Set **`coordinator.auth.disable_password_login`** to **`true`** to hide internal and LDAP password methods from **`GET /auth/providers`** and reject **`POST /auth/sessions`** with **403**. The bundled UI then shows only OAuth controls.

Scripts and CLI cannot use username/password in this mode. For IP alias CRUD and other admin APIs, create an **admin Auth-Token** (Settings → Auth tokens) and send **`Auth-Token: <secret>`** — see [UI_COORDINATOR_AUTH_AND_TOKENS.md — OAuth-only CLI](./UI_COORDINATOR_AUTH_AND_TOKENS.md#oauth-only-cli--scripts-without-password-login).

Typical Azure / Entra ID setup:

```json
{
  "coordinator": {
    "auth": {
      "disable_password_login": true
    },
    "oauth2_provider": {
      "enable": true,
      "name": "azure",
      "auto_redirect": true,
      "client_id": "YOUR_APP_ID",
      "client_secret": "YOUR_SECRET",
      "auth_url": "https://login.microsoftonline.com/{tenant}/oauth2/v2.0/authorize",
      "token_url": "https://login.microsoftonline.com/{tenant}/oauth2/v2.0/token",
      "redirect_url": "https://homer.example.com/api/v4/auth/oauth2/azure/callback",
      "profile_url": "https://graph.microsoft.com/oidc/userinfo",
      "callback_url": "https://homer.example.com/"
    }
  }
}
```

Environment variables:

- `HOMER_COORDINATOR_AUTH_DISABLE_PASSWORD_LOGIN=true`
- `HOMER_COORDINATOR_OAUTH2_PROVIDER_AUTO_REDIRECT=true`

Full sample: **`examples/homer-coordinator-oauth2-azure.sample.json`**.

### SSO profile photos (Microsoft Entra / OIDC)

The UI must not call IdP photo URLs (for example `https://graph.microsoft.com/v1.0/me/photo/$value`) directly — the browser has no access token. Homer 11 instead:

1. On OAuth callback, the coordinator fetches the photo with the IdP **access token** and caches it in memory (24h TTL per username).
2. **`GET /api/v4/me`** returns **`avatar: "/me/avatar"`** when a cached photo exists.
3. **`GET /api/v4/me/avatar`** (JWT required) streams the image bytes.
4. The bundled UI loads the photo via authenticated `fetch` and shows **initials** when no photo is available.

For Azure, include **`User.Read`** in **`scopes`** (see sample JSON) so Graph can return the profile photo.

**`auto_redirect`** alone only redirects the browser to the IdP on page load; it does not disable password login. Use **`disable_password_login`** when password login must be blocked.

### Pre-provisioned OAuth users (`skip_auto_provision`)

When **`skip_auto_provision`** is **`true`**, OAuth login succeeds only if the IdP user already exists in DuckDB (matched by **username** first, then **email**). No new row is created on login.

1. Sign in once as admin (before enabling OAuth-only, or via break-glass config).
2. Create users in **Settings → Users**, or via **`POST /api/v4/users`**.
3. Set **`skip_auto_provision: true`** on **`oauth2_provider`** and restart.

Docker:

```yaml
HOMER_COORDINATOR_OAUTH2_PROVIDER_SKIP_AUTO_PROVISION: "true"
```

If the user is missing, the callback redirects with **`?oauth_error=no matching user and skip_auto_provision is true`**.

Combined Azure-only + no auto-provision:

```json
{
  "coordinator": {
    "auth": { "disable_password_login": true },
    "oauth2_provider": {
      "enable": true,
      "name": "azure",
      "auto_redirect": true,
      "skip_auto_provision": true
    }
  }
}
```

---

## JWT sessions and logout

- **`coordinator.jwt.secret`** must be non-empty for signing tokens. If omitted in config, Homer auto-generates a secret and persists **`/.homer_jwt_secret`** beside `settings_db_path` (see [SECURITY.md](./SECURITY.md#jwt-secret-coordinatorjwtsecret)). Protected API routes always require authentication.
- Session id is JWT **`jti`**.
- `DELETE /api/v4/auth/sessions/{sessionId}` revokes the current `jti` for the remaining token lifetime (in-memory store).

---

## See also

- OpenAPI: `src/coordinator/docs/openapi.yaml` — `LoginRequest`, `AuthProvidersResponse`, OAuth routes.
- UI and tokens overview: [UI_COORDINATOR_AUTH_AND_TOKENS.md](./UI_COORDINATOR_AUTH_AND_TOKENS.md).
- Legacy OAuth-only pointer: [AUTH_OAUTH.md](./AUTH_OAUTH.md); this file is the **canonical** internal + LDAP + OAuth guide.
