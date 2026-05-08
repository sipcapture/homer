# UI and coordinator authentication

This document explains which authentication modes the Homer **coordinator** exposes to the **web UI** and API clients, how operators switch between **local (internal)**, **LDAP**, and **OAuth2**, and how **tokens** (JWT sessions vs static API tokens) are used.

For full LDAP and OAuth2 configuration field tables and examples, see **[AUTH_LDAP_AND_OAUTH.md](./AUTH_LDAP_AND_OAUTH.md)** (canonical). OpenAPI details live in `src/coordinator/docs/openapi.yaml`.

---

## Architecture

- The **coordinator** is the authority: it serves `GET /api/v4/auth/providers`, creates sessions, validates requests, and issues JWTs.
- The **bundled UI** (`src/ui`) calls the v4 API under `import.meta.env.VITE_API_BASE` or the default **`/api/v4`**. It loads provider metadata when the user is logged out and stores a session JWT in `localStorage` under the key `homer_v4_token`.

There is no separate “auth mode” flag in the UI alone: available methods are **entirely determined by coordinator configuration** after restart.

---

## Authentication types

| Mode | Coordinator | Password / flow | Typical `POST /api/v4/auth/sessions` body |
|------|----------------|-----------------|---------------------------------------------|
| **Local (internal)** | Always advertised as enabled | Users in the coordinator **settings DuckDB** (`users` table), plus optional bootstrap **`coordinator.auth.admin_user`** / **`admin_password_hash`**. | `{"username":"…","password":"…"}` or `"type":"internal"` (default). |
| **LDAP** | Advertised only if **`coordinator.ldap.enable`** is true **and** **`coordinator.ldap.host`** is non-empty | Directory bind + optional group rules for admin vs user. | `{"username":"…","password":"…","type":"ldap"}`. |
| **OAuth2** | Optional; **at most one** provider from **`coordinator.oauth2_provider`** | Browser redirect to IdP `url`, callback to coordinator, then token exchange (see below). | No password session; use OAuth routes. |

Notes:

- **Internal** and **LDAP** are both “password backends”. The API accepts `type` of **`internal`** or **`ldap`** only; anything else is `400`.
- **OAuth2** is orthogonal: it does not use `type` on `POST /auth/sessions`; it uses **`GET /api/v4/auth/oauth2/{provider}/redirect`**, **`GET /api/v4/auth/oauth2/{provider}/callback`**, and **`POST /api/v4/auth/oauth2/token`**.

---

## Discovery: what the UI shows

`GET /api/v4/auth/providers` (no authentication) returns `data.internal`, `data.ldap`, and `data.oauth2` (array with **zero or one** enabled row).

The UI (`src/ui/src/loginProviders.ts`, `LoginPage.tsx`):

1. Builds the list of **enabled password methods** from `internal` and `ldap` rows (`enable: true`).
2. If **more than one** password method is enabled, it shows an **“Authentication”** dropdown so the user picks **internal** vs **ldap**; the selected value is sent as JSON **`type`** on `POST /api/v4/auth/sessions`.
3. If only one password method is enabled, **`type`** is forced to that method’s `type` (no dropdown).
4. For OAuth2, it shows a **“Continue with …”** control per enabled provider name and navigates the browser to  
   `{apiBase}/auth/oauth2/{name}/redirect`.
5. If the provider has **`auto_redirect: true`**, the UI immediately starts that redirect once on the login page.

**Switching between local and LDAP for end users:** use the login dropdown when both are enabled.

**Switching at deployment level (operators):**

- **Local only:** leave LDAP disabled or omit host; remove or disable `oauth2_provider`.
- **Local + LDAP:** set `coordinator.ldap` with `enable: true` and a non-empty `host` (see canonical doc).
- **Add OAuth2:** set `coordinator.oauth2_provider` with `enable: true`, stable `name`, `url`, `callback_url`, etc.; restart coordinator.
- **Force OAuth-first UX:** set `auto_redirect: true` on the single OAuth provider (users still need internal/LDAP if you keep password methods for break-glass accounts).

---

## OAuth2 flow and JWT

