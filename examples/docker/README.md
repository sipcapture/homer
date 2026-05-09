# Homer + RustFS (Compose)

Docker Compose stack for **Homer** with **RustFS** as S3-compatible **cold** storage.

Configuration uses **only environment variables** (`HOMER_*`), not a mounted JSON file. Rules and naming are documented in [`docs/ENVIRONMENT_VARIABLES.md`](../../docs/ENVIRONMENT_VARIABLES.md) (prefix `HOMER`, dots → underscores, indexed slices). Tiering behaviour matches [`docs/STORAGE_POLICIES.md`](../../docs/STORAGE_POLICIES.md).

## Storage layout

| Tier | Where | Role |
|------|--------|------|
| **Hot** | Local disk `/data/homer/parquet` (`homer_data` volume) | New writes. |
| **Cold** | `s3://homer-cold/data/` on RustFS (`http://rustfs:9000`) | Older partitions (see `HOMER_STORAGE_DUCKLAKE_STORAGE_POLICY_*` in `.env.example`). |

RustFS runs **single-disk** (`rustfs_data` → `/data`, `RUSTFS_VOLUMES=/data`).

## Quick start

```bash
cd examples/docker
cp .env.example .env
# Edit secrets (JWT, tokens, RustFS keys if you change them from defaults)
docker compose up -d
```

You **must** copy `.env.example` to `.env` — the `homer` service loads **`env_file: .env`**.

- **Coordinator UI:** http://localhost:8080  
- **HEP / HTTP ingest:** UDP/TCP `9060`, HTTP `9080`  
- **RustFS S3 API:** http://localhost:9000 (`rustfs:9000` inside the stack)  
- **RustFS console:** http://localhost:9001  
- **Prometheus:** http://localhost:9090/metrics  

## Credentials

Match RustFS keys between `rustfs` (`RUSTFS_ACCESS_KEY` / `RUSTFS_SECRET_KEY` in `.env`) and Homer cold-tier variables (`HOMER_*_S3_ACCESS_KEY_ID`, `HOMER_*_S3_SECRET_ACCESS_KEY`). Defaults mirror [`docs/STORAGE_POLICIES.md`](../../docs/STORAGE_POLICIES.md) (`rustfsadmin`).

Create bucket **`homer-cold`** in RustFS if cold-tier writes fail until the bucket exists.

## References

- Env vars: [`docs/ENVIRONMENT_VARIABLES.md`](../../docs/ENVIRONMENT_VARIABLES.md)  
- Tiered storage: [`docs/STORAGE_POLICIES.md`](../../docs/STORAGE_POLICIES.md)  
- Images: `HOMER_IMAGE` in `.env`; RustFS `rustfs/rustfs:latest`.  
