# Migrating from Homer 10 with Grafana Loki–only history to Homer 11

> **⚠️ Experimental — not tested in production. Use at your own risk.**
>
> This note describes **operational expectations** when your historical SIP/HEP
> observability lived in **Grafana Loki** (for example **heplify-server → Loki**),
> not in PostgreSQL `hep_proto_*` tables. There is **no built-in** Homer 11
> command to rebuild DuckLake transaction/search state from Loki log lines.
> Validate retention, agent cutover, and user workflows on a **non-production**
> stack before switching traffic.

This document is **only about data that already sits in Loki**. If you also run
classic [homer-app](https://github.com/sipcapture/homer-app) with PostgreSQL
and want to migrate settings or replay `hep_proto_*` rows, use
[MIGRATION_HOMER7.md](MIGRATION_HOMER7.md) — it is the authoritative guide for
that path.

## What “Homer 10 + Loki” means here

Typical pattern:

- Capture agents (for example **heplify** / **heplify-server**) send HEP to a
  pipeline that **indexes SIP as logs** in Loki.
- Operators search with **LogQL**, Grafana dashboards, or similar — not via the
  Homer 11 **DuckLake** API (`/api/v4/transactions/search`, etc.).

In Homer 11 the primary storage for capture-backed search is **DuckLake**
(Parquet + catalog); ingest receives **HEP** over UDP/TCP/HTTP as documented in
the [README](../../README.md).

## Target state (Homer 11)

| Concern | Homer 11 behaviour |
|---|---|
| New capture | HEP → **ingest** → DuckLake ([architecture](../../README.md#architecture)) |
| Search / UI | Coordinator REST and UI query DuckLake-backed data |
| Optional Loki | After cutover you can **duplicate** accepted HEP into Loki again via **`remote_logging.loki`** (see [Configuration snippet](#optional-duplicate-hep-to-loki-after-cutover)) |

## Loki → DuckLake: what Homer ships

| Question | Answer |
|---|---|
| Is there a `homer migrate loki` (or similar)? | **No.** Nothing in this repo reads Loki and materializes the same columnar/SIP timeline DuckLake expects. |
| Why? | Loki holds **log streams and labels**, not the same row layout as `hep_proto_*` replay or live HEP ingestion. Reconstruction would be lossy and deployment-specific. |
| What happens to old Loki data? | It **stays in Loki** for as long as your retention rules allow. Homer 11 does not automatically import it into DuckLake. |
| When does Homer 11 gain searchable SIP history? | From the moment agents send **HEP to Homer 11 ingest** onward — a clear **cutover time** is useful for operators and users. |

Implementation reference for optional Loki push from the writer path:
[`src/remotelog/remotelog.go`](../../src/remotelog/remotelog.go) (shared types;
configured under root-level `remote_logging` in [`src/config/config.go`](../../src/config/config.go)).

## Recommended operational order

1. **Deploy Homer 11** using your normal procedure ([README](../../README.md),
   storage sizing [STORAGE_POLICIES.md](../STORAGE_POLICIES.md) /
   [STORAGE_ARCHITECTURE.md](../STORAGE_ARCHITECTURE.md) as needed).

2. **Point capture agents at Homer 11 ingest** — match `ingest.udp` /
   `ingest.tcp` / `ingest.http` ports in your `homer` JSON (defaults often **9060 /
   9061 / 9080** in examples).

3. **Record a cutover timestamp**: before that time, authoritative history for
   “log-style” investigations remains **LogQL / Grafana on Loki**; after that,
   new data is queryable through **Homer 11** APIs and UI ([SEARCH.md](../SEARCH.md)).

4. **Keep Loki running** if you still need historical LogQL access; align
   **retention** with compliance needs. Homer 11 **TTL / compaction** applies
   only to DuckLake data, not to your Loki cluster.

5. **Optional**: enable `remote_logging.loki` so post-cutover HEP is also pushed
   to Loki (see below).

### Optional: duplicate HEP to Loki after cutover

Top-level `remote_logging` in your Homer config (shape mirrors
[`examples/homer-writer.json`](../../examples/homer-writer.json); additional
fields such as `bulk`, `timer`, `hep_filter` exist — see
[`LokiLogConfig`](../../src/config/config.go)):

```json
"remote_logging": {
  "loki": {
    "enable": true,
    "url": "http://your-loki:3100/loki/api/v1/push"
  }
}
```

This **does not backfill** old Loki volumes; it only affects packets processed
after you enable it.

## If you must reuse legacy Loki content alongside Homer 11

Any approach here is **custom and unsupported** by this repository: for example
scheduled **LogQL** / Loki API export into another store, ETL jobs, or keeping
two UIs (Grafana for the past, Homer 11 for the present). Expect differences in
field sets, correlation, and payload fidelity versus native HEP ingestion.

## Caveats

- **Search semantics**: LogQL text search over log lines is not identical to
  Homer 11 transaction/search APIs over DuckLake — train users on the new UI or
  provide bookmarks for both tools during transition.
- **Dashboards**: Grafana boards built on Loki do not migrate into Homer
  dashboards; rebuild where needed.
- **Retention**: Independent policies on Loki vs DuckLake can confuse “how far
  back can I search?” — document both clearly per environment.

## Related docs

- [MIGRATION_HOMER7.md](MIGRATION_HOMER7.md) — PostgreSQL homer-app → Homer 11
  (`migrate settings`, `migrate hep`)
- [SEARCH.md](../SEARCH.md)
- [STORAGE_POLICIES.md](../STORAGE_POLICIES.md)
