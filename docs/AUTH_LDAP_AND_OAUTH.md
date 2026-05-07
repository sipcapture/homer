# External authentication: LDAP and OAuth2 (Coordinator)

This guide describes how to enable **LDAP / Active Directory** password login and **OAuth2** redirects in the homer-core **coordinator** API v4, how the **web UI** discovers providers, and which JSON keys to set in configuration.

For JWT lifecycle and logout, see the notes at the end.

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

- **`internal`** — local users in the coordinator settings DuckDB (`users` table) plus optional bootstrap **`coordinator.auth.admin_user`** / **`admin_password_hash`**.
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

### Enable in configuration

Under **`coordinator.oauth2_provider`**, set one object shaped like `OAuthProviderConfig`:

| Field | Description |
|--------|-------------|
| `enable` | If `true`, provider appears in `/auth/providers` and redirect works. |
| `name` | **Stable id** used in URLs: `/api/v4/auth/oauth2/{name}/redirect`. Must match what the UI sends. |
| `position` | Display ordering in discovery responses (single provider). |
| `type` | Usually `oauth2`. |
| `provider_name` / `provider_image` | Optional display hints for clients. |
| `url` | Full **authorization** URL of the IdP (user is redirected here). |
| `callback_url` | Where the coordinator redirects after **`/auth/oauth2/{name}/callback`** with `?token=...`. For the bundled SPA, this is typically the Homer URL with path/hash so the app can read `token` and call **`POST /auth/oauth2/token`**. |
| `auto_redirect` | If `true`, the UI may redirect immediately to this provider (same idea as homer-app). |

### Example (generic IdP)

```json
{
  "coordinator": {
    "oauth2_provider": {
      "enable": true,
      "name": "keycloak",
      "position": 10,
      "type": "oauth2",
      "provider_name": "Keycloak",
      "url": "https://idp.example.org/realms/homer/protocol/openid-connect/auth?client_id=homer-ui&response_type=code&scope=openid&redirect_uri=https%3A%2F%2Fhomer.example.org%2Fapi%2Fv4%2Fauth%2Foauth2%2Fkeycloak%2Fcallback",
      "auto_redirect": false,
      "callback_url": "https://homer.example.org/?oauth=1"
    }
  }
}
```

Adjust **`redirect_uri`** inside `url` to point at the coordinator callback route:

`/api/v4/auth/oauth2/{name}/callback`

The IdP must accept that callback URL.

### OAuth endpoints (v4)

| Method | Path | Role |
|--------|------|------|
| `GET` | `/api/v4/auth/providers` | Lists `internal`, `ldap`, and **at most one** object in `oauth2`. |
| `POST` | `/api/v4/auth/sessions` | Username/password (`type`: `internal` or `ldap`). |
| `GET` | `/api/v4/auth/oauth2/{provider}/redirect` | HTTP redirect to provider `url`. |
| `GET` | `/api/v4/auth/oauth2/{provider}/callback` | Validates provider flow, issues **one-time** token, redirects to `callback_url` or `redirect_uri` with `token=`. |
| `POST` | `/api/v4/auth/oauth2/token` | Body `{"token":"<one-time>"}` → JWT. |

### One-time tokens

- The callback stores a **short-lived** one-time token in memory and redirects the browser with `?token=...`.
- The UI exchanges it once via **`POST /api/v4/auth/oauth2/token`**.
- Multi-instance deployments need a **shared** store for one-time tokens (current implementation is in-process).

---

## JWT sessions and logout

- Session id is JWT **`jti`**.
- `DELETE /api/v4/auth/sessions/{sessionId}` revokes the current `jti` for the remaining token lifetime (in-memory store).

---

## See also

- OpenAPI: `src/coordinator/docs/openapi.yaml` — `LoginRequest`, `AuthProvidersResponse`, OAuth routes.
- Legacy OAuth-only notes: `docs/AUTH_OAUTH.md` (short); this file is the **canonical** LDAP + OAuth guide.
