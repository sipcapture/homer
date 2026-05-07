# Homer v4 UI

Small React UI for the v4 API (Auth, Me, Users). The build output is written to
`src/dist` and served by the existing `homer-core` static server.

## Requirements

- Node.js 18+
- Running `homer-core` in coordinator mode

## Environment variables

- `VITE_API_BASE` — API base path, defaults to `/api/v4`.
- `VITE_API_TARGET` — backend for Vite proxy in dev mode, defaults to
  `http://127.0.0.1:9080`.

## Dev mode (no CORS)

1. Start the backend (`homer-core`) on the desired port, e.g. `9080`.
2. Start the UI:

```bash
cd /home/shurik/Projects/homer-core/src/ui
VITE_API_TARGET="http://127.0.0.1:9080" npm install
VITE_API_BASE="/api/v4" npm run dev
```

The UI will be available at `http://127.0.0.1:5173`, and all `/api/*` requests
are proxied to the backend.

## Production build (`src/dist`)

```bash
cd /home/shurik/Projects/homer-core/src/ui
npm install
npm run build
```

After the build, files in `src/dist` are updated: `index.html` and assets. These
files are served directly by `homer-core`.

## Check

- Open `/` on `homer-core` and sign in via `/api/v4/auth/sessions`.
- Verify the `Me` view and loading the `Users` list.

## Implemented feature coverage

- Dashboard widgets: search, proto search, smart input, results, charts, iframe/grafana, alert, pcap uploader, code editor, clock, note.
- Settings pages: profile, about, users, user settings, aliases (IP aliases / CIDR), advanced, mappings, hepsubs, auth tokens, agent subscriptions, dashboards, scripts, import, statistics, loki, grafana, system, reset, API docs.
- Transaction details tabs: messages, flow, qos, logs, call info, events, hepsub.
- Role-based settings navigation/action visibility: `admin`, `commonUser`, `external`.

## Tests

```bash
cd /home/shurik/Projects/homer-core/src/ui
npm run test
```

Current smoke/integration coverage:

- App login flow (`src/App.test.jsx`)
- Role-based settings sidebar visibility (`src/settings/SettingsSidebar.test.jsx`)

## Transactions storage fields

Transaction search responses include storage origin metadata when DuckLake is
configured with tiered volumes:

- `storage_lake` — DuckLake lake name (e.g. `homer_lake_hot`)
- `storage_volume` — volume name (e.g. `hot`, `cold`)
