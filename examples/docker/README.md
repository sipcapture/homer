# Homer + RustFS (Compose)

Docker Compose stack for **Homer** (`ghcr.io/sipcapture/homer:latest`) with **RustFS** as S3-compatible **cold** storage, following the tiered layout in [`docs/STORAGE_POLICIES.md`](../../docs/STORAGE_POLICIES.md) (see **Local + RustFS**).

## What this runs

| Service | Image | Purpose |
|--------|--------|---------|
| **homer** | `ghcr.io/sipcapture/homer:latest` | HEP ingest, DuckLake hot volume on disk, coordinator UI |
| **rustfs** | `rustfs/rustfs:latest` | S3 API for the cold tier (`s3://homer-cold/data/`) |

Hot data stays under `/data/homer/parquet` in the `homer_data` volume. Older partitions are tiered to RustFS per `storage_policy` (same semantics as the doc: TTL interval, `move_factor`, volumes).

## Quick start

```bash
cd examples/docker
cp .env.example .env
docker compose up -d
```

- **Coordinator UI:** http://localhost:8080  
- **HEP / HTTP ingest:** UDP/TCP `9060`, HTTP `9080`  
- **RustFS S3 API:** http://localhost:9000 (from the host; inside Compose the hostname is `rustfs:9000`)  
- **RustFS console:** http://localhost:9001  
- **Prometheus metrics (Homer):** http://localhost:9090/metrics  

## Credentials and cold tier

Default RustFS keys match [`docs/STORAGE_POLICIES.md`](../../docs/STORAGE_POLICIES.md) (`rustfsadmin` / `rustfsadmin`) and are repeated in `config/homer.json` for `s3_access_key_id` / `s3_secret_access_key`.

If you change `RUSTFS_ACCESS_KEY` / `RUSTFS_SECRET_KEY` in `.env`, update **`config/homer.json`** the same way (Homer does not read RustFS env vars).

Create the **`homer-cold`** bucket in RustFS (console or any S3 client) before relying on cold-tier writes, if your RustFS build does not auto-create buckets.

## Configuration reference

- Tiered storage options: [`docs/STORAGE_POLICIES.md`](../../docs/STORAGE_POLICIES.md)  
- Similar JSON shape (different endpoint): [`examples/homer-writer-rustfs.json`](../homer-writer-rustfs.json)  

## Images

- Homer: `ghcr.io/sipcapture/homer` — override with `HOMER_IMAGE` in `.env`.  
- RustFS: upstream publishes `rustfs/rustfs` (see [RustFS Docker install](https://docs.rustfs.com/installation/docker) and [compose-simple](https://github.com/rustfs/rustfs/blob/main/docker-compose-simple.yml)).  
