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
