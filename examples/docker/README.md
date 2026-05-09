# Homer + RustFS (Compose)

Docker Compose stack for **Homer** with **RustFS** as S3-compatible **cold** storage.

**All Homer configuration is in [`docker-compose.yml`](docker-compose.yml)** under the `homer` service `environment` map (`HOMER_*`). RustFS settings are under `rustfs.environment`. Naming rules: [`docs/ENVIRONMENT_VARIABLES.md`](../../docs/ENVIRONMENT_VARIABLES.md). Tiering concepts: [`docs/STORAGE_POLICIES.md`](../../docs/STORAGE_POLICIES.md).

## Storage layout

| Tier | Where | Role |
|------|--------|------|
| **Hot** | Local `/data/homer/parquet` (`homer_data` volume) | New writes. |
| **Cold** | `s3://homer-cold/data/` on RustFS (`http://rustfs:9000`) | Older partitions per `HOMER_STORAGE_DUCKLAKE_STORAGE_POLICY_*`. |

## Quick start

```bash
cd examples/docker
docker compose up -d
```

Edit **`docker-compose.yml`** to change ports, image (`ghcr.io/sipcapture/homer:latest`), secrets, or tiering knobs—no separate `.env` file required.

- **Coordinator UI:** http://localhost:8080  
- **HEP / HTTP ingest:** UDP/TCP `9060`, HTTP `9080`  
- **RustFS S3 API:** http://localhost:9000 (`rustfs:9000` in the stack)  
- **RustFS console:** http://localhost:9001  
- **Prometheus:** http://localhost:9090/metrics  

RustFS keys in `rustfs.environment` must match the Homer cold-tier `HOMER_*_S3_ACCESS_KEY_ID` / `HOMER_*_S3_SECRET_ACCESS_KEY` entries (default `rustfsadmin`). Create bucket **`homer-cold`** if cold-tier writes fail until it exists.

## References

- Env naming: [`docs/ENVIRONMENT_VARIABLES.md`](../../docs/ENVIRONMENT_VARIABLES.md)  
- Tiered storage: [`docs/STORAGE_POLICIES.md`](../../docs/STORAGE_POLICIES.md)  
