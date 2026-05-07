# otlp_synthetic

Small Go tool that builds a **rich OTLP/HTTP export** (traces, metrics, logs) and POSTs it to an OTLP receiver—for example **homer-core** on port `4318`.

## Prerequisites

- Go 1.22+ (see `go.mod`)
- A running OTLP HTTP endpoint (default `http://127.0.0.1:4318`) with paths `/v1/traces`, `/v1/metrics`, `/v1/logs`

## Quick start

```bash
cd examples/otlp_synthetic
make build          # binary: bin/otlp_synthetic
make dry-run        # marshal only, print payload sizes (no network)
make run            # POST JSON to http://127.0.0.1:4318
```

Or without Make:

```bash
cd examples/otlp_synthetic
go run . -dry-run
go run .
```

## Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-url` | `http://127.0.0.1:4318` | OTLP HTTP base URL (no trailing `/`) |
| `-format` | `json` | `json` or `proto` (`application/json` vs `application/x-protobuf`) |
| `-dry-run` | off | Only serialize payloads and print sizes; do not send |

Example:

```bash
./bin/otlp_synthetic -url http://192.168.1.10:4318 -format proto
```

With Make:

```bash
make run URL=http://192.168.1.10:4318
make run-proto URL=http://127.0.0.1:4318
```

## Dashboard metric chart

After ingesting metrics, open the **Homer Core** UI dashboard: add **OTLP Metric Search** (Protocol Search preset), set the dashboard time range, pick a metric name from the loaded list (or type it), then **Search**. Add a **Time Chart** widget, set **Target** in the search widget to that chart, and run **Search** again — the chart aggregates points (default Y prefers `value_double` / `value_int`). Optionally narrow the metric name list with **Service name** before loading names.

## Make targets

Run `make help` for the full list (`all`, `tidy`, `vet`, `fmt`, `clean`, `install`, `dry-run-proto`, etc.).
