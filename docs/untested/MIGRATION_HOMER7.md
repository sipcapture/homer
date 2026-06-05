# Migrating from Homer 7 (homer-app) to Homer 11 (homer-core)

> **⚠️ Experimental — not tested in production. Use at your own risk.**
>
> The `migrate` subcommand has not been validated on real production
> Homer 7 deployments yet. There may be edge cases (large datasets,
> custom schemas, non-standard auth setups, exotic HEP payloads) that
> are not handled correctly. Always run it against a **copy** of your
> homer-app database first, keep backups, and verify the imported
> data before pointing users at the new install.

`homer-core migrate` is a built-in subcommand that copies an existing
[homer-app](https://github.com/sipcapture/homer-app) deployment into a fresh
homer-core install. It has two independent actions:

| Action | What it does | Source | Target |
|---|---|---|---|
| `settings` | Copies users, ACLs, aliases, auth tokens, HEPsub mappings, agent sessions, optional field-mapping schemas and dashboard metadata | PostgreSQL `homer_config` database | DuckDB settings file (`coordinator.settings_db_path`) |
| `hep`      | Replays raw HEP/SIP/RTCP/LOG rows back into a running homer-core node as standard HEP3 packets, preserving original timestamps, capture id and correlation id | PostgreSQL `homer_data` database (tables `hep_proto_*`) | DuckLake (Parquet on object storage), via the node's HEP listener |

Both actions are **idempotent** and **resumable** — they UPSERT by natural key
(guid / username / token) and the HEP replay persists a checkpoint, so you
can re-run after an interruption without re-emitting duplicates.

## Why two actions?

The settings tables in homer-app are small (KB-MB) and have schemas almost
identical to homer-core's — so we can copy them with straight INSERTs.

The HEP data tables (`hep_proto_*`) are huge and have completely different
storage in v11 (Parquet on object storage instead of partitioned PG tables).
The cheapest correct way to bring this data over is to **replay** the rows as
HEP3 packets back into the node, so they go through the normal ingestion
pipeline (alias enrichment, field mapping, correlation, DuckLake writer) and
end up indistinguishable from data the node captured itself.

## Quick start

```bash
# 1. Bring up homer-core normally (it will create an empty settings DuckDB
#    on first start). Then stop it for the settings migration:
sudo systemctl stop homer-core

# 2. Migrate settings.
sudo -u homer ./homer-core migrate settings \
  --pg-dsn "postgres://homer_user:secret@homer7-db:5432/homer_config?sslmode=disable" \
  --config-path /etc/homer/homer-core_config.json \
  --verbose

# 3. Start homer-core back up — users/aliases/tokens are now usable, login
#    with existing homer 7 passwords works.
sudo systemctl start homer-core

# 4. Migrate HEP data while homer-core is running.
./homer-core migrate hep \
  --pg-dsn "postgres://homer_user:secret@homer7-db:5432/homer_data?sslmode=disable" \
  --hep-target 127.0.0.1:9060 \
  --hep-proto udp \
  --tables hep_proto_1_call,hep_proto_1_default,hep_proto_1_registration,hep_proto_5_default,hep_proto_100_default \
  --since 2024-01-01T00:00:00Z \
  --until 2025-01-01T00:00:00Z \
  --rate 5000 \
  --checkpoint /var/lib/homer/migrate.checkpoint.json
```

That's it. After both steps, `homer-core` exposes the same accounts, aliases
and HEP timeline you had on Homer 7, queryable through the v11 UI / API.

## `migrate settings`

```
homer-core migrate settings [flags]
```

| Flag | Default | Description |
|---|---|---|
| `--pg-dsn` | _required_ | PostgreSQL DSN for the homer-app `homer_config` database |
| `--config-path` | _empty_ | Path to homer-core config (used to locate the DuckDB settings file) |
| `--duckdb` | _from config_ | Override the destination DuckDB file path |
| `--owner-default` | `admin` | Username used when a homer-app row lacks one |
| `--with-mappings` | `false` | Also migrate `mapping_schema` rows. Off by default — homer-core seeds its own, and v7 fields_mapping shape differs |
| `--with-dashboards` | `true` | Migrate `dashboard_settings` metadata only; widget bodies are wiped (see below) |
| `--skip-users` | `false` | Skip the `users` table (useful when SSO/LDAP is the source of truth) |
| `--dry-run` | `false` | Don't write anything; just count |
| `--verbose` | `false` | Log every row |

### What is migrated, exactly

| homer-app PG table | homer-core DuckDB table | Notes |
|---|---|---|
| `users` | `users` | Preserves bcrypt `password` → `password_hash`. Existing logins keep working. |
| `global_settings` | `global_settings` | 1:1. |
| `user_settings` | `user_preferences` | Renamed in v11; same schema. |
| `dashboard_settings` | `dashboard_settings` | Metadata only — see "Dashboards" below. |
| `mapping_schema` | `mapping_schema` | Only with `--with-mappings`. |
| `hepsub_mapping_schema` | `hepsub_mapping_schema` | 1:1. HEPsub queries continue to work. |
| `alias` | `alias` | 1:1; v11-only columns (`custom_image`, `tag1..4`) stay NULL. |
| `auth_token` | `auth_token` | 1:1. Existing API tokens keep working. |
| `agent_location_session` | `agent_location_session` | 1:1. |

### Dashboards

The dashboard widget tree changed completely between v7 and v11
(`widgetRegistry` is incompatible). The migrator copies dashboard
**metadata** (id, owner, partid, name, type, weight, shared) so that users
can see and re-open their dashboards in v11, but it stores the v7 widget
body verbatim under `data._homer7_legacy` and resets the widget tree to
empty.

Re-create the widget layouts manually in the v11 UI; the legacy JSON is
preserved as a recovery aid.

To skip migrating dashboards entirely, pass `--with-dashboards=false`.

### Idempotency

The migration UPSERTs by natural key:

- `users` → `username`
- everything else → `guid`

Re-running the migration is safe; existing rows in the destination are not
overwritten (the migrator skips them and counts them as `skipped`).

## `migrate hep`

```
homer-core migrate hep [flags]
```

| Flag | Default | Description |
|---|---|---|
| `--pg-dsn` | _required_ | PostgreSQL DSN for the homer-app `homer_data` database |
| `--hep-target` | `127.0.0.1:9060` | host:port of the homer-core node's HEP listener |
| `--hep-proto` | `udp` | Transport: `udp` or `tcp` |
| `--hep-capture-id` | `0` (preserve) | Override the capture agent id on every emitted packet (chunk 0x000c). 0 = use the value stored in `protocol_header.captureId` |
| `--tables` | `hep_proto_1_call,hep_proto_1_default,hep_proto_1_registration,hep_proto_5_default,hep_proto_100_default` | Comma-separated source tables |
| `--since` | _none_ | Only replay rows with `create_date >= TIME` (RFC3339) |
| `--until` | _none_ | Only replay rows with `create_date <  TIME` (RFC3339) |
| `--batch-size` | `5000` | Rows fetched per SELECT batch |
| `--rate` | `5000` | Throttle to N packets/sec (0 = unlimited; can saturate the node) |
| `--limit` | `0` | Stop after N total rows (0 = no limit) |
| `--checkpoint` | `homer7-migrate.checkpoint.json` | Resume file: `{table → max_id_processed}` |
| `--dry-run` | `false` | Reconstruct packets but don't send them; useful to validate the source |

### How replay works

For each row in `hep_proto_*` we read:

- `id` (BIGINT) — used for resume checkpoint
- `sid` (VARCHAR) — Call-ID, used as a fallback for HEP correlation chunk (0x0011)
- `create_date` (TIMESTAMP) — used for HEP timestamp chunks (0x0009, 0x000a) when not present in `protocol_header`
- `protocol_header` (JSONB) — source of family / proto / src/dst IP+port / payloadType / capture id / node / capture pass / correlation id / mos
- `raw` (TEXT) — the actual SIP / RTCP / LOG payload

Then we assemble a standard HEP3 datagram (`HEP3` magic + length + chunks)
and send it to the target via UDP or TCP.

The reconstructor produces the following chunks (all under vendor 0x0000):

```
0x0001 IP family   0x0002 protocol id   0x0003/0004 src/dst IP   0x0007/0008 src/dst port
0x0009/000a time sec/usec   0x000b payload type   0x000c capture agent id
0x000e capture pass (if present)   0x000f payload   0x0011 correlation id
0x0013 node name (if present)
```

This is exactly the layout that heplify and other production capture agents
emit, so the receiving node treats it identically.

### Resume / fault tolerance

After every batch the migrator writes the highest replayed `id` per table to
the checkpoint file (atomic rename). On the next run the migrator will
`WHERE id > <checkpoint>`, so an interrupted job resumes exactly where it
left off — no duplicates, no gaps.

### Throughput tips

- Start with `--rate 5000` (default) and watch `du -sh /var/lib/homer/lake/`
  grow. Increase by 2× until the node CPU pegs.
- For the largest installs, drop `--rate` to `0` and run a separate replay
  per table in parallel; HEP UDP fan-in scales linearly with cores.
- Replays that take longer than your retention window will lose tail data
  due to homer-core's TTL — chunk the time range with `--since` / `--until`
  if needed.

## Share / drill-down URLs (homer-ui → Homer 11)

Homer 7 homer-ui links often look like:

```text
http://homer:9080/search/result?{"timestamp":{"from":...,"to":...},"param":{"search":{"1_call":{"callid":["..."]}}}}=
```

In Homer 11:

| Homer 7 | Homer 11 equivalent |
|---------|---------------------|
| `/search/result?{json}=` on UI port | `https://<coordinator>/?call_id=...&from=...&to=...#dashboard` (user logged in) |
| homer-app search/share API (POST) | `POST /api/v4/transactions/view/link` + redirect to `/export/view/<uuid>` |

Full examples and API-token flows: [Dashboard URL search — external applications](../SEARCH_URL.md#external-apps).

## Caveats

- **Custom widget configurations** in v7 dashboards do not transfer
  automatically; only metadata + a `_homer7_legacy` recovery payload do.
- **Custom field mappings** (`mapping_schema.fields_mapping`) have a
  slightly different shape in v11; we only migrate them on explicit
  `--with-mappings` and you may need to fix them up manually.
- **PCAP/raw payload retention**: if `hep_proto_*.raw` in v7 was truncated
  (e.g. some installs strip large bodies), the migrated row will be
  truncated too — the migrator does not invent data.
- **Time travel / DuckLake snapshots**: replayed packets are written using
  the current wall-clock partition layout, but with the original HEP
  timestamp inside the row. Search by time works as expected; system
  metadata (snapshot ids, file mtimes) reflects the migration date.

## Source layout

The migrator lives in [`src/cli`](../src/cli):

- [`migrate_cmd.go`](../src/cli/migrate_cmd.go) — flags & dispatcher
- [`migrate_settings.go`](../src/cli/migrate_settings.go) — settings tables
- [`migrate_hep.go`](../src/cli/migrate_hep.go) — HEP3 reconstruction & replay
- [`migrate_hep_test.go`](../src/cli/migrate_hep_test.go) — round-trip tests
