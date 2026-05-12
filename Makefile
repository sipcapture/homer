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

# Used only by target glibc-polyfill (plain make release/all does not patch).
# Example: make glibc-polyfill GLIBC_TARGET=2.28
GLIBC_TARGET ?= 2.34

# Static libstdc++ wrapper directory: contains ONLY libstdc++.a (no .so symlink).
# Placed first in the linker search path so that -lstdc++ (including the one
# hardcoded in DuckDB's #cgo LDFLAGS) resolves to the static archive instead of
# the shared library sitting next to it in /usr/lib/gcc/.../13/.
STATIC_CXX_DIR  := $(CURDIR)/.static-cxx-link
LIBSTDCXX_A     := $(shell gcc --print-file-name=libstdc++.a 2>/dev/null)
LIBLUAJIT_A     := $(shell gcc --print-file-name=libluajit-5.1.a 2>/dev/null)

# Prefer static LuaJIT (archive) while keeping other libs dynamic — matches golua's -lluajit-5.1.
CGO_LDFLAGS_STATIC_LUA := -Wl,-Bstatic -lluajit-5.1 -Wl,-Bdynamic

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

$(STATIC_CXX_DIR)/libluajit-5.1.a:
	@mkdir -p $(STATIC_CXX_DIR)
	@if [ -z "$(LIBLUAJIT_A)" ] || [ ! -f "$(LIBLUAJIT_A)" ]; then \
		echo "error: libluajit-5.1.a not found (install libluajit-5.1-dev)" >&2; \
		exit 1; \
	fi
	@ln -sf $(LIBLUAJIT_A) $(STATIC_CXX_DIR)/libluajit-5.1.a
	@echo "Static LuaJIT wrapper: $(STATIC_CXX_DIR)/libluajit-5.1.a -> $(LIBLUAJIT_A)"

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
release: frontend $(STATIC_CXX_DIR)/libstdc++.a $(STATIC_CXX_DIR)/libluajit-5.1.a
	cd src && CGO_LDFLAGS="-L$(STATIC_CXX_DIR) $(CGO_LDFLAGS_STATIC_LUA)" go build -ldflags "$(LDFLAGS_COMPAT)" -o ../$(NAME)

all: release

# Go binary only — run after frontend (Dockerfile calls: make frontend && make homer-only).
homer-only: $(STATIC_CXX_DIR)/libstdc++.a $(STATIC_CXX_DIR)/libluajit-5.1.a
	cd src && CGO_LDFLAGS="-L$(STATIC_CXX_DIR) $(CGO_LDFLAGS_STATIC_LUA)" go build -ldflags "$(LDFLAGS_COMPAT)" -o ../$(NAME)

# Patch existing binary with polyfill-glibc (not part of release/homer-only).
# Requires ./$(NAME); default target glibc is $(GLIBC_TARGET) (2.34). Override: make glibc-polyfill GLIBC_TARGET=2.28
glibc-polyfill:
	@if [ ! -f ./$(NAME) ]; then \
		echo "error: ./$(NAME) missing — run make release or make homer-only first"; \
		exit 1; \
	fi
	$(call apply-glibc-polyfill,./$(NAME))

# Alias for glibc-polyfill (same spelling as in docs/tools).
glibc-polyfil: glibc-polyfill

frontend:
	cd src/ui && npm install && npm run build

debug:
	cd src && go build -ldflags "-X main.VERSION_APPLICATION=$(VERSION) -X main.BuildDate=$(BUILD_DATE) -X main.BuildTime=$(BUILD_TIME) -X main.GitCommit=$(GIT_COMMIT)" -o ../$(NAME)

modules:
	cd src && go get ./...

.PHONY: all release homer-only clean frontend debug modules glibc-polyfill glibc-polyfil
clean:
	rm -fr $(NAME) src/dist $(STATIC_CXX_DIR)