1. **`GET /api/v4/auth/oauth2/{provider}/redirect`** — HTTP 302 to the IdP authorization URL configured in `oauth2_provider.url`.
2. **`GET /api/v4/auth/oauth2/{provider}/callback`** — Coordinator issues a **short-lived one-time token**, stores it server-side, and redirects the browser to **`callback_url`** (or `redirect_uri` query param) with **`?token=<one-time>`**.
3. **`POST /api/v4/auth/oauth2/token`** with body `{"token":"<one-time>"}` — Consumes the one-time token once and returns **`data.token`** as the **JWT** session (same shape as password login).

API clients and SPAs should **exchange** the query `token` for the JWT and then send **`Authorization: Bearer <jwt>`** on subsequent calls. One-time tokens are **not** JWTs and will not validate as Bearer credentials.

**Multi-instance warning:** one-time OAuth tokens are stored in-process; multiple coordinator replicas need a shared store for this flow to be reliable (see canonical doc).

---

## JWT session tokens

- **Login response:** `POST /api/v4/auth/sessions` (password) and `POST /api/v4/auth/oauth2/token` (after exchange) return `data.token` (JWT) and `data.scope` / `data.user.admin`.
- **Usage:** `Authorization: Bearer <jwt>` on protected v4 routes. The UI builds this in `App.tsx` as `authHeader`.
- **Lifetime:** `coordinator.jwt.expire_hours` (default 24) and `coordinator.jwt.secret`.
- **Logout / revocation:** JWT **`jti`** is the session id. **`DELETE /api/v4/auth/sessions/{sessionId}`** revokes that session for the remainder of the token lifetime (in-memory revocation store). The bundled UI’s logout control currently clears the client token only; integrations that need server-side invalidation should call this DELETE with the **`jti`** from the JWT claims.

---

## Static API tokens (`Auth-Token`)

Separate from JWT login, the coordinator can accept **static tokens** stored in the settings DuckDB **`auth_token`** table when API token access is enabled.

**Coordinator configuration** (`coordinator.api_settings`):

| Field | Purpose |
|--------|---------|
| **`enable_token_access`** | If `true`, the JWT middleware first checks the configured header for a raw token matching an `auth_token` row. |
| **`auth_token_header`** | Header name (default **`Auth-Token`**). |

When a valid row is found, the request is treated as authenticated with a **synthetic** user derived from the row’s **`user_object`** JSON: **`username`** and admin if **`usergroup`** equals **`admin`** (case-insensitive) — see `authenticateWithAuthTokenHeader` in `src/coordinator/handlers/auth.go`.

**How to use:**

1. Enable `enable_token_access` and restart.
2. Create a token (admin UI **Settings → Auth tokens** or the v4 CRUD API under `/api/v4/auth-tokens`).
3. Call APIs with **`Auth-Token: <secret>`** (or your custom header name) **instead of** `Authorization: Bearer …`, or rely on middleware order: if token access is enabled, that header is tried before Bearer.

Tokens support optional **expiry**, **call limits**, and **active** flag; the server increments usage on successful lookups.

---

## Summary table

| Goal | Mechanism |
|------|-----------|
| Local users | DuckDB `users` + `POST /auth/sessions` with `type` omitted or `internal`. |
| LDAP users | Configure LDAP; `POST /auth/sessions` with `"type":"ldap"`. |
| OAuth2 users | Configure single `oauth2_provider`; redirect + **`POST /auth/oauth2/token`**. |
| Scripts / integrations without JWT | Enable `api_settings.enable_token_access`; send **`Auth-Token`**. |
| Revoke JWT session | `DELETE /api/v4/auth/sessions/{jti}`. |

---

## See also

- **[AUTH_LDAP_AND_OAUTH.md](./AUTH_LDAP_AND_OAUTH.md)** — LDAP and OAuth2 configuration, examples, operational notes.
- **[AUTH_OAUTH.md](./AUTH_OAUTH.md)** — pointer to the canonical LDAP+OAuth doc.
- `src/coordinator/handlers/auth.go`, `auth_v4.go` — session creation, JWT middleware, Auth-Token path.
- `src/ui/src/LoginPage.tsx`, `loginProviders.ts` — UI discovery and password `type` behaviour.
