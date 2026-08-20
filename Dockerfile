# Debian/glibc: DuckDB static libs expect glibc (backtrace, malloc_trim, resolver);
# Alpine/musl link fails. Runtime must match libc linked into the binary.
#
# Base tags are digest-pinned (GHSA-9c8w-qvmp-pvjj). Refresh digests when
# rebuilding: docker buildx imagetools inspect <image>:<tag>
FROM node:22-bookworm-slim@sha256:d649c27dae7ba0137b3cef5dd75baa422c08dc3d9e3fc0c23dfb172dc3cc6436 AS nodejs

FROM golang:bookworm@sha256:484ef6066fa69acb059fdfeda7ba2b8f7391f2ef6abc6f9b8411e669ebd56466 AS devbase

# Base build deps (no Debian nodejs — bookworm ships Node 18; Vite 7 needs 20.19+ / 22.12+).
# Node comes from the official image above — do not curl|bash NodeSource as root.
RUN apt-get update && apt-get install -y --no-install-recommends \
    git ca-certificates build-essential \
    libluajit-5.1-dev \
    && rm -rf /var/lib/apt/lists/*

COPY --from=nodejs /usr/local/bin/node /usr/local/bin/node
COPY --from=nodejs /usr/local/lib/node_modules /usr/local/lib/node_modules
RUN ln -sf ../lib/node_modules/npm/bin/npm-cli.js /usr/local/bin/npm \
    && ln -sf ../lib/node_modules/npm/bin/npx-cli.js /usr/local/bin/npx

FROM devbase AS builder

COPY . /homer-core
WORKDIR /homer-core
RUN make modules && make all

FROM debian:bookworm-slim@sha256:abd67ffcfa541b485a3dff59865ab629aa048a6c613e639d36e7456b0b229241

RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates bash sqlite3 \
    && rm -rf /var/lib/apt/lists/* \
    && groupadd --gid 1000 homer \
    && useradd --uid 1000 --gid 1000 --home-dir /data/homer --no-create-home --shell /usr/sbin/nologin homer \
    && mkdir -p /data/homer/.duckdb_spill \
    && chown -R homer:homer /data/homer

WORKDIR /
COPY --from=builder /homer-core/homer .
COPY --from=builder /homer-core/src/dist /usr/local/homer-core/dist
RUN ln -s /usr/local/homer-core/dist /dist \
    && chmod 0755 /homer

# DuckDB caps for the generic all-in-one image. Override at runtime, e.g.
#   docker run -e HOMER_STORAGE_DUCKLAKE_TUNING_MEMORY_LIMIT=8GB ...
# Give the container ~8GB RAM; raise MEMORY_LIMIT toward 50% of that budget
# under SIPREC / high ingest (see docs/OOM.md).
ENV HOMER_STORAGE_DUCKLAKE_TUNING_MEMORY_LIMIT=4GB \
    HOMER_STORAGE_DUCKLAKE_TUNING_THREADS=2 \
    HOMER_STORAGE_DUCKLAKE_TUNING_TEMP_DIRECTORY=/data/homer/.duckdb_spill \
    HOMER_NODE_DUCKLAKE_TUNING_THREADS=2 \
    HOMER_NODE_DUCKLAKE_TUNING_TEMP_DIRECTORY=/data/homer/.duckdb_spill

USER homer:homer
CMD ["/homer"]
