# HEP Server Performance Benchmarks

This document describes performance benchmarks for TCP, UDP, and TLS servers.

## Running Benchmarks

### Quick Start
```bash
./run_benchmarks.sh
```

### Manual Run

#### All protocols separately:
```bash
# TCP
go test -vet=off ./server -bench=BenchmarkTCPThroughput -benchtime=5s -run=^$ -v

# UDP  
go test -vet=off ./server -bench=BenchmarkUDPThroughput -benchtime=5s -run=^$ -v

# TLS
go test -vet=off ./server -bench=BenchmarkTLSThroughput -benchtime=5s -run=^$ -v
```

#### All together:
```bash
go test -vet=off ./server -bench=BenchmarkComparison -benchtime=5s -run=^$ -v
```

#### With concurrent connections:
```bash
go test -vet=off ./server -bench=BenchmarkTCPConcurrentConnections -benchtime=5s -run=^$ -v
```

## Benchmark Parameters

- **Duration**: 5 seconds (configurable via `benchmarkDuration`)
- **Packet size**: 100 bytes payload + 6 bytes HEP header = 106 bytes total
- **TCP**: Uses gnet with multiprocessing
- **UDP**: Uses gnet event loop
- **TLS**: Uses gnet + gnet-io/tls

## Metrics

Benchmarks measure:
- **packets/sec** - number of processed packets per second
- **packets** - total number of processed packets
- **Sent packets** - number of sent packets
- **Processed packets** - number of packets processed by the server

## Expected Results

Typical results on modern hardware:

- **UDP**: Fastest (~100k-500k packets/sec)
- **TCP**: High performance (~50k-200k packets/sec)
- **TLS**: Slower due to encryption (~10k-50k packets/sec)

*Note: Actual results depend on hardware, load, and packet size.*

## Interpreting Results

1. **UDP** should be the fastest, as there is no connection overhead
2. **TCP** is slower than UDP due to connection establishment, but faster than TLS
3. **TLS** is the slowest due to encryption overhead and handshake

## Troubleshooting

If benchmarks fail to run:

1. Make sure the code compiles: `go build .`
2. Check that ports 19060-19063 are free
3. Increase timeout if needed: `timeout 120 go test ...`
