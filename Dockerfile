# Debian/glibc: DuckDB static libs expect glibc (backtrace, malloc_trim, resolver).
# Alpine/musl link fails. Runtime must match libc linked into the binary.
FROM debian:bullseye AS builder

# Base build deps (no Debian nodejs — distro package is too old for Vite 7).
RUN apt-get update && apt-get install -y --no-install-recommends \
    git ca-certificates build-essential curl \
    libluajit-5.1-dev \
    && apt-get clean \
    && rm -rf /var/lib/apt/lists/* /var/cache/apt/archives/*

# Go 1.26.x (bullseye has no official golang:1.26-bullseye image tag).
RUN set -eux; \
    arch="$(dpkg --print-architecture)"; \
    case "${arch}" in \
      amd64) goarch='amd64'; gosha='990e6b4bbba816dc3ee129eaeaf4b42f17c2800b88a2166c265ac1a200262282' ;; \
      arm64) goarch='arm64'; gosha='c958a1fe1b361391db163a485e21f5f228142d6f8b584f6bef89b26f66dc5b23' ;; \
      *) echo "unsupported architecture: ${arch}" >&2; exit 1 ;; \
    esac; \
    curl -fsSL "https://go.dev/dl/go1.26.2.linux-${goarch}.tar.gz" -o /tmp/go.tgz; \
    echo "${gosha}  /tmp/go.tgz" | sha256sum -c -; \
    tar -C /usr/local -xzf /tmp/go.tgz; \
    rm -f /tmp/go.tgz
ENV PATH="/usr/local/go/bin:${PATH}"

# Node.js 22 LTS + npm (matches CI node-version: 22; Tailwind oxide / Vite need current Node).
RUN curl -fsSL https://deb.nodesource.com/setup_22.x | bash - \
    && apt-get update \
    && apt-get install -y --no-install-recommends nodejs \
    && apt-get clean \
    && rm -rf /var/lib/apt/lists/* /var/cache/apt/archives/*

COPY . /homer-core
WORKDIR /homer-core
RUN make modules && make frontend && make homer-only

FROM debian:bullseye-slim AS runtime

RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates bash libluajit-5.1-2 \
    && apt-get clean \
    && rm -rf /var/lib/apt/lists/* /var/cache/apt/archives/*

RUN groupadd --system homer && useradd --system --gid homer --home-dir /nonexistent --shell /usr/sbin/nologin homer

WORKDIR /
COPY --from=builder /homer-core/homer .
COPY --from=builder /homer-core/src/dist /dist
USER homer
CMD ["/homer"]
