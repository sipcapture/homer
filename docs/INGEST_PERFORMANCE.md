# Ingest and writer CPU tuning (modular config)

When the HEP writer is under very high packet rates, CPU time is dominated by decoding, DuckLake appends, and (to a lesser extent) Prometheus counter updates on the ingest path. This PR adjusts several defaults (see sections below for the new values); operators can further tune behaviour via JSON / YAML / environment variables.

## `ingest.udp.multicore` / `ingest.tcp.multicore`

When `true` (also the Viper default), the UDP/TCP listeners use a multi-loop / multi-reactor style setup appropriate for many-core hosts. Set to `false` on small VMs or when debugging single-threaded behaviour.

Environment override: `HOMER_INGEST_UDP_MULTICORE`, `HOMER_INGEST_TCP_MULTICORE`.

## `storage.ducklake.batch_size`

Rows are buffered in memory until the batch reaches this size (or the flush interval fires). **Larger batches** amortise catalog/append work and can **lower CPU per packet** at the cost of **higher latency** before data is visible and **more RAM** per table writer.

Default in Viper: `10000`. Tune down for low-latency visibility; tune up (within memory limits) for sustained multi‑Gbps ingest after measuring with `go tool pprof`.

Environment override: `HOMER_STORAGE_DUCKLAKE_BATCH_SIZE`.

### Production / lab stand

For sustained high PPS on an all-in-one host with 2GB+ DuckDB memory limit,
**25000–50000** is a reasonable starting range (default example config uses
**25000**). Fewer flushes reduce DuckLake append overhead; visibility latency
grows until the batch fills or `flush_interval_sec` fires.

Apply on a `homer-core` systemd install:

```bash
sudo ./scripts/apply-stand-ducklake-batch.sh 25000
# or: sudo ./scripts/apply-stand-ducklake-batch.sh 50000
```

Rebuild and reinstall the package after pulling ingest perf commits so the
binary matches the repo (`homer --version` should show the current commit).

## `ingest.worker_metrics_flush_packets`

Writer workers batch updates to Prometheus counters (`homer_hep_packets_received_total`, `homer_hep_packets_processed_total`, `homer_bytes_received_total`, …) so the hot path does not hit atomics on every packet.

- **`0` or omitted:** use the built-in default (**128** packets per flush).
- **Positive value:** flush after that many packets (per worker, per protocol label batch). Values above **1048576** are capped at **1048576**.

Raising this (for example **256–1024**) can **reduce CPU** from Prometheus scraping/update overhead on extreme PPS; metrics become **coarser** between scrapes.

Environment override: `HOMER_INGEST_WORKER_METRICS_FLUSH_PACKETS`.

## Example fragment

```json
{
  "ingest": {
    "worker_metrics_flush_packets": 512,
    "udp": { "enable": true, "multicore": true },
    "tcp": { "enable": true, "multicore": true }
  },
  "storage": {
    "ducklake": {
      "batch_size": 4000
    }
  }
}
```

After changes, validate with `go tool pprof` (`homer --pprof=127.0.0.1:6060`) and watch queue depth / drop counters.

## Repeatable ingest CPU profile (script)

From the **repository root** (with `./homer` built, e.g. `make`):

```bash
# Frees default homer-check ports, starts homer with --pprof, runs UDP load, writes profile + top text under /tmp/homer-profile-ingest/
./scripts/profile_ingest_load.sh --kill-ports

# Or via Makefile (same as above)
make profile-ingest
```

Useful overrides (environment):

| Variable | Default | Meaning |
|----------|---------|--------|
| `HOMER` | `$REPO_ROOT/homer` | Path to `homer` binary |
| `CONFIG` | `$REPO_ROOT/homer-check.json` | Modular JSON config |
| `PPROF_ADDR` | `127.0.0.1:6066` | `--pprof` listen address |
| `UDP_ADDR` | `127.0.0.1:19060` | HEP UDP target for the load tool |
| `PPS` | `12000` | Target datagrams/sec |
| `PROFILE_SEC` | `22` | `profile?seconds=` duration |
| `LOAD_SEC` | `24` | How long the UDP generator runs |
| `OUT_DIR` | `/tmp/homer-profile-ingest` | Profile `.pb.gz`, `pprof-top.txt`, `homer.log` |
| `SKIP_HOMER` | `0` | Set to `1` if homer is already running (same `--pprof` URL must respond) |

The load generator is **`go run ./cmd/hepudpload`** from `src/` (minimal HEP3 + SIP INVITE). Manual run:

```bash
cd src && go run ./cmd/hepudpload -addr=127.0.0.1:19060 -pps=12000 -duration=30s
```

Artifacts after the script: `cpu.pb.gz`, `pprof-top.txt`, `homer.log` under `OUT_DIR`. Interactive view: `go tool pprof -http=:0 "$OUT_DIR/cpu.pb.gz"`.

## `duckdb-go-bindings`: upstream vs fork

Homer pulls DuckDB’s CGO stack through **`github.com/duckdb/duckdb-go/v2`**, which depends on the prebuilt static libs in **[`github.com/duckdb/duckdb-go-bindings`](https://github.com/duckdb/duckdb-go-bindings)** (see that repo for versioning, e.g. DuckDB `v1.5.5` → module tag **`v0.10505.0`**).

By default **`src/go.mod` contains a `replace` directive** pointing to a fork that eliminates per-string CGO malloc/free in the Appender hot path (visible as `VectorAssignStringElementLen` + `duckdb_free` per-column in profiles). This fork has been benchmarked against upstream and showed a measurable reduction in CGO overhead at high PPS.

Then `go mod tidy`, rebuild, and compare with **`./scripts/profile_ingest_load.sh`** using the same `PROFILE_SEC`, `PPS`, and `OUT_DIR` naming. Use a **warm-up** (send traffic for several seconds before `profile?seconds=`) and **≥20–30 s** profiles so `runtime.cgocall` / Appender rows dominate over one-off init noise.

Example A/B on one machine (same `PPS`/`PROFILE_SEC`): upstream vs fork showed **~1%** difference in total CPU sample time over 10 s (within run-to-run variance); treat small deltas as inconclusive until you repeat on your hardware and workload mix.
