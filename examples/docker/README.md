# Homer + RustFS (Compose)

This folder contains two Docker Compose variants for running Homer with RustFS-backed storage.

- `docker-compose.yaml` — Homer with **local Parquet hot storage** plus **RustFS S3 cold storage**.
- `docker-compose_s3direct.yaml` — Homer with **S3-only storage** on RustFS.

All Homer configuration is in the `homer` service `environment` map (`HOMER_*`). RustFS settings are configured under `rustfs.environment`. Naming rules: [`docs/ENVIRONMENT_VARIABLES.md`](../../docs/ENVIRONMENT_VARIABLES.md). Tiering concepts: [`docs/STORAGE_POLICIES.md`](../../docs/STORAGE_POLICIES.md).

## Storage layouts

### `docker-compose.yaml`

| Tier | Where | Role |
|------|--------|------|
| **Hot** | Local `/data/homer/parquet` (`homer_data` volume) | New writes and recent partitions. |
| **Cold** | `s3://homer-data-cold` on RustFS (`http://rustfs:9000`) | Older partitions moved by Homer tiering policies. |

### `docker-compose_s3direct.yaml`

| Tier | Where | Role |
|------|--------|------|
| **Cold only** | `s3://homer-data-cold` on RustFS (`http://rustfs:9000`) | All Homer data is written directly to S3-compatible storage. |

## Quick start

```bash
cd examples/docker
docker compose -f docker-compose.yaml up -d
```

Or use the S3-only stack:

```bash
cd examples/docker
docker compose -f docker-compose_s3direct.yaml up -d
```

Edit the chosen compose file to change ports, image (`ghcr.io/sipcapture/homer:latest`), secrets, or storage settings—no separate `.env` file is required.

## Memory (DuckDB)

The image bakes in writer defaults (`HOMER_STORAGE_DUCKLAKE_TUNING_*`):
`MEMORY_LIMIT=4GB`, `THREADS=2`, spill under `/data/homer/.duckdb_spill`.
Compose repeats the same variables so they are visible and easy to
override without rebuilding. The all-in-one container needs **~8GB RAM**.

## Non-root runtime

The image runs as uid/gid **1000** (`USER homer`). Named volumes from older
root images need a one-shot chown; both compose files include `homer-init`
for that. Homer listens on ports >1024, so the compose services use
`cap_drop: ALL` and `no-new-privileges`.

```bash
# docker run
docker run -e HOMER_STORAGE_DUCKLAKE_TUNING_MEMORY_LIMIT=8GB ghcr.io/sipcapture/homer:latest

# compose: edit homer.environment in docker-compose.yaml
```

Raise `MEMORY_LIMIT` toward 50% of the container budget under SIPREC /
high ingest. Details: [OOM.md](../../docs/OOM.md), [DUCKDB_TUNING.md](../../docs/DUCKDB_TUNING.md).

## First login

Omit `HOMER_COORDINATOR_AUTH_ADMIN_PASSWORD_HASH`. On first start the coordinator
creates `admin` with a random bcrypt password and logs it once. Existing installs
that still have the historical `sipcapture` hash can sign in, then must set a
new password in the UI before using the rest of the API.

## Endpoints

- **Coordinator UI:** http://localhost:8080
- **HEP / HTTP ingest:** UDP/TCP `9060`, HTTP `9080`
- **RustFS S3 API:** http://localhost:9000 (`rustfs:9000` inside the stack)
- **RustFS console:** http://localhost:9001
- **Prometheus:** http://localhost:9090/metrics

RustFS keys in `rustfs.environment` must match the Homer S3 access key/secret entries. The default credentials are `rustfsadmin` / `rustfsadmin`.

## References

- Env naming: [`docs/ENVIRONMENT_VARIABLES.md`](../../docs/ENVIRONMENT_VARIABLES.md)
- Tiered storage: [`docs/STORAGE_POLICIES.md`](../../docs/STORAGE_POLICIES.md)
