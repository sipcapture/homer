NAME=homer

# Without this, the first rule ($(STATIC_CXX_DIR)/libstdc++.a) becomes the default
# and plain `make` only prepares the static C++ wrapper — not the binary/UI.
.DEFAULT_GOAL := all

# Version variables
VERSION ?= $(shell grep 'VERSION_APPLICATION = ' src/version.go | head -1 | cut -d'"' -f2)
BUILD_DATE := $(shell date +%Y-%m-%d)
BUILD_TIME := $(shell date +%H:%M:%S)
GIT_COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")

# Build flags
LDFLAGS := -s -w -X main.VERSION_APPLICATION=$(VERSION) -X main.BuildDate=$(BUILD_DATE) -X main.BuildTime=$(BUILD_TIME) -X main.GitCommit=$(GIT_COMMIT)

# GLIBC polyfill target version — set to match the oldest supported distro.
# RHEL 8 / AlmaLinux 8 / Rocky Linux 8 ship GLIBC 2.28.
# RHEL 9 ships GLIBC 2.34. Override via: make GLIBC_TARGET=2.34
GLIBC_TARGET ?= 2.28

# Static libstdc++ wrapper directory: contains ONLY libstdc++.a (no .so symlink).
# Placed first in the linker search path so that -lstdc++ (including the one
# hardcoded in DuckDB's #cgo LDFLAGS) resolves to the static archive instead of
# the shared library sitting next to it in /usr/lib/gcc/.../13/.
STATIC_CXX_DIR  := $(CURDIR)/.static-cxx-link
LIBSTDCXX_A     := $(shell gcc --print-file-name=libstdc++.a 2>/dev/null)

# Static C++ runtime compatibility flags.
# -static-libgcc: eliminate libgcc_s.so.1 runtime dependency.
# Static libstdc++ is handled via STATIC_CXX_DIR (wrapper-dir trick).
# -ldl: required by glibc_compat.go which uses dlsym(RTLD_NEXT, ...) to proxy
#        _dl_find_object to the real glibc implementation on modern systems.
LDFLAGS_COMPAT := $(LDFLAGS) -extldflags '-static-libgcc -ldl'

$(STATIC_CXX_DIR)/libstdc++.a:
	@mkdir -p $(STATIC_CXX_DIR)
	@if [ -n "$(LIBSTDCXX_A)" ] && [ -f "$(LIBSTDCXX_A)" ]; then \
		ln -sf $(LIBSTDCXX_A) $(STATIC_CXX_DIR)/libstdc++.a; \
		echo "Static libstdc++ wrapper: $(STATIC_CXX_DIR)/libstdc++.a -> $(LIBSTDCXX_A)"; \
	else \
		echo "Warning: libstdc++.a not found — static C++ linking skipped"; \
	fi

define apply-glibc-polyfill
	@if command -v polyfill-glibc >/dev/null 2>&1; then \
		echo "Applying glibc polyfill (target=$(GLIBC_TARGET))..."; \
		polyfill-glibc --target-glibc=$(GLIBC_TARGET) $(1) || \
		{ echo "Warning: polyfill-glibc failed, continuing without glibc patching..."; true; }; \
	else \
		echo "polyfill-glibc not found, skipping glibc compatibility patch..."; \
	fi
endef

# go:embed in static.go requires src/dist; "all" must build the UI first (Dockerfile and CI use make all).
release: frontend $(STATIC_CXX_DIR)/libstdc++.a
	cd src && CGO_LDFLAGS="-L$(STATIC_CXX_DIR)" go build -ldflags "$(LDFLAGS_COMPAT)" -o ../$(NAME)
	$(call apply-glibc-polyfill,./$(NAME))

all: release

# Go binary only — run after frontend (Dockerfile calls: make frontend && make homer-only).
homer-only: $(STATIC_CXX_DIR)/libstdc++.a
	cd src && CGO_LDFLAGS="-L$(STATIC_CXX_DIR)" go build -ldflags "$(LDFLAGS_COMPAT)" -o ../$(NAME)
	$(call apply-glibc-polyfill,./$(NAME))

frontend:
	cd src/ui && npm install && npm run build

debug:
	cd src && go build -ldflags "-X main.VERSION_APPLICATION=$(VERSION) -X main.BuildDate=$(BUILD_DATE) -X main.BuildTime=$(BUILD_TIME) -X main.GitCommit=$(GIT_COMMIT)" -o ../$(NAME)

modules:
	cd src && go get ./...

.PHONY: all release homer-only clean frontend debug modules
clean:
	rm -fr $(NAME) src/dist $(STATIC_CXX_DIR)
