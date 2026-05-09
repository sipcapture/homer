# Homer + RustFS (Compose)

Docker Compose stack for **Homer** (`ghcr.io/sipcapture/homer:latest`) with **RustFS** as S3-compatible **cold** storage. Tiering matches [`docs/STORAGE_POLICIES.md`](../../docs/STORAGE_POLICIES.md) (**Local + RustFS**).

## Storage layout (Homer)

Configured in `config/homer.json`:

| Tier | Where | Role |
|------|--------|------|
| **Hot** | Local disk `/data/homer/parquet` (`homer_data` volume) | New writes land here (lowest `priority`, volume `hot`). |
| **Cold** | `s3://homer-cold/data/` on RustFS (`http://rustfs:9000`) | Older partitions move here automatically. |

Homer uses **`storage.ducklake.storage_policy`** so the tiering service can:

1. Keep recent data on **local** parquet under `/data/homer/parquet`.
2. Move **older** date partitions to **RustFS** when they exceed **`max_data_age_days`** on the hot volume (here **7 days** — tune as needed).
3. Optionally also react when the hot volume fills past **`move_factor` × `max_size_gb`** (see the doc table).

The **`node.ducklake.volumes`** block lists the same **hot** (SQLite catalog + local path) and **cold** (SQLite catalog for cold + S3 prefix + endpoint credentials) so queries can union hot + cold.

RustFS runs in **single-node single-disk** mode: one Docker volume (`rustfs_data` → `/data`), matching [RustFS single-disk](https://docs.rustfs.com/installation/linux/single-node-single-disk.html) style setups — no four-disk compose layout.

## What this runs

| Service | Image | Purpose |
|--------|--------|---------|
| **homer** | `ghcr.io/sipcapture/homer:latest` | Ingest, DuckLake tiered storage, coordinator UI |
| **rustfs** | `rustfs/rustfs:latest` | S3 API + console for the cold tier |

## Quick start

```bash
cd examples/docker
cp .env.example .env
docker compose up -d
```

- **Coordinator UI:** http://localhost:8080  
- **HEP / HTTP ingest:** UDP/TCP `9060`, HTTP `9080`  
- **RustFS S3 API:** http://localhost:9000 (inside Compose: `rustfs:9000`)  
- **RustFS console:** http://localhost:9001  
- **Prometheus metrics (Homer):** http://localhost:9090/metrics  

## Credentials

Default RustFS keys match [`docs/STORAGE_POLICIES.md`](../../docs/STORAGE_POLICIES.md) (`rustfsadmin` / `rustfsadmin`) and are set again under **`storage_policy`** and **`node.ducklake`** for the cold volume in `config/homer.json`.

If you change `RUSTFS_ACCESS_KEY` / `RUSTFS_SECRET_KEY` in `.env`, update **`config/homer.json`** the same way (Homer does not read RustFS env vars).

Create the **`homer-cold`** bucket in RustFS (console or any S3 client) before relying on cold-tier writes, if your RustFS build does not auto-create buckets.

## References

- Tiered storage: [`docs/STORAGE_POLICIES.md`](../../docs/STORAGE_POLICIES.md)  
- Similar JSON (different host/port): [`examples/homer-writer-rustfs.json`](../homer-writer-rustfs.json)  
- Homer image: `ghcr.io/sipcapture/homer` — override with `HOMER_IMAGE` in `.env`.  
- RustFS: [Docker install](https://docs.rustfs.com/installation/docker).  
