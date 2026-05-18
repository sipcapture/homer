# HEP UDP/TCP ingest — Mpps benchmark and bottleneck analysis

> Test bench: Intel Core Ultra 9 185H (22 logical cores), Linux 6.8,
> loopback; after `sudo ./scripts/tune-udp-sysctl.sh`:
> `net.core.rmem_max = 33554432` (32 MiB).
> Benchmarks live in `src/server/mpps_benchmark_test.go`. See
> [Reproduce](#reproduce) for exact commands.

The numbers below answer two questions:

1. **What end-to-end Mpps can a single Homer ingest process sustain
   for HEP3 UDP and TCP?**
2. **Which line of code is the actual bottleneck?**

The short version (after pool + metrics batching fixes, 2026-05-19):

- **TCP peak: ~1.46 Mpps (≈8.6 Gbps) at 16 workers / 16 connections.**
- **UDP peak (sysctl tuned): ~0.83 Mpps at 16w/16s**; best drop rate on
  **1 sender: ~0.53 Mpps, ~18% drop** (w2_s1). Multi-sender loopback
  still drops ~80%+ (senders faster than decode, not rmem-limited).
- **Theoretical decode ceiling: ≈ 3.9 Mpps** (`BenchmarkHEPDecodeOnly`,
  259 ns/op after pooling; was 502 ns / 8 allocs before).
- **Before fixes** (broken per-server `packetPool` + per-packet Prometheus):
  TCP ~0.41 Mpps, UDP ~0.35 Mpps on the same host.

Remaining limits:

1. Kernel UDP `net.core.rmem_max` (208 KiB default) — still causes
   drops under burst load.
2. Decoder / DuckLake writer once receive path is no longer allocating
   per packet.

## Headline numbers (after fixes)

### UDP HEP (pool fix + `tune-udp-sysctl.sh`, loopback)

| workers | senders | hep Mpps | drop %  | bandwidth |
| ------: | ------: | -------: | ------: | --------: |
|       2 |       1 |    0.529 |   17.8% | 3121 Mbps |
|       4 |       1 |    0.469 |   26.8% | 2771 Mbps |
|       4 |       4 |    0.679 |   73.7% | 4008 Mbps |
|       8 |       8 |    0.809 |   82.5% | 4774 Mbps |
|      16 |      16 |    0.828 |   85.8% | 4886 Mbps |

With default `rmem_max=212992` (before sysctl), the same matrix showed
~86–94% drop and ~0.12–0.35 Mpps — raising rmem fixes **single-sender**
paths first.

### TCP HEP (single process, loopback connections)

| workers | senders | hep Mpps | bandwidth |
| ------: | ------: | -------: | --------: |
|       2 |       1 |    0.393 | 2320 Mbps |
|       4 |       1 |    0.406 | 2399 Mbps |
|       4 |       4 |    1.256 | 7414 Mbps |
|       8 |       8 |    1.326 | 7831 Mbps |
|      16 |      16 |    1.458 | 8607 Mbps |

TCP has zero packet loss (kernel back-pressures the sender). Above
8 workers / 8 connections, throughput drops because event loop and
worker scheduling contention starts winning.

### Pure decode ceiling (no networking, no channels)

| benchmark                    | ns/op |          throughput |
| ---------------------------- | ----: | ------------------: |
| `BenchmarkHEPDecodeSerial`   |  1675 |   ≈ 597 k pps / core |
| `BenchmarkHEPDecodeOnly`     |   502 | **≈ 1.99 Mpps total** (22 cores) |

8 allocations per HEP3 packet, mostly from the SIP zero-copy parser.
This is the upper bound the rest of the pipeline competes against.

## Where the CPU goes

CPU profile from `BenchmarkUDPMpps/w8_s8` (5-second window, 84.8 s of
combined CPU time):

| component                          | cum %  | notes                                  |
| ---------------------------------- | -----: | -------------------------------------- |
| `runMppsUDP.func1` (test senders)  | 55.7 % | loopback `net.conn.Write` + syscalls    |
| `gnet` event loops (receive side)  | 30.3 % | poll + accept + readUDP                |
| `(*udpServer).OnTraffic`           | 25.2 % | of which:                              |
| ↳ `packetPool.Get` → `mallocgc`    | 18.85 s | **fresh 64 KiB allocation per packet** |
| ↳ Prometheus metric updates (×3)   |  1.18 s | hot-path counter / histogram updates   |
| ↳ `inputCh <- pkt`                  |  0.79 s | channel send                           |
| ↳ `time.Now()`                     |  0.15 s |                                        |
| `(*HEPInput).worker`               |  7.6 % |                                        |
| ↳ `decoder.DecodeHEP`              |  5.08 s | dominated by SIP parse                 |

Two things stand out: workers are **not** the bottleneck (they spend
~16 % of their on-CPU time waiting in `select`), and the receive
path spends 88 % of its time on `packetPool.Get` falling through to
`runtime.mallocgc`.

## Pool mismatch (fixed 2026-05-19)

**Status: fixed.** All gnet receivers now use `HEPInput.getPacketBuf()`
/ `putPacketBuf()` (shared `h.buffer` pool). Per-server `packetPool`
was removed from UDP/TCP/TLS.

Historical note — before the fix, `src/server/udp.go`,
`src/server/tcp.go`, `src/server/tls.go` each created a per-server
`packetPool` and `Get()` from it on every `OnTraffic` event:

```118:122:src/server/udp.go
	// Copy packet data into pooled buffer (Next buffer is reused by gnet)
	buf := us.packetPool.Get().([]byte)
	buf = buf[:len(packet)]
	copy(buf, packet)
```

The decoder worker, however, returns the buffer to a **different**
pool — `h.buffer`, which is created in `NewHEPInput`:

```326:367:src/server/server.go
				if cap(msg.data) >= maxPktLen {
					h.buffer.Put(msg.data[:maxPktLen])
				}
```

`h.buffer` is never read by anyone on the hot path, so every UDP/TCP
packet effectively allocates a fresh 64 KiB buffer
(`maxPktLen = 65535`) and the GC reclaims it later. At ~300 k pps
that's ~18 GiB/s of byte-buffer allocations going through `mallocgc`,
which matches the profile exactly.

**Applied fix:** single shared buffer pool (`getPacketBuf` /
`putPacketBuf` on `HEPInput`). Measured speed-up on the same hardware:
≈ **2.6× UDP** and **3.2× TCP** Mpps (loopback, before sysctl tuning).

Receive-path Prometheus updates are batched via `ingestReceiveMetrics`
(`ingest_metrics.go`, flush every 128 packets, labels resolved once).

## P2: SIP / HEP decode pooling (2026-05-19)

Changes: `HEP` + `SipMsg` `sync.Pool`, `ReleaseHEP()` after worker/write,
`ipv4BytesToString` (no `net.IP` on hot path).

| metric | before | after |
|--------|--------|-------|
| `BenchmarkHEPDecodeOnly` | 502 ns, 8 allocs, 1872 B | **259 ns, 6 allocs, 722 B** |
| theoretical decode Mpps | ~2.0 | **~3.9** |

Mandatory pairing: every `DecodeHEP` / `Decoder.Decode` on the hot path must
call `decoder.ReleaseHEP(hep)` when done (ingest worker, writer, benchmarks).

## P1: DuckLake `WriteHEP` profile (2026-05-19)

Benchmarks in `src/storage/ducklake/writehep_bench_test.go` (warmup excludes
first-table `CREATE` / `Exec`).

| benchmark | ns/op | allocs | ~pps |
|-----------|------:|-------:|-----:|
| `BenchmarkWriteHEP_SIP` (decode once) | ~7.1 µs | 31 | ~140k |
| `BenchmarkDecodeAndWriteHEP_SIP` | ~8.3 µs | 37 | **~120k** |

**CPU (steady state):** decode+SIP parse ~45–50%; DuckLake append/batch
(`TableWriter.Write` / Appender) ~35–40%; remainder mutex/scheduling.

**Allocations per WriteHEP:** `buildExtraJSON` → `string(b)` copy (~required
for row lifetime), `Payload`/`CID` string copies from HEP parse, `fastUUID`,
pooled `[]interface{}` row. `flushBatch` dominates alloc profile only when
the batch fills (every 10k rows by default).

**Implication:** on a writer node with DuckLake enabled, **~120k SIP pps per
core** end-to-end is a realistic loopback ceiling before batch flush / catalog
IO; multi-core scales with `worker_count` and `shard_count`.

```bash
go test -vet=off -tags='!vet' -run='^$' -bench=BenchmarkDecodeAndWriteHEP_SIP \
    -benchmem -benchtime=3s ./storage/ducklake/
```

## P3: deferred `data_extra` + HTTP ingest (2026-05-19)

- **`buildExtraJSONCell`**: SIP `data_extra` stored as pooled `*[]byte` on the
  WriteHEP path; `string()` conversion moved to `flushBatch` via
  `cellToDriverValue` (one alloc per row at flush, not per enqueue).
- **HTTP/HTTPS**: batched receive metrics (`ingestReceiveMetrics`), shared
  buffer pool (`copyPacketToPool`), common `handleIngestPOST`.

## Secondary bottlenecks

After pool + metrics + decode pooling + deferred JSON, the next ceilings are:

- **Kernel UDP socket buffer.** With `net.core.rmem_max = 212992`,
  gnet's `SO_RCVBUF` request is silently capped. Drops happen as soon
  as a single event-loop pause exceeds ~1 ms. Recommended sysctl for
  high-rate captures:

  ```bash
  sudo ./scripts/tune-udp-sysctl.sh
  # persistent:
  sudo cp examples/sysctl/99-homer-udp-buffers.conf /etc/sysctl.d/
  sudo sysctl --system
  ```

  Homer config already defaults `socket_recv_buffer` to 8 MiB; after
  sysctl, gnet will get the full value instead of being capped at
  `rmem_max`.

- **Prometheus hot-path metrics.** `RecordHEPPacketReceived`,
  `RecordHEPPacketSize`, and `RecordBytesReceived` together cost
  ~1.2 s of the 5 s window — i.e. **~18 % of OnTraffic time** even
  before the pool fix. Two cheap wins:
  - Batch the size histogram (`HEPPacketSize.Observe`) into a
    per-worker accumulator, flushed at the same cadence as
    `serverWorkerMetrics`.
  - Skip `WithLabelValues("udp")` resolution per packet by caching
    the curried child once per `udpServer`.

- **`buildExtraJSON` copy** and **batch flush** on the DuckLake path
  (see P1 above). Tune `batch_size` / `flush_interval`.

## Reproduce

```bash
# Pure decode ceiling
cd src
go test -vet=off -tags='!vet' -run='^$' \
    -bench='BenchmarkHEPDecode(Serial|Only)' -benchtime=3s \
    -timeout=120s ./server/

# UDP Mpps matrix
go test -vet=off -tags='!vet' -run='^$' \
    -bench=BenchmarkUDPMpps -benchtime=1x -timeout=180s ./server/

# TCP Mpps matrix
go test -vet=off -tags='!vet' -run='^$' \
    -bench=BenchmarkTCPMpps -benchtime=1x -timeout=180s ./server/

# CPU profile for the worst-loaded UDP case
go test -vet=off -tags='!vet' -run='^$' \
    -bench=BenchmarkUDPMpps/w8_s8 -benchtime=1x \
    -cpuprofile=cpu_udp.prof ./server/
go tool pprof -top -cum cpu_udp.prof | head -40
go tool pprof -list 'OnTraffic' cpu_udp.prof
```

The benchmark uses a real HEP3-encapsulated SIP `INVITE` (738 B
total, including SDP) so the decoder and SIP parser execute the same
paths they would in production. Each sub-bench warms up for 1 s, then
samples `HEPCount` over a 5 s window for steady-state Mpps.
