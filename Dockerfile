# Debian/glibc: DuckDB static libs expect glibc (backtrace, malloc_trim, resolver);
# Alpine/musl link fails. Runtime must match libc linked into the binary.
FROM golang:bookworm AS builder

# Base build deps (no Debian nodejs — bookworm ships Node 18; Vite 7 needs 20.19+ / 22.12+).
RUN apt-get update && apt-get install -y --no-install-recommends \
    git ca-certificates build-essential curl \
    libluajit-5.1-dev \
    && rm -rf /var/lib/apt/lists/*

# Node.js 22 LTS + npm (matches CI node-version: 22; Tailwind oxide / Vite need current Node).
RUN curl -fsSL https://deb.nodesource.com/setup_22.x | bash - \
    && apt-get install -y --no-install-recommends nodejs \
    && rm -rf /var/lib/apt/lists/*

COPY . /homer-core
WORKDIR /homer-core
RUN make modules && make all

FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates bash libluajit-5.1-2 \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /
COPY --from=builder /homer-core/homer .
COPY --from=builder /homer-core/src/dist /usr/local/homer-core/dist
RUN ln -s /usr/local/homer-core/dist /dist
CMD ["/homer"]
