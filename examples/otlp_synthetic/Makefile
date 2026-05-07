# OTLP synthetic payload generator — builds a rich Export*ServiceRequest for
# traces, metrics, and logs, then POSTs to an OTLP HTTP receiver (e.g. homer-core :4318).
#
# Usage:
#   make help
#   make build
#   make dry-run
#   make run URL=http://127.0.0.1:4318

.PHONY: help all tidy vet fmt build clean install dry-run dry-run-proto run run-proto

GO       ?= go
BIN_DIR  ?= bin
BINARY   := $(BIN_DIR)/otlp_synthetic
URL      ?= http://127.0.0.1:4318

help:
	@echo "Targets:"
	@echo "  make all            - tidy, vet, and build"
	@echo "  make tidy           - go mod tidy"
	@echo "  make vet            - go vet ./..."
	@echo "  make fmt            - go fmt ./..."
	@echo "  make build          - compile to $(BINARY)"
	@echo "  make install        - go install (GOBIN / GOPATH/bin)"
	@echo "  make clean          - remove $(BIN_DIR)/"
	@echo "  make dry-run        - serialize JSON only, print sizes (no HTTP)"
	@echo "  make dry-run-proto  - same with protobuf encoding"
	@echo "  make run            - POST JSON to URL (default: $(URL))"
	@echo "  make run-proto      - POST protobuf to URL"
	@echo "Variables: GO, BIN_DIR, URL"

all: tidy vet build

tidy:
	$(GO) mod tidy

vet:
	$(GO) vet ./...

fmt:
	$(GO) fmt ./...

$(BINARY): main.go go.mod go.sum
	mkdir -p $(BIN_DIR)
	$(GO) build -o $(BINARY) .

build: $(BINARY)

install:
	$(GO) install .

clean:
	rm -rf $(BIN_DIR)

dry-run:
	$(GO) run . -dry-run

dry-run-proto:
	$(GO) run . -dry-run -format proto

run: build
	$(BINARY) -url "$(URL)"

run-proto: build
	$(BINARY) -url "$(URL)" -format proto
